/*
 * API 即时检索。
 *
 * 为什么要自建：Material 内置搜索按「整页文本」建索引，搜 `InjectText`
 * 会把所有提到它的页面都列出来，但**分不清哪一条是它的定义**；而且内置
 * 索引要等 mkdocs build 才生成，改一行 API 也得重建。
 *
 * 这里读的是 `assets/api-index.json`——由 tools/apidoc/gensite 直接产出，
 * 每条记录带 名称/签名/描述/类别/所属页面/是否仅内置/源文件:行号。
 * 因此可以做到：
 *   - 按名称搜（精确/前缀优先）
 *   - 按描述搜（中文按字、英文按词，都对 API 的文档注释做匹配）
 *   - 按签名搜（如 "(string) error"）
 *   - 过滤「仅内置」——外部插件作者最容易被这个绊住
 *
 * 设计取舍：纯前端、零依赖、不阻塞页面。索引 ~130 条、约 40KB，一次拉取足够。
 */
(function () {
  "use strict";

  var INDEX_URL = (function () {
    // 文档站可能部署在子路径下，按当前页面深度回推到站点根。
    var path = window.location.pathname;
    var marker = "/api/";
    var i = path.indexOf(marker);
    if (i >= 0) return path.slice(0, i) + "/assets/api-index.json";
    // guide/ 等目录同样回退一层。
    var lastSlash = path.lastIndexOf("/");
    return path.slice(0, lastSlash) + "/assets/api-index.json";
  })();

  var state = { all: [], loaded: false, loading: false };

  function load() {
    if (state.loaded || state.loading) return Promise.resolve(state.all);
    state.loading = true;
    return fetch(INDEX_URL)
      .then(function (r) {
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      })
      .then(function (data) {
        state.all = data || [];
        state.loaded = true;
        return state.all;
      })
      .catch(function () {
        state.all = [];
        return [];
      });
  }

  /* ---------- 打分 ---------- */
  //
  // 三级优先级：名称命中 > 描述命中 > 签名命中。
  // 名称命中里再分「完全相等 / 前缀 / 子串」，因为用户敲 `InjectText` 时
  // 想要的是那个符号，不是所有名字里含它的。

  function score(item, q) {
    var name = (item.n || "").toLowerCase();
    var ql = q.toLowerCase();
    var s = 0;

    if (name === ql) s += 1000;
    else if (name.indexOf(ql) === 0) s += 600;
    else if (name.indexOf(ql) > 0) s += 350;

    // 中文检索关键词（keywords.json 产出，字段 g）。
    // 为什么需要：SDK 里 66/100 个符号是英文注释（`RegisterTool registers a
    // tool that the LLM can call.`），懂中文的人搜「注册工具」会一条都找不到。
    // 关键词命中给较高权重（仅次于名称精确命中），因为它就是为「按功能找」准备的。
    var kws = item.g || [];
    for (var ki = 0; ki < kws.length; ki++) {
      var kw = String(kws[ki]).toLowerCase();
      if (kw === ql) { s += 480; break; }
      if (kw.indexOf(ql) >= 0) { s += 300; break; }
    }

    // 限定符：PluginSDK.RegisterTool / IOInjector.InjectText。
    // 额外支持「去掉 API/SDK 后缀」与「去掉点号」两种写法，
    // 因为读者习惯写 `memory.recall`（RPC 名），而 Go 名是 `MemoryAPI.Recall`。
    var qual = ((item.r || "") + "." + name).toLowerCase();
    if (item.r && qual.indexOf(ql) >= 0) s += 200;
    if (item.r) {
      var flat = qual.replace(/[._]/g, "").replace(/apis?dk|sdk|api/g, "");
      var qflat = ql.replace(/[._\s]/g, "");
      if (qflat && flat.indexOf(qflat) >= 0) s += 180;
    }

    var desc = (item.d || "").toLowerCase();
    if (desc.indexOf(ql) >= 0) s += 120;

    // 签名按 token 匹配：把查询拆词（去掉括号/逗号等标点），全部命中才算。
    // 这样 `(string) error`、`ContentBlock 媒体` 这类片段都能搜到。
    // 注意必须先去标点：否则 token `(string)` 永远匹配不到签名里的 `string`。
    var sig = (item.s || "").toLowerCase();
    if (sig.indexOf(ql) >= 0) s += 60;
    var toks = ql
      .replace(/[()\[\]{},;:]/g, " ")
      .split(/\s+/)
      .filter(function (t) { return t.length > 1; });
    if (toks.length && sig.length) {
      var allSig = toks.every(function (t) { return sig.indexOf(t) >= 0; });
      if (allSig) s += 55;
    }

    // 中文按字匹配：中文没有词边界，逐字命中比整串更实用。
    // 注意只把它当作**弱信号**：光靠逐字会把「注册工具」匹到凡是含「工具」
    // 字样的任何东西（实测 ContextPolicyNone 的说明里有「工具调用」也会命中）。
    // 所以阈值卡在 60% 以上才算有效命中。
    if (/[\u4e00-\u9fa5]/.test(q)) {
      var hit = 0;
      var hay = desc + " " + kws.join(" ");
      for (var i = 0; i < q.length; i++) {
        if (hay.indexOf(q[i]) >= 0) hit++;
      }
      var ratio = hit / q.length;
      if (ratio >= 0.6) s += Math.round(hit * 6);
    }

    // 公开 API 略优先于「仅内置」——后者通常是噪声。
    if (s > 0 && !item.b) s += 15;
    return s;
  }

  function search(q) {
    var qq = (q || "").trim();
    if (!qq) return [];
    var out = [];
    for (var i = 0; i < state.all.length; i++) {
      var sc = score(state.all[i], qq);
      if (sc > 0) out.push({ item: state.all[i], score: sc });
    }
    out.sort(function (a, b) {
      if (b.score !== a.score) return b.score - a.score;
      return (a.item.n || "").length - (b.item.n || "").length;
    });
    return out;
  }

  /* ---------- 渲染 ---------- */
  //
  // 挂在 Material 首页/目录页的一个容器上：#api-search。
  // 没找到容器就不做任何事——这样同一份 JS 可以安全地全站引入。

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  }

  function render(mount, q) {
    mount.innerHTML = "";
    if (!q.trim()) {
      mount.appendChild(el("p", "api-hint",
        "输入 API 名称、描述或签名片段。例：InjectText、注册工具、崩溃、memory.recall、ContentBlock"));
      return;
    }
    var results = search(q);
    if (!results.length) {
      mount.appendChild(el("p", "api-hint", "没有匹配的 API。试试更短的词，或按功能描述搜（如「注入」「重载」）。"));
      return;
    }
    var head = el("p", "api-count", "命中 " + results.length + " 个 API");
    mount.appendChild(head);

    var list = el("ul", "api-results");
    results.slice(0, 40).forEach(function (r) {
      var it = r.item;
      var li = el("li", "api-result");

      var title = el("a", "api-name", (it.r ? it.r + "." : "") + it.n);
      // 锚点必须用**完整标题文本**（`PluginSDK.InjectText`，点号被 slug 丢掉），
      // 不是裸方法名 —— 否则跳到页面顶部而到不了那一条。
      title.href = pageURL(it.p) + "#" + anchorOf((it.r ? it.r + "." : "") + it.n);
      li.appendChild(title);

      if (it.b) {
        var badge = el("span", "api-badge api-badge-builtin", "仅内置");
        badge.title = "外部（第三方）插件运行时拿不到这个 API";
        li.appendChild(badge);
      }

      li.appendChild(el("code", "api-sig", it.s || ""));

      if (it.d) {
        var d = el("span", "api-desc", it.d);
        li.appendChild(d);
      }
      // 关键词是给检索用的；显示出来能让读者明白“为什么这条被匹配到”。
      if (it.g && it.g.length) {
        li.appendChild(el("span", "api-kw", it.g.slice(0, 6).join(" · ")));
      }
      if (it.f) {
        li.appendChild(el("span", "api-loc", it.f + (it.l ? ":" + it.l : "")));
      }
      list.appendChild(li);
    });
    mount.appendChild(list);
  }

  function pageURL(page) {
    if (!page) return "#";
    // 所有 API 章节都在 /api/ 下（生成物），示例页在 /examples/。
    // 从当前 URL 回推到站点根，保证部署在子路径下也能用。
    var path = window.location.pathname;
    var i = path.indexOf("/api/");
    var root;
    if (i >= 0) {
      root = path.slice(0, i + 1);
    } else {
      var j = path.indexOf("/guide/");
      if (j >= 0) root = path.slice(0, j + 1);
      else if (path.indexOf("/examples/") >= 0) root = path.slice(0, path.indexOf("/examples/") + 1);
      else root = path.slice(0, path.lastIndexOf("/") + 1);
    }
    var dir = page === "examples" ? "examples" : "api";
    return root + dir + "/" + page + "/";
  }

  // anchorOf 复现 MkDocs 的 slug：小写、去掉非 [a-z0-9_-] 的字符（点号被去掉）、
  // 下划线保留、空格转连字符。
  function anchorOf(name) {
    return String(name)
      .toLowerCase()
      .replace(/[^a-z0-9_ -]/g, "")
      .replace(/\s+/g, "-");
  }

  function mount() {
    var box = document.getElementById("api-search");
    if (!box) return;

    var input = el("input", "api-input");
    input.type = "search";
    input.placeholder = "搜索 API：名称、描述、签名…";
    input.setAttribute("autocomplete", "off");
    input.setAttribute("spellcheck", "false");

    var out = el("div", "api-output");
    box.appendChild(input);
    box.appendChild(out);

    load().then(function () {
      render(out, "");
      input.addEventListener("input", function () {
        render(out, input.value);
      });
    });

    // 支持 ?q= 直达（可从别处链接到一次检索）。
    var m = /[?&]q=([^&]+)/.exec(window.location.search);
    if (m) {
      input.value = decodeURIComponent(m[1].replace(/\+/g, " "));
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", mount);
  } else {
    mount();
  }
})();

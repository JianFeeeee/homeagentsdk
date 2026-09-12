package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestPlugin(t *testing.T, h http.HandlerFunc) (*Plugin, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	p := &Plugin{
		name:      "deepsearch",
		searxURL:  srv.URL,
		maxItems:  5,
		language:  "zh-CN",
		fetchMax:  1000,
		userAgent: "test-agent",
		http:      srv.Client(),
	}
	return p, srv
}

// 一份贴近真实 SearXNG 的响应：含重复 URL、缺摘要、多引擎、无响应引擎
const sampleResponse = `{
  "query": "deepin 被开除",
  "results": [
    {"url":"https://www.zhihu.com/question/1?utm_source=x","title":"网传统信内核开发工程师因没穿西服被开除","content":"截止1月9日最新情况…","engines":["duckduckgo","brave"],"score":9.5,"publishedDate":"2026-09-10T00:00:00"},
    {"url":"https://www.zhihu.com/question/1","title":"网传统信内核开发工程师因没穿西服被开除（重复项）","content":"重复条目","engines":["brave"],"score":1.0},
    {"url":"https://www.163.com/dy/article/KIQURODQ.html","title":"离谱！传某信创操作系统大厂因西装开除核心开发者","content":"一位负责Linux内核开发的核心工程师…","engines":["brave","quark"],"score":7.2},
    {"url":"https://bbs.deepin.org.cn/zh","title":"deepin官方论坛","content":"","engines":["duckduckgo"],"score":2.0}
  ],
  "answers": [],
  "suggestions": ["deepin 王勇 离职"],
  "unresponsive_engines": [["baidu","CAPTCHA"],["sogou","unexpected crash"]],
  "timings": {"search": 1.2}
}`

// 1) 检索：去重 + 按分数排序 + 摘要/覆盖度输出
func TestSearchDedupAndFormat(t *testing.T) {
	var gotQuery url.Values
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			gotQuery = r.URL.Query()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(sampleResponse))
			return
		}
		http.NotFound(w, r)
	})
	res, err := p.handleSearch(map[string]interface{}{"query": "deepin 被开除", "count": float64(5)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if gotQuery.Get("format") != "json" {
		t.Errorf("必须要求 json 输出，实际 %q", gotQuery.Get("format"))
	}
	if gotQuery.Get("limit") != "5" {
		t.Errorf("limit 未生效: %q", gotQuery.Get("limit"))
	}
	txt := res.(map[string]interface{})["content"].(string)
	// utm_source 应被规范化掉，重复项只剩一条
	if n := strings.Count(txt, "zhihu.com/question/1"); n != 1 {
		t.Errorf("URL 未正确去重（出现 %d 次）:\n%s", n, txt)
	}
	if !strings.Contains(txt, "网传统信内核开发工程师") {
		t.Errorf("缺少标题: %s", txt)
	}
	if !strings.Contains(txt, "摘要：") {
		t.Errorf("应输出摘要: %s", txt)
	}
	if !strings.Contains(txt, "baidu(CAPTCHA)") {
		t.Errorf("应回报无响应引擎（让模型知道覆盖度）: %s", txt)
	}
	if !strings.Contains(txt, "duckduckgo") || !strings.Contains(txt, "quark") {
		t.Errorf("应回报引擎覆盖: %s", txt)
	}
	// 高分条目应排在前面
	if strings.Index(txt, "统信内核开发工程师") > strings.Index(txt, "离谱！") {
		t.Errorf("未按分数排序:\n%s", txt)
	}
}

// 2) 403（未开 json）必须给出可操作提示，而不是裸错误
func TestSearchForbiddenHint(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Forbidden"))
	})
	_, err := p.handleSearch(map[string]interface{}{"query": "x"})
	if err == nil {
		t.Fatal("应返回错误")
	}
	msg := err.Error()
	if !strings.Contains(msg, "403") || !strings.Contains(msg, "formats") {
		t.Errorf("403 提示应指向 json/limiter 配置，实际: %s", msg)
	}
}

// 3) 空结果：要给出原因与下一步建议
func TestSearchEmptyHint(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"query":"x","results":[],"suggestions":["换个词"],"unresponsive_engines":[["google","CAPTCHA"]]}`))
	})
	res, err := p.handleSearch(map[string]interface{}{"query": "x"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	txt := res.(map[string]interface{})["content"].(string)
	for _, want := range []string{"未返回结果", "google(CAPTCHA)", "换个词", "deepsearch_news"} {
		if !strings.Contains(txt, want) {
			t.Errorf("空结果提示缺少 %q: %s", want, txt)
		}
	}
}

// 4) 新闻：应带 categories=news 与 time_range=week；新闻为空时回退 general
func TestNewsParamsAndFallback(t *testing.T) {
	var calls []url.Values
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Query())
		if r.URL.Query().Get("categories") == "news" {
			_, _ = w.Write([]byte(`{"query":"n","results":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"query":"n","results":[{"url":"https://a.com/1","title":"回退结果","content":"内容","engines":["brave"],"score":1}]}`))
	})
	res, err := p.handleNews(map[string]interface{}{"query": "某事"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("新闻为空时应回退 general，实际调用 %d 次", len(calls))
	}
	if calls[0].Get("categories") != "news" || calls[0].Get("time_range") != "week" {
		t.Errorf("首次应为 news + week，实际 categories=%q time_range=%q", calls[0].Get("categories"), calls[0].Get("time_range"))
	}
	if tmp := res.(map[string]interface{})["content"].(string); !strings.Contains(tmp, "回退结果") {
		t.Errorf("回退结果未被采用: %s", tmp)
	}
}

// 5) 正文抽取：去脚本/样式/导航，保留 article
func TestFetchExtractsArticle(t *testing.T) {
	page := `<!doctype html><html><head><title>测试标题 - 站点</title>
	<style>.x{color:red}</style><script>var secret="SHOULD_NOT_APPEAR";</script></head>
	<body><nav>导航链接</nav><article>
	<p>第一段正文，包含关键事实。</p><p>第二段正文。</p>
	</article><footer>页脚</footer></body></html>`
	var srvURL string
	p, srv := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(page))
	})
	srvURL = srv.URL
	// 注意：不要用 example.com 之类真实域名——本机 DNS/proxy 会把它们转走，测试会飘
	res, err := p.handleFetch(map[string]interface{}{"url": srvURL + "/a"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	txt := res.(map[string]interface{})["content"].(string)
	if !strings.Contains(txt, "第一段正文") {
		t.Errorf("正文丢失: %s", txt)
	}
	if strings.Contains(txt, "SHOULD_NOT_APPEAR") {
		t.Errorf("脚本内容不应出现: %s", txt)
	}
	if strings.Contains(txt, "导航链接") || strings.Contains(txt, "页脚") {
		t.Errorf("导航/页脚应被剥离: %s", txt)
	}
	if !strings.Contains(txt, "测试标题") {
		t.Errorf("标题应被提取: %s", txt)
	}
}

// 6) 深检索：候选 + 正文证据；单篇失败不应导致整体失败
func TestDeepSearch(t *testing.T) {
	var srvURL string // 处理函数先于 server 存在，故用闭包变量回填
	p, srv := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			_, _ = w.Write([]byte(`{"query":"d","results":[
			  {"url":"` + srvURL + `/ok1","title":"好文一","content":"摘要一","engines":["brave"],"score":3},
			  {"url":"` + srvURL + `/bad","title":"打不开的","content":"摘要二","engines":["brave"],"score":2},
			  {"url":"` + srvURL + `/ok2","title":"好文二","content":"摘要三","engines":["brave"],"score":1}]}`))
		case "/ok1", "/ok2":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body><article><p>正文内容 " + r.URL.Path + "</p></article></body></html>"))
		case "/bad":
			w.WriteHeader(http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	})
	srvURL = srv.URL
	res, err := p.handleDeep(map[string]interface{}{"query": "d", "top_k": float64(3), "max_chars": float64(500)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	txt := res.(map[string]interface{})["content"].(string)
	for _, want := range []string{"候选清单", "正文证据", "正文内容 /ok1", "正文内容 /ok2", "抓取失败"} {
		if !strings.Contains(txt, want) {
			t.Errorf("深检索输出缺少 %q:\n%s", want, txt)
		}
	}
}

// 7) 自检：健康检查 + 探测检索 + 引擎覆盖统计
func TestStatusReportsEngines(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("OK"))
			return
		}
		_, _ = w.Write([]byte(sampleResponse))
	})
	res, err := p.handleStatus(map[string]interface{}{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	m := res.(map[string]interface{})
	if m["healthz"] != 200 {
		t.Errorf("healthz 应为 200，实际 %v", m["healthz"])
	}
	if m["search_ok"] != true {
		t.Errorf("search_ok 应为 true：%v", m["search_ok"])
	}
	engs, ok := m["engines_returning_results"].(map[string]int)
	if !ok || engs["brave"] == 0 || engs["quark"] == 0 {
		t.Errorf("引擎统计不正确: %#v", m["engines_returning_results"])
	}
}

// 8) 摘要压成一行并按字符截断（避免巨长摘要吃掉上下文）
func TestOneLineTruncate(t *testing.T) {
	got := oneLine("第一行\n第二行\t第三行", 5)
	if strings.Contains(got, "\n") {
		t.Errorf("应为单行: %q", got)
	}
	if r := []rune(got); len(r) != 6 { // 5 字符 + 省略号
		t.Errorf("截断长度不符: %q (%d runes)", got, len(r))
	}
}

// 9) 正文抽取长度上限生效
func TestHtmlToTextTruncation(t *testing.T) {
	long := strings.Repeat("字", 5000)
	_, text := htmlToText("<html><body><article><p>"+long+"</p></article></body></html>", 100)
	if !strings.Contains(text, "已截断") {
		t.Errorf("超长正文应被截断: %d", len([]rune(text)))
	}
}

// 10) 非 http(s) 协议应被拒绝
func TestFetchRejectsBadScheme(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {})
	if _, err := p.handleFetch(map[string]interface{}{"url": "file:///etc/passwd"}); err == nil {
		t.Fatal("file:// 应被拒绝")
	}
	if _, err := p.handleFetch(map[string]interface{}{"url": "javascript:alert(1)"}); err == nil {
		t.Fatal("javascript: 应被拒绝")
	}
}

// 11) raw 模式返回结构化 JSON（排查用）
func TestSearchRawMode(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleResponse))
	})
	res, err := p.handleSearch(map[string]interface{}{"query": "q", "raw": true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	m, ok := res.(*searxResponse)
	if !ok {
		t.Fatalf("raw 应返回结构化响应，实际 %T", res)
	}
	if len(m.Results) != 4 {
		t.Errorf("结果数应为 4（raw 不去重），实际 %d", len(m.Results))
	}
	if _, err := json.Marshal(m); err != nil {
		t.Errorf("结构化结果应可序列化: %v", err)
	}
}

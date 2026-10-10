# browser · 统一浏览器

一个插件覆盖三种"访问网页"的能力，从最轻到最重。**按需选层**是这个插件的重点 ——
绝大多数抓取用 HTTP 就够，不该为了一句话启动 Chromium。

## 三种能力层

| 层 | 工具 | 何时用 |
|---|---|---|
| **搜索** | `browser_search` | 要的是"找到哪些页面"，不是页面本身 |
| **quick（纯 HTTP）** | `browser_fetch`（`mode=quick`） | 静态页、API、能直接拿到 HTML |
| **normal（无头渲染）** | `browser_render` / `browser_fetch`（`mode=render`） | JS 渲染的页面，HTTP 拿不到内容 |
| **interactive（CDP）** | `browser_start` + `navigate`/`click`/`type`/`scroll`/`html`/`screenshot` | 需要交互：登录、点按、翻页 |

`browser_fetch` 的 `mode`：

- `auto`（默认）：先试 HTTP，**遇 403/429 才降级**用 Chromium 渲染
- `render`：强制 Chromium
- `quick`：纯 HTTP，不降级

## 工具

| 工具 | 说明 |
|---|---|
| `browser_search` | 网页搜索 |
| `browser_fetch` | 抓取 URL 内容，三种 mode 见上 |
| `browser_render` | 无头 Chromium 渲染并提取文本（normal） |
| `browser_start` | 启动交互式浏览器会话（CDP） |
| `browser_navigate` | 导航到指定 URL |
| `browser_click` | 点击元素 |
| `browser_type` | 输入文本 |
| `browser_scroll` | 滚动页面 |
| `browser_html` | 取当前页 HTML |
| `browser_screenshot` | 截图，**保存为 PNG 并返回路径**（拿 path 调 `describe_image` / `ocr_image`） |
| `browser_install` | 安装 systemd 托管的共享浏览器后端 |
| `browser_close` | 关闭会话 |

## 共享浏览器后端

`browser_install` 安装 `homeagent-browser.service`（systemd 托管）。
装上之后**所有 agent 共享同一个 Chromium 实例与登录态**，各自占独立标签页互不干扰
（同 source 复用自己的标签页）。

前提：本机已有 chromium 二进制，没有会提示先装（`apt install chromium` 或等价）。

## 实现要点

- **搜索用 `cn.bing.com` 而不是 `www.bing.com`**：后者对程序化请求常回 302（同意/重定向页），
  根本拿不到结果块。
- **标题取 `<h2>` 里的 `<a>`**：直接抓结果块里第一个 `<a>` 会拿到来源行而非标题。
- **摘要认 `b_lineclamp`**：旧版 Bing 用 `b_caption`，新版已迁走，两套都匹配。
- **有 SSRF 防护**：见源码 `SSRF` 段，抓取前校验目标地址，避免被诱导访问内网。

## 测试

```bash
go test -count=1 ./...
```

`testdata/bing_cn.html` 是搜索解析的固定样本，用它做离线断言，避免测试依赖真实网络。

## 构建

```bash
hmapdev build
```

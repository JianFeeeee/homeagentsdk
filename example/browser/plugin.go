package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	sdk "gitcode.com/JianFeeeee/homeagent-sdk/sdk"

	"github.com/chromedp/chromedp"
)

type Plugin struct {
	name    string
	sdk     *sdk.PluginSDK
	mu      sync.RWMutex
	timeout int
	proxy   string
	client  *http.Client

	sessions map[string]*BrowserSession
	nextID   int
	wg       sync.WaitGroup
	stopCh   chan struct{}
}

type BrowserSession struct {
	id        string
	allocCtx  context.Context
	cancel    context.CancelFunc
	ctx       context.Context
	createdAt time.Time
	timeout   time.Duration
	closed    bool
	mu        sync.Mutex
	currentURL string
}

func NewPlugin(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{name: name, stopCh: make(chan struct{}), sessions: make(map[string]*BrowserSession)}, nil
}

func (p *Plugin) Name() string { return p.name }

func readCfg[T string | int64 | float64](s sdk.SettingsAPI, key string, fallback T) T {
	v, err := s.Get(key)
	if err == nil && v != nil {
		if s, ok := v.(string); ok && s != "" {
			switch any(fallback).(type) {
			case string:
				return any(s).(T)
			case int64:
				if n, err := strconv.ParseInt(s, 10, 64); err == nil {
					return any(n).(T)
				}
			case float64:
				if n, err := strconv.ParseFloat(s, 64); err == nil {
					return any(n).(T)
				}
			}
		}
	}
	v2, err2 := s.GetCore("plugin." + "browser" + "." + key)
	if err2 == nil && v2 != nil {
		if s, ok := v2.(string); ok && s != "" {
			switch any(fallback).(type) {
			case string:
				return any(s).(T)
			case int64:
				if n, err := strconv.ParseInt(s, 10, 64); err == nil {
					return any(n).(T)
				}
			case float64:
				if n, err := strconv.ParseFloat(s, 64); err == nil {
					return any(n).(T)
				}
			}
		}
	}
	return fallback
}

func readArg[T string | int64 | float64](args map[string]interface{}, key string, fallback T) T {
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	switch any(fallback).(type) {
	case string:
		if s, ok := v.(string); ok {
			return any(s).(T)
		}
	case int64:
		switch val := v.(type) {
		case float64:
			return any(int64(val)).(T)
		case string:
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				return any(n).(T)
			}
		}
	case float64:
		switch val := v.(type) {
		case float64:
			return any(val).(T)
		case string:
			if n, err := strconv.ParseFloat(val, 64); err == nil {
				return any(n).(T)
			}
		}
	}
	return fallback
}

func errResult(msg string) map[string]interface{} {
	return map[string]interface{}{"isError": true, "content": msg}
}

func newHTTPClient(timeout int, proxyURL string) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   time.Duration(timeout) * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   time.Duration(timeout) * time.Second,
		ResponseHeaderTimeout: time.Duration(timeout) * time.Second,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Timeout:   time.Duration(timeout) * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

}

// ── Start / Stop ──────────────────────────────────────────

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	p.sdk = s
	s.SetAutoRestart(true)

	s.Settings().RegisterDef(sdk.ConfigDef{
		Key: "timeout", Default: "30", Type: "int",
		DisplayName: "HTTP 超时(秒)", Description: "HTTP 请求超时时间",
		Category: "browser",
	})
	s.Settings().RegisterDef(sdk.ConfigDef{
		Key: "proxy", Default: "", Type: "string",
		DisplayName: "HTTP 代理", Description: "HTTP 代理地址，如 http://proxy:port",
		Category: "browser",
	})

	t := readCfg(s.Settings(), "timeout", float64(30))
	p.timeout = int(t)
	if p.timeout < 5 {
		p.timeout = 5
	}
	if p.timeout > 120 {
		p.timeout = 120
	}
	p.proxy = readCfg(s.Settings(), "proxy", "")
	p.client = newHTTPClient(p.timeout, p.proxy)

	tp := p.name + "_"

	s.RegisterTool(tp+"search", sdk.ToolDef{
		Name:        tp + "search",
		Description: "使用 Bing 搜索网页。返回标题、URL 和摘要。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "搜索关键词"},
				"count": map[string]interface{}{"type": "integer", "description": "结果数量(1-20，默认5)"},
			},
			"required": []string{"query"},
		},
	}, p.handleSearch)

	s.RegisterTool(tp+"fetch", sdk.ToolDef{
		Name:        tp + "fetch",
		Description: "抓取 URL 内容。mode=auto 时遇 403/429 自动降级用无头 Chromium 渲染；mode=render 强制用 Chromium；mode=quick 纯 HTTP 不降级。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":       map[string]interface{}{"type": "string", "description": "HTTP/HTTPS URL"},
				"max_chars": map[string]interface{}{"type": "integer", "description": "最大返回字符数(默认20000)"},
				"mode":      map[string]interface{}{"type": "string", "description": "auto(默认)/render(强制Chromium)/quick(纯HTTP)"},
			},
			"required": []string{"url"},
		},
	}, p.handleFetch)

	s.RegisterTool(tp+"render", sdk.ToolDef{
		Name:        tp + "render",
		Description: "无头 Chromium 渲染网页并提取文本(normal 模式)。支持 JS 渲染的页面。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":  map[string]interface{}{"type": "string", "description": "URL"},
				"wait": map[string]interface{}{"type": "integer", "description": "等待 JS 渲染的秒数(默认0)"},
			},
			"required": []string{"url"},
		},
	}, p.handleRender)

	s.RegisterTool(tp+"start", sdk.ToolDef{
		Name:        tp + "start",
		Description: "启动交互式浏览器会话(interactive 模式)。通过 CDP 连接 Chromium，支持导航、截图、点击、输入等操作。返回会话 ID。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":     map[string]interface{}{"type": "string", "description": "初始导航 URL（可选）"},
				"timeout": map[string]interface{}{"type": "string", "description": "会话超时(如 5m, 10m，默认 10m)"},
			},
		},
	}, p.handleBrowserStart)

	s.RegisterTool(tp+"navigate", sdk.ToolDef{
		Name:        tp + "navigate",
		Description: "在交互式浏览器中导航到指定 URL。自动等待页面 body 加载完成。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":   map[string]interface{}{"type": "string", "description": "浏览器会话 ID"},
				"url":  map[string]interface{}{"type": "string", "description": "目标 URL"},
				"wait": map[string]interface{}{"type": "integer", "description": "页面加载后额外等待秒数(默认2，反爬页面建议5)"},
			},
			"required": []string{"id", "url"},
		},
	}, p.handleNavigate)

	s.RegisterTool(tp+"screenshot", sdk.ToolDef{
		Name:        tp + "screenshot",
		Description: "对交互式浏览器当前页面截图。返回 base64 编码的 PNG 图片。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":     map[string]interface{}{"type": "string", "description": "浏览器会话 ID"},
				"full":   map[string]interface{}{"type": "boolean", "description": "是否全页截图(默认 false，仅视口)"},
				"format": map[string]interface{}{"type": "string", "description": "图片格式: png 或 jpeg(默认 png)"},
			},
			"required": []string{"id"},
		},
	}, p.handleScreenshot)

	s.RegisterTool(tp+"html", sdk.ToolDef{
		Name:        tp + "html",
		Description: "获取交互式浏览器当前页面 JS 渲染后的完整 HTML。用于模型分析页面结构、定位元素。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":        map[string]interface{}{"type": "string", "description": "浏览器会话 ID"},
				"max_chars": map[string]interface{}{"type": "integer", "description": "最大返回字符数(默认50000)"},
			},
			"required": []string{"id"},
		},
	}, p.handleHTML)

	s.RegisterTool(tp+"click", sdk.ToolDef{
		Name:        tp + "click",
		Description: "在交互式浏览器中点击元素。自动等待元素可见后再点击。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":       map[string]interface{}{"type": "string", "description": "浏览器会话 ID"},
				"selector": map[string]interface{}{"type": "string", "description": "CSS 选择器"},
				"wait":     map[string]interface{}{"type": "integer", "description": "等待元素出现的超时毫秒数(默认3000)"},
			},
			"required": []string{"id", "selector"},
		},
	}, p.handleClick)

	s.RegisterTool(tp+"type", sdk.ToolDef{
		Name:        tp + "type",
		Description: "在交互式浏览器中向输入框输入文字。自动等待元素可见、聚焦后清空再输入。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":       map[string]interface{}{"type": "string", "description": "浏览器会话 ID"},
				"selector": map[string]interface{}{"type": "string", "description": "CSS 选择器"},
				"text":     map[string]interface{}{"type": "string", "description": "要输入的文字"},
				"submit":   map[string]interface{}{"type": "boolean", "description": "输入后按回车(默认 false)"},
				"wait":     map[string]interface{}{"type": "integer", "description": "等待元素出现的超时毫秒数(默认3000)"},
			},
			"required": []string{"id", "selector", "text"},
		},
	}, p.handleType)

	s.RegisterTool(tp+"scroll", sdk.ToolDef{
		Name:        tp + "scroll",
		Description: "在交互式浏览器中滚动页面。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id":     map[string]interface{}{"type": "string", "description": "浏览器会话 ID"},
				"dir":    map[string]interface{}{"type": "string", "description": "方向: up, down, left, right(默认 down)"},
				"amount": map[string]interface{}{"type": "integer", "description": "滚动像素数(默认 500)"},
			},
			"required": []string{"id"},
		},
	}, p.handleScroll)

	s.RegisterTool(tp+"close", sdk.ToolDef{
		Name:        tp + "close",
		Description: "关闭交互式浏览器会话，释放资源。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{"type": "string", "description": "浏览器会话 ID"},
			},
			"required": []string{"id"},
		},
	}, p.handleBrowserClose)

	p.wg.Add(1)
	go p.cleanupLoop()

	log.Printf("[%s] started, timeout=%ds proxy=%q", p.name, p.timeout, p.proxy)
	return nil
}

func (p *Plugin) Stop() error {
	close(p.stopCh)
	p.wg.Wait()
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
	p.mu.Lock()
	for _, s := range p.sessions {
		s.Close()
	}
	p.sessions = nil
	p.mu.Unlock()
	log.Printf("[%s] stopped", p.name)
	return nil
}

// ── SSRF ──────────────────────────────────────────────────

var privateCIDRs []*net.IPNet

func init() {
	for _, c := range []string{
		"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12",
		"192.168.0.0/16", "100.64.0.0/10", "169.254.0.0/16",
		"::1/128", "fc00::/7", "fe80::/10",
	} {
		_, n, _ := net.ParseCIDR(c)
		if n != nil {
			privateCIDRs = append(privateCIDRs, n)
		}
	}
}

func isPrivateIP(ip net.IP) bool {
	for _, n := range privateCIDRs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (p *Plugin) ssrfCheck(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https allowed, got: %s", u.Scheme)
	}
	ips, err := net.LookupHost(u.Hostname())
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %w", err)
	}
	for _, ip := range ips {
		if parsed := net.ParseIP(ip); parsed != nil && isPrivateIP(parsed) {
			return fmt.Errorf("blocked request to private IP: %s (%s)", u.Hostname(), ip)
		}
	}
	return nil
}

// ── Bing Search ───────────────────────────────────────────

type searchResult struct {
	Title, URL, Snippet string
}

func (p *Plugin) bingSearch(query string, count int) ([]searchResult, error) {
	u := fmt.Sprintf("https://www.bing.com/search?q=%s&count=%d", url.QueryEscape(query), count)
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return parseBingResults(string(body), count), nil
}

func parseBingResults(html string, count int) []searchResult {
	var results []searchResult
	re := regexp.MustCompile(`<li class="b_algo"(?s)(.*?)</li>`)
	matches := re.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		if len(results) >= count {
			break
		}
		block := m[1]
		var r searchResult
		hrefRe := regexp.MustCompile(`<a[^>]+href="([^"]+)"[^>]*>`)
		if hm := hrefRe.FindStringSubmatch(block); len(hm) > 1 {
			r.URL = hm[1]
		}
		titleRe := regexp.MustCompile(`<a[^>]+href="[^"]+"[^>]*>(.*?)</a>`)
		if tm := titleRe.FindStringSubmatch(block); len(tm) > 1 {
			r.Title = stripTags(tm[1])
		}
		snipRe := regexp.MustCompile(`<div class="b_caption">.*?<p>(.*?)</p>`)
		if sm := snipRe.FindStringSubmatch(block); len(sm) > 1 {
			r.Snippet = stripTags(sm[1])
		}
		if r.URL != "" && r.Title != "" {
			results = append(results, r)
		}
	}
	return results
}

func (p *Plugin) handleSearch(args map[string]interface{}) (interface{}, error) {
	query := readArg(args, "query", "")
	if query == "" {
		return errResult("query is required"), nil
	}
	count := 5
	if v, ok := args["count"].(float64); ok && v > 0 {
		count = int(v)
	}
	if count < 1 {
		count = 1
	}
	if count > 20 {
		count = 20
	}
	results, err := p.bingSearch(query, count)
	if err != nil {
		return errResult("search failed: " + err.Error()), nil
	}
	if len(results) == 0 {
		return map[string]interface{}{"content": "No results found."}, nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for %q:\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}
	return map[string]interface{}{"content": strings.TrimSpace(sb.String())}, nil
}

// ── Web Fetch (quick) ──────────────────────────────────────

func htmlToText(html string) string {
	for _, tag := range []string{"<script", "<style"} {
		closing := "</" + tag[1:] + ">"
		for {
			start := strings.Index(strings.ToLower(html), tag)
			if start < 0 {
				break
			}
			end := strings.Index(html[start:], closing)
			if end < 0 {
				break
			}
			html = html[:start] + html[start+end+len(closing):]
		}
	}
	for _, tag := range []string{"</p>", "</div>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>", "</li>", "</tr>", "</blockquote>", "<br", "</pre>"} {
		html = strings.ReplaceAll(html, tag, "\n")
	}
	html = stripTags(html)
	for _, pair := range [][2]string{
		{"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"},
		{"&quot;", "\""}, {"&#39;", "'"}, {"&nbsp;", " "},
	} {
		html = strings.ReplaceAll(html, pair[0], pair[1])
	}
	lines := strings.Split(html, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		in := []rune(line)
		var out []rune
		space := false
		for _, r := range in {
			if unicode.IsSpace(r) {
				if !space {
					out = append(out, ' ')
					space = true
				}
			} else {
				out = append(out, r)
				space = false
			}
		}
		cleaned = append(cleaned, string(out))
	}
	return strings.Join(cleaned, "\n")
}

func stripTags(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func (p *Plugin) handleFetch(args map[string]interface{}) (interface{}, error) {
	rawURL := readArg(args, "url", "")
	if rawURL == "" {
		return errResult("url is required"), nil
	}
	maxChars := 20000
	if v, ok := args["max_chars"].(float64); ok && v > 0 {
		maxChars = int(v)
	}
	if maxChars > 500000 {
		maxChars = 500000
	}
	if err := p.ssrfCheck(rawURL); err != nil {
		return errResult(err.Error()), nil
	}

	mode := readArg(args, "mode", "auto")
	if mode == "render" {
		return p.fetchWithChromium(rawURL, maxChars)
	}

	req, _ := http.NewRequest("GET", rawURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	resp, err := p.client.Do(req)
	if err != nil {
		return errResult("fetch failed: " + err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		resp.Body.Close()
		if (resp.StatusCode == 403 || resp.StatusCode == 429) && mode == "auto" {
			return p.fetchWithChromium(rawURL, maxChars)
		}
		return errResult(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)), nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxChars)+50000))
	rawText := string(body)
	ct := resp.Header.Get("Content-Type")
	var extracted string
	if strings.Contains(ct, "text/html") {
		extracted = htmlToText(rawText)
	} else if strings.Contains(ct, "application/json") {
		var v interface{}
		if json.Unmarshal(body, &v) == nil {
			if pretty, err := json.MarshalIndent(v, "", "  "); err == nil {
				extracted = string(pretty)
			}
		}
		if extracted == "" {
			extracted = rawText
		}
	} else {
		extracted = rawText
	}
	extracted = strings.TrimSpace(extracted)
	if len(extracted) > maxChars {
		extracted = extracted[:maxChars] + "\n\n[Content truncated]"
	}
	if extracted == "" {
		extracted = "(empty content)"
	}
	return map[string]interface{}{
		"content": extracted,
		"details": map[string]interface{}{"url": rawURL, "status": resp.StatusCode, "content_type": ct},
	}, nil
}

// ── Chromium Render (normal) ──────────────────────────────

func (p *Plugin) fetchWithChromium(rawURL string, maxChars int) (interface{}, error) {
	chromiumPath := "/usr/local/bin/chromium"
	if _, err := os.Stat(chromiumPath); err != nil {
		return errResult("HTTP 403 且 Chromium 不可用，无法降级渲染"), nil
	}
	var out bytes.Buffer
	cmd := exec.Command(chromiumPath, "--headless", "--disable-gpu", "--no-sandbox", "--dump-dom", rawURL)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return errResult("Chromium 渲染失败: " + err.Error()), nil
	}
	text := htmlToText(out.String())
	if len(text) > maxChars {
		text = text[:maxChars] + "\n\n[Content truncated]"
	}
	return map[string]interface{}{
		"content": text,
		"details": map[string]interface{}{"url": rawURL, "mode": "render_fallback", "content_type": "text/html"},
	}, nil
}

func (p *Plugin) handleRender(args map[string]interface{}) (interface{}, error) {
	rawURL := readArg(args, "url", "")
	if rawURL == "" {
		return errResult("url is required"), nil
	}
	waitSec := int64(readArg(args, "wait", float64(0)))
	if waitSec > 0 {
		time.Sleep(time.Duration(waitSec) * time.Second)
	}
	var html string
	chromiumPath := "/usr/local/bin/chromium"
	if _, err := os.Stat(chromiumPath); err == nil {
		var out bytes.Buffer
		cmd := exec.Command(chromiumPath, "--headless", "--disable-gpu", "--no-sandbox", "--dump-dom", rawURL)
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return errResult("chromium: " + err.Error()), nil
		}
		html = out.String()
	} else {
		resp, err := http.Get(rawURL)
		if err != nil {
			return errResult("http get: " + err.Error()), nil
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		html = string(body)
	}
	title := ""
	if m := regexp.MustCompile(`<title>([^<]+)</title>`).FindStringSubmatch(html); len(m) > 1 {
		title = m[1]
	}
	text := htmlToText(html)
	origLen := len(text)
	truncated := origLen > 5000
	if truncated {
		text = text[:5000]
	}
	result := ""
	if title != "" {
		result = fmt.Sprintf("标题: %s\nURL: %s\n\n", title, rawURL)
	}
	result += text
	if truncated {
		result += fmt.Sprintf("\n\n...(仅显示前 5000 字符，共 %d 字符)", origLen)
	}
	return map[string]interface{}{"content": result, "title": title}, nil
}

// ── Interactive Browser Session (CDP) ─────────────────────

func (p *Plugin) handleBrowserStart(args map[string]interface{}) (interface{}, error) {
	timeoutStr := readArg(args, "timeout", "10m")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 10 * time.Minute
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(1280, 800),
	)
	if p.proxy != "" {
		opts = append(opts, chromedp.Flag("proxy-server", p.proxy))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, _ := chromedp.NewContext(allocCtx)

	// 立即分配浏览器和 Target，确保后续 Run 的 timeout context 不会杀死浏览器进程
	// chromedp 官方警告：首调用带 timeout 的 Run 会杀死整个浏览器
	if err := chromedp.Run(ctx); err != nil {
		cancel()
		return errResult("browser init failed: " + err.Error()), nil
	}

	session := &BrowserSession{
		allocCtx:  allocCtx,
		cancel:    cancel,
		ctx:       ctx,
		createdAt: time.Now(),
		timeout:   timeout,
	}

	p.mu.Lock()
	p.nextID++
	id := fmt.Sprintf("browser_%d", p.nextID)
	session.id = id
	p.sessions[id] = session
	p.mu.Unlock()

	initURL := readArg(args, "url", "")
	if initURL != "" {
		if err := chromedp.Run(ctx,
			chromedp.Navigate(initURL),
			chromedp.WaitReady("body"),
		); err != nil {
			session.Close()
			p.mu.Lock()
			delete(p.sessions, id)
			p.mu.Unlock()
			return errResult("navigate failed: " + err.Error()), nil
		}
		session.currentURL = initURL
		p.sdk.InjectText(p.name, p.name, fmt.Sprintf("[浏览器 %s 已打开 %s]", id, initURL))
	}

	log.Printf("[%s] created browser session %s: url=%s timeout=%v", p.name, id, initURL, timeout)
	return map[string]interface{}{
		"id":      id,
		"status":  "created",
		"url":     initURL,
		"timeout": timeout.String(),
	}, nil
}

func (p *Plugin) getSession(id string) (*BrowserSession, error) {
	p.mu.Lock()
	s, ok := p.sessions[id]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("浏览器会话 %s 不存在或已关闭", id)
	}
	return s, nil
}

func (p *Plugin) handleNavigate(args map[string]interface{}) (interface{}, error) {
	id := readArg(args, "id", "")
	rawURL := readArg(args, "url", "")
	if id == "" || rawURL == "" {
		return errResult("id 和 url 不能为空"), nil
	}
	s, err := p.getSession(id)
	if err != nil {
		return errResult(err.Error()), nil
	}
	waitSec := int64(readArg(args, "wait", float64(2)))
	if err := chromedp.Run(s.ctx,
		chromedp.Navigate(rawURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(time.Duration(waitSec)*time.Second),
	); err != nil {
		return errResult("navigate failed: " + err.Error()), nil
	}
	s.currentURL = rawURL
	p.sdk.InjectText(p.name, p.name, fmt.Sprintf("[浏览器 %s 已导航到 %s]", id, rawURL))
	return map[string]interface{}{"status": "ok", "url": rawURL}, nil
}

func (p *Plugin) handleScreenshot(args map[string]interface{}) (interface{}, error) {
	id := readArg(args, "id", "")
	if id == "" {
		return errResult("id is required"), nil
	}
	s, sessErr := p.getSession(id)
	if sessErr != nil {
		return errResult(sessErr.Error()), nil
	}
	full := false
	if v, ok := args["full"].(bool); ok {
		full = v
	}
	format := readArg(args, "format", "png")
	var buf []byte
	var err error
	if full {
		err = chromedp.Run(s.ctx, chromedp.FullScreenshot(&buf, 90))
	} else {
		err = chromedp.Run(s.ctx, chromedp.Screenshot("body", &buf))
	}
	if err != nil {
		return errResult("screenshot failed: " + err.Error()), nil
	}
	b64 := base64.StdEncoding.EncodeToString(buf)
	return map[string]interface{}{
		"status":    "ok",
		"format":    format,
		"size":      len(buf),
		"base64":    b64,
		"data_uri":  fmt.Sprintf("data:image/%s;base64,%s", format, b64),
	}, nil
}

func (p *Plugin) handleHTML(args map[string]interface{}) (interface{}, error) {
	id := readArg(args, "id", "")
	if id == "" {
		return errResult("id is required"), nil
	}
	s, err := p.getSession(id)
	if err != nil {
		return errResult(err.Error()), nil
	}
	maxChars := 50000
	if v, ok := args["max_chars"].(float64); ok && v > 0 {
		maxChars = int(v)
	}
	var html string
	if err := chromedp.Run(s.ctx, chromedp.OuterHTML("html", &html)); err != nil {
		return errResult("get html failed: " + err.Error()), nil
	}
	var title, currentURL string
	chromedp.Run(s.ctx,
		chromedp.Title(&title),
		chromedp.Location(&currentURL),
	)
	truncated := len(html) > maxChars
	if truncated {
		html = html[:maxChars] + "\n\n[HTML truncated]"
	}
	return map[string]interface{}{
		"status":  "ok",
		"title":   title,
		"url":     currentURL,
		"html":    html,
		"length":  len(html),
	}, nil
}

func (p *Plugin) handleClick(args map[string]interface{}) (interface{}, error) {
	id := readArg(args, "id", "")
	selector := readArg(args, "selector", "")
	if id == "" || selector == "" {
		return errResult("id 和 selector 不能为空"), nil
	}
	s, err := p.getSession(id)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if err := chromedp.Run(s.ctx,
		chromedp.WaitVisible(selector),
		chromedp.Click(selector),
	); err != nil {
		return errResult("click failed (element may not exist or page blocking): " + err.Error()), nil
	}
	return map[string]interface{}{"status": "ok", "selector": selector}, nil
}

func (p *Plugin) handleType(args map[string]interface{}) (interface{}, error) {
	id := readArg(args, "id", "")
	selector := readArg(args, "selector", "")
	text := readArg(args, "text", "")
	if id == "" || selector == "" || text == "" {
		return errResult("id, selector, text 不能为空"), nil
	}
	s, err := p.getSession(id)
	if err != nil {
		return errResult(err.Error()), nil
	}
	actions := []chromedp.Action{
		chromedp.WaitVisible(selector),
		chromedp.Click(selector, chromedp.NodeVisible),
		chromedp.Clear(selector),
		chromedp.SendKeys(selector, text),
	}
	submit := false
	if v, ok := args["submit"].(bool); ok {
		submit = v
	}
	if submit {
		actions = append(actions, chromedp.SendKeys(selector, "\r"))
	}
	if err := chromedp.Run(s.ctx, actions...); err != nil {
		return errResult("type failed (element may not exist or page blocking): " + err.Error()), nil
	}
	return map[string]interface{}{"status": "ok", "selector": selector}, nil
}

func (p *Plugin) handleScroll(args map[string]interface{}) (interface{}, error) {
	id := readArg(args, "id", "")
	if id == "" {
		return errResult("id is required"), nil
	}
	s, err := p.getSession(id)
	if err != nil {
		return errResult(err.Error()), nil
	}
	dir := readArg(args, "dir", "down")
	amount := readArg(args, "amount", float64(500))
	var scrollJS string
	switch dir {
	case "up":
		scrollJS = fmt.Sprintf("window.scrollBy(0, -%d)", int(amount))
	case "down":
		scrollJS = fmt.Sprintf("window.scrollBy(0, %d)", int(amount))
	case "left":
		scrollJS = fmt.Sprintf("window.scrollBy(-%d, 0)", int(amount))
	case "right":
		scrollJS = fmt.Sprintf("window.scrollBy(%d, 0)", int(amount))
	default:
		return errResult("dir 必须是 up/down/left/right"), nil
	}
	if err := chromedp.Run(s.ctx, chromedp.Evaluate(scrollJS, nil)); err != nil {
		return errResult("scroll failed: " + err.Error()), nil
	}
	return map[string]interface{}{"status": "ok", "dir": dir, "amount": amount}, nil
}

func (p *Plugin) handleBrowserClose(args map[string]interface{}) (interface{}, error) {
	id := readArg(args, "id", "")
	if id == "" {
		return errResult("id is required"), nil
	}
	p.mu.Lock()
	s, ok := p.sessions[id]
	if ok {
		delete(p.sessions, id)
	}
	p.mu.Unlock()
	if !ok {
		return errResult(fmt.Sprintf("浏览器会话 %s 不存在或已关闭", id)), nil
	}
	s.Close()
	log.Printf("[%s] closed browser session %s", p.name, id)
	return map[string]interface{}{"status": "closed", "id": id}, nil
}

func (s *BrowserSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.cancel()
}

func (p *Plugin) cleanupLoop() {
	defer p.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.mu.Lock()
			for id, s := range p.sessions {
				if time.Since(s.createdAt) >= s.timeout {
					log.Printf("[%s] cleanup: browser session %s expired", p.name, id)
					delete(p.sessions, id)
					go s.Close()
					p.sdk.InjectText(p.name, p.name, fmt.Sprintf("[浏览器会话 %s 已超时关闭]", id))
				}
			}
			p.mu.Unlock()
		}
	}
}

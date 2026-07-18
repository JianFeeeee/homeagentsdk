package main

import (
	"bytes"
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
	"strings"
	"sync"
	"time"
	"unicode"

	"gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

type Plugin struct {
	name       string
	sdk        *sdk.PluginSDK
	mu         sync.RWMutex
	timeout    int
	proxy      string
	client     *http.Client
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
		u, err := url.Parse(proxyURL)
		if err == nil {
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

func (p *Plugin) Name() string { return p.name }

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	s.SetAutoRestart(true)
	p.sdk = s

	s.Settings().RegisterDef(sdk.ConfigDef{
		Key: "plugin.browser.timeout", Default: "30", Type: "int",
		DisplayName: "HTTP 超时(秒)", Description: "HTTP 请求超时时间",
		Category: "browser",
	})
	s.Settings().RegisterDef(sdk.ConfigDef{
		Key: "plugin.browser.proxy", Default: "", Type: "string",
		DisplayName: "HTTP 代理", Description: "HTTP 代理地址，如 http://proxy:port。为空则不使用代理",
		Category: "browser",
	})
	t := getSetting[float64](s.Settings(), "timeout", 30)
	p.timeout = int(t)
	if p.timeout < 5 { p.timeout = 5 }
	if p.timeout > 120 { p.timeout = 120 }

	p.proxy = getSetting[string](s.Settings(), "proxy", "")
	p.client = newHTTPClient(p.timeout, p.proxy)

	tp := p.name + "_"

	s.RegisterTool(tp+"search", sdk.ToolDef{
		Name: tp + "search",
		Description: "Search the web for current information using DuckDuckGo. Returns formatted results with titles, URLs, and snippets.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{"type": "string", "description": "Search query"},
				"count": map[string]interface{}{"type": "integer", "description": "Number of results (1-20, default 5)"},
			},
			"required": []string{"query"},
		},
	}, p.handleSearch)

	s.RegisterTool(tp+"fetch", sdk.ToolDef{
		Name: tp + "fetch",
		Description: "Fetch a URL and extract readable content as markdown-like text. Blocked on private/internal IPs.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":       map[string]interface{}{"type": "string", "description": "HTTP/HTTPS URL to fetch"},
				"max_chars": map[string]interface{}{"type": "integer", "description": "Max characters to return (default 20000)"},
			},
			"required": []string{"url"},
		},
	}, p.handleFetch)

	s.RegisterTool(tp+"render", sdk.ToolDef{
		Name: tp + "render",
		Description: "Render a web page using headless Chromium browser and extract the text content. Supports JavaScript-rendered pages. Returns title and first 5000 characters.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":  map[string]interface{}{"type": "string", "description": "URL to render"},
				"wait": map[string]interface{}{"type": "integer", "description": "Seconds to wait for JS rendering (default 0)"},
			},
			"required": []string{"url"},
		},
	}, p.handleRender)

	log.Printf("[%s] started, timeout=%ds proxy=%q", p.name, p.timeout, p.proxy)
	return nil
}

func (p *Plugin) Stop() error {
	if p.client != nil {
		p.client.CloseIdleConnections()
	}
	log.Printf("[%s] stopped", p.name)
	return nil
}

func getSetting[T any](s sdk.SettingsAPI, key string, def T) T {
	v, err := s.Get(key)
	if err != nil || v == nil { return def }
	val, ok := v.(T)
	if !ok { return def }
	return val
}

func convInt64(v interface{}) (int64, error) {
	switch x := v.(type) {
	case float64: return int64(x), nil
	case int64: return x, nil
	case json.Number: return x.Int64()
	default: return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}

func errorResult(msg string) map[string]interface{} {
	return map[string]interface{}{"isError": true, "content": msg}
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
		if n != nil { privateCIDRs = append(privateCIDRs, n) }
	}
}

func isPrivateIP(ip net.IP) bool {
	for _, n := range privateCIDRs {
		if n.Contains(ip) { return true }
	}
	return false
}

func (p *Plugin) ssrfCheck(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil { return fmt.Errorf("invalid URL: %w", err) }
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("only http/https URLs allowed, got: %s", u.Scheme)
	}
	ips, err := net.LookupHost(u.Hostname())
	if err != nil { return fmt.Errorf("DNS lookup failed: %w", err) }
	for _, ip := range ips {
		if parsed := net.ParseIP(ip); parsed != nil && isPrivateIP(parsed) {
			return fmt.Errorf("blocked request to private IP: %s (%s)", u.Hostname(), ip)
		}
	}
	return nil
}

// ── DuckDuckGo Search ─────────────────────────────────────

type ddgResult struct {
	Title, URL, Snippet string
}

func (p *Plugin) ddgSearch(query string, count int) ([]ddgResult, error) {
	form := url.Values{"q": {query}}
	req, _ := http.NewRequest("POST", "https://html.duckduckgo.com/html/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := p.client.Do(req)
	if err != nil { return nil, fmt.Errorf("request failed: %w", err) }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return parseDDGResults(string(body), count), nil
}

func parseDDGResults(html string, count int) []ddgResult {
	var results []ddgResult
	marker := `result__body"`
	for i := 0; i < len(html); i++ {
		idx := strings.Index(html[i:], marker)
		if idx < 0 { break }
		i += idx
		closeIdx := findClosingTag(html, i, "</div>")
		if closeIdx < 0 { break }
		if r := parseSingleDDGResult(html[i : closeIdx+6]); r.URL != "" {
			results = append(results, r)
			if len(results) >= count { break }
		}
		i = closeIdx + 6
	}
	return results
}

func findClosingTag(s string, start int, tag string) int {
	depth := 1
	for pos := start; pos < len(s); {
		nextOpen := strings.Index(s[pos:], `<div`)
		nextClose := strings.Index(s[pos:], tag)
		if nextClose < 0 { return -1 }
		if nextOpen >= 0 && nextOpen < nextClose {
			depth++
			pos += nextOpen + 4
		} else {
			depth--
			if depth == 0 { return pos + nextClose }
			pos += nextClose + len(tag)
		}
	}
	return -1
}

func parseSingleDDGResult(block string) ddgResult {
	var r ddgResult
	urlMarker := `class="result__a" href="`
	if uIdx := strings.Index(block, urlMarker); uIdx >= 0 {
		start := uIdx + len(urlMarker)
		if end := strings.Index(block[start:], `"`); end >= 0 {
			r.URL = block[start : start+end]
		}
	}
	for _, marker := range []string{`<a class="result__snippet`, `<div class="result__snippet`} {
		if sIdx := strings.Index(block, marker); sIdx >= 0 {
			if aStart := strings.Index(block[sIdx:], `>`); aStart >= 0 {
				snipStart := sIdx + aStart + 1
				snipEnd := strings.Index(block[snipStart:], `</a>`)
				if snipEnd < 0 { snipEnd = strings.Index(block[snipStart:], `</div>`) }
				if snipEnd >= 0 { r.Snippet = stripTags(block[snipStart : snipStart+snipEnd]) }
			}
			break
		}
	}
	return r
}

func (p *Plugin) handleSearch(args map[string]interface{}) (interface{}, error) {
	query, _ := args["query"].(string)
	if query == "" { return errorResult("query is required"), nil }
	count := 5
	if v, ok := args["count"].(float64); ok && v > 0 { count = int(v) }
	if count < 1 { count = 1 }
	if count > 20 { count = 20 }

	results, err := p.ddgSearch(query, count)
	if err != nil { return errorResult("search failed: " + err.Error()), nil }
	if len(results) == 0 { return map[string]interface{}{"content": "No results found."}, nil }

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for %q:\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}
	return map[string]interface{}{"content": strings.TrimSpace(sb.String())}, nil
}

// ── Web Fetch ─────────────────────────────────────────────

func htmlToText(html string) string {
	for _, tag := range []string{"<script", "<style"} {
		closing := "</" + tag[1:] + ">"
		for {
			start := strings.Index(strings.ToLower(html), tag)
			if start < 0 { break }
			end := strings.Index(html[start:], closing)
			if end < 0 { break }
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
		if line == "" { continue }
		in := []rune(line)
		var out []rune
		space := false
		for _, r := range in {
			if unicode.IsSpace(r) {
				if !space { out = append(out, ' '); space = true }
			} else { out = append(out, r); space = false }
		}
		cleaned = append(cleaned, string(out))
	}
	return strings.Join(cleaned, "\n")
}

func stripTags(s string) string {
	var out strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' { inTag = true; continue }
		if r == '>' { inTag = false; continue }
		if !inTag { out.WriteRune(r) }
	}
	return out.String()
}

func (p *Plugin) handleFetch(args map[string]interface{}) (interface{}, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" { return errorResult("url is required"), nil }
	maxChars := 20000
	if v, ok := args["max_chars"].(float64); ok && v > 0 { maxChars = int(v) }
	if maxChars > 500000 { maxChars = 500000 }
	if err := p.ssrfCheck(rawURL); err != nil { return errorResult(err.Error()), nil }

	req, _ := http.NewRequest("GET", rawURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := p.client.Do(req)
	if err != nil { return errorResult("fetch failed: " + err.Error()), nil }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return errorResult(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)), nil
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
			if pretty, err := json.MarshalIndent(v, "", "  "); err == nil { extracted = string(pretty) }
		}
		if extracted == "" { extracted = rawText }
	} else { extracted = rawText }

	extracted = strings.TrimSpace(extracted)
	if len(extracted) > maxChars { extracted = extracted[:maxChars] + "\n\n[Content truncated]" }
	if extracted == "" { extracted = "(empty content)" }

	return map[string]interface{}{
		"content": extracted,
		"details": map[string]interface{}{"url": rawURL, "status": resp.StatusCode, "content_type": ct},
	}, nil
}

// ── Chromium Render ───────────────────────────────────────

func (p *Plugin) handleRender(args map[string]interface{}) (interface{}, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" { return nil, fmt.Errorf("url is required") }
	waitSec, _ := convInt64(args["wait"])
	if waitSec > 0 { time.Sleep(time.Duration(waitSec) * time.Second) }

	var html string
	chromiumPath := "/usr/local/bin/chromium"
	if _, err := os.Stat(chromiumPath); err == nil {
		var out bytes.Buffer
		cmd := exec.Command(chromiumPath, "--headless", "--disable-gpu", "--no-sandbox", "--dump-dom", rawURL)
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("chromium: %w", err)
		}
		html = out.String()
	} else {
		resp, err := http.Get(rawURL)
		if err != nil { return nil, fmt.Errorf("http get: %w", err) }
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil { return nil, fmt.Errorf("read body: %w", err) }
		html = string(body)
	}

	title := ""
	if m := regexp.MustCompile(`<title>([^<]+)</title>`).FindStringSubmatch(html); len(m) > 1 {
		title = m[1]
	}

	var textOut bytes.Buffer
	pyCmd := exec.Command("python3", "-c", `
import sys, re, html
raw = sys.stdin.read()
text = re.sub(r'<[^>]+>', ' ', raw)
text = re.sub(r'\s+', ' ', text).strip()
text = html.unescape(text)
sys.stdout.write(text)
`)
	pyCmd.Stdin = strings.NewReader(html)
	pyCmd.Stdout = &textOut
	pyCmd.Run()
	text := strings.TrimSpace(textOut.String())

	origLen := len(text)
	truncated := origLen > 5000
	if truncated { text = text[:5000] }

	result := ""
	if title != "" { result = fmt.Sprintf("标题: %s\nURL: %s\n\n", title, rawURL) }
	result += text
	if truncated { result += fmt.Sprintf("\n\n...（内容过长，仅显示前 5000 字符，共 %d 字符）", origLen) }

	return map[string]interface{}{"content": result, "title": title}, nil
}

func NewPlugin(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{name: name}, nil
}

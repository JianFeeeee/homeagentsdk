package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseBrowserSessionTimeoutRequiresExplicitValue(t *testing.T) {
	_, err := parseBrowserSessionTimeout(map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "timeout is required") {
		t.Fatalf("expected required timeout error, got %v", err)
	}
}

func TestParseBrowserSessionTimeoutAcceptsPositiveDuration(t *testing.T) {
	got, err := parseBrowserSessionTimeout(map[string]interface{}{"timeout": "2h30m"})
	if err != nil {
		t.Fatal(err)
	}
	if got != 2*time.Hour+30*time.Minute {
		t.Fatalf("timeout=%v", got)
	}
}

func TestParseBrowserSessionTimeoutRejectsInvalidOrNonPositive(t *testing.T) {
	for _, value := range []string{"invalid", "0s", "-1m"} {
		if _, err := parseBrowserSessionTimeout(map[string]interface{}{"timeout": value}); err == nil {
			t.Errorf("timeout %q should be rejected", value)
		}
	}
}

func TestBrowserStartReuseResetsExplicitCloseTime(t *testing.T) {
	p := &Plugin{
		name: "browser",
		sessions: map[string]*BrowserSession{
			"browser_1": {
				id:         "browser_1",
				shared:     true,
				sessionKey: "qq",
				createdAt:  time.Now().Add(-time.Hour),
				timeout:    time.Minute,
				currentURL: "https://example.com",
			},
		},
	}

	before := time.Now()
	result, err := p.handleBrowserStart(map[string]interface{}{"source": "qq", "timeout": "3h"})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]interface{})
	if out["status"] != "reused" || out["timeout"] != "3h0m0s" {
		t.Fatalf("unexpected result: %#v", out)
	}
	s := p.sessions["browser_1"]
	if s.timeout != 3*time.Hour || s.createdAt.Before(before) {
		t.Fatalf("deadline not reset: createdAt=%v timeout=%v", s.createdAt, s.timeout)
	}
}

// ── Bing 解析器（2026-09 版式）─────────────────────────────
//
// 背景：旧实现把块内**第一个 <a>** 当标题 —— 拿到的是 Bing 的「来源行」
// `deepin.orghttps://www.deepin.org`；摘要正则 `<div class="b_caption">.*?<p>`
// 对现代 Bing 命中 0/N（摘要已迁到 p.b_lineclamp*），于是结果「有标题没摘要」，
// 模型只好反复换词重搜。夹具 testdata/bing_cn.html 是真实 cn.bing.com 响应裁剪。

func TestParseBingResultsRealBingHTML(t *testing.T) {
	page, err := os.ReadFile("testdata/bing_cn.html")
	if err != nil {
		t.Fatalf("读取夹具失败: %v", err)
	}
	results := parseBingResults(string(page), 3)
	if len(results) != 3 {
		t.Fatalf("应解析出 3 条，实际 %d 条: %+v", len(results), results)
	}
	for i, r := range results {
		if !strings.HasPrefix(r.URL, "http") {
			t.Errorf("第 %d 条 URL 不是真实地址: %q", i+1, r.URL)
		}
		if strings.Contains(r.Title, "http") || strings.Contains(r.Title, "://") {
			t.Errorf("第 %d 条标题混入了 URL（旧 bug 的典型症状）: %q", i+1, r.Title)
		}
		if r.Snippet == "" {
			t.Errorf("第 %d 条没有摘要（旧 bug 的典型症状）: %+v", i+1, r)
		}
	}
	// 第一条必须与样本里的真实结果一致
	if results[0].URL != "https://www.deepin.org/" {
		t.Errorf("第一条 URL 应为 https://www.deepin.org/，实际 %q", results[0].URL)
	}
	if !strings.Contains(results[0].Title, "deepin") {
		t.Errorf("第一条标题不对: %q", results[0].Title)
	}
	if len(results[0].Snippet) < 10 || strings.Contains(results[0].Snippet, "://") {
		t.Errorf("第一条摘要不对（应是有内容的文本）: %q", results[0].Snippet)
	}
}

// 块内嵌套 <li>（deep links）时不能截断 —— 旧的 `<li class="b_algo"(?s)(.*?)</li>` 会在此翻车
func TestParseBingResultsNestedLiKeepsResult(t *testing.T) {
	page := `<ol id="b_results"><li class="b_algo" data-id iid=SERP.1>` +
		`<h2><a href="https://a.example/x" h="ID=SERP,1">真标题</a></h2>` +
		`<div class="b_caption"><p class="b_lineclamp2">真摘要</p></div>` +
		`<div><ul><li><a href="https://sub.example/deeplink">子链接</a></li></ul></div>` +
		`</li><li class="b_algo"><h2><a href="https://b.example/y">第二条</a></h2>` +
		`<p class="b_lineclamp3">摘要二</p></li></ol>`
	rs := parseBingResults(page, 5)
	if len(rs) != 2 {
		t.Fatalf("应解析 2 条，实际 %d 条: %+v", len(rs), rs)
	}
	if rs[0].URL != "https://a.example/x" || rs[0].Title != "真标题" || rs[0].Snippet != "真摘要" {
		t.Errorf("第一条解析错误: %+v", rs[0])
	}
	if rs[1].Title != "第二条" || rs[1].Snippet != "摘要二" {
		t.Errorf("第二条（无 b_caption，摘要走 b_lineclamp3）解析错误: %+v", rs[1])
	}
}

func TestBingRealURLDecodesRedirectWrapper(t *testing.T) {
	// Bing 跳转包装：/ck/a?...&u=a1<base64url>
	wrapped := "/ck/a?!&&p=abc&u=a1aHR0cHM6Ly93d3cuZGVlcGluLm9yZy96aC9EZWVwaW4v&ntb=1"
	if got := bingRealURL(wrapped); got != "https://www.deepin.org/zh/Deepin/" {
		t.Errorf("未解开跳转包装: %q", got)
	}
	if got := bingRealURL("https://direct.example/p"); got != "https://direct.example/p" {
		t.Errorf("直链不应被改动: %q", got)
	}
	// 解不开时保守返回原值，不能返回空
	bad := "/ck/a?u=a1!!!!"
	if got := bingRealURL(bad); got == "" {
		t.Errorf("解不开时应保留原值，实际返回空")
	}
}

// roundTripFunc 把任意请求转给本地测试服务器，从而离线测 bingSearch 的完整路径
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBingSearchReportsParseFailureInsteadOfEmptyResult(t *testing.T) {
	var seenURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html><body>no result blocks here</body></html>"))
	}))
	defer srv.Close()

	p := &Plugin{name: "browser", client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seenURL = r.URL.String()
		return srv.Client().Transport.RoundTrip(&http.Request{
			Method: r.Method, URL: mustParseURL(t, srv.URL), Header: r.Header, Body: r.Body,
		})
	})}}

	if _, err := p.bingSearch("任意查询", 5); err == nil {
		t.Fatal("解析不出结果时必须报错，而不是伪装成「没有结果」")
	} else if !strings.Contains(err.Error(), "未解析出结果") {
		t.Errorf("错误信息应说明是解析失败: %v", err)
	}
	// 数据源必须是 cn.bing.com（www.bing.com 对程序化请求回 302，拿不到结果块）
	if !strings.Contains(seenURL, "cn.bing.com") {
		t.Errorf("应请求 cn.bing.com，实际 %q", seenURL)
	}
	if strings.Contains(seenURL, "www.bing.com") {
		t.Errorf("不应再请求 www.bing.com: %q", seenURL)
	}
}

// 正常路径：能解析出结果时返回结果且不报错
func TestBingSearchParsesFixtureThroughClient(t *testing.T) {
	page, err := os.ReadFile("testdata/bing_cn.html")
	if err != nil {
		t.Fatalf("读取夹具失败: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	}))
	defer srv.Close()

	p := &Plugin{name: "browser", client: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return srv.Client().Transport.RoundTrip(&http.Request{
			Method: r.Method, URL: mustParseURL(t, srv.URL), Header: r.Header, Body: r.Body,
		})
	})}}

	results, err := p.bingSearch("deepin", 2)
	if err != nil {
		t.Fatalf("应成功，实际 %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("应返回 2 条（count 生效），实际 %d", len(results))
	}
	if results[0].Snippet == "" {
		t.Errorf("摘要不应为空: %+v", results[0])
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("解析测试 URL 失败: %v", err)
	}
	return u
}

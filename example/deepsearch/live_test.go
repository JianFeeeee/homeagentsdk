package main

import (
	"os"
	"strings"
	"testing"
)

// 真实后端联调（默认跳过，需显式指定地址）：
//
//	DEEPSEARCH_LIVE_SEARXNG=http://127.0.0.1:8888 go test -run TestLiveSearxng -v ./...
//
// 它跑的就是当初失败的场景（日志里那条「你的搜索能力好像不太行啊」对应的查询），
// 用来回答一个具体问题：换了后端之后，模型拿到的是不是「带摘要的相关结果」。
func TestLiveSearxng(t *testing.T) {
	base := os.Getenv("DEEPSEARCH_LIVE_SEARXNG")
	if base == "" {
		t.Skip("未设置 DEEPSEARCH_LIVE_SEARXNG，跳过真实后端联调")
	}
	p := &Plugin{
		name:      "deepsearch",
		searxURL:  strings.TrimRight(base, "/"),
		maxItems:  6,
		language:  "zh-CN",
		fetchMax:  1200,
		userAgent: defaultUA,
	}
	p.ensure()

	// 1) 自检
	st, err := p.handleStatus(map[string]interface{}{})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("status: %v", st)

	// 2) 当初失败的那条查询
	res, err := p.handleSearch(map[string]interface{}{"query": "深度科技 deepin 开发者 被开除"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	txt := res.(map[string]interface{})["content"].(string)
	t.Logf("检索结果：\n%s", txt)
	if !strings.Contains(txt, "摘要：") {
		t.Errorf("结果里应当有摘要（这正是原实现缺失的东西）")
	}
	if !strings.Contains(txt, "覆盖：") {
		t.Errorf("应报告引擎覆盖度")
	}

	// 3) 正文抓取（取第一条结果的 URL）
	var firstURL string
	for _, line := range strings.Split(txt, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "http") {
			firstURL = l
			break
		}
	}
	if firstURL == "" {
		t.Fatal("未从结果中解析出 URL")
	}
	page, err := p.handleFetch(map[string]interface{}{"url": firstURL, "max_chars": float64(600)})
	if err != nil {
		t.Logf("抓取 %s 失败（真实站点有反爬/需 JS 属正常）：%v", firstURL, err)
	} else {
		body := page.(map[string]interface{})["content"].(string)
		t.Logf("抓取 %s 正文前 400 字：%s", firstURL, oneLine(body, 400))
	}

	// 4) 深检索
	deep, err := p.handleDeep(map[string]interface{}{"query": "统信 UOS 内核工程师 西装 事件", "top_k": float64(2)})
	if err != nil {
		t.Fatalf("deep: %v", err)
	}
	dTxt := deep.(map[string]interface{})["content"].(string)
	if !strings.Contains(dTxt, "候选清单") || !strings.Contains(dTxt, "正文证据") {
		t.Errorf("深检索输出结构不对")
	}
	t.Logf("深检索输出前 800 字：\n%s", oneLine(dTxt, 800))
}

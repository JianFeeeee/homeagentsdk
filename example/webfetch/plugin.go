package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

type Plugin struct {
	name string
	sdk  *sdk.PluginSDK
}

func (p *Plugin) Name() string { return p.name }

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	s.SetAutoRestart(true)
	p.sdk = s

	s.RegisterTool("web_fetch", sdk.ToolDef{
		Name:        "web_fetch",
		Description: "获取网页文字内容。使用无头 Chromium 浏览器渲染页面后提取正文文字，返回标题和前 5000 字符。适用于需要查看网页内容的场景。",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":  map[string]interface{}{"type": "string", "description": "要访问的网页 URL"},
				"wait": map[string]interface{}{"type": "integer", "description": "等待秒数（用于 JS 渲染页面，默认 0）"},
			},
			"required": []string{"url"},
		},
	}, p.handleWebFetch)

	log.Printf("[%s] plugin started", p.name)
	return nil
}

func (p *Plugin) Stop() error { return nil }

func convInt64(v interface{}) (int64, error) {
	switch x := v.(type) {
	case float64:
		return int64(x), nil
	case int64:
		return x, nil
	case json.Number:
		return x.Int64()
	case string:
		return 0, fmt.Errorf("cannot convert string to int64")
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}

func (p *Plugin) handleWebFetch(args map[string]interface{}) (interface{}, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	waitSec, _ := convInt64(args["wait"])

	if waitSec > 0 {
		time.Sleep(time.Duration(waitSec) * time.Second)
	}

	var html string
	chromiumPath := "/usr/local/bin/chromium"
	if _, err := os.Stat(chromiumPath); err == nil {
		var out bytes.Buffer
		argsList := []string{"--headless", "--disable-gpu", "--no-sandbox", "--dump-dom", url}
		cmd := exec.Command(chromiumPath, argsList...)
		cmd.Stdout = &out
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("chromium: %w", err)
		}
		html = out.String()
	} else {
		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("http get: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
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
	pyCmd.Stderr = nil
	pyCmd.Run()
	text := strings.TrimSpace(textOut.String())

	origLen := len(text)
	truncated := origLen > 5000
	if truncated {
		text = text[:5000]
	}

	result := ""
	if title != "" {
		result = fmt.Sprintf("标题: %s\nURL: %s\n\n", title, url)
	}
	result += text
	if truncated {
		result += fmt.Sprintf("\n\n...（内容过长，仅显示前 5000 字符，共 %d 字符）", origLen)
	}

	return map[string]interface{}{
		"content": result,
		"title":   title,
	}, nil
}

func NewPlugin(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{name: name}, nil
}

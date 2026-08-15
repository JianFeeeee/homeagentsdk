// Package main 是一个外部插件示例（编译为 .so 通过 -buildmode=plugin）。
// 在 StagePostAction 阶段清洗 LLM 输出中的工具调用残留（思维泄漏）。
//
// 编译：
//
//	go build -buildmode=plugin -o sanitizer.so .
//
// 安装到 HomeAgent 插件目录（如 plugins/sanitizer/plugin.so），
// HomeAgent 自动通过 tryLoadSO 加载。
package main

import (
	"log"
	"regexp"
	"strings"

	"gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

var (
	toolCallTagRE   = regexp.MustCompile(`(?s)<tool_call[^>]*>.*?</tool_call>`)
	invokeTagRE     = regexp.MustCompile(`(?s)<invoke[^>]*>.*?</invoke>`)
	toolTagRE       = regexp.MustCompile(`(?s)<tool[^>]*>.*?</tool>`)
	functionTagRE   = regexp.MustCompile(`(?s)<function[^>]*>.*?</function>`)
	toolCodeBlockRE = regexp.MustCompile("(?s)```(?:xml|json)?\\s*<tool_call[^>]*>.*?</tool_call>\\s*```")
	invokeCodeBlockRE = regexp.MustCompile("(?s)```(?:xml|json)?\\s*<invoke[^>]*>.*?</invoke>\\s*```")
	toolCodeBlockRE2 = regexp.MustCompile("(?s)```(?:xml|json)?\\s*<tool[^>]*>.*?</tool>\\s*```")
	chineseMarkerRE = regexp.MustCompile(`(?s)【tool_call】.*?【/tool_call】`)
	multiNewlineRE  = regexp.MustCompile(`\n{3,}`)
	toolNameRE      = regexp.MustCompile(`^(cmd_run|terminal_create|terminal_write|memory_|knowledge_|doc_|social_|output_set_channel|output_send|llm_|plgreload|spawn_child|child_result|describe_image|transcribe_audio|ocr_image|timer_set|plugin_install|plugin_remove|qq_|a2a_|mcp_|healthcheck|files_|web_)`)
	placeholderRE   = regexp.MustCompile(`(?i)\{\{\s*tool\s*[:：][^}]*\}\}`)
	atToolRE        = regexp.MustCompile(`(?i)^@\s*tool\b`)
)

type Plugin struct{}

func (p *Plugin) Name() string { return "sanitizer" }

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	s.SetAutoRestart(true)
	s.RegisterStage(sdk.StagePostAction, func(ctx *sdk.StageContext) error {
		ctx.Lock()
		before := len(ctx.LLMText)
		ctx.LLMText = cleanToolCallLeakage(ctx.LLMText)
		after := len(ctx.LLMText)
		ctx.Unlock()
		if before != after {
			log.Printf("[sanitizer] cleaned %d bytes (before=%d after=%d)", before-after, before, after)
		}
		return nil
	})
	log.Printf("[sanitizer] stage PostAction registered")
	return nil
}

func (p *Plugin) Stop() error { return nil }

func NewPluginFactory(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{}, nil
}

func cleanToolCallLeakage(content string) string {
	if content == "" {
		return content
	}

	before := len(content)

	content = toolCodeBlockRE.ReplaceAllString(content, "")
	content = invokeCodeBlockRE.ReplaceAllString(content, "")
	content = toolCodeBlockRE2.ReplaceAllString(content, "")

	content = toolCallTagRE.ReplaceAllString(content, "")
	content = invokeTagRE.ReplaceAllString(content, "")
	content = toolTagRE.ReplaceAllString(content, "")
	content = functionTagRE.ReplaceAllString(content, "")

	content = chineseMarkerRE.ReplaceAllString(content, "")

	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			cleaned = append(cleaned, line)
			continue
		}
		if placeholderRE.MatchString(trimmed) || atToolRE.MatchString(trimmed) {
			continue
		}
		if m := toolNameRE.FindStringIndex(trimmed); m != nil {
			rest := trimmed[m[1]:]
			if strings.HasPrefix(rest, "(") && strings.Contains(rest, ")") {
				continue
			}
		}
		cleaned = append(cleaned, line)
	}
	content = strings.Join(cleaned, "\n")

	content = multiNewlineRE.ReplaceAllString(content, "\n\n")
	content = strings.TrimSpace(content)

	if len(content) != before {
		log.Printf("[sanitizer] cleanToolCallLeakage: %d bytes removed", before-len(content))
	}
	return content
}

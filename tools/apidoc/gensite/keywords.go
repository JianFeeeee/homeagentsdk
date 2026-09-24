package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 中文检索关键词。
//
// 问题：SDK 里 100 个有摘要的符号中 **66 个是英文注释**
// （`RegisterTool registers a tool that the LLM can call.`），
// 于是搜「注册工具」找不到 RegisterTool —— 而受众主要是中文。
//
// 解法不是翻译源码注释（那会让代码与文档脱节），而是给检索索引补一层
// **人工标注的功能词**：不影响页面展示，只让中文能搜到。
//
// 词表在 tools/apidoc/keywords.json。规则用「前缀 → 词」批量覆盖
// （Register* 全带「注册」），少数重点符号再逐条补充。
type keywordTable struct {
	Rules   []keywordRule       `json:"rules"`
	Symbols map[string][]string `json:"symbols"`
}

type keywordRule struct {
	// Prefix 匹配符号名前缀；Contains 匹配名字里是否含该子串。二选一。
	Prefix   string   `json:"prefix,omitempty"`
	Contains string   `json:"contains,omitempty"`
	Words    []string `json:"words"`
}

// loadKeywords 读词表。找不到就返回空表（不报错中断）——
// 检索关键词是**增强**，缺失时退化为原行为，不该让构建失败。
func loadKeywords() (*keywordTable, error) {
	path := keywordPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return &keywordTable{}, err
	}
	var t keywordTable
	if err := json.Unmarshal(data, &t); err != nil {
		return &keywordTable{}, fmt.Errorf("解析 %s: %w", path, err)
	}
	return &t, nil
}

// lookup 返回某符号的中文检索词（可能为空）。
func (t *keywordTable) lookup(name, docBrief string) []string {
	if t == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(ws []string) {
		for _, w := range ws {
			w = strings.TrimSpace(w)
			if w == "" || seen[w] {
				continue
			}
			seen[w] = true
			out = append(out, w)
		}
	}
	for _, r := range t.Rules {
		if r.Prefix != "" && strings.HasPrefix(name, r.Prefix) {
			add(r.Words)
		}
		if r.Contains != "" && strings.Contains(name, r.Contains) {
			add(r.Words)
		}
	}
	add(t.Symbols[name])
	return out
}

// keywordPath 找 keywords.json：可执行文件旁 → 源码目录 → 工作目录。
func keywordPath() string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "keywords.json"))
	}
	candidates = append(candidates,
		filepath.Join("tools", "apidoc", "keywords.json"),
		"keywords.json",
	)
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

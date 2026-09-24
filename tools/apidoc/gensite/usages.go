package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Usage 是一条真实用法：某个示例插件在某行用了这个 API。
//
// 为什么要有它：API 参考最容易变成「签名罗列」——读者看得到参数，
// 却不知道该怎么用。SDK 的源码注释里几乎没有可运行示例（实测只有 7 行缩进
// 代码块），但 example/ 下有 21 个**真实可编译**的示例插件。把它们里的调用点
// 反查到每个 API 上，读者就能直接跳到能跑的代码。
type Usage struct {
	Plugin string `json:"plugin"` // 示例名（example 下的目录名）
	File   string `json:"file"`   // 相对 SDK 仓根的路径
	Line   int    `json:"line"`
	Code   string `json:"code"` // 该行原文（裁剪首尾空白）
}

var callRe = regexp.MustCompile(`\.([A-Z][A-Za-z0-9_]*)\s*\(`)

// scanUsages 遍历 example/ 下所有 .go 文件，找出每个 API 的真实调用点。
//
// names 是要找的符号名集合（越小越快）。只扫 example/，不扫 tools/——
// 工具链自己也会调 SDK，那属于内部实现，不是「用法示例」。
func scanUsages(exDir string, names map[string]bool) map[string][]Usage {
	out := map[string][]Usage{}
	_ = filepath.Walk(exDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Dir(exDir), path)
		plugin := pluginName(path, exDir)
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		line := 0
		for sc.Scan() {
			line++
			text := sc.Text()
			trimmed := strings.TrimSpace(text)
			// 跳过注释行：注释里提到 API 名不算「用法」。
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
				continue
			}
			for _, m := range callRe.FindAllStringSubmatch(text, -1) {
				name := m[1]
				if !names[name] {
					continue
				}
				out[name] = append(out[name], Usage{
					Plugin: plugin, File: rel, Line: line, Code: truncate(trimmed, 110),
				})
			}
		}
		return nil
	})
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool {
			if out[k][i].Plugin != out[k][j].Plugin {
				return out[k][i].Plugin < out[k][j].Plugin
			}
			return out[k][i].Line < out[k][j].Line
		})
		// 每个 API 最多留 4 条，避免页面被用法淹没；优先保留不同插件。
		out[k] = dedupeByPlugin(out[k], 4)
	}
	return out
}

// pluginName 从路径里取示例名：example/<plugin>/....go → <plugin>。
func pluginName(path, exDir string) string {
	rel, err := filepath.Rel(exDir, path)
	if err != nil {
		return "?"
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) > 0 {
		return parts[0]
	}
	return "?"
}

func dedupeByPlugin(in []Usage, limit int) []Usage {
	seen := map[string]int{}
	var out []Usage
	for _, u := range in {
		if seen[u.Plugin] >= 1 {
			continue
		}
		seen[u.Plugin]++
		out = append(out, u)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

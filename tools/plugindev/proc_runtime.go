package main

import (
	"embed"
	"fmt"
	"os"
)

// 子进程插件运行时（外部插件多进程化，Part 3）。
//
// 与旧 C ABI bridge 的差异：
//   - 模板改为**真实 .go 源文件**（templates/proc_main.go.tmpl）而非 raw string：
//     900+ 行代码塞在字符串里，写错只能等生成插件时才炸；作为源文件可被
//     gofmt / go vet / parser 直接检查。
//   - 构建从 `-buildmode=c-shared` + CGO_ENABLED=1 改为普通 `go build` + CGO_ENABLED=0，
//     交叉编译不再需要目标平台的 C 工具链（§3.1 连带消失项）。
//
// 设计依据：docs/zh/架构迁移评估.md §3、docs/zh/plugin-migration-plan.md Part 3

//go:embed templates/proc_main.go.tmpl
var procTemplates embed.FS

// procEntryFile 是子进程插件的入口二进制名（与内核 internal/plugin/dynamic.go 的 binEntry 一致）。
const procEntryFile = "plugin.bin"

// procGenFile 是生成的运行时文件名。
// 前缀 z_ 使其在目录列表中排在业务代码之后，且与旧 bridge 的 z_bridge_gen.go 风格一致。
const procGenFile = "z_proc_gen.go"

// generateProcRuntime 把子进程运行时写入插件目录，返回清理函数。
//
// 与 generateBridge 的差异：只写一个 .go 文件，不需要 C 入口（z_entry.c）。
func generateProcRuntime() (func(), error) {
	data, err := procTemplates.ReadFile("templates/proc_main.go.tmpl")
	if err != nil {
		return nil, fmt.Errorf("读取内嵌模板: %w", err)
	}

	// 清理可能残留的 C ABI 产物：同目录同时存在两套 main 会编译冲突。
	// 这也让 .so → .bin 的切换无需人工清理。
	for _, stale := range []string{"z_bridge_gen.go", "z_entry.c"} {
		os.Remove(stale)
	}

	if err := os.WriteFile(procGenFile, data, 0644); err != nil {
		return nil, fmt.Errorf("写入 %s: %w", procGenFile, err)
	}
	return func() { os.Remove(procGenFile) }, nil
}

// isProcEntry 判断 plg.json 的 entry 是否声明了子进程模式。
func isProcEntry(entry string) bool {
	return entry == procEntryFile
}

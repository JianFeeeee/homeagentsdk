// 命令 annotate_parallel 按 SDK 声明风格为工具加并发安全声明。
//
// 风格要求（照 SDK 的 NoMemory 走，不自创）：
//
//	· 声明项是**结构体字段**（ParallelSafe / Serial），不是注释标记；
//	· 插在 Parameters 之后、handler 之前 —— 即字面量的**末尾**，
//	  与 SDK 里 NoMemory/ContextPolicy/RecallPolicy 的位置一致；
//	· Name 保持在首位，不打散 gofmt 对齐。
//
// 为什么用括号深度定位插入点：之前用正则找"最后一个顶层字段"，
// 会被嵌套 map 里的同形文本骗到，结果把声明插到 Parameters 中间，
// 甚至把文件改坏（823 处重排）。深度计数是唯一可靠的。
//
// 用法：annotate_parallel <file> <tool:kind:note> ...
//
//	kind: parallel | serial
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "用法: annotate_parallel <file> <tool:kind:note>...")
		os.Exit(2)
	}
	path := os.Args[1]
	lines := readLines(path)
	// 从后往前改，避免行号漂移
	type job struct {
		tool, kind, note string
	}
	var jobs []job
	for _, arg := range os.Args[2:] {
		p := strings.SplitN(arg, ":", 3)
		if len(p) != 3 {
			fmt.Fprintf(os.Stderr, "参数格式错: %q\n", arg)
			os.Exit(2)
		}
		jobs = append(jobs, job{p[0], p[1], p[2]})
	}
	// 反序处理（同一文件里多个工具，位置互不影响，但保守起见从后往前）
	for i := len(jobs) - 1; i >= 0; i-- {
		j := jobs[i]
		at, err := findInsertPoint(lines, j.tool)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  跳过 %s: %v\n", j.tool, err)
			continue
		}
		// 幂等：块内已有并发声明就跳过。
		//
		// 不加这条时，重跑会在已标注的工具上**再插一份** —— 而
		// duplicate field name 是编译期错误，跨文件批量跑时定位成本很高。
		if blockHasDecl(lines, j.tool) {
			fmt.Printf("  %s 已有声明，跳过\n", j.tool)
			continue
		}
		field := "ParallelSafe: true,"
		if j.kind == "serial" {
			field = "Serial: true,"
		}
		ins := []string{"\t\t// " + j.note, "\t\t" + field}
		out := append([]string{}, lines[:at]...)
		out = append(out, ins...)
		out = append(out, lines[at:]...)
		lines = out
		fmt.Printf("  %s @line %d (%s)\n", j.tool, at+1, j.kind)
	}
	writeLines(path, lines)
}

// findInsertPoint 找到该 RegisterTool 字面量中，Parameters 闭合之后的位置。
func findInsertPoint(lines []string, tool string) (int, error) {
	// 匹配两种注册形式：
	//   RegisterTool("get_article", ...)        字面量
	//   RegisterTool(tp+"get_article", ...)     变量前缀 + 字面量
	// 只认字面量会漏掉后者 —— example 里绝大多数是变量前缀形式。
	head := fmt.Sprintf(`RegisterTool("%s"`, tool)
	alt := fmt.Sprintf(`+"%s"`, tool)
	start := -1
	for i, l := range lines {
		if strings.Contains(l, head) {
			start = i
			break
		}
	}
	if start < 0 {
		// ★ 必须 RegisterTool( 与字面量在**同一行**。
		//
		// 我第一版只找含 `+"name"` 的行，命中了函数体里的散落字面量
		// （vanblog 的 handleAuth 里满是 "restore"/"update" 这类 case 分支），
		// 起点错到函数体中间，深度追踪再也回不到 2 ⇒ 插入点落在 1300+ 行，
		// 把文件改坏。
		//
		// 症状离原因很远：报错说"expected 1 expression"，指向的是一处
		// 看起来完全正常的 case 分支。
		for i, l := range lines {
			if strings.Contains(l, alt) && strings.Contains(l, "RegisterTool(") {
				start = i
				break
			}
		}
	}
	if start < 0 {
		return 0, fmt.Errorf("找不到 RegisterTool(%q)", tool)
	}
	// 从 RegisterTool( 开始做括号深度追踪，找到 ToolDef 字面量的闭合 "}," 行
	depth := 0
	started := false
	enteredAt := -1
	inStr := false
	esc := false
	for i := start; i < len(lines); i++ {
		// peak = 本行内的峰值深度。
		//
		// 每行重置：它表示"这一行曾深入到多深"，不是全程最大值 ——
		// 全程最大值一旦到过 3 就永远是 3，"曾进入 Parameters"判据随之失效。
		peak := depth
		for _, ch := range lines[i] {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' && inStr {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = !inStr
				continue
			}
			if inStr {
				continue
			}
			switch ch {
			case '(', '{', '[':
				depth++
				started = true
				if depth > peak {
					peak = depth
				}
			case ')', '}', ']':
				depth--
			}
		}
		// 插入点 = Parameters 字段的**闭合之后**。
		//
		// 精确定位法：Parameters 起始处的深度是 3（RegisterTool( → ToolDef{ →
		// Parameters{）；它的闭合就是深度**首次从 3 回到 2** 的那一行。
		//
		// ⚠️ 我第一版没有这样做，而是"找 required 行的下一行，没有就返回字面量
		// 闭合行"。对于没有 required 的工具（如 config_list_plugins），
		// 后者落在 properties{} 内部 —— 生成的代码是
		//     "properties": map[string]interface{}{},
		//     ParallelSafe: true,        ← 跑到 map 里去了
		// 编译报 undefined: ParallelSafe，症状离原因很远。
		// 插入点 = Parameters 字段闭合的**下一行**。
		//
		// 深度：RegisterTool( =1, ToolDef{ =2, Parameters{ =3。
		//
		// ★ 两个坑都是"同一行内深度进出平衡"造成的：
		//
		//  1. 空 properties：`"properties": map[string]interface{}{},`
		//     深度 3→2 在**同一行**完成。所以不能用"曾触及 3"作门控，
		//     必须记住**行号**：进入 3 的那行之后，首个回到 2 的行才是闭合行。
		//
		//  2. Name:/Description: 本来就在深度 2，早于 Parameters。
		//     只判 depth==2 会在 Name 行就返回，插入点跑到 RegisterTool 之前，
		//     编译报 "expected 1 expression"。
		// 判定"进入过 Parameters"要按**行内峰值深度**，不能只看行末净深度。
		//
		// get_meta 的 Parameters 全在一行：
		//     Parameters: map[string]interface{}{"type":"object","properties":map[string]interface{}{}},
		// 这行净深度变化是 0（进去又出来）—— 只看净深就永远察觉不到曾进入
		// 深度 3 ⇒ 追踪一路跑到 1305 行才"收敛"，插入点落在某个 case 分支
		// 中间，文件改坏。症状离原因很远：报错指向一处看起来完全正常的
		// switch case。
		if started && peak >= 3 {
			enteredAt = i
		}
		// 闭合判定：进入过 Parameters（enteredAt）之后，深度回到 2 的那一行
		// **就是** Parameters 的闭合行；插入点取它的**下一行**。
		//
		// ★ 不能要求 i > enteredAt：Parameters 写在单行时（get_meta 就是）
		//   enteredAt 与闭合行是**同一行**，加上这个条件会跳到再下一行，
		//   插到 log.Printf 之前 —— 不报错，但声明落在了字面量外面。
		if enteredAt >= 0 && i >= enteredAt && depth <= 2 {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("括号深度追踪未收敛")
}

// blockHasDecl 报告该工具的字面量里是否已有并发声明。
func blockHasDecl(lines []string, tool string) bool {
	head := fmt.Sprintf(`RegisterTool("%s"`, tool)
	alt := fmt.Sprintf(`+"%s"`, tool)
	start := -1
	for i, l := range lines {
		if strings.Contains(l, head) || (strings.Contains(l, alt) && strings.Contains(l, "RegisterTool(")) {
			start = i
			break
		}
	}
	if start < 0 {
		return false
	}
	// 从注册行往后找 30 行（工具定义不会更长）
	for i := start; i < len(lines) && i <= start+30; i++ {
		if strings.Contains(lines[i], "ParallelSafe:") || strings.Contains(lines[i], "Serial:") {
			return true
		}
	}
	return false
}

func readLines(p string) []string {
	f, err := os.Open(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

func writeLines(p string, lines []string) {
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

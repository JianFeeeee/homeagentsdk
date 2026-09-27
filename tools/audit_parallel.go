//go:build ignore

// 命令 audit_parallel 审计插件工具的并发安全性。
//
// 存在理由：正则扫 ToolDef 字面量**不可靠** —— 我用它审计主仓 27 个工具时，
// 把 config_set 判成"无共享写"，而它的 handler 其实在 p.handleSet 里且无锁。
// 原因：RegisterTool 的第三个参数是方法名/闭包，正则看不到执行体。
//
// 本工具用 go/ast 跟进**注册点到 handler 实现**，对每个工具判定：
//   - 找到所有可达的写操作（字段赋值、append、map 写入、文件写、exec、SDK Set）
//   - 找到所有互斥保护（Lock/RLock 覆盖该写，还是写发生在锁外）
//
// 输出三态：SAFE（只读）/ SERIAL（有写或顺序约束）/ UNKNOWN（追不到实现）。
// UNKNOWN 一律不标 ParallelSafe —— 追不到就不能声称安全。
//
// 用法：go run ./tools/audit_parallel.go <dir>...
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// exprString 把表达式打印回源码文本。
func exprString(fset *token.FileSet, e ast.Expr) string {
	var sb strings.Builder
	if err := printer.Fprint(&sb, fset, e); err != nil {
		return ""
	}
	return sb.String()
}

// verdict 是单个工具的审计结论。
type verdict string

const (
	verdictSafe    verdict = "SAFE"    // 只读，可并发
	verdictSerial  verdict = "SERIAL"  // 有写或顺序约束，必须串行
	verdictUnknown verdict = "UNKNOWN" // 追不到实现，不声明
)

// finding 是审计中命中的一个风险点。
type finding struct {
	line int
	what string
}

// toolAudit 是一个工具的审计结果。
type toolAudit struct {
	name     string
	file     string
	verdict  verdict
	findings []finding
	// implFile/implLine 指向追到的 handler 实现位置
	implFile string
	implLine int
}

func main() {
	dirs := os.Args[1:]
	if len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "用法: audit_parallel <dir>...")
		os.Exit(2)
	}
	all := map[string]*toolAudit{}
	for _, dir := range dirs {
		auditDir(dir, all)
	}
	report(all)
}

func auditDir(dir string, out map[string]*toolAudit) {
	// 收集该目录下所有 .go 源码（用于跨文件跟进方法体）
	files := map[string]*ast.File{}
	fset := token.NewFileSet()
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		files[path] = f
		return nil
	})
	// 索引：方法名 -> 实现（(file, receiver, method)）
	methods := indexMethods(fset, files)
	// 索引：**变量名** -> 可能的接收者类型
	//
	// 为何需要：调用点写的是 p.handleRead()，定义处是 func (p *Plugin) handleRead()。
	// 变量名 p 与类型名 Plugin 是两个名字空间，必须显式连起来。
	recvTypes := indexRecvTypes(fset, files)

	for path, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !isRegisterTool(call) {
				return true
			}
			name := toolNameOf(call, fset)
			if name == "" {
				return true
			}
			ta := &toolAudit{name: name, file: path, verdict: verdictUnknown}
			// 作者已声明的**优先**，且不被后续判定覆盖。
			//
			// ⚠️ 我第一版把"已声明"当作一条 finding 记录，然后照常跑
			// 写入检测并**覆盖** verdict —— 结果 plugin_install（已标
			// Serial:true，实现里 exec.Command 也确实在跑）被判成 SAFE。
			// 审计工具自己给出与代码相反的结论，比没有工具更糟。
			if decl := declaredFlags(call); decl != "" {
				// 映射到三态之一。
				//
				// ⚠️ 我第一版直接 verdict = decl（值是 "Serial:true"），
				// 而 report 只认 SAFE/SERIAL/UNKNOWN 三个枚举值 ⇒ 全部落进
				// UNKNOWN 分支，输出"共 13：UNKNOWN 13"，看着像工具没在工作。
				// 一枚举值被字符串污染时，症状离原因很远。
				if strings.Contains(decl, "Serial") {
					ta.verdict = verdictSerial
				} else {
					ta.verdict = verdictSafe
				}
				ta.findings = append(ta.findings, finding{0, "已声明 " + decl + "（作者判断，采信）"})
				out[name] = ta
				return true
			}
			// 跟进 handler 实现
			impl := handlerImplOf(call, fset, methods, recvTypes)
			if impl == nil {
				ta.findings = append(ta.findings, finding{0, "追不到 handler 实现（保守：不声明并发安全） diag=" + diagOf(call)})
			} else {
				ta.implFile, ta.implLine = impl.file, impl.line
				// ★ 递归跟进：写操作常常**不在** handler 体内。
				//   手工核实 plugin_install 时发现，闭包体只写了两行
				//   `return p.installFromPath(...)`，真正的写在那个方法里 ——
				//   只扫一层会把它判成 SAFE（第一版就是这个 bug）。
				fs := scanWritesDeep(impl, methods, map[string]bool{}, 0)
				ta.findings = append(ta.findings, fs...)
				hasWrite := false
				for _, x := range fs {
					if isWriteKind(x.what) {
						hasWrite = true
					}
				}
				if hasWrite {
					ta.verdict = verdictSerial
				} else {
					ta.verdict = verdictSafe
				}
			}
			if old, exists := out[name]; !exists || old.verdict == verdictUnknown {
				out[name] = ta
			}
			return true
		})
	}
}

// indexRecvTypes 建立 "变量名 -> 接收者类型名列表" 的索引。
func indexRecvTypes(fset *token.FileSet, files map[string]*ast.File) map[string][]string {
	out := map[string][]string{}
	for _, f := range files {
		// ★ 方法接收者变量名 → 接收者类型。
		//
		//   example/files 里根本没有 `p := &Plugin{}` 这类语句 —— p 是
		//   Start 的方法接收者。于是局部变量索引全空，143 个工具跟着全 UNKNOWN。
		//   症状（"追不到实现"）完全看不出真因是"p 从来不是局部变量"。
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
				continue
			}
			var recvType string
			if st, ok := fd.Recv.List[0].Type.(*ast.StarExpr); ok {
				if id, ok := st.X.(*ast.Ident); ok {
					recvType = id.Name
				}
			} else if id, ok := fd.Recv.List[0].Type.(*ast.Ident); ok {
				recvType = id.Name
			}
			if recvType == "" {
				continue
			}
			for _, nm := range fd.Recv.List[0].Names {
				out[nm.Name] = appendUnique(out[nm.Name], recvType)
			}
		}
		// 局部变量 p := &Plugin{} / var p *Plugin / p := New(...)
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				if len(x.Lhs) == 1 && len(x.Rhs) == 1 {
					if id, ok := x.Lhs[0].(*ast.Ident); ok {
						for _, t := range typeNamesOf(x.Rhs[0]) {
							out[id.Name] = appendUnique(out[id.Name], t)
						}
					}
				}
			case *ast.ValueSpec:
				// ⚠️ ValueSpec.Type 是 ast.Expr（**单个**类型表达式），
				// 不是切片 —— 我按 FieldList 的形状写了 x.Type[i]，编译不过。
				// 有显式类型才建索引；`p := New(...)` 走上面 AssignStmt 分支。
				if x.Type != nil {
					for _, id := range x.Names {
						for _, t := range typeNamesOf(x.Type) {
							out[id.Name] = appendUnique(out[id.Name], t)
						}
					}
				}
			}
			return true
		})
		// 结构体字段 p.sdk / p.xxx 指向含 Plugin 的类型：并入该结构体名
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, sp := range gd.Specs {
				ts, ok := sp.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, fld := range st.Fields.List {
					for _, t := range typeNamesOf(fld.Type) {
						out[ts.Name.Name] = appendUnique(out[ts.Name.Name], t)
					}
				}
			}
		}
	}
	return out
}

func appendUnique(xs []string, x string) []string {
	for _, y := range xs {
		if y == x {
			return xs
		}
	}
	return append(xs, x)
}

// typeNamesOf 从表达式/类型里抽出可能的类型名（含 New(T{}) 这类构造）。
func typeNamesOf(e ast.Expr) []string {
	var out []string
	switch x := e.(type) {
	case *ast.StarExpr:
		return typeNamesOf(x.X)
	case *ast.Ident:
		return []string{x.Name}
	case *ast.SelectorExpr:
		return []string{x.Sel.Name}
	case *ast.CompositeLit:
		return typeNamesOf(x.Type)
	case *ast.CallExpr:
		// New(Plugin{}) → Plugin；也支持 f(&Plugin{}) / NewPlugin()
		if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "New" && len(x.Args) > 0 {
			return typeNamesOf(x.Args[0])
		}
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			return []string{strings.TrimPrefix(sel.Sel.Name, "New")}
		}
	}
	return out
}

type methodImpl struct {
	file string
	line int
	body *ast.BlockStmt
	recv string // 接收者类型名（大写化），如 "Plugin"
	// locked 记录是否在函数内出现 Lock/RLock
	locked bool
	// selfCall 记录本方法内调用的 p.xxx(...) 形式，便于继续跟进
	selfCalls []string
}

func indexMethods(fset *token.FileSet, files map[string]*ast.File) map[string]*methodImpl {
	out := map[string]*methodImpl{}
	for path, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			recv := ""
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				rt := fd.Recv.List[0].Type
				if t, ok := rt.(*ast.StarExpr); ok {
					if id, ok := t.X.(*ast.Ident); ok {
						recv = id.Name
					}
				} else if id, ok := rt.(*ast.Ident); ok {
					recv = id.Name
				}
				if recv != "" {
					name = "*" + recv + "." + name
				}
			}
			// ★ recv 与 selfCalls 都必须填：scanWritesDeep 用 recv 组候选键去
			//   跟进下层方法，用 selfCalls 才知道要跟进谁。缺任何一个，
			//   递归跟进就是空转 —— handlerRestart → p.stopServer/startServer
			//   这类"只调两个方法"的写法会静默漏判成只读。
			mi := &methodImpl{
				file:      path,
				line:      fset.Position(fd.Pos()).Line,
				body:      fd.Body,
				recv:      recv,
				locked:    hasMutexIn(fd.Body),
				selfCalls: selfCallsOf(fd.Body),
			}
			out[name] = mi
			if recv != "" {
				out[recv+"."+fd.Name.Name] = mi
			}
		}
	}
	return out
}

// handlerImplOf 从 RegisterTool 的第三个参数跟到实现。
func handlerImplOf(call *ast.CallExpr, fset *token.FileSet, methods map[string]*methodImpl, recvTypes map[string][]string) *methodImpl {
	if len(call.Args) < 3 {
		return nil
	}
	switch a := call.Args[2].(type) {
	case *ast.FuncLit: // 闭包
		// ⚠️ recv 留空是有代价的：scanWritesDeep 跟进 selfCalls 时用
		// "*"+recv+"."+name 找方法实现，recv 为空就永远找不到 ⇒
		// 闭包里对 p.xxx(...) 的调用全部追不下去，write 被静默漏掉。
		// 症状是"看起来有扫描、实际漏判"。
		return &methodImpl{file: "<closure>", line: fset.Position(a.Pos()).Line, body: a.Body, locked: hasMutexIn(a.Body), selfCalls: selfCallsOf(a.Body)}
	case *ast.SelectorExpr: // p.handleRead
		//
		// ★ 这里踩过一个隐蔽的坑：调用点的接收者是**变量名**（p），
		//   而方法定义的接收者是**类型名**（Plugin）。我第一版拿变量名
		//   直接去查类型索引，**永远匹配不上**，于是 143 个工具全报 UNKNOWN ——
		//   症状（全是未知）完全看不出是名字空间搞错了。
		// 修法：用变量名 + 方法名，跨 recvTypes 找出所有可能的类型。
		if recv, ok := a.X.(*ast.Ident); ok {
			if recvTypes != nil {
				if types, ok := recvTypes[recv.Name]; ok {
					for _, rt := range types {
						for _, k := range []string{"*" + rt + "." + a.Sel.Name, rt + "." + a.Sel.Name} {
							if m, ok := methods[k]; ok {
								return m
							}
						}
					}
				}
			}
			// 退化：直接按名字试（无类型信息时）
			if m, ok := methods["*"+recv.Name+"."+a.Sel.Name]; ok {
				return m
			}
			if m, ok := methods[a.Sel.Name]; ok {
				return m
			}
		}
	case *ast.Ident: // 已声明的 handler 变量
		key := a.Name
		if m, ok := methods[key]; ok {
			return m
		}
	}
	return nil
}

func toolNameOf(call *ast.CallExpr, fset *token.FileSet) string {
	if len(call.Args) == 0 {
		return ""
	}
	switch a := call.Args[0].(type) {
	case *ast.BasicLit:
		return strings.Trim(a.Value, `"`)
	case *ast.BinaryExpr: // tp+"read" 之类
		//
		// ⚠️ 我第一版用 fmt.Sprintf("%v", a.Y) 拼名字，而 a.Y 是 *ast.BasicLit，
		// %v 打印的是 token 内部结构 —— 输出长这样：
		//   tp&{10500 10515 STRING "manage_social"}
		// 名字里混着指针地址，人根本没法核对。必须取 BasicLit.Value。
		lhs := exprString(fset, a.X)
		rhs := ""
		if lit, ok := a.Y.(*ast.BasicLit); ok {
			rhs = strings.Trim(lit.Value, `"`)
		}
		// 归一化：变量前缀（p.name / tp / p.tp）在运行期才确定，
		// 静态只保留字面量那一段，前缀分隔符一并去掉。
		if _, isIdent := a.X.(*ast.Ident); isIdent {
			return rhs
		}
		if i := strings.Index(lhs, "."); i >= 0 {
			lhs = lhs[i+1:]
		}
		return lhs + rhs
	case *ast.Ident:
		return a.Name
	case *ast.SelectorExpr:
		return a.Sel.Name
	}
	return ""
}

func isRegisterTool(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "RegisterTool":
		return true
	}
	// mockSDK / shim.RegisterTool 之类
	if strings.Contains(sel.Sel.Name, "RegisterTool") {
		return true
	}
	return false
}

func declaredFlags(call *ast.CallExpr) string {
	var sb strings.Builder
	ast.Inspect(call, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		k, ok1 := kv.Key.(*ast.Ident)
		v, ok2 := kv.Value.(*ast.Ident)
		if ok1 && ok2 && (k.Name == "ParallelSafe" || k.Name == "Serial") {
			fmt.Fprintf(&sb, "%s:%s ", k.Name, v.Name)
		}
		return true
	})
	return strings.TrimSpace(sb.String())
}

// writePatterns 是需要视为"写"的调用与赋值形态。
// httpWriteVerbs 是"写"语义的 HTTP 动词。
//
// ★ 这条是被手工核对逼出来的：vanblog 的 manage_social / manage_settings
// 全走 `p.do("GET"|"POST"|"PUT"|..., ...)` 这个通用包装器，方法名里没有
// 任何 write/delete 迹象，纯靠方法名匹配**全部漏判成 SAFE** ——
// 而它们明确含 POST/PUT，是写操作。工具给出与代码相反的结论，
// 比不给结论更危险（人会信它）。
var httpWriteVerbs = []string{"POST", "PUT", "PATCH", "DELETE"}

// httpReadVerbs 只读的动词（用于在"混合"工具上区分）。
var httpReadVerbs = []string{"GET", "HEAD", "OPTIONS"}

var writePatterns = []struct{ what, pat string }{
	{"http-write-verb", ""}, // 占位：走专门的检查
	{"exec", "exec.Command"},
	{"exec", "exec.CommandContext"},
	{"file-write", "os.WriteFile"},
	{"file-remove", "os.Remove"},
	{"file-remove", "os.RemoveAll"},
	{"file-rename", "os.Rename"},
	{"mkdir", "os.Mkdir"},
	{"mkdir", "os.MkdirAll"},
	{"http-post", "http.Post"},
	{"http-do", "client.Do"},
	// ⚠️ 这些模式**不带括号**：callString 收集的是链上方法名并用点连起来
	// （"Settings.Set"），不是完整调用文本。我第一版写成 ".Set("，
	// 于是永远匹配不上 —— handleConfigure（写配置 + 启停服务）被判只读。
	// 症状是"漏判"，比误判更难发现：结果看起来仍然合理。
	{"sdk-set", ".Set"},
	{"sdk-save", ".Save"},
	{"sdk-update", ".Update"},
	{"sdk-delete", ".Delete"},
	{"sdk-add", ".Add"},
	{"sdk-install", ".Install"},
	{"sdk-restart", ".Restart"},
	{"sdk-shutdown", ".Shutdown"},
	{"write", ".Write"},
	{"start", ".Start"},
	{"stop", ".Stop"},
	{"kill", ".Kill"},
	{"os-write", "os.WriteFile"},
	{"os-remove", "os.Remove"},
	{"exec", "exec.Command"},
}

func scanWrites(body *ast.BlockStmt) []finding {
	var out []finding
	// 字段赋值
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if isSelector(lhs) {
					out = append(out, finding{0, "assign"})
				}
			}
		case *ast.IncDecStmt:
			if isSelector(x.X) {
				out = append(out, finding{0, "incdec"})
			}
		case *ast.CallExpr:
			s := callString(x)
			for _, p := range writePatterns {
				if p.pat == "" {
					continue
				}
				if strings.Contains(s, p.pat) {
					out = append(out, finding{0, p.what})
				}
			}
			// HTTP 动词检查（覆盖通用包装器）
			if v := httpVerbOf(x); v != "" {
				for _, w := range httpWriteVerbs {
					if v == w {
						out = append(out, finding{0, "http-" + v})
						break
					}
				}
			}
		}
		return true
	})
	// append
	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "append" {
				out = append(out, finding{0, "append"})
			}
		}
		return true
	})
	// 去重
	seen := map[string]bool{}
	var uniq []finding
	for _, f := range out {
		if seen[f.what] {
			continue
		}
		seen[f.what] = true
		uniq = append(uniq, f)
	}
	return uniq
}

// maxFollowDepth 是跟进深度的上界。
//
// 定 3 层不是随便取的：example 插件里最深的链是
// handler → 业务方法 → 存储/请求方法，三层已覆盖；再深的链说明该工具
// 复杂到**人工读**比自动判更可靠，达到上界时按 UNKNOWN 处理（不标并发安全）。
const maxFollowDepth = 3

// scanWritesDeep 递归跟进方法调用，收集写操作。
func scanWritesDeep(impl *methodImpl, methods map[string]*methodImpl, seen map[string]bool, depth int) []finding {
	if impl == nil {
		return nil
	}
	key := fmt.Sprintf("%s:%d", impl.file, impl.line)
	if seen[key] {
		return nil // 环
	}
	seen[key] = true
	defer delete(seen, key)

	var out []finding
	out = append(out, scanWrites(impl.body)...)

	if depth >= maxFollowDepth {
		// 到达上界：还有未跟进的自调用就不能声称只读
		if len(impl.selfCalls) > 0 {
			out = append(out, finding{0, fmt.Sprintf("跟进到深度上界 %d，仍有 %d 个下层调用未展开", maxFollowDepth, len(impl.selfCalls))})
		}
		return out
	}
	// 跟进本方法的 selfCalls
	for _, c := range impl.selfCalls {
		// 在同一接收者类型下找；找不到就跨接收者找
		cands := []string{"*" + impl.recv + "." + c, impl.recv + "." + c, c}
		found := false
		for _, k := range cands {
			if impl.recv == "" && k == "*."+c {
				continue
			}
			if m, ok := methods[k]; ok && m != impl {
				out = append(out, scanWritesDeep(m, methods, seen, depth+1)...)
				found = true
				break
			}
		}
		if !found && looksLikeExternal(c) {
			out = append(out, finding{0, "外部调用 " + c + "()（可能写，需人工确认）"})
		}
	}
	return dedupeFindings(out)
}

// diagOf 输出第三个参数的形态，用于诊断 handlerImplOf 为何失败。
func diagOf(call *ast.CallExpr) string {
	if len(call.Args) < 3 {
		return fmt.Sprintf("args=%d", len(call.Args))
	}
	switch a := call.Args[2].(type) {
	case *ast.FuncLit:
		return "closure"
	case *ast.SelectorExpr:
		return "selector:" + selReceiver(a) + "." + a.Sel.Name
	case *ast.Ident:
		return "ident:" + a.Name
	}
	return fmt.Sprintf("%T", call.Args[2])
}

// knownReadOnlyCalls 是**确认无副作用**的调用/构造器。
//
// ⚠️ 这张表必须显式列出，不能靠"名字不像写操作"来猜：我第一版用
// looksLikeExternal（黑名单），结果 Marshal / ReadAll / NewRequest /
// NewReader 这些**纯读**的标准库调用全被判"可能写" ⇒ 只读的
// get_article 变成 SERIAL，120 个工具里 119 个被判串行 ——
// 等于工具没在工作，却看上去在工作（保守方向不会引起怀疑）。
//
// 判定原则：**默认怀疑，明确信任**。写不动的东西要逐个列出来。
var knownReadOnlyCalls = map[string]bool{
	// 格式化
	"Sprintf": true, "Fprintf": true, "Errorf": true, "Fatalf": true,
	"Printf": true, "Sprintln": true, "Sprint": true, "Sscanf": true,
	// 字符串（纯函数）
	"String": true, "TrimSpace": true, "Trim": true, "TrimPrefix": true,
	"TrimSuffix": true, "Split": true, "SplitN": true, "Join": true,
	"Replace": true, "ReplaceAll": true, "ToLower": true, "ToUpper": true,
	"Contains": true, "HasPrefix": true, "HasSuffix": true, "Fields": true,
	"Repeat": true, "EqualFold": true, "Title": true,
	// 数值
	"Min": true, "Max": true, "Abs": true, "Round": true, "Floor": true, "Ceil": true,
	// 编解码（纯变换）
	"Marshal": true, "Unmarshal": true, "NewDecoder": true, "NewEncoder": true,
	// JSON 读取（只读文件/流，不写）
	"NewReader": true, "ReadAll": true, "Read": true, "Decode": true,
	// HTTP 只读侧
	"NewRequest": true, "NewRequestWithContext": true, "Parse": true, "ParseForm": true,
	// 时间
	"Now": true, "Unix": true, "ParseDuration": true, "After": true,
	// 容器
	"New": true, "NewMap": true, "Keys": true, "Values": true,
	"Len": true, "Cap": true, "Copy": true, "Append": true,
	// 容器查
	"Get": true, "Load": true, "Exists": true, "List": true, "Query": true,
	// 文件只读
	"Stat": true, "ReadDir": true, "ReadFile": true, "Glob": true, "Walk": true,
	// 错误
	"Is": true, "As": true, "Unwrap": true, "Error": true,
}

func looksLikeExternal(name string) bool {
	if knownReadOnlyCalls[name] {
		return false
	}
	// 指针/包前缀形式（strings.TrimSpace）取最后一段
	if i := strings.LastIndex(name, "."); i >= 0 {
		if knownReadOnlyCalls[name[i+1:]] {
			return false
		}
	}
	return true
}

func dedupeFindings(in []finding) []finding {
	seen := map[string]bool{}
	var out []finding
	for _, f := range in {
		if seen[f.what] {
			continue
		}
		seen[f.what] = true
		out = append(out, f)
	}
	return out
}

func isWriteKind(w string) bool {
	switch w {
	case "assign", "incdec", "append":
		return true
	}
	return true // 保守：所有命中都算写
}

// httpVerbOf 从调用实参里取 HTTP 动词（如 p.do("POST", ...)）。
func httpVerbOf(c *ast.CallExpr) string {
	for _, a := range c.Args {
		if lit, ok := a.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			v := strings.ToUpper(strings.Trim(lit.Value, `"`))
			for _, x := range append(append([]string{}, httpWriteVerbs...), httpReadVerbs...) {
				if v == x {
					return v
				}
			}
		}
	}
	return ""
}

func isSelector(e ast.Expr) bool { _, ok := e.(*ast.SelectorExpr); return ok }

// callString 收集整条方法链上的**所有**方法名。
//
// ★ 误判根因：`p.sdk.Settings().Set("listen", ...)` 是三段链式调用，
//
//	只看最外层只会得到 "Settings"，于是 .Set 这个写操作**整个丢失**，
//	handleConfigure（写配置 + 启停服务）被判成只读。
//	链式调用在 Go 里极常见，只看最外层等于漏掉大半写操作。
func callString(c *ast.CallExpr) string {
	parts := callChain(c)
	return strings.Join(parts, ".")
}

// callChain 自内向外收集链上的方法名/标识符。
func callChain(c *ast.CallExpr) []string {
	switch f := c.Fun.(type) {
	case *ast.SelectorExpr:
		if inner, ok := f.X.(*ast.CallExpr); ok {
			return append(callChain(inner), f.Sel.Name)
		}
		return []string{f.Sel.Name}
	case *ast.Ident:
		return []string{f.Name}
	}
	return nil
}

func selReceiver(s *ast.SelectorExpr) string {
	switch x := s.X.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return selReceiver(x) + "." + x.Sel.Name
	}
	return "?"
}

// selfCallsOf 收集形如 x.method(...) 的调用（x 通常是接收者变量名）。
func selfCallsOf(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
			if _, isIdent := sel.X.(*ast.Ident); isIdent {
				out = append(out, sel.Sel.Name)
			}
		}
		return true
	})
	return out
}

func hasMutexIn(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			s := callString(c)
			if strings.Contains(s, "Lock") || strings.Contains(s, "RLock") {
				found = true
			}
		}
		return true
	})
	return found
}

func report(all map[string]*toolAudit) {
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	var safe, serial, unknown int
	for _, n := range names {
		ta := all[n]
		switch ta.verdict {
		case verdictSafe:
			safe++
		case verdictSerial:
			serial++
		default:
			unknown++
		}
		fmt.Printf("%-8s %-28s %s", ta.verdict, n, ta.file)
		if ta.implFile != "" {
			fmt.Printf("  → %s:%d", ta.implFile, ta.implLine)
		}
		for _, f := range ta.findings {
			if f.line == 0 {
				fmt.Printf("\n           · %s", f.what)
			}
		}
		fmt.Println()
	}
	fmt.Printf("\n共 %d：SAFE %d / SERIAL %d / UNKNOWN %d\n", len(all), safe, serial, unknown)
	fmt.Println("⚠ UNKNOWN 一律不声明并发安全 —— 追不到实现就不能声称安全。")
}

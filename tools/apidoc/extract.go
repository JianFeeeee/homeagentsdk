// Command apidoc 从 SDK 源码提取公开 API 面，输出 JSON 供文档站生成使用。
//
// 设计约束：
//   - **只用标准库**（go/ast、go/parser、go/token）——不需要网络、不依赖
//     golang.org/x/tools，clone 下来就能跑。
//   - **只读源码**，不做 import 解析：它按文件解析 `sdk/*.go`，因此不必处于
//     任何 Go module 内，也不会把依赖带进 SDK 主 module。
//   - 输出是文档站生成的**唯一事实源**：文档里的签名、注释、示例代码块
//     全部来自这里，不手抄，避免文档与源码漂移。
//
// 用法：
//
//	go run ./tools/apidoc -pkgdir ./sdk -out /tmp/api.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Symbol 是一条 API 记录。
type Symbol struct {
	Kind      string   `json:"kind"`      // func / method / type / const / var
	Recv      string   `json:"recv"`      // 方法接收者（仅 method）
	Name      string   `json:"name"`      // 符号名
	Signature string   `json:"signature"` // 一行签名
	Doc       string   `json:"doc"`       // 文档注释（原文，含 markdown）
	DocBrief  string   `json:"doc_brief"` // 首行摘要
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Group     string   `json:"group"` // 归属分组（由 groupFor 决定）
	Exported  bool     `json:"exported"`
	Examples  []string `json:"examples"` // 注释里的 ```go 代码块
	// Deprecated/Since 由注释里的标记提取，供文档打标。
	Deprecated bool `json:"deprecated"`
	// BuiltinOnly 标记「仅内核内置插件可用」——由 tiers.json 注入。
	BuiltinOnly bool `json:"builtin_only"`
	// Tier 是可见级别（public / builtin / bridge）——由 tiers.json 注入。
	// 用显式字段而非从 TierReason 里找关键字判断：中文说明里「桥接」两字
	// 在公开条目的理由里也会出现（“桥接模板注入…外部插件可用”），
	// 靠 strings.Contains 判断会把公开 API 误标成装配点。
	Tier       string `json:"tier"`
	TierReason string `json:"tier_reason"`
}

// Interface 是一个接口类型及其方法。
type Interface struct {
	Name    string   `json:"name"`
	Doc     string   `json:"doc"`
	Methods []Symbol `json:"methods"`
}

// Package 是提取结果。
type Package struct {
	ImportPath string      `json:"import_path"`
	Doc        string      `json:"doc"`
	Symbols    []Symbol    `json:"symbols"`
	Interfaces []Interface `json:"interfaces"`
	// ConstGroups 保留源码里 const(...) 的分组结构。
	ConstGroups []ConstGroup `json:"const_groups"`
	SDKVersion  string       `json:"sdk_version"`
}

type ConstGroup struct {
	Doc    string   `json:"doc"`
	Consts []Symbol `json:"consts"`
}

var (
	codeBlockRe  = regexp.MustCompile("(?s)```(?:go|bash|json|)\n(.*?)```")
	deprecatedRe = regexp.MustCompile(`(?i)\b(deprecated|已废弃|已弃用|即将移除)\b`)
)

func main() {
	pkgdir := flag.String("pkgdir", "./sdk", "要解析的包目录")
	out := flag.String("out", "-", "输出 JSON 路径，- 表示 stdout")
	flag.Parse()

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, *pkgdir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		log.Fatalf("解析 %s 失败: %v", *pkgdir, err)
	}

	var result Package
	for name, pkg := range pkgs {
		result.ImportPath = name
		// doc.New 会归并同名符号、抽取示例，并给出包级文档。
		d := doc.New(pkg, name, doc.AllDecls)
		result.Doc = strings.TrimSpace(d.Doc)

		for _, f := range d.Funcs {
			result.Symbols = append(result.Symbols, makeFunc(fset, f, *pkgdir))
		}
		for _, t := range d.Types {
			result.Symbols = append(result.Symbols, makeType(fset, t, *pkgdir))
			for _, m := range t.Methods {
				result.Symbols = append(result.Symbols, makeMethod(fset, m, t.Name, *pkgdir))
			}
			if iface, ok := t.Decl.Specs[0].(*ast.TypeSpec).Type.(*ast.InterfaceType); ok {
				result.Interfaces = append(result.Interfaces,
					makeInterface(t, iface, fset, *pkgdir))
			}
		}
		// const/var 用 Value 承载，按源码 const 块分组保留。
		for _, v := range d.Consts {
			result.Symbols = append(result.Symbols, makeValue(fset, v, "const", *pkgdir)...)
		}
		for _, v := range d.Vars {
			result.Symbols = append(result.Symbols, makeValue(fset, v, "var", *pkgdir)...)
		}
		result.ConstGroups = groupConsts(fset, pkg, *pkgdir)
	}

	sort.Slice(result.Symbols, func(i, j int) bool {
		if result.Symbols[i].Group != result.Symbols[j].Group {
			return result.Symbols[i].Group < result.Symbols[j].Group
		}
		return result.Symbols[i].Name < result.Symbols[j].Name
	})
	for i := range result.Interfaces {
		sort.Slice(result.Interfaces[i].Methods, func(a, b int) bool {
			return result.Interfaces[i].Methods[a].Name < result.Interfaces[i].Methods[b].Name
		})
	}

	if err := applyTiers(&result); err != nil {
		log.Fatalf("应用能力分层失败: %v", err)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if *out == "-" {
		os.Stdout.Write(data)
		return
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stderr, "提取 %d 个符号 → %s\n", len(result.Symbols), *out)
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func brief(doc string) string {
	for _, line := range strings.Split(doc, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "//") {
			return line
		}
	}
	return ""
}

func examplesOf(doc string) []string {
	var out []string
	for _, m := range codeBlockRe.FindAllStringSubmatch(doc, -1) {
		out = append(out, strings.TrimRight(m[1], "\n"))
	}
	return out
}

func finish(s *Symbol) Symbol {
	s.Doc = strings.TrimSpace(s.Doc)
	s.DocBrief = brief(s.Doc)
	s.Examples = examplesOf(s.Doc)
	s.Deprecated = deprecatedRe.MatchString(s.DocBrief)
	s.Group = groupFor(s)
	return *s
}

func makeFunc(fset *token.FileSet, f *doc.Func, pkgdir string) Symbol {
	pos := fset.Position(f.Decl.Pos())
	return finish(&Symbol{
		Kind:      "func",
		Name:      f.Name,
		Signature: sigOf(fset, f.Decl),
		Doc:       f.Doc,
		File:      rel(pkgdir, pos.Filename),
		Line:      pos.Line,
		Exported:  ast.IsExported(f.Name),
	})
}

func makeMethod(fset *token.FileSet, f *doc.Func, recv, pkgdir string) Symbol {
	pos := fset.Position(f.Decl.Pos())
	return finish(&Symbol{
		Kind:      "method",
		Recv:      recv,
		Name:      f.Name,
		Signature: sigOf(fset, f.Decl),
		Doc:       f.Doc,
		File:      rel(pkgdir, pos.Filename),
		Line:      pos.Line,
		Exported:  ast.IsExported(f.Name),
	})
}

func makeType(fset *token.FileSet, t *doc.Type, pkgdir string) Symbol {
	pos := fset.Position(t.Decl.Pos())
	return finish(&Symbol{
		Kind:      "type",
		Name:      t.Name,
		Signature: "type " + t.Name + " " + typeShape(fset, t),
		Doc:       t.Doc,
		File:      rel(pkgdir, pos.Filename),
		Line:      pos.Line,
		Exported:  ast.IsExported(t.Name),
	})
}

func makeValue(fset *token.FileSet, v *doc.Value, kind, pkgdir string) []Symbol {
	// 一个 const/var 声明里可能有多组名字（如 CapText/CapFile/... 同块），
	// 拆成多条——把名字用逗号拼成一条在文档里很难读，检索也搜不到。
	var out []Symbol
	for _, spec := range v.Decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		docText := strings.TrimSpace(vs.Doc.Text())
		if docText == "" {
			docText = strings.TrimSpace(v.Doc)
		}
		for _, n := range vs.Names {
			if !ast.IsExported(n.Name) {
				continue
			}
			pos := fset.Position(n.Pos())
			out = append(out, finish(&Symbol{
				Kind:      kind,
				Name:      n.Name,
				Signature: kind + " " + n.Name,
				Doc:       docText,
				File:      rel(pkgdir, pos.Filename),
				Line:      pos.Line,
				Exported:  true,
			}))
		}
	}
	return out
}

// makeInterface 从接口类型的 AST 直接读方法。
//
// 注意：不能用 doc.Type.Methods —— 那个字段只收集**具名类型的方法声明**
// （即 `func (x T) M()`），不含接口内嵌的方法列表。接口成员只能在 AST 的
// InterfaceType.Methods 里拿到。
func makeInterface(t *doc.Type, iface *ast.InterfaceType, fset *token.FileSet, pkgdir string) Interface {
	it := Interface{Name: t.Name, Doc: strings.TrimSpace(t.Doc)}
	for _, field := range iface.Methods.List {
		if len(field.Names) == 0 {
			// 内嵌接口（如 interface { io.Closer }）：记为一条说明性条目。
			pos := fset.Position(field.Pos())
			embedded := oneLine(exprString(field.Type))
			it.Methods = append(it.Methods, finish(&Symbol{
				Kind:      "embedded",
				Recv:      t.Name,
				Name:      embedded,
				Signature: embedded,
				Doc:       strings.TrimSpace(field.Doc.Text()),
				File:      rel(pkgdir, pos.Filename),
				Line:      pos.Line,
				Exported:  true,
			}))
			continue
		}
		ft, ok := field.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		pos := fset.Position(field.Pos())
		sig := oneLine(exprString(ft))
		for _, n := range field.Names {
			if !ast.IsExported(n.Name) {
				continue
			}
			full := name(n.Name) + strings.TrimPrefix(sig, "func")
			it.Methods = append(it.Methods, finish(&Symbol{
				Kind:      "method",
				Recv:      t.Name,
				Name:      n.Name,
				Signature: full,
				Doc:       strings.TrimSpace(field.Doc.Text()),
				File:      rel(pkgdir, pos.Filename),
				Line:      pos.Line,
				Exported:  true,
			}))
		}
	}
	_ = iface
	return it
}

// exprString 用 go/printer 把 AST 节点还原成源码文本。
func exprString(n ast.Node) string {
	var buf strings.Builder
	if err := printer.Fprint(&buf, token.NewFileSet(), n); err != nil {
		return "?"
	}
	return buf.String()
}

func name(s string) string { return s }

func sigOf(fset *token.FileSet, fn *ast.FuncDecl) string {
	if fn.Type == nil {
		return fn.Name.Name
	}
	// 用源码原文截取签名，保证与源码逐字一致（不重新格式化）。
	start := fset.Position(fn.Pos()).Offset
	end := fset.Position(fn.Type.End()).Offset
	src, err := os.ReadFile(fset.Position(fn.Pos()).Filename)
	if err == nil && start < end && end <= len(src) {
		return oneLine(string(src[start:end]))
	}
	return fn.Name.Name
}

func typeShape(fset *token.FileSet, t *doc.Type) string {
	spec, ok := t.Decl.Specs[0].(*ast.TypeSpec)
	if !ok {
		return "?"
	}
	start := fset.Position(spec.Type.Pos()).Offset
	end := fset.Position(spec.Type.End()).Offset
	src, err := os.ReadFile(fset.Position(spec.Type.Pos()).Filename)
	if err != nil || start >= end || end > len(src) {
		return "?"
	}
	raw := src[start:end]
	// 结构体只保留第一行 + 字段数提示，完整字段在 API 页单独展开。
	if len(raw) > 160 {
		return oneLine(string(raw[:160])) + " …"
	}
	return oneLine(string(raw))
}

func rel(base, p string) string {
	if r, err := filepath.Rel(base, p); err == nil {
		return r
	}
	return p
}

// groupFor 把符号归到文档站的章节。规则集中在这里，避免散落。
func groupFor(s *Symbol) string {
	switch {
	case s.Recv == "PluginSDK" || strings.HasPrefix(s.Name, "PluginSDK"):
		return "plugin-sdk"
	case s.Recv == "StageContext":
		return "stages"
	case s.Kind == "const" || s.Kind == "var":
		return "constants"
	case s.Recv == "" && s.Kind == "func":
		return "functions"
	case s.Kind == "type":
		return "types"
	case s.Recv != "":
		return "interfaces"
	}
	return "misc"
}

func groupConsts(fset *token.FileSet, pkg *ast.Package, pkgdir string) []ConstGroup {
	var groups []ConstGroup
	for _, f := range pkg.Files {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			g := ConstGroup{}
			if gd.Doc != nil {
				g.Doc = strings.TrimSpace(gd.Doc.Text())
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				var names []string
				for _, n := range vs.Names {
					if ast.IsExported(n.Name) {
						names = append(names, n.Name)
					}
				}
				if len(names) == 0 {
					continue
				}
				pos := fset.Position(vs.Pos())
				g.Consts = append(g.Consts, Symbol{
					Kind:     "const",
					Name:     strings.Join(names, ", "),
					Doc:      strings.TrimSpace(vs.Doc.Text()),
					DocBrief: brief(strings.TrimSpace(vs.Doc.Text())),
					File:     rel(pkgdir, pos.Filename),
					Line:     pos.Line,
					Exported: true,
					Group:    "constants",
				})
			}
			if len(g.Consts) > 0 {
				groups = append(groups, g)
			}
		}
	}
	return groups
}

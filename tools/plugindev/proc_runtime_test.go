package main

import (
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"
)

// 子进程运行时模板的静态检查（Part 3）。
//
// 为什么需要这些测试：模板是插件的运行时半身，它与内核 internal/plugin/proc/
// 的协议名、共享段布局、字段索引必须逐一对齐。任一处漂移都会导致
// 「插件编译通过但运行时读错字段」——比编译错误难查得多。
//
// 模板改为真实 .go 源文件（而非 raw string）的直接收益就是这类检查可行。

func loadProcTemplate(t *testing.T) string {
	t.Helper()
	data, err := procTemplates.ReadFile("templates/proc_main.go.tmpl")
	if err != nil {
		t.Fatalf("读取内嵌模板: %v", err)
	}
	return string(data)
}

// stripComments 去掉源码中的注释（用空白填充以保持偏移），只留可执行代码。
func stripComments(t *testing.T, src string) string {
	t.Helper()
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "proc_main.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("解析模板: %v", err)
	}
	out := []byte(src)
	for _, cg := range f.Comments {
		s := fs.Position(cg.Pos()).Offset
		e := fs.Position(cg.End()).Offset
		for i := s; i < e && i < len(out); i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}
	return string(out)
}

// 模板必须是合法 Go 源码。
func TestProcTemplate_ParsesAsGo(t *testing.T) {
	src := loadProcTemplate(t)
	fs := token.NewFileSet()
	if _, err := parser.ParseFile(fs, "proc_main.go", src, parser.AllErrors); err != nil {
		t.Fatalf("模板不是合法 Go 源码: %v", err)
	}
}

// 模板必须提供 main()，且不得含 cgo 痕迹。
//
// 零 cgo 是迁移的核心收益之一（§3.7 锁仲裁回内核后整个架构无 cgo）；
// 一旦有人往模板里加 import "C"，交叉编译立刻退回需要目标平台 C 工具链。
func TestProcTemplate_HasMainAndNoCgo(t *testing.T) {
	src := loadProcTemplate(t)

	if !strings.Contains(src, "func main()") {
		t.Error("子进程模板必须有 main() 入口")
	}
	// 只检查代码，不检查注释——模板顶部的说明文字本身就提到了 C.CString/C.free
	code := stripComments(t, src)
	for _, forbidden := range []string{
		`import "C"`,
		"//export ",
		"C.CString",
		"C.GoString",
		"C.free",
	} {
		if strings.Contains(code, forbidden) {
			t.Errorf("模板不应含 cgo 痕迹 %q（零 cgo 是迁移的核心收益）", forbidden)
		}
	}
}

// 模板引用的 method 名必须与内核 internal/plugin/proc/protocol.go 一致。
//
// 这里硬编码一份清单做对照：内核侧改了 method 名而模板没跟上时，
// 表现是插件调用返回「未知 method」，测试能提前拦住。
func TestProcTemplate_CoversAllCoreMethods(t *testing.T) {
	src := loadProcTemplate(t)

	// 51 个 C ABI method id 平移后的名字（§3.2），加 stage 锁仲裁 2 个
	required := []string{
		// 注册面
		"tool.register", "stage.register", "output.register", "api.register", "input.register",
		// IO 注入
		"io.injectText", "io.injectInterrupt", "io.injectTextNoMem", "io.injectInputSync",
		"io.setToolBlocks",
		// 生命周期
		"lifecycle.autoRestart",
		// 图记忆
		"memory.recall", "memory.commit", "memory.introspect", "memory.merge", "memory.purge",
		// 文档记忆
		"doc.query", "doc.insert", "doc.remove", "doc.stats",
		// 知识库
		"knowledge.search", "knowledge.add", "knowledge.list",
		// 文本记忆
		"textmemory.append",
		// 设置
		"settings.get", "settings.set", "settings.registerDef",
		"settings.getCore", "settings.setCore", "settings.listCore",
		"settings.getPlugin", "settings.setPlugin", "settings.listPlugin",
		"settings.list", "settings.defs", "settings.dump", "settings.plugins",
		"settings.dataDir",
		// LLM
		"llm.listSources", "llm.setSource", "llm.currentSource",
		// 社交图
		"social.getPerson", "social.getNetwork", "social.getTrait",
		"social.getRelations", "social.listPersons",
		// 插件管理
		"plugin.reloadOne", "plugin.listLoaded", "plugin.isDisabled",
		// 共享段锁仲裁（新增，C ABI 下不存在此概念）
		"stage.lock", "stage.unlock",
	}
	for _, m := range required {
		if !strings.Contains(src, `"`+m+`"`) {
			t.Errorf("模板缺少 core method %q（内核已提供，插件侧未接线）", m)
		}
	}
}

// 模板必须处理内核发来的全部 7 个调用（原 C ABI 的 7 个 //export）。
func TestProcTemplate_HandlesAllKernelCalls(t *testing.T) {
	src := loadProcTemplate(t)
	for _, m := range []string{
		"handshake",
		"plugin.init", "plugin.start", "plugin.stop",
		"tool.invoke", "stage.invoke", "output.invoke",
	} {
		if !strings.Contains(src, `case "`+m+`"`) {
			t.Errorf("模板未处理内核调用 %q", m)
		}
	}
}

// 共享段布局常量必须与内核 internal/plugin/proc/shm.go 一致。
//
// 字段索引错位是最危险的漂移：插件会读到相邻字段的数据，
// 而两边都不报错（同为 []byte）。
func TestProcTemplate_ShmLayoutMatchesKernel(t *testing.T) {
	src := loadProcTemplate(t)

	// 与内核 shm.go 的 offXxx 常量对齐（值比较，不依赖 gofmt 的对齐空白）
	layout := map[string]string{
		"shmOffMagic":     "0",
		"shmOffVersion":   "4",
		"shmOffArenaBase": "8",
		"shmOffArenaCap":  "12",
		"shmOffArenaUsed": "16",
		"shmOffCtxBase":   "20",
		"shmOffSeq":       "24",
		// 与内核 stageFieldCount / sliceSize 对齐
		"shmStageFieldCount": "18",
		"shmSliceSize":       "8",
		"shmVersion":         "1",
	}
	constRe := func(name, want string) bool {
		// gofmt 会对齐常量块，故容许 name 与 = 之间有任意空白
		re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*=\s*` + regexp.QuoteMeta(want) + `\b`)
		return re.MatchString(src)
	}
	for name, want := range layout {
		if !constRe(name, want) {
			t.Errorf("共享段常量 %s 应为 %s（须与内核 internal/plugin/proc/shm.go 一致）", name, want)
		}
	}

	// 字段枚举顺序：内核 stageField 的前若干项
	fieldOrder := []string{
		"fRawMessage = iota", "fUserID", "fGroupID", "fLLMText",
		"fReasoningContent", "fFinalText", "fResponse", "fPhase",
		"fContextMsgs", "fToolCalls", "fToolResults", "fMemory",
		"fTokenUsage", "fErrors",
		"fExtraMediaBlocks", "fExtraMediaType", "fExtraInputSource", "fExtraOutputChannel",
	}
	idx := -1
	for _, f := range fieldOrder {
		at := strings.Index(src, f)
		if at < 0 {
			t.Fatalf("模板缺少字段常量 %s", f)
		}
		if at <= idx {
			t.Errorf("字段常量 %s 的声明顺序与内核 stageField 枚举不一致", f)
		}
		idx = at
	}
}

// stage 处理必须「拿锁 → 读 → handler → 只写脏字段 → 放锁」。
//
// 只写脏字段是消除 lost update 的核心：只读插件零写入，
// 不可能覆盖其他插件的改写（对照 C ABI 副本模型实测 35.8~36.8% 丢失）。
func TestProcTemplate_StageFlowUsesLockAndDirtyWrite(t *testing.T) {
	src := loadProcTemplate(t)

	for _, want := range []string{
		"func handleStageInvoke(",
		"stage.lock",
		"readStageContext()",
		"takeStageSnapshot(",
		"writeStageDirty(",
		"stage.unlock",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("stage 处理链路缺少 %q", want)
		}
	}

	// 顺序检查：加锁必须在读取之前，写回必须在解锁之前
	iLock := strings.Index(src, `callCoreVoid("stage.lock"`)
	iRead := strings.Index(src, "readStageContext()")
	iWrite := strings.Index(src, "writeStageDirty(sc, snap)")
	if iLock < 0 || iRead < 0 || iWrite < 0 {
		t.Fatal("stage 链路关键调用缺失")
	}
	// readStageContext 的定义在前，调用在后；取 handleStageInvoke 内的位置
	stageFn := src[strings.Index(src, "func handleStageInvoke("):]
	iLockFn := strings.Index(stageFn, `callCoreVoid("stage.lock"`)
	iReadFn := strings.Index(stageFn, "readStageContext()")
	iWriteFn := strings.Index(stageFn, "writeStageDirty(sc, snap)")
	if !(iLockFn < iReadFn && iReadFn < iWriteFn) {
		t.Error("stage 链路顺序应为 加锁 → 读取 → 写回")
	}
}

// 快照必须存序列化字符串而非 Go 值。
//
// ❗ 这是修 C ABI 侧 11.3 时踩过的坑：StageContext 的切片字段与读出的值
// 共享底层内容，handler 原地改元素（sc.ToolResults[0].Result = x）时，
// 直接持有 Go 值的快照会跟着变，脏字段计算失效、修复静默失效。
func TestProcTemplate_SnapshotStoresSerializedStrings(t *testing.T) {
	src := loadProcTemplate(t)

	if !strings.Contains(src, "strs        map[int]string") ||
		!strings.Contains(src, "jsons       map[int]string") {
		t.Error("stageSnapshot 必须存序列化字符串（切片共享底层数组，存 Go 值会让脏字段计算失效）")
	}
	if !strings.Contains(src, "json.Marshal(v)") {
		t.Error("takeStageSnapshot 应对容器字段做 json.Marshal")
	}
}

// arena 用尽必须显式报错，不得静默截断（§4.4 风险登记）。
func TestProcTemplate_ArenaExhaustionErrors(t *testing.T) {
	src := loadProcTemplate(t)
	if !strings.Contains(src, "arena 空间不足") {
		t.Error("shmWrite 在 arena 不足时必须报错，不得静默截断")
	}
}

// 日志必须走 stderr：stdout 是 RPC 通道，写日志会破坏 NDJSON 帧。
func TestProcTemplate_LogsToStderr(t *testing.T) {
	src := loadProcTemplate(t)
	if !strings.Contains(src, "log.SetOutput(os.Stderr)") {
		t.Error("日志必须走 stderr，否则会破坏 stdout 的 RPC 帧")
	}
}

// 请求必须在独立 goroutine 里处理。
//
// handler 内会反向调用内核并等应答；若在读循环里同步处理，
// 就没人读应答帧 → 死锁。
func TestProcTemplate_DispatchesRequestsConcurrently(t *testing.T) {
	src := loadProcTemplate(t)
	if !strings.Contains(src, "go handleKernelRequest(&req)") {
		t.Error("请求须在独立 goroutine 处理（handler 内反向调用内核，同步处理会死锁）")
	}
}

// 协议与共享段版本不匹配必须拒绝，不得半兼容运行。
func TestProcTemplate_RejectsVersionMismatch(t *testing.T) {
	src := loadProcTemplate(t)
	for _, want := range []string{"协议版本不匹配", "共享段版本不匹配", "共享段魔数不匹配"} {
		if !strings.Contains(src, want) {
			t.Errorf("握手应校验并拒绝 %q", want)
		}
	}
}

// isProcEntry 只认 plugin.bin。
func TestIsProcEntry(t *testing.T) {
	if !isProcEntry("plugin.bin") {
		t.Error("plugin.bin 应为 proc 模式")
	}
	for _, e := range []string{"plugin.so", "plugin.dll", "plugin.dylib", "main.lua", "", "plugin.exe"} {
		if isProcEntry(e) {
			t.Errorf("%q 不应被判为 proc 模式", e)
		}
	}
}

// proc 模式下各平台产物统一为 plugin.bin（进程边界即 ABI 边界，无平台扩展名）。
func TestResolveBuild_ProcModeUsesBinOnAllPlatforms(t *testing.T) {
	for _, target := range []string{"linux/amd64", "darwin/arm64", "windows/amd64", "freebsd/amd64"} {
		cfg, errMsg := resolveBuild(target, true)
		if cfg == nil {
			t.Fatalf("resolveBuild(%q, proc) 失败: %s", target, errMsg)
		}
		if cfg.entryFile != procEntryFile {
			t.Errorf("%s: proc 模式产物应为 %s，实际 %s", target, procEntryFile, cfg.entryFile)
		}
		if !cfg.proc {
			t.Errorf("%s: proc 标志应为 true", target)
		}
	}
}

// 非 proc 模式行为不变（回归保护：.so 通道必须与改动前一致）。
func TestResolveBuild_CABIModeUnchanged(t *testing.T) {
	cases := map[string]string{
		"linux/amd64":   "plugin.so",
		"darwin/amd64":  "plugin.dylib",
		"freebsd/amd64": "plugin.so",
		"windows/amd64": "plugin.dll",
	}
	for target, want := range cases {
		cfg, errMsg := resolveBuild(target, false)
		if cfg == nil {
			t.Fatalf("resolveBuild(%q) 失败: %s", target, errMsg)
		}
		if cfg.entryFile != want {
			t.Errorf("%s: 应产出 %s，实际 %s", target, want, cfg.entryFile)
		}
		if cfg.proc {
			t.Errorf("%s: 非 proc 模式的 proc 标志应为 false", target)
		}
	}
}

// bundle 模式下 proc 产物在 zip 内按平台加后缀（同名会相互覆盖）。
func TestProcBundleTargets_HavePlatformSuffixedEntries(t *testing.T) {
	seen := map[string]bool{}
	for _, bt := range allProcBundleTargets {
		if seen[bt.entry] {
			t.Errorf("zip 条目名重复: %s（会相互覆盖）", bt.entry)
		}
		seen[bt.entry] = true
		if !strings.HasPrefix(bt.entry, procEntryFile+".") {
			t.Errorf("proc bundle 条目 %q 应以 %s. 为前缀", bt.entry, procEntryFile)
		}
	}
	if len(allProcBundleTargets) != len(allBundleTargets) {
		t.Errorf("proc 与 cabi 的 bundle 平台数应一致：%d vs %d",
			len(allProcBundleTargets), len(allBundleTargets))
	}
}

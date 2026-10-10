package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 截图必须**落盘并返回路径**，而不是把 base64 塞进工具结果。
//
// # 为何这条判据重要（2026-10-10）
//
// 工具结果是 JSON 文本，base64 PNG 会整段进模型上下文，而模型**看不见**
// 这些字符里的图。实测一次视口截图 100-500KB ⇒ base64 后 137-685KB
// （约 3-17 万 token），纯粹是上下文浪费。
//
// 更关键的是它把「解码 + 写文件」这件重复劳动推给了每个消费方，
// 而模型做不到。生产日志实证过 agent 的绕行链路：
//
//	browser_screenshot → cmd_run（自己 base64 解码写盘）→ describe_image
//
// 那一轮跑了 920 秒。内核早有对应约定：describe_image 的 path 参数描述写着
// 「★处理设备/工具回传的媒体时必须传（screensee / camerasue 会回传 file 路径）」。
func TestSaveScreenshotWritesFileAndReturnsPath(t *testing.T) {
	dir := t.TempDir()
	p := &Plugin{name: "browser", screenshotsDir: dir}

	// 造一个最小 PNG 头（内容不重要，测的是落盘行为）
	data := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	path, err := p.saveScreenshot(data, "png", "sess-1")
	if err != nil {
		t.Fatalf("saveScreenshot: %v", err)
	}

	// ① 返回的必须是**绝对路径**且指向真实存在的文件
	if !filepath.IsAbs(path) {
		t.Errorf("返回的应是绝对路径（模型要拿它调 describe_image），实为 %q", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("截图的路径不可读（%s）: %v", path, err)
	}
	if string(got) != string(data) {
		t.Errorf("落盘内容与截图字节不一致：%d vs %d", len(got), len(data))
	}

	// ② 路径必须在 screenshotsDir 内
	if !strings.HasPrefix(path, dir+string(filepath.Separator)) {
		t.Errorf("截图应落在 %s 内，实为 %q", dir, path)
	}

	// ③ 文件名要能看出是截图且带 png 后缀
	if !strings.HasSuffix(path, ".png") {
		t.Errorf("文件名应带 .png 后缀，实为 %q", path)
	}
	if !strings.Contains(filepath.Base(path), "shot-") {
		t.Errorf("文件名应可识别为截图，实为 %q", filepath.Base(path))
	}
}

// 同一会话连续截图不能互相覆盖。
//
// 为何专门测：生产日志里**一轮之内**就调了 3 次 browser_screenshot，
// 而 agent 很可能还想回看前一张。固定文件名会静默覆盖。
func TestSaveScreenshotDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	p := &Plugin{name: "browser", screenshotsDir: dir}

	first, err := p.saveScreenshot([]byte("AAA"), "png", "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	// 确保时间戳至少差 1ms（文件名精度是毫秒）
	time.Sleep(2 * time.Millisecond)
	second, err := p.saveScreenshot([]byte("BBB"), "png", "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("两次截图写到了同一个文件（%s）—— 前一张被覆盖", first)
	}
	b1, _ := os.ReadFile(first)
	if string(b1) != "AAA" {
		t.Errorf("第一次截图被覆盖了：%q", b1)
	}
}

// 会话 id 不能把截图写出目录外（它来自模型，且被拼进路径）。
func TestSaveScreenshotRejectsPathTraversalInSessionID(t *testing.T) {
	dir := t.TempDir()
	p := &Plugin{name: "browser", screenshotsDir: dir}

	for _, evil := range []string{"../../etc/evil", "a/../../b", `win\..\..\x`} {
		path, err := p.saveScreenshot([]byte("X"), "png", evil)
		if err != nil {
			t.Fatalf("saveScreenshot(%q): %v", evil, err)
		}
		// 关键：解析后仍必须在 dir 内
		abs, _ := filepath.Abs(path)
		base, _ := filepath.Abs(dir)
		if !strings.HasPrefix(abs, base+string(filepath.Separator)) {
			t.Errorf("会话 id %q 让截图写到了目录外：%s（应在 %s 内）", evil, abs, base)
		}
	}
}

// 未注入 data_dir 时应降级到临时目录，而不是让截图整条失败。
//
// 理由：截图本身是成功的，只有存储位置该降级。
// 直接报错会把「环境没配」变成「功能不可用」。
func TestSaveScreenshotFallsBackWhenDirUnset(t *testing.T) {
	p := &Plugin{name: "browser"} // screenshotsDir 为空
	path, err := p.saveScreenshot([]byte("X"), "png", "s1")
	if err != nil {
		t.Fatalf("未注入目录时应降级到临时目录，实为报错: %v", err)
	}
	defer os.Remove(path)
	if !filepath.IsAbs(path) {
		t.Errorf("降级路径也应是绝对路径，实为 %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("降级路径的文件不存在: %v", err)
	}
}

// 工具描述必须告诉模型「返回路径 + 拿 path 去 describe_image」。
//
// 为何钉描述而不只钉实现：模型是靠描述决定怎么用工具的。
// 描述若退回「返回 base64」，模型就会又去自己解码。
func TestScreenshotToolDescriptionMentionsPath(t *testing.T) {
	src, err := os.ReadFile("plugin.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i := strings.Index(s, `tp+"screenshot"`)
	if i < 0 {
		t.Fatal("找不到 screenshot 工具注册")
	}
	seg := s[i:]
	if end := strings.Index(seg, "}, p.handleScreenshot)"); end > 0 {
		seg = seg[:end]
	}

	if !strings.Contains(seg, "path") {
		t.Error("工具描述必须提到返回 path（模型靠描述决定怎么用工具）")
	}
	if !strings.Contains(seg, "describe_image") {
		t.Error("工具描述应指明拿 path 去调 describe_image —— " +
			"不写的话模型不知道下一步该做什么")
	}
	if strings.Contains(seg, "返回 base64 编码的 PNG 图片") {
		t.Error("工具描述退回了「返回 base64」，与实现不符（实现已改为落盘 + 返回路径）")
	}
}

// handleScreenshot 的**返回结构**必须是 path 而不是 base64。
//
// # 为何用源码断言而不是调用它
//
// handleScreenshot 需要真实的 chromedp 会话（真 Chromium + CDP），
// 单测跑不动。而「结果里返回什么字段」是**静态可判定**的结构事实，
// 源码断言能可靠守住，回归时立刻红。
//
// 为何需要这条额外判据：saveScreenshot 的单测只覆盖落盘函数本身，
// 把 handleScreenshot 改回「返回 base64」**不会**让它变红
// （变异测试实测过 —— 这是一个真实的判据漏洞）。
func TestHandleScreenshotReturnsPathNotBase64(t *testing.T) {
	src, err := os.ReadFile("plugin.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i := strings.Index(s, "func (p *Plugin) handleScreenshot(")
	if i < 0 {
		t.Fatal("找不到 handleScreenshot")
	}
	seg := s[i:]
	if end := strings.Index(seg, "\nfunc (p *Plugin) saveScreenshot("); end > 0 {
		seg = seg[:end]
	}

	// ① 必须调用 saveScreenshot 并把它返回的路径带进结果
	if !strings.Contains(seg, "p.saveScreenshot(") {
		t.Error("handleScreenshot 必须调用 saveScreenshot 落盘（而不是直接编 base64）")
	}
	if !strings.Contains(seg, `"path":`) {
		t.Error("handleScreenshot 的结果必须含 \"path\" 字段 —— " +
			"模型拿它去调 describe_image / ocr_image")
	}

	// ② base64 只能是**可选**（受 inline 开关控制），不能无条件出现。
	//
	// 注意：这条要挡住的是「无条件把 base64 塞进返回值」，因为那会让
	// 上下文被灌爆——正是本次要修的问题。变异测试发现单看
	// `res["base64"] = b64` 是否存在不够：把 base64 写成 map 字面量的
	// 一个键（`"base64": ...`）同样绕过它，所以两种写法都要查。
	// ⚠️ 这条必须查**真实代码行**而不是整段文本：注释里也出现过 "inline"
	//    和 "base64"，按整段 Contains 判会把注释算成保护开关
	//    （变异测试实测：把 `if ...inline...` 改成 `if false` 仍能通过）。
	code := stripLineComments(seg)
	if strings.Contains(code, `"base64"`) || strings.Contains(code, `data_uri`) {
		if !strings.Contains(code, "args[\"inline\"]") {
			t.Error("返回结构里出现了 base64/data_uri，但没有真实的 inline 开关保护：" +
				"无条件带上就等于把十几万 token 又灌回上下文（本次修复要消掉的问题）")
		}
	}

	// ③ 落盘失败时不得静默退回 base64
	if strings.Contains(code, "saveErr") && !strings.Contains(code, `return errResult("screenshot saved failed`) {
		t.Error("screenshot saved 失败时必须明确报错（return errResult），" +
			"不得静默退回 base64 —— 那会让上下文被灌爆这件事没有迹象")
	}
}

// stripLineComments 去掉整行注释与行尾注释，只留可执行代码。
//
// 为何需要：源码断言若把注释也算进去，就会出现「注释里的词冒充保护逻辑」
// 这种假通过。变异测试实测过这个坑。
func stripLineComments(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") {
			continue
		}
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

package main

import (
	"os"
	"strings"
	"testing"
)

// 视口截图**必须**走 CaptureScreenshot，不能用 Screenshot(sel)。
//
// # 为什么这条判据存在（2026-10-11 实测）
//
// chromedp 的 Screenshot(sel) 内部是 QueryAfter + **NodeVisible**，
// 而 NodeVisible 在本机的 headless Chromium 上**永远不会满足** —— 它先
// 用 dom.GetBoxModel 拿盒子（实测 300µs 就成功），再用 visibleJS 判定
// （实测返回 true，offsetWidth=480），两步都对，但它仍一直等下去。
//
// 同一后端上的完整对照（实测）：
//
//	Do(Query body)        ✓        Do(WaitReady body)     ✓
//	Do(WaitVisible body)  ✗挂住     Do(WaitVisible html)   ✗挂住
//	Run(Screenshot html)  ✗挂住     Run(FullScreenshot)    ✓
//	Run(CaptureScreenshot) ✓
//
// 且**旧版 chromedp v0.9.5 同样挂** —— 这不是升级引入的，是既有缺陷。
// 生产日志佐证：成功的都是 agent 传了 full:true（走 FullScreenshot），
// 超时的都是默认分支（走 Screenshot("body")）。
//
// 所以这条判据守的是：不许把 CaptureScreenshot 改回 Screenshot(sel)。
func TestViewportScreenshotUsesCaptureScreenshot(t *testing.T) {
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
	if j := strings.Index(seg, "\nfunc (p *Plugin) saveScreenshot("); j > 0 {
		seg = seg[:j]
	}

	// ① 视口分支必须用 CaptureScreenshot
	if !strings.Contains(seg, "chromedpCaptureScreenshot()") {
		t.Error("视口截图必须走 chromedpCaptureScreenshot()（=CaptureScreenshot）：\n" +
			"  用 Screenshot(\"body\") 会因 NodeVisible 在 headless 下永不满足而挂到超时")
	}

	// ② 不得再出现挂住的那种写法
	if strings.Contains(seg, `chromedpScreenshot(`) {
		t.Error("handleScreenshot 里出现了 chromedpScreenshot(sel) —— " +
			"该动作在 headless 下必挂（实测），改用 chromedpCaptureScreenshot()")
	}

	// ③ full 分支仍走 FullScreenshot（那条一直是好的）
	if !strings.Contains(seg, "chromedpFullScreenshot(") {
		t.Error("full=true 应走 chromedpFullScreenshot（FullScreenshot 实测正常）")
	}
}

// TestCompatLayerExists 确认兼容层还在（迁移收敛点）。
//
// 这一层是为了让 20+ 处调用点的迁移不淹没 diff。若有人删掉它而没把
// 调用点改对，编译会红；但若有人把它改成直接透传错误 API，就只会
// 在运行时挂住 —— 所以这里钉住关键几个。
func TestCompatLayerExists(t *testing.T) {
	src, err := os.ReadFile("chromedp_compat.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for _, need := range []string{
		"func chromedpDo(",
		"func newRemoteAllocator(",
		"func chromedpCaptureScreenshot(",
		"func chromedpFullScreenshot(",
		"func chromedpTitle(",
	} {
		if !strings.Contains(s, need) {
			t.Errorf("chromedp 兼容层缺少 %s", need)
		}
	}
	// 必须用 remote 子模块的分配器（v0.20 把它移出去了）
	if !strings.Contains(s, "remote.NewAllocator") {
		t.Error("newRemoteAllocator 必须用 remote.NewAllocator（v0.20 移出核心模块）")
	}
}

// TestNoLegacyPointerScreenshot 确认没有残留的旧式「传指针接收」写法。
//
// v0.20 的截图动作是**返回值**，传 &buf 会直接编译失败 —— 但这条判据
// 仍值得留着：它锁的是「值语义」这个契约，而不是某次编译结果。
func TestNoLegacyPointerScreenshot(t *testing.T) {
	for _, f := range []string{"plugin.go", "chromedp_compat.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if strings.Contains(s, "Screenshot(&") || strings.Contains(s, "FullScreenshot(&") {
			t.Errorf("%s 仍用旧式传指针接收截图（v0.20 已改为返回值）", f)
		}
	}
}

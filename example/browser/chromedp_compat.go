package main

import (
	"context"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/remote"
)

// chromedp v0.20 是**泛型重写**（Action[T] 返回值而不填指针），
// 本文件把旧写法收敛成几个小助手，让业务代码的改动面最小。
//
// 为何不直接在每个调用点改：插件里有 20+ 处 `chromedp.Run(ctx, 步骤...)`，
// 逐处改成「Run 取一个值 / Do 跑多步」会让 diff 淹没在机械改写里，
// 真正要审的「行为变了没有」反而看不清。收敛到助手后，业务代码保持原样。

// chromedpDo 依次跑多个不返回值的步骤（旧 Run(ctx, steps...) 的等价物）。
//
// 对应 v0.20 的 chromedp.Do。放在这里是为了让调用点看起来与旧代码一致。
func chromedpDo(ctx context.Context, steps ...chromedp.Action[chromedp.Void]) error {
	return chromedp.Do(ctx, steps...)
}

// newRemoteAllocator 连接一个已在运行的 CDP 端点（如共享浏览器后端）。
//
// v0.20 把它移到了独立子模块：chromedp.NewRemoteAllocator → remote.NewAllocator。
// 子模块存在的理由：核心模块现在只用标准库 + cdproto，需要 websocket 的东西
// 全部搬出去（见 chromedp docs/decisions/2026-10-04-...）。
func newRemoteAllocator(parent context.Context, url string) (context.Context, context.CancelFunc) {
	return remote.NewAllocator(parent, url)
}

// chromedpTitle 取当前页标题。
func chromedpTitle(sel ...chromedp.QueryOption) chromedp.Action[string] {
	return chromedp.Title()
}

// chromedpLocation 取当前页 URL。
func chromedpLocation() chromedp.Action[string] {
	return chromedp.Location()
}

// chromedpOuterHTML 取元素的外部 HTML（旧写法传 &html 接收，现在直接返回）。
func chromedpOuterHTML(sel string, opts ...chromedp.QueryOption) chromedp.Action[string] {
	return chromedp.OuterHTML(sel, opts...)
}

// chromedpScreenshot 截图（旧写法传 &buf 接收，现在直接返回）。
func chromedpScreenshot(sel string, opts ...chromedp.QueryOption) chromedp.Action[[]byte] {
	return chromedp.Screenshot(sel, opts...)
}

// chromedpFullScreenshot 整页截图。
func chromedpFullScreenshot(quality int) chromedp.Action[[]byte] {
	return chromedp.FullScreenshot(quality)
}

// chromedpCaptureScreenshot 截当前**视口**。
//
// ★ 为何不用 chromedp.Screenshot("body")（2026-10-11）：后者内部是
// QueryAfter + NodeVisible，而 NodeVisible 在本机的 headless Chromium 上
// 永远不会满足 —— 实测会一直挂到调用方超时（生产表现为 60s 工具超时）。
// 详见 plugin.go 里 handleScreenshot 的注释（含完整对照表）。
func chromedpCaptureScreenshot() chromedp.Action[[]byte] {
	return chromedp.CaptureScreenshot()
}

// chromedpEvaluate 执行 JS（返回 Void 表示不关心返回值）。
func chromedpEvaluateVoid(expr string) chromedp.Action[chromedp.Void] {
	return chromedp.Evaluate[chromedp.Void](expr, nil)
}

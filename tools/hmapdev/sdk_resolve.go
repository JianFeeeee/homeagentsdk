package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// sdkVersionEntry 是本地 SDK 存储里的一个版本。
//
// Dir 是存储目录名（历史上有 "v0.8.0" 与 "1.2.0" 两种写法都出现过，所以
// 目录名与规范化版本号要分开存），Version 是去掉 v 前缀的 x.y.z。
type sdkVersionEntry struct {
	Dir     string
	Version string
}

// normalizeSDKVersion 去掉常见的前缀写法，得到 x.y.z。
func normalizeSDKVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// parseSDKVersion 解析 x.y.z / x.y（后者补 0）。
func parseSDKVersion(v string) (maj, min, patch int, ok bool) {
	v = normalizeSDKVersion(v)
	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	nums := make([]int, 0, 3)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums = append(nums, n)
	}
	for len(nums) < 3 {
		nums = append(nums, 0)
	}
	return nums[0], nums[1], nums[2], true
}

// compareSDKVersion 比较两个 x.y.z（a<b 返回 -1，相等 0，a>b 返回 1）。
func compareSDKVersion(a, b string) int {
	amaj, amin, apat, aok := parseSDKVersion(a)
	bmaj, bmin, bpat, bok := parseSDKVersion(b)
	if !aok || !bok {
		return strings.Compare(normalizeSDKVersion(a), normalizeSDKVersion(b))
	}
	for _, d := range [][2]int{{amaj, bmaj}, {amin, bmin}, {apat, bpat}} {
		switch {
		case d[0] < d[1]:
			return -1
		case d[0] > d[1]:
			return 1
		}
	}
	return 0
}

// listInstalledSDKs 列出存储里已安装的 SDK，按版本升序。
func listInstalledSDKs() []sdkVersionEntry {
	store := sdkStore()
	entries, err := os.ReadDir(store)
	if err != nil {
		return nil
	}
	var out []sdkVersionEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "current" || strings.HasPrefix(name, ".") {
			continue
		}
		v := normalizeSDKVersion(name)
		if _, _, _, ok := parseSDKVersion(v); !ok {
			continue // 非版本目录（用户放别的东西进去时不误判）
		}
		out = append(out, sdkVersionEntry{Dir: name, Version: v})
	}
	sort.Slice(out, func(i, j int) bool { return compareSDKVersion(out[i].Version, out[j].Version) < 0 })
	return out
}

// ResolveSDKForProject 按项目声明的 SDK 版本（plg.json 的 "sdk" 字段）在本地存储里定位 SDK。
//
// 声明必须是**完整版本号**（x.y.z，如 "1.2.0"）：SDK 的版本纪律是「跟随内核中版本，
// patch 位恒为 .0」（内核的 patch 不碰公开接口 → SDK 不跟版），所以一条内核线
// 只对应一个 SDK 版本号，写 "1.2" 这种区间写法既不必要、又容易让人以为
// 「同一条线里还能挑不同 SDK」。工具链直接拒它，顺便把这条规矩说清楚。
//
// 找不到时必须报**可执行**的错误：列出已装版本 + 可直接粘贴的安装命令 ——
// 只说 "not found" 会让人以为是工具链坏了。
func ResolveSDKForProject(declared string) (dir, version string, err error) {
	declared = normalizeSDKVersion(declared)
	if strings.Count(declared, ".") != 2 {
		return "", "", fmt.Errorf(
			"plg.json 的 sdk 字段 %q 必须是完整版本号（如 \"1.2.0\"）——\n"+
				"      SDK 版本跟随内核中版本、patch 位恒为 .0，一条内核线只有一个 SDK 版本", declared)
	}
	maj, min, pat, ok := parseSDKVersion(declared)
	if !ok {
		return "", "", fmt.Errorf("plg.json 的 sdk 字段 %q 不是合法版本号（写法：\"1.2.0\"）", declared)
	}

	installed := listInstalledSDKs()
	for i := range installed {
		e := installed[i]
		emaj, emin, epat, _ := parseSDKVersion(e.Version)
		if emaj == maj && emin == min && epat == pat {
			return filepath.Join(sdkStore(), e.Dir), e.Version, nil
		}
	}

	// 未命中：给出可执行的下一步
	var have []string
	for _, e := range installed {
		have = append(have, e.Version)
	}
	avail := "（存储里还没有任何 SDK）"
	if len(have) > 0 {
		avail = "已安装：" + strings.Join(have, ", ")
	}
	return "", "", fmt.Errorf(
		"项目声明 sdk=%s，但本地 SDK 存储里没有这个版本；%s\n"+
			"      安装：hmapdev sdk install v%s\n"+
			"      查看：hmapdev sdk list",
		declared, avail, declared)
}

package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type BuildConfig struct {
	OutDir   string
	Targets  []string
	Bundle   bool
	SDKPath  string
	Replaces []string
}

func cmdBuild(args []string) {
	// Read all config from plg.json first
	plg, err := readPlgJSON("plg.json")
	if err != nil {
		fmt.Printf("error: read plg.json: %v\n", err)
		os.Exit(1)
	}

	// Base config from plg.json
	outDir := plg.OutDirDefault()
	targets := plg.TargetList()
	bundle := plg.BundleDefault()
	sdkPath := plg.SDKPath
	var cliReplaces []string

	// CLI flags override plg.json
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--outdir":
			if i+1 < len(args) {
				outDir = args[i+1]
				i++
			}
		case "--target":
			if i+1 < len(args) {
				targets = append(targets, args[i+1])
				i++
			}
		case "--bundle":
			bundle = true
		case "--no-bundle":
			bundle = false
		case "--sdk-path":
			if i+1 < len(args) {
				sdkPath = args[i+1]
				i++
			}
		case "--replace", "-R":
			if i+1 < len(args) {
				cliReplaces = append(cliReplaces, args[i+1])
				i++
			}
		}
	}

	if plg.IsLua() {
		buildTarget(plg, "lua", outDir, "")
		return
	}

	// Ensure go.mod exists with correct SDK path
	sdkModule := ensureGoMod(plg, sdkPath)

	// First build: fetch the SDK module (generates go.sum with zip hash)
	if sdkModule != "" {
		if _, err := os.Stat("go.sum"); os.IsNotExist(err) {
			dl := exec.Command("go", "mod", "download", sdkModule)
			dl.Env = os.Environ()
			dl.Stdout = os.Stdout
			dl.Stderr = os.Stderr
			fmt.Println("  downloading SDK module deps...")
			if err := dl.Run(); err != nil {
				fmt.Printf("  error: go mod download: %v\n", err)
			}
		}
	}

	// Merge plg.json replaces + CLI overrides
	replaceSlice := plg.ReplacesToSlice()
	replaceSlice = append(replaceSlice, cliReplaces...)

	// Apply go.mod replace directives for single-target go build
	gmPatcher := NewGoModPatcher(".", replaceSlice)
	gmRestore, err := gmPatcher.Apply()
	if err != nil {
		fmt.Printf("warn: apply go.mod replaces: %v\n", err)
	}
	defer gmRestore()

	if bundle || len(targets) == 0 {
		buildBundle(plg, outDir, sdkPath)
		return
	}

	for _, t := range targets {
		buildTarget(plg, t, outDir, sdkPath)
	}
}

// allBundleTargets 是 --bundle 模式构建的全部平台。
//
// 子进程模式下各平台产物同名（plugin.bin）——进程边界即 ABI 边界，
// 不存在平台特有扩展名，故 zip 内按平台加后缀区分；
// 内核安装时按当前平台挑对应条目重命名为 plugin.bin。
var allBundleTargets = []struct {
	target string
	entry  string // 二进制在 zip 中的文件名
}{
	{"linux/amd64", "plugin.bin.linux.amd64"},
	{"darwin/amd64", "plugin.bin.darwin.amd64"},
	{"windows/amd64", "plugin.bin.windows.amd64"},
}

func buildBundle(plg *PlgConfig, outDir string, sdkPath string) {
	os.MkdirAll(outDir, 0755)
	buildDir := "build"
	os.MkdirAll(buildDir, 0755)

	runtimeCleanup, err := generateProcRuntime()
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	defer runtimeCleanup()

	thirdpartCleanup := linkThirdpart(plg, "linux/amd64")
	defer thirdpartCleanup()

	var binaries []binEntry

	for _, bt := range allBundleTargets {
		cfg, errMsg := resolveBuild(bt.target)
		if cfg == nil {
			fmt.Printf("  error: %s\n", errMsg)
			return
		}

		// 每平台产物落到独立路径，避免相互覆盖
		outName := fmt.Sprintf("%s_%s_%s", cfg.entryFile, cfg.goos, cfg.goarch)
		outPath := filepath.Join(buildDir, outName)

		// 零 cgo：跨平台交叉编译不需目标平台 C 工具链
		cmd := exec.Command("go", "build", "-trimpath", "-o", outPath)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "GOOS="+cfg.goos, "GOARCH="+cfg.goarch, "CGO_ENABLED=0")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Printf("  compiling %s/%s (子进程模式，CGO_ENABLED=0)...\n", cfg.goos, cfg.goarch)
		if err := cmd.Run(); err != nil {
			fmt.Printf("  error: build %s/%s: %v\n", cfg.goos, cfg.goarch, err)
			return
		}
		binaries = append(binaries, binEntry{src: outPath, zip: bt.entry})
	}

	// Write plugin.json with all platforms declared
	platforms := map[string]bool{}
	for _, bt := range allBundleTargets {
		parts := strings.SplitN(bt.target, "/", 2)
		platforms[parts[0]] = true
	}
	plats := make([]string, 0, len(platforms))
	for p := range platforms {
		plats = append(plats, p)
	}
	writePluginJSON(plg, plats, procEntryFile)

	// package single .hmap with correctly named entries
	hmapPath := filepath.Join(outDir, fmt.Sprintf("%s_bundle.hmap", toSnake(plg.NameEn)))
	createBundleHmap(hmapPath, "plugin.json", binaries)
	fmt.Printf("  packaged %s\n", filepath.Base(hmapPath))
}

// IsLua 判断是否为 Lua 插件（走解释器，不经过 Go 编译）。
//
// 这是 entry 字段唯一仍在使用的用途：Go 插件不再看 entry 值，一律产出 plugin.bin。
func (p *PlgConfig) IsLua() bool { return p.Entry == luaEntryFile }

func readPlgJSON(path string) (*PlgConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var plg PlgConfig
	if err := json.Unmarshal(data, &plg); err != nil {
		return nil, err
	}
	return &plg, nil
}

func parseTargets(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "native" {
		return nil
	}
	var t []string
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			t = append(t, s)
		}
	}
	return t
}

func writePluginJSON(plg *PlgConfig, platforms []string, entry string) {
	m := map[string]interface{}{
		"name":        plg.Name,
		"name_zh":     plg.NameZh,
		"name_en":     plg.NameEn,
		"version":     plg.Version,
		"description": plg.Description,
		"author":      plg.Author,
		"entry":       entry,
	}
	if len(platforms) > 0 {
		m["platforms"] = platforms
	}
	if len(plg.Tags) > 0 {
		m["tags"] = plg.Tags
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile("plugin.json", data, 0644)
}

type buildConfig struct {
	goos      string
	goarch    string
	entryFile string // 一律为 plugin.bin（进程边界即 ABI 边界，无平台特有扩展名）
}

// resolveBuild 解析目标平台。
//
// 全平台统一产出 plugin.bin：子进程模式下不存在 .so/.dylib/.dll 的区分，
// 因为进程边界本身就是 ABI 边界——这正是三套独立 ABI 实现收敛为
// 单一 RPC 实现的直接后果（§9.2：Windows 不再是能力退化的第三套实现）。
func resolveBuild(target string) (*buildConfig, string) {
	if target == "lua" || target == "" {
		return nil, "lua"
	}

	goos, goarch, _ := strings.Cut(target, "/")
	if goos == "" {
		goos = runtime.GOOS
		if goarch == "" {
			goarch = runtime.GOARCH
		}
	}

	switch goos {
	case "linux", "darwin", "freebsd", "windows":
		return &buildConfig{goos: goos, goarch: goarch, entryFile: procEntryFile}, ""
	default:
		return nil, fmt.Sprintf("unsupported OS %q", goos)
	}
}

// ensureGoMod 确保插件项目的 go.mod 包含 SDK 的 replace 指令。
// 如果 go.mod 不存在或已有正确 replace，则跳过。
func ensureGoMod(plg *PlgConfig, sdkPath string) string {
	gomodPath := "go.mod"
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return "" // no go.mod, skip
	}

	lines := strings.Split(string(data), "\n")
	var sdkModule string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		var mod string
		if strings.HasPrefix(line, "require ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				mod = parts[1]
			}
		} else if !strings.HasPrefix(line, "require") &&
			!strings.HasPrefix(line, "module ") &&
			!strings.HasPrefix(line, "go ") &&
			!strings.HasPrefix(line, "replace ") {
			// require 块内行（无前缀）或 import 行
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				mod = parts[0]
			}
		}
		if mod != "" && strings.Contains(mod, "homeagent-sdk") {
			sdkModule = mod
			break
		}
	}
	if sdkModule == "" {
		return ""
	}

	if sdkPath == "" {
		// 仅显式配置（plg.json sdk_path 或 --sdk-path）才写入 replace，
		// 避免 go.mod 中出现本地绝对路径。
		return sdkModule
	}

	absSDK, _ := filepath.Abs(sdkPath)
	absSDK = strings.ReplaceAll(absSDK, "\\", "/")

	// Remove any existing replace line for this module (even if path differs)
	var keep []string
	replaceLine := fmt.Sprintf("replace %s => %s", sdkModule, absSDK)
	alreadyExists := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "replace ") &&
			strings.Contains(line, sdkModule) {
			parts := strings.Fields(line)
			if len(parts) >= 3 && strings.ReplaceAll(parts[2], "\\", "/") == absSDK {
				alreadyExists = true
			}
			continue // strip any existing replace for this module
		}
		keep = append(keep, line)
	}
	if alreadyExists {
		return sdkModule
	}
	keep = append(keep, replaceLine, "")
	if err := os.WriteFile(gomodPath, []byte(strings.Join(keep, "\n")), 0644); err != nil {
		fmt.Printf("  warn: update go.mod replace: %v\n", err)
	}
	return sdkModule
}

func resolveSDKPath(sdkPath string) string {
	if sdkPath != "" {
		abs, _ := filepath.Abs(sdkPath)
		if _, err := os.Stat(filepath.Join(abs, "sdk", "plugin.go")); err == nil {
			return abs
		}
		fmt.Printf("error: --sdk-path %q not a valid SDK\n", sdkPath)
		os.Exit(1)
	}
	// Detect from plugindev's own location (internal dev)
	self, err := os.Executable()
	if err == nil {
		cand := filepath.Dir(filepath.Dir(filepath.Dir(self)))
		if _, err := os.Stat(filepath.Join(cand, "sdk", "plugin.go")); err == nil {
			return cand
		}
	}
	// Active SDK via plugindev sdk use
	store := os.Getenv("HOMEAGENT_SDK_DIR")
	if store == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			store = filepath.Join(home, ".homeagent", "plugindev", "sdk")
		}
	}
	if store != "" {
		if d, err := os.ReadFile(filepath.Join(store, "current")); err == nil {
			ver := strings.TrimSpace(string(d))
			if ver != "" {
				root := filepath.Join(store, ver)
				if _, err := os.Stat(filepath.Join(root, "sdk", "plugin.go")); err == nil {
					return root
				}
			}
		}
	}
	fmt.Printf("error: cannot locate SDK. Use --sdk-path or 'plugindev sdk use'\n")
	os.Exit(1)
	return ""
}

func buildTarget(plg *PlgConfig, target, outDir, sdkPath string) {
	os.MkdirAll(outDir, 0755)

	// Lua: no compilation, package source directly
	if target == "lua" {
		writePluginJSON(plg, nil, "main.lua")
		pkgFiles := []string{"plugin.json", "main.lua"}
		for _, f := range []string{"README.md", "LICENSE"} {
			if _, err := os.Stat(f); err == nil {
				pkgFiles = append(pkgFiles, f)
			}
		}
		// Include thirdpart Lua files
		if entries, err := os.ReadDir("thirdpart"); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".lua") {
					path := filepath.Join("thirdpart", e.Name())
					if _, err := os.Stat(path); err == nil {
						pkgFiles = append(pkgFiles, path)
					}
				}
			}
		}
		hmapName := fmt.Sprintf("%s_lua.hmap", toSnake(plg.NameEn))
		createHmap(filepath.Join(outDir, hmapName), pkgFiles)
		fmt.Printf("  packaged %s\n", hmapName)
		return
	}

	// Resolve build config（全平台统一产出 plugin.bin）
	cfg, errMsg := resolveBuild(target)
	if cfg == nil {
		fmt.Printf("  error: %s\n", errMsg)
		return
	}

	buildDir := "build"
	os.MkdirAll(buildDir, 0755)
	outPath := filepath.Join(buildDir, cfg.entryFile)

	runtimeCleanup, err := generateProcRuntime()
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	defer runtimeCleanup()

	// Auto-link thirdpart/ contents + source_dirs + replace targets
	thirdpartCleanup := linkThirdpart(plg, target)
	defer thirdpartCleanup()

	// Write plugin.json with the correct entry for this target
	writePluginJSON(plg, nil, cfg.entryFile)

	// 普通 go build + 零 cgo：交叉编译不再需要目标平台的 C 工具链
	// （旧路径靠 detectWindowsCC 找 MinGW，现在整个问题消失）。
	cmd := exec.Command("go", "build", "-trimpath", "-o", outPath)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GOOS="+cfg.goos, "GOARCH="+cfg.goarch, "CGO_ENABLED=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("  compiling %s/%s (子进程模式，CGO_ENABLED=0)...\n", cfg.goos, cfg.goarch)
	if err := cmd.Run(); err != nil {
		fmt.Printf("  error: build %s/%s: %v\n", cfg.goos, cfg.goarch, err)
		return
	}

	// package
	pkgFiles := []string{"plugin.json", outPath}
	for _, f := range []string{"README.md", "LICENSE"} {
		if _, err := os.Stat(f); err == nil {
			pkgFiles = append(pkgFiles, f)
		}
	}
	hmapName := fmt.Sprintf("%s_%s_%s.hmap", toSnake(plg.NameEn), cfg.goos, cfg.goarch)
	createHmap(filepath.Join(outDir, hmapName), pkgFiles)
	fmt.Printf("  packaged %s\n", hmapName)
}

type binEntry struct {
	src string // 磁盘路径，如 build/plugin.so
	zip string // zip 中条目名，如 plugin.so
}

// createBundleHmap 创建包含多平台二进制的 bundle .hmap 文件。
// jsonName 是 plugin.json 在 zip 中的条目名；
// binaries 的 src 为磁盘路径，zip 为 zip 中的条目名。
func createBundleHmap(hmapPath, jsonName string, binaries []binEntry) {
	f, err := os.Create(hmapPath)
	if err != nil {
		fmt.Printf("error: create hmap %s: %v\n", hmapPath, err)
		return
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	// Add plugin.json
	writeZipEntry := func(zipName, diskPath string) {
		info, err := os.Stat(diskPath)
		if err != nil {
			return
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return
		}
		hdr.Method = zip.Deflate
		hdr.Name = zipName
		writer, err := w.CreateHeader(hdr)
		if err != nil {
			return
		}
		src, err := os.Open(diskPath)
		if err != nil {
			return
		}
		io.Copy(writer, src)
		src.Close()
	}

	writeZipEntry(jsonName, jsonName)

	// Add each platform binary with the correct zip entry name
	for _, b := range binaries {
		if _, err := os.Stat(b.src); err == nil {
			writeZipEntry(b.zip, b.src)
		}
	}

	// Add optional metadata files
	for _, fname := range []string{"README.md", "LICENSE"} {
		if _, err := os.Stat(fname); err == nil {
			writeZipEntry(fname, fname)
		}
	}
}

func createHmap(hmapPath string, files []string) {
	f, err := os.Create(hmapPath)
	if err != nil {
		fmt.Printf("error: create hmap %s: %v\n", hmapPath, err)
		return
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for _, path := range files {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			continue
		}
		hdr.Method = zip.Deflate
		hdr.Name = filepath.Base(path)
		writer, err := w.CreateHeader(hdr)
		if err != nil {
			continue
		}
		src, err := os.Open(path)
		if err != nil {
			continue
		}
		io.Copy(writer, src)
		src.Close()
	}
}

func toSnake(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "_"))
}

// stripIncludeGuard strips preprocessor guards and C++ comments from a C header,
// since these can confuse cgo's type resolution.
// linkThirdpart scans thirdpart/, source_dirs from plg.json, and replace target dirs
// for source files, generating auto-import stubs. Returns cleanup function.
func linkThirdpart(plg *PlgConfig, target string) func() {
	const importFile = "z_thirdpart.go"
	os.Remove(importFile)

	if target == "lua" {
		return func() {}
	}

	gomodPath := "go.mod"
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return func() {}
	}
	modulePath := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "module ") {
			modulePath = strings.TrimSpace(line[7:])
			break
		}
	}
	if modulePath == "" {
		return func() {}
	}

	// Collect directories to scan: thirdpart/ + source_dirs from plg.json + replace target dirs
	var dirs []string
	if info, err := os.Stat("thirdpart"); err == nil && info.IsDir() {
		dirs = append(dirs, "thirdpart")
	}
	dirs = append(dirs, plg.SourceDirs...)
	for _, r := range plg.ReplacesToSlice() {
		_, to, found := strings.Cut(r, "=")
		if !found {
			continue
		}
		if abs, err := filepath.Abs(to); err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				dirs = append(dirs, abs)
			}
		}
	}

	// Deduplicate
	seen := map[string]bool{}
	var unique []string
	for _, d := range dirs {
		abs, _ := filepath.Abs(d)
		if abs != "" && !seen[abs] {
			seen[abs] = true
			unique = append(unique, d)
		}
	}

	// Generate import stubs for each directory with .go files
	var stubs []string
	for _, d := range unique {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		hasGo := false
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				hasGo = true
				break
			}
		}
		if !hasGo {
			continue
		}

		if !filepath.IsAbs(d) {
			importPath := modulePath + "/" + d
			stubs = append(stubs, importPath)
		} else {
			// External directory: must be in replaces to get a valid import path
			for _, r := range plg.ReplacesToSlice() {
				from, to, found := strings.Cut(r, "=")
				if !found {
					continue
				}
				if absTo, _ := filepath.Abs(to); absTo == d {
					stubs = append(stubs, strings.TrimSpace(from))
					break
				}
			}
		}
	}

	if len(stubs) > 0 {
		var sb strings.Builder
		sb.WriteString("package main\n")
		for _, s := range stubs {
			sb.WriteString("import _ \"" + s + "\"\n")
		}
		os.WriteFile(importFile, []byte(sb.String()), 0644)
	}

	return func() {
		os.Remove(importFile)
	}
}

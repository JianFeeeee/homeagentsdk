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
		fmt.Printf("error: read plg.json: %v\n", err); os.Exit(1)
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
				outDir = args[i+1]; i++
			}
		case "--target":
			if i+1 < len(args) {
				targets = append(targets, args[i+1]); i++
			}
		case "--bundle":
			bundle = true
		case "--no-bundle":
			bundle = false
		case "--sdk-path":
			if i+1 < len(args) {
				sdkPath = args[i+1]; i++
			}
		case "--replace", "-R":
			if i+1 < len(args) {
				cliReplaces = append(cliReplaces, args[i+1]); i++
			}
		}
	}

	if plg.IsLua() {
		buildTarget(plg, "lua", outDir, "")
		return
	}

	// Ensure go.mod exists with correct SDK path
	ensureGoMod(plg, sdkPath)

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
// 每个 OS 只有一个架构（amd64），避免二进制文件名冲突。
var allBundleTargets = []struct {
	target string
	entry  string // 二进制在 zip 中的文件名
}{
	{"linux/amd64", "plugin.so"},
	{"darwin/amd64", "plugin.dylib"},
	{"windows/amd64", "plugin.dll"},
}

func buildBundle(plg *PlgConfig, outDir string, sdkPath string) {
	os.MkdirAll(outDir, 0755)
	buildDir := "build"
	os.MkdirAll(buildDir, 0755)

	// Auto-generate C ABI bridge for non-Windows
	bridgeCleanup := generateBridge("")
	defer bridgeCleanup()
	thirdpartCleanup := linkThirdpart(plg, "linux/amd64")
	defer thirdpartCleanup()

	var binaries []binEntry

	for _, bt := range allBundleTargets {
		cfg, errMsg := resolveBuild(bt.target)
		if cfg == nil {
			fmt.Printf("  error: %s\n", errMsg)
			return
		}

		outPath := filepath.Join(buildDir, cfg.entryFile)

		cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", outPath)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "GOOS="+cfg.goos, "GOARCH="+cfg.goarch, "CGO_ENABLED=1")

		if cfg.goos == "windows" {
			cc := detectWindowsCC()
			if cc != "" {
				cmd.Env = append(cmd.Env, "CC="+cc)
			}
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("  compiling %s/%s (-buildmode=c-shared)...\n", cfg.goos, cfg.goarch)
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
	writePluginJSON(plg, plats, "plugin.so")

	// package single .hmap with correctly named entries
	hmapPath := filepath.Join(outDir, fmt.Sprintf("%s_bundle.hmap", toSnake(plg.NameEn)))
	createBundleHmap(hmapPath, "plugin.json", binaries)
	fmt.Printf("  packaged %s\n", filepath.Base(hmapPath))
}

func (p *PlgConfig) IsLua() bool { return p.Entry == "main.lua" }

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
	entryFile string // "plugin.so" or "plugin.dll"
}

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
	case "linux":
		return &buildConfig{goos: goos, goarch: goarch, entryFile: "plugin.so"}, ""
	case "darwin":
		return &buildConfig{goos: goos, goarch: goarch, entryFile: "plugin.dylib"}, ""
	case "freebsd":
		return &buildConfig{goos: goos, goarch: goarch, entryFile: "plugin.so"}, ""
	case "windows":
		return &buildConfig{goos: goos, goarch: goarch, entryFile: "plugin.dll"}, ""
	default:
		return nil, fmt.Sprintf("unsupported OS %q", goos)
	}
}

// ensureGoMod 确保插件项目的 go.mod 包含 SDK 的 replace 指令。
// 如果 go.mod 不存在或已有正确 replace，则跳过。
func ensureGoMod(plg *PlgConfig, sdkPath string) {
	if sdkPath == "" {
		// 从 plugindev 自身推断 SDK 路径
		self, err := os.Executable()
		if err != nil {
			return
		}
		cand := filepath.Dir(filepath.Dir(filepath.Dir(self)))
		if _, err := os.Stat(filepath.Join(cand, "sdk", "plugin.go")); err != nil {
			return
		}
		sdkPath = cand
	}

	gomodPath := "go.mod"
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return // no go.mod, skip
	}

	lines := strings.Split(string(data), "\n")
	var sdkModule string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require ") || strings.HasPrefix(line, "require (") {
			continue
		}
		if strings.Contains(line, "homeagent-sdk/sdk") || strings.Contains(line, "homeagent-sdk") {
			parts := strings.Fields(line)
			if len(parts) >= 1 && !strings.HasPrefix(parts[0], "//") && !strings.HasPrefix(parts[0], "replace") {
				sdkModule = parts[0]
			}
		}
	}
	if sdkModule == "" {
		return
	}

	// 检查是否已有 replace 指令
	absSDK, _ := filepath.Abs(sdkPath)
	absSDK = strings.ReplaceAll(absSDK, "\\", "/")
	for _, line := range lines {
		if strings.Contains(line, "replace") && strings.Contains(line, sdkModule) {
			parts := strings.Fields(line)
			if len(parts) >= 3 && strings.ReplaceAll(parts[2], "\\", "/") == absSDK {
				return // 已存在且路径正确
			}
		}
	}

	// 追加 replace 指令
	replaceLine := fmt.Sprintf("replace %s => %s", sdkModule, absSDK)
	newData := string(data) + "\n" + replaceLine + "\n"
	if err := os.WriteFile(gomodPath, []byte(newData), 0644); err != nil {
		fmt.Printf("  warn: update go.mod replace: %v\n", err)
	}
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

	// Resolve build config
	cfg, errMsg := resolveBuild(target)
	if cfg == nil {
		fmt.Printf("  error: %s\n", errMsg)
		return
	}

	buildDir := "build"
	os.MkdirAll(buildDir, 0755)
	outPath := filepath.Join(buildDir, cfg.entryFile)

	// Auto-generate C ABI bridge (all platforms use c-shared)
	bridgeCleanup := generateBridge(cfg.goos)
	_ = bridgeCleanup // DISABLED cleanup for debug

	// Auto-link thirdpart/ contents + source_dirs + replace targets
	thirdpartCleanup := linkThirdpart(plg, target)
	defer thirdpartCleanup()

	// Write plugin.json with the correct entry for this target
	writePluginJSON(plg, nil, cfg.entryFile)

	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", outPath)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "GOOS="+cfg.goos, "GOARCH="+cfg.goarch, "CGO_ENABLED=1")

	// Auto-detect MinGW gcc on Windows
	if cfg.goos == "windows" {
		cc := detectWindowsCC()
		if cc != "" {
			cmd.Env = append(cmd.Env, "CC="+cc)
		}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// DEBUG: list files before building
	entries, _ := os.ReadDir(".")
	for _, e := range entries {
		fmt.Printf("  [DEBUG] file: %s\n", e.Name())
	}

	fmt.Printf("  compiling %s/%s (-buildmode=c-shared)...\n", cfg.goos, cfg.goarch)
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

// detectWindowsCC looks for a MinGW-w64 gcc on Windows for c-shared builds.
func detectWindowsCC() string {
	// Check CC from environment first
	if cc := os.Getenv("CC"); cc != "" {
		if _, err := exec.LookPath(cc); err == nil {
			return cc
		}
	}
	// Check common MinGW install paths
	candidates := []string{
		"C:\\mingw64\\bin\\gcc.exe",
		"C:\\MinGW\\bin\\gcc.exe",
		"C:\\msys64\\mingw64\\bin\\gcc.exe",
		"C:\\Users\\21989\\AppData\\Local\\Temp\\mingw64\\mingw64\\bin\\gcc.exe",
	}
	// Also search PATH for gcc
	if path, err := exec.LookPath("gcc"); err == nil {
		return path
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// stripIncludeGuard strips preprocessor guards and C++ comments from a C header,
// since these can confuse cgo's type resolution.
func stripIncludeGuard(header string) string {
	lines := strings.Split(header, "\n")
	var out []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "#ifndef HOMEAGENT_CABI_H" || trimmed == "#define HOMEAGENT_CABI_H" {
			continue
		}
		if trimmed == "#endif" || strings.HasPrefix(trimmed, "#endif") {
			continue
		}
		if trimmed == "#ifdef __cplusplus" || trimmed == "extern \"C\" {" || trimmed == "}" {
			continue
		}
		// Strip C++-style comments (cgo parser may not handle them in /* */ blocks)
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		cleaned := strings.TrimSpace(line)
		if cleaned == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// generateBridge generates the C ABI bridge files for non-Lua builds.
// Returns a cleanup function to remove generated files.
func generateBridge(goos string) func() {
	const bridgeFile = "z_bridge_gen.go"
	const cEntryFile = "z_entry.c"
	os.Remove(bridgeFile)
	os.Remove(cEntryFile)

	var files []string

	if goos == "windows" {
		if err := os.WriteFile(bridgeFile, []byte(tmplBridge), 0644); err != nil {
			fmt.Printf("  error: write bridge: %v\n", err)
			return func() {}
		}
		files = append(files, bridgeFile)
	} else {
		if err := os.WriteFile(bridgeFile, []byte(tmplLinuxBridge), 0644); err != nil {
			fmt.Printf("  error: write bridge: %v\n", err)
			return func() {}
		}
		files = append(files, bridgeFile)
		// Write C entry point file
		if err := os.WriteFile(cEntryFile, []byte(tmplPluginInitC), 0644); err != nil {
			fmt.Printf("  error: write C entry: %v\n", err)
			return func() {}
		}
		files = append(files, cEntryFile)
	}

	return func() {
		for _, f := range files {
			os.Remove(f)
		}
	}
}

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
	for _, to := range plg.Replaces {
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

		// Determine import path: for relative dirs under module, use module path prefix;
		// for absolute paths, derive from replace or use package name
		dirName := filepath.Base(d)
		if !filepath.IsAbs(d) {
			importPath := modulePath + "/" + d
			stubs = append(stubs, importPath)
		} else {
			// External directory: use the replace "from" key if found, else use dir name
			found := false
			for from, to := range plg.Replaces {
				if absTo, _ := filepath.Abs(to); absTo == d {
					stubs = append(stubs, from)
					found = true
					break
				}
			}
			if !found && dirName != "" {
				stubs = append(stubs, modulePath+"/"+dirName)
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

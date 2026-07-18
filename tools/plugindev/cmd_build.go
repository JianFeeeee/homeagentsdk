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
	OutDir  string
	Targets []string // "linux/amd64", "windows/amd64", "lua"
}

func cmdBuild(args []string) {
	cfg := BuildConfig{OutDir: "dist"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--outdir":
			if i+1 < len(args) {
				cfg.OutDir = args[i+1]
				i++
			}
		case "--target":
			if i+1 < len(args) {
				cfg.Targets = append(cfg.Targets, args[i+1])
				i++
			}
		}
	}

	// read plg.json
	plg, err := readPlgJSON("plg.json")
	if err != nil {
		fmt.Printf("error: read plg.json: %v\n", err)
		os.Exit(1)
	}

	// determine targets
	targets := cfg.Targets
	if len(targets) == 0 {
		targets = parseTargets(plg.Targets)
	}
	if len(targets) == 0 {
		targets = []string{"native"}
	}

	// build for each target
	for _, t := range targets {
		buildTarget(plg, t, cfg.OutDir)
	}
}

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

func writePluginJSON(plg *PlgConfig, entry string) {
	m := map[string]interface{}{
		"name":        plg.Name,
		"name_zh":     plg.NameZh,
		"name_en":     plg.NameEn,
		"version":     plg.Version,
		"description": plg.Description,
		"author":      plg.Author,
		"entry":       entry,
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
	case "linux", "darwin", "freebsd", "windows":
		ext := ".so"
		if goos == "windows" {
			ext = ".dll"
		}
		return &buildConfig{
			goos:      goos,
			goarch:    goarch,
			entryFile: "plugin" + ext,
		}, ""
	default:
		return nil, fmt.Sprintf("unsupported OS %q", goos)
	}
}

func buildTarget(plg *PlgConfig, target, outDir string) {
	os.MkdirAll(outDir, 0755)

	// Lua: no compilation, package source directly
	if target == "lua" {
		writePluginJSON(plg, "main.lua")
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

	// Auto-link thirdpart/ contents
	thirdpartCleanup := linkThirdpart(target)
	defer thirdpartCleanup()

	// Write plugin.json with the correct entry for this target
	writePluginJSON(plg, cfg.entryFile)

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

// linkThirdpart scans thirdpart/ for source files and generates auto-import stubs.
// For Go plugins: if thirdpart/*.go exists, generate z_thirdpart.go with import.
// For Lua plugins: no action needed (thirdpart/*.lua is packaged separately in buildTarget).
// Returns cleanup function to remove generated files.
func linkThirdpart(target string) func() {
	const thirdpartDir = "thirdpart"
	const importFile = "z_thirdpart.go"
	os.Remove(importFile)

	if info, err := os.Stat(thirdpartDir); err != nil || !info.IsDir() {
		return func() {}
	}

	entries, err := os.ReadDir(thirdpartDir)
	if err != nil {
		return func() {}
	}

	// For Go builds: check for .go files
	if target != "lua" {
		hasGo := false
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				hasGo = true
				break
			}
		}
		if hasGo {
			// Read go.mod to get the module path
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
			if modulePath != "" {
				importPath := modulePath + "/" + thirdpartDir
				stub := "package main\nimport _ \"" + importPath + "\"\n"
				os.WriteFile(importFile, []byte(stub), 0644)
			}
		}
	}

	return func() {
		os.Remove(importFile)
	}
}

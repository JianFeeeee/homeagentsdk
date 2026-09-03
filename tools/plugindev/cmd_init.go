package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

func (p *PlgConfig) ReplacesToSlice() []string {
	var s []string
	for from, to := range p.Replaces {
		s = append(s, from+"="+to)
	}
	sort.Strings(s) // deterministic order
	return s
}

type PlgConfig struct {
	Name        string            `json:"name"`
	NameZh      string            `json:"name_zh"`
	NameEn      string            `json:"name_en"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	Entry       string            `json:"entry"`
	Tags        []string          `json:"tags"`
	Targets     string            `json:"targets"`
	OutDir      string            `json:"outdir,omitempty"`
	Bundle      *bool             `json:"bundle,omitempty"`
	SDKPath     string            `json:"sdk_path,omitempty"`
	GoVersion   string            `json:"go_version,omitempty"`
	Replaces    map[string]string `json:"replaces,omitempty"`
	SourceDirs  []string          `json:"source_dirs,omitempty"`
}

// TargetList parses the Targets string into a slice.
func (p *PlgConfig) TargetList() []string { return parseTargets(p.Targets) }

// BundleDefault returns true if bundle mode is not explicitly disabled.
func (p *PlgConfig) BundleDefault() bool { return p.Bundle == nil || *p.Bundle }

// OutDirDefault returns the output directory, defaulting to "dist".
func (p *PlgConfig) OutDirDefault() string {
	if p.OutDir != "" {
		return p.OutDir
	}
	return "dist"
}

type TemplateData struct {
	Plg   PlgConfig
	IsLua bool

	// Go module info (for go.mod)
	ModulePath string
	GoVersion  string
	SDKModule  string
	SDKVersion string

	// SDKLocalPath 是本机 SDK 源码绝对路径，写入生成的 go.mod 作为 replace 目标。
	//
	// 为何必须写：gitcode 的模块不在 proxy.golang.org 上，只 require 一个
	// 版本号的 go.mod 配上缺失的 go.sum，新用户第一次 `plugindev build`
	// 必定死在 "missing go.sum entry"，而 `go mod tidy` 又会去公共 proxy 拉
	// 一个不存在的条目。有了本地 replace，go 完全不需要 go.sum 条目。
	SDKLocalPath string
}

func cmdInit(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plugindev init <name> [--lua] [--type remotedevice]")
		os.Exit(1)
	}

	name := args[0]
	isLua := false
	isRemoteDevice := false
	for _, a := range args[1:] {
		switch a {
		case "--lua":
			isLua = true
		case "--type", "-t":
			// handled in next iteration
		}
	}
	// also check --type remotedevice as a single arg
	for i, a := range args[1:] {
		if a == "--type" || a == "-t" {
			if i+1 < len(args[1:]) {
				if args[1:][i+1] == "remotedevice" {
					isRemoteDevice = true
				}
			}
		}
		if a == "--type=remotedevice" || a == "-t=remotedevice" {
			isRemoteDevice = true
		}
	}

	if isRemoteDevice && isLua {
		fmt.Println("error: --type remotedevice and --lua are mutually exclusive")
		os.Exit(1)
	}

	// Remote device projects use different scaffold
	if isRemoteDevice {
		scaffoldRemoteDevice(name)
		return
	}

	dir := name
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		fmt.Printf("error: directory %q already exists\n", dir)
		os.Exit(1)
	}

	// Go 插件统一产出 plugin.bin（v1.0.0 子进程模式）。
	//
	// 此前这里写 "plugin.so"，scaffold 出来的 plg.json 就带着一个已退场的
	// entry 值，新手跟着模板走会误以为自己在做 C ABI 插件。
	// build 实际不看这个值（只用它区分 Lua），但模板不应误导。
	entry := "plugin.bin"
	var targets string
	if isLua {
		entry = "main.lua"
		targets = "lua"
	} else {
		targets = "linux/amd64,windows/amd64"
	}

	nameEn := strings.ReplaceAll(name, "-", " ")
	nameEn = strings.Title(nameEn)

	data := TemplateData{
		Plg: PlgConfig{
			Name:        name,
			NameZh:      "中文名",
			NameEn:      nameEn,
			Version:     "0.1.0",
			Description: name + " plugin",
			Author:      "HomeAgent",
			Entry:       entry,
			Tags:        []string{name},
			Targets:     targets,
		},
		IsLua: isLua,
	}

	// Detect SDK info for Go plugin go.mod.
	// 生成的 go.mod 除 require 外还写一条指向本机 SDK 的 replace：
	// 否则 scaffold 出来的项目第一次 build 必定失败（详见 SDKLocalPath 注释）。
	if !isLua {
		sdkMod, goVer, sdkRoot, sdkVer := detectSDKInfo()
		data.ModulePath = name
		data.GoVersion = goVer
		data.SDKModule = sdkMod
		data.SDKVersion = "v" + sdkVer
		data.SDKLocalPath = strings.ReplaceAll(sdkRoot, "\\", "/")
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("error: create dir: %v\n", err)
		os.Exit(1)
	}

	// write plg.json
	writeTemplate(filepath.Join(dir, "plg.json"), tmplPlgJSON, data)

	// Lua plugins get main.lua + sdk.lua; Go plugins get plugin.go only
	if isLua {
		writeTemplate(filepath.Join(dir, "main.lua"), tmplMainLua, data)
		writeTemplate(filepath.Join(dir, "sdk.lua"), tmplSDKLua, data)
	} else {
		writeTemplate(filepath.Join(dir, "plugin.go"), tmplPluginGo, data)
	}

	// write README.md
	writeTemplate(filepath.Join(dir, "README.md"), tmplReadme, data)

	// write go.mod for Go plugins
	if !isLua {
		writeTemplate(filepath.Join(dir, "go.mod"), tmplGoMod, data)
	}

	// create thirdpart directory for external library sources
	os.MkdirAll(filepath.Join(dir, "thirdpart"), 0755)

	fmt.Printf("Created plugin project %q (%s)\n", dir, entry)
	if isLua {
		fmt.Printf("  cd %s && lua main.lua  (standalone test)\n", dir)
	}
	fmt.Printf("  cd %s && plugindev build\n", dir)
}

// detectSDKInfo reads the HomeAgent SDK's go.mod and meta to get module path, go version, and SDK version.
func detectSDKInfo() (modulePath, goVersion, sdkPath, sdkVersion string) {
	root := activeSDKRoot()
	gomodPath := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		fmt.Printf("error: cannot read SDK go.mod at %s: %v\n", gomodPath, err)
		os.Exit(1)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			modulePath = strings.TrimSpace(line[7:])
		}
		if strings.HasPrefix(line, "go ") {
			goVersion = strings.TrimSpace(line[3:])
		}
	}
	if modulePath == "" {
		fmt.Printf("error: no module directive in %s\n", gomodPath)
		os.Exit(1)
	}
	if goVersion == "" {
		goVersion = "1.21"
	}
	sdkVersion = readMetaVersion(root)
	return modulePath, goVersion, root, sdkVersion
}

// scaffoldRemoteDevice 创建远程设备适配器项目脚手架
func scaffoldRemoteDevice(name string) {
	dir := name
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		fmt.Printf("error: directory %q already exists\n", dir)
		os.Exit(1)
	}

	nameEn := strings.Title(strings.ReplaceAll(name, "-", " "))

	data := TemplateData{
		Plg: PlgConfig{
			Name:        name,
			NameZh:      "中文名",
			NameEn:      nameEn,
			Version:     "0.1.0",
			Description: name + " remote device adapter",
			Author:      "HomeAgent",
			Entry:       name,
			Tags:        []string{name, "remotedevice"},
		},
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Printf("error: create dir: %v\n", err)
		os.Exit(1)
	}

	// 写入 main.c
	writeTemplate(filepath.Join(dir, "main.c"), tmplRemoteDeviceMain, data)

	// 写入 CMakeLists.txt
	writeTemplate(filepath.Join(dir, "CMakeLists.txt"), tmplRemoteDeviceCMake, data)

	// 创建 SDK 目录（symlink/copy）
	sdkSrc := filepath.Join("..", "remotedevice")
	sdkDst := filepath.Join(dir, "ha_remotedevice")
	if _, err := os.Stat(sdkDst); os.IsNotExist(err) {
		// 尝试创建符号链接，失败则提示
		if err := os.Symlink(sdkSrc, sdkDst); err != nil {
			fmt.Printf("  note: could not create symlink to SDK, copy manually:\n")
			fmt.Printf("    cp -r %s %s\n", sdkSrc, sdkDst)
		}
	}

	fmt.Printf("Created remote device adapter project %q\n", dir)
	fmt.Printf("  cd %s && mkdir build && cd build && cmake .. && make\n", dir)
	fmt.Printf("  Or include as subdirectory in your project:\n")
	fmt.Printf("    add_subdirectory(%s)\n", dir)
}

func writeTemplate(path, content string, data TemplateData) {
	tmpl, err := template.New("").Parse(content)
	if err != nil {
		fmt.Printf("error: parse template: %v\n", err)
		os.Exit(1)
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Printf("error: create %s: %v\n", path, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := tmpl.Execute(f, data); err != nil {
		fmt.Printf("error: execute template: %v\n", err)
		os.Exit(1)
	}
}

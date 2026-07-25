package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"gitcode.com/JianFeeeee/homeagent-sdk/meta"
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
	SDKReplace string

	// C ABI
	CABIVersion int
	CABIHeader  string
}

func cmdInit(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: plugindev init <name> [--lua]")
		os.Exit(1)
	}

	name := args[0]
	isLua := false
	for _, a := range args[1:] {
		switch a {
		case "--lua":
			isLua = true
		}
	}

	dir := name
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		fmt.Printf("error: directory %q already exists\n", dir)
		os.Exit(1)
	}

	entry := "plugin.so"
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
		IsLua:       isLua,
		CABIVersion: meta.ABIVersion,
		CABIHeader:  tmplCABIHeader,
	}

	// Detect SDK info for Go plugin go.mod
	if !isLua {
		sdkMod, goVer, sdkPath, sdkVer := detectSDKInfo()
		sdkReplace := sdkPath
		// Make replace path absolute and use forward slashes
		if abs, err := filepath.Abs(sdkPath); err == nil {
			sdkReplace = strings.ReplaceAll(abs, "\\", "/")
		}
		data.ModulePath = name
		data.GoVersion = goVer
		data.SDKModule = sdkMod
		data.SDKVersion = "v" + sdkVer
		data.SDKReplace = sdkReplace
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

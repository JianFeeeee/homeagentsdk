package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

// sdkRoot is the HomeAgent SDK root directory, computed at init time from source location.
var sdkRoot string

const cabiVersion = 1

func init() {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return
	}
	// filename: <sdk_root>/tools/plugindev/cmd_init.go
	sdkRoot = filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

type PlgConfig struct {
	Name        string   `json:"name"`
	NameZh      string   `json:"name_zh"`
	NameEn      string   `json:"name_en"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Entry       string   `json:"entry"`
	Tags        []string `json:"tags"`
	Targets     string   `json:"targets"`
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
		CABIVersion: cabiVersion,
		CABIHeader:  tmplCABIHeader,
	}

	// Detect SDK info for Go plugin go.mod
	if !isLua {
		sdkMod, goVer, sdkPath := detectSDKInfo()
		sdkReplace := sdkPath
		// Make replace path absolute and use forward slashes
		if abs, err := filepath.Abs(sdkPath); err == nil {
			sdkReplace = strings.ReplaceAll(abs, "\\", "/")
		}
		data.ModulePath = name
		data.GoVersion = goVer
		data.SDKModule = sdkMod
		data.SDKVersion = "v0.0.0"
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

// detectSDKInfo reads the HomeAgent SDK's go.mod to get module path and go version.
func detectSDKInfo() (modulePath, goVersion, sdkPath string) {
	if sdkRoot == "" {
		fmt.Printf("error: cannot detect SDK root (built outside SDK tree?)\n")
		os.Exit(1)
	}
	gomodPath := filepath.Join(sdkRoot, "go.mod")
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
	return modulePath, goVersion, sdkRoot
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

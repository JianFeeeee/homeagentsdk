package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		help()
		return
	}
	switch os.Args[1] {
	case "init":
		cmdInit(os.Args[2:])
	case "build":
		cmdBuild(os.Args[2:])
	case "clean":
		cmdClean(os.Args[2:])
	case "debug":
		cmdDebug(os.Args[2:])
	default:
		help()
	}
}

func help() {
	fmt.Println(`HomeAgent Plugin Dev Tool

Usage:
  plugindev init <name>           Scaffold a new plugin project
  plugindev build [flags]         Compile and package plugin
  plugindev clean                 Clean build/dist artifacts
  plugindev debug [dir]           Interpret and debug plugin source

Flags:
  --outdir    Output directory (default: dist)
  --target    Target OS/arch (e.g. linux/amd64), repeatable
  --lua       Create Lua plugin (for init)
`)
}

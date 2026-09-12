package main

import (
	"fmt"
	"os"
)

func cmdClean(args []string) {
	outDir := "dist"
	if len(args) > 0 && args[0] == "--outdir" && len(args) > 1 {
		outDir = args[1]
	}

	dirs := []string{"build", outDir}
	for _, d := range dirs {
		if _, err := os.Stat(d); os.IsNotExist(err) {
			continue
		}
		if err := os.RemoveAll(d); err != nil {
			fmt.Printf("error: remove %s: %v\n", d, err)
		} else {
			fmt.Printf("removed %s/\n", d)
		}
	}

	// also clean generated files
	for _, f := range []string{"plugin.json", "z_bridge_gen.go"} {
		if _, err := os.Stat(f); err == nil {
			os.Remove(f)
			fmt.Printf("removed %s\n", f)
		}
	}
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type GoModPatcher struct {
	dir      string
	replaces []string
	backup   string
}

func NewGoModPatcher(dir string, replaces []string) *GoModPatcher {
	return &GoModPatcher{dir: dir, replaces: replaces}
}

func (p *GoModPatcher) Apply() (func(), error) {
	if len(p.replaces) == 0 {
		return func() {}, nil
	}

	gomodPath := filepath.Join(p.dir, "go.mod")
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return func() {}, fmt.Errorf("read go.mod: %w", err)
	}
	p.backup = string(data)

	var sb strings.Builder
	sb.WriteString(strings.TrimRight(string(data), "\n"))
	sb.WriteString("\n")
	for _, r := range p.replaces {
		from, to, found := strings.Cut(r, "=")
		if !found {
			fmt.Printf("  warn: invalid replace %q, skipping\n", r)
			continue
		}
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		absTo, err := filepath.Abs(to)
		if err != nil {
			fmt.Printf("  warn: resolve path %q: %v, skipping\n", to, err)
			continue
		}
		absTo = strings.ReplaceAll(absTo, "\\", "/")
		sb.WriteString(fmt.Sprintf("replace %s => %s\n", from, absTo))
	}

	if err := os.WriteFile(gomodPath, []byte(sb.String()), 0644); err != nil {
		return func() {}, fmt.Errorf("write go.mod: %w", err)
	}

	return p.restore, nil
}

func (p *GoModPatcher) restore() {
	if p.backup == "" {
		return
	}
	gomodPath := filepath.Join(p.dir, "go.mod")
	os.WriteFile(gomodPath, []byte(p.backup), 0644)
	p.backup = ""
}

func (p *GoModPatcher) ReplaceDirs() []string {
	var dirs []string
	for _, r := range p.replaces {
		_, to, found := strings.Cut(r, "=")
		if found {
			dirs = append(dirs, strings.TrimSpace(to))
		}
	}
	return dirs
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCheckLuaSyntax 钉住 Lua 打包前的语法预检：语法错必须被拒。
// 没有 lua/luac 的构建机跳过（预检按设计降级为警告）。
func TestCheckLuaSyntax(t *testing.T) {
	if _, err := exec.LookPath("luac"); err != nil {
		if _, err := exec.LookPath("lua"); err != nil {
			t.Skip("no lua/luac in PATH")
		}
	}
	dir := t.TempDir()

	good := filepath.Join(dir, "good.lua")
	if err := os.WriteFile(good, []byte("local x = 1\nreturn x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkLuaSyntax(good); err != nil {
		t.Fatalf("valid Lua rejected: %v", err)
	}

	bad := filepath.Join(dir, "bad.lua")
	if err := os.WriteFile(bad, []byte("function broken(\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := checkLuaSyntax(bad); err == nil {
		t.Fatal("invalid Lua accepted; syntax check is not effective")
	}
}

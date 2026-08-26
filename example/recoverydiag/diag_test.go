package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var realCfg = "/home/newqqagent/config.db"
var realLog = "/home/newqqagent/log"

func TestDiagTriage(t *testing.T) {
	p := &Plugin{name: "recoverydiag"}

	cases := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{"signal", map[string]interface{}{"exit_code": 0, "signal": "SIGSEGV"}, "process_death"},
		{"oom", map[string]interface{}{"exit_code": 0, "signal": "SIGKILL", "crash_reason": "oom-kill"}, "process_starvation"},
		{"nonzero", map[string]interface{}{"exit_code": 1}, "process_death"},
		{"healthy", map[string]interface{}{"exit_code": 0}, "normal_stop"},
		{"alive", map[string]interface{}{"still_alive": true, "signal": "SIGKILL"}, "config_unreachable"},
	}
	for _, c := range cases {
		r, _ := p.handleTriage(c.args)
		m, ok := r.(map[string]interface{})
		if !ok {
			t.Fatalf("%s: not a map", c.name)
		}
		if got, _ := m["class"].(string); got != c.want {
			t.Errorf("%s: class = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDiagDB(t *testing.T) {
	if _, err := os.Stat(realCfg); err != nil {
		t.Skip("config.db not present, skipping")
	}
	p := &Plugin{name: "recoverydiag"}
	r, err := p.handleDB(map[string]interface{}{"db_path": realCfg})
	if err != nil {
		t.Fatalf("handleDB: %v", err)
	}
	m := r.(map[string]interface{})
	t.Logf("integrity=%v sources=%v verdict=%v summary=%v", m["integrity"], m["source_count"], m["verdict"], m["summary"])
	if m["integrity"] != "ok" {
		t.Errorf("integrity = %v, want ok", m["integrity"])
	}
	if m["source_count"] == 0 {
		t.Errorf("source_count == 0, expected LLM sources")
	}
	if got, _ := m["source_failed"].(int); got != 0 {
		t.Errorf("source_failed = %d, want 0 (all sources OK): %v", got, m["missing_fields"])
	}
}

func TestDiagLogScan(t *testing.T) {
	if _, err := os.Stat(realLog); err != nil {
		t.Skip("log dir not present, skipping")
	}
	p := &Plugin{name: "recoverydiag"}
	r, err := p.handleLogScan(map[string]interface{}{
		"log_dir":       realLog,
		"since_minutes": 60 * 24 * 3,
	})
	if err != nil {
		t.Fatalf("handleLogScan: %v", err)
	}
	m := r.(map[string]interface{})
	t.Logf("matched=%v counts=%v dominant=%v conclusion=%v", m["lines_matched"], m["counts"], m["dominant"], m["conclusion"])
}

func TestDiagDelta(t *testing.T) {
	base := t.TempDir()
	cur := t.TempDir()
	sub := filepath.Join(base, "sub")
	os.MkdirAll(sub, 0755)

	// modified: same path, different content
	os.WriteFile(filepath.Join(base, "a.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(cur, "a.txt"), []byte("world!"), 0644)
	// created
	os.WriteFile(filepath.Join(cur, "b.txt"), []byte("new"), 0644)
	// deleted
	os.WriteFile(filepath.Join(base, "gone.txt"), []byte("bye"), 0644)
	// unchanged
	os.WriteFile(filepath.Join(base, "same.txt"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(cur, "same.txt"), []byte("x"), 0644)

	p := &Plugin{name: "recoverydiag"}
	r, err := p.handleDelta(map[string]interface{}{"baseline_dir": base, "current_dir": cur})
	if err != nil {
		t.Fatalf("handleDelta: %v", err)
	}
	m := r.(map[string]interface{})
	sum := m["summary"].(map[string]int)
	t.Logf("summary=%v total=%v", sum, m["total_diff"])
	if sum["created"] != 1 || sum["deleted"] != 1 || sum["modified"] != 1 {
		t.Errorf("summary = %v, want modified=1 created=1 deleted=1", sum)
	}
}

func TestDiagLoc(t *testing.T) {
	p := &Plugin{name: "recoverydiag"}
	r, _ := p.handleLoc(map[string]interface{}{
		"triage":   map[string]interface{}{"class": "process_death", "verdict": "down"},
		"db":       map[string]interface{}{"verdict": "ok"},
		"log_scan": map[string]interface{}{"dominant": "panic"},
		"delta":    map[string]interface{}{"summary": map[string]interface{}{"created": 0, "modified": 0, "deleted": 0}},
	})
	m := r.(map[string]interface{})
	// 经 JSON 往返，模拟内核把子结论以 JSON 传给 diag_loc 的真实路径
	raw, _ := json.Marshal(m)
	var dec map[string]interface{}
	json.Unmarshal(raw, &dec)
	hs := dec["ranked_hypotheses"].([]interface{})
	if len(hs) == 0 {
		t.Fatal("no hypotheses")
	}
	top := hs[0].(map[string]interface{})
	t.Logf("top cause=%v conf=%v rec=%v", top["cause"], top["confidence"], top["recommendation"])
	if top["cause"] != "code_panic_loop" {
		t.Errorf("expected code_panic_loop, got %v", top["cause"])
	}
}

func TestDiagLocPersist(t *testing.T) {
	kb := filepath.Join(t.TempDir(), "recovery_kb")
	p := &Plugin{name: "recoverydiag", dataDir: filepath.Dir(kb)}
	args := map[string]interface{}{
		"persist":  true,
		"triage":   map[string]interface{}{"class": "process_death", "verdict": "down"},
		"db":       map[string]interface{}{"verdict": "ok"},
		"log_scan": map[string]interface{}{"dominant": "panic"},
		"delta":    map[string]interface{}{"summary": map[string]interface{}{"created": 0, "modified": 0, "deleted": 0}},
	}
	if _, err := p.handleLoc(args); err != nil {
		t.Fatalf("handleLoc: %v", err)
	}
	entries, err := os.ReadDir(kb)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected persisted diag json, got err=%v entries=%v", err, entries)
	}
	data, _ := os.ReadFile(filepath.Join(kb, entries[0].Name()))
	if !strings.Contains(string(data), `"cause"`) {
		t.Errorf("persisted file missing cause field: %s", data)
	}
}

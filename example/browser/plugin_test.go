package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseBrowserSessionTimeoutRequiresExplicitValue(t *testing.T) {
	_, err := parseBrowserSessionTimeout(map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "timeout is required") {
		t.Fatalf("expected required timeout error, got %v", err)
	}
}

func TestParseBrowserSessionTimeoutAcceptsPositiveDuration(t *testing.T) {
	got, err := parseBrowserSessionTimeout(map[string]interface{}{"timeout": "2h30m"})
	if err != nil {
		t.Fatal(err)
	}
	if got != 2*time.Hour+30*time.Minute {
		t.Fatalf("timeout=%v", got)
	}
}

func TestParseBrowserSessionTimeoutRejectsInvalidOrNonPositive(t *testing.T) {
	for _, value := range []string{"invalid", "0s", "-1m"} {
		if _, err := parseBrowserSessionTimeout(map[string]interface{}{"timeout": value}); err == nil {
			t.Errorf("timeout %q should be rejected", value)
		}
	}
}

func TestBrowserStartReuseResetsExplicitCloseTime(t *testing.T) {
	p := &Plugin{
		name: "browser",
		sessions: map[string]*BrowserSession{
			"browser_1": {
				id:         "browser_1",
				shared:     true,
				sessionKey: "qq",
				createdAt:  time.Now().Add(-time.Hour),
				timeout:    time.Minute,
				currentURL: "https://example.com",
			},
		},
	}

	before := time.Now()
	result, err := p.handleBrowserStart(map[string]interface{}{"source": "qq", "timeout": "3h"})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(map[string]interface{})
	if out["status"] != "reused" || out["timeout"] != "3h0m0s" {
		t.Fatalf("unexpected result: %#v", out)
	}
	s := p.sessions["browser_1"]
	if s.timeout != 3*time.Hour || s.createdAt.Before(before) {
		t.Fatalf("deadline not reset: createdAt=%v timeout=%v", s.createdAt, s.timeout)
	}
}

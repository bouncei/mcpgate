package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValid(t *testing.T) {
	p := writeTemp(t, `
upstream:
  url: "http://localhost:9000/mcp"
  timeout: 15s
listen: ":7070"
keys:
  - label: "a"
    hash: "abc"
    allow: ["read_file"]
    rate_limit: { rps: 5, burst: 10 }
  - label: "b"
    hash: "def"
    allow: ["*"]
defaults:
  rate_limit: { rps: 2, burst: 4 }
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Upstream.URL != "http://localhost:9000/mcp" {
		t.Errorf("upstream url = %q", cfg.Upstream.URL)
	}
	if cfg.Upstream.Timeout.Std() != 15*time.Second {
		t.Errorf("timeout = %v", cfg.Upstream.Timeout.Std())
	}
	if cfg.Listen != ":7070" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	if cfg.Keys[1].RateLimit == nil || cfg.Keys[1].RateLimit.RPS != 2 {
		t.Errorf("default rate limit not applied to key b: %+v", cfg.Keys[1].RateLimit)
	}
}

func TestLoadMissingUpstream(t *testing.T) {
	p := writeTemp(t, `
listen: ":8080"
keys:
  - label: "a"
    hash: "abc"
`)
	if _, err := Load(p); err == nil {
		t.Fatal("expected error for missing upstream url")
	}
}

func TestDefaultsApplied(t *testing.T) {
	p := writeTemp(t, `
upstream:
  url: "http://x/mcp"
keys:
  - label: "a"
    hash: "abc"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("default listen = %q", cfg.Listen)
	}
	if cfg.Upstream.Timeout.Std() != 30*time.Second {
		t.Errorf("default timeout = %v", cfg.Upstream.Timeout.Std())
	}
	if cfg.Audit.Output != "stdout" {
		t.Errorf("default audit output = %q", cfg.Audit.Output)
	}
}

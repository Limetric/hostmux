package config

import (
	"path/filepath"
	"testing"
)

func TestLoadExpandsBareHostsUsingTopLevelDomain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
domain = "example.com."

[[app]]
hosts = ["api", "admin.other.test"]
upstream = "http://127.0.0.1:8080"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
	want := []string{"api.example.com", "admin.other.test"}
	if len(cfg.Apps) != 1 {
		t.Fatalf("apps len = %d", len(cfg.Apps))
	}
	if got := cfg.Apps[0].Hosts; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("hosts = %v, want %v", got, want)
	}
}

func TestLoadExpandsBareHostsUsingDefaultDomain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[[app]]
hosts = ["api"]
upstream = "http://127.0.0.1:8080"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Domain != "localhost" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
	if got := cfg.Apps[0].Hosts; len(got) != 1 || got[0] != "api.localhost" {
		t.Fatalf("hosts = %v", got)
	}
}

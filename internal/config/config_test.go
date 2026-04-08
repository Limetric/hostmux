package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
listen = ":8080"
socket = "/tmp/x.sock"

[[app]]
hosts = ["api.local"]
upstream = "http://127.0.0.1:8080"

[[app]]
hosts = ["a.local", "b.local"]
upstream = "http://127.0.0.1:9000"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Fatalf("listen = %q", cfg.Listen)
	}
	if cfg.Socket != "/tmp/x.sock" {
		t.Fatalf("socket = %q", cfg.Socket)
	}
	if len(cfg.Apps) != 2 {
		t.Fatalf("apps len = %d", len(cfg.Apps))
	}
	if cfg.Apps[1].Hosts[1] != "b.local" {
		t.Fatalf("apps[1].hosts[1] = %q", cfg.Apps[1].Hosts[1])
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, ``)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Fatalf("listen default = %q", cfg.Listen)
	}
}

func TestLoadRejectsAppWithEmptyHosts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[[app]]
hosts = []
upstream = "http://127.0.0.1:8080"
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadRejectsAppWithEmptyUpstream(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[[app]]
hosts = ["a.local"]
upstream = ""
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadMissingFileReturnsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected error")
	}
}

func TestRouterEntries(t *testing.T) {
	cfg := &Config{
		Apps: []App{
			{Hosts: []string{"a.local"}, Upstream: "http://127.0.0.1:1"},
			{Hosts: []string{"b.local", "c.local"}, Upstream: "http://127.0.0.1:2"},
		},
	}
	entries := cfg.RouterEntries()
	if len(entries) != 2 {
		t.Fatalf("len = %d", len(entries))
	}
	if entries[1].Hosts[1] != "c.local" {
		t.Fatalf("entries[1].Hosts[1] = %q", entries[1].Hosts[1])
	}
	if entries[0].Upstream != "http://127.0.0.1:1" {
		t.Fatalf("entries[0].Upstream = %q", entries[0].Upstream)
	}
}

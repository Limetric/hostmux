package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/config"
)

func TestPersistExposedRouteAppendsValidApp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.toml")
	if err := os.WriteFile(path, []byte("domain = \"example.com\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := persistExposedRoute(path, []string{"api.example.com"}, "http://127.0.0.1:3000", nil)
	if err != nil {
		t.Fatalf("persistExposedRoute: %v", err)
	}
	if written != path {
		t.Fatalf("written = %q, want %q", written, path)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("resulting config invalid: %v", err)
	}
	if len(cfg.Apps) != 1 || cfg.Apps[0].Upstream != "http://127.0.0.1:3000" {
		t.Fatalf("apps = %+v", cfg.Apps)
	}
	// Existing content preserved.
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "domain = \"example.com\"") {
		t.Fatalf("existing content lost:\n%s", raw)
	}
}

func TestPersistExposedRouteCreatesFileWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "hostmux.toml")
	if _, err := persistExposedRoute(path, []string{"app.localhost"}, "http://127.0.0.1:8080", map[string]string{"env": "dev"}); err != nil {
		t.Fatalf("persistExposedRoute: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("resulting config invalid: %v", err)
	}
	if len(cfg.Apps) != 1 || cfg.Apps[0].Labels["env"] != "dev" {
		t.Fatalf("apps = %+v", cfg.Apps)
	}
}

func TestPersistExposedRouteRejectsDuplicateAndLeavesFileUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.toml")
	original := "domain = \"example.com\"\n\n[[app]]\nhosts = [\"api.example.com\"]\nupstream = \"http://127.0.0.1:1\"\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := persistExposedRoute(path, []string{"api.example.com"}, "http://127.0.0.1:2", nil)
	if err == nil {
		t.Fatal("expected duplicate-host rejection")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != original {
		t.Fatalf("config was modified despite rejection:\n%s", raw)
	}
	// No stray temp files left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".hostmux-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestPersistExposedRouteFollowsSymlinkAndPreservesIt(t *testing.T) {
	realDir := t.TempDir()
	linkDir := t.TempDir()
	real := filepath.Join(realDir, "hostmux.toml")
	link := filepath.Join(linkDir, "hostmux.toml")
	if err := os.WriteFile(real, []byte("domain = \"example.com\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if _, err := persistExposedRoute(link, []string{"api.example.com"}, "http://127.0.0.1:3000", nil); err != nil {
		t.Fatalf("persistExposedRoute: %v", err)
	}
	// The link must still be a symlink pointing at the real file...
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced by a regular file")
	}
	// ...and the real target must contain the new app.
	cfg, err := config.Load(real)
	if err != nil {
		t.Fatalf("target config invalid: %v", err)
	}
	if len(cfg.Apps) != 1 {
		t.Fatalf("real target not updated: apps = %+v", cfg.Apps)
	}
}

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/config"
)

func TestInitWritesValidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.toml")
	var buf bytes.Buffer
	if err := runInit(initOptions{ConfigPath: path, Domain: "example.com", Listen: ":8443", Writer: &buf}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	// File exists and is valid per config.Check.
	if _, diags := config.Check(path); hasCheckError(diags) {
		t.Fatalf("generated config has errors: %+v", diags)
	}
	// And loads cleanly via the daemon path too.
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Domain != "example.com" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
}

func TestInitRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.toml")
	if err := os.WriteFile(path, []byte("domain = \"old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runInit(initOptions{ConfigPath: path, Domain: "new", Writer: &bytes.Buffer{}}); err == nil {
		t.Fatal("expected error overwriting without --force")
	}
	// Original content preserved.
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "old") {
		t.Fatalf("file was overwritten: %s", b)
	}
}

func TestInitForceOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.toml")
	if err := os.WriteFile(path, []byte("domain = \"old\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runInit(initOptions{ConfigPath: path, Domain: "new.example", Force: true, Writer: &bytes.Buffer{}}); err != nil {
		t.Fatalf("runInit --force: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Domain != "new.example" {
		t.Fatalf("domain = %q", cfg.Domain)
	}
}

func TestInitTunnelPrintsSnippetAndSetsHidePort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.toml")
	var buf bytes.Buffer
	if err := runInit(initOptions{ConfigPath: path, Domain: "example.com", Tunnel: true, Writer: &buf}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "cloudflared ingress snippet") || !strings.Contains(out, `hostname: "*.example.com"`) {
		t.Fatalf("missing tunnel snippet:\n%s", out)
	}
	cfg, _ := config.Load(path)
	if !cfg.HidePort {
		t.Fatal("--tunnel should set hide_port = true")
	}
}

func TestInitLocalhostWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.toml")
	var buf bytes.Buffer
	if err := runInit(initOptions{ConfigPath: path, Domain: "localhost", Listen: ":8443", Writer: &buf}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if !strings.Contains(buf.String(), "443") {
		t.Fatalf("expected localhost/:443 note:\n%s", buf.String())
	}
}

func TestInitRejectsBadListen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.toml")
	if err := runInit(initOptions{ConfigPath: path, Listen: "8443", Writer: &bytes.Buffer{}}); err == nil {
		t.Fatal("expected error for invalid --listen")
	}
}

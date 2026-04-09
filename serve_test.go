package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveServeSocketPath_InvalidConfigReturnsError(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "bad.toml")
	if err := os.WriteFile(bad, []byte("listen = \n[[[not toml\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := resolveServeSocketPath(bad, "")
	if err == nil {
		t.Fatal("expected error from invalid config")
	}
}

func TestResolveServeSocketPath_UsesSocketFromConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	dir := t.TempDir()
	wantSock := filepath.Join(dir, "fromcfg.sock")
	cfgPath := filepath.Join(dir, "hostmux.toml")
	body := fmt.Sprintf("listen = \"127.0.0.1:0\"\nsocket = %q\n", wantSock)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveServeSocketPath(cfgPath, "")
	if err != nil {
		t.Fatalf("resolveServeSocketPath: %v", err)
	}
	if got != wantSock {
		t.Fatalf("got %q want %q", got, wantSock)
	}
}

func TestResolveServeSocketPath_SocketFlagOverridesConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	dir := t.TempDir()
	cfgSock := filepath.Join(dir, "fromcfg.sock")
	flagSock := filepath.Join(dir, "flag.sock")
	cfgPath := filepath.Join(dir, "hostmux.toml")
	body := fmt.Sprintf("listen = \"127.0.0.1:0\"\nsocket = %q\n", cfgSock)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveServeSocketPath(cfgPath, flagSock)
	if err != nil {
		t.Fatalf("resolveServeSocketPath: %v", err)
	}
	if got != flagSock {
		t.Fatalf("got %q want %q", got, flagSock)
	}
}

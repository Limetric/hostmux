package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

func TestRunForegroundDaemonReturnsTLSListenError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	dir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfgPath := filepath.Join(dir, "hostmux.toml")
	sockPath := filepath.Join(dir, "hostmux.sock")
	body := fmt.Sprintf("listen = %q\nsocket = %q\n", ln.Addr().String(), sockPath)
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	err = runForegroundDaemon(startOptions{ConfigPath: cfgPath})
	if err == nil {
		t.Fatal("expected listen error")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("error = %q, want address already in use", err)
	}
}

func TestReadConfigDomain_EmptyPathReturnsEmpty(t *testing.T) {
	got, err := readConfigDomain("")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestReadConfigDomain_MissingFileIsNotAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.toml")
	got, err := readConfigDomain(missing)
	if err != nil {
		t.Fatalf("err = %v, want nil for missing file", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestReadConfigDomain_MalformedTOMLReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(path, []byte("listen = \n[[[not toml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readConfigDomain(path)
	if err == nil {
		t.Fatal("err = nil, want parse error")
	}
	if got != "" {
		t.Fatalf("got %q, want empty on parse error", got)
	}
}

func TestReadConfigDomain_DomainUnsetReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-domain.toml")
	if err := os.WriteFile(path, []byte("listen = \":8443\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readConfigDomain(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty when domain unset (no localhost default leak)", got)
	}
}

func TestReadConfigDomain_NormalizesValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "normalize.toml")
	if err := os.WriteFile(path, []byte(`domain = "  example.com.  "`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readConfigDomain(path)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "example.com" {
		t.Fatalf("got %q, want %q", got, "example.com")
	}
}

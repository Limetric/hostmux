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

func TestShouldWarnLocalhostPort(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		port   int
		want   bool
	}{
		{"localhost on 443", "localhost", 443, false},
		{"localhost on 8443", "localhost", 8443, true},
		{"localhost on 0 (parse failed)", "localhost", 0, false},
		{"example.com on 8443", "example.com", 8443, false},
		{"empty domain on 8443", "", 8443, false},
		{"trailing dot localhost on 8443", "localhost.", 8443, true},
	}
	for _, tc := range cases {
		got := shouldWarnLocalhostPort(tc.domain, tc.port)
		if got != tc.want {
			t.Errorf("%s: shouldWarnLocalhostPort(%q, %d) = %v, want %v", tc.name, tc.domain, tc.port, got, tc.want)
		}
	}
}

func TestExtractListenPort(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{":443", 443, false},
		{":8443", 8443, false},
		{"0.0.0.0:443", 443, false},
		{"127.0.0.1:8443", 8443, false},
		{"[::1]:8443", 8443, false},
		{":0", 0, true},
		{"", 0, true},
		{"junk", 0, true},
		{":notaport", 0, true},
	}
	for _, tc := range cases {
		got, err := extractListenPort(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("extractListenPort(%q) = %d, nil; want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("extractListenPort(%q) error = %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("extractListenPort(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

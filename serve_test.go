package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestPrivilegedPortHint(t *testing.T) {
	cases := []struct {
		name string
		err  error
		port int
		want bool // true = expect a non-empty hint
	}{
		{"EACCES on 443", syscall.EACCES, 443, true},
		{"EACCES on 80", syscall.EACCES, 80, true},
		{"EACCES wrapped", &os.PathError{Op: "listen", Path: ":443", Err: syscall.EACCES}, 443, true},
		{"EACCES on non-privileged", syscall.EACCES, 8443, false},
		{"nil err", nil, 443, false},
		{"unrelated err on 443", errors.New("connection reset"), 443, false},
		{"permission denied string wrap", errors.New("listen tcp :443: permission denied"), 443, true},
	}
	for _, tc := range cases {
		hint := privilegedPortHint(tc.err, tc.port)
		got := hint != ""
		if got != tc.want {
			t.Errorf("%s: privilegedPortHint(...) = %q (present=%v), want present=%v", tc.name, hint, got, tc.want)
		}
	}
}

func TestAdvertisedPort(t *testing.T) {
	cases := []struct {
		name     string
		realPort int
		hide     bool
		want     int
	}{
		{"hide off, 8443 stays 8443", 8443, false, 8443},
		{"hide on, 8443 becomes 0", 8443, true, 0},
		{"hide off, 443 stays 443", 443, false, 443},
		{"hide on, 443 becomes 0", 443, true, 0},
		{"hide off, 0 stays 0", 0, false, 0},
		{"hide on, 0 stays 0", 0, true, 0},
	}
	for _, tc := range cases {
		got := advertisedPort(tc.realPort, tc.hide)
		if got != tc.want {
			t.Errorf("%s: advertisedPort(%d, %v) = %d, want %d", tc.name, tc.realPort, tc.hide, got, tc.want)
		}
	}
}

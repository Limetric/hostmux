package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/tlsconfig"
	"github.com/Limetric/hostmux/internal/trust"
)

// fakeTrustState records trust operations issued via the injected runner so a
// test can assert behavior without touching the real OS trust store.
type fakeTrustState struct {
	trusted bool
	calls   []string
}

// installFakeTrust points baseTrustOptions/currentGOOS at an in-memory trust
// store for the duration of the test.
func installFakeTrust(t *testing.T, goos string) *fakeTrustState {
	t.Helper()
	st := &fakeTrustState{}
	origGOOS := currentGOOS
	origOpts := baseTrustOptions
	currentGOOS = func() string { return goos }
	baseTrustOptions = func() trust.Options {
		return trust.Options{
			GOOS: goos,
			Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
				st.calls = append(st.calls, name+" "+strings.Join(args, " "))
				return nil, nil
			},
		}
	}
	t.Cleanup(func() {
		currentGOOS = origGOOS
		baseTrustOptions = origOpts
	})
	return st
}

// writeTrustConfig writes a config pointing TLS at temp cert/key paths so
// EnsurePair generates real files under the test's temp dir, never the home
// directory.
func writeTrustConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cert := filepath.Join(dir, "hostmux.crt")
	key := filepath.Join(dir, "hostmux.key")
	path := filepath.Join(dir, "hostmux.toml")
	body := "domain = \"example.com\"\n[tls]\ncert = \"" + cert + "\"\nkey = \"" + key + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunTrustInstalls(t *testing.T) {
	st := installFakeTrust(t, "windows") // windows path: addstore, no verify-cert
	cfgPath := writeTrustConfig(t)
	var buf bytes.Buffer
	if err := runTrust(trustOptions{ConfigPath: cfgPath, Writer: &buf}); err != nil {
		t.Fatalf("runTrust: %v", err)
	}
	joined := strings.Join(st.calls, "\n")
	if !strings.Contains(joined, "certutil -addstore -user Root") {
		t.Fatalf("expected addstore call, got:\n%s", joined)
	}
	if !strings.Contains(buf.String(), "trusted") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunUntrustRemoves(t *testing.T) {
	st := installFakeTrust(t, "darwin")
	cfgPath := writeTrustConfig(t)
	var buf bytes.Buffer
	if err := runTrust(trustOptions{ConfigPath: cfgPath, Remove: true, Writer: &buf}); err != nil {
		t.Fatalf("runTrust remove: %v", err)
	}
	if !strings.Contains(strings.Join(st.calls, "\n"), "security remove-trusted-cert") {
		t.Fatalf("expected remove-trusted-cert, got %v", st.calls)
	}
}

func TestRunTrustUnsupportedPlatform(t *testing.T) {
	installFakeTrust(t, "plan9")
	cfgPath := writeTrustConfig(t)
	if err := runTrust(trustOptions{ConfigPath: cfgPath, Writer: &bytes.Buffer{}}); err == nil {
		t.Fatal("expected unsupported-platform error")
	}
}

func TestMaybeAutoTrustOffByDefault(t *testing.T) {
	st := installFakeTrust(t, "windows")
	t.Setenv("HOSTMUX_TLS_AUTO_TRUST", "")
	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, f) }
	maybeAutoTrust(nil, "/tmp/whatever.crt", logf)
	if len(st.calls) != 0 {
		t.Fatalf("auto-trust should be off by default, calls = %v", st.calls)
	}
}

func TestMaybeAutoTrustViaEnv(t *testing.T) {
	st := installFakeTrust(t, "windows")
	cfgPath := writeTrustConfig(t)
	// Generate a real cert to point at.
	tlsCfg, err := resolveServeTLS(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := tlsconfig.EnsurePair(tlsCfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOSTMUX_TLS_AUTO_TRUST", "1")
	maybeAutoTrust(nil, tlsCfg.CertFile, func(string, ...any) {})
	if !strings.Contains(strings.Join(st.calls, "\n"), "certutil -addstore") {
		t.Fatalf("expected auto-trust to install, calls = %v", st.calls)
	}
}

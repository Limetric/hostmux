package tlsconfig

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/config"
)

func TestResolveDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := Resolve(nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Listen != ":8443" {
		t.Fatalf("listen = %q", cfg.Listen)
	}
	if got, want := cfg.CertFile, filepath.Join(home, ".hostmux", "tls", "hostmux.crt"); got != want {
		t.Fatalf("cert = %q, want %q", got, want)
	}
	if got, want := cfg.KeyFile, filepath.Join(home, ".hostmux", "tls", "hostmux.key"); got != want {
		t.Fatalf("key = %q, want %q", got, want)
	}
}

func TestResolveHonorsOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfg, err := Resolve(&config.TLSBlock{
		Listen: ":9443",
		Cert:   "~/certs/dev.crt",
		Key:    "~/certs/dev.key",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if cfg.Listen != ":9443" {
		t.Fatalf("listen = %q", cfg.Listen)
	}
	if !strings.HasSuffix(cfg.CertFile, filepath.Join("certs", "dev.crt")) {
		t.Fatalf("cert = %q", cfg.CertFile)
	}
	if !strings.HasSuffix(cfg.KeyFile, filepath.Join("certs", "dev.key")) {
		t.Fatalf("key = %q", cfg.KeyFile)
	}
}

func TestEnsurePairCreatesFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Listen:   ":8443",
		CertFile: filepath.Join(dir, "tls", "hostmux.crt"),
		KeyFile:  filepath.Join(dir, "tls", "hostmux.key"),
	}

	if err := EnsurePair(cfg); err != nil {
		t.Fatalf("EnsurePair: %v", err)
	}
	if _, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile); err != nil {
		t.Fatalf("LoadX509KeyPair: %v", err)
	}
	keyInfo, err := os.Stat(cfg.KeyFile)
	if err != nil {
		t.Fatalf("Stat key: %v", err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %o", keyInfo.Mode().Perm())
	}
}

func TestEnsurePairReusesExistingPair(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Listen:   ":8443",
		CertFile: filepath.Join(dir, "tls", "hostmux.crt"),
		KeyFile:  filepath.Join(dir, "tls", "hostmux.key"),
	}

	if err := EnsurePair(cfg); err != nil {
		t.Fatalf("EnsurePair first: %v", err)
	}
	before, err := os.ReadFile(cfg.CertFile)
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}
	if err := EnsurePair(cfg); err != nil {
		t.Fatalf("EnsurePair second: %v", err)
	}
	after, err := os.ReadFile(cfg.CertFile)
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("cert changed on reuse")
	}
}

func TestEnsurePairRejectsPartialState(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Listen:   ":8443",
		CertFile: filepath.Join(dir, "tls", "hostmux.crt"),
		KeyFile:  filepath.Join(dir, "tls", "hostmux.key"),
	}
	if err := os.MkdirAll(filepath.Dir(cfg.CertFile), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(cfg.CertFile, []byte("cert"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := EnsurePair(cfg); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnsurePairRejectsInvalidExistingPair(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Listen:   ":8443",
		CertFile: filepath.Join(dir, "tls", "hostmux.crt"),
		KeyFile:  filepath.Join(dir, "tls", "hostmux.key"),
	}
	if err := os.MkdirAll(filepath.Dir(cfg.CertFile), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(cfg.CertFile, []byte("bad cert"), 0o644); err != nil {
		t.Fatalf("Write cert: %v", err)
	}
	if err := os.WriteFile(cfg.KeyFile, []byte("bad key"), 0o600); err != nil {
		t.Fatalf("Write key: %v", err)
	}

	if err := EnsurePair(cfg); err == nil {
		t.Fatal("expected error")
	}
}

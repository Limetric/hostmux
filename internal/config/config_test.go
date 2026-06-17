package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if cfg.Listen != ":8443" {
		t.Fatalf("listen default = %q", cfg.Listen)
	}
	if cfg.Domain != "localhost" {
		t.Fatalf("domain default = %q", cfg.Domain)
	}
}

func TestLoadTLSDefaultsListenWhenBlockPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[tls]
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TLS == nil {
		t.Fatal("expected tls block")
	}
	if cfg.TLS.Listen != ":8443" {
		t.Fatalf("tls.listen default = %q", cfg.TLS.Listen)
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

func TestLoadRejectsAppWithInvalidUpstream(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[[app]]
hosts = ["a.local"]
upstream = "127.0.0.1:8080"
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadRejectsAppWithWhitespaceUpstream(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[[app]]
hosts = ["a.local"]
upstream = "  http://127.0.0.1:8080/  "
`)
	if _, err := Load(path); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestLoadAcceptsUppercaseUpstreamScheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[[app]]
hosts = ["a.local"]
upstream = "HTTP://127.0.0.1:8080"
`)
	if _, err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoadRejectsAppWithInvalidHost(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[[app]]
hosts = ["a..b.local"]
upstream = "http://127.0.0.1:8080"
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

func TestLoadHidePort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
hide_port = true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.HidePort {
		t.Fatalf("hide_port = false, want true")
	}
}

func TestLoadHidePortDefaultsFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, ``)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HidePort {
		t.Fatalf("hide_port = true, want false (default)")
	}
}

func TestRouterEntriesCarryLabels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	content := `domain = "example.com"

[[app]]
hosts = ["api"]
upstream = "http://127.0.0.1:8080"
labels = { team = "web", kind = "api" }
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	entries := cfg.RouterEntries()
	if len(entries) != 1 {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].Labels["team"] != "web" || entries[0].Labels["kind"] != "api" {
		t.Fatalf("labels = %v", entries[0].Labels)
	}
}

func TestProxyBlockParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, `
[proxy]
read_header_timeout = "10s"
idle_timeout = "2m"
response_header_timeout = "30s"
dial_timeout = "5s"
max_header_bytes = 1048576
upstream_insecure_skip_verify = true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Proxy
	if p == nil {
		t.Fatal("proxy block nil")
	}
	if p.ReadHeaderTimeout.AsDuration() != 10*time.Second {
		t.Fatalf("read_header_timeout = %v", p.ReadHeaderTimeout.AsDuration())
	}
	if p.IdleTimeout.AsDuration() != 2*time.Minute {
		t.Fatalf("idle_timeout = %v", p.IdleTimeout.AsDuration())
	}
	if p.ResponseHeaderTimeout.AsDuration() != 30*time.Second {
		t.Fatalf("response_header_timeout = %v", p.ResponseHeaderTimeout.AsDuration())
	}
	if p.DialTimeout.AsDuration() != 5*time.Second {
		t.Fatalf("dial_timeout = %v", p.DialTimeout.AsDuration())
	}
	if p.MaxHeaderBytes != 1048576 {
		t.Fatalf("max_header_bytes = %d", p.MaxHeaderBytes)
	}
	if !p.UpstreamInsecureSkipVerify {
		t.Fatal("upstream_insecure_skip_verify should be true")
	}
}

func TestProxyBlockRejectsBadDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, "[proxy]\nidle_timeout = \"notaduration\"\n")
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for bad duration")
	}
}

func TestNoProxyBlockYieldsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	writeFile(t, path, "domain = \"example.com\"\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy != nil {
		t.Fatalf("expected nil proxy block, got %+v", cfg.Proxy)
	}
}

func TestLogFormatValidation(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		val string
		ok  bool
	}{{"", true}, {"text", true}, {"json", true}, {"xml", false}} {
		path := filepath.Join(dir, "c.toml")
		writeFile(t, path, "access_log = true\nlog_format = \""+tc.val+"\"\n")
		_, err := Load(path)
		if tc.ok && err != nil {
			t.Errorf("log_format=%q: unexpected error %v", tc.val, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("log_format=%q: expected error", tc.val)
		}
	}
}

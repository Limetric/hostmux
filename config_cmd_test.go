package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hostmux.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigCheckValid(t *testing.T) {
	path := writeConfig(t, `domain = "example.com"
[[app]]
hosts = ["api"]
upstream = "http://127.0.0.1:8080"
`)
	var buf bytes.Buffer
	if err := runConfigCheck(configCheckOptions{ConfigPath: path, Writer: &buf}); err != nil {
		t.Fatalf("expected exit 0, got %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "OK") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestConfigCheckInvalidExitsNonZero(t *testing.T) {
	path := writeConfig(t, `listen = "8443"
[[app]]
hosts = ["api"]
upstream = "nope"
`)
	var buf bytes.Buffer
	err := runConfigCheck(configCheckOptions{ConfigPath: path, Writer: &buf})
	var ee exitError
	if !errors.As(err, &ee) || ee.code == 0 {
		t.Fatalf("expected non-zero exit, got %v", err)
	}
	if !strings.Contains(buf.String(), "FAIL") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestConfigCheckJSON(t *testing.T) {
	path := writeConfig(t, `domain = "localhost"
listen = ":8443"
[[app]]
hosts = ["app"]
upstream = "http://127.0.0.1:3000"
`)
	var buf bytes.Buffer
	if err := runConfigCheck(configCheckOptions{ConfigPath: path, JSON: true, Writer: &buf}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var res configCheckResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if !res.OK {
		t.Fatalf("expected OK=true (warnings only), got %+v", res)
	}
	if len(res.Diagnostics) == 0 {
		t.Fatalf("expected a warning diagnostic, got %+v", res)
	}
}

func TestConfigCheckMissingFile(t *testing.T) {
	err := runConfigCheck(configCheckOptions{ConfigPath: filepath.Join(t.TempDir(), "nope.toml")})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

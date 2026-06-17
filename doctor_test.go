package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/tlsconfig"
)

func TestDoctorHealthy(t *testing.T) {
	cfgPath, _, _ := writeCertConfig(t) // domain example.com, custom temp cert/key
	tlsCfg, err := resolveServeTLS(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := tlsconfig.EnsurePair(tlsCfg); err != nil {
		t.Fatal(err)
	}
	sock := serveInfoAndList(t, "example.com", 8443, nil)

	var buf bytes.Buffer
	if err := runDoctor(doctorOptions{ConfigPath: cfgPath, SocketPath: sock, Writer: &buf}); err != nil {
		t.Fatalf("expected healthy (exit 0), got %v\n%s", err, buf.String())
	}
	out := buf.String()
	for _, want := range []string{"running", "valid", "0 error(s)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsConfigError(t *testing.T) {
	path := writeConfig(t, `domain = "example.com"
[[app]]
hosts = ["api"]
upstream = "nope"
`)
	var buf bytes.Buffer
	err := runDoctor(doctorOptions{ConfigPath: path, SocketPath: "/nonexistent/hm.sock", Writer: &buf})
	var ee exitError
	if !errors.As(err, &ee) || ee.code == 0 {
		t.Fatalf("expected non-zero exit for bad config, got %v", err)
	}
	if !strings.Contains(buf.String(), "error") {
		t.Fatalf("output = %s", buf.String())
	}
}

func TestDoctorDaemonDownIsWarningNotError(t *testing.T) {
	cfgPath, _, _ := writeCertConfig(t) // valid config, cert not generated
	var buf bytes.Buffer
	// Unreachable socket -> daemon warning, but no error-level findings.
	if err := runDoctor(doctorOptions{ConfigPath: cfgPath, SocketPath: filepath.Join(t.TempDir(), "down.sock"), Writer: &buf}); err != nil {
		t.Fatalf("daemon-down should not be an error: %v\n%s", err, buf.String())
	}
	if !strings.Contains(buf.String(), "not reachable") {
		t.Fatalf("expected daemon warning:\n%s", buf.String())
	}
}

func TestDoctorJSON(t *testing.T) {
	cfgPath, _, _ := writeCertConfig(t)
	tlsCfg, _ := resolveServeTLS(cfgPath)
	_ = tlsconfig.EnsurePair(tlsCfg)
	sock := serveInfoAndList(t, "example.com", 8443, nil)

	var buf bytes.Buffer
	if err := runDoctor(doctorOptions{ConfigPath: cfgPath, SocketPath: sock, JSON: true, Writer: &buf}); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
	var out struct {
		OK       bool            `json:"ok"`
		Findings []doctorFinding `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if !out.OK || len(out.Findings) == 0 {
		t.Fatalf("expected ok=true with findings, got %+v", out)
	}
}

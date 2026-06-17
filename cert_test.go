package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/tlsconfig"
)

// writeCertConfig writes a config whose TLS cert/key live in a temp dir, so
// cert operations never touch ~/.hostmux.
func writeCertConfig(t *testing.T) (cfgPath, certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()
	certPath = filepath.Join(dir, "hostmux.crt")
	keyPath = filepath.Join(dir, "hostmux.key")
	cfgPath = filepath.Join(dir, "hostmux.toml")
	body := "domain = \"example.com\"\n[tls]\ncert = \"" + certPath + "\"\nkey = \"" + keyPath + "\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func TestCertPath(t *testing.T) {
	cfgPath, certPath, keyPath := writeCertConfig(t)
	var buf bytes.Buffer
	if err := runCertPath(certOptions{ConfigPath: cfgPath, Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, certPath) || !strings.Contains(out, keyPath) {
		t.Fatalf("output = %q", out)
	}
}

func TestCertInfoNotGenerated(t *testing.T) {
	cfgPath, _, _ := writeCertConfig(t)
	var buf bytes.Buffer
	if err := runCertInfo(certOptions{ConfigPath: cfgPath, Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "not generated") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestCertRenewAndInfo(t *testing.T) {
	cfgPath, certPath, _ := writeCertConfig(t)

	// Custom cert: renew requires --force.
	if err := runCertRenew(certOptions{ConfigPath: cfgPath, Writer: &bytes.Buffer{}}); err == nil {
		t.Fatal("expected refusal to renew custom cert without --force")
	}

	// With --force it generates.
	if err := runCertRenew(certOptions{ConfigPath: cfgPath, Force: true, Writer: &bytes.Buffer{}}); err != nil {
		t.Fatalf("renew --force: %v", err)
	}
	first, err := tlsconfig.Inspect(certPath)
	if err != nil {
		t.Fatal(err)
	}

	// Info now reports a valid cert.
	var info bytes.Buffer
	if err := runCertInfo(certOptions{ConfigPath: cfgPath, Writer: &info}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(info.String(), "valid") {
		t.Fatalf("info = %q", info.String())
	}

	// Renewing again rotates the serial.
	if err := runCertRenew(certOptions{ConfigPath: cfgPath, Force: true, Writer: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	second, err := tlsconfig.Inspect(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if first.Serial == second.Serial {
		t.Fatal("renew should rotate the certificate serial")
	}
}

func TestCertInfoJSON(t *testing.T) {
	cfgPath, _, _ := writeCertConfig(t)
	if err := runCertRenew(certOptions{ConfigPath: cfgPath, Force: true, Writer: &bytes.Buffer{}}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runCertInfo(certOptions{ConfigPath: cfgPath, JSON: true, Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	var out certInfoJSON
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if !out.Exists || out.Managed {
		t.Fatalf("expected exists=true managed=false, got %+v", out)
	}
	if out.CommonName == "" || out.Expired {
		t.Fatalf("unexpected cert info %+v", out)
	}
}

func TestCertManaged(t *testing.T) {
	// Custom cert -> not managed.
	cfgPath, _, _ := writeCertConfig(t)
	if m, _ := certManaged(cfgPath); m {
		t.Fatal("custom cert should not be managed")
	}
	// No tls.cert -> managed.
	dir := t.TempDir()
	noCert := filepath.Join(dir, "c.toml")
	if err := os.WriteFile(noCert, []byte("domain = \"example.com\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m, _ := certManaged(noCert); !m {
		t.Fatal("config without tls.cert should be managed")
	}
}

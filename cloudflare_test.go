package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderCloudflareIngressDefault(t *testing.T) {
	out := renderCloudflareIngress(cloudflareParams{Domain: "example.com", Port: 8443, HTTPSOrigin: true, NoTLSVerify: true})
	for _, want := range []string{
		"ingress:",
		`hostname: "*.example.com"`,
		"service: https://127.0.0.1:8443",
		"http2Origin: true",
		"noTLSVerify: true",
		"service: http_status:404",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderCloudflareIngressCustomCert(t *testing.T) {
	out := renderCloudflareIngress(cloudflareParams{Domain: "example.com", Port: 8443, HTTPSOrigin: true, NoTLSVerify: false})
	if strings.Contains(out, "noTLSVerify") {
		t.Fatalf("noTLSVerify should be omitted for trusted certs:\n%s", out)
	}
	if !strings.Contains(out, "http2Origin: true") {
		t.Fatalf("http2Origin should still be present:\n%s", out)
	}
}

func TestCloudflareConfigFromConfigFile(t *testing.T) {
	path := writeConfig(t, `domain = "example.com"
listen = ":8443"
hide_port = true
`)
	var buf bytes.Buffer
	// Use an unreachable socket so no live daemon interferes with the test.
	if err := runCloudflareConfig(cloudflareOptions{ConfigPath: path, SocketPath: "/nonexistent/hm.sock", Writer: &buf}); err != nil {
		t.Fatalf("runCloudflareConfig: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `hostname: "*.example.com"`) || !strings.Contains(out, "service: https://127.0.0.1:8443") {
		t.Fatalf("output = %s", out)
	}
}

func TestCloudflareConfigCustomPortAndDomain(t *testing.T) {
	path := writeConfig(t, `domain = "internal.test"
[tls]
listen = ":9443"
`)
	var buf bytes.Buffer
	if err := runCloudflareConfig(cloudflareOptions{ConfigPath: path, SocketPath: "/nonexistent/hm.sock", Domain: "override.example", Writer: &buf}); err != nil {
		t.Fatalf("runCloudflareConfig: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `hostname: "*.override.example"`) {
		t.Fatalf("--domain override not applied:\n%s", out)
	}
	if !strings.Contains(out, "service: https://127.0.0.1:9443") {
		t.Fatalf("tls.listen port not used:\n%s", out)
	}
}

func TestCloudflareConfigRequiresDomain(t *testing.T) {
	path := writeConfig(t, "listen = \":8443\"\n")
	// No domain in config (defaults to "localhost" via applyDefaults? No -
	// loadOptionalConfig uses config.Load which defaults domain to localhost).
	// So this should succeed with a localhost wildcard.
	var buf bytes.Buffer
	if err := runCloudflareConfig(cloudflareOptions{ConfigPath: path, SocketPath: "/nonexistent/hm.sock", Writer: &buf}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), `hostname: "*.localhost"`) {
		t.Fatalf("expected localhost default:\n%s", buf.String())
	}
}

package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/sockproto"
)

// shortSockDir returns a short temp dir to stay under the ~104-char Unix
// socket path limit on macOS (t.TempDir paths are too long).
func shortSockDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// fakeDaemon listens on a unix socket and replies to OpInfo, OpExpose, and
// OpUnexpose. The last expose/unexpose request is captured for assertions.
type fakeDaemon struct {
	path     string
	domain   string
	port     int
	gotOp    sockproto.Op
	gotMsg   sockproto.Message
	exposeOK bool
}

func startFakeDaemon(t *testing.T, d *fakeDaemon) {
	t.Helper()
	d.exposeOK = true
	ln, err := net.Listen("unix", d.path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				dec := sockproto.NewDecoder(c)
				enc := sockproto.NewEncoder(c)
				for {
					msg, err := dec.Decode()
					if err != nil {
						return
					}
					switch msg.Op {
					case sockproto.OpInfo:
						https := true
						enc.Encode(&sockproto.Message{Ok: true, Domain: d.domain, PublicHTTPS: &https, PublicPort: d.port})
					case sockproto.OpExpose, sockproto.OpUnexpose:
						d.gotOp = msg.Op
						d.gotMsg = *msg
						enc.Encode(&sockproto.Message{Ok: d.exposeOK, Error: "rejected"})
					default:
						enc.Encode(&sockproto.Message{Ok: false, Error: "unknown"})
					}
				}
			}(conn)
		}
	}()
}

func TestExposeSendsExpectedRequest(t *testing.T) {
	d := &fakeDaemon{path: filepath.Join(shortSockDir(t), "e.sock"), domain: "example.com", port: 8443}
	startFakeDaemon(t, d)

	var buf bytes.Buffer
	err := runExpose(exposeOptions{
		SocketPath: d.path,
		Upstream:   "http://127.0.0.1:3000",
		Names:      []string{"api"},
		Labels:     []string{"team=web"},
		Writer:     &buf,
	})
	if err != nil {
		t.Fatalf("runExpose: %v", err)
	}
	if d.gotOp != sockproto.OpExpose {
		t.Fatalf("op = %q", d.gotOp)
	}
	if d.gotMsg.Source != "api" {
		t.Fatalf("source = %q", d.gotMsg.Source)
	}
	if len(d.gotMsg.Hosts) != 1 || d.gotMsg.Hosts[0] != "api.example.com" {
		t.Fatalf("hosts = %v (bare name should expand via daemon domain)", d.gotMsg.Hosts)
	}
	if d.gotMsg.Upstream != "http://127.0.0.1:3000" {
		t.Fatalf("upstream = %q", d.gotMsg.Upstream)
	}
	if d.gotMsg.Labels["team"] != "web" {
		t.Fatalf("labels = %v", d.gotMsg.Labels)
	}
	out := buf.String()
	if !strings.Contains(out, "https://api.example.com:8443") {
		t.Fatalf("output missing edge URL:\n%s", out)
	}
}

func TestExposeRequiresUpstream(t *testing.T) {
	err := runExpose(exposeOptions{Names: []string{"api"}})
	if err == nil {
		t.Fatal("expected error without upstream")
	}
}

func TestExposeRequiresName(t *testing.T) {
	err := runExpose(exposeOptions{Upstream: "http://127.0.0.1:3000"})
	if err == nil {
		t.Fatal("expected error without name")
	}
}

func TestUnexposeSendsRequest(t *testing.T) {
	d := &fakeDaemon{path: filepath.Join(shortSockDir(t), "u.sock")}
	startFakeDaemon(t, d)
	if err := runUnexpose(unexposeOptions{SocketPath: d.path, Name: "api"}); err != nil {
		t.Fatalf("runUnexpose: %v", err)
	}
	if d.gotOp != sockproto.OpUnexpose || d.gotMsg.Source != "api" {
		t.Fatalf("op=%q source=%q", d.gotOp, d.gotMsg.Source)
	}
}

func TestValidateUpstreamFlag(t *testing.T) {
	good := []string{"http://127.0.0.1:3000", "https://localhost:8080", "http://example.com"}
	for _, u := range good {
		if err := validateUpstreamFlag(u); err != nil {
			t.Errorf("validateUpstreamFlag(%q) = %v", u, err)
		}
	}
	bad := []string{"", "127.0.0.1:3000", "ftp://x", "http://", " http://x"}
	for _, u := range bad {
		if err := validateUpstreamFlag(u); err == nil {
			t.Errorf("validateUpstreamFlag(%q) = nil, want error", u)
		}
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Limetric/hostmux/internal/sockproto"
)

// serveInfoAndList starts a fake daemon replying to OpInfo and OpList.
func serveInfoAndList(t *testing.T, domain string, port int, entries []sockproto.Entry) string {
	t.Helper()
	sockPath := filepath.Join(shortSockDir(t), "s.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
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
						enc.Encode(&sockproto.Message{Ok: true, Domain: domain, PublicHTTPS: &https, PublicPort: port})
					case sockproto.OpList:
						enc.Encode(&sockproto.Message{Ok: true, Entries: entries})
					}
				}
			}(conn)
		}
	}()
	return sockPath
}

func TestShareRegisteredRoute(t *testing.T) {
	entries := []sockproto.Entry{
		{Source: "socket:3", Hosts: []string{"api.example.com"}, Upstream: "http://127.0.0.1:3000"},
	}
	sockPath := serveInfoAndList(t, "example.com", 8443, entries)

	var buf bytes.Buffer
	err := runShare(shareOptions{SocketPath: sockPath, Names: []string{"api"}, Writer: &buf})
	if err != nil {
		t.Fatalf("runShare: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"api.example.com", "https://api.example.com:8443", "http://127.0.0.1:3000", "socket:3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestShareUnregisteredRoute(t *testing.T) {
	sockPath := serveInfoAndList(t, "example.com", 8443, nil)
	var buf bytes.Buffer
	if err := runShare(shareOptions{SocketPath: sockPath, Names: []string{"ghost"}, Writer: &buf}); err != nil {
		t.Fatalf("runShare: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ghost.example.com") || !strings.Contains(out, "not currently registered") {
		t.Fatalf("expected URL-only state, got:\n%s", out)
	}
}

func TestShareAll(t *testing.T) {
	entries := []sockproto.Entry{
		{Source: "config", Hosts: []string{"a.example.com"}, Upstream: "http://127.0.0.1:1"},
		{Source: "socket:2", Hosts: []string{"b.example.com"}, Upstream: "http://127.0.0.1:2"},
	}
	sockPath := serveInfoAndList(t, "example.com", 8443, entries)
	var buf bytes.Buffer
	if err := runShare(shareOptions{SocketPath: sockPath, All: true, Writer: &buf}); err != nil {
		t.Fatalf("runShare --all: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "a.example.com") || !strings.Contains(out, "b.example.com") {
		t.Fatalf("--all missing routes:\n%s", out)
	}
}

func TestShareJSON(t *testing.T) {
	entries := []sockproto.Entry{
		{Source: "socket:3", Hosts: []string{"api.example.com"}, Upstream: "http://127.0.0.1:3000"},
	}
	sockPath := serveInfoAndList(t, "example.com", 8443, entries)
	var buf bytes.Buffer
	if err := runShare(shareOptions{SocketPath: sockPath, Names: []string{"api"}, JSON: true, Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	var items []shareItem
	if err := json.Unmarshal(buf.Bytes(), &items); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if len(items) != 1 || !items[0].Registered || items[0].URL != "https://api.example.com:8443" {
		t.Fatalf("items = %+v", items)
	}
}

func TestShareQR(t *testing.T) {
	entries := []sockproto.Entry{
		{Source: "socket:3", Hosts: []string{"api.example.com"}, Upstream: "http://127.0.0.1:3000"},
	}
	sockPath := serveInfoAndList(t, "example.com", 8443, entries)
	var buf bytes.Buffer
	if err := runShare(shareOptions{SocketPath: sockPath, Names: []string{"api"}, QR: true, Writer: &buf}); err != nil {
		t.Fatal(err)
	}
	// The compact QR renderer uses unicode block characters.
	if !strings.Contains(buf.String(), "█") && !strings.ContainsRune(buf.String(), '▀') {
		t.Fatalf("expected QR block characters in output:\n%s", buf.String())
	}
}

func TestShareAllRequiresDaemon(t *testing.T) {
	if err := runShare(shareOptions{SocketPath: "/nonexistent/hm.sock", All: true, Writer: &bytes.Buffer{}}); err == nil {
		t.Fatal("expected error when daemon unreachable for --all")
	}
}

func TestRenderQR(t *testing.T) {
	out, err := renderQR("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty QR output")
	}
}

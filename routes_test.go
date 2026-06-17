package main

import (
	"bytes"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Limetric/hostmux/internal/sockproto"
)

func TestRunRoutesPrintsRouteTable(t *testing.T) {
	sockDir := t.TempDir()
	sockPath := filepath.Join(sockDir, "t.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		dec := sockproto.NewDecoder(conn)
		enc := sockproto.NewEncoder(conn)
		msg, err := dec.Decode()
		if err != nil || msg.Op != sockproto.OpList {
			return
		}
		_ = enc.Encode(&sockproto.Message{
			Ok: true,
			Entries: []sockproto.Entry{
				{Source: "config", Hosts: []string{"a.example.com", "b.example.com"}, Upstream: "http://127.0.0.1:9000"},
			},
		})
	}()

	var buf bytes.Buffer
	if err := runRoutes(routesOptions{SocketPath: sockPath, Writer: &buf}); err != nil {
		t.Fatalf("runRoutes: %v", err)
	}

	out := buf.String()
	for _, line := range []string{
		"SOURCE",
		"HOSTS",
		"UPSTREAM",
		"config",
		"a.example.com,b.example.com",
		"http://127.0.0.1:9000",
	} {
		if !strings.Contains(out, line) {
			t.Fatalf("output missing %q\n%s", line, out)
		}
	}
}

// serveList starts a fake daemon that replies to OpList with the given
// entries and returns the socket path.
func serveList(t *testing.T, entries []sockproto.Entry) string {
	t.Helper()
	sockPath := filepath.Join(shortSockDir(t), "l.sock")
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
					if msg.Op == sockproto.OpList {
						enc.Encode(&sockproto.Message{Ok: true, Entries: entries})
					}
				}
			}(conn)
		}
	}()
	return sockPath
}

func TestRunRoutesJSON(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	entries := []sockproto.Entry{
		{
			Source:       "socket:1",
			Hosts:        []string{"api.example.com"},
			Upstream:     "http://127.0.0.1:3000",
			Labels:       map[string]string{"team": "web"},
			PID:          4242,
			Command:      "bun run dev",
			Cwd:          "/work/app",
			RegisteredAt: now.Unix() - 120, // 2 minutes old
		},
	}
	sockPath := serveList(t, entries)

	var buf bytes.Buffer
	err := runRoutes(routesOptions{SocketPath: sockPath, JSON: true, Writer: &buf, now: func() time.Time { return now }})
	if err != nil {
		t.Fatalf("runRoutes: %v", err)
	}
	var out []routeJSON
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("json: %v\n%s", err, buf.String())
	}
	if len(out) != 1 {
		t.Fatalf("out = %+v", out)
	}
	r := out[0]
	if r.Source != "socket:1" || r.PID != 4242 || r.Command != "bun run dev" || r.Labels["team"] != "web" {
		t.Fatalf("route = %+v", r)
	}
	if r.AgeSeconds != 120 {
		t.Fatalf("age_seconds = %d, want 120", r.AgeSeconds)
	}
}

func TestRunRoutesWide(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	entries := []sockproto.Entry{{
		Source:       "manual:api",
		Hosts:        []string{"api.example.com"},
		Upstream:     "http://127.0.0.1:3000",
		Labels:       map[string]string{"kind": "api"},
		PID:          99,
		Command:      "external",
		RegisteredAt: now.Unix() - 3600,
	}}
	sockPath := serveList(t, entries)

	var buf bytes.Buffer
	if err := runRoutes(routesOptions{SocketPath: sockPath, Wide: true, Writer: &buf, now: func() time.Time { return now }}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"PID", "LABELS", "COMMAND", "manual:api", "99", "kind=api", "1h"} {
		if !strings.Contains(out, want) {
			t.Fatalf("wide output missing %q\n%s", want, out)
		}
	}
}

func TestHumanizeDuration(t *testing.T) {
	cases := map[time.Duration]string{
		5 * time.Second:  "5s",
		90 * time.Second: "1m",
		2 * time.Hour:    "2h",
		49 * time.Hour:   "2d",
	}
	for d, want := range cases {
		if got := humanizeDuration(d); got != want {
			t.Errorf("humanizeDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

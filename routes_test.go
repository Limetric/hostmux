package main

import (
	"bytes"
	"net"
	"path/filepath"
	"strings"
	"testing"

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

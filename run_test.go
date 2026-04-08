package main

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Limetric/hostmux/internal/sockproto"
)

func TestCmdRunUsesDashBetweenPrefixAndHost(t *testing.T) {
	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "hostmux.sock")
	hostsCh := make(chan []string, 1)
	errCh := make(chan error, 1)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		msg, err := sockproto.NewDecoder(conn).Decode()
		if err != nil {
			errCh <- err
			return
		}
		hostsCh <- msg.Hosts
		errCh <- sockproto.NewEncoder(conn).Encode(&sockproto.Message{Ok: true})
	}()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	code := cmdRun([]string{
		"--socket", sockPath,
		"--prefix", "feature-x",
		"myapp.test",
		"--",
		"/usr/bin/true",
	})
	if code != 0 {
		t.Fatalf("cmdRun exit code = %d, want 0", code)
	}

	select {
	case hosts := <-hostsCh:
		want := []string{"feature-x-myapp.test"}
		if !reflect.DeepEqual(hosts, want) {
			t.Fatalf("registered hosts = %v, want %v", hosts, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for registration")
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server completion")
	}
}

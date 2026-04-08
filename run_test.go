package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestCmdRunExpandsBareHostWithDomainFlag(t *testing.T) {
	hosts, code, stderr := runCmdRunAndCapture(t, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"api",
		"--",
		"/usr/bin/true",
	})
	if code != 0 {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"api.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestCmdRunPreservesFullHostnameWithDomainFlag(t *testing.T) {
	hosts, code, stderr := runCmdRunAndCapture(t, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"admin.other.test",
		"--",
		"/usr/bin/true",
	})
	if code != 0 {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"admin.other.test"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestCmdRunAppliesPrefixBeforeDomainExpansion(t *testing.T) {
	hosts, code, stderr := runCmdRunAndCapture(t, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"--prefix", "feature-x",
		"api",
		"--",
		"/usr/bin/true",
	})
	if code != 0 {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"feature-x-api.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestCmdRunUsesDaemonDomainForBareHost(t *testing.T) {
	hosts, code, stderr := runCmdRunAndCapture(t, runServerScript{
		domain: "example.com",
	}, []string{
		"api",
		"--",
		"/usr/bin/true",
	})
	if code != 0 {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"api.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestCmdRunRejectsBareHostWithoutAnyDomain(t *testing.T) {
	_, code, stderr := runCmdRunAndCapture(t, runServerScript{}, []string{
		"api",
		"--",
		"/usr/bin/true",
	})
	if code == 0 {
		t.Fatal("expected cmdRun to fail")
	}
	if !strings.Contains(stderr, "require --domain or daemon config domain") {
		t.Fatalf("stderr = %q", stderr)
	}
}

type runServerScript struct {
	domain string
}

func runCmdRunAndCapture(t *testing.T, script runServerScript, args []string) ([]string, int, string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

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

		dec := sockproto.NewDecoder(conn)
		enc := sockproto.NewEncoder(conn)
		for {
			msg, err := dec.Decode()
			if err != nil {
				errCh <- err
				return
			}
			switch msg.Op {
			case sockproto.OpInfo:
				if err := enc.Encode(&sockproto.Message{Ok: true, Domain: script.domain}); err != nil {
					errCh <- err
				}
			case sockproto.OpRegister:
				hostsCh <- msg.Hosts
				errCh <- enc.Encode(&sockproto.Message{Ok: true})
				return
			default:
				errCh <- enc.Encode(&sockproto.Message{Ok: false, Error: "unexpected op"})
				return
			}
		}
	}()

	var stderr bytes.Buffer
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	done := make(chan struct{})
	go func() {
		_, _ = stderr.ReadFrom(r)
		close(done)
	}()

	code := cmdRun(append([]string{"--socket", sockPath}, args...))

	_ = w.Close()
	<-done

	var hosts []string
	select {
	case hosts = <-hostsCh:
	case <-time.After(2 * time.Second):
	}

	select {
	case err := <-errCh:
		if code == 0 && err != nil {
			t.Fatalf("server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		if code == 0 {
			t.Fatal("timed out waiting for server completion")
		}
	}

	return hosts, code, stderr.String()
}

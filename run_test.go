package main

import (
	"bytes"
	"fmt"
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

		dec := sockproto.NewDecoder(conn)
		enc := sockproto.NewEncoder(conn)
		for i := 0; i < 2; i++ {
			msg, err := dec.Decode()
			if err != nil {
				errCh <- err
				return
			}
			switch msg.Op {
			case sockproto.OpInfo:
				https := true
				if err := enc.Encode(&sockproto.Message{Ok: true, PublicHTTPS: &https}); err != nil {
					errCh <- err
					return
				}
			case sockproto.OpRegister:
				hostsCh <- msg.Hosts
				errCh <- enc.Encode(&sockproto.Message{Ok: true})
				return
			default:
				errCh <- fmt.Errorf("unexpected op %q", msg.Op)
				return
			}
		}
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

func TestCmdRunPassesThroughBareHostWhenNoDomainAvailable(t *testing.T) {
	hosts, code, stderr := runCmdRunAndCapture(t, runServerScript{}, []string{
		"api",
		"--",
		"/usr/bin/true",
	})
	if code != 0 {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"api"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestCmdRunFallsBackWhenDaemonDoesNotSupportInfo(t *testing.T) {
	hosts, code, stderr := runCmdRunAndCapture(t, runServerScript{
		infoOk:    false,
		infoError: "unsupported operation",
	}, []string{
		"api",
		"--",
		"sh", "-c", `[ -z "${HOSTMUX_URL}" ]`,
	})
	if code != 0 {
		t.Fatalf("cmdRun exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"api"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
	if got := stderr; got == "" || !bytes.Contains([]byte(got), []byte("using bare hosts unchanged")) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCmdRunHostmuxURLSchemeMatchesDaemonEdge(t *testing.T) {
	tests := []struct {
		name      string
		plainEdge bool
		wantURL   string
	}{
		{"tls", false, "https://api.example.com"},
		{"plain", true, "http://api.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, code, stderr := runCmdRunAndCapture(t, runServerScript{
				domain:    "example.com",
				plainEdge: tt.plainEdge,
			}, []string{
				"api",
				"--",
				"sh", "-c", `test "$HOSTMUX_URL" = "` + tt.wantURL + `"`,
			})
			if code != 0 {
				t.Fatalf("cmdRun exit code = %d, stderr = %q", code, stderr)
			}
		})
	}
}

type runServerScript struct {
	domain    string
	infoOk    bool
	infoError string
	// plainEdge is true when the fake daemon uses plain HTTP on its public
	// listener (OpInfo reports public_https: false).
	plainEdge bool
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
				ok := true
				if script.infoError != "" {
					ok = script.infoOk
				}
				msg := &sockproto.Message{Ok: ok, Domain: script.domain, Error: script.infoError}
				if ok {
					https := !script.plainEdge
					msg.PublicHTTPS = &https
				}
				if err := enc.Encode(msg); err != nil {
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

package main

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Limetric/hostmux/internal/sockproto"
)

func TestURLCommandPrintsExpandedURLWithDomainFlag(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		Domain:  "example.com",
		HostArg: "backend",
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestURLCommandAppliesPrefixBeforeDomainExpansion(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		Domain:  "example.com",
		Prefix:  "feature-x",
		HostArg: "backend",
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://feature-x-backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestURLCommandNoPrefixFlagLeavesHostUnprefixed(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		Domain:   "example.com",
		NoPrefix: true,
		HostArg:  "backend",
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestURLCommandUsesDaemonDomainForBareHost(t *testing.T) {
	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	sockPath := filepath.Join(sockDir, "hostmux.sock")
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
		if msg.Op != sockproto.OpInfo {
			errCh <- sockproto.NewEncoder(conn).Encode(&sockproto.Message{Ok: false, Error: "unexpected op"})
			return
		}
		errCh <- sockproto.NewEncoder(conn).Encode(&sockproto.Message{Ok: true, Domain: "example.com"})
	}()

	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		SocketPath: sockPath,
		HostArg:    "backend",
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
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

func TestURLCommandPassesThroughBareHostWhenNoDomainAvailable(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		SocketPath: filepath.Join(t.TempDir(), "missing.sock"),
		HostArg:    "backend",
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://backend\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if !strings.Contains(stderr, "using bare host unchanged") {
		t.Fatalf("stderr = %q, want warning about bare host fallback", stderr)
	}
}

func TestURLCommandPreservesFullHostnameWithDomainFlag(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		Domain:  "example.com",
		HostArg: "admin.other.test",
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://admin.other.test\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestURLCommandDomainFlagTakesPriorityOverDaemonLookup(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		SocketPath: filepath.Join(t.TempDir(), "missing.sock"),
		Domain:     "example.com",
		HostArg:    "backend",
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr", stderr)
	}
}

func TestURLCommandRejectsMultipleHosts(t *testing.T) {
	_, stderr, err := runURLAndCapture(t, urlOptions{HostArg: "backend,admin"})
	if err == nil {
		t.Fatal("runURL error = nil, want usage error")
	}
	if !strings.Contains(err.Error(), "single hostname") {
		t.Fatalf("error = %v, stderr = %q", err, stderr)
	}
	if exit, ok := err.(exitError); !ok || exit.code != 2 {
		t.Fatalf("error = %#v, want exitError{code: 2}", err)
	}
}

func runURLAndCapture(t *testing.T, opts urlOptions) (string, string, error) {
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

	stdout, restoreStdout := captureURLFileOutput(t, &os.Stdout)
	stderr, restoreStderr := captureURLFileOutput(t, &os.Stderr)

	err = runURL(opts)

	restoreStdout()
	restoreStderr()

	return stdout.String(), stderr.String(), err
}

func captureURLFileOutput(t *testing.T, target **os.File) (*bytes.Buffer, func()) {
	t.Helper()

	var buf bytes.Buffer
	old := *target
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	*target = w

	done := make(chan struct{})
	go func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	}()

	return &buf, func() {
		_ = w.Close()
		<-done
		*target = old
	}
}

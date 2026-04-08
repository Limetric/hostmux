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

func TestCmdGetPrintsExpandedURLWithDomainFlag(t *testing.T) {
	stdout, stderr, code := runCmdGetAndCapture(t, []string{
		"--domain", "example.com",
		"backend",
	})
	if code != 0 {
		t.Fatalf("cmdGet exit code = %d, stderr = %q", code, stderr)
	}
	if got, want := stdout, "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCmdGetNoArgsPrintsUsage(t *testing.T) {
	stdout, stderr, code := runCmdGetAndCapture(t, nil)
	if code != 2 {
		t.Fatalf("cmdGet exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "usage: hostmux get HOST") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCmdGetAppliesPrefixBeforeDomainExpansion(t *testing.T) {
	stdout, stderr, code := runCmdGetAndCapture(t, []string{
		"--domain", "example.com",
		"--prefix", "feature-x",
		"backend",
	})
	if code != 0 {
		t.Fatalf("cmdGet exit code = %d, stderr = %q", code, stderr)
	}
	if got, want := stdout, "https://feature-x-backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCmdGetNoPrefixFlagLeavesHostUnprefixed(t *testing.T) {
	stdout, stderr, code := runCmdGetAndCapture(t, []string{
		"--domain", "example.com",
		"--no-prefix",
		"backend",
	})
	if code != 0 {
		t.Fatalf("cmdGet exit code = %d, stderr = %q", code, stderr)
	}
	if got, want := stdout, "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCmdGetUsesDaemonDomainForBareHost(t *testing.T) {
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

	stdout, stderr, code := runCmdGetAndCapture(t, []string{
		"--socket", sockPath,
		"backend",
	})
	if code != 0 {
		t.Fatalf("cmdGet exit code = %d, stderr = %q", code, stderr)
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

func TestCmdGetPassesThroughBareHostWhenNoDomainAvailable(t *testing.T) {
	stdout, stderr, code := runCmdGetAndCapture(t, []string{
		"--socket", filepath.Join(t.TempDir(), "missing.sock"),
		"backend",
	})
	if code != 0 {
		t.Fatalf("cmdGet exit code = %d, stderr = %q", code, stderr)
	}
	if got, want := stdout, "https://backend\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if !strings.Contains(stderr, "using bare host unchanged") {
		t.Fatalf("stderr = %q, want warning about bare host fallback", stderr)
	}
}

func TestCmdGetPreservesFullHostnameWithDomainFlag(t *testing.T) {
	stdout, stderr, code := runCmdGetAndCapture(t, []string{
		"--domain", "example.com",
		"admin.other.test",
	})
	if code != 0 {
		t.Fatalf("cmdGet exit code = %d, stderr = %q", code, stderr)
	}
	if got, want := stdout, "https://admin.other.test\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCmdGetDomainFlagTakesPriorityOverDaemonLookup(t *testing.T) {
	stdout, stderr, code := runCmdGetAndCapture(t, []string{
		"--socket", filepath.Join(t.TempDir(), "missing.sock"),
		"--domain", "example.com",
		"backend",
	})
	if code != 0 {
		t.Fatalf("cmdGet exit code = %d, stderr = %q", code, stderr)
	}
	if got, want := stdout, "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr", stderr)
	}
}

func TestCmdGetRejectsMultipleHosts(t *testing.T) {
	stdout, stderr, code := runCmdGetAndCapture(t, []string{"backend,admin"})
	if code != 2 {
		t.Fatalf("cmdGet exit code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "HOST must be a single hostname") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func runCmdGetAndCapture(t *testing.T, args []string) (string, string, int) {
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

	stdout, restoreStdout := captureFileOutput(t, &os.Stdout)
	stderr, restoreStderr := captureFileOutput(t, &os.Stderr)

	code := cmdGet(args)

	restoreStdout()
	restoreStderr()

	return stdout.String(), stderr.String(), code
}

func captureFileOutput(t *testing.T, target **os.File) (*bytes.Buffer, func()) {
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

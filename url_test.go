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
		Domain: "example.com",
		Names:  []string{"backend"},
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestURLCommandAcceptsPositionalName(t *testing.T) {
	cmd := newURLCmd()
	cmd.SetArgs([]string{"--domain", "example.com", "--no-prefix", "my-app"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := stdout.String(), "https://my-app.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr", stderr.String())
	}
}

func TestURLCommandUsesCobraOutputWriter(t *testing.T) {
	cmd := newURLCmd()
	cmd.SetArgs([]string{"--domain", "example.com", "--no-prefix", "--name", "backend"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := stdout.String(), "https://backend.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr", stderr.String())
	}
}

func TestURLCommandInfersNameFromPackageJSONWhenNameOmitted(t *testing.T) {
	oldWD := mustChdirTempDir(t)
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	mustWriteFile(t, filepath.Join(".", "package.json"), `{"name":"@scope/Web App"}`)

	cmd := newURLCmd()
	cmd.SetArgs([]string{"--domain", "example.com", "--no-prefix"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := stdout.String(), "https://web-app.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr", stderr.String())
	}
}

func TestURLCommandPrintsOneLinePerRepeatedNameFlag(t *testing.T) {
	cmd := newURLCmd()
	cmd.SetArgs([]string{"--domain", "example.com", "--no-prefix", "--name", "backend", "--name", "admin"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := stdout.String(), "https://backend.example.com\nhttps://admin.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr", stderr.String())
	}
}

func TestURLCommandRejectsInvalidExplicitName(t *testing.T) {
	_, stderr, err := runURLAndCapture(t, urlOptions{Names: []string{"My App"}})
	if err == nil {
		t.Fatal("runURL error = nil, want error")
	}
	if got := err.Error(); !strings.Contains(got, "valid bare label, hostname, or IP literal") {
		t.Fatalf("error = %q, stderr = %q", got, stderr)
	}
}

func TestRunURLSupportsMultipleExplicitNames(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		Domain: "example.com",
		Names:  []string{"backend", "admin"},
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://backend.example.com\nhttps://admin.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestURLHelpShowsRepeatedNameUsage(t *testing.T) {
	cmd := newURLCmd()
	cmd.SetArgs([]string{"--help"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := stdout.String()
	for _, want := range []string{
		"url [NAME]... [--name NAME]...",
		"Print the public URL for a host",
		"repeatable hostname to print",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q\n%s", want, help)
		}
	}
}

func TestURLCommandAppliesPrefixBeforeDomainExpansion(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		Domain: "example.com",
		Prefix: "feature-x",
		Names:  []string{"backend"},
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
		Names:    []string{"backend"},
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
		Names:      []string{"backend"},
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
		Names:      []string{"backend"},
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
		Domain: "example.com",
		Names:  []string{"admin.other.test"},
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
		Names:      []string{"backend"},
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

func TestURLCommandPrintsMultipleHostsFromNames(t *testing.T) {
	stdout, stderr, err := runURLAndCapture(t, urlOptions{
		Domain: "example.com",
		Names:  []string{"backend", "admin"},
	})
	if err != nil {
		t.Fatalf("runURL error = %v, stderr = %q", err, stderr)
	}
	if got, want := stdout, "https://backend.example.com\nhttps://admin.example.com\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func runURLAndCapture(t *testing.T, opts urlOptions) (string, string, error) {
	t.Helper()

	oldWD := mustChdirTempDir(t)
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	var stdout bytes.Buffer
	opts.Writer = &stdout
	stderr, restoreStderr := captureRootFileOutput(t, &os.Stderr)

	err := runURL(opts)

	restoreStderr()

	return stdout.String(), stderr.String(), err
}

func mustChdirTempDir(t *testing.T) string {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return oldWD
}

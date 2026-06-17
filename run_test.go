package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Limetric/hostmux/internal/sockproto"
)

func TestRunCommandSeparatesNamesAndChildArgs(t *testing.T) {
	oldRunner := runRunner
	t.Cleanup(func() { runRunner = oldRunner })

	var got runOptions
	runRunner = func(opts runOptions) error {
		got = opts
		return nil
	}

	cmd := newRunCmd()
	cmd.SetArgs([]string{
		"--socket", "/tmp/hostmux.sock",
		"--domain", "example.com",
		"--name", "backend",
		"--name", "admin",
		"--prefix", "feature-x",
		"--",
		"bin/server",
		"--listen",
		":8080",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got.SocketPath != "/tmp/hostmux.sock" {
		t.Fatalf("SocketPath = %q, want %q", got.SocketPath, "/tmp/hostmux.sock")
	}
	if got.Domain != "example.com" {
		t.Fatalf("Domain = %q, want %q", got.Domain, "example.com")
	}
	if got.Prefix != "feature-x" {
		t.Fatalf("Prefix = %q, want %q", got.Prefix, "feature-x")
	}
	if want := []string{"backend", "admin"}; !reflect.DeepEqual(got.Names, want) {
		t.Fatalf("Names = %v, want %v", got.Names, want)
	}
	wantArgv := []string{"bin/server", "--listen", ":8080"}
	if !reflect.DeepEqual(got.Argv, wantArgv) {
		t.Fatalf("Argv = %v, want %v", got.Argv, wantArgv)
	}
}

func TestRunCommandDelegatesToRunner(t *testing.T) {
	oldRunner := runRunner
	t.Cleanup(func() { runRunner = oldRunner })

	var got runOptions
	runRunner = func(opts runOptions) error {
		got = opts
		return nil
	}

	cmd := newRunCmd()
	cmd.SetArgs([]string{
		"--socket", "/tmp/hostmux.sock",
		"--domain", "example.com",
		"--name", "api",
		"--",
		"bin/server",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.SocketPath != "/tmp/hostmux.sock" {
		t.Fatalf("SocketPath = %q, want %q", got.SocketPath, "/tmp/hostmux.sock")
	}
	if got.Domain != "example.com" {
		t.Fatalf("Domain = %q, want %q", got.Domain, "example.com")
	}
	if want := []string{"api"}; !reflect.DeepEqual(got.Names, want) {
		t.Fatalf("Names = %v, want %v", got.Names, want)
	}
	wantArgv := []string{"bin/server"}
	if !reflect.DeepEqual(got.Argv, wantArgv) {
		t.Fatalf("Argv = %v, want %v", got.Argv, wantArgv)
	}
}

func TestRunCommandWithoutDoubleDashPassesArgsAsChild(t *testing.T) {
	oldRunner := runRunner
	t.Cleanup(func() { runRunner = oldRunner })

	var got runOptions
	runRunner = func(opts runOptions) error {
		got = opts
		return nil
	}

	cmd := newRunCmd()
	cmd.SetArgs([]string{"vite", "dev"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantArgv := []string{"vite", "dev"}
	if !reflect.DeepEqual(got.Argv, wantArgv) {
		t.Fatalf("Argv = %v, want %v", got.Argv, wantArgv)
	}
}

func TestRunCommandChildAfterFlagsWithoutDoubleDash(t *testing.T) {
	oldRunner := runRunner
	t.Cleanup(func() { runRunner = oldRunner })

	var got runOptions
	runRunner = func(opts runOptions) error {
		got = opts
		return nil
	}

	cmd := newRunCmd()
	cmd.SetArgs([]string{"--name", "api", "bin/server"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if want := []string{"api"}; !reflect.DeepEqual(got.Names, want) {
		t.Fatalf("Names = %v, want %v", got.Names, want)
	}
	wantArgv := []string{"bin/server"}
	if !reflect.DeepEqual(got.Argv, wantArgv) {
		t.Fatalf("Argv = %v, want %v", got.Argv, wantArgv)
	}
}

func TestRunCommandRejectsPositionalsBeforeDoubleDash(t *testing.T) {
	var stderr bytes.Buffer
	cmd := newRunCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"api", "--", "hostname"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want usage error")
	}
	var exitErr exitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Execute() error = %T %v, want exitError", err, err)
	}
	if exitErr.code != 2 {
		t.Fatalf("exit code = %d, want 2", exitErr.code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("usage: hostmux run")) {
		t.Fatalf("stderr = %q, want usage substring", stderr.String())
	}
}

func TestRunCommandAllowsMissingNamesAtParserLevel(t *testing.T) {
	oldRunner := runRunner
	t.Cleanup(func() { runRunner = oldRunner })

	called := false
	runRunner = func(opts runOptions) error {
		called = true
		if len(opts.Names) != 0 {
			t.Fatalf("Names = %v, want empty", opts.Names)
		}
		wantArgv := []string{"bin/server"}
		if !reflect.DeepEqual(opts.Argv, wantArgv) {
			t.Fatalf("Argv = %v, want %v", opts.Argv, wantArgv)
		}
		return nil
	}

	cmd := newRunCmd()
	cmd.SetArgs([]string{"--", "bin/server"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("runRunner was not called")
	}
}

func TestRunCommandPreservesEmptyExplicitNamesAtParserLevel(t *testing.T) {
	oldRunner := runRunner
	t.Cleanup(func() { runRunner = oldRunner })

	called := false
	runRunner = func(opts runOptions) error {
		called = true
		want := []string{"", "admin"}
		if !reflect.DeepEqual(opts.Names, want) {
			t.Fatalf("Names = %v, want %v", opts.Names, want)
		}
		return nil
	}

	cmd := newRunCmd()
	cmd.SetArgs([]string{"--name=", "--name", "admin", "--", "bin/server"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("runRunner was not called")
	}
}

func TestRunCommandUsesDashBetweenPrefixAndHost(t *testing.T) {
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
		for range 2 {
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

	cmd := newRunCmd()
	cmd.SetArgs([]string{
		"--socket", sockPath,
		"--prefix", "feature-x",
		"--name", "myapp.test",
		"--",
		"hostname",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
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

func TestRunCommandExpandsBareHostWithDomainFlag(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"--name", "api",
		"--",
		"hostname",
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"api.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestRunCommandPreservesFullHostnameWithDomainFlag(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"--name", "admin.other.test",
		"--",
		"hostname",
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"admin.other.test"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestRunCommandAppliesPrefixBeforeDomainExpansion(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"--prefix", "feature-x",
		"--name", "api",
		"--",
		"hostname",
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"feature-x-api.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestRunCommandUsesDaemonDomainForBareHost(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{
		domain: "example.com",
	}, []string{
		"--name", "api",
		"--",
		"hostname",
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"api.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestRunCommandInfersNameFromPackageJSONWhenFlagOmitted(t *testing.T) {
	wd := t.TempDir()
	mustWriteFile(t, filepath.Join(wd, "package.json"), `{"name":"@scope/Web App"}`)

	hosts, code, stderr := runRunCommandAndCaptureInDir(t, wd, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"--",
		"hostname",
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"web-app.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestRunCommandRegistersMultipleExplicitNamesInOrder(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"--name", "backend",
		"--name", "admin",
		"--",
		"hostname",
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"backend.example.com", "admin.example.com"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestRunCommandUsesFirstResolvedHostForHostmuxURLWhenMultipleNamesProvided(t *testing.T) {
	_, code, stderr := runRunCommandAndCapture(t, runServerScript{
		domain: "ignored.test",
	}, []string{
		"--domain", "example.com",
		"--name", "backend",
		"--name", "admin",
		"--",
		"sh", "-c", `test "$HOSTMUX_URL" = "https://backend.example.com"`,
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
}

func TestRunCommandHostmuxURLIncludesDaemonPort(t *testing.T) {
	_, code, stderr := runRunCommandAndCapture(t, runServerScript{
		domain:     "example.com",
		publicPort: 8443,
	}, []string{
		"--name", "api",
		"--",
		"sh", "-c", `test "$HOSTMUX_URL" = "https://api.example.com:8443"`,
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
}

func TestRunCommandRejectsEmptyExplicitName(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{}, []string{
		"--name=",
		"--",
		"hostname",
	})
	if code == 0 {
		t.Fatalf("run command exit code = %d, want non-zero", code)
	}
	if len(hosts) != 0 {
		t.Fatalf("registered hosts = %v, want none", hosts)
	}
	if got := stderr; !bytes.Contains([]byte(got), []byte("--name must be non-empty")) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunCommandRejectsMixedValidAndEmptyExplicitNames(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{}, []string{
		"--name", "backend",
		"--name=",
		"--",
		"hostname",
	})
	if code == 0 {
		t.Fatalf("run command exit code = %d, want non-zero", code)
	}
	if len(hosts) != 0 {
		t.Fatalf("registered hosts = %v, want none", hosts)
	}
	if got := stderr; !bytes.Contains([]byte(got), []byte("--name must be non-empty")) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunCommandRejectsInvalidExplicitName(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{}, []string{
		"--name", "My App",
		"--",
		"hostname",
	})
	if code == 0 {
		t.Fatalf("run command exit code = %d, want non-zero", code)
	}
	if len(hosts) != 0 {
		t.Fatalf("registered hosts = %v, want none", hosts)
	}
	if got := stderr; !bytes.Contains([]byte(got), []byte("valid bare label, hostname, or IP literal")) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunCommandRejectsExplicitNameWithPort(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{}, []string{
		"--name", "api:8080",
		"--",
		"hostname",
	})
	if code == 0 {
		t.Fatalf("run command exit code = %d, want non-zero", code)
	}
	if len(hosts) != 0 {
		t.Fatalf("registered hosts = %v, want none", hosts)
	}
	if got := stderr; !bytes.Contains([]byte(got), []byte("valid bare label, hostname, or IP literal")) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunCommandRejectsExplicitNameWithSurroundingSpaces(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{}, []string{
		"--name", " api ",
		"--",
		"hostname",
	})
	if code == 0 {
		t.Fatalf("run command exit code = %d, want non-zero", code)
	}
	if len(hosts) != 0 {
		t.Fatalf("registered hosts = %v, want none", hosts)
	}
	if got := stderr; !bytes.Contains([]byte(got), []byte("valid bare label, hostname, or IP literal")) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunCommandPassesThroughBareHostWhenNoDomainAvailable(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{}, []string{
		"--name", "api",
		"--",
		"hostname",
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"api"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
}

func TestRunCommandFallsBackWhenDaemonDoesNotSupportInfo(t *testing.T) {
	hosts, code, stderr := runRunCommandAndCapture(t, runServerScript{
		infoOk:    false,
		infoError: "unsupported operation",
	}, []string{
		"--name", "api",
		"--",
		"sh", "-c", `[ -z "${HOSTMUX_URL}" ]`,
	})
	if code != 0 {
		t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
	}
	want := []string{"api"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("registered hosts = %v, want %v", hosts, want)
	}
	if got := stderr; got == "" || !bytes.Contains([]byte(got), []byte("using bare hosts unchanged")) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunCommandHostmuxURLSchemeMatchesDaemonEdge(t *testing.T) {
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
			_, code, stderr := runRunCommandAndCapture(t, runServerScript{
				domain:    "example.com",
				plainEdge: tt.plainEdge,
			}, []string{
				"--name", "api",
				"--",
				"sh", "-c", `test "$HOSTMUX_URL" = "` + tt.wantURL + `"`,
			})
			if code != 0 {
				t.Fatalf("run command exit code = %d, stderr = %q", code, stderr)
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
	// publicPort is what the fake daemon reports as public_port in its
	// OpInfo response. Zero means "unset" — both old daemons that predate
	// the field and new daemons listening on the scheme default.
	publicPort int
}

func runRunCommandAndCapture(t *testing.T, script runServerScript, args []string) ([]string, int, string) {
	t.Helper()
	return runRunCommandAndCaptureInDir(t, t.TempDir(), script, args)
}

func runRunCommandAndCaptureInDir(t *testing.T, wd string, script runServerScript, args []string) ([]string, int, string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(wd); err != nil {
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
					msg.PublicPort = script.publicPort
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

	stderr, restoreStderr := captureRootFileOutput(t, &os.Stderr)

	cmd := newRunCmd()
	cmd.SetArgs(append([]string{"--socket", sockPath}, args...))
	err = cmd.Execute()
	code := 0
	if err != nil {
		var exitErr exitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("Execute() error = %T %v", err, err)
		}
		code = exitErr.code
		if exitErr.text != "" {
			_, _ = fmt.Fprintln(os.Stderr, exitErr.text)
		}
	}

	restoreStderr()

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

func TestResolveRunSocketPathPrefersLiveDiscoveryOverConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	sockDir, err := os.MkdirTemp("", "hm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	liveSock := filepath.Join(sockDir, "live.sock")
	ln, err := net.Listen("unix", liveSock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	hostmuxDir := filepath.Join(tmp, ".hostmux")
	if err := os.MkdirAll(hostmuxDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostmuxDir, "socket"), []byte(liveSock+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgDir := filepath.Join(tmp, ".config", "hostmux")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgBody := fmt.Sprintf("socket = %q\n", filepath.Join(tmp, "other.sock"))
	if err := os.WriteFile(filepath.Join(cfgDir, "hostmux.toml"), []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveRunSocketPath("")
	if err != nil {
		t.Fatalf("resolveRunSocketPath: %v", err)
	}
	if got != liveSock {
		t.Fatalf("got %q want live discovery %q", got, liveSock)
	}
}

func TestResolveRunSocketPathUsesConfigWhenNoLiveDiscovery(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOSTMUX_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	cfgDir := filepath.Join(tmp, ".config", "hostmux")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configSock := filepath.Join(tmp, "configured.sock")
	cfgBody := fmt.Sprintf("socket = %q\n", configSock)
	if err := os.WriteFile(filepath.Join(cfgDir, "hostmux.toml"), []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveRunSocketPath("")
	if err != nil {
		t.Fatalf("resolveRunSocketPath: %v", err)
	}
	if got != configSock {
		t.Fatalf("got %q want config socket %q", got, configSock)
	}
}

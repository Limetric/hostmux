package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestNewRootCmdRegistersRedesignedSubcommands(t *testing.T) {
	cmd := newRootCmd()

	want := []string{"start", "run", "url", "routes", "stop", "version"}
	for _, name := range want {
		if _, _, err := cmd.Find([]string{name}); err != nil {
			t.Fatalf("Find(%q) error = %v", name, err)
		}
	}

	if found, _, err := cmd.Find([]string{"serve"}); err == nil && found != cmd {
		t.Fatalf("Find(%q) resolved to %q, want root only", "serve", found.Name())
	}
}

func TestRootHelpShowsRedesignedCommands(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"help"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	help := stdout.String()
	for _, want := range []string{
		"start",
		"run",
		"url",
		"routes",
		"stop",
		"version",
		"hostmux run --name api --name admin -- COMMAND [ARGS...]",
		"hostmux url --name api",
		"hostmux start --foreground",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q\n%s", want, help)
		}
	}

	for _, unwanted := range []string{"serve", "get", "list"} {
		if strings.Contains(help, unwanted) {
			t.Fatalf("help output contains removed command %q\n%s", unwanted, help)
		}
	}
}

func TestVersionCommandPrintsBuildVersion(t *testing.T) {
	oldVersion := Version
	Version = "v1.2.3"
	t.Cleanup(func() { Version = oldVersion })

	cmd := newRootCmd()
	cmd.SetArgs([]string{"version"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got, want := stdout.String(), "hostmux v1.2.3\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExecuteUsageAndDispatchErrorsExitWithCode2(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{
			name:       "run missing command",
			args:       []string{"hostmux", "run"},
			wantStderr: "usage: hostmux run [--name NAME]...",
		},
		{
			name:       "run rejects positional host arg",
			args:       []string{"hostmux", "run", "api", "--", "bin/server"},
			wantStderr: "usage: hostmux run [--name NAME]...",
		},
		{
			name:       "url rejects positional host arg",
			args:       []string{"hostmux", "url", "api"},
			wantStderr: "usage: hostmux url [--name NAME]...",
		},
		{
			name:       "unknown subcommand",
			args:       []string{"hostmux", "nope"},
			wantStderr: "hostmux: unknown subcommand \"nope\"",
		},
		{
			name:       "start rejects stray positional arg as usage error",
			args:       []string{"hostmux", "start", "extra"},
			wantStderr: "usage: hostmux start [--config PATH] [--socket PATH] [--force] [--foreground]",
		},
		{
			name:       "version rejects stray positional arg as usage error",
			args:       []string{"hostmux", "version", "extra"},
			wantStderr: "usage: hostmux version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runExecuteAndCapture(t, tt.args)
			if code != 2 {
				t.Fatalf("execute() code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
			}
			if !bytes.Contains([]byte(stderr), []byte(tt.wantStderr)) {
				t.Fatalf("stderr = %q, want substring %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestExecuteRootVersionAliasesPrintBuildVersion(t *testing.T) {
	oldVersion := Version
	Version = "v1.2.3"
	t.Cleanup(func() { Version = oldVersion })

	for _, args := range [][]string{{"hostmux", "--version"}, {"hostmux", "-v"}} {
		t.Run(args[1], func(t *testing.T) {
			stdout, stderr, code := runExecuteAndCapture(t, args)
			if code != 0 {
				t.Fatalf("execute() code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
			}
			if got, want := stdout, "hostmux v1.2.3\n"; got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty stderr", stderr)
			}
		})
	}
}

func TestStartCommandPassesFlagsToRunner(t *testing.T) {
	oldRunner := startRunner
	t.Cleanup(func() { startRunner = oldRunner })

	called := false
	var got startOptions
	startRunner = func(opts startOptions) error {
		called = true
		got = opts
		return nil
	}

	cmd := newStartCmd()
	cmd.SetArgs([]string{
		"--config", "/tmp/hostmux.toml",
		"--socket", "/tmp/hostmux.sock",
		"--force",
		"--foreground",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("startRunner was not called")
	}

	want := startOptions{
		ConfigPath: "/tmp/hostmux.toml",
		SocketPath: "/tmp/hostmux.sock",
		Force:      true,
		Foreground: true,
	}
	if got != want {
		t.Fatalf("startRunner options = %#v, want %#v", got, want)
	}
}

func TestRoutesCommandRejectsPositionalArgs(t *testing.T) {
	cmd := newRoutesCmd()
	cmd.SetArgs([]string{"extra"})

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
	if got, want := exitErr.text, "usage: hostmux routes [--socket PATH]"; got != want {
		t.Fatalf("usage = %q, want %q", got, want)
	}
}

func TestStopCommandPassesSocketFlagToRunner(t *testing.T) {
	oldRunner := stopRunner
	t.Cleanup(func() { stopRunner = oldRunner })

	called := false
	var got stopOptions
	stopRunner = func(opts stopOptions) error {
		called = true
		got = opts
		return nil
	}

	cmd := newStopCmd()
	cmd.SetArgs([]string{"--socket", "/tmp/hostmux.sock"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("stopRunner was not called")
	}
	if got.SocketPath != "/tmp/hostmux.sock" {
		t.Fatalf("SocketPath = %q, want %q", got.SocketPath, "/tmp/hostmux.sock")
	}
}

func runExecuteAndCapture(t *testing.T, args []string) (string, string, int) {
	t.Helper()

	oldArgs := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = oldArgs })

	stdout, restoreStdout := captureRootFileOutput(t, &os.Stdout)
	stderr, restoreStderr := captureRootFileOutput(t, &os.Stderr)

	code := execute()

	restoreStdout()
	restoreStderr()

	return stdout.String(), stderr.String(), code
}

func captureRootFileOutput(t *testing.T, target **os.File) (*bytes.Buffer, func()) {
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
		*target = old
		<-done
		_ = r.Close()
	}
}

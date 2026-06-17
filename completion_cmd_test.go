package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompletionGeneratesForEachShell(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			root := newRootCmd()
			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetErr(&buf)
			root.SetArgs([]string{"completion", shell})
			if err := root.Execute(); err != nil {
				t.Fatalf("completion %s: %v", shell, err)
			}
			if buf.Len() == 0 {
				t.Fatalf("completion %s produced no output", shell)
			}
			// Generated scripts reference the binary name.
			if !strings.Contains(buf.String(), "hostmux") {
				t.Fatalf("completion %s output does not mention hostmux", shell)
			}
		})
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"completion", "tcsh"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unsupported shell")
	}
}

func TestCompletionRequiresArg(t *testing.T) {
	root := newRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"completion"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error when no shell is given")
	}
}

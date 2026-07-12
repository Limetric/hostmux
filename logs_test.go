package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogsPrintsWholeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.log")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runLogs(logsOptions{Writer: &buf, pathOverride: path}); err != nil {
		t.Fatalf("runLogs: %v", err)
	}
	if got := buf.String(); got != "alpha\nbeta\ngamma\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestLogsLastNLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.log")
	if err := os.WriteFile(path, []byte("l1\nl2\nl3\nl4\nl5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runLogs(logsOptions{Writer: &buf, pathOverride: path, Lines: 2}); err != nil {
		t.Fatalf("runLogs: %v", err)
	}
	if got := buf.String(); got != "l4\nl5\n" {
		t.Fatalf("output = %q, want last 2 lines", got)
	}
}

func TestLogsMissingFileErrors(t *testing.T) {
	var buf bytes.Buffer
	err := runLogs(logsOptions{Writer: &buf, pathOverride: filepath.Join(t.TempDir(), "nope.log")})
	if err == nil {
		t.Fatal("expected error for missing log file")
	}
	if !strings.Contains(err.Error(), "no daemon log yet") {
		t.Fatalf("error = %v", err)
	}
}

func TestLogsFollowStreamsAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hostmux.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("first\n"); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var buf bytes.Buffer
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runLogs(logsOptions{
			Writer:       &lockedWriter{mu: &mu, buf: &buf},
			pathOverride: path,
			Follow:       true,
			pollInterval: 10 * time.Millisecond,
			stop:         stop,
		})
	}()

	// Append after the initial read; follow should pick it up.
	time.Sleep(30 * time.Millisecond)
	if _, err := f.WriteString("second\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := buf.String()
		mu.Unlock()
		if strings.Contains(got, "first") && strings.Contains(got, "second") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	if err := <-done; err != nil {
		t.Fatalf("runLogs(follow): %v", err)
	}
	mu.Lock()
	got := buf.String()
	mu.Unlock()
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("follow output = %q, want both lines", got)
	}
}

type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

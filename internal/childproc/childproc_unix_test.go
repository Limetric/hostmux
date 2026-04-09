//go:build !windows

package childproc

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunPropagatesPortEnvAndExitCode(t *testing.T) {
	ctx := context.Background()
	port := 54321
	code, err := Run(ctx, RunOpts{
		Port:       port,
		Host:       "127.0.0.1",
		HostmuxURL: "http://app.test",
		Argv: []string{"sh", "-c",
			"test \"$PORT\" = \"54321\" && test \"$HOST\" = \"127.0.0.1\" && test \"$HOSTMUX_URL\" = \"http://app.test\""},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestRunDefaultHostIsLoopback(t *testing.T) {
	code, err := Run(context.Background(), RunOpts{
		Port: 54321,
		Argv: []string{"sh", "-c", `test "$HOST" = "127.0.0.1"`},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestRunReturnsNonZeroOnChildFailure(t *testing.T) {
	code, err := Run(context.Background(), RunOpts{
		Port: 1,
		Argv: []string{"sh", "-c", "exit 7"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 7 {
		t.Fatalf("code = %d, want 7", code)
	}
}

func TestRunCancelsChildOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, RunOpts{
			Port: 1,
			Argv: []string{"sh", "-c", "sleep 30"},
		})
		done <- err
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

package childproc

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestAllocateFreePortReturnsBindablePort(t *testing.T) {
	port, err := AllocateFreePort()
	if err != nil {
		t.Fatalf("AllocateFreePort: %v", err)
	}
	if port == 0 {
		t.Fatal("port = 0")
	}
	// Confirm the port is bindable now that the helper has released it.
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("could not bind reported free port %d: %v", port, err)
	}
	l.Close()
}

func TestRunPropagatesPortEnvAndExitCode(t *testing.T) {
	// `sh -c 'test "$PORT" = "54321"'` — verifies the env var was set and we
	// get a clean exit.
	ctx := context.Background()
	port := 54321 // deterministic
	code, err := Run(ctx, RunOpts{
		Port: port,
		Argv: []string{"sh", "-c", "test \"$PORT\" = \"54321\""},
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

func TestRunFailsOnMissingBinary(t *testing.T) {
	_, err := Run(context.Background(), RunOpts{
		Port: 1,
		Argv: []string{"this-binary-does-not-exist-xyz"},
	})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	// Make sure the type is a usable exec error so callers can format it.
	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Fatalf("expected *exec.Error in chain, got %T: %v", err, err)
	}
}

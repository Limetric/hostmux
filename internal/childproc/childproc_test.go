package childproc

import (
	"context"
	"errors"
	"net"
	"os/exec"
	"strconv"
	"testing"
)

func TestAllocateFreePortReturnsBindablePort(t *testing.T) {
	port, err := AllocateFreePort()
	if err != nil {
		t.Fatalf("AllocateFreePort: %v", err)
	}
	if port == 0 {
		t.Fatal("port = 0")
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("could not bind reported free port %d: %v", port, err)
	}
	l.Close()
}

func TestRunFailsOnMissingBinary(t *testing.T) {
	_, err := Run(context.Background(), RunOpts{
		Port: 1,
		Argv: []string{"this-binary-does-not-exist-xyz"},
	})
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Fatalf("expected *exec.Error in chain, got %T: %v", err, err)
	}
}

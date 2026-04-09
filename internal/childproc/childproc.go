// Package childproc spawns developer dev-server child processes on behalf of
// `hostmux run`. It allocates a free TCP port, injects PORT=, HOST=, and
// optionally HOSTMUX_URL= into the child environment, forwards signals to the
// child process, and returns the child's exit code.
package childproc

import (
	"errors"
	"fmt"
	"net"
)

// RunOpts configures a child-process run.
type RunOpts struct {
	// Port is set as the PORT env var passed to the child.
	Port int
	// Host is set as the HOST env var (listening address). Empty means
	// "127.0.0.1".
	Host string
	// HostmuxURL is set as HOSTMUX_URL when non-empty (public base URL of the
	// app, including scheme).
	HostmuxURL string
	// Argv is the command and arguments to execute (Argv[0] is looked up on PATH).
	Argv []string
	// ExtraEnv lists extra environment variables to pass to the child, on
	// top of the parent's environment and the injected variables above.
	ExtraEnv []string
}

// AllocateFreePort asks the kernel for an ephemeral TCP port and immediately
// releases it. There is a benign race here — between releasing and the child
// binding — but it is acceptable for a developer workflow.
func AllocateFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("childproc: listen: %w", err)
	}
	defer l.Close()
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return 0, errors.New("childproc: unexpected listener addr type")
	}
	return addr.Port, nil
}

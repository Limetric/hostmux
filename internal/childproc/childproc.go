// Package childproc spawns developer dev-server child processes on behalf of
// `hostmux run`. It allocates a free TCP port, injects PORT=, HOST=, and
// optionally HOSTMUX_URL= into the child environment, forwards
// SIGINT/SIGTERM to the child's process group, and returns the child's exit
// code.
package childproc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
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

// Run spawns the child, wires stdio to the calling process, forwards signals,
// and returns the child's exit code. Context cancellation kills the child
// process group with SIGTERM, waits for it to exit, and returns ctx.Err().
//
// Run is intended to be called at most once per process (matching the
// hostmux run workflow). Calling Run concurrently in the same process will
// deliver each incoming parent signal to every active child's process
// group, because Go's os/signal package fans out to all registered channels.
func Run(ctx context.Context, opts RunOpts) (int, error) {
	if len(opts.Argv) == 0 {
		return 0, errors.New("childproc: empty argv")
	}

	// Use plain exec.Command (not CommandContext) so we control the kill
	// ourselves. exec.CommandContext kills only the direct process, which
	// does not reach subprocesses when Setpgid is true.
	cmd := exec.Command(opts.Argv[0], opts.Argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	host := opts.Host
	if host == "" {
		host = "127.0.0.1"
	}
	env := append(os.Environ(),
		"PORT="+strconv.Itoa(opts.Port),
		"HOST="+host,
	)
	if opts.HostmuxURL != "" {
		env = append(env, "HOSTMUX_URL="+opts.HostmuxURL)
	}
	cmd.Env = append(env, opts.ExtraEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("childproc: start: %w", err)
	}

	// killGroup sends sig to the entire process group.
	killGroup := func(sig syscall.Signal) {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, sig)
		}
	}

	// Forward SIGINT/SIGTERM from the parent to the child's process group.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	doneCh := make(chan error, 1)
	go func() { doneCh <- cmd.Wait() }()

	for {
		select {
		case <-ctx.Done():
			// Kill the whole process group on context cancellation.
			killGroup(syscall.SIGTERM)
			// Drain doneCh; ignore the error — the child was killed by us.
			<-doneCh
			return 0, ctx.Err()
		case sig := <-sigCh:
			killGroup(sig.(syscall.Signal))
		case err := <-doneCh:
			if err == nil {
				return 0, nil
			}
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return ee.ExitCode(), nil
			}
			return 0, err
		}
	}
}

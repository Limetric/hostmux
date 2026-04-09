//go:build !windows

package childproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
)

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

	killGroup := func(sig syscall.Signal) {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, sig)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	doneCh := make(chan error, 1)
	go func() { doneCh <- cmd.Wait() }()

	for {
		select {
		case <-ctx.Done():
			killGroup(syscall.SIGTERM)
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

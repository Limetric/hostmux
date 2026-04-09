//go:build windows

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

// Run spawns the child, wires stdio to the calling process, and returns the
// child's exit code. On Windows, process group kill is not available; context
// cancellation kills the direct child process only.
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
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("childproc: start: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	doneCh := make(chan error, 1)
	go func() { doneCh <- cmd.Wait() }()

	for {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-doneCh
			return 0, ctx.Err()
		case <-sigCh:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
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

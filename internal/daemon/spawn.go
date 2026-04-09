// Package daemon contains the auto-spawn helper used by `hostmux run` when
// the Unix socket is missing. It forks `hostmux start --foreground` detached (its own
// session) so the daemon outlives the parent, redirects stdout/stderr to
// ~/.hostmux/hostmux.log, and polls until the socket accepts connections or
// the supplied context expires.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// DefaultEnsureTimeout is the suggested deadline for [EnsureRunning] when using
// the real detached spawn (TLS setup and disk can exceed a couple of seconds).
const DefaultEnsureTimeout = 8 * time.Second

// EnsureOpts allows tests to inject a fake Spawn function.
type EnsureOpts struct {
	// Spawn is called when the socket is missing or not accepting connections.
	// If nil, the real implementation forks `hostmux start --foreground` detached.
	Spawn func() error
}

// EnsureRunning blocks until a TCP-style connect to the Unix socket at sockPath
// succeeds (daemon is listening), or ctx expires. If the socket is not ready on
// entry, it calls opts.Spawn (or the real fork helper) once and then polls.
func EnsureRunning(ctx context.Context, sockPath string, opts EnsureOpts) error {
	if unixDialOK(sockPath) {
		return nil
	}
	spawn := opts.Spawn
	if spawn == nil {
		spawn = defaultSpawn
	}
	if err := spawn(); err != nil {
		return fmt.Errorf("daemon: spawn: %w", err)
	}
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		if unixDialOK(sockPath) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("daemon: timed out waiting for socket")
		case <-tick.C:
		}
	}
}

func unixDialOK(sockPath string) bool {
	conn, err := net.DialTimeout("unix", sockPath, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func defaultSpawn() error {
	return SpawnDetached("start", "--foreground")
}

// SpawnDetached forks `<self> <args...>` detached, with stdout/stderr to
// ~/.hostmux/hostmux.log.
func SpawnDetached(args ...string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	hostmuxDir := filepath.Join(home, ".hostmux")
	if err := os.MkdirAll(hostmuxDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(hostmuxDir, "hostmux.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(self, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	// The child inherited its own copy of the log file's fd via fork; the
	// parent's copy is no longer needed and would leak if left open.
	logFile.Close()
	// Detach: don't Wait, let the daemon outlive us.
	go func() { _ = cmd.Process.Release() }()
	return nil
}

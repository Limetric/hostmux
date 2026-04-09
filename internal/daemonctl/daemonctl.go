// Package daemonctl provides helpers to stop a running hostmux daemon.
// Both the `hostmux stop` subcommand and daemon takeover flows such as
// `hostmux start --force` route through Stop so their shutdown semantics
// stay identical.
package daemonctl

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Limetric/hostmux/internal/filelock"
	"github.com/Limetric/hostmux/internal/sockproto"
)

// probeFlock reports whether another process currently holds an exclusive
// flock on path. It attempts a non-blocking LOCK_EX, immediately releases
// on success, and returns held=true when the acquire would have blocked.
// The file is created if missing so that a fresh call on a path that has
// never had a daemon returns held=false cleanly.
func probeFlock(path string) (held bool, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return false, fmt.Errorf("open pid file: %w", err)
	}
	defer f.Close()
	held, err = filelock.TryLock(f)
	if err != nil {
		return false, fmt.Errorf("flock probe: %w", err)
	}
	if !held {
		_ = filelock.Unlock(f)
	}
	return held, nil
}

// waitForFlockRelease polls the PID-file flock every 20ms until it can be
// acquired (indicating the previous holder has exited) or until timeout
// elapses. Returns nil on successful acquire-and-release, or a timeout error.
func waitForFlockRelease(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		held, err := probeFlock(path)
		if err != nil {
			return err
		}
		if !held {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for pid file %s to be released", path)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// readPID reads and parses the PID written by the daemon into its PID file.
// The format is "<pid>\n" (see serve.go acquirePIDLock).
func readPID(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read pid file: %w", err)
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0, fmt.Errorf("pid file %s is empty", path)
	}
	pid, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse pid %q: %w", s, err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("invalid pid %d", pid)
	}
	return pid, nil
}

// StopOptions configures a Stop call.
type StopOptions struct {
	// SockPath is the Unix socket the daemon listens on. Stop tries to
	// dial this first for a graceful OpShutdown before falling back to
	// the PID-file kill path.
	SockPath string
	// PIDPath is the path to the PID file the daemon holds a flock on.
	// This is the authoritative "is a daemon running" signal.
	PIDPath string
	// GracefulTimeout is how long to wait after asking the daemon to
	// shut down before escalating. Defaults to 5 seconds.
	GracefulTimeout time.Duration
	// KillTimeout is how long to wait after the forceful kill before giving up.
	// Defaults to 2 seconds.
	KillTimeout time.Duration
}

// StopResult reports what happened in a Stop call.
type StopResult struct {
	// NotRunning is true when no daemon was holding the PID-file flock at
	// call time. In that case Stop is a successful no-op.
	NotRunning bool
	// UsedSocket is true when the graceful OpShutdown path succeeded.
	UsedSocket bool
	// UsedSIGKILL is true when the graceful path timed out and Stop had
	// to escalate to a forceful kill.
	UsedSIGKILL bool
}

// Stop attempts to gracefully shut down a hostmux daemon identified by the
// given socket + PID file paths. Returns a result describing the outcome.
// Returns nil error both when a daemon was successfully stopped AND when no
// daemon was running (check res.NotRunning to distinguish).
func Stop(opts StopOptions) (StopResult, error) {
	if opts.GracefulTimeout == 0 {
		opts.GracefulTimeout = 5 * time.Second
	}
	if opts.KillTimeout == 0 {
		opts.KillTimeout = 2 * time.Second
	}
	var res StopResult

	// 1. Probe the flock. If nobody holds it, there's no daemon.
	held, err := probeFlock(opts.PIDPath)
	if err != nil {
		return res, err
	}
	if !held {
		res.NotRunning = true
		return res, nil
	}

	// 2. Try the graceful socket path.
	if err := askSocketShutdown(opts.SockPath); err == nil {
		res.UsedSocket = true
	} else {
		// Fall through to the kill path.
		if killErr := killProcess(opts.PIDPath, true); killErr != nil {
			return res, fmt.Errorf("graceful shutdown failed (%v) and kill failed: %w", err, killErr)
		}
	}

	// 3. Wait for the daemon to release the flock.
	if err := waitForFlockRelease(opts.PIDPath, opts.GracefulTimeout); err == nil {
		return res, nil
	}

	// 4. Escalate to forceful kill.
	res.UsedSIGKILL = true
	if err := killProcess(opts.PIDPath, false); err != nil {
		return res, fmt.Errorf("forceful kill: %w", err)
	}
	if err := waitForFlockRelease(opts.PIDPath, opts.KillTimeout); err != nil {
		return res, fmt.Errorf("daemon did not exit even after forceful kill: %w", err)
	}
	return res, nil
}

// askSocketShutdown dials the Unix socket and sends an OpShutdown message,
// expecting an ok reply. Returns an error if the socket is missing, the
// dial fails, the write fails, or the daemon replies non-ok.
func askSocketShutdown(sockPath string) error {
	conn, err := net.DialTimeout("unix", sockPath, 500*time.Millisecond)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	enc := sockproto.NewEncoder(conn)
	dec := sockproto.NewDecoder(conn)
	if err := enc.Encode(&sockproto.Message{Op: sockproto.OpShutdown}); err != nil {
		return err
	}
	resp, err := dec.Decode()
	if err != nil {
		return err
	}
	if !resp.Ok {
		return fmt.Errorf("shutdown rejected: %s", resp.Error)
	}
	return nil
}

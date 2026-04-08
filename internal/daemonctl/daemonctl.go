// Package daemonctl provides helpers to stop a running hostmux daemon.
// Both the `hostmux stop` subcommand and `hostmux serve --force` route
// through Stop so their shutdown semantics stay identical.
package daemonctl

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) {
			return true, nil
		}
		return false, fmt.Errorf("flock probe: %w", err)
	}
	// Acquired successfully — nobody was holding it. Release immediately.
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return false, nil
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

// Package daemon contains the auto-spawn helper used by `hostmux run` when
// the Unix socket is missing. It forks `hostmux start --foreground` detached so
// the daemon outlives the parent, redirects stdout/stderr to
// ~/.hostmux/hostmux.log, and polls until the socket accepts connections or
// the supplied context expires.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// DefaultEnsureTimeout is the suggested deadline for [EnsureRunning] when using
// the real detached spawn. The child runs full daemon init (TLS, cert generation,
// listeners, control socket); keep this comfortably above slow disk or CPU, and
// avoid shrinking it for micro-optimization — dial readiness alone is usually fast.
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
//
// If sockPath is occupied by a stale socket file that still blocks bind(2), the
// spawned daemon may fail to listen (see logs); this path does not unlink or
// replace it — use `hostmux start --force` or remove the path manually.
func EnsureRunning(ctx context.Context, sockPath string, opts EnsureOpts) error {
	if unixDialOK(ctx, sockPath) {
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
		if unixDialOK(ctx, sockPath) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("daemon: timed out waiting for socket")
		case <-tick.C:
		}
	}
}

func unixDialOK(ctx context.Context, sockPath string) bool {
	d := net.Dialer{Timeout: 100 * time.Millisecond}
	conn, err := d.DialContext(ctx, "unix", sockPath)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func defaultSpawn() error {
	return SpawnDetached("start", "--foreground")
}

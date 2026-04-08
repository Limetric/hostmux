// Package sockpath resolves the Unix socket path used by the hostmux daemon
// and its clients. The resolution chain is shared between client commands
// (Resolve) and the daemon (ResolveServe); see the function docs for the
// exact order of precedence.
package sockpath

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Options bundles per-invocation overrides for path resolution.
type Options struct {
	// Flag is the value of an explicit --socket command-line flag.
	Flag string
	// ConfigSocket is the value of `socket = ...` in the daemon's TOML
	// config. Only consulted by ResolveServe.
	ConfigSocket string
}

// Resolve returns the Unix socket path for client commands (run, list).
// Resolution order: flag → $HOSTMUX_SOCKET → discovery file → XDG → default.
func Resolve(opts Options) (string, error) {
	if opts.Flag != "" {
		return opts.Flag, nil
	}
	if env := os.Getenv("HOSTMUX_SOCKET"); env != "" {
		return env, nil
	}
	if p, ok := readDiscovery(); ok {
		return p, nil
	}
	return defaultSocket()
}

// ResolveServe returns the Unix socket path for the daemon. Same chain as
// Resolve, but config-file-supplied socket is consulted between env and
// default (and the discovery file itself is irrelevant — the daemon IS
// the discovery file's writer).
func ResolveServe(opts Options) (string, error) {
	if opts.Flag != "" {
		return opts.Flag, nil
	}
	if env := os.Getenv("HOSTMUX_SOCKET"); env != "" {
		return env, nil
	}
	if opts.ConfigSocket != "" {
		return expandHome(opts.ConfigSocket)
	}
	return defaultSocket()
}

// IsExplicit reports whether the user explicitly named the socket path
// (via --socket or $HOSTMUX_SOCKET). Used by hostmux run to decide whether
// auto-spawn is permitted.
func IsExplicit(opts Options) bool {
	if opts.Flag != "" {
		return true
	}
	if os.Getenv("HOSTMUX_SOCKET") != "" {
		return true
	}
	return false
}

// WriteDiscovery writes the daemon-bound socket path to ~/.hostmux/socket so
// later client invocations can find it without flags.
func WriteDiscovery(path string) error {
	dir, err := hostmuxDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "socket"), []byte(path+"\n"), 0o644)
}

// RemoveDiscovery deletes the discovery file. Safe to call when missing.
func RemoveDiscovery() error {
	dir, err := hostmuxDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, "socket"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// DefaultSocket returns the well-known fallback socket path.
func DefaultSocket() (string, error) {
	return defaultSocket()
}

func defaultSocket() (string, error) {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, "hostmux.sock"), nil
	}
	dir, err := hostmuxDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hostmux.sock"), nil
}

func readDiscovery() (string, bool) {
	dir, err := hostmuxDir()
	if err != nil {
		return "", false
	}
	b, err := os.ReadFile(filepath.Join(dir, "socket"))
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", false
	}
	return s, true
}

func hostmuxDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".hostmux"), nil
}

func expandHome(p string) (string, error) {
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~/")), nil
}

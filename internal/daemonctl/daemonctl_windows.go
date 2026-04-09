//go:build windows

package daemonctl

import (
	"fmt"
	"os"
)

// killProcess terminates the process identified by the PID written in pidPath.
// On Windows, both graceful and forceful termination use TerminateProcess —
// there is no OS-level SIGTERM. The socket-based graceful shutdown path in
// Stop runs before this fallback.
func killProcess(pidPath string, _ bool) error {
	pid, err := readPID(pidPath)
	if err != nil {
		return err
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := p.Kill(); err != nil {
		return fmt.Errorf("kill %d: %w", pid, err)
	}
	return nil
}

//go:build !windows

package daemonctl

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// killProcess delivers a signal to the process identified by the PID written
// in pidPath. graceful=true sends SIGTERM; graceful=false sends SIGKILL.
func killProcess(pidPath string, graceful bool) error {
	pid, err := readPID(pidPath)
	if err != nil {
		return err
	}
	sig := unix.Signal(unix.SIGKILL)
	if graceful {
		sig = unix.SIGTERM
	}
	if err := unix.Kill(pid, sig); err != nil {
		return fmt.Errorf("kill %d: %w", pid, err)
	}
	return nil
}

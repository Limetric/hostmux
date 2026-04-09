//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// SpawnDetached forks `<self> <args...>` detached in its own session, with
// stdout/stderr appended to ~/.hostmux/hostmux.log. The parent does not wait
// for the child — it returns as soon as the child process is started.
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
	logFile.Close()
	go func() { _ = cmd.Process.Release() }()
	return nil
}

//go:build windows

package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// detachedProcess is the Windows DETACHED_PROCESS creation flag (0x00000008).
// It detaches the child from the parent's console so the daemon outlives the
// parent terminal session.
const detachedProcess = 0x00000008

// SpawnDetached forks `<self> <args...>` detached from the parent's console,
// with stdout/stderr appended to ~/.hostmux/hostmux.log.
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
	// DETACHED_PROCESS prevents the daemon from inheriting the parent's
	// console (equivalent to Setsid on Unix). CREATE_NEW_PROCESS_GROUP
	// assigns the daemon its own process group so Ctrl+C signals sent to
	// the parent terminal are not propagated to the daemon.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return err
	}
	logFile.Close()
	go func() { _ = cmd.Process.Release() }()
	return nil
}

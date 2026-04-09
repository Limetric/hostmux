//go:build !windows

package daemonctl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/Limetric/hostmux/internal/filelock"
)

func TestStopFallbackToSIGTERM(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "never.sock") // does not exist — forces kill path
	pidPath := filepath.Join(dir, "daemon.pid")

	// Launch `sleep 30` as our fake daemon.
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("sleep: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	// Write the child's PID to the file.
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Hold the flock on pidPath on behalf of the "daemon".
	holdF, err := os.OpenFile(pidPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if err := filelock.Lock(holdF); err != nil {
		holdF.Close()
		t.Fatalf("flock: %v", err)
	}

	// Watch for child exit and release the flock when it happens — mirrors
	// the real daemon's behavior where the kernel releases the flock on
	// process exit.
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		_ = filelock.Unlock(holdF)
		_ = holdF.Close()
		close(done)
	}()

	res, err := Stop(StopOptions{
		SockPath:        sock,
		PIDPath:         pidPath,
		GracefulTimeout: 3 * time.Second,
		KillTimeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if res.NotRunning {
		t.Fatal("expected NotRunning=false")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("child did not exit after Stop")
	}
}

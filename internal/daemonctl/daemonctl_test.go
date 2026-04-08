package daemonctl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// holdFlock opens path (creating if needed), takes an exclusive flock on it,
// and returns a release func that unlocks and closes. Fatal on failure.
func holdFlock(t *testing.T, path string) func() {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		t.Fatalf("flock: %v", err)
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}
}

func TestProbeFlockHeld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")
	release := holdFlock(t, path)
	defer release()

	held, err := probeFlock(path)
	if err != nil {
		t.Fatalf("probeFlock: %v", err)
	}
	if !held {
		t.Fatal("expected held=true when another fd holds the flock")
	}
}

func TestProbeFlockNotHeld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")
	// File doesn't exist yet — probe should create, acquire, release, report not held.
	held, err := probeFlock(path)
	if err != nil {
		t.Fatalf("probeFlock: %v", err)
	}
	if held {
		t.Fatal("expected held=false on empty path")
	}
	// After the probe, we should still be able to hold our own flock on it.
	release := holdFlock(t, path)
	release()
}

func TestWaitForFlockReleaseWithinTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")
	release := holdFlock(t, path)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		release()
	}()
	start := time.Now()
	if err := waitForFlockRelease(path, time.Second); err != nil {
		t.Fatalf("waitForFlockRelease: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 80*time.Millisecond {
		t.Fatalf("returned too quickly: %v", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("returned too slowly: %v", elapsed)
	}
	wg.Wait()
}

func TestWaitForFlockReleaseTimeout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")
	release := holdFlock(t, path)
	defer release()

	start := time.Now()
	err := waitForFlockRelease(path, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	elapsed := time.Since(start)
	if elapsed < 90*time.Millisecond {
		t.Fatalf("returned before timeout: %v", elapsed)
	}
}

func TestReadPID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")
	if err := os.WriteFile(path, []byte("12345\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pid, err := readPID(path)
	if err != nil {
		t.Fatalf("readPID: %v", err)
	}
	if pid != 12345 {
		t.Fatalf("pid = %d, want 12345", pid)
	}
}

func TestReadPIDGarbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")
	if err := os.WriteFile(path, []byte("not-a-number\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPID(path); err == nil {
		t.Fatal("expected error parsing garbage PID, got nil")
	}
}

func TestReadPIDMissingFile(t *testing.T) {
	_, err := readPID(filepath.Join(t.TempDir(), "nope.pid"))
	if err == nil {
		t.Fatal("expected error reading missing file")
	}
}

func TestStopNoDaemon(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "hostmux.sock")
	pid := filepath.Join(dir, "hostmux.pid")
	// No daemon running → Stop should return nil (no-op) and report NotRunning.
	res, err := Stop(StopOptions{
		SockPath:        sock,
		PIDPath:         pid,
		GracefulTimeout: 200 * time.Millisecond,
		KillTimeout:     200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !res.NotRunning {
		t.Fatalf("expected NotRunning=true, got %+v", res)
	}
}

func TestStopFallbackToSIGTERM(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "never.sock") // does not exist — forces PID-file path
	pidPath := filepath.Join(dir, "daemon.pid")

	// Launch `sleep 30` as our fake daemon. Stop will read its PID from the
	// file and SIGTERM it. We simulate the daemon's flock-lifetime by holding
	// an flock on pidPath from a goroutine that releases when the child exits.
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
	if err := unix.Flock(int(holdF.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		holdF.Close()
		t.Fatalf("flock: %v", err)
	}

	// Watch for child exit and release the flock when it happens — mirrors
	// the real daemon's behavior where the kernel releases the flock on
	// process exit.
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		_ = unix.Flock(int(holdF.Fd()), unix.LOCK_UN)
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

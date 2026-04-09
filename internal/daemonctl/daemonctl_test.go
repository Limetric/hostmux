package daemonctl

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Limetric/hostmux/internal/filelock"
)

// holdFlock opens path (creating if needed), acquires an exclusive lock on it,
// and returns a release func that unlocks and closes. Fatal on failure.
func holdFlock(t *testing.T, path string) func() {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := filelock.Lock(f); err != nil {
		f.Close()
		t.Fatalf("flock: %v", err)
	}
	return func() {
		_ = filelock.Unlock(f)
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
	held, err := probeFlock(path)
	if err != nil {
		t.Fatalf("probeFlock: %v", err)
	}
	if held {
		t.Fatal("expected held=false on empty path")
	}
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

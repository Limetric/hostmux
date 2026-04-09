package filelock_test

import (
	"os"
	"testing"

	"github.com/Limetric/hostmux/internal/filelock"
)

func TestLockUnlock(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "flock*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := filelock.Lock(f); err != nil {
		t.Fatalf("Lock: %v", err)
	}
	if err := filelock.Unlock(f); err != nil {
		t.Fatalf("Unlock after Lock: %v", err)
	}
}

func TestTryLockAcquireRelease(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "flock*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	held, err := filelock.TryLock(f)
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if held {
		t.Fatal("TryLock on unheld file: got held=true, want held=false")
	}
	if err := filelock.Unlock(f); err != nil {
		t.Fatalf("Unlock after TryLock: %v", err)
	}
}

// TestTryLockConflict verifies that TryLock returns held=true when another
// file descriptor already holds an exclusive lock on the same file.
func TestTryLockConflict(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/conflict.lock"

	// First descriptor acquires the lock.
	f1, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f1.Close()
	if err := filelock.Lock(f1); err != nil {
		t.Fatalf("Lock on f1: %v", err)
	}

	// Second descriptor on the same file must observe contention.
	f2, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	held, err := filelock.TryLock(f2)
	if err != nil {
		t.Fatalf("TryLock on f2: %v", err)
	}
	if !held {
		t.Fatal("TryLock on contested file: got held=false, want held=true")
	}

	// Release f1; f2 should now be acquirable.
	if err := filelock.Unlock(f1); err != nil {
		t.Fatalf("Unlock f1: %v", err)
	}
	held, err = filelock.TryLock(f2)
	if err != nil {
		t.Fatalf("TryLock on f2 after release: %v", err)
	}
	if held {
		t.Fatal("TryLock after release: got held=true, want held=false")
	}
	_ = filelock.Unlock(f2)
}

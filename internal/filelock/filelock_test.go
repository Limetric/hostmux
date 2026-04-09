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

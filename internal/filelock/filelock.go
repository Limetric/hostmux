// Package filelock provides cross-platform advisory file locking.
// It abstracts flock(2) on Unix and LockFileEx on Windows behind a
// uniform three-function API.
package filelock

import "os"

// Lock acquires an exclusive advisory lock on f. It blocks until the lock
// is acquired or an error occurs.
func Lock(f *os.File) error {
	return lock(f)
}

// TryLock attempts to acquire a non-blocking exclusive advisory lock on f.
// It returns held=true when another process holds the lock (this is not an
// error condition — the caller should retry or back off). Any other failure
// returns a non-nil err.
func TryLock(f *os.File) (held bool, err error) {
	return tryLock(f)
}

// Unlock releases a previously acquired lock on f.
func Unlock(f *os.File) error {
	return unlock(f)
}

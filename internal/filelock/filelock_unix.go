//go:build !windows

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX)
}

func tryLock(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return false, nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) {
		return true, nil
	}
	return false, err
}

func unlock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}

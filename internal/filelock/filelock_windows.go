//go:build windows

package filelock

import (
	"os"

	"golang.org/x/sys/windows"
)

func lock(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0, 1, 0, ol,
	)
}

func tryLock(f *os.File) (bool, error) {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, ol,
	)
	if err == nil {
		return false, nil
	}
	if err == windows.ERROR_LOCK_VIOLATION {
		return true, nil
	}
	return false, err
}

func unlock(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}

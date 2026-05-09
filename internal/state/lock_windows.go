//go:build windows

package state

import "golang.org/x/sys/windows"

// lockFile takes an exclusive lock on fd via LockFileEx. Mirrors the
// flock(LOCK_EX) shape used on unix — blocks until the lock is
// available.
func lockFile(fd uintptr) error {
	ol := new(windows.Overlapped)
	return windows.LockFileEx(windows.Handle(fd), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol)
}

func unlockFile(fd uintptr) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(fd), 0, 1, 0, ol)
}

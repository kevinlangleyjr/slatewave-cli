//go:build !windows

package state

import "golang.org/x/sys/unix"

// lockFile takes an exclusive advisory flock on fd. Blocks until the
// lock is available — typical hold times are sub-millisecond (load,
// in-memory mutate, save), so callers don't need a timeout.
func lockFile(fd uintptr) error {
	return unix.Flock(int(fd), unix.LOCK_EX)
}

func unlockFile(fd uintptr) error {
	return unix.Flock(int(fd), unix.LOCK_UN)
}

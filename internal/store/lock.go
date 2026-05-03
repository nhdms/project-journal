//go:build unix

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// withLock acquires an exclusive advisory lock on <dataDir>/.lock, runs fn,
// then releases the lock. Uses syscall.Flock (POSIX advisory lock) which is
// inherited across goroutines within a process and respected across processes
// on the same host. Safe for concurrent pj invocations (e.g. hook + CLI).
func withLock(dataDir string, fn func() error) error {
	lockPath := filepath.Join(dataDir, ".lock")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("lock: mkdir %s: %w", dataDir, err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("lock: open %s: %w", lockPath, err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock: flock %s: %w", lockPath, err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	return fn()
}

// ErrFinishAlreadyRunning is returned by WithFinishLock when another process
// is already running `pj finish` for the same task.
type ErrFinishAlreadyRunning struct{ TaskID string }

func (e ErrFinishAlreadyRunning) Error() string {
	return fmt.Sprintf("pj finish already running for task %s", e.TaskID)
}

// WithFinishLock acquires a non-blocking exclusive lock on
// <dataDir>/locks/finish-<taskID>.lock and runs fn. Returns
// ErrFinishAlreadyRunning immediately (without blocking) if another process
// holds the lock. This prevents two concurrent `pj finish --auto` invocations
// (e.g. from overlapping stop hook calls) from racing on the same task.
func WithFinishLock(dataDir, taskID string, fn func() error) error {
	locksDir := filepath.Join(dataDir, "locks")
	if err := os.MkdirAll(locksDir, 0o755); err != nil {
		return fmt.Errorf("finish lock: mkdir %s: %w", locksDir, err)
	}
	lockPath := filepath.Join(locksDir, "finish-"+taskID+".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("finish lock: open %s: %w", lockPath, err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if err == syscall.EWOULDBLOCK {
			return ErrFinishAlreadyRunning{TaskID: taskID}
		}
		return fmt.Errorf("finish lock: flock %s: %w", lockPath, err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	return fn()
}

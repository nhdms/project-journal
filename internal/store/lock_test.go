//go:build unix

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestWithLockNoConcurrentCorruption spawns N goroutines each doing a
// read-increment-write of a counter file inside withLock. Without the lock,
// lost updates would cause the final value to be less than N*iters.
func TestWithLockNoConcurrentCorruption(t *testing.T) {
	dir := t.TempDir()
	counterFile := filepath.Join(dir, "counter")
	if err := os.WriteFile(counterFile, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}

	const goroutines = 10
	const iters = 20

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				err := withLock(dir, func() error {
					data, err := os.ReadFile(counterFile)
					if err != nil {
						return err
					}
					n, err := strconv.Atoi(strings.TrimSpace(string(data)))
					if err != nil {
						return fmt.Errorf("parse counter: %w", err)
					}
					n++
					return os.WriteFile(counterFile, []byte(strconv.Itoa(n)), 0o644)
				})
				if err != nil {
					t.Errorf("withLock: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatal(err)
	}
	got, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse final counter: %v", err)
	}
	want := goroutines * iters
	if got != want {
		t.Errorf("counter = %d, want %d (lost %d updates)", got, want, want-got)
	}
}

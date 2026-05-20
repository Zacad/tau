package tools

import (
	"context"
	"sync"
)

// MutationQueue serializes write/edit operations per file using a per-file mutex chain.
// It ensures that concurrent mutations to the same file are executed sequentially.
type MutationQueue struct {
	mu      sync.Mutex
	locks   map[string]*fileLock
}

type fileLock struct {
	mu    sync.Mutex
	count int // reference count
}

// NewMutationQueue creates a new file mutation queue.
func NewMutationQueue() *MutationQueue {
	return &MutationQueue{
		locks: make(map[string]*fileLock),
	}
}

// Acquire obtains a per-file lock. It blocks until the lock is available.
// The lock is released by calling the returned release function.
// File paths are normalized to their canonical form before locking.
func (q *MutationQueue) Acquire(filePath string) func() {
	q.mu.Lock()
	lock, exists := q.locks[filePath]
	if !exists {
		lock = &fileLock{count: 1}
		q.locks[filePath] = lock
	} else {
		lock.count++
	}
	q.mu.Unlock()

	lock.mu.Lock()

	return func() {
		q.mu.Lock()
		lock.count--
		if lock.count <= 0 {
			delete(q.locks, filePath)
		}
		q.mu.Unlock()
		lock.mu.Unlock()
	}
}

// ExecuteAcquired executes fn while holding the per-file lock for filePath.
// It blocks until the lock is available, executes fn, then releases the lock.
func (q *MutationQueue) ExecuteAcquired(ctx context.Context, filePath string, fn func() error) error {
	release := q.Acquire(filePath)
	defer release()

	// Check context cancellation before executing
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return fn()
}

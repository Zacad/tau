package tools

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMutationQueue_SerializesAccess(t *testing.T) {
	q := NewMutationQueue()

	var order []int
	var mu sync.Mutex
	filePath := "/tmp/test.txt"
	done := make(chan bool)

	// Launch 5 concurrent operations on the same file
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := q.ExecuteAcquired(context.Background(), filePath, func() error {
				// Simulate work
				time.Sleep(10 * time.Millisecond)
				mu.Lock()
				order = append(order, id)
				mu.Unlock()
				return nil
			})
			if err != nil {
				t.Errorf("ExecuteAcquired error: %v", err)
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for operations to complete")
	}

	// All operations should have completed
	if len(order) != 5 {
		t.Fatalf("expected 5 operations, got %d", len(order))
	}
}

func TestMutationQueue_DifferentFilesConcurrent(t *testing.T) {
	q := NewMutationQueue()

	var mu sync.Mutex
	executions := make(map[string]int)

	var wg sync.WaitGroup
	files := []string{"/tmp/a.txt", "/tmp/b.txt", "/tmp/c.txt"}
	for _, f := range files {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			_ = q.ExecuteAcquired(context.Background(), path, func() error {
				time.Sleep(20 * time.Millisecond)
				mu.Lock()
				executions[path]++
				mu.Unlock()
				return nil
			})
		}(f)
	}

	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	for _, f := range files {
		if executions[f] != 1 {
			t.Errorf("expected 1 execution for %s, got %d", f, executions[f])
		}
	}
}

func TestMutationQueue_ContextCancellation(t *testing.T) {
	q := NewMutationQueue()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := q.ExecuteAcquired(ctx, "/tmp/test.txt", func() error {
		return nil
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestMutationQueue_Release(t *testing.T) {
	q := NewMutationQueue()

	// Acquire and release manually
	release := q.Acquire("/tmp/test.txt")
	release()

	// Should be able to acquire again
	release2 := q.Acquire("/tmp/test.txt")
	release2()
}

func TestMutationQueue_ConcurrentSameFile(t *testing.T) {
	q := NewMutationQueue()

	var counter int
	var mu sync.Mutex
	filePath := "/tmp/same.txt"

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = q.ExecuteAcquired(context.Background(), filePath, func() error {
				mu.Lock()
				counter++
				mu.Unlock()
				time.Sleep(5 * time.Millisecond)
				return nil
			})
		}()
	}

	wg.Wait()

	if counter != 10 {
		t.Errorf("expected 10 increments, got %d", counter)
	}
}

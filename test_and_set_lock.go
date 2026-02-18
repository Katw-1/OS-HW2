package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// waiter represents a blocked thread/goroutine waiting for the lock.
type waiter struct {
	ch chan struct{} // used to park/unpark
}

// lock_t is the Go equivalent of the C struct:
//
//	typedef struct __lock_t {
//	  int flag;
//	  int guard;
//	  queue_t *q;
//	} lock_t;
type lock_t struct {
	flag  int32 // 0 = unlocked, 1 = locked
	guard int32 // spinlock protecting flag + queue

	// FIFO queue of waiters
	q []*waiter
}

// lock_init(lock_t *m)
func lock_init(m *lock_t) {
	atomic.StoreInt32(&m.flag, 0)
	atomic.StoreInt32(&m.guard, 0)
	m.q = nil
}

// TestAndSet(&m->guard, 1) == 1
// In Go we implement as CAS in a spin loop.
func acquire_guard(m *lock_t) {
	for !atomic.CompareAndSwapInt32(&m.guard, 0, 1) {
		// spin (yield helps keep the runtime responsive)
		runtime.Gosched()
	}
}

func release_guard(m *lock_t) {
	atomic.StoreInt32(&m.guard, 0)
}

// queue_add(m->q, gettid())
func queue_add(m *lock_t, w *waiter) {
	m.q = append(m.q, w)
}

// queue_empty(m->q)
func queue_empty(m *lock_t) bool {
	return len(m.q) == 0
}

// queue_remove(m->q)  (FIFO)
func queue_remove(m *lock_t) *waiter {
	w := m.q[0]
	m.q = m.q[1:]
	return w
}

// park(): block current goroutine until unpark() signals it
func park(w *waiter) {
	<-w.ch
}

// unpark(tid): wake specific waiter
func unpark(w *waiter) {
	// Non-blocking send is safer (prevents “lost wakeup”-style issues).
	// Channel is buffered size 1, so one wakeup can be “remembered”.
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

// lock(lock_t *m)
func lock(m *lock_t) {
	acquire_guard(m)

	if atomic.LoadInt32(&m.flag) == 0 {
		// lock is free, take it
		atomic.StoreInt32(&m.flag, 1)
		release_guard(m)
		return
	}

	// lock is held: enqueue and sleep
	w := &waiter{ch: make(chan struct{}, 1)}
	queue_add(m, w)
	release_guard(m)
	park(w)

	// When we wake, we "own" the lock (handoff semantics like the figure).
}

// unlock(lock_t *m)
func unlock(m *lock_t) {
	acquire_guard(m)

	if queue_empty(m) {
		// no waiters: release lock
		atomic.StoreInt32(&m.flag, 0)
	} else {
		// hand off lock directly to next waiter
		next := queue_remove(m)
		unpark(next)

		// Note: we intentionally keep flag = 1 (lock stays "held"),
		// but ownership transfers to the awakened waiter.
	}

	release_guard(m)
}

func main() {
	var m lock_t
	lock_init(&m)

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: grab lock first and hold it
	go func() {
		defer wg.Done()

		lock(&m)
		fmt.Println("Thread 1 acquired lock")

		time.Sleep(2 * time.Second) // hold lock so Thread 2 must wait

		unlock(&m)
		fmt.Println("Thread 1 released lock")
	}()

	// Small delay so Thread 1 likely acquires first
	time.Sleep(100 * time.Millisecond)

	// Goroutine 2: measure how long it waits to acquire
	go func() {
		defer wg.Done()

		start := time.Now()
		lock(&m) // blocks (parks) until Thread 1 unlocks & unparks it
		wait := time.Since(start)

		fmt.Println("Thread 2 acquired lock")
		fmt.Printf("Thread 2 wait time: %v\n", wait)

		unlock(&m)
	}()

	wg.Wait()
}

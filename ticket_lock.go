package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type lock_t struct {
	ticket int32
	turn   int32
}

// Equivalent of lock_init()
func lock_init(lock *lock_t) {
	atomic.StoreInt32(&lock.ticket, 0)
	atomic.StoreInt32(&lock.turn, 0)
}

// Equivalent of lock()
func lock(lock *lock_t) {
	// FetchAndAdd: atomic.Add returns NEW value, so subtract 1
	myturn := atomic.AddInt32(&lock.ticket, 1) - 1

	// Spin while it's not our turn
	for atomic.LoadInt32(&lock.turn) != myturn {
		// busy wait
	}
}

// Equivalent of unlock()
func unlock(lock *lock_t) {
	atomic.AddInt32(&lock.turn, 1)
}

func main() {

	var m lock_t //Create the lcok
	lock_init(&m) //Labels flag as available

	var wg sync.WaitGroup
	wg.Add(2)

	// Thread 1
	go func() {
		defer wg.Done()

		lock(&m)
		fmt.Println("Thread 1 acquired lock")

		// Hold lock so thread 2 must wait
		time.Sleep(2 * time.Second)

		unlock(&m)
		fmt.Println("Thread 1 released lock")
	}()

	// Small delay to ensure thread 1 gets lock first
	time.Sleep(100 * time.Millisecond)

	// Thread 2
	go func() {
		defer wg.Done()

		start := time.Now()

		lock(&m) // this will spin until thread 1 unlocks

		waitTime := time.Since(start)

		fmt.Println("Thread 2 acquired lock")
		fmt.Println("Thread 2 waited:", waitTime)

		unlock(&m)
	}()

	wg.Wait()
}

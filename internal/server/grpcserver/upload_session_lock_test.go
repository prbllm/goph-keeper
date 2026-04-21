package grpcserver

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUploadSessionLocks_acquire_serializesSameKey(t *testing.T) {
	t.Parallel()
	const n = 48
	var l uploadSessionLocks
	key := uploadLockKey("user-1", "session-1")
	var wg sync.WaitGroup
	var inside int32
	var broken int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := l.acquire(key)
			v := atomic.AddInt32(&inside, 1)
			if v != 1 {
				atomic.StoreInt32(&broken, 1)
			}
			runtime.Gosched()
			time.Sleep(time.Microsecond * 50)
			atomic.AddInt32(&inside, -1)
			release()
		}()
	}
	wg.Wait()
	if atomic.LoadInt32(&broken) != 0 {
		t.Fatal("mutual exclusion broken: more than one goroutine in critical section for same key")
	}
}

func TestUploadSessionLocks_differentKeysAllowParallel(t *testing.T) {
	t.Parallel()
	var l uploadSessionLocks
	keyA := uploadLockKey("u", "a")
	keyB := uploadLockKey("u", "b")
	start := make(chan struct{})
	var inside int32
	var sawTwo int32
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		rel := l.acquire(keyA)
		if v := atomic.AddInt32(&inside, 1); v == 2 {
			atomic.StoreInt32(&sawTwo, 1)
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inside, -1)
		rel()
	}()
	go func() {
		defer wg.Done()
		<-start
		rel := l.acquire(keyB)
		if v := atomic.AddInt32(&inside, 1); v == 2 {
			atomic.StoreInt32(&sawTwo, 1)
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inside, -1)
		rel()
	}()
	close(start)
	wg.Wait()
	if atomic.LoadInt32(&sawTwo) == 0 {
		t.Fatal("expected two different keys to be locked concurrently at some point")
	}
}

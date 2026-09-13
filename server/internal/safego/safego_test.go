package safego

import (
	"net/http"
	"sync"
	"testing"
)

func TestRecoverLogsAndReturns(t *testing.T) {
	// Recover should not propagate the panic.
	var wg sync.WaitGroup
	wg.Add(1)
	survived := false
	go func() {
		defer wg.Done()
		defer func() { survived = true }()
		defer Recover("test")
		panic("boom")
	}()
	wg.Wait()
	if !survived {
		t.Fatal("Recover did not catch the panic")
	}
}

func TestRecoverRepanicsErrAbortHandler(t *testing.T) {
	caught := false
	func() {
		defer func() {
			if r := recover(); r == http.ErrAbortHandler {
				caught = true
			}
		}()
		defer Recover("test-abort")
		panic(http.ErrAbortHandler)
	}()
	if !caught {
		t.Fatal("Recover should re-panic http.ErrAbortHandler")
	}
}

func TestGoRecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan struct{})
	Go("test-go", func() {
		defer wg.Done()
		defer close(done)
		panic("go boom")
	})
	wg.Wait()
	<-done // ensures the goroutine completed
}

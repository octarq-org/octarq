package safego

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestRecoverLogsAndReturns(t *testing.T) {
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
	<-done
}

func TestRecoverHandler(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("http handler boom")
	})
	safeHandler := RecoverHandler("test-handler", handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic
	safeHandler.ServeHTTP(rec, req)
}

func TestRecoverHandlerRepanicsErrAbortHandler(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	})
	safeHandler := RecoverHandler("test-handler-abort", handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	caught := false
	func() {
		defer func() {
			if r := recover(); r == http.ErrAbortHandler {
				caught = true
			}
		}()
		safeHandler.ServeHTTP(rec, req)
	}()
	if !caught {
		t.Fatal("RecoverHandler should re-panic http.ErrAbortHandler")
	}
}

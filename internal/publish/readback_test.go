package publish

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadBackWaitsForEdgePropagation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			w.WriteHeader(404)
		case 2:
			_, _ = w.Write([]byte(`{"version":"old"}`))
		default:
			_, _ = w.Write([]byte(`{"version":"current"}`))
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := verifyReadBackWithRetry(ctx, server.URL, []byte(`{"version":"current"}`), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal(calls.Load())
	}
}

func TestReadBackRetryIsBoundedAndCancelable(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = w.Write([]byte(`old`)) }))
	defer server.Close()
	if err := verifyReadBackWithRetry(context.Background(), server.URL, []byte("new"), time.Millisecond); err == nil {
		t.Fatal("accepted stale asset")
	}
	if calls.Load() != 6 {
		t.Fatal("unbounded retry", calls.Load())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := verifyReadBackWithRetry(ctx, server.URL, []byte("new"), time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls.Load() != 6 {
		t.Fatal("request after cancellation")
	}
}

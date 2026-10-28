package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"ap-cloud-server/internal/config"
)

func TestRateLimitIsolatesTrafficBehindProxy(t *testing.T) {
	s := &Server{limiter: newRateLimiter(0.001, 1)}
	handler := s.rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	// All traffic uses the proxy's peer address. Each category has a separate
	// budget; changing endpoints within the category must not reset it.
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/api/v2/get/research", http.StatusNoContent},
		{"/api/v2/get/drop", http.StatusTooManyRequests},
		{"/internal/v1/statistics/results", http.StatusNoContent},
		{"/internal/v1/worker/sync", http.StatusTooManyRequests},
		{"/api/v1/admin/statistics/overview", http.StatusNoContent},
		{"/admin/assets/app.js", http.StatusTooManyRequests},
		{"/healthz", http.StatusNoContent},
		{"/healthz", http.StatusNoContent},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = "172.18.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.path, rec.Code, tc.want)
		}
		if rec.Code == http.StatusTooManyRequests && rec.Header().Get("Retry-After") != "1" {
			t.Fatalf("%s: missing retry hint", tc.path)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/get/research", nil)
	req.RemoteAddr = "192.0.2.1:4321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("another peer shared exhausted budget: %d", rec.Code)
	}
}

func TestHealthCheckDoesNotConsumePublicBudget(t *testing.T) {
	s := &Server{limiter: newRateLimiter(0.001, 1)}
	handler := s.rateLimitMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/healthz", http.StatusNoContent},
		{http.MethodHead, "/healthz", http.StatusNoContent},
		{http.MethodGet, "/api/v2/get/research", http.StatusNoContent},
		{http.MethodGet, "/api/v2/get/drop", http.StatusTooManyRequests},
		{http.MethodPost, "/healthz", http.StatusTooManyRequests},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Fatalf("%s %s: got %d, want %d", tc.method, tc.path, rec.Code, tc.want)
		}
	}
}

func TestRateLimitConcurrentBurst(t *testing.T) {
	// Keep refills below one token during the test to verify exact burst
	// accounting under contention, independent of machine speed.
	lim := newRateLimiter(0.001, 8000)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			for range 128 {
				if lim.allow("public\x00192.0.2.1") {
					allowed.Add(1)
				}
			}
		})
	}
	wg.Wait()
	if got := allowed.Load(); got != 8000 {
		t.Fatalf("concurrent burst allowed %d requests, want 8000", got)
	}
}

func TestRateLimitRejectionsAreLogged(t *testing.T) {
	var logs bytes.Buffer
	s := New(Deps{
		Cfg:    config.Config{RateLimitRPS: 0.001, RateLimitBurst: 1},
		Logger: slog.New(slog.NewTextHandler(&logs, nil)),
	})
	for range 2 {
		s.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))
	}
	if !strings.Contains(logs.String(), "status=429") {
		t.Fatalf("rate rejection missing from access log: %s", logs.String())
	}
}

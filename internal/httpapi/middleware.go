package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// recoverMiddleware 兜底恢复 panic，返回不泄露内部信息的 500。
func recoverMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", "method", r.Method, "path", r.URL.Path, "panic", rec)
				writeError(w, r, errInternal())
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// logMiddleware 输出结构化访问日志。
func logMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", float64(time.Since(start).Microseconds())/1000.0,
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(b)
}

// limiterEntry 记录每个客户端 IP 的令牌桶与最近活跃时间。
type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// rateLimiter 是按客户端 IP 的限流器，附带空闲回收，
// 避免限流映射表无限增长（本身就是一种资源泄露）。
type rateLimiter struct {
	rps     float64
	burst   int
	maxIdle time.Duration

	mu       sync.Mutex
	entries  map[string]*limiterEntry
	stopM    sync.Mutex
	wg       sync.WaitGroup
	stopFunc context.CancelFunc
}

func newRateLimiter(rps float64, burst int) *rateLimiter {
	return &rateLimiter{
		rps:     rps,
		burst:   burst,
		maxIdle: 5 * time.Minute,
		entries: make(map[string]*limiterEntry),
	}
}

// Run 启动空闲条目回收协程；ctx 取消后退出。
func (l *rateLimiter) Run(ctx context.Context) {
	l.stopM.Lock()
	defer l.stopM.Unlock()
	if l.stopFunc != nil {
		return
	}
	runCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	l.stopFunc = stop
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				l.evictIdle()
			}
		}
	}()
}

// Close 停止回收协程并等待退出。
func (l *rateLimiter) Close() {
	l.stopM.Lock()
	stop := l.stopFunc
	l.stopFunc = nil
	l.stopM.Unlock()
	if stop != nil {
		stop()
	}
	l.wg.Wait()
}

func (l *rateLimiter) evictIdle() {
	cutoff := time.Now().Add(-l.maxIdle)
	l.mu.Lock()
	defer l.mu.Unlock()
	for ip, entry := range l.entries {
		if entry.lastSeen.Before(cutoff) {
			delete(l.entries, ip)
		}
	}
}

func (l *rateLimiter) allow(ip string) bool {
	l.mu.Lock()
	entry, ok := l.entries[ip]
	if !ok {
		entry = &limiterEntry{limiter: rate.NewLimiter(rate.Limit(l.rps), l.burst)}
		l.entries[ip] = entry
	}
	entry.lastSeen = time.Now()
	lim := entry.limiter
	l.mu.Unlock()
	return lim.Allow()
}

// clientIP 解析直连地址；反代场景由部署层保证可信头。
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimitMiddleware 对每个请求执行按 IP 限流，超限返回 429 + Retry-After。
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !s.limiter.allow(ip) {
			writeError(w, r, errRateLimited("1"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// timeoutMiddleware 为每个请求设置处理超时。
func timeoutMiddleware(d, publishTimeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timeout := d
		publishing := (r.URL.Path == "/api/v1/admin/announcement" && (r.Method == http.MethodPut || r.Method == http.MethodDelete)) || (r.URL.Path == "/api/v1/admin/update-policy" && r.Method == http.MethodPut)
		if publishing {
			timeout = publishTimeout + 5*time.Second
			controller := http.NewResponseController(w)
			_ = controller.SetWriteDeadline(time.Now().Add(timeout))
			defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/statistics/preview/") {
			timeout = 50 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

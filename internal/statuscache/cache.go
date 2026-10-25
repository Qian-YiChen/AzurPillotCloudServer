// Package statuscache 提供区服状态的 TTL 缓存：
// 懒刷新 + single-flight（并发刷新共享）+ 后台保活循环。
//
// 语义与参考实现（AzurLaneServerStatus/cache.py）一致：
//   - 缓存是限流层：调用方无法绕过缓存强制回源；
//   - 刷新失败时复用旧条目（即使已过期）并记录错误；
//   - 只有完全没有缓存时才向上层返回查询错误。
//
// 所有后台协程都由 Run 启动、随 ctx 取消退出并由 WaitGroup 回收，
// Close 会阻塞等待全部协程结束，保证不发生协程泄露。
package statuscache

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"ap-cloud-server/internal/gateway"
)

// Entry 是一个区服的缓存条目。Error 为最近一次刷新失败的错误码
// （成功刷新后清空）。
type Entry struct {
	Servers   []gateway.RawServer
	QueriedAt time.Time
	Error     string
}

// QueryFunc 是可注入的查询函数，便于测试替身。
type QueryFunc func(ctx context.Context, region string) ([]gateway.RawServer, error)

type flight struct {
	done   chan struct{}
	entry  Entry
	cached bool
	err    error
}

// Cache 是全部区服的缓存。并发安全。
type Cache struct {
	ttl             time.Duration
	refreshInterval time.Duration
	queryTimeout    time.Duration
	query           QueryFunc
	logger          *slog.Logger

	mu      sync.Mutex
	entries map[string]*Entry
	flights map[string]*flight

	wg    sync.WaitGroup
	stop  context.CancelFunc
	stopM sync.Mutex
}

// New 创建缓存。query 一般为 gateway.QueryRegion 的包装。
func New(ttl, refreshInterval, queryTimeout time.Duration, query QueryFunc, logger *slog.Logger) *Cache {
	if logger == nil {
		logger = slog.Default()
	}
	return &Cache{
		ttl:             ttl,
		refreshInterval: refreshInterval,
		queryTimeout:    queryTimeout,
		query:           query,
		logger:          logger,
		entries:         make(map[string]*Entry),
		flights:         make(map[string]*flight),
	}
}

// Run 启动后台保活刷新循环，按 refreshInterval 周期刷新全部区服。
// ctx 取消或 Close 调用后循环退出。
func (c *Cache) Run(ctx context.Context) {
	c.stopM.Lock()
	if c.stop != nil {
		c.stopM.Unlock()
		return // 已在运行
	}
	runCtx, stop := context.WithCancel(context.WithoutCancel(ctx))
	c.stop = stop
	c.stopM.Unlock()

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				c.refreshAll(runCtx)
			}
		}
	}()
}

// Close 停止后台刷新并等待协程全部退出。
func (c *Cache) Close() {
	c.stopM.Lock()
	stop := c.stop
	c.stop = nil
	c.stopM.Unlock()
	if stop != nil {
		stop()
	}
	c.wg.Wait()
}

func (c *Cache) refreshAll(ctx context.Context) {
	for _, key := range cacheRegions {
		select {
		case <-ctx.Done():
			return
		default:
		}
		c.refresh(ctx, key)
	}
}

// Get 返回一个区服的缓存条目。cached=true 表示结果来自缓存
// （新鲜命中，或刷新失败后复用的旧数据）；只有完全无缓存时才返回错误。
// 调用方 ctx 取消时立即返回；刷新本身继续在后台完成并写入缓存。
func (c *Cache) Get(ctx context.Context, key string) (Entry, bool, error) {
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && time.Since(entry.QueriedAt) < c.ttl {
		out := *entry
		c.mu.Unlock()
		return out, true, nil
	}
	// 已过期或不存在：并入或发起 single-flight 刷新。
	if f, ok := c.flights[key]; ok {
		c.mu.Unlock()
		return c.waitFlight(ctx, f)
	}
	f := &flight{done: make(chan struct{})}
	c.flights[key] = f
	c.mu.Unlock()

	c.launchFlight(ctx, key, f)
	return c.waitFlight(ctx, f)
}

// launchFlight 在独立协程中执行刷新（受 WaitGroup 管控），
// 完成后清理 flight 注册。
func (c *Cache) launchFlight(ctx context.Context, key string, f *flight) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.executeFlight(ctx, key, f)
		c.mu.Lock()
		if c.flights[key] == f {
			delete(c.flights, key)
		}
		c.mu.Unlock()
	}()
}

// waitFlight 等待刷新结果或调用方取消；结果经 channel close 同步，无数据竞争。
func (c *Cache) waitFlight(ctx context.Context, f *flight) (Entry, bool, error) {
	select {
	case <-ctx.Done():
		return Entry{}, false, ctx.Err()
	case <-f.done:
	}
	return f.entry, f.cached, f.err
}

func (c *Cache) executeFlight(ctx context.Context, key string, f *flight) {
	defer close(f.done)

	queryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.queryTimeout)
	defer cancel()

	servers, err := c.query(queryCtx, key)
	now := time.Now()
	if err == nil {
		entry := &Entry{Servers: servers, QueriedAt: now}
		c.mu.Lock()
		c.entries[key] = entry
		c.mu.Unlock()
		f.entry = *entry
		f.cached = false
		f.err = nil
		return
	}

	// 刷新失败：复用旧条目（即使已过期），只记录错误码。
	code := errorCode(err)
	c.mu.Lock()
	old, ok := c.entries[key]
	if ok {
		old.Error = code
		f.entry = *old
		f.cached = true
		f.err = nil
	}
	c.mu.Unlock()
	if !ok {
		f.entry = Entry{}
		f.cached = false
		f.err = &gateway.RegionError{Code: code, Err: err}
	}
	c.logger.Warn("region refresh failed", "region", key, "code", code, "err", err)
}

// refresh 强制刷新一个区服（后台保活用）；已有进行中的 flight 则复用。
// 刷新在独立协程执行，不阻塞保活循环处理其他区服。
func (c *Cache) refresh(ctx context.Context, key string) {
	c.mu.Lock()
	if f, ok := c.flights[key]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-f.done:
		}
		return
	}
	f := &flight{done: make(chan struct{})}
	c.flights[key] = f
	c.mu.Unlock()

	c.launchFlight(ctx, key, f)
}

// Snapshot 返回缓存诊断信息（键、条目时间、错误码）。
func (c *Cache) Snapshot() map[string]Entry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]Entry, len(c.entries))
	for k, v := range c.entries {
		out[k] = *v
	}
	return out
}

func errorCode(err error) string {
	var re *gateway.RegionError
	if errors.As(err, &re) {
		return re.Code
	}
	return "network_error"
}

// cacheRegions 是缓存覆盖的区服列表（跳过 v2 API 不暴露的 kr 亦可，
// 但保留完整探测有助于诊断；此处与网关配置保持一致）。
var cacheRegions = []string{"cn", "cn_ios", "cn_channel", "jp", "en", "kr", "tw"}

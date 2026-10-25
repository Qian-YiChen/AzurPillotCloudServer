package statuscache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ap-cloud-server/internal/gateway"
)

func TestGetFreshAndExpiry(t *testing.T) {
	var calls atomic.Int64
	c := New(50*time.Millisecond, time.Hour, time.Second,
		func(ctx context.Context, region string) ([]gateway.RawServer, error) {
			calls.Add(1)
			return []gateway.RawServer{{ID: 28, Name: "长弓计划", State: 0}}, nil
		}, nil)

	entry, cached, err := c.Get(context.Background(), "cn")
	if err != nil || cached || len(entry.Servers) != 1 {
		t.Fatalf("首次查询错误: entry=%+v cached=%v err=%v", entry, cached, err)
	}
	// TTL 内命中缓存。
	_, cached, _ = c.Get(context.Background(), "cn")
	if !cached || calls.Load() != 1 {
		t.Fatalf("TTL 内应命中缓存: cached=%v calls=%d", cached, calls.Load())
	}
	// 过期后重新查询。
	time.Sleep(60 * time.Millisecond)
	_, cached, _ = c.Get(context.Background(), "cn")
	if cached || calls.Load() != 2 {
		t.Fatalf("过期后应重新查询: cached=%v calls=%d", cached, calls.Load())
	}
}

func TestGetFailureWithoutCache(t *testing.T) {
	c := New(time.Second, time.Hour, time.Second,
		func(ctx context.Context, region string) ([]gateway.RawServer, error) {
			return nil, errors.New("boom")
		}, nil)

	_, cached, err := c.Get(context.Background(), "cn")
	var re *gateway.RegionError
	if !errors.As(err, &re) || re.Code != "network_error" || cached {
		t.Fatalf("期望 network_error 且不走缓存: err=%v cached=%v", err, cached)
	}
}

func TestFailureReusesStaleEntry(t *testing.T) {
	fail := false
	c := New(30*time.Millisecond, time.Hour, time.Second,
		func(ctx context.Context, region string) ([]gateway.RawServer, error) {
			if fail {
				return nil, &gateway.RegionError{Code: "timeout"}
			}
			return []gateway.RawServer{{ID: 1}}, nil
		}, nil)

	if _, _, err := c.Get(context.Background(), "cn"); err != nil {
		t.Fatalf("首次查询不应失败: %v", err)
	}
	time.Sleep(40 * time.Millisecond) // 过期
	fail = true

	entry, cached, err := c.Get(context.Background(), "cn")
	if err != nil || !cached {
		t.Fatalf("失败时应复用旧缓存: err=%v cached=%v", err, cached)
	}
	if entry.Error != "timeout" || len(entry.Servers) != 1 {
		t.Fatalf("应记录错误码并保留服务器: %+v", entry)
	}
}

func TestSingleFlight(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int64
	c := New(time.Millisecond, time.Hour, time.Second,
		func(ctx context.Context, region string) ([]gateway.RawServer, error) {
			calls.Add(1)
			<-release
			return []gateway.RawServer{{ID: 1}}, nil
		}, nil)

	var wg sync.WaitGroup
	results := make([]Entry, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			entry, _, err := c.Get(context.Background(), "cn")
			if err != nil {
				t.Errorf("并发查询失败: %v", err)
			}
			results[i] = entry
		}(i)
	}
	time.Sleep(50 * time.Millisecond) // 等全部协程进入等待
	close(release)
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("single-flight 应只触发一次查询，实际 %d", calls.Load())
	}
	for i, entry := range results {
		if len(entry.Servers) != 1 {
			t.Fatalf("协程 %d 未收到结果: %+v", i, entry)
		}
	}
}

func TestRunRefreshesInBackgroundAndClose(t *testing.T) {
	var calls atomic.Int64
	c := New(time.Hour, 20*time.Millisecond, time.Second,
		func(ctx context.Context, region string) ([]gateway.RawServer, error) {
			calls.Add(1)
			return []gateway.RawServer{{ID: 1}}, nil
		}, nil)
	c.Run(context.Background())
	time.Sleep(90 * time.Millisecond)
	c.Close()

	if calls.Load() < int64(len(cacheRegions)) {
		t.Fatalf("后台保活应刷新全部区服，实际 %d 次", calls.Load())
	}
	// Close 后不再刷新。
	before := calls.Load()
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != before {
		t.Fatalf("Close 后不应继续刷新")
	}
}

func TestGetRespectsCallerCancel(t *testing.T) {
	c := New(time.Hour, time.Hour, time.Second,
		func(ctx context.Context, region string) ([]gateway.RawServer, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				return nil, nil
			}
		}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, _, err := c.Get(ctx, "cn")
	if err == nil {
		t.Fatal("取消后应返回错误")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("取消后应立即返回，实际耗时 %v", elapsed)
	}
	c.Close()
}

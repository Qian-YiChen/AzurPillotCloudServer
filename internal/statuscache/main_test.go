package statuscache

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain 在全部测试结束后检查协程泄露：缓存的后台刷新、single-flight
// 与等待协程都必须随 Close / ctx 取消完全退出。
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

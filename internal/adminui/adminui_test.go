package adminui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlerServesEntry 验证入口页与 SPA 回退（embed 与 stub 两种变体都应满足）。
func TestHandlerServesEntry(t *testing.T) {
	h := Handler()

	t.Run("入口页", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("期望 200，得到 %d", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("期望 text/html，得到 %s", ct)
		}
		if !strings.Contains(rec.Body.String(), "管理后台") {
			t.Fatalf("入口页内容异常: %s", rec.Body.String())
		}
	})

	t.Run("SPA 回退", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/some/spa/route", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("未知路径应回退入口页，得到 %d", rec.Code)
		}
	})

	t.Run("方法限制", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST 应被拒绝，得到 %d", rec.Code)
		}
	})
}

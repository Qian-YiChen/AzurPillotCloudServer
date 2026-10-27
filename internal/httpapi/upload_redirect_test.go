package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type unreadUploadBody struct{ t *testing.T }

func (b unreadUploadBody) Read([]byte) (int, error) {
	b.t.Fatal("C must not read or forward upload bodies")
	return 0, nil
}

func (unreadUploadBody) Close() error { return nil }

func TestUploadRedirectPreservesProtocolWithoutContactingA(t *testing.T) {
	s, _ := newTestServer(t, &fakeBackend{}, nil)
	// A need not be reachable from C. Use a reserved documentation address.
	s.deps.Cfg.LegacyUploadURL = "http://192.0.2.1:30207/api/upload"
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/v1/uploads/init"},
		{"PUT", "/api/v1/uploads/11111111-1111-1111-1111-111111111111/files/0?test=1"},
		{"POST", "/api/v1/uploads/11111111-1111-1111-1111-111111111111/complete"},
	} {
		t.Run(c.method+c.path, func(t *testing.T) {
			r := httptest.NewRequest(c.method, c.path, nil)
			r.Body = unreadUploadBody{t}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusTemporaryRedirect {
				t.Fatalf("HTTP %d: %s", w.Code, w.Body)
			}
			if got := w.Header().Get("Location"); got != "http://192.0.2.1:30207"+c.path {
				t.Fatal(got)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("upload routing must not be cached")
			}
		})
	}
}

func TestUploadRedirectRejectsInvalidAndOversizeRequests(t *testing.T) {
	s, _ := newTestServer(t, &fakeBackend{}, nil)
	s.deps.Cfg.StatisticsFileBytes = 8
	for _, c := range []struct {
		path, body string
		want       int
	}{
		{"bad/files/0", "", 400}, {"11111111-1111-1111-1111-111111111111/files/no", "", 400},
		{"11111111-1111-1111-1111-111111111111/files/0", "123456789", 413},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/v1/uploads/"+c.path, strings.NewReader(c.body))
		s.Handler().ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatal(w.Code, w.Body)
		}
	}
}

func TestUploadRedirectRequiresConfiguredPublicA(t *testing.T) {
	s, _ := newTestServer(t, &fakeBackend{}, nil)
	for _, target := range []string{"", "file:///tmp/upload", "http://user:password@192.0.2.1:8090"} {
		s.deps.Cfg.LegacyUploadURL = target
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/uploads/init", nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatal(w.Code, w.Body)
		}
	}
}

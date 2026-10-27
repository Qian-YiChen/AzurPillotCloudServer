package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOnlyPublishMutationsUsePublicationTimeout(t *testing.T) {
	for _, c := range []struct {
		method, path string
		publishing   bool
	}{
		{"PUT", "/api/v1/admin/announcement", true}, {"DELETE", "/api/v1/admin/announcement", true}, {"PUT", "/api/v1/admin/update-policy", true},
		{"GET", "/api/v1/admin/announcement", false}, {"GET", "/api/v1/admin/update-policy", false}, {"POST", "/api/v2/post/cl1", false},
	} {
		h := timeoutMiddleware(100*time.Millisecond, 2*time.Second, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deadline, ok := r.Context().Deadline()
			if !ok {
				t.Fatal("missing deadline")
			}
			left := time.Until(deadline)
			if c.publishing && left < 6*time.Second {
				t.Fatal(c, "publication inherited short timeout", left)
			}
			if !c.publishing && left > 100*time.Millisecond {
				t.Fatal(c, "ordinary request timeout changed", left)
			}
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(c.method, c.path, nil))
	}
}

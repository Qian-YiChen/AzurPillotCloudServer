package httpapi

import (
	"ap-cloud-server/internal/maintenance"
	"context"
	"errors"
	"net/http/httptest"
	"testing"
)

type testMaintenanceSync struct{ err error }

func (f *testMaintenanceSync) Status(context.Context) (maintenance.Status, error) {
	return maintenance.Status{Enabled: true, IntervalSeconds: 600}, nil
}
func (f *testMaintenanceSync) Sync(context.Context) error { return f.err }

func TestMaintenanceSourceAuthorizationAndRefresh(t *testing.T) {
	s, _ := newTestServer(t, &fakeBackend{}, nil)
	f := &testMaintenanceSync{}
	s.deps.Maintenance = f
	for _, c := range []struct {
		method, token string
		err           error
		want          int
	}{
		{"GET", "", nil, 401}, {"GET", "test-admin-token", nil, 200}, {"POST", "test-admin-token", nil, 200},
		{"POST", "test-admin-token", maintenance.ErrBusy, 409}, {"POST", "test-admin-token", errors.New("upstream failed"), 503},
		{"DELETE", "test-admin-token", nil, 405},
	} {
		f.err = c.err
		r := httptest.NewRequest(c.method, "/api/v1/admin/maintenance/source", nil)
		if c.token != "" {
			r.Header.Set("Authorization", "Bearer "+c.token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != c.want {
			t.Fatal(c, w.Code, w.Body)
		}
	}
}

package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUploadGatewayStreamsBeforeBodyComplete(t *testing.T) {
	started := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/uploads/11111111-1111-1111-1111-111111111111/files/0" || r.Method != "PUT" {
			t.Error(r.Method, r.URL)
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("private C credentials forwarded")
		}
		b := make([]byte, 1)
		if _, err := io.ReadFull(r.Body, b); err != nil {
			t.Error(err)
			return
		}
		started <- struct{}{}
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(202)
	}))
	defer upstream.Close()
	s, _ := newTestServer(t, &fakeBackend{}, nil)
	s.deps.Cfg.LegacyUploadURL = upstream.URL + "/api/upload"
	front := httptest.NewServer(s.Handler())
	defer front.Close()
	rd, wr := io.Pipe()
	defer rd.Close()
	defer wr.Close()
	req, _ := http.NewRequest("PUT", front.URL+"/api/v1/uploads/11111111-1111-1111-1111-111111111111/files/0", rd)
	req.Header.Set("Authorization", "Bearer private-token")
	req.Header.Set("Cookie", "private=value")
	done := make(chan error, 1)
	go func() {
		resp, err := front.Client().Do(req)
		if resp != nil {
			_ = resp.Body.Close()
			if resp.StatusCode != 202 {
				t.Errorf("HTTP %d", resp.StatusCode)
			}
		}
		done <- err
	}()
	if _, err := wr.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("proxy buffered the request body")
	}
	_, _ = wr.Write([]byte("remaining"))
	_ = wr.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestUploadGatewayRejectsInvalidAndOversizeRequests(t *testing.T) {
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

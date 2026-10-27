package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"ap-cloud-server/internal/gateway"
)

func TestLegacyServerCheckerContract(t *testing.T) {
	query := func(ctx context.Context, region string) ([]gateway.RawServer, error) {
		if region == "jp" {
			return nil, &gateway.RegionError{Code: "timeout"}
		}
		return []gateway.RawServer{{ID: 1, Name: region + "-test", State: 1}}, nil
	}
	s, _ := newTestServer(t, &fakeBackend{}, query)
	for _, region := range []string{"cn", "cn_ios", "cn_channel", "en", "tw"} {
		w := get(s, "/api/v1/servers/"+region+"_1?region=jp&server_id=999", nil)
		var result struct {
			Region string
			Server struct {
				ID           int
				Name, Status string
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 {
			t.Fatal(w.Code, w.Body, err)
		}
		if result.Region != region || result.Server.ID != 1 || result.Server.Name != region+"-test" || result.Server.Status != "maintenance" {
			t.Fatal(result)
		}
	}
	for _, c := range []struct {
		path   string
		status int
	}{{"cn_999", 404}, {"jp_1", 503}, {"kr_1", 400}, {"cn_0", 400}, {"cn_no", 400}, {"cn", 400}} {
		w := get(s, "/api/v1/servers/"+c.path, nil)
		if w.Code != c.status {
			t.Fatal(c, w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/servers/cn_1", nil))
	if w.Code != 405 {
		t.Fatal(w.Code, w.Body)
	}
}

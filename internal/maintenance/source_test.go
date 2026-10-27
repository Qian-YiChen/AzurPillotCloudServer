package maintenance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestOfficialSourceLive(t *testing.T) {
	if os.Getenv("TEST_OFFICIAL_NEWS") != "1" {
		t.Skip("live source check disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	n, err := NewSource().Fetch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(n.Title, n.URL, n.Plan.StartsAt, n.Plan.EndsAt)
}

func TestOfficialArticleDates(t *testing.T) {
	for _, c := range []struct {
		name, title, content, created, wantStart, wantEnd string
		bad                                               bool
	}{
		{"official split spans", "2026维护公告（2026年10月22日10:00）", `<p>司令部将于<span>10月22日</span><span>10:00~16:00</span>对以下港区进行为期6小时的改造建设</p><p>活动时间11月1日00:00~11月30日23:59</p>`, "", "2026-10-22T10:00:00+08:00", "2026-10-22T16:00:00+08:00", false},
		{"new year", "维护公告", `将于1月1日00:00～06:00停服维护`, "2026-12-31 12:00:00", "2027-01-01T00:00:00+08:00", "2027-01-01T06:00:00+08:00", false},
		{"overnight", "维护公告", `将于12月31日23:00至次日02:00维护`, "2026-12-30 12:00:00", "2026-12-31T23:00:00+08:00", "2027-01-01T02:00:00+08:00", false},
		{"explicit duration", "维护公告", `将于10月22日10:00对港区进行为期6小时的改造建设`, "2026-10-21 12:00:00", "2026-10-22T10:00:00+08:00", "2026-10-22T16:00:00+08:00", false},
		{"no inferred end", "维护公告", `将于10月22日10:00停服维护，结束时间另行通知`, "2026-10-21 12:00:00", "", "", true},
		{"invalid date", "维护公告", `将于2月30日10:00~16:00维护`, "2026-02-28 12:00:00", "", "", true},
		{"event range alone", "维护公告", `活动时间10月22日10:00~16:00，维护后开启`, "2026-10-21 12:00:00", "", "", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, err := parse(article{ID: 18495, Title: c.title, Content: c.content, Created: c.created})
			if c.bad {
				if err == nil {
					t.Fatal("expected parse error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.StartsAt.Format(time.RFC3339) != c.wantStart || m.EndsAt.Format(time.RFC3339) != c.wantEnd {
				t.Fatal(m)
			}
		})
	}
}

func TestFetchIgnoresPinnedAnnouncementAndRejectsLatestParseFailure(t *testing.T) {
	bad := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/news/list" {
			if r.URL.Query().Get("gameExtensionId") != "103" || r.URL.Query().Get("positionId") != "2" {
				t.Error(r.URL)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":[{"id":1,"title":"概率公示","createTime":"2026-10-27 00:00:00"},{"id":2,"title":"2026维护公告（2026年10月22日10:00）"},{"id":3,"title":"2026维护公告（2026年10月15日10:00）"}]}`))
			return
		}
		if r.URL.Path != "/news/2" {
			t.Error(r.URL)
			w.WriteHeader(404)
			return
		}
		body := "司令部将于10月22日10:00~16:00进行维护"
		if bad {
			body = "未知格式"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": article{ID: 2, GameID: 103, Title: "2026维护公告（2026年10月22日10:00）", Content: body}})
	}))
	defer srv.Close()
	s := &Source{client: srv.Client(), baseURL: srv.URL}
	n, err := s.Fetch(context.Background())
	if err != nil || n.URL != OfficialPage+"#news_detail_id=2" {
		t.Fatal(n, err)
	}
	bad = true
	if _, err = s.Fetch(context.Background()); err == nil {
		t.Fatal("latest parse failure must not become success")
	}
}

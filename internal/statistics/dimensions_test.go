package statistics

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"testing"
	"time"
)

func applyDimensionResult(t *testing.T, s *Service, r Result) {
	t.Helper()
	ack, err := s.Apply(context.Background(), r)
	if err != nil || ack.Status != "accepted" {
		t.Fatal(ack, err)
	}
}

func readDimensionQuery(t *testing.T, s *Service, path string, target any) {
	t.Helper()
	w := call(t, s, "GET", path, "", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), target); err != nil {
		t.Fatal(err, w.Body)
	}
}

func TestResearchDenominatorsSeparateSeriesAndFocus(t *testing.T) {
	s := service(t)
	scene := fixture(t).Source.Metadata.Scene
	for _, dimension := range []struct {
		series, focus, selected, other int
	}{
		{8, 9, 1, 3}, {9, 9, 3, 1}, {8, 8, 1, 0}, {8, 0, 1, 0},
	} {
		r := fixture(t)
		r.Source.Metadata.Project = "research"
		r.Source.Metadata.Scene = scene
		r.Source.Metadata.CollectedAt = "2026-09-10T16:10:00Z"
		r.Events = nil
		for i := 0; i < dimension.selected+dimension.other; i++ {
			project := "Q-A"
			if i >= dimension.selected {
				project = "Q-B"
			}
			r.Events = append(r.Events, Event{Type: "refresh", Series: dimension.series, FocusSeries: dimension.focus, Project: project})
		}
		applyDimensionResult(t, s, r)
	}
	query := url.Values{"from": {"2026-09-10T00:00:00Z"}, "to": {"2026-09-12T00:00:00Z"}, "scene": {scene}, "project": {"Q-A"}}
	type row struct {
		Series int     `json:"series"`
		Focus  int     `json:"focus_series"`
		Total  int     `json:"total_appearances"`
		Rate   float64 `json:"appearance_rate"`
	}
	expect := func(rows []row) {
		t.Helper()
		if len(rows) != 4 {
			t.Fatal("dimensions merged", rows)
		}
		for _, r := range rows {
			wantTotal, wantRate := 1, 1.0
			if r.Focus == 9 {
				wantTotal, wantRate = 4, .25
				if r.Series == 9 {
					wantRate = .75
				}
			}
			if r.Total != wantTotal || math.Abs(r.Rate-wantRate) > 1e-9 {
				t.Fatalf("wrong denominator for series/focus: %+v", r)
			}
		}
	}
	var summary struct {
		Groups []row `json:"groups"`
		Daily  []struct {
			Day string `json:"day"`
		} `json:"daily"`
	}
	readDimensionQuery(t, s, "/api/v2/get/research?"+query.Encode(), &summary)
	expect(summary.Groups)
	if len(summary.Daily) != 1 || summary.Daily[0].Day != "2026-09-10" {
		t.Fatal("daily data did not preserve the default UTC timezone", summary.Daily)
	}
	var slots struct {
		Slots []row `json:"slots"`
	}
	readDimensionQuery(t, s, "/api/v2/get/research/time-slots?"+query.Encode(), &slots)
	expect(slots.Slots)
	query.Set("focus_series", "unknown")
	readDimensionQuery(t, s, "/api/v2/get/research?"+query.Encode(), &summary)
	if len(summary.Groups) != 1 || summary.Groups[0].Focus != 0 {
		t.Fatal("unknown focus merged with recognized focuses", summary.Groups)
	}
	query.Del("focus_series")
	query.Set("timezone", "Asia/Shanghai")
	readDimensionQuery(t, s, "/api/v2/get/research?"+query.Encode(), &summary)
	if summary.Daily[0].Day != "2026-09-11" {
		t.Fatal("explicit Shanghai timezone ignored", summary.Daily)
	}
	query.Set("timezone", "UTC")
	readDimensionQuery(t, s, "/api/v2/get/research?"+query.Encode(), &summary)
	if summary.Daily[0].Day != "2026-09-10" {
		t.Fatal("explicit daily timezone ignored", summary.Daily)
	}
}

func TestResearchRewardsKeepFocusAndMissingRewardsSeparate(t *testing.T) {
	s := service(t)
	r := fixture(t)
	r.Source.Metadata.Project = "research"
	r.Events = []Event{
		{Type: "complete", Series: 9, Project: "Q-A", Duration: 3600, Items: []Item{{Item: "Coins", Amount: 10}}},
		{Type: "complete", Series: 9, Project: "Q-A", Duration: 3600},
		{Type: "complete", Series: 9, FocusSeries: 9, Project: "Q-A", Items: []Item{{Item: "Coins", Amount: 90}}},
		{Type: "complete", Series: 8, Project: "Q-A", Items: []Item{{Item: "Coins", Amount: 1000}}},
	}
	applyDimensionResult(t, s, r)
	var reply struct {
		Items []struct {
			Focus   int      `json:"focus_series"`
			Samples int      `json:"samples"`
			Amount  int      `json:"amount"`
			Rate    float64  `json:"drop_rate"`
			Average float64  `json:"average_per_sample"`
			Hourly  *float64 `json:"amount_per_hour"`
		} `json:"items"`
	}
	query := url.Values{"scene": {r.Source.Metadata.Scene}, "series": {"9"}}
	readDimensionQuery(t, s, "/api/v2/get/research?"+query.Encode(), &reply)
	if len(reply.Items) != 2 {
		t.Fatal("reward dimensions merged", reply.Items)
	}
	for _, item := range reply.Items {
		if item.Focus == 0 {
			if item.Samples != 2 || item.Amount != 10 || item.Rate != .5 || item.Average != 5 || item.Hourly == nil || *item.Hourly != 5 {
				t.Fatal("zero-reward sample was removed from the denominator", item)
			}
		} else if item.Focus != 9 || item.Samples != 1 || item.Amount != 90 || item.Rate != 1 || item.Hourly != nil {
			t.Fatal("recognized focus or missing duration miscounted", item)
		}
	}
}

func TestDropSourcesHaveIndependentSamplesAndTags(t *testing.T) {
	s := service(t)
	query := url.Values{"from": {"2026-09-13T00:00:00Z"}, "to": {"2026-09-14T00:00:00Z"}, "server": {"tw"}}
	for _, scene := range []string{"commission", "opsi_daily"} {
		r := fixture(t)
		r.Source.Metadata.Scene = scene
		r.Source.Metadata.Server = "tw"
		r.Source.Metadata.CollectedAt = "2026-09-13T08:00:00Z"
		if scene == "commission" {
			r.Events = []Event{{Type: "drop", Items: []Item{{Item: "Coins", Amount: 10}}}, {Type: "drop"}}
		} else {
			r.Events = []Event{{Type: "drop", Items: []Item{{Item: "Coins", Amount: 90}, {Item: "Coins", Tag: "bonus", Amount: 2}}}}
		}
		applyDimensionResult(t, s, r)
	}
	var reply struct {
		Items []struct {
			Scene   string  `json:"scene"`
			Tag     string  `json:"tag"`
			Samples int     `json:"samples"`
			Amount  int     `json:"amount"`
			Rate    float64 `json:"drop_rate"`
		} `json:"items"`
	}
	readDimensionQuery(t, s, "/api/v2/get/drop?"+query.Encode(), &reply)
	if len(reply.Items) != 3 {
		t.Fatal("scenes or tags merged", reply.Items)
	}
	query.Set("scene", "commission")
	readDimensionQuery(t, s, "/api/v2/get/drop?"+query.Encode(), &reply)
	if len(reply.Items) != 1 || reply.Items[0].Scene != "commission" || reply.Items[0].Samples != 2 || reply.Items[0].Amount != 10 || reply.Items[0].Rate != .5 {
		t.Fatal("daily drops changed commission results", reply.Items)
	}
}

func TestDimensionOptionsUseAvailableDataAndScope(t *testing.T) {
	s := service(t)
	for _, sample := range []struct {
		series             int
		server, status, at string
	}{
		{8, "tw", "complete", "2026-09-15T08:00:00Z"},
		{11, "tw", "complete", "2026-09-15T08:00:00Z"},
		{12, "cn", "complete", "2026-09-15T08:00:00Z"},
		{13, "tw", "needs_review", "2026-09-15T08:00:00Z"},
		{14, "tw", "complete", "2026-09-16T00:00:00Z"},
	} {
		r := fixture(t)
		r.Source.Metadata.Project = "research"
		r.Source.Metadata.Server = sample.server
		r.Source.Metadata.CollectedAt = sample.at
		r.Status = sample.status
		r.Events = []Event{{Type: "refresh", Series: sample.series, FocusSeries: 9, Project: "Q-A"}}
		applyDimensionResult(t, s, r)
	}
	// A selected project, scene or period must not hide the other choices.
	query := url.Values{"from": {"2026-09-15T00:00:00Z"}, "to": {"2026-09-16T00:00:00Z"}, "server": {"tw"}, "series": {"8"}, "scene": {"missing"}, "project": {"missing"}}
	var reply struct {
		Options []struct {
			Series      int `json:"series"`
			Appearances int `json:"appearances"`
			Samples     int `json:"samples"`
		} `json:"options"`
	}
	readDimensionQuery(t, s, "/api/v2/get/research/options?"+query.Encode(), &reply)
	if len(reply.Options) != 2 || reply.Options[0].Series != 11 || reply.Options[1].Series != 8 || reply.Options[0].Appearances != 1 || reply.Options[0].Samples != 0 {
		t.Fatal("options were hardcoded or ignored status, interval or server", reply.Options)
	}
}

func TestSummaryDefaultFromFollowsExplicitTo(t *testing.T) {
	s := service(t)
	var reply struct {
		From time.Time `json:"from"`
		To   time.Time `json:"to"`
	}
	readDimensionQuery(t, s, "/api/v2/get/research?to=2026-01-01T00:00:00Z", &reply)
	if reply.To.Sub(reply.From) != 30*24*time.Hour {
		t.Fatal("default range did not follow explicit to", reply)
	}
}

func TestNewDataRemainsInItsOwnDimensions(t *testing.T) {
	s := service(t)
	ctx := context.Background()
	query := url.Values{"from": {"2026-09-17T00:00:00Z"}, "to": {"2026-09-18T00:00:00Z"}, "server": {"jp"}}
	makeDrop := func(scene string, count, amount int) Result {
		t.Helper()
		r := fixture(t)
		r.Source.Metadata.Server = "jp"
		r.Source.Metadata.Scene = scene
		r.Source.Metadata.CollectedAt = "2026-09-17T08:00:00Z"
		r.Events = nil
		for range count {
			r.Events = append(r.Events, Event{Type: "drop", Items: []Item{{Item: "Coins", Amount: int64(amount)}}})
		}
		return r
	}
	applyDimensionResult(t, s, makeDrop("commission", 2, 5))
	applyDimensionResult(t, s, makeDrop("opsi_daily", 1, 90))
	expectSources := func(want map[string][2]int) {
		t.Helper()
		var summary struct {
			Items []struct {
				Scene   string `json:"scene"`
				Samples int    `json:"samples"`
				Amount  int    `json:"amount"`
			} `json:"items"`
		}
		readDimensionQuery(t, s, "/api/v2/get/drop?"+query.Encode(), &summary)
		if len(summary.Items) != len(want) {
			t.Fatal("new source was merged or an old source disappeared", summary.Items)
		}
		for _, item := range summary.Items {
			if actual := [2]int{item.Samples, item.Amount}; want[item.Scene] != actual {
				t.Fatalf("source %q: got %v, want %v", item.Scene, actual, want[item.Scene])
			}
		}
		var options struct {
			Options []struct {
				Scene string `json:"scene"`
			} `json:"options"`
		}
		readDimensionQuery(t, s, "/api/v2/get/drop/options?"+query.Encode(), &options)
		if len(options.Options) != len(want) {
			t.Fatal("new source did not become an available option", options.Options)
		}
		for _, option := range options.Options {
			if _, ok := want[option.Scene]; !ok {
				t.Fatal("unexpected source option", option.Scene)
			}
		}
	}
	expectSources(map[string][2]int{"commission": {2, 10}, "opsi_daily": {1, 90}})
	added := makeDrop("commission", 50, 3)
	applyDimensionResult(t, s, added)
	applyDimensionResult(t, s, makeDrop("opsi_future_mode", 3, 2))
	expectSources(map[string][2]int{"commission": {52, 160}, "opsi_daily": {1, 90}, "opsi_future_mode": {3, 6}})
	ack, err := s.Apply(ctx, added)
	if err != nil || ack.Status != "duplicate" {
		t.Fatal("retransmission was not deduplicated", ack, err)
	}
	expectSources(map[string][2]int{"commission": {52, 160}, "opsi_daily": {1, 90}, "opsi_future_mode": {3, 6}})
	// Correcting a source must remove its old contribution before adding the new one.
	added.Revision++
	added.Source.Metadata.Scene = "opsi_future_mode"
	applyDimensionResult(t, s, added)
	expectSources(map[string][2]int{"commission": {2, 10}, "opsi_daily": {1, 90}, "opsi_future_mode": {53, 156}})

	for _, series := range []int{9, 13} {
		r := makeDrop("research_refresh", 1, 1)
		r.Source.Metadata.Project = "research"
		r.Events = []Event{{Type: "refresh", Series: series, FocusSeries: 9, Project: "Q-A"}}
		applyDimensionResult(t, s, r)
	}
	var periods struct {
		Options []struct {
			Series int `json:"series"`
		} `json:"options"`
	}
	readDimensionQuery(t, s, "/api/v2/get/research/options?"+query.Encode(), &periods)
	if len(periods.Options) != 2 || periods.Options[0].Series != 13 || periods.Options[1].Series != 9 {
		t.Fatal("new research period was not discovered independently", periods.Options)
	}
	var research struct {
		Groups []struct {
			Series      int `json:"series"`
			Appearances int `json:"appearances"`
			Total       int `json:"total_appearances"`
		} `json:"groups"`
	}
	query.Set("series", "9")
	readDimensionQuery(t, s, "/api/v2/get/research?"+query.Encode(), &research)
	if len(research.Groups) != 1 || research.Groups[0].Series != 9 || research.Groups[0].Appearances != 1 || research.Groups[0].Total != 1 {
		t.Fatal("new period changed the existing period", research.Groups)
	}
}

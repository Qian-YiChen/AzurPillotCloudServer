package statistics

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"testing"
	"time"
)

func TestTimeSlotsUseCollectedTimeAndFullDenominator(t *testing.T) {
	s := service(t)
	scene := fixture(t).Source.Metadata.Scene
	for _, sample := range []struct {
		at, project string
		focus       int
	}{
		{"2026-10-25T00:05:00Z", "Q-A", 9},
		{"2026-10-25T00:10:00Z", "Q-B", 9},
		{"2026-10-25T00:29:59Z", "Q-B", 9},
		{"2026-10-25T00:30:00Z", "Q-A", 9},
		{"2026-10-25T00:05:00Z", "Q-B", 8},
	} {
		r := fixture(t)
		r.Source.Metadata.Project = "research"
		r.Source.Metadata.Scene = scene
		r.Source.Metadata.CollectedAt = sample.at
		r.Events = []Event{{Type: "refresh", Series: 9, Project: sample.project, FocusSeries: sample.focus}}
		ack, err := s.Apply(context.Background(), r)
		if err != nil || ack.Status != "accepted" {
			t.Fatal(ack, err)
		}
	}
	query := url.Values{"from": {"2026-10-25T00:00:00Z"}, "to": {"2026-10-26T00:00:00Z"}, "scene": {scene}, "focus_series": {"9"}, "project": {"Q-A"}, "timezone": {"Asia/Shanghai"}}
	path := "/api/v2/get/research/time-slots?" + query.Encode()
	w := call(t, s, "GET", path, "", nil)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var result struct {
		Timezone string `json:"timezone"`
		Slots    []struct {
			Start       string  `json:"slot_start"`
			Appearances int     `json:"appearances"`
			Total       int     `json:"total_appearances"`
			Rate        float64 `json:"appearance_rate"`
		} `json:"slots"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Timezone != "Asia/Shanghai" || len(result.Slots) != 2 {
		t.Fatal(w.Body)
	}
	if a := result.Slots[0]; a.Start != "08:00" || a.Appearances != 1 || a.Total != 3 || math.Abs(a.Rate-1.0/3) > 1e-9 {
		t.Fatal("project filter changed denominator or timestamps", a)
	}
	if a := result.Slots[1]; a.Start != "08:30" || a.Total != 1 || a.Rate != 1 {
		t.Fatal("half hour boundary merged", a)
	}
	// The query interval is [from,to); processing time must never move an
	// observation across the half-hour capture-time boundary.
	query.Set("to", "2026-10-25T00:30:00Z")
	w = call(t, s, "GET", "/api/v2/get/research?"+query.Encode(), "", nil)
	var overall struct {
		Groups []struct {
			Rate float64 `json:"appearance_rate"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &overall); err != nil || w.Code != 200 || len(overall.Groups) != 1 || math.Abs(overall.Groups[0].Rate-1.0/3) > 1e-9 {
		t.Fatal("aggregate denominator", w.Code, w.Body, err)
	}
	query.Set("timezone", "America/New_York")
	w = call(t, s, "GET", "/api/v2/get/research/time-slots?"+query.Encode(), "", nil)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Slots[0].Start != "20:00" {
		t.Fatal("timezone conversion failed", w.Body, err)
	}
}

func TestTimeSlotFilterValidation(t *testing.T) {
	// Validation happens before database access.
	s := &Service{Now: time.Now}
	for _, filter := range []string{"timezone=invalid-zone", "focus_series=0", "focus_series=no"} {
		w := call(t, s, "GET", "/api/v2/get/research/time-slots?"+filter, "", nil)
		if w.Code != 400 {
			t.Fatal(filter, w.Code, w.Body)
		}
	}
}

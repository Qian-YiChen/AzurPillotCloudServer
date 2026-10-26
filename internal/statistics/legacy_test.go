package statistics

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLegacyReportQueriesAndOptionalIdempotency(t *testing.T) {
	s := service(t)
	device := fixture(t).SHA256[:32]
	t.Cleanup(func() {
		_, _ = s.Pool.Exec(context.Background(), `DELETE FROM legacy_azurstat_reports WHERE device_id=$1`, device)
	})
	req := legacyReport{Device: device, Task: "opsi_meowfficer_farming", CollectedAt: "2026-10-25T08:05:00+08:00", Body: legacyBody{Zone: "NA", ZoneType: "SAFE", ZoneID: 22, Hazard: 3, Combat: 10, Items: []legacyItem{{Item: "Coins", Amount: 20}, {Item: "Coins", Amount: 10, IsMeow: true}, {Item: "图纸", Amount: 2}}}}
	for i := 0; i < 2; i++ {
		w := call(t, s, "POST", "/api/azurstat", "", req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	req.RequestID = "retry"
	for i := 0; i < 2; i++ {
		w := call(t, s, "POST", "/api/azurstat", "", req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	req.Body.Combat = 11
	if w := call(t, s, "POST", "/api/azurstat", "", req); w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
	get := func(path string) map[string]any {
		w := call(t, s, "GET", path, "", nil)
		if w.Code != 200 {
			t.Fatal(path, w.Code, w.Body)
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	q := "?device_id=" + device + "&start_date=2026-10-25&end_date=2026-10-25"
	stats := get("/api/azurstat/stats" + q)
	if stats["total_reports"] != float64(3) || stats["total_combat_count"] != float64(30) || stats["total_item_amount"] != float64(96) {
		t.Fatal(stats)
	}
	meow := get("/api/azurstat/stats" + q + "&is_meow=true")
	if meow["total_combat_count"] != float64(30) || meow["total_item_amount"] != float64(30) {
		t.Fatal("item filter changed denominator", meow)
	}
	items := get("/api/azurstat/items" + q)["items"].([]any)
	if items[0].(map[string]any)["avg_per_combat"] != float64(3) {
		t.Fatal(items)
	}
	if len(get("/api/azurstat/history" + q + "&interval=month")["history"].([]any)) != 1 {
		t.Fatal("history")
	}
	get("/api/azurstat/filters")
	data := get("/api/data/opsi_items?device_id=" + device + "&limit=2&offset=1")["data"].([]any)
	if len(data) != 2 || data[0].(map[string]any)["hazard_level"] != float64(3) {
		t.Fatal(data)
	}
	if len(get("/api/data/opsi_items?device_id=" + device + "&offset=9")["data"].([]any)) != 0 {
		t.Fatal("pagination")
	}
	// Authenticated OCR and legacy client reports keep independent provenance.
	r := fixture(t)
	r.Source.Metadata.DeviceID = device
	r.Source.Metadata.Scene = "opsi_meowfficer_farming"
	r.Source.Metadata.CombatCount = 7
	r.Events[0].HazardLevel = 4
	r.Events[0].ZoneID = 44
	if ack, err := s.Apply(context.Background(), r); err != nil || ack.Status != "accepted" {
		t.Fatal(ack, err)
	}
	t.Cleanup(func() { _, _ = s.Pool.Exec(context.Background(), `DELETE FROM stat_results WHERE sha256=$1`, r.SHA256) })
	r.Revision = 2
	r.Events[0].Items[0].Amount = 15
	if ack, err := s.Apply(context.Background(), r); err != nil || ack.Status != "accepted" {
		t.Fatal(ack, err)
	}
	stats = get("/api/azurstat/stats?device_id=" + device)
	if stats["total_reports"] != float64(4) || stats["total_combat_count"] != float64(37) || stats["total_item_amount"] != float64(111) {
		t.Fatal("OCR correction double counted", stats)
	}
}

func TestLegacyTelemetrySSEFlushAndCancel(t *testing.T) {
	s := service(t)
	mux := http.NewServeMux()
	s.Register(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/telemetry/stats/stream", nil)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "data: {") {
		t.Fatal(line, err)
	}
	cancel()
	for _, path := range []string{"/api/telemetry/stats", "/api/telemetry/global_history", "/api/telemetry/history?device_id=" + strings.Repeat("a", 32)} {
		if w := call(t, s, "GET", path, "", nil); w.Code != 200 {
			t.Fatal(path, w.Code, w.Body)
		}
	}
}

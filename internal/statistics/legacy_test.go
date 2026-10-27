package statistics

import (
	"context"
	"encoding/json"
	"testing"
)

func TestLegacyAPOpsiItemsPaginationAndCorrection(t *testing.T) {
	s := service(t)
	r := fixture(t)
	device := r.SHA256[:32]
	r.Source.Metadata.DeviceID = device
	r.Source.Metadata.Scene = "opsi_meowfficer_farming"
	r.Source.Metadata.CombatCount = 12
	r.Events[0].Zone = "NA"
	r.Events[0].ZoneType = "SAFE"
	r.Events[0].ZoneID = 22
	r.Events[0].HazardLevel = 3
	r.Events[0].Items = []Item{{Item: "Coins", Amount: 20, Tag: "meow"}, {Item: "Fuel", Amount: 10, Tag: "log"}, {Item: "Parts", Amount: 2, Tag: "scan"}}
	t.Cleanup(func() { _, _ = s.Pool.Exec(context.Background(), `DELETE FROM stat_results WHERE sha256=$1`, r.SHA256) })
	for _, amount := range []int64{20, 25} {
		r.Events[0].Items[0].Amount = amount
		if ack, err := s.Apply(context.Background(), r); err != nil || ack.Status != "accepted" {
			t.Fatal(ack, err)
		}
		r.Revision++
	}
	get := func(query string) []map[string]any {
		w := call(t, s, "GET", "/api/data/opsi_items?device_id="+device+query, "", nil)
		var out struct{ Data []map[string]any }
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || w.Code != 200 {
			t.Fatal(w.Code, w.Body, err)
		}
		return out.Data
	}
	rows := get("")
	if len(rows) != 3 || rows[0]["amount"] != float64(25) || rows[0]["combat_count"] != float64(12) || rows[0]["hazard_level"] != float64(3) || rows[0]["imgid"] != r.SHA256 {
		t.Fatal(rows)
	}
	if rows[1]["tag"] != "log" || rows[2]["tag"] != "scan" {
		t.Fatal(rows)
	}
	if page := get("&limit=1&offset=1"); len(page) != 1 || page[0]["item"] != "Fuel" {
		t.Fatal(page)
	}
	if len(get("&offset=3")) != 0 || len(get("&genre=opsi_hazard1_leveling")) != 0 {
		t.Fatal("pagination/genre")
	}
	r.Status = "needs_review"
	r.Diagnostics = []string{"test-review"}
	if ack, err := s.Apply(context.Background(), r); err != nil || ack.Status != "accepted" {
		t.Fatal(ack, err)
	}
	if len(get("")) != 0 {
		t.Fatal("unverified result leaked into legacy query")
	}
}

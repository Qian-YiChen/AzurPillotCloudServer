package statistics

import (
	"fmt"
	"net/url"
	"testing"
)

func TestSummaryReturnsAllRowsBeyondOldLimits(t *testing.T) {
	s := service(t)
	r := fixture(t)
	r.Source.Metadata.Project = "research"
	r.Events = nil
	const count = 1200
	for start := 0; start < count; start += 200 {
		e := Event{Type: "complete", Series: 9, Project: "Q-ALL", Duration: 3600}
		for i := start; i < start+200; i++ {
			e.Items = append(e.Items, Item{Item: fmt.Sprintf("Item%04d", i), Amount: 2})
		}
		r.Events = append(r.Events, e)
	}
	applyDimensionResult(t, s, r)
	query := url.Values{"server": {"cn"}, "scene": {r.Source.Metadata.Scene}}
	type group struct {
		Project string `json:"project"`
		Samples int    `json:"samples"`
	}
	type item struct {
		Project string `json:"project"`
		Item    string `json:"item"`
		Samples int    `json:"samples"`
		Amount  int    `json:"amount"`
	}
	var reply struct {
		Groups []group `json:"groups"`
		Items  []item  `json:"items"`
		Daily  []struct {
			Samples int `json:"samples"`
		} `json:"daily"`
	}
	readDimensionQuery(t, s, "/api/v2/get/research?"+query.Encode(), &reply)
	if len(reply.Groups) != 1 || len(reply.Items) != count {
		t.Fatalf("wide reward query lost rows: groups=%d items=%d", len(reply.Groups), len(reply.Items))
	}
	for _, row := range reply.Items {
		if row.Samples != 6 || row.Amount != 2 {
			t.Fatalf("wide query changed reward totals or denominator: %+v", row)
		}
	}
	// Grow project dimensions independently of item dimensions. Both arrays must
	// stay complete, including the final row, with no change to sample totals.
	for start := 0; start < count; start += 200 {
		added := fixture(t)
		added.Source.Metadata = r.Source.Metadata
		added.Events = nil
		for i := start; i < start+200; i++ {
			added.Events = append(added.Events, Event{Type: "complete", Series: 9, Project: fmt.Sprintf("Q-%04d", i), Items: []Item{{Item: "Coins", Amount: 1}}})
		}
		applyDimensionResult(t, s, added)
	}
	readDimensionQuery(t, s, "/api/v2/get/research?"+query.Encode(), &reply)
	if len(reply.Groups) != count+1 || len(reply.Items) != count*2 {
		t.Fatalf("wide project query lost rows: groups=%d items=%d", len(reply.Groups), len(reply.Items))
	}
	var samples, amount int
	for _, row := range reply.Groups {
		samples += row.Samples
	}
	for _, row := range reply.Items {
		amount += row.Amount
	}
	if samples != count+6 || amount != count*3 || len(reply.Daily) != 1 || reply.Daily[0].Samples != samples {
		t.Fatalf("wide query changed totals: samples=%d amount=%d daily=%+v", samples, amount, reply.Daily)
	}
	if reply.Groups[len(reply.Groups)-1].Project != "Q-ALL" || reply.Items[len(reply.Items)-1].Item != "Item1199" {
		t.Fatal("last aggregate rows were truncated")
	}
	var slots struct {
		Slots []group `json:"slots"`
	}
	readDimensionQuery(t, s, "/api/v2/get/research/time-slots?"+query.Encode(), &slots)
	if len(slots.Slots) != count+1 {
		t.Fatalf("time slots disagree with complete project groups: %d", len(slots.Slots))
	}
}

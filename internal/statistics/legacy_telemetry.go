package statistics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func ratio(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func (s *Service) legacyTelemetryStats(ctx context.Context) (map[string]any, error) {
	var devices, battles, rounds, encounters, ap int64
	err := s.Pool.QueryRow(ctx, `WITH latest AS (SELECT DISTINCT ON(device_id,instance_id) * FROM cl1_snapshots ORDER BY device_id,instance_id,month DESC) SELECT count(*),COALESCE(sum(battle_count),0)::bigint,COALESCE(sum(battle_rounds),0)::bigint,COALESCE(sum(akashi_encounters),0)::bigint,COALESCE(sum(akashi_ap),0)::bigint FROM latest WHERE updated_at>$1`, s.Now().Add(-24*time.Hour)).Scan(&devices, &battles, &rounds, &encounters, &ap)
	cost := rounds * 5 // Historical dashboard accounting; v2 retains its own formula.
	return map[string]any{"total_devices": devices, "total_battle_count": battles, "total_battle_rounds": rounds, "total_sortie_cost": cost, "total_akashi_encounters": encounters, "avg_akashi_probability": ratio(encounters, rounds), "avg_stamina": ratio(ap, encounters), "total_stamina_gain": ap, "net_stamina_gain": ap - cost, "cycle_efficiency": ratio(ap-cost, cost)}, err
}

func (s *Service) legacyTelemetryQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "METHOD_NOT_ALLOWED", "GET required")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/stats") {
		data, err := s.legacyTelemetryStats(r.Context())
		if err != nil {
			internal(w, err)
			return
		}
		output(w, 200, data)
		return
	}
	device := ""
	if strings.HasSuffix(r.URL.Path, "/history") {
		device = strings.ToLower(r.URL.Query().Get("device_id"))
		if !legacyDevicePattern.MatchString(device) {
			fail(w, 400, "VALIDATION_ERROR", "完整 device_id is required")
			return
		}
		if len(device) != 32 {
			hash := sha256.Sum256([]byte("legacy-device\x00" + device))
			device = hex.EncodeToString(hash[:16])
		}
	}
	rows, err := s.Pool.Query(r.Context(), `SELECT month,sum(battle_count)::bigint,sum(battle_rounds)::bigint,sum(sortie_cost)::bigint,sum(akashi_encounters)::bigint,sum(akashi_ap)::bigint,max(updated_at) FROM cl1_snapshots WHERE ($1='' OR device_id=$1) GROUP BY month ORDER BY month DESC`, device)
	if err != nil {
		internal(w, err)
		return
	}
	defer rows.Close()
	history := []map[string]any{}
	var battles, rounds, cost, encounters, ap int64
	for rows.Next() {
		var month string
		var b, br, c, e, a int64
		var at time.Time
		if err = rows.Scan(&month, &b, &br, &c, &e, &a, &at); err != nil {
			internal(w, err)
			return
		}
		history = append(history, map[string]any{"month": month, "battle_count": b, "battle_rounds": br, "sortie_cost": c, "akashi_encounters": e, "net_stamina_gain": a - br*5, "average_stamina": ratio(a, e), "updated_at": at})
		battles += b
		rounds += br
		cost += c
		encounters += e
		ap += a
	}
	if err = rows.Err(); err != nil {
		internal(w, err)
		return
	}
	total := map[string]any{"battle_count": battles, "battle_rounds": rounds, "sortie_cost": cost, "akashi_encounters": encounters, "akashi_probability": ratio(encounters, rounds), "average_stamina": ratio(ap, encounters), "net_stamina_gain": ap - rounds*5}
	data := map[string]any{"total": total, "history": history}
	if device != "" {
		data["device_id"] = device
		data["username"] = "未知指挥官"
	} else {
		var n int64
		if err = s.Pool.QueryRow(r.Context(), `SELECT count(DISTINCT (device_id,instance_id)) FROM cl1_snapshots`).Scan(&n); err != nil {
			internal(w, err)
			return
		}
		total["total_devices"] = n
		total["sortie_cost"] = rounds * 5
		total["cycle_efficiency"] = ratio(ap-rounds*5, rounds*5)
	}
	output(w, 200, data)
}

func (s *Service) legacyTelemetryStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "METHOD_NOT_ALLOWED", "GET required")
		return
	}
	data, err := s.legacyTelemetryStats(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	send := func(payload string) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprint(w, payload); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	raw, _ := json.Marshal(data)
	if !send("data: " + string(raw) + "\n\n") {
		return
	}
	previous := string(raw)
	poll := time.NewTicker(5 * time.Second)
	defer poll.Stop()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if !send(": heartbeat\n\n") {
				return
			}
		case <-poll.C:
			ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
			data, err = s.legacyTelemetryStats(ctx)
			cancel()
			if err != nil {
				return
			}
			raw, _ = json.Marshal(data)
			if string(raw) != previous {
				if !send("data: " + string(raw) + "\n\n") {
					return
				}
				previous = string(raw)
			}
		}
	}
}

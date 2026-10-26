package statistics

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var legacyDevicePattern = regexp.MustCompile(`^[0-9a-fA-F]{32,64}$`)

type legacyItem struct {
	Item   string `json:"item"`
	Amount int64  `json:"amount"`
	IsMeow bool   `json:"is_meow"`
}
type legacyBody struct {
	Zone     string       `json:"zone"`
	ZoneType string       `json:"zone_type"`
	ZoneID   int          `json:"zone_id"`
	Hazard   int          `json:"hazard_level"`
	Combat   int64        `json:"combat_count"`
	Items    []legacyItem `json:"items"`
}
type legacyReport struct {
	Device      string     `json:"device_id"`
	Task        string     `json:"task"`
	Body        legacyBody `json:"body"`
	RequestID   string     `json:"request_id,omitempty"`
	CollectedAt string     `json:"collected_at,omitempty"`
}

func (s *Service) legacySubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		fail(w, 405, "METHOD_NOT_ALLOWED", "POST required")
		return
	}
	var req legacyReport
	if !decode(w, r, &req) {
		return
	}
	req.Device = strings.ToLower(req.Device)
	if !legacyDevicePattern.MatchString(req.Device) || (req.Task != "opsi_hazard1_leveling" && req.Task != "opsi_meowfficer_farming") || req.Body.Hazard < 1 || req.Body.Hazard > 6 || req.Body.Combat < 1 || req.Body.Combat > 1e9 || req.Body.ZoneID < 0 || req.Body.ZoneID > 10000 || len(req.Body.Zone) > 128 || len(req.Body.ZoneType) > 32 || len(req.Body.Items) == 0 || len(req.Body.Items) > 1000 {
		fail(w, 400, "VALIDATION_ERROR", "无效旧版上报")
		return
	}
	for i := range req.Body.Items {
		req.Body.Items[i].Item = strings.TrimSpace(req.Body.Items[i].Item)
		if req.Body.Items[i].Item == "" || len(req.Body.Items[i].Item) > 256 || req.Body.Items[i].Amount < 1 || req.Body.Items[i].Amount > 1e9 {
			fail(w, 400, "VALIDATION_ERROR", "无效物品")
			return
		}
	}
	at := s.Now().UTC()
	if req.CollectedAt != "" {
		var err error
		at, err = time.Parse(time.RFC3339Nano, req.CollectedAt)
		if err != nil {
			fail(w, 400, "VALIDATION_ERROR", "collected_at 需要带时区")
			return
		}
	}
	key := req.RequestID
	if h := r.Header.Get("Idempotency-Key"); h != "" {
		if key != "" && key != h {
			fail(w, 400, "VALIDATION_ERROR", "幂等标识冲突")
			return
		}
		key = h
	}
	if len(key) > 128 || strings.ContainsAny(key, "\x00\r\n") {
		fail(w, 400, "VALIDATION_ERROR", "无效幂等标识")
		return
	}
	if key == "" {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			internal(w, err)
			return
		}
		key = hex.EncodeToString(id[:])
	}
	key = req.Device + ":" + key
	req.RequestID = ""
	raw, _ := json.Marshal(req)
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		internal(w, err)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	var id int64
	err = tx.QueryRow(r.Context(), `INSERT INTO legacy_azurstat_reports(request_key,content_hash,device_id,task,zone,zone_type,zone_id,hazard_level,combat_count,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(request_key) DO NOTHING RETURNING id`, key, digest, req.Device, req.Task, req.Body.Zone, req.Body.ZoneType, req.Body.ZoneID, req.Body.Hazard, req.Body.Combat, at).Scan(&id)
	duplicate := false
	if errors.Is(err, pgx.ErrNoRows) {
		var previous string
		err = tx.QueryRow(r.Context(), `SELECT id,content_hash,occurred_at FROM legacy_azurstat_reports WHERE request_key=$1`, key).Scan(&id, &previous, &at)
		if err != nil {
			internal(w, err)
			return
		}
		if previous != digest {
			fail(w, 409, "REQUEST_CONFLICT", "幂等标识正文冲突")
			return
		}
		duplicate = true
	} else if err != nil {
		internal(w, err)
		return
	}
	if !duplicate {
		for i, item := range req.Body.Items {
			if _, err = tx.Exec(r.Context(), `INSERT INTO legacy_azurstat_items(report_id,ordinal,item,amount,is_meow) VALUES($1,$2,$3,$4,$5)`, id, i, item.Item, item.Amount, item.IsMeow); err != nil {
				internal(w, err)
				return
			}
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		internal(w, err)
		return
	}
	output(w, 200, map[string]any{"status": "success", "message": "AzurStat report saved", "report_id": id, "device_id": req.Device, "item_count": len(req.Body.Items), "created_at": at, "combat_count": req.Body.Combat, "duplicate": duplicate})
}

func legacyFilters(r *http.Request) (string, []any, error) {
	q := r.URL.Query()
	hazard := 0
	var err error
	if v := q.Get("hazard_level"); v != "" {
		hazard, err = strconv.Atoi(v)
		if err != nil || hazard < 1 || hazard > 6 {
			return "", nil, fmt.Errorf("invalid hazard_level")
		}
	}
	zone := -1
	if v := q.Get("zone_id"); v != "" {
		zone, err = strconv.Atoi(v)
		if err != nil || zone < 0 {
			return "", nil, fmt.Errorf("invalid zone_id")
		}
	}
	var start, end *time.Time
	for _, v := range []struct {
		key    string
		target **time.Time
	}{{"start_date", &start}, {"end_date", &end}} {
		if raw := q.Get(v.key); raw != "" {
			t, e := time.Parse("2006-01-02", raw)
			if e != nil {
				return "", nil, e
			}
			if v.key == "end_date" {
				t = t.AddDate(0, 0, 1)
			}
			*v.target = &t
		}
	}
	if start != nil && end != nil && !start.Before(*end) {
		return "", nil, fmt.Errorf("invalid date range")
	}
	var meow *bool
	if v := q.Get("is_meow"); v != "" {
		b, e := strconv.ParseBool(v)
		if e != nil {
			return "", nil, e
		}
		meow = &b
	}
	device := strings.ToLower(q.Get("device_id"))
	if device != "" && !legacyDevicePattern.MatchString(device) {
		return "", nil, fmt.Errorf("invalid device_id")
	}
	args := []any{q.Get("task"), zone, hazard, start, end, device, meow}
	cte := `WITH reports AS (SELECT * FROM legacy_report_view WHERE ($1='' OR task=$1) AND ($2::int<0 OR zone_id=$2) AND ($3::int=0 OR hazard_level=$3) AND ($4::timestamptz IS NULL OR occurred_at>=$4) AND ($5::timestamptz IS NULL OR occurred_at<$5) AND ($6='' OR device_id=$6)),items AS (SELECT i.* FROM legacy_item_view i JOIN reports r USING(report_key) WHERE ($7::bool IS NULL OR i.is_meow=$7)) `
	return cte, args, nil
}

func (s *Service) legacyQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "METHOD_NOT_ALLOWED", "GET required")
		return
	}
	cte, args, err := legacyFilters(r)
	if err != nil {
		fail(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	queries := map[string]string{
		"stats": `SELECT (SELECT count(*) FROM reports) AS total_reports,(SELECT count(DISTINCT device_id) FROM reports) AS total_devices,(SELECT COALESCE(sum(combat_count),0)::bigint FROM reports) AS total_combat_count,COALESCE(sum(amount),0)::bigint AS total_item_amount,count(DISTINCT item) AS total_item_types FROM items`,
		"items": `SELECT item,sum(amount)::bigint AS total_amount,count(DISTINCT report_key) AS drop_reports,COALESCE(sum(amount) FILTER(WHERE is_meow),0)::bigint AS meow_amount,COALESCE(sum(amount) FILTER(WHERE NOT is_meow),0)::bigint AS normal_amount,COALESCE(sum(amount)::float8/NULLIF((SELECT sum(combat_count) FROM reports),0),0) AS avg_per_combat FROM items GROUP BY item ORDER BY total_amount DESC,item LIMIT $8`,
	}
	if name == "filters" {
		rows, e := s.Pool.Query(r.Context(), `SELECT DISTINCT task,zone_id,hazard_level FROM legacy_report_view ORDER BY task,zone_id,hazard_level`)
		if e != nil {
			internal(w, e)
			return
		}
		defer rows.Close()
		tasks, zones, levels := map[string]bool{}, map[int]bool{}, map[int]bool{}
		for rows.Next() {
			var task string
			var zone, h int
			if e = rows.Scan(&task, &zone, &h); e != nil {
				internal(w, e)
				return
			}
			tasks[task] = true
			zones[zone] = true
			levels[h] = true
		}
		if e = rows.Err(); e != nil {
			internal(w, e)
			return
		}
		output(w, 200, legacyFilterOutput(tasks, zones, levels))
		return
	}
	query := queries[name]
	if name == "items" {
		limit, e := legacyLimit(r, "limit", 50, 200)
		if e != nil {
			fail(w, 400, "VALIDATION_ERROR", e.Error())
			return
		}
		args = append(args, limit)
	}
	interval := "day"
	if name == "history" {
		if v := r.URL.Query().Get("interval"); v != "" {
			interval = v
		}
		format := "YYYY-MM-DD"
		if interval == "month" {
			format = "YYYY-MM"
		} else if interval != "day" {
			fail(w, 400, "VALIDATION_ERROR", "invalid interval")
			return
		}
		query = `SELECT to_char(r.occurred_at AT TIME ZONE 'UTC','` + format + `') AS date,count(*) AS report_count,sum(r.combat_count)::bigint AS combat_count,COALESCE(sum(i.amount),0)::bigint AS item_amount,count(DISTINCT r.device_id) AS device_count FROM reports r LEFT JOIN (SELECT report_key,sum(amount) AS amount FROM items GROUP BY report_key) i USING(report_key) GROUP BY date ORDER BY date DESC`
	}
	rows, err := s.Pool.Query(r.Context(), cte+query, args...)
	if err != nil {
		internal(w, err)
		return
	}
	data, err := collect(rows)
	if err != nil {
		internal(w, err)
		return
	}
	switch name {
	case "stats":
		output(w, 200, data[0])
	case "items":
		output(w, 200, map[string]any{"items": data, "total": len(data)})
	case "history":
		output(w, 200, map[string]any{"interval": interval, "history": data})
	}
}

func legacyLimit(r *http.Request, key string, fallback, maximum int) (int, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback, nil
	}
	n, e := strconv.Atoi(raw)
	if e != nil || n < 1 {
		return 0, fmt.Errorf("invalid %s", key)
	}
	if n > maximum {
		n = maximum
	}
	return n, nil
}

func legacyFilterOutput(tasks map[string]bool, zones, levels map[int]bool) map[string]any {
	t := []string{}
	z := []string{}
	h := []int{}
	for v := range tasks {
		t = append(t, v)
	}
	for v := range zones {
		z = append(z, strconv.Itoa(v))
	}
	for v := range levels {
		h = append(h, v)
	}
	sort.Strings(t)
	sort.Strings(z)
	sort.Ints(h)
	return map[string]any{"tasks": t, "zones": z, "hazard_levels": h}
}

func (s *Service) legacyOpsiItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		fail(w, 405, "METHOD_NOT_ALLOWED", "GET required")
		return
	}
	q := r.URL.Query()
	device := strings.ToLower(q.Get("device_id"))
	if !legacyDevicePattern.MatchString(device) {
		fail(w, 400, "VALIDATION_ERROR", "device_id is required")
		return
	}
	limit, err := legacyLimit(r, "limit", 1000, 1000)
	if err != nil {
		fail(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	offset := 0
	if v := q.Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil || offset < 0 || offset > 10000000 {
			fail(w, 400, "VALIDATION_ERROR", "invalid offset")
			return
		}
	}
	genre := q.Get("genre")
	if genre == "" {
		genre = "opsi_meowfficer_farming"
	}
	rows, err := s.Pool.Query(r.Context(), `SELECT r.report_key AS imgid,r.server,r.zone,r.zone_type,r.zone_id,r.hazard_level,r.combat_count,i.item,i.amount,i.tag,r.occurred_at AS created_at FROM legacy_report_view r JOIN legacy_item_view i USING(report_key) WHERE r.device_id=$1 AND r.task=$2 ORDER BY r.occurred_at,r.report_key,i.ordinal LIMIT $3 OFFSET $4`, device, genre, limit, offset)
	if err != nil {
		internal(w, err)
		return
	}
	data, err := collect(rows)
	if err != nil {
		internal(w, err)
		return
	}
	output(w, 200, map[string]any{"data": data, "limit": limit, "offset": offset})
}

func legacyPublic(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-AzurPilot-ID, X-Device-Id")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next(w, r)
	}
}

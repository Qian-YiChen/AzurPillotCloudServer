package statistics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
)

func collect(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		v, e := rows.Values()
		if e != nil {
			return nil, e
		}
		m := map[string]any{}
		for i, f := range rows.FieldDescriptions() {
			m[f.Name] = v[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Service) summary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	for k, v := range q {
		if len(v) != 1 || !strings.Contains("|from|to|server|scene|series|project|focus_series|timezone|", "|"+k+"|") {
			fail(w, 400, "VALIDATION_ERROR", "无效筛选参数")
			return
		}
	}
	to := s.Now().UTC()
	from := to.AddDate(0, 0, -30)
	var e error
	if q.Get("to") != "" {
		to, e = time.Parse(time.RFC3339Nano, q.Get("to"))
		if e != nil {
			fail(w, 400, "VALIDATION_ERROR", "to 需要带时区")
			return
		}
	}
	if q.Get("from") != "" {
		from, e = time.Parse(time.RFC3339Nano, q.Get("from"))
		if e != nil {
			fail(w, 400, "VALIDATION_ERROR", "from 需要带时区")
			return
		}
	}
	series := 0
	if q.Get("series") != "" {
		series, e = strconv.Atoi(q.Get("series"))
		if e != nil || series < 1 || series > 100 {
			fail(w, 400, "VALIDATION_ERROR", "无效期数")
			return
		}
	}
	focus := 0
	if q.Get("focus_series") != "" {
		focus, e = strconv.Atoi(q.Get("focus_series"))
		if e != nil || focus < 1 || focus > 100 {
			fail(w, 400, "VALIDATION_ERROR", "无效科研倾向期数")
			return
		}
	}
	zone := q.Get("timezone")
	if zone == "" {
		zone = "Asia/Shanghai"
	}
	if _, e = time.LoadLocation(zone); e != nil {
		fail(w, 400, "VALIDATION_ERROR", "无效时区")
		return
	}
	if !from.Before(to) || to.Sub(from) > 366*24*time.Hour {
		fail(w, 400, "VALIDATION_ERROR", "查询范围必须在 366 天以内")
		return
	}
	kind := "drop"
	if strings.Contains(r.URL.Path, "/research") {
		kind = "research"
	}
	args := []any{kind, from, to, q.Get("server"), q.Get("scene"), series, q.Get("project"), focus}
	filtered := `WITH base AS (SELECT * FROM stat_events WHERE kind=$1 AND occurred_at >=$2 AND occurred_at<$3 AND ($4='' OR server=$4) AND ($5='' OR scene=$5) AND ($6::int=0 OR series=$6) AND ($8::int=0 OR focus_series=$8)),filtered AS (SELECT * FROM base WHERE ($7='' OR project=$7)) `
	tx, e := s.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		internal(w, e)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if strings.HasSuffix(r.URL.Path, "/time-slots") {
		rows, err := tx.Query(r.Context(), filtered+`,bucketed AS (SELECT *, (extract(hour FROM occurred_at AT TIME ZONE $9)*2+floor(extract(minute FROM occurred_at AT TIME ZONE $9)/30))::int AS slot FROM base),totals AS (SELECT slot,count(*) FILTER(WHERE event_type='refresh') AS appearances FROM bucketed GROUP BY slot) SELECT b.slot AS slot_index,to_char(make_time(b.slot/2,(b.slot%2)*30,0),'HH24:MI') AS slot_start,b.series,b.project,b.scene,count(*) FILTER(WHERE b.event_type='refresh') AS appearances,count(*) FILTER(WHERE b.event_type<>'refresh') AS samples,t.appearances AS total_appearances,COALESCE((count(*) FILTER(WHERE b.event_type='refresh'))::float8/NULLIF(t.appearances,0),0) AS appearance_rate FROM bucketed b JOIN totals t USING(slot) WHERE ($7='' OR b.project=$7) GROUP BY b.slot,b.series,b.project,b.scene,t.appearances ORDER BY b.slot,b.series,b.project,b.scene LIMIT 10001`, append(args, zone)...)
		if err != nil {
			internal(w, err)
			return
		}
		slots, err := collect(rows)
		if err != nil {
			internal(w, err)
			return
		}
		if len(slots) > 10000 {
			fail(w, 400, "QUERY_TOO_BROAD", "请缩小时段筛选范围")
			return
		}
		if err = tx.Commit(r.Context()); err != nil {
			internal(w, err)
			return
		}
		output(w, 200, map[string]any{"schema_version": 2, "from": from, "to": to, "timezone": zone, "interval_minutes": 30, "slots": slots})
		return
	}
	rows, e := tx.Query(r.Context(), filtered+`SELECT series,project,scene,count(*) FILTER(WHERE event_type='refresh') AS appearances,count(*) FILTER(WHERE event_type<>'refresh') AS samples,COALESCE(sum(duration_seconds) FILTER(WHERE event_type<>'refresh'),0)::bigint AS duration_seconds FROM filtered GROUP BY series,project,scene ORDER BY series,project,scene LIMIT 1001`, args...)
	if e != nil {
		internal(w, e)
		return
	}
	groups, e := collect(rows)
	if e != nil {
		internal(w, e)
		return
	}
	if len(groups) > 1000 {
		fail(w, 400, "QUERY_TOO_BROAD", "请缩小筛选范围")
		return
	}
	rows, e = tx.Query(r.Context(), filtered+`, denominators AS (SELECT series,project,scene,count(*) AS samples,sum(duration_seconds) AS seconds,count(*) FILTER(WHERE duration_seconds>0) AS timed_samples FROM filtered WHERE event_type<>'refresh' GROUP BY series,project,scene) SELECT e.series,e.project,e.scene,i.item,i.tag,count(*) AS occurrences,sum(i.amount)::bigint AS amount,d.samples,d.timed_samples,round(sum(i.amount)::numeric/d.samples,6)::float8 AS average_per_sample,round(count(*)::numeric/d.samples,6)::float8 AS drop_rate,CASE WHEN d.seconds>0 THEN round(COALESCE(sum(i.amount) FILTER(WHERE e.duration_seconds>0),0)::numeric*3600/d.seconds,6)::float8 ELSE NULL END AS amount_per_hour FROM filtered e JOIN stat_items i ON i.sha256=e.sha256 AND i.ordinal=e.ordinal JOIN denominators d USING(series,project,scene) WHERE e.event_type<>'refresh' GROUP BY e.series,e.project,e.scene,i.item,i.tag,d.samples,d.seconds,d.timed_samples ORDER BY e.series,e.project,e.scene,i.item,i.tag LIMIT 1001`, args...)
	if e != nil {
		internal(w, e)
		return
	}
	items, e := collect(rows)
	if e != nil {
		internal(w, e)
		return
	}
	if len(items) > 1000 {
		fail(w, 400, "QUERY_TOO_BROAD", "请缩小筛选范围")
		return
	}
	rows, e = tx.Query(r.Context(), filtered+`SELECT (occurred_at AT TIME ZONE 'UTC')::date::text AS day,count(*) FILTER(WHERE event_type<>'refresh') AS samples,count(*) FILTER(WHERE event_type='refresh') AS appearances FROM filtered GROUP BY day ORDER BY day`, args...)
	if e != nil {
		internal(w, e)
		return
	}
	daily, e := collect(rows)
	if e != nil {
		internal(w, e)
		return
	}
	var appearances int64
	if e = tx.QueryRow(r.Context(), filtered+`SELECT count(*) FROM base WHERE event_type='refresh'`, args...).Scan(&appearances); e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		internal(w, e)
		return
	}
	for _, g := range groups {
		rate := 0.0
		if appearances > 0 {
			rate = float64(g["appearances"].(int64)) / float64(appearances)
		}
		g["appearance_rate"] = rate
	}
	output(w, 200, map[string]any{"schema_version": 2, "from": from, "to": to, "groups": groups, "items": items, "daily": daily})
}

func (s *Service) adminList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "overview") {
		counts := map[string]int64{}
		for _, name := range []string{"complete", "needs_review", "invalid"} {
			var n int64
			if e := s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM stat_results WHERE status=$1`, name).Scan(&n); e != nil {
				internal(w, e)
				return
			}
			counts[name] = n
		}
		var pending int64
		if e := s.Pool.QueryRow(r.Context(), `SELECT count(*) FROM template_candidates WHERE status='pending'`).Scan(&pending); e != nil {
			internal(w, e)
			return
		}
		rows, e := s.Pool.Query(r.Context(), `SELECT id,payload,updated_at,updated_at>now()-interval '90 seconds' AS online FROM stat_workers ORDER BY id`)
		if e != nil {
			internal(w, e)
			return
		}
		workers, e := collect(rows)
		if e != nil {
			internal(w, e)
			return
		}
		output(w, 200, map[string]any{"counts": counts, "pending_candidates": pending, "workers": workers})
		return
	}
	queries := map[string]string{
		"results":             `SELECT sha256,revision,status,payload,updated_at FROM stat_results WHERE ($1='' OR status=$1) ORDER BY updated_at DESC LIMIT 100`,
		"template-candidates": `SELECT c.*, (SELECT count(*) FROM candidate_sources cs WHERE cs.candidate_id=c.id) AS occurrences, (SELECT count(DISTINCT sample_sha256) FROM candidate_sources cs WHERE cs.candidate_id=c.id) AS blocked_samples, (SELECT jsonb_agg(metadata||jsonb_build_object('sample_sha256',sample_sha256,'occurrence',occurrence)) FROM (SELECT * FROM candidate_sources WHERE candidate_id=c.id LIMIT 20) x) AS sources FROM template_candidates c WHERE ($1='' OR status=$1) ORDER BY last_seen DESC LIMIT 100`,
		"templates":           `SELECT id,item_id,name,category,rarity,series,scene,enabled,image FROM stat_templates WHERE ($1='' OR item_id=$1) ORDER BY item_id,id LIMIT 1000`,
		"template-releases":   `SELECT id,sha256,created_at,id=(SELECT release_id FROM template_active WHERE id) AS active FROM template_releases WHERE ($1='') ORDER BY id DESC LIMIT 30`,
		"worker-commands":     `SELECT * FROM worker_commands WHERE ($1='' OR state=$1) ORDER BY id DESC LIMIT 30`,
	}
	name := path[strings.LastIndex(path, "/")+1:]
	query, ok := queries[name]
	if !ok {
		fail(w, 404, "NOT_FOUND", "未知查询")
		return
	}
	status := r.URL.Query().Get("status")
	if name == "templates" {
		status = r.URL.Query().Get("item_id")
	}
	if name == "template-releases" {
		status = ""
	}
	rows, e := s.Pool.Query(r.Context(), query, status)
	if e != nil {
		internal(w, e)
		return
	}
	data, e := collect(rows)
	if e != nil {
		internal(w, e)
		return
	}
	output(w, 200, map[string]any{"items": data})
}

func commandPayload(sources []json.RawMessage, templates []Template) []byte {
	b, _ := json.Marshal(map[string]any{"sources": sources, "templates": templates})
	return b
}
func validateTemplate(t Template) error {
	if !lootItemRE.MatchString(t.ItemID) || t.Name == "" || len(t.Name) > 128 || t.Series < 0 || t.Series > 100 || t.Rarity < 0 || t.Rarity > 6 {
		return fmt.Errorf("物品标识、名称、期数或稀有度无效")
	}
	switch t.Category {
	case "ship_blueprint", "equipment_blueprint", "consumable", "other":
	default:
		return fmt.Errorf("无效物品分类")
	}
	if t.Scene != "research" && t.Scene != "drop" && t.Scene != "all" {
		return fmt.Errorf("无效模板场景")
	}
	return nil
}

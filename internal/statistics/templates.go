package statistics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/png"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func (s *Service) candidate(w http.ResponseWriter, r *http.Request) {
	var c Candidate
	if !decode(w, r, &c) {
		return
	}
	b, e := base64.StdEncoding.DecodeString(c.Image)
	if e != nil || len(b) > 128<<10 || !hashRE.MatchString(c.ID) || !hashRE.MatchString(c.Sample) || len(c.Occurrence) > 128 || c.Occurrence == "" || (c.Scene != "research" && c.Scene != "drop") {
		fail(w, 400, "VALIDATION_ERROR", "候选图格式无效")
		return
	}
	sum := sha256.Sum256(b)
	if c.ID != hex.EncodeToString(sum[:]) {
		fail(w, 400, "VALIDATION_ERROR", "候选图哈希不一致")
		return
	}
	cfg, e := png.DecodeConfig(bytes.NewReader(b))
	if e != nil || cfg.Width < 8 || cfg.Height < 8 || cfg.Width > 256 || cfg.Height > 256 {
		fail(w, 400, "VALIDATION_ERROR", "候选只接受 8..256 像素 PNG 小图")
		return
	}
	meta, e := json.Marshal(c.Metadata)
	if e != nil || len(meta) > 16384 {
		fail(w, 400, "VALIDATION_ERROR", "候选描述过大")
		return
	}
	tx, e := s.Pool.Begin(r.Context())
	if e != nil {
		internal(w, e)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if _, e = tx.Exec(r.Context(), `INSERT INTO template_candidates(id,scene,image,metadata) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET last_seen=now()`, c.ID, c.Scene, c.Image, meta); e != nil {
		internal(w, e)
		return
	}
	if _, e = tx.Exec(r.Context(), `INSERT INTO candidate_sources(candidate_id,sample_sha256,occurrence,metadata) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, c.ID, c.Sample, c.Occurrence, meta); e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		internal(w, e)
		return
	}
	output(w, 200, map[string]any{"id": c.ID, "accepted": true})
}
func (s *Service) classify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status   string   `json:"status"`
		Template Template `json:"template"`
	}
	if !decode(w, r, &body) {
		return
	}
	id := r.PathValue("id")
	if !hashRE.MatchString(id) || (body.Status != "assigned" && body.Status != "ignored") {
		fail(w, 400, "VALIDATION_ERROR", "无效分类操作")
		return
	}
	if body.Status == "assigned" {
		if e := validateTemplate(body.Template); e != nil {
			fail(w, 400, "VALIDATION_ERROR", e.Error())
			return
		}
	}
	tx, e := s.Pool.Begin(r.Context())
	if e != nil {
		internal(w, e)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	// Take the publication lock before editing, so each snapshot includes all
	// classifications committed before it and cannot overwrite a newer snapshot.
	if _, e = tx.Exec(r.Context(), `SELECT id FROM template_active WHERE id FOR UPDATE`); e != nil {
		internal(w, e)
		return
	}
	var image string
	e = tx.QueryRow(r.Context(), `SELECT image FROM template_candidates WHERE id=$1 FOR UPDATE`, id).Scan(&image)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "NOT_FOUND", "候选不存在")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	if body.Status == "assigned" {
		t := body.Template
		_, e = tx.Exec(r.Context(), `INSERT INTO stat_templates(id,item_id,name,category,rarity,series,scene,enabled,image) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO UPDATE SET item_id=EXCLUDED.item_id,name=EXCLUDED.name,category=EXCLUDED.category,rarity=EXCLUDED.rarity,series=EXCLUDED.series,scene=EXCLUDED.scene,enabled=EXCLUDED.enabled,updated_at=now()`, id, t.ItemID, t.Name, t.Category, t.Rarity, t.Series, t.Scene, t.Enabled, image)
	} else {
		_, e = tx.Exec(r.Context(), `UPDATE stat_templates SET enabled=false,updated_at=now() WHERE id=$1`, id)
	}
	if e != nil {
		internal(w, e)
		return
	}
	if _, e = tx.Exec(r.Context(), `UPDATE template_candidates SET status=$2 WHERE id=$1`, id, body.Status); e != nil {
		internal(w, e)
		return
	}
	ts, e := readTemplates(r.Context(), tx)
	if e != nil {
		internal(w, e)
		return
	}
	packageJSON, e := templatePackage(ts)
	if e != nil {
		fail(w, 400, "VALIDATION_ERROR", e.Error())
		return
	}
	releaseID, queued, e := releaseInTx(r.Context(), tx, packageJSON)
	if e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		internal(w, e)
		return
	}
	output(w, 200, map[string]any{"saved": true, "release_id": releaseID, "queued_samples": queued})
}
func (s *Service) templates(ctx context.Context) ([]Template, error) {
	return readTemplates(ctx, s.Pool)
}

type templateReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readTemplates(ctx context.Context, reader templateReader) ([]Template, error) {
	rows, e := reader.Query(ctx, `SELECT id,item_id,name,category,rarity,series,scene,enabled,image FROM stat_templates ORDER BY id LIMIT 2001`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		var t Template
		if e = rows.Scan(&t.ID, &t.ItemID, &t.Name, &t.Category, &t.Rarity, &t.Series, &t.Scene, &t.Enabled, &t.Image); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func templatePackage(ts []Template) ([]byte, error) {
	if len(ts) > 2000 {
		return nil, errors.New("单版本最多 2000 个模板")
	}
	b, e := json.Marshal(ts)
	if e == nil && len(b) > 32<<20 {
		e = errors.New("模板包超过 32 MiB")
	}
	return b, e
}
func (s *Service) sources(ctx context.Context, ids []string) ([]json.RawMessage, error) {
	rows, e := s.Pool.Query(ctx, `SELECT DISTINCT r.payload->'source' FROM stat_results r WHERE cardinality($1::text[])=0 OR r.sha256=ANY($1::text[]) OR r.sha256 IN(SELECT sample_sha256 FROM candidate_sources WHERE candidate_id=ANY($1::text[])) OR (r.payload->'used_templates') ?| $1::text[] LIMIT 1001`, ids)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
func (s *Service) createRelease(ctx context.Context, payload []byte) (int64, error) {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize publication and rollback so the active pointer is ordered.
	if _, e = tx.Exec(ctx, `SELECT id FROM template_active WHERE id FOR UPDATE`); e != nil {
		return 0, e
	}
	id, _, e := releaseInTx(ctx, tx, payload)
	if e != nil {
		return 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	return id, nil
}

func releaseInTx(ctx context.Context, tx pgx.Tx, payload []byte) (int64, int64, error) {
	sum := sha256.Sum256(payload)
	var e error
	var id int64
	if e = tx.QueryRow(ctx, `INSERT INTO template_releases(sha256,payload) VALUES($1,$2) RETURNING id`, hex.EncodeToString(sum[:]), payload).Scan(&id); e != nil {
		return 0, 0, e
	}
	if _, e = tx.Exec(ctx, `UPDATE template_active SET release_id=$1 WHERE id`, id); e != nil {
		return 0, 0, e
	}
	// Reprocess every current unresolved sample, and samples that previously
	// used managed templates. Each source is durable; batching happens in B.
	// Invalid captures (including duplicate list frames) cannot be fixed by an
	// icon template. Every needs_review sample is retried, even without a candidate.
	queued, e := queueUnresolved(ctx, tx, true)
	if e != nil {
		return 0, 0, e
	}
	return id, queued, nil
}

func queueUnresolved(ctx context.Context, tx pgx.Tx, includeManaged bool) (int64, error) {
	var release *int64
	if e := tx.QueryRow(ctx, `SELECT release_id FROM template_active WHERE id`).Scan(&release); e != nil {
		return 0, e
	}
	var version int64
	if release != nil {
		version = *release
	}
	// A worker may already have fetched a command still marked pending on C.
	// Never merge a new publication into that older command: its local ID is
	// idempotent and it may already have run with the previous templates.
	queued, e := tx.Exec(ctx, `INSERT INTO worker_commands(action,payload) SELECT 'reprocess',jsonb_build_object('sources',jsonb_build_array(r.payload->'source'),'templates','[]'::jsonb,'template_release',$2::bigint) FROM stat_results r WHERE (status='needs_review' OR ($1 AND CASE WHEN jsonb_typeof(r.payload->'used_templates')='array' THEN jsonb_array_length(r.payload->'used_templates') ELSE 0 END>0)) AND NOT EXISTS (SELECT 1 FROM worker_commands c WHERE c.action='reprocess' AND c.state='pending' AND COALESCE((c.payload->>'template_release')::bigint,0)=$2 AND c.payload->'sources' @> jsonb_build_array(r.payload->'source'))`, includeManaged, version)
	return queued.RowsAffected(), e
}

func (s *Service) reprocessUnresolved(w http.ResponseWriter, r *http.Request) {
	tx, e := s.Pool.Begin(r.Context())
	if e != nil {
		internal(w, e)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	// Share the publication lock to merge repeated clicks and classification jobs.
	if _, e = tx.Exec(r.Context(), `SELECT id FROM template_active WHERE id FOR UPDATE`); e != nil {
		internal(w, e)
		return
	}
	queued, e := queueUnresolved(r.Context(), tx, false)
	if e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		internal(w, e)
		return
	}
	output(w, 202, map[string]any{"queued_samples": queued})
}
func (s *Service) publish(w http.ResponseWriter, r *http.Request) {
	tx, e := s.Pool.Begin(r.Context())
	if e != nil {
		internal(w, e)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if _, e = tx.Exec(r.Context(), `SELECT id FROM template_active WHERE id FOR UPDATE`); e != nil {
		internal(w, e)
		return
	}
	ts, e := readTemplates(r.Context(), tx)
	if e != nil {
		internal(w, e)
		return
	}
	b, e := templatePackage(ts)
	if e != nil {
		fail(w, 400, "VALIDATION_ERROR", e.Error())
		return
	}
	id, queued, e := releaseInTx(r.Context(), tx, b)
	if e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		internal(w, e)
		return
	}
	output(w, 200, map[string]any{"release_id": id, "queued_samples": queued})
}
func (s *Service) rollback(w http.ResponseWriter, r *http.Request) {
	var b struct {
		ID int64 `json:"release_id"`
	}
	if !decode(w, r, &b) {
		return
	}
	var payload []byte
	e := s.Pool.QueryRow(r.Context(), `SELECT payload FROM template_releases WHERE id=$1`, b.ID).Scan(&payload)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "NOT_FOUND", "版本不存在")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	id, e := s.createRelease(r.Context(), payload)
	if e != nil {
		internal(w, e)
		return
	}
	output(w, 200, map[string]any{"release_id": id, "restored_from": b.ID})
}
func (s *Service) release(w http.ResponseWriter, r *http.Request) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "VALIDATION_ERROR", "无效版本")
		return
	}
	var data []byte
	var digest string
	e = s.Pool.QueryRow(r.Context(), `SELECT payload,sha256 FROM template_releases WHERE id=$1`, id).Scan(&data, &digest)
	if errors.Is(e, pgx.ErrNoRows) {
		fail(w, 404, "NOT_FOUND", "版本不存在")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-SHA256", digest)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = w.Write(data)
}
func (s *Service) reprocess(w http.ResponseWriter, r *http.Request)    { s.command(w, r, "reprocess") }
func (s *Service) testTemplate(w http.ResponseWriter, r *http.Request) { s.command(w, r, "test") }
func (s *Service) command(w http.ResponseWriter, r *http.Request, action string) {
	var b struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &b) {
		return
	}
	if len(b.IDs) == 0 || len(b.IDs) > 100 {
		fail(w, 400, "VALIDATION_ERROR", "请选择 1..100 个来源或候选")
		return
	}
	for _, id := range b.IDs {
		if !hashRE.MatchString(id) {
			fail(w, 400, "VALIDATION_ERROR", "无效来源标识")
			return
		}
	}
	sources, e := s.sources(r.Context(), b.IDs)
	if e != nil {
		internal(w, e)
		return
	}
	if len(sources) == 0 || len(sources) > 1000 {
		fail(w, 400, "VALIDATION_ERROR", "没有关联样本或超过 1000 份，请缩小范围")
		return
	}
	ts := []Template{}
	if action == "test" {
		ts, e = s.templates(r.Context())
		if e != nil {
			internal(w, e)
			return
		}
	}
	var id int64
	e = s.Pool.QueryRow(r.Context(), `INSERT INTO worker_commands(action,payload) VALUES($1,$2) RETURNING id`, action, commandPayload(sources, ts)).Scan(&id)
	if e != nil {
		internal(w, e)
		return
	}
	output(w, 202, map[string]any{"command_id": id, "samples": len(sources)})
}
func (s *Service) syncWorker(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkerID string `json:"worker_id"`
		Release  int64  `json:"release_id"`
		Pending  int    `json:"pending"`
		Updates  []struct {
			ID     int64           `json:"id"`
			State  string          `json:"state"`
			Result json.RawMessage `json:"result"`
		} `json:"updates"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !itemRE.MatchString(body.WorkerID) || len(body.Updates) > 100 {
		fail(w, 400, "VALIDATION_ERROR", "无效 worker")
		return
	}
	for _, u := range body.Updates {
		if u.State != "running" && u.State != "done" && u.State != "failed" {
			fail(w, 400, "VALIDATION_ERROR", "无效指令状态")
			return
		}
		if len(u.Result) == 0 {
			u.Result = json.RawMessage(`{}`)
		}
		if _, e := s.Pool.Exec(r.Context(), `UPDATE worker_commands SET state=$2,result=$3,updated_at=now() WHERE id=$1 AND state NOT IN ('done','failed')`, u.ID, u.State, []byte(u.Result)); e != nil {
			internal(w, e)
			return
		}
	}
	b, _ := json.Marshal(map[string]any{"release_id": body.Release, "pending": body.Pending})
	if _, e := s.Pool.Exec(r.Context(), `INSERT INTO stat_workers(id,payload) VALUES($1,$2) ON CONFLICT(id) DO UPDATE SET payload=EXCLUDED.payload,updated_at=now()`, body.WorkerID, b); e != nil {
		internal(w, e)
		return
	}
	var release *int64
	// Fetch the active version and commands from one publication snapshot.
	tx, e := s.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		internal(w, e)
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	if e := tx.QueryRow(r.Context(), `SELECT release_id FROM template_active WHERE id`).Scan(&release); e != nil {
		internal(w, e)
		return
	}
	rows, e := tx.Query(r.Context(), `SELECT id,action,payload FROM worker_commands WHERE state='pending' OR (state='running' AND updated_at<now()-interval '10 minutes') ORDER BY id LIMIT 20`)
	if e != nil {
		internal(w, e)
		return
	}
	commands, e := collect(rows)
	if e != nil {
		internal(w, e)
		return
	}
	if e := tx.Commit(r.Context()); e != nil {
		internal(w, e)
		return
	}
	output(w, 200, map[string]any{"release_id": release, "commands": commands})
}

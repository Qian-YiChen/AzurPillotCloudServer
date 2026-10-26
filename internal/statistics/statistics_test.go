package statistics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ap-cloud-server/internal/store"
)

func fixture(t *testing.T) Result {
	t.Helper()
	f := sha256.Sum256([]byte(t.Name() + time.Now().String()))
	sum := sha256.Sum256(f[:])
	hash := hex.EncodeToString(sum[:])
	source := Source{SHA256: hash, Metadata: Metadata{DeviceID: strings.Repeat("a", 32), InstanceID: strings.Repeat("b", 16), Project: "drop", Scene: "test_" + hash[:12], Server: "cn", CollectedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339), Sizes: []int64{8}}, Files: []SourceFile{{Index: 0, Size: 8, SHA256: hex.EncodeToString(f[:]), FileID: 1}}}
	return Result{SHA256: hash, Revision: 1, Status: "complete", Parser: "test-v1", Source: source, Events: []Event{{Type: "drop", Duration: 3600, Items: []Item{{Item: "Coins", Amount: 10}}}}}
}
func service(t *testing.T) *Service {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	st, e := store.Open(context.Background(), url, 24*time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(st.Close)
	return New(st.Pool, "worker-token", "admin-token")
}
func call(t *testing.T, s *Service, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if body != nil {
		var e error
		data, e = json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	m := http.NewServeMux()
	s.Register(m)
	response := httptest.NewRecorder()
	m.ServeHTTP(response, req)
	return response
}

func TestResultValidation(t *testing.T) {
	valid := fixture(t)
	if e := valid.validate(); e != nil {
		t.Fatal(e)
	}
	bad := valid
	bad.Source = valid.Source
	bad.Source.Files = append([]SourceFile{}, valid.Source.Files...)
	bad.Source.Files[0].SHA256 = strings.Repeat("0", 64)
	if bad.validate() == nil {
		t.Fatal("accepted a forged source fingerprint")
	}
	bad = valid
	bad.Events = []Event{{Type: "drop", Items: []Item{{Item: "Coins", Amount: 0}}}}
	if bad.validate() == nil {
		t.Fatal("accepted zero item count")
	}
	bad = valid
	bad.Diagnostics = []string{"missing_template"}
	if bad.validate() == nil {
		t.Fatal("accepted incomplete formal statistics")
	}
}

func TestConfiguredSourceLimits(t *testing.T) {
	r := fixture(t)
	r.Source.Files[0].Size = 20 << 20
	r.Source.Metadata.Sizes[0] = 20 << 20
	if r.validate() == nil {
		t.Fatal("default file limit ignored")
	}
	if e := r.validateLimits(20<<20, 40<<20); e != nil {
		t.Fatal(e)
	}
	if r.validateLimits(20<<20, 19<<20) == nil {
		t.Fatal("group limit ignored")
	}
}

func TestAuthenticatedPreviewAndInvalidIndex(t *testing.T) {
	s := service(t)
	result := fixture(t)
	if _, e := s.Apply(context.Background(), result); e != nil {
		t.Fatal(e)
	}
	var hits atomic.Int32
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Authorization") != "Bearer preview-token" {
			t.Error("preview credential missing")
		}
		var body struct {
			Source Source `json:"source"`
			Index  int    `json:"file_index"`
			Frame  int    `json:"frame_index"`
		}
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil || body.Source.SHA256 != result.SHA256 || body.Index != 0 || body.Frame != 0 {
			t.Error("invalid preview source", e)
		}
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, image.NewRGBA(image.Rect(0, 0, 16, 16)))
	}))
	defer worker.Close()
	s.WithWorker(worker.URL, "preview-token")
	path := "/api/v1/admin/statistics/preview/" + result.SHA256 + "/0?frame=0"
	if r := call(t, s, "GET", path, "", nil); r.Code != 401 {
		t.Fatal(r.Code)
	}
	if r := call(t, s, "GET", strings.Replace(path, "/0?", "/invalid?", 1), "admin-token", nil); r.Code != 400 {
		t.Fatal(r.Code)
	}
	r := call(t, s, "GET", path, "admin-token", nil)
	if r.Code != 200 || r.Header().Get("Cache-Control") != "no-store" || hits.Load() != 1 {
		t.Fatalf("%d %s", r.Code, r.Body.String())
	}
	if _, e := png.Decode(bytes.NewReader(r.Body.Bytes())); e != nil {
		t.Fatal(e)
	}
}
func TestResultReplacementAndConcurrentReplay(t *testing.T) {
	s := service(t)
	r := fixture(t)
	var accepted, duplicates atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			ack, e := s.Apply(context.Background(), r)
			if e != nil {
				t.Error(e)
				return
			}
			switch ack.Status {
			case "accepted":
				accepted.Add(1)
			case "duplicate":
				duplicates.Add(1)
			default:
				t.Errorf("unexpected %+v", ack)
			}
		})
	}
	wg.Wait()
	if accepted.Load() != 1 || duplicates.Load() != 11 {
		t.Fatalf("not idempotent: %d %d", accepted.Load(), duplicates.Load())
	}
	changed := r
	changed.Events = []Event{{Type: "drop", Duration: 3600, Items: []Item{{Item: "Coins", Amount: 4}}}}
	ack, e := s.Apply(context.Background(), changed)
	if e != nil || ack.Status != "conflict" {
		t.Fatalf("same revision overwrote: %+v %v", ack, e)
	}
	changed.Revision = 2
	ack, e = s.Apply(context.Background(), changed)
	if e != nil || ack.Status != "accepted" {
		t.Fatalf("replacement failed %+v %v", ack, e)
	}
	rec := call(t, s, "GET", "/api/v2/get/drop?scene="+r.Source.Metadata.Scene, "", nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var summary struct {
		Items []struct {
			Amount  int64   `json:"amount"`
			Rate    float64 `json:"drop_rate"`
			Average float64 `json:"average_per_sample"`
		} `json:"items"`
	}
	if e = json.Unmarshal(rec.Body.Bytes(), &summary); e != nil {
		t.Fatal(e)
	}
	if len(summary.Items) != 1 || summary.Items[0].Amount != 4 || summary.Items[0].Rate != 1 || summary.Items[0].Average != 4 {
		t.Fatalf("wrong contribution: %s", rec.Body.String())
	}
	changed.Revision = 3
	changed.Status = "needs_review"
	changed.Diagnostics = []string{"uncertain_amount"}
	ack, e = s.Apply(context.Background(), changed)
	if e != nil || ack.Status != "accepted" {
		t.Fatal(ack, e)
	}
	rec = call(t, s, "GET", "/api/v2/get/drop?scene="+r.Source.Metadata.Scene, "", nil)
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("incomplete sample stayed in stats: %s", rec.Body.String())
	}
}
func TestStatisticsAuthenticationAndBounds(t *testing.T) {
	s := service(t)
	for _, path := range []string{"/api/v1/admin/statistics/overview", "/internal/v1/statistics/results/" + strings.Repeat("a", 64)} {
		rec := call(t, s, "GET", path, "", nil)
		if rec.Code != 401 {
			t.Fatalf("unauthenticated %s: %d", path, rec.Code)
		}
	}
	for _, query := range []string{"series=-1", "series=1&series=2", "from=bad", "unknown=1", "from=2020-01-01T00:00:00Z"} {
		rec := call(t, s, "GET", "/api/v2/get/research?"+query, "", nil)
		if rec.Code != 400 {
			t.Fatalf("accepted query %s: %s", query, rec.Body.String())
		}
	}
	rec := call(t, s, "POST", "/internal/v1/statistics/results", "worker-token", map[string]any{"results": []Result{}})
	if rec.Code != 400 {
		t.Fatal(rec.Body.String())
	}
}
func TestCandidateClassificationPublicationAndRollback(t *testing.T) {
	s := service(t)
	r := fixture(t)
	r.Status = "needs_review"
	r.Diagnostics = []string{"missing_template"}
	if ack, e := s.Apply(context.Background(), r); e != nil || ack.Status != "accepted" {
		t.Fatal(ack, e)
	}
	im := image.NewRGBA(image.Rect(0, 0, 49, 49))
	for x := range 49 {
		for y := range 49 {
			im.Set(x, y, color.RGBA{uint8(x * 5), uint8(y * 5), uint8(len(t.Name())), 255})
		}
	}
	var pngBytes bytes.Buffer
	if e := png.Encode(&pngBytes, im); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(pngBytes.Bytes())
	id := hex.EncodeToString(sum[:])
	candidate := Candidate{ID: id, Scene: "drop", Image: base64.StdEncoding.EncodeToString(pngBytes.Bytes()), Sample: r.SHA256, Occurrence: "0:0:0", Metadata: map[string]any{"file_index": 0, "frame_index": 0}}
	for range 2 {
		rec := call(t, s, "POST", "/internal/v1/template-candidates", "worker-token", candidate)
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
	}
	var count int64
	if e := s.Pool.QueryRow(context.Background(), `SELECT count(*) FROM candidate_sources WHERE candidate_id=$1 AND sample_sha256=$2`, id, r.SHA256).Scan(&count); e != nil || count != 1 {
		t.Fatal("candidate replay increased occurrence", count, e)
	}
	template := Template{ItemID: "TestCoins", Name: "物资", Category: "consumable", Scene: "drop", Enabled: true}
	rec := call(t, s, "PATCH", "/api/v1/admin/template-candidates/"+id, "admin-token", map[string]any{"status": "assigned", "template": template})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	rec = call(t, s, "POST", "/api/v1/admin/templates/test", "admin-token", map[string]any{"ids": []string{id}})
	if rec.Code != 202 {
		t.Fatal(rec.Body.String())
	}
	rec = call(t, s, "POST", "/api/v1/admin/templates/publish", "admin-token", nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var published struct {
		ID int64 `json:"release_id"`
	}
	if e := json.Unmarshal(rec.Body.Bytes(), &published); e != nil {
		t.Fatal(e)
	}
	rec = call(t, s, "GET", "/internal/v1/template-releases/"+jsonNumber(published.ID), "worker-token", nil)
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	digest := sha256.Sum256(rec.Body.Bytes())
	if rec.Header().Get("X-Content-SHA256") != hex.EncodeToString(digest[:]) {
		t.Fatal("release checksum mismatch")
	}
	rec = call(t, s, "POST", "/api/v1/admin/templates/rollback", "admin-token", map[string]any{"release_id": published.ID})
	if rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var rollback struct {
		ID int64 `json:"release_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rollback)
	if rollback.ID <= published.ID {
		t.Fatal("rollback must be a new activation version")
	}
	rec = call(t, s, "POST", "/internal/v1/worker/sync", "worker-token", map[string]any{"worker_id": "integration-test", "release_id": 0, "pending": 1, "updates": []any{}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"commands"`) {
		t.Fatal(rec.Body.String())
	}
}
func jsonNumber(n int64) string { b, _ := json.Marshal(n); return string(b) }

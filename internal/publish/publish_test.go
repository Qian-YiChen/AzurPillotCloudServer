package publish

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"ap-cloud-server/internal/store"

	"go.uber.org/goleak"
)

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// mockCF 模拟 Cloudflare 资产上传与部署 API，并记录收到的载荷。
type mockCF struct {
	mu          sync.Mutex
	manifest    map[string]manifestEntry
	uploaded    map[string][]byte // hash -> content
	deployMeta  string
	deployMain  string
	skipBuckets bool
}

func (m *mockCF) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/assets-upload-session"):
			body, _ := io.ReadAll(r.Body)
			var req struct {
				Manifest map[string]manifestEntry `json:"manifest"`
			}
			if err := jsonUnmarshal(body, &req); err != nil {
				t.Errorf("manifest 解析失败: %v", err)
			}
			m.mu.Lock()
			m.manifest = req.Manifest
			m.mu.Unlock()
			resp := `{"success":true,"result":{"jwt":"upload-jwt","buckets":[["` + hashOf(req.Manifest) + `"]]}}`
			if m.skipBuckets {
				resp = `{"success":true,"result":{"jwt":"completion-jwt","buckets":[]}}`
			}
			w.Write([]byte(resp))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workers/assets/upload"):
			if err := m.parseUpload(r); err != nil {
				t.Errorf("上传解析失败: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"jwt":"completion-jwt"}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/client/v4/accounts/acc/workers/scripts/"):
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("部署 multipart 解析失败: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			metaFile, _, err := r.FormFile("metadata")
			if err != nil {
				t.Errorf("部署缺少 metadata: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			metaRaw, _ := io.ReadAll(metaFile)
			metaFile.Close()
			meta := string(metaRaw)
			file, _, err := r.FormFile("main.js")
			if err != nil {
				t.Errorf("部署缺少 main.js: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer file.Close()
			main, _ := io.ReadAll(file)
			m.mu.Lock()
			m.deployMeta, m.deployMain = meta, string(main)
			m.mu.Unlock()
			w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			t.Errorf("意外请求: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func (m *mockCF) parseUpload(r *http.Request) error {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		return err
	}
	if m.uploaded == nil {
		m.uploaded = map[string][]byte{}
	}
	for _, headers := range r.MultipartForm.File {
		for _, fh := range headers {
			rc, err := fh.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			raw, err := io.ReadAll(rc)
			if err != nil {
				return err
			}
			// 文件内容按规范是 base64 编码；这里直接存原始内容不还原，
			// 仅验证哈希键与内容存在性。
			m.uploaded[fh.Filename] = raw
		}
	}
	return nil
}

func hashOf(manifest map[string]manifestEntry) string {
	for hash := range manifest {
		return hash
	}
	return ""
}

func newMockServer(t *testing.T, m *mockCF) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(m.handler(t))
	t.Cleanup(srv.Close)
	return srv
}

func TestClientPublishWorkerFullFlow(t *testing.T) {
	m := &mockCF{}
	srv := newMockServer(t, m)
	client := NewClient("acc", "token", 5*time.Second, srv.URL)

	content := []byte(`{"schema_version":2}` + "\n")
	files := []AssetFile{{Path: "/api/v2/get/notice/data.json", Content: content, ContentType: "application/json"}}
	if err := client.PublishWorker(context.Background(), "ap-notice", workerMainJS, files); err != nil {
		t.Fatalf("发布失败: %v", err)
	}

	// manifest 应包含完整路径与正确哈希。
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.manifest["/api/v2/get/notice/data.json"]
	if !ok {
		t.Fatalf("manifest 缺少资产: %v", m.manifest)
	}
	if entry.Hash != assetHash(content) || entry.Size != len(content) {
		t.Fatalf("manifest 条目错误: %+v", entry)
	}
	if len(m.uploaded) != 1 {
		t.Fatalf("应上传 1 个文件: %d", len(m.uploaded))
	}
	if !strings.Contains(m.deployMeta, `"jwt":"completion-jwt"`) ||
		!strings.Contains(m.deployMeta, `"main_module":"main.js"`) {
		t.Fatalf("部署 metadata 错误: %s", m.deployMeta)
	}
	if !strings.Contains(m.deployMain, "ASSETS.fetch") {
		t.Fatalf("部署缺少 Worker 模块: %s", m.deployMain)
	}
}

func TestClientDeployFailsKeepPrevious(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"success":false,"errors":[{"code":1,"message":"deploy boom"}]}`))
			return
		}
		w.Write([]byte(`{"success":true,"result":{"jwt":"completion-jwt","buckets":[]}}`))
	}))
	defer srv.Close()

	client := NewClient("acc", "token", 5*time.Second, srv.URL)
	err := client.PublishWorker(context.Background(), "ap-notice", workerMainJS, []AssetFile{
		{Path: "/data.json", Content: []byte(`{}`), ContentType: "application/json"},
	})
	if err == nil || !strings.Contains(err.Error(), "deploy boom") {
		t.Fatalf("部署失败应向上传递错误: %v", err)
	}
}

func TestRenderNoticeAndNull(t *testing.T) {
	content := "公告内容"
	url := "https://example.com/releases"
	doc, err := RenderNotice(&store.Announcement{ID: "notice-20261026-001", Title: "更新公告", Content: &content, URL: &url}, time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	if !strings.Contains(s, `"id":"notice-20261026-001"`) ||
		!strings.Contains(s, `"content":"公告内容"`) ||
		!strings.Contains(s, `"url":"https://example.com/releases"`) {
		t.Fatalf("公告文档错误: %s", s)
	}

	nullDoc, err := RenderNotice(nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nullDoc), `"announcement":null`) {
		t.Fatalf("空公告应渲染 announcement:null: %s", nullDoc)
	}
}

func TestRenderUpdate(t *testing.T) {
	doc, err := RenderUpdate(true, time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `"force_update":true`) {
		t.Fatalf("更新策略文档错误: %s", doc)
	}
}

func TestPublisherSerialAndReadBack(t *testing.T) {
	dir := t.TempDir()
	noticePath, _ := Paths(dir)
	content := []byte(`{"schema_version":2,"announcement":null}` + "\n")

	// 公网读取确认端点：返回与待发布一致的内容。
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer public.Close()

	m := &mockCF{}
	cfSrv := newMockServer(t, m)
	client := NewClient("acc", "token", 5*time.Second, cfSrv.URL)

	p := New(client, nil, dir, slog.Default())
	p.Run(context.Background())
	defer p.Close()

	res := p.Submit(context.Background(), Job{
		Worker:    NoticeWorker,
		Path:      "/api/v2/get/notice/data.json",
		LocalPath: noticePath,
		Content:   content,
		PublicURL: public.URL + "/api/v2/get/notice/data.json",
	}, 10*time.Second)
	if !res.Success || res.Skipped {
		t.Fatalf("发布应成功: %+v", res)
	}

	written, err := os.ReadFile(noticePath)
	if err != nil || string(written) != string(content) {
		t.Fatalf("本地文件内容错误: %v %s", err, written)
	}
}

func TestPublisherReadBackMismatch(t *testing.T) {
	dir := t.TempDir()
	noticePath, _ := Paths(dir)
	content := []byte(`{"schema_version":2,"announcement":null}` + "\n")

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"schema_version":1}`)) // 内容不一致
	}))
	defer public.Close()

	m := &mockCF{}
	cfSrv := newMockServer(t, m)
	client := NewClient("acc", "token", 5*time.Second, cfSrv.URL)

	p := New(client, nil, dir, slog.New(slog.DiscardHandler))
	p.Run(context.Background())
	defer p.Close()

	res := p.Submit(context.Background(), Job{
		Worker:    NoticeWorker,
		Path:      "/api/v2/get/notice/data.json",
		LocalPath: noticePath,
		Content:   content,
		PublicURL: public.URL,
	}, 10*time.Second)
	if res.Success || !strings.Contains(res.Error, "读取确认") {
		t.Fatalf("读取确认失败应记录错误: %+v", res)
	}
}

func TestPublisherWithoutCredentialsWritesLocalOnly(t *testing.T) {
	dir := t.TempDir()
	_, updatePath := Paths(dir)
	content := []byte(`{"schema_version":2,"force_update":false}` + "\n")

	p := New(nil, nil, dir, slog.New(slog.DiscardHandler))
	p.Run(context.Background())
	defer p.Close()

	res := p.Submit(context.Background(), Job{
		Worker:    UpdateWorker,
		Path:      "/api/v2/get/update/data.json",
		LocalPath: updatePath,
		Content:   content,
	}, 5*time.Second)
	if !res.Success || !res.Skipped {
		t.Fatalf("无凭据应跳过并成功: %+v", res)
	}
}

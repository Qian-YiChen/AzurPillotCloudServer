package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"ap-cloud-server/internal/store"
)

//go:embed worker/main.js
var workerMainJS []byte

// Worker 名称常量，与配置默认值一致。
const (
	NoticeWorker = "ap-notice"
	UpdateWorker = "ap-update-policy"
)

// Job 是一次发布任务。
type Job struct {
	Worker    string
	Path      string // 资产路径，如 /api/v2/get/notice/data.json
	LocalPath string // 本地静态文件路径
	Content   []byte // 渲染后的文件内容
	PublicURL string // 公网读取确认地址；为空则跳过读取确认
}

// Result 是发布任务的最终状态。
type Result struct {
	Success bool
	Skipped bool // 未配置 Cloudflare 凭据时只写本地文件
	Error   string
}

// Publisher 按 Worker 串行执行发布任务。
type Publisher struct {
	cf     *Client
	store  *store.Store
	logger *slog.Logger
	dir    string

	queues map[string]chan queuedJob
	wg     sync.WaitGroup
	stopM  sync.Mutex
	stops  []context.CancelFunc
}

type queuedJob struct {
	job     Job
	result  chan Result
	reqCtx  context.Context
	timeout time.Duration
}

// New 创建发布器。cf 可为 nil（未配置凭据时只生成本地文件）。
func New(cf *Client, st *store.Store, dir string, logger *slog.Logger) *Publisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Publisher{
		cf:     cf,
		store:  st,
		logger: logger,
		dir:    dir,
		queues: map[string]chan queuedJob{},
	}
}

// Run 为每个已知 Worker 启动一个串行消费协程；ctx 取消后退出。
func (p *Publisher) Run(ctx context.Context) {
	p.stopM.Lock()
	defer p.stopM.Unlock()
	if p.stops != nil {
		return
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	p.stops = []context.CancelFunc{cancel}
	for _, worker := range []string{NoticeWorker, UpdateWorker} {
		queue := make(chan queuedJob, 16)
		p.queues[worker] = queue
		p.wg.Add(1)
		go p.consume(runCtx, worker, queue)
	}
}

// Close 停止消费协程并等待退出。
func (p *Publisher) Close() {
	p.stopM.Lock()
	stops := p.stops
	p.stops = nil
	p.stopM.Unlock()
	for _, stop := range stops {
		stop()
	}
	p.wg.Wait()
}

// Submit 提交一个发布任务并阻塞等待其完成（串行保证由队列提供）。
func (p *Publisher) Submit(ctx context.Context, job Job, timeout time.Duration) Result {
	p.stopM.Lock()
	queue, ok := p.queues[job.Worker]
	p.stopM.Unlock()
	if !ok {
		return Result{Error: fmt.Sprintf("未知 Worker %q", job.Worker)}
	}
	qj := queuedJob{job: job, result: make(chan Result, 1), reqCtx: ctx, timeout: timeout}
	select {
	case <-ctx.Done():
		return Result{Error: "发布被取消"}
	case queue <- qj:
	}
	select {
	case res := <-qj.result:
		return res
	case <-ctx.Done():
		return Result{Error: "发布超时或被取消"}
	}
}

// consume 是单个 Worker 的串行发布循环。
func (p *Publisher) consume(ctx context.Context, worker string, queue chan queuedJob) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case qj := <-queue:
			result := p.process(ctx, worker, qj)
			select {
			case qj.result <- result:
			default:
			}
		}
	}
}

// process 执行完整的发布流程：写本地文件 → 上传部署 → 读取确认 → 记录历史。
func (p *Publisher) process(ctx context.Context, worker string, qj queuedJob) Result {
	job := qj.job
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(qj.reqCtx), qj.timeout)
	defer cancel()

	res := p.publish(runCtx, worker, job)
	status, errText := "success", ""
	if res.Error != "" {
		status, errText = "failed", res.Error
	}
	if res.Skipped {
		status = "skipped"
	}
	if p.store != nil {
		sum := sha256.Sum256(job.Content)
		recErr := p.store.RecordPublish(ctx, store.PublishRecord{
			Worker:      worker,
			ContentHash: hex.EncodeToString(sum[:8]),
			Status:      status,
			Error:       errText,
		})
		if recErr != nil {
			p.logger.Warn("记录发布历史失败", "worker", worker, "err", recErr)
		}
	}
	return res
}

func (p *Publisher) publish(ctx context.Context, worker string, job Job) Result {
	// 1. 原子写入本地静态目录。
	if err := writeAtomic(job.LocalPath, job.Content); err != nil {
		return Result{Error: fmt.Sprintf("写入本地静态文件失败: %v", err)}
	}

	// 2. 未配置凭据：只保留本地文件（开发环境）。
	if p.cf == nil {
		return Result{Success: true, Skipped: true}
	}

	// 3. 上传资产并部署（原子替换，失败自动保留上一版本）。
	files := []AssetFile{{
		Path:        job.Path,
		Content:     job.Content,
		ContentType: "application/json",
	}}
	if err := p.cf.PublishWorker(ctx, worker, workerMainJS, files); err != nil {
		p.logger.Warn("发布失败，保留上一版本", "worker", worker, "err", err)
		return Result{Error: err.Error()}
	}

	// 4. 读取公网内容确认一致；不一致按失败记录（部署已生效，人工核查）。
	if job.PublicURL != "" {
		if err := verifyReadBack(ctx, job.PublicURL, job.Content); err != nil {
			return Result{Error: fmt.Sprintf("发布后读取确认失败: %v", err)}
		}
	}
	return Result{Success: true}
}

// verifyReadBack 从公网地址拉取文件并逐字节比对。
func verifyReadBack(ctx context.Context, url string, want []byte) error {
	return verifyReadBackWithRetry(ctx, url, want, time.Second)
}

// An upload is deployed before every edge necessarily serves its new assets.
// Retry within the publication context, never report stale content as success.
func verifyReadBackWithRetry(ctx context.Context, url string, want []byte, delay time.Duration) error {
	var last error
	for attempt := 0; attempt < 6; attempt++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(err, last)
		}
		last = readBackOnce(ctx, url, want)
		if last == nil {
			return nil
		}
		if attempt == 5 {
			return last
		}
		timer := time.NewTimer(delay << attempt)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(ctx.Err(), last)
		case <-timer.C:
		}
	}
	return last
}

func readBackOnce(ctx context.Context, url string, want []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("公网读取返回 HTTP %d", resp.StatusCode)
	}
	got, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		return errors.New("公网内容与待发布内容不一致")
	}
	return nil
}

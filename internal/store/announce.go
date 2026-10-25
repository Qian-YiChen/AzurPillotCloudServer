package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Announcement 是一条公告。Content 与 URL 至少一项有内容（由调用方校验）。
type Announcement struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   *string   `json:"content"`
	URL       *string   `json:"url"`
	Active    bool      `json:"active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GenerateAnnouncementID 按计划示例格式生成交互 ID：notice-YYYYMMDD-NNN。
func (s *Store) GenerateAnnouncementID(ctx context.Context, now time.Time) (string, error) {
	dateKey := now.UTC().Format("20060102")
	var count int
	if err := s.Pool.QueryRow(ctx, `
		SELECT count(*) FROM announcements
		WHERE id LIKE $1`, "notice-"+dateKey+"-%",
	).Scan(&count); err != nil {
		return "", fmt.Errorf("生成公告 ID 失败: %w", err)
	}
	return fmt.Sprintf("notice-%s-%03d", dateKey, count+1), nil
}

// SetAnnouncement 撤下旧公告并写入新的有效公告。
func (s *Store) SetAnnouncement(ctx context.Context, a Announcement) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE announcements SET active = FALSE, updated_at = now() WHERE active`); err != nil {
		return fmt.Errorf("撤下旧公告失败: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO announcements (id, title, content, url, active, updated_at)
		VALUES ($1, $2, $3, $4, TRUE, now())
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			url = EXCLUDED.url,
			active = TRUE,
			updated_at = now()`,
		a.ID, a.Title, a.Content, a.URL,
	); err != nil {
		return fmt.Errorf("写入公告失败: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交公告事务失败: %w", err)
	}
	return nil
}

// WithdrawAnnouncement 撤回全部有效公告（发布 announcement: null）。
func (s *Store) WithdrawAnnouncement(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE announcements SET active = FALSE, updated_at = now() WHERE active`)
	if err != nil {
		return fmt.Errorf("撤回公告失败: %w", err)
	}
	return nil
}

// LatestAnnouncement 返回当前有效公告；没有时返回 (nil, nil)。
func (s *Store) LatestAnnouncement(ctx context.Context) (*Announcement, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT id, title, content, url, active, updated_at
		FROM announcements
		WHERE active
		ORDER BY updated_at DESC
		LIMIT 1`)

	var a Announcement
	err := row.Scan(&a.ID, &a.Title, &a.Content, &a.URL, &a.Active, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询公告失败: %w", err)
	}
	return &a, nil
}

// ListAnnouncements 返回最近的公告历史，供管理端查看。
func (s *Store) ListAnnouncements(ctx context.Context, limit int) ([]Announcement, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, title, content, url, active, updated_at
		FROM announcements
		ORDER BY updated_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("查询公告列表失败: %w", err)
	}
	defer rows.Close()

	out := []Announcement{}
	for rows.Next() {
		var a Announcement
		if err := rows.Scan(&a.ID, &a.Title, &a.Content, &a.URL, &a.Active, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("解析公告失败: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetUpdatePolicy 返回强制更新开关；表为空时返回默认 false。
func (s *Store) GetUpdatePolicy(ctx context.Context) (bool, error) {
	var force bool
	err := s.Pool.QueryRow(ctx, `SELECT force_update FROM update_policy WHERE id`).Scan(&force)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("查询更新策略失败: %w", err)
	}
	return force, nil
}

// SetUpdatePolicy 保存强制更新开关。
func (s *Store) SetUpdatePolicy(ctx context.Context, force bool) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO update_policy (id, force_update, updated_at)
		VALUES (TRUE, $1, now())
		ON CONFLICT (id) DO UPDATE SET force_update = EXCLUDED.force_update, updated_at = now()`,
		force)
	if err != nil {
		return fmt.Errorf("保存更新策略失败: %w", err)
	}
	return nil
}

// PublishRecord 是一次静态文件发布的结果。
type PublishRecord struct {
	Worker      string    `json:"worker"`
	ContentHash string    `json:"content_hash"`
	Status      string    `json:"status"` // success / failed / skipped
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// RecordPublish 追加一条发布历史。
func (s *Store) RecordPublish(ctx context.Context, rec PublishRecord) error {
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO publish_history (worker, content_hash, status, error)
		VALUES ($1, $2, $3, NULLIF($4, ''))`,
		rec.Worker, rec.ContentHash, rec.Status, rec.Error)
	if err != nil {
		return fmt.Errorf("记录发布历史失败: %w", err)
	}
	return nil
}

// LatestPublishByWorker 返回每个 Worker 最近一次发布记录。
func (s *Store) LatestPublishByWorker(ctx context.Context) (map[string]PublishRecord, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT ON (worker) worker, content_hash, status, COALESCE(error, ''), created_at
		FROM publish_history
		ORDER BY worker, created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("查询发布历史失败: %w", err)
	}
	defer rows.Close()

	out := map[string]PublishRecord{}
	for rows.Next() {
		var rec PublishRecord
		if err := rows.Scan(&rec.Worker, &rec.ContentHash, &rec.Status, &rec.Error, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("解析发布历史失败: %w", err)
		}
		out[rec.Worker] = rec
	}
	return out, rows.Err()
}

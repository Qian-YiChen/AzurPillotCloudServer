package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Maintenance 是一条区服维护计划。
type Maintenance struct {
	Server    string     `json:"server"`
	Name      string     `json:"name"`
	StartsAt  time.Time  `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Source    string     `json:"source,omitempty"`
	SourceURL string     `json:"source_url,omitempty"`
}

// Both readers use the same precedence: an active manual override wins; an
// expired manual row does not hide a newly announced official schedule.
const maintenancePlans = `(
 SELECT server,name,starts_at,ends_at,updated_at,'manual' AS source,'' AS source_url,0 AS priority
 FROM maintenance_schedules
 UNION ALL
 SELECT server,name,starts_at,ends_at,synced_at,'official',source_url,1
 FROM maintenance_sources WHERE starts_at IS NOT NULL
) AS plans`

// UpsertMaintenance 保存一个区服的维护计划（覆盖式）。
func (s *Store) UpsertMaintenance(ctx context.Context, m Maintenance) error {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO maintenance_schedules (server, name, starts_at, ends_at, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (server) DO UPDATE SET
			name      = EXCLUDED.name,
			starts_at = EXCLUDED.starts_at,
			ends_at   = EXCLUDED.ends_at,
			updated_at = now()`,
		m.Server, m.Name, m.StartsAt, m.EndsAt,
	)
	if err != nil {
		return fmt.Errorf("保存维护计划失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("保存维护计划未生效")
	}
	return nil
}

// LatestMaintenance 返回一个区服当前有效的维护安排：
// 尚未结束（ends_at 为空或大于等于当前时间）的最新一条。
// 没有安排时返回 (nil, nil)。
func (s *Store) LatestMaintenance(ctx context.Context, server string) (*Maintenance, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT server, name, starts_at, ends_at, updated_at, source, source_url
		FROM `+maintenancePlans+`
		WHERE server = $1 AND (ends_at IS NULL OR ends_at >= now())
		ORDER BY priority, starts_at DESC
		LIMIT 1`, server)

	var m Maintenance
	var endsAt *time.Time
	err := row.Scan(&m.Server, &m.Name, &m.StartsAt, &endsAt, &m.UpdatedAt, &m.Source, &m.SourceURL)
	if errors.Is(err, pgx.ErrNoRows) {
		if server == "cn" {
			var sourceError string
			checkErr := s.Pool.QueryRow(ctx, `SELECT error FROM maintenance_sources WHERE server='cn'`).Scan(&sourceError)
			if checkErr != nil && !errors.Is(checkErr, pgx.ErrNoRows) {
				return nil, checkErr
			}
			if sourceError != "" {
				return nil, fmt.Errorf("官方维护来源暂不可用: %s", sourceError)
			}
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询维护计划失败: %w", err)
	}
	m.EndsAt = endsAt
	return &m, nil
}

// ListMaintenances 返回全部维护计划（含已结束），供管理端查看。
func (s *Store) ListMaintenances(ctx context.Context) ([]Maintenance, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT ON (server) server, name, starts_at, ends_at, updated_at, source, source_url
		FROM `+maintenancePlans+`
		ORDER BY server, CASE WHEN ends_at IS NULL OR ends_at >= now() THEN 0 ELSE 1 END, priority`)
	if err != nil {
		return nil, fmt.Errorf("查询维护计划列表失败: %w", err)
	}
	defer rows.Close()

	out := []Maintenance{}
	for rows.Next() {
		var m Maintenance
		if err := rows.Scan(&m.Server, &m.Name, &m.StartsAt, &m.EndsAt, &m.UpdatedAt, &m.Source, &m.SourceURL); err != nil {
			return nil, fmt.Errorf("解析维护计划失败: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DeleteMaintenance 删除一个区服的维护计划，返回是否确实删除了记录。
func (s *Store) DeleteMaintenance(ctx context.Context, server string) (bool, error) {
	tag, err := s.Pool.Exec(ctx,
		`DELETE FROM maintenance_schedules WHERE server = $1`, server)
	if err != nil {
		return false, fmt.Errorf("删除维护计划失败: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

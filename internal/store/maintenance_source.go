package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaintenanceSource retains the last parsed announcement even after its window
// ends. Public queries filter it by ends_at; administrators can still audit it.
type MaintenanceSource struct {
	Server    string       `json:"server"`
	Title     string       `json:"title"`
	URL       string       `json:"url"`
	Plan      *Maintenance `json:"plan"`
	CheckedAt *time.Time   `json:"checked_at"`
	SyncedAt  *time.Time   `json:"synced_at"`
	Error     string       `json:"error"`
}

func (s *Store) MaintenanceSource(ctx context.Context) (MaintenanceSource, error) {
	state := MaintenanceSource{Server: "cn"}
	var name string
	var starts, ends *time.Time
	err := s.Pool.QueryRow(ctx, `SELECT name,starts_at,ends_at,title,source_url,checked_at,synced_at,error
	 FROM maintenance_sources WHERE server='cn'`).Scan(&name, &starts, &ends,
		&state.Title, &state.URL, &state.CheckedAt, &state.SyncedAt, &state.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("查询官方维护来源失败: %w", err)
	}
	if starts != nil && state.SyncedAt != nil {
		state.Plan = &Maintenance{Server: "cn", Name: name, StartsAt: *starts, EndsAt: ends,
			UpdatedAt: *state.SyncedAt, Source: "official", SourceURL: state.URL}
	}
	return state, nil
}

func (s *Store) SaveOfficialMaintenance(ctx context.Context, m Maintenance, title, sourceURL string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO maintenance_sources
	 (server,name,starts_at,ends_at,title,source_url,checked_at,synced_at,error)
	 VALUES ('cn',$1,$2,$3,$4,$5,now(),now(),'') ON CONFLICT (server) DO UPDATE SET
	 name=EXCLUDED.name,starts_at=EXCLUDED.starts_at,ends_at=EXCLUDED.ends_at,
	 title=EXCLUDED.title,source_url=EXCLUDED.source_url,checked_at=now(),synced_at=now(),error=''`,
		m.Name, m.StartsAt, m.EndsAt, title, sourceURL)
	if err != nil {
		return fmt.Errorf("保存官方维护计划失败: %w", err)
	}
	return nil
}

// A network or parse failure must never erase the previous successful result.
func (s *Store) RecordMaintenanceSyncError(ctx context.Context, message string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO maintenance_sources(server,checked_at,error)
	 VALUES ('cn',now(),$1) ON CONFLICT (server) DO UPDATE SET checked_at=now(),error=EXCLUDED.error`, message)
	return err
}

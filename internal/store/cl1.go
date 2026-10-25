package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Cl1Outcome 是 CL1 提交事务的业务结果。
type Cl1Outcome int

const (
	// Cl1Accepted 表示快照与去重记录都已提交成功。
	Cl1Accepted Cl1Outcome = iota
	// Cl1Duplicate 表示同一 request_id 同一正文的重复请求，
	// Payload 中携带原业务结果。
	Cl1Duplicate
	// Cl1RequestConflict 表示同一 request_id 携带不同正文。
	Cl1RequestConflict
	// Cl1StaleSnapshot 表示旧快照不能覆盖新快照。
	Cl1StaleSnapshot
)

// Cl1Record 是一次 CL1 月度快照写入。
type Cl1Record struct {
	RequestID         string
	DeviceID          string
	InstanceID        string
	Month             string
	CollectedAt       time.Time
	BattleCount       int64
	BattleRounds      int64
	SortieCost        int64
	AkashiEncounters  int64
	AkashiAP          int64
	AkashiProbability float64
	AverageStamina    float64
	NetStaminaGain    int64
}

// SubmitCl1 在单个事务里完成快照覆盖写与请求去重登记。
//
// 语义（重构计划 3.5）：
//   - 按 (device_id, instance_id, month) 覆盖更新，不累加；
//   - 旧快照（collected_at 更早）不能覆盖新快照 → Cl1StaleSnapshot；
//   - 同 request_id 同正文重试 → Cl1Duplicate，返回原业务结果；
//   - 同 request_id 不同正文 → Cl1RequestConflict；
//   - responseJSON 是 duplicate=false 的完整成功响应体，
//     首次成功时与去重记录一起原子提交。
//
// 返回 (结果, 需要返回给客户端的响应体, error)。
func (s *Store) SubmitCl1(ctx context.Context, rec Cl1Record, bodyHash string, responseJSON []byte) (Cl1Outcome, []byte, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("开启事务失败: %w", err)
	}
	defer tx.Rollback(ctx) // 提交后调用为无害空操作。

	// 1. 查询 request_id 去重结果。
	var storedHash string
	var storedResponse []byte
	err = tx.QueryRow(ctx,
		`SELECT body_hash, response FROM cl1_dedup WHERE request_id = $1`, rec.RequestID,
	).Scan(&storedHash, &storedResponse)
	switch {
	case err == nil:
		if storedHash == bodyHash {
			return Cl1Duplicate, storedResponse, nil
		}
		return Cl1RequestConflict, nil, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return 0, nil, fmt.Errorf("查询去重记录失败: %w", err)
	}

	// 2. 覆盖写快照；WHERE 条件保证旧快照不覆盖新快照。
	tag, err := tx.Exec(ctx, `
		INSERT INTO cl1_snapshots (
			device_id, instance_id, month, collected_at,
			battle_count, battle_rounds, sortie_cost,
			akashi_encounters, akashi_ap,
			akashi_probability, average_stamina, net_stamina_gain
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (device_id, instance_id, month) DO UPDATE SET
			collected_at       = EXCLUDED.collected_at,
			battle_count       = EXCLUDED.battle_count,
			battle_rounds      = EXCLUDED.battle_rounds,
			sortie_cost        = EXCLUDED.sortie_cost,
			akashi_encounters  = EXCLUDED.akashi_encounters,
			akashi_ap          = EXCLUDED.akashi_ap,
			akashi_probability = EXCLUDED.akashi_probability,
			average_stamina    = EXCLUDED.average_stamina,
			net_stamina_gain   = EXCLUDED.net_stamina_gain,
			updated_at         = now()
		WHERE cl1_snapshots.collected_at <= EXCLUDED.collected_at`,
		rec.DeviceID, rec.InstanceID, rec.Month, rec.CollectedAt,
		rec.BattleCount, rec.BattleRounds, rec.SortieCost,
		rec.AkashiEncounters, rec.AkashiAP,
		rec.AkashiProbability, rec.AverageStamina, rec.NetStaminaGain,
	)
	if err != nil {
		return 0, nil, fmt.Errorf("写入快照失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Cl1StaleSnapshot, nil, nil
	}

	// 3. 登记去重结果；并发同 ID 请求撞主键时回读已有结果。
	tag, err = tx.Exec(ctx, `
		INSERT INTO cl1_dedup (request_id, device_id, instance_id, body_hash, response, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (request_id) DO NOTHING`,
		rec.RequestID, rec.DeviceID, rec.InstanceID, bodyHash, responseJSON, time.Now().Add(s.DedupTTL),
	)
	if err != nil {
		return 0, nil, fmt.Errorf("写入去重记录失败: %w", err)
	}
	if tag.RowsAffected() == 0 {
		err = tx.QueryRow(ctx,
			`SELECT body_hash, response FROM cl1_dedup WHERE request_id = $1`, rec.RequestID,
		).Scan(&storedHash, &storedResponse)
		if err != nil {
			return 0, nil, fmt.Errorf("回读去重记录失败: %w", err)
		}
		if storedHash == bodyHash {
			return Cl1Duplicate, storedResponse, nil
		}
		return Cl1RequestConflict, nil, nil
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, nil, fmt.Errorf("提交事务失败: %w", err)
	}
	return Cl1Accepted, responseJSON, nil
}

// Cl1Row 是一条已存储的月度快照。
type Cl1Row struct {
	DeviceID          string
	InstanceID        string
	Month             string
	CollectedAt       time.Time
	BattleCount       int64
	AkashiEncounters  int64
	AkashiAP          int64
	AkashiProbability float64
	AverageStamina    float64
	UpdatedAt         time.Time
}

// ListCl1Snapshots 返回指定设备的月度快照（按更新时间倒序）。
func (s *Store) ListCl1Snapshots(ctx context.Context, deviceID, month string, limit int) ([]Cl1Row, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT device_id, instance_id, month, collected_at,
		       battle_count, akashi_encounters, akashi_ap,
		       akashi_probability, average_stamina, updated_at
		FROM cl1_snapshots
		WHERE device_id = $1 AND month = $2
		ORDER BY updated_at DESC
		LIMIT $3`, deviceID, month, limit)
	if err != nil {
		return nil, fmt.Errorf("查询快照失败: %w", err)
	}
	defer rows.Close()

	var out []Cl1Row
	for rows.Next() {
		var r Cl1Row
		if err := rows.Scan(&r.DeviceID, &r.InstanceID, &r.Month, &r.CollectedAt,
			&r.BattleCount, &r.AkashiEncounters, &r.AkashiAP,
			&r.AkashiProbability, &r.AverageStamina, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("解析快照失败: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

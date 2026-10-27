package maintenance

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"ap-cloud-server/internal/store"
)

var ErrBusy = errors.New("官方公告正在同步，请稍后刷新")

type Backend interface {
	MaintenanceSource(context.Context) (store.MaintenanceSource, error)
	SaveOfficialMaintenance(context.Context, store.Maintenance, string, string) error
	RecordMaintenanceSyncError(context.Context, string) error
}

type Status struct {
	store.MaintenanceSource
	Enabled         bool `json:"enabled"`
	IntervalSeconds int  `json:"interval_seconds"`
	Running         bool `json:"running"`
}

type Syncer struct {
	backend           Backend
	fetch             func(context.Context) (Notice, error)
	interval, timeout time.Duration
	enabled           bool
	logger            *slog.Logger
	mu                sync.Mutex
	lifecycle         sync.Mutex
	stop              context.CancelFunc
	wg                sync.WaitGroup
}

func New(b Backend, enabled bool, interval, timeout time.Duration, logger *slog.Logger) *Syncer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Syncer{backend: b, fetch: NewSource().Fetch, enabled: enabled, interval: interval, timeout: timeout, logger: logger}
}

func (s *Syncer) Status(ctx context.Context) (Status, error) {
	state, err := s.backend.MaintenanceSource(ctx)
	if state.URL == "" {
		state.URL = OfficialPage
	}
	running := !s.mu.TryLock()
	if !running {
		s.mu.Unlock()
	}
	return Status{MaintenanceSource: state, Enabled: s.enabled, IntervalSeconds: int(s.interval / time.Second), Running: running}, err
}

func (s *Syncer) Sync(ctx context.Context) error {
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	run, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	n, err := s.fetch(run)
	if err == nil {
		err = s.backend.SaveOfficialMaintenance(run, n.Plan, n.Title, n.URL)
	}
	if err != nil {
		record, cancelRecord := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancelRecord()
		if saveErr := s.backend.RecordMaintenanceSyncError(record, err.Error()); saveErr != nil {
			s.logger.Warn("记录维护同步失败状态", "err", saveErr)
		}
		return err
	}
	return nil
}

func (s *Syncer) Run(ctx context.Context) {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if !s.enabled || s.stop != nil {
		return
	}
	run, stop := context.WithCancel(context.WithoutCancel(ctx))
	s.stop = stop
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			if err := s.Sync(run); err != nil && run.Err() == nil {
				s.logger.Warn("官方维护公告同步失败，保留上次结果", "err", err)
			}
			select {
			case <-run.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Syncer) Close() {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.stop != nil {
		s.stop()
		s.wg.Wait()
		s.stop = nil
	}
}

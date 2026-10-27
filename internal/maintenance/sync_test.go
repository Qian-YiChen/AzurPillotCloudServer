package maintenance

import (
	"ap-cloud-server/internal/store"
	"context"
	"errors"
	"go.uber.org/goleak"
	"testing"
	"time"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type fakeStore struct{ state store.MaintenanceSource }

func (f *fakeStore) MaintenanceSource(context.Context) (store.MaintenanceSource, error) {
	return f.state, nil
}
func (f *fakeStore) SaveOfficialMaintenance(_ context.Context, m store.Maintenance, title, url string) error {
	f.state.Plan = &m
	f.state.Title = title
	f.state.URL = url
	f.state.Error = ""
	return nil
}
func (f *fakeStore) RecordMaintenanceSyncError(_ context.Context, e string) error {
	f.state.Error = e
	return nil
}

func TestFailedSyncKeepsPlanAndConcurrentSyncIsRejected(t *testing.T) {
	f := &fakeStore{state: store.MaintenanceSource{Plan: &store.Maintenance{Name: "retained"}}}
	s := New(f, true, time.Minute, time.Second, nil)
	s.fetch = func(context.Context) (Notice, error) { return Notice{}, errors.New("upstream down") }
	if err := s.Sync(context.Background()); err == nil || f.state.Plan.Name != "retained" || f.state.Error == "" {
		t.Fatal(err, f.state)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	s.fetch = func(context.Context) (Notice, error) {
		close(started)
		<-release
		return Notice{Plan: store.Maintenance{Name: "new"}}, nil
	}
	go func() { done <- s.Sync(context.Background()) }()
	<-started
	if err := s.Sync(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if f.state.Plan.Name != "new" || f.state.Error != "" {
		t.Fatal(f.state)
	}
}

func TestCloseCancelsStartupSync(t *testing.T) {
	s := New(&fakeStore{}, true, time.Hour, time.Hour, nil)
	started := make(chan struct{})
	s.fetch = func(ctx context.Context) (Notice, error) { close(started); <-ctx.Done(); return Notice{}, ctx.Err() }
	s.Run(context.Background())
	<-started
	s.Close()
}

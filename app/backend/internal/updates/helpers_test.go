package updates

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// Health results the update check rows are read against.
const (
	OK      = health.OK
	Warning = health.Warning
)

// world is a store, an update Checker and the health checks that show its row.
type world struct {
	s   *database.Store
	e   *Checker
	h   *health.Engine
	now time.Time
}

func newWorld(t *testing.T) *world {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w := &world{s: s, now: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	w.e = &Checker{Store: s, Version: "v1.1.0", Now: func() time.Time { return w.now }}
	w.h = &health.Engine{Store: s, Version: "v1.1.0", Now: func() time.Time { return w.now }, FreeSpace: func(string) int64 { return 50 << 30 },
		UpdateCheck: func(ctx context.Context) (string, bool, bool) {
			info, ran := w.e.Scheduled(ctx)
			return info.Latest, info.Newer, ran
		}}
	return w
}

func (w *world) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// run runs the health checks once and returns them by key.
func (w *world) run(t *testing.T) map[string]domain.HealthCheck {
	t.Helper()
	w.now = w.now.Add(5 * time.Minute)
	cs, err := w.h.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]domain.HealthCheck{}
	for _, c := range cs {
		out[c.Key] = c
	}
	return out
}

// ReadSettings and SaveSettings are the health settings page, which carries
// the update check's switches.
func ReadSettings(ctx context.Context, s *database.Store) (health.Settings, error) {
	return health.ReadSettings(ctx, s, DefaultReleaseFeed)
}

var SaveSettings = health.SaveSettings

package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// capsServer answers caps while up is true, and fails otherwise.
func capsServer(t *testing.T, up *atomic.Bool, calls *atomic.Int32) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !up.Load() {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`<caps><searching><search available="yes"/></searching></caps>`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// A Source not used for the feed, paused because it failed while its
// network was still starting, is tried again once its wait is
// over and goes back to working on its own.
func TestFeedCheck_RetriesRestingSearchOnlySource(t *testing.T) {
	var up atomic.Bool
	var calls atomic.Int32
	up.Store(true)
	f := seed(t, capsServer(t, &up, &calls))
	exec(t, f, `UPDATE sources SET state = 'paused', pause_reason = 'down', paused_until = ?, extra = json_set(extra, '$.failures', '1')`,
		database.FormatTime(time.Now().Add(-time.Minute)))

	needed, _, err := (&Searcher{Store: f.s}).ProbeFeed(context.Background(), time.Now())
	if err != nil || !needed {
		t.Fatalf("probe: needed %v, error %v", needed, err)
	}
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)

	var src domain.Source
	if err := database.Get(context.Background(), f.s.Read(), "sources", f.srcID, &src); err != nil {
		t.Fatal(err)
	}
	if src.State != "enabled" || src.Extra["failures"] != "" {
		t.Fatalf("Source still resting: state %q extra %v", src.State, src.Extra)
	}
	if h := HealthOf(src); h.State != StateWorking {
		t.Fatalf("health %+v", h)
	}
}

// One still down stays paused with a longer wait, and one still waiting
// is not called at all.
func TestFeedCheck_RestingSourceStillDown(t *testing.T) {
	var up atomic.Bool
	var calls atomic.Int32
	f := seed(t, capsServer(t, &up, &calls))
	exec(t, f, `UPDATE sources SET state = 'paused', pause_reason = 'down', paused_until = ?, extra = json_set(extra, '$.failures', '1')`,
		database.FormatTime(time.Now().Add(-time.Minute)))
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)

	var src domain.Source
	if err := database.Get(context.Background(), f.s.Read(), "sources", f.srcID, &src); err != nil {
		t.Fatal(err)
	}
	if src.State != "paused" || src.PausedUntil == nil || !src.PausedUntil.After(time.Now()) || src.Extra["failures"] != "2" {
		t.Fatalf("still-down Source: state %q until %v extra %v", src.State, src.PausedUntil, src.Extra)
	}

	n := calls.Load()
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if calls.Load() != n {
		t.Fatalf("a Source still waiting was called")
	}
}

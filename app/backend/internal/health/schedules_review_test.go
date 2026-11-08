package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Turning automatic work off does not change who may run it by hand.
func TestRunScheduleTurnedOff(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE schedules SET enabled = 0 WHERE kind = 'feed_check'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	staff := webserver.Caller{UserID: 2, Role: webserver.RoleAdmin, Via: "session", Perms: []webserver.Perm{webserver.PermManageLibrary}}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/schedules/feed_check/run", nil)
	r.SetPathValue("kind", "feed_check")
	r = r.WithContext(webserver.ContextWith(r.Context(), staff))
	w := httptest.NewRecorder()
	f.sch.RunNow(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("staff: %d %s", w.Code, w.Body.String())
	}
}

// A Job's error and a loop's reason are masked before they are shown.
func TestScheduleErrorsMasked(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO jobs (created_at, created_by, updated_at, updated_by, kind, state, params, error, ended_at, run_after, asked_by)
			VALUES ('2026-10-14T00:00:00Z', '{}', '2026-10-14T00:00:00Z', '{}', 'feed_check', 'failed', '{}', '{"code":"x","message":"GET http://u:pw@h/x?apikey=XXXX_SECRET_KEY failed"}', '2026-10-14T00:00:00Z', '2026-10-14T00:00:00Z', '{}')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var got schedulesAnswer
	if err := json.Unmarshal(do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	s := scheduleNamed(t, got, "feed_check")
	if strings.Contains(s.LastJobError, "XXXX_SECRET_KEY") || strings.Contains(s.LastJobError, "pw@") || !strings.Contains(s.LastJobError, logs.Hidden) {
		t.Fatalf("error shown: %q", s.LastJobError)
	}
}

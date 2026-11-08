package main

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// History keep times live in Settings > This app: routine 30 days and the
// rest forever unless changed; negative or huge values are refused, only
// the owner may change them, and each change is logged as settings_changed.
func TestHistoryKeepSetting(t *testing.T) {
	t.Parallel()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a := buildHandlers(services{Store: st, Version: "test"})
	if s := health.UISettingsOf(context.Background(), st); s.HistoryRoutineDays != 30 || s.HistoryOtherDays != 0 {
		t.Fatalf("defaults %+v", s)
	}
	put := func(role, body string) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/v1/settings/ui", strings.NewReader(body))
		r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Name: "luis", Role: role}))
		a.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if c := put(webserver.RoleAdmin, `{"history_other_days": 7}`); c != 403 {
		t.Fatalf("admin answered %d", c)
	}
	if c := put(webserver.RoleOwner, `{"history_routine_days": -1}`); c != 400 {
		t.Fatalf("-1 answered %d", c)
	}
	if c := put(webserver.RoleOwner, `{"history_other_days": 365}`); c != 200 {
		t.Fatalf("365 answered %d", c)
	}
	if s := health.UISettingsOf(context.Background(), st); s.HistoryRoutineDays != 30 || s.HistoryOtherDays != 365 {
		t.Fatalf("after put %+v", s)
	}
	// Other this_app switches stay open to settings people.
	if c := put(webserver.RoleAdmin, `{"autoplay_trailers": false}`); c != 200 {
		t.Fatalf("admin autoplay answered %d", c)
	}
	es, _, err := st.Events(context.Background(), database.EventFilter{Kinds: []string{health.EventSettingsChanged}})
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[1].Data["area"] != "ui" || es[1].Actor.Kind != "user" || es[1].Actor.ID != 1 {
		t.Fatalf("settings_changed events %+v", es)
	}
}

package api

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

// History keep times live in Settings > This app: routine 30 days and the
// rest forever unless changed; negative or huge values are refused.
func TestHistoryKeepSetting(t *testing.T) {
	a, _ := signinRig(t, nil)
	if s := a.uiSettings(context.Background()); s.HistoryRoutineDays != 30 || s.HistoryOtherDays != 0 {
		t.Fatalf("defaults %+v", s)
	}
	put := func(body string) int {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("PUT", "/api/v1/settings/ui", strings.NewReader(body)))
		return w.Code
	}
	if c := put(`{"history_routine_days": -1}`); c != 400 {
		t.Fatalf("-1 answered %d", c)
	}
	if c := put(`{"history_other_days": 365}`); c != 200 {
		t.Fatalf("365 answered %d", c)
	}
	if s := a.uiSettings(context.Background()); s.HistoryRoutineDays != 30 || s.HistoryOtherDays != 365 {
		t.Fatalf("after put %+v", s)
	}
}

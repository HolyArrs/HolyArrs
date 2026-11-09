package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// "Count this install" is on by default, round-trips through
// /settings/ui and is passed on to the metadata server's requests.
func TestCountInstallSetting(t *testing.T) {
	t.Parallel()
	a, _ := signinRig(t, nil)
	var told []bool
	a.s.CountryCount = func(on bool) { told = append(told, on) }
	ctx := context.Background()
	if !health.UISettingsOf(ctx, a.s.Store).CountInstall {
		t.Fatal("count_install is off by default")
	}
	a.ApplyCountryCount(ctx)
	put := func(body string) health.UISettings {
		t.Helper()
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("PUT", "/api/v1/settings/ui", strings.NewReader(body)))
		var s health.UISettings
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &s) != nil {
			t.Fatalf("put %s: %d %s", body, w.Code, w.Body.String())
		}
		return s
	}
	if !health.CountInstall(ctx, a.s.Store.Read()) {
		t.Fatal("stored count_install is off by default (read before the Job queue starts)")
	}
	if s := put(`{"count_install": false}`); s.CountInstall || health.UISettingsOf(ctx, a.s.Store).CountInstall {
		t.Fatal("count_install off not kept")
	}
	if health.CountInstall(ctx, a.s.Store.Read()) {
		t.Fatal("stored count_install off not read before the Job queue starts")
	}
	put(`{"autoplay_trailers": true}`)
	if s := put(`{"count_install": true}`); !s.CountInstall {
		t.Fatal("count_install on not kept")
	}
	if len(told) != 3 || !told[0] || told[1] || !told[2] {
		t.Fatalf("metadata requests told %v, want [true false true]", told)
	}
}

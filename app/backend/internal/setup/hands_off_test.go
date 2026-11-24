package setup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// The hands-off setting reads and saves {"on": bool}, refuses anything
// else, and while it is on a Subtitles route answers with the hands-off
// reason instead of the plain "part is off" line.
func TestHandsOffSettingRoutes(t *testing.T) {
	parts.Register()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctl := &parts.Control{Store: st, Reg: jobs.NewRegistry()}
	if err := ctl.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	var events []string
	h := &PartHandlers{Store: st, Parts: ctl,
		Actor:   func(*http.Request) domain.Actor { return domain.System },
		Confirm: func(http.ResponseWriter, *http.Request, string) bool { return true },
		Event:   func(_ *http.Request, action, _, _ string) { events = append(events, action) }}
	m := http.NewServeMux()
	h.Routes(m)
	m.HandleFunc("GET /api/v1/subtitles/languages", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	gate := &Handlers{Deps: Deps{Store: st, Parts: ctl}}
	srv := httptest.NewServer(gate.PartGate(m, m))
	t.Cleanup(srv.Close)
	u := srv.URL + "/api/v1/settings/hands-off"
	steps := []struct {
		method, body string
		code         int
		answer       string
	}{
		{"GET", "", 200, `{"on":false}`},
		{"PUT", `{}`, 400, "send"},
		{"PUT", `{"on":"yes"}`, 400, "send"},
		{"PUT", `{"on":true}`, 200, `{"on":true}`},
		{"GET", "", 200, `{"on":true}`},
		{"PUT", `{"on":false}`, 200, `{"on":false}`},
		{"GET", "", 200, `{"on":false}`},
	}
	for _, s := range steps {
		b := string(do(t, s.method, u, s.body, s.code))
		if !strings.Contains(b, s.answer) {
			t.Fatalf("%s %s: %s, want %s", s.method, s.body, b, s.answer)
		}
	}
	if len(events) != 2 {
		t.Fatalf("events %q", events)
	}
	do(t, "GET", srv.URL+"/api/v1/subtitles/languages", "", 200)
	do(t, "PUT", u, `{"on":true}`, 200)
	if b := string(do(t, "GET", srv.URL+"/api/v1/subtitles/languages", "", 409)); !strings.Contains(b, "Hands-off mode is on") {
		t.Fatalf("subtitles answer %s", b)
	}
}

// Changing hands-off mode needs a recent password check, except in the
// first-run setup session, which counts as checked until the wizard is
// done (an owner made with Plex, Jellyfin or Emby has no password).
func TestHandsOffPutNeedsARecentCheckAfterSetup(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := &PartHandlers{Store: st,
		Actor: func(*http.Request) domain.Actor { return domain.System },
		Event: func(*http.Request, string, string, string) {}}
	m := http.NewServeMux()
	h.Routes(m)
	owner := webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "session"}
	cases := []struct {
		name string
		who  func() webserver.Caller
		code int
	}{
		{"setup session, no password", func() webserver.Caller { c := owner; c.SetupRecent, c.NoPassword = true, true; return c }, 200},
		{"after setup, not checked again", func() webserver.Caller { return owner }, 403},
		{"after setup, password checked", func() webserver.Caller { c := owner; c.LocalRecent = true; return c }, 200},
		{"api key", func() webserver.Caller { c := owner; c.Via = "api_key"; return c }, 200},
		{"trusted network skip", func() webserver.Caller { c := owner; c.Via = "local"; return c }, 403},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("PUT", "/api/v1/settings/hands-off", strings.NewReader(`{"on":true}`))
		req = req.WithContext(webserver.ContextWith(req.Context(), tc.who()))
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Errorf("%s: %d, want %d: %s", tc.name, rec.Code, tc.code, rec.Body.String())
		}
	}
}

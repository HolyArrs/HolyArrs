package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// route: GET /api/v1/settings/alerts
// route: PUT /api/v1/settings/alerts
func TestSettingsRoutesKeepOmittedFields(t *testing.T) {
	s, _ := open(t)
	mux := http.NewServeMux()
	(&Handlers{Store: s}).Routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	put := func(body string) int {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings/alerts", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if c := put(`{"pack_alerts":"each","each_limit":7}`); c != 200 {
		t.Fatalf("first put: %d", c)
	}
	if c := put(`{"pack_alerts":"one"}`); c != 200 {
		t.Fatalf("second put: %d", c)
	}
	resp, err := http.Get(srv.URL + "/api/v1/settings/alerts")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("get: %v %v", resp, err)
	}
	var got struct {
		PackAlerts string `json:"pack_alerts"`
		EachLimit  int    `json:"each_limit"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.PackAlerts != "one" || got.EachLimit != 7 {
		t.Fatalf("settings %+v: an omitted field lost its value", got)
	}
	if c := put(`{"each_limit":501}`); c != 400 {
		t.Fatalf("each_limit over the cap: %d, want 400", c)
	}
	if c := put(`{"pack_alerts":"sometimes"}`); c != 400 {
		t.Fatalf("unknown pack_alerts: %d, want 400", c)
	}
	if c := put(`not json`); c != 400 {
		t.Fatalf("bad body: %d, want 400", c)
	}
}

func TestRoutesWithoutVaultOnlySettings(t *testing.T) {
	s, _ := open(t)
	mux := http.NewServeMux()
	(&Handlers{Store: s}).Routes(mux)
	for _, c := range [][2]string{
		{"GET", "/api/v1/alerts"}, {"POST", "/api/v1/alerts"},
		{"PUT", "/api/v1/alerts/1"}, {"DELETE", "/api/v1/alerts/1"}, {"POST", "/api/v1/alerts/1/test"},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(c[0], c[1], strings.NewReader("{}")))
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s mounted without a vault: %d", c[0], c[1], rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/settings/alerts", nil))
	if rec.Code != 200 {
		t.Fatalf("settings without a vault: %d", rec.Code)
	}
}

// route: POST /api/v1/alerts/{id}/test
func TestWebhookTestSendsTestMoment(t *testing.T) {
	s, v := open(t)
	hook, got := fakeHook(t)
	a, err := Add(t.Context(), s, v, Input{Name: "Hook", Kind: "webhook", URL: hook.URL + "/h", Moments: []string{MomentReady}, AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&Handlers{Store: s, Secrets: v, Sender: &Sender{Store: s, Vault: v, Backoff: []time.Duration{}}}).Routes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/alerts/"+strconv.FormatInt(a.ID, 10)+"/test", nil))
	if rec.Code != 200 {
		t.Fatalf("test: %d %s", rec.Code, rec.Body)
	}
	g := got()
	if len(g) != 1 {
		t.Fatalf("hook called %d times", len(g))
	}
	var p Payload
	if err := json.Unmarshal(g[0].body, &p); err != nil {
		t.Fatal(err)
	}
	if p.Moment != MomentTest || !p.Test || g[0].header.Get(HeaderEvent) != MomentTest {
		t.Fatalf("payload %+v header %v: not marked as a test", p, g[0].header)
	}
}

// route: POST /api/v1/alerts/{id}/test
func TestAlertTestUsesOwnEmail(t *testing.T) {
	s, v := open(t)
	port, mails := fakeSMTP(t)
	a, err := Add(t.Context(), s, v, Input{Name: "Mail", Kind: "email", SMTPHost: "127.0.0.1", SMTPPort: port, SMTPSecurity: "none",
		SMTPFrom: "holyarrs@example.test", Moments: []string{MomentReady}, AllowLocal: &yes}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		email string
		fail  error
		want  int
	}{{"maya@example.test", nil, 200}, {"", context.Canceled, 400}} {
		var gotID int64
		mux := http.NewServeMux()
		(&Handlers{Store: s, Secrets: v, Sender: &Sender{Store: s, Vault: v, Backoff: []time.Duration{}},
			Caller:   func(*http.Request) (int64, string) { return 7, "Maya" },
			OwnEmail: func(_ context.Context, id int64) (string, error) { gotID = id; return c.email, c.fail },
		}).Routes(mux)
		before := len(mails())
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/alerts/"+strconv.FormatInt(a.ID, 10)+"/test", nil))
		if rec.Code != c.want || gotID != 7 {
			t.Fatalf("own email %q: %d %s, user %d", c.email, rec.Code, rec.Body, gotID)
		}
		got := mails()[before:]
		if c.want == 200 && (len(got) != 1 || len(got[0].to) != 1 || got[0].to[0] != c.email) {
			t.Fatalf("mail went to %+v, want %s", got, c.email)
		}
		if c.want != 200 && len(got) != 0 {
			t.Fatalf("mail sent without an address: %+v", got)
		}
	}
}

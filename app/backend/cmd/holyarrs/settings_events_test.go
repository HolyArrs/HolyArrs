package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Removing an item straight in the Downloader is logged as download_removed
// by the person who did it.
func TestClientItemRemoveEvent(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	f := &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	do(t, "POST", x.srv.URL+"/api/v1/downloaders/"+itoa(did)+"/items/"+strings.ToUpper(outHash)+"/remove", `{"delete_files":true}`, http.StatusOK)
	es, _, err := x.s.Events(context.Background(), database.EventFilter{Kinds: []string{"download_removed"}})
	if err != nil || len(es) != 1 {
		t.Fatalf("events %v %v", es, err)
	}
	if e := es[0]; e.Data["by_hand"] != true || e.Data["delete_files"] != true || e.Data["downloader"] != "Q" || e.Actor.Kind != "user" {
		t.Fatalf("event %+v", e)
	}
}

// A change to a named item keeps its name (read before a delete, after a
// change or add) in data.name and the summary.
func TestSettingsChangedName(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	h := x.api.settingsLog().Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			exec41(t, x.s, "UPDATE downloaders SET name = 'Q2' WHERE id = ?", did)
		} else {
			exec41(t, x.s, "UPDATE downloaders SET name = 'gone' WHERE id = ?", did) // the name is read before the delete
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, m := range []string{"PUT", "DELETE"} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(m, "/api/v1/downloaders/"+itoa(did), nil))
	}
	es, _, err := x.s.Events(context.Background(), database.EventFilter{Kinds: []string{health.EventSettingsChanged}})
	if err != nil || len(es) != 2 {
		t.Fatalf("events %v %v", es, err)
	}
	sums := map[string]bool{}
	for _, e := range es {
		if e.Data["name"] != "Q2" {
			t.Fatalf("name %+v", e.Data)
		}
		sums[e.Summary] = true
	}
	if !sums[`Downloader "Q2" changed by The owner`] || !sums[`Downloader "Q2" deleted by The owner`] {
		t.Fatalf("summaries %v", sums)
	}
	if x.api.settingsLog().Name(context.Background(), "settings", 0) != "" {
		t.Fatal("plain settings have no name")
	}
}

// Saving the search or Downloader settings leaves one audit Event (area and
// who) and never the older setting_changed row that held the values sent.
func TestSettingsSaveLeavesOnlyTheAuditEvent(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	for _, area := range []string{"search", "downloaders"} {
		body := do(t, "GET", x.srv.URL+"/api/v1/settings/"+area, "", http.StatusOK)
		do(t, "PUT", x.srv.URL+"/api/v1/settings/"+area, string(body), http.StatusOK)
	}
	old, _, err := x.s.Events(context.Background(), database.EventFilter{Kinds: []string{"setting_changed"}})
	if err != nil || len(old) != 0 {
		t.Fatalf("setting_changed rows %v %v", old, err)
	}
	es, _, err := x.s.Events(context.Background(), database.EventFilter{Kinds: []string{health.EventSettingsChanged}})
	if err != nil || len(es) != 2 {
		t.Fatalf("settings_changed rows %v %v", es, err)
	}
	for _, e := range es {
		if _, ok := e.Data["search"]; ok {
			t.Fatalf("values kept %+v", e.Data)
		}
		if _, ok := e.Data["downloaders"]; ok {
			t.Fatalf("values kept %+v", e.Data)
		}
		// A whole settings page names no list item (not "the newest Downloader").
		if _, ok := e.Data["name"]; ok {
			t.Fatalf("settings page named an item %+v", e)
		}
	}
}

// History rows name the member who did them, so readers without the
// member list still see who.
func TestEventsNameTheMember(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES (7, 't', '{}', 't', '{}', 'mara', 'member', 'x')`)
	id, name := int64(7), "mara"
	if err := x.s.AddEvent(context.Background(), &domain.Event{Actor: domain.Actor{Kind: "user", ID: id}, Kind: "title_added", Summary: "added"}); err != nil {
		t.Fatal(err)
	}
	body := string(do(t, "GET", x.srv.URL+"/api/v1/events?limit=5", "", http.StatusOK))
	if !strings.Contains(body, `"actor_name":"`+name+`"`) {
		t.Fatalf("no actor_name %q in %s", name, body)
	}
}

// Changing the sign-in settings (local-network skip, two-factor rules) is
// on the audit trail like any other settings page, and names no list item.
func TestSignInSettingsAreAudited(t *testing.T) {
	t.Parallel()
	f := newCtl(t, true)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	csrf := ""
	call := func(method, path, body string) (int, string) {
		req, _ := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			req.Header.Set(accounts.CSRFHeader, csrf)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	st, body := call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	var v map[string]any
	_ = json.Unmarshal([]byte(body), &v)
	csrf, _ = v["csrf"].(string)
	st, body = call("GET", "/api/v1/auth/settings", "")
	if st != 200 {
		t.Fatalf("get %d %s", st, body)
	}
	if st, b := call("PUT", "/api/v1/auth/settings", body); st >= 300 {
		t.Fatalf("put %d %s", st, b)
	}
	es, _, err := f.s.Events(context.Background(), database.EventFilter{Kinds: []string{health.EventSettingsChanged}})
	if err != nil || len(es) != 1 || es[0].Data["area"] != "sign-in" {
		t.Fatalf("events %+v %v", es, err)
	}
	if _, ok := es[0].Data["name"]; ok {
		t.Fatalf("named an item %+v", es[0])
	}
}

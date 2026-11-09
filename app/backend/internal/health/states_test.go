package health

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Every linked app reaches each state it can be in, with a reason in plain
// words for every state but running.
func TestLinkStates(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	str := func(s string) *string { return &s }
	now := time.Now().UTC()
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		src := func(name, state string, reason *string) error {
			_, err := database.Insert(tx, "sources", &domain.Source{Name: name, Protocol: "torrent", Format: "torznab", URL: "http://x/" + name,
				UseFor: domain.SourceUse{AutoSearch: true}, Kinds: []string{"movie"}, Categories: map[string][]int{"movie": {2000}},
				Limits: domain.SourceLimits{GapMS: 2000, SearchesPerWindow: 100, WindowSeconds: 3600}, Priority: 25, State: state,
				PauseReason: reason, Extra: map[string]string{}}, domain.System)
			return err
		}
		for _, x := range []struct {
			n, st string
			r     *string
		}{{"s-ok", "enabled", nil}, {"s-off", "disabled", nil}, {"s-down", "paused", str("down")}, {"s-key", "paused", str("login_refused")}, {"s-broken", "paused", str("broken_definition")}} {
			if err := src(x.n, x.st, x.r); err != nil {
				return err
			}
		}
		for _, x := range []struct{ n, st string }{{"d-ok", "ok"}, {"d-off", "disabled"}, {"d-down", "unreachable"}, {"d-auth", "auth_failed"}} {
			if _, err := database.Insert(tx, "downloaders", &domain.Downloader{Name: x.n, Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q/" + x.n,
				Category: "c", Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: x.st, LastError: "dial tcp 10.9.8.7:8080: /srv/private"}, domain.System); err != nil {
				return err
			}
		}
		for _, x := range []struct {
			n, st string
			r     any
		}{{"t-ok", "enabled", nil}, {"t-off", "disabled", nil}, {"t-slow", "paused", "rate_limited"}, {"t-key", "paused", "login_refused"}} {
			if _, err := tx.Exec(`INSERT INTO subtitle_sites (created_at, created_by, updated_at, updated_by, name, kind, url, state, pause_reason)
				VALUES ('t', '{}', 't', '{}', ?, 'fake', 'http://s', ?, ?)`, x.n, x.st, x.r); err != nil {
				return err
			}
		}
		for _, m := range []domain.MediaServer{{Name: "m-ok", Enabled: true, LastOKAt: &now}, {Name: "m-off"}, {Name: "m-new", Enabled: true},
			{Name: "m-bad", Enabled: true, LastError: "401 from http://10.9.8.7/x?token=abc"}} {
			m.Kind, m.URL = "jellyfin", "http://j/"+m.Name
			if _, err := database.Insert(tx, "media_servers", &m, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ls, err := w.e.Links(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]jobs.Status{}
	for _, l := range ls {
		got[l.Name] = l
	}
	want := map[string]jobs.State{
		"s-ok": jobs.Running, "s-off": jobs.Off, "s-down": jobs.Retrying, "s-key": jobs.Failed, "s-broken": jobs.Failed,
		"d-ok": jobs.Running, "d-off": jobs.Off, "d-down": jobs.Retrying, "d-auth": jobs.Failed,
		"t-ok": jobs.Running, "t-off": jobs.Off, "t-slow": jobs.Retrying, "t-key": jobs.Failed,
		"m-ok": jobs.Running, "m-off": jobs.Off, "m-new": jobs.Starting, "m-bad": jobs.Failed,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d links: %+v", len(got), ls)
	}
	for name, st := range want {
		l := got[name]
		if l.State != st {
			t.Errorf("%s: %s, want %s (%q)", name, l.State, st, l.Reason)
		}
		if strings.Contains(l.Reason, "10.9.8.7") || strings.Contains(l.Reason, "/srv") || strings.Contains(l.Reason, "token") {
			t.Errorf("%s: reason %q shows detail that belongs in the log", name, l.Reason)
		}
		if (st == jobs.Running) != (l.Reason == "") {
			t.Errorf("%s: reason %q in state %s", name, l.Reason, l.State)
		}
	}
	if got["d-down"].Kind != "downloader" || got["t-key"].Kind != "subtitle_site" || got["m-bad"].Kind != "media_server" || got["s-ok"].Kind != "source" {
		t.Errorf("kinds: %+v", ls)
	}
}

// An outside call waiting to try again reads as its kind in plain words; the
// breaker's fingerprint ID never reaches the row or its key.
func TestOutsideCallRowsHideTheBreakerID(t *testing.T) {
	w := newWorld(t)
	const fp = "B68gdslpF06jsy8_OogpQb7_"
	w.e.Outside = func() []outbound.State {
		return []outbound.State{
			{ID: fp, Kind: "metadata_tmdb", Status: "retrying", Reason: "It does not answer; HolyArrs tries again on its own."},
			{ID: "Zz9_other-fingerprint", Kind: "metadata_tmdb", Status: "retrying"},
			{ID: "Qq1_third", Kind: "downloader", Status: "retrying"},
		}
	}
	ls, err := w.e.Links(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var calls []jobs.Status
	for _, l := range ls {
		if l.Kind == "outside_call" {
			calls = append(calls, l)
		}
	}
	if len(calls) != 3 {
		t.Fatalf("want 3 outside rows, got %d: %+v", len(calls), calls)
	}
	want := []struct{ name, key string }{{"Metadata server", "outside:metadata_tmdb:1"}, {"Metadata server", "outside:metadata_tmdb:2"}, {"Downloader", "outside:downloader:1"}}
	for i, c := range calls {
		if c.Name != want[i].name || c.Key != want[i].key || c.State != jobs.Retrying {
			t.Errorf("row %d = %q %q %v, want %q %q retrying", i, c.Name, c.Key, c.State, want[i].name, want[i].key)
		}
		for _, id := range []string{fp, "Zz9_other-fingerprint", "Qq1_third"} {
			if strings.Contains(c.Name+c.Key+c.Reason, id) {
				t.Errorf("row %d shows the breaker ID %s", i, id)
			}
		}
	}
	if outsideName("something_new") != "Outside service" {
		t.Error("an unknown kind needs a plain fallback name")
	}
}

package health

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
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
			_, err := store.Insert(tx, "sources", &domain.Source{Name: name, Protocol: "torrent", Format: "torznab", URL: "http://x/" + name,
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
			if _, err := store.Insert(tx, "downloaders", &domain.Downloader{Name: x.n, Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q/" + x.n,
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
			if _, err := store.Insert(tx, "media_servers", &m, domain.System); err != nil {
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
	got := map[string]supervisor.Status{}
	for _, l := range ls {
		got[l.Name] = l
	}
	want := map[string]supervisor.State{
		"s-ok": supervisor.Running, "s-off": supervisor.Off, "s-down": supervisor.Retrying, "s-key": supervisor.Failed, "s-broken": supervisor.Failed,
		"d-ok": supervisor.Running, "d-off": supervisor.Off, "d-down": supervisor.Retrying, "d-auth": supervisor.Failed,
		"t-ok": supervisor.Running, "t-off": supervisor.Off, "t-slow": supervisor.Retrying, "t-key": supervisor.Failed,
		"m-ok": supervisor.Running, "m-off": supervisor.Off, "m-new": supervisor.Starting, "m-bad": supervisor.Failed,
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
		if (st == supervisor.Running) != (l.Reason == "") {
			t.Errorf("%s: reason %q in state %s", name, l.Reason, l.State)
		}
	}
	if got["d-down"].Kind != "downloader" || got["t-key"].Kind != "subtitle_site" || got["m-bad"].Kind != "media_server" || got["s-ok"].Kind != "source" {
		t.Errorf("kinds: %+v", ls)
	}
}

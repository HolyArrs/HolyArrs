package musictrend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	rgFresh = "a1111111-1111-4111-8111-111111111111"
	rgUPC   = "a2222222-2222-4222-8222-222222222222"
	rgName  = "a3333333-3333-4333-8333-333333333333"
	rgSoon  = "a4444444-4444-4444-8444-444444444444"
	artNew  = "b1111111-1111-4111-8111-111111111111"
)

func freshServer(t *testing.T, agent *atomic.Value, fail *atomic.Bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent.Store(r.Header.Get("User-Agent"))
		if fail != nil && fail.Load() {
			w.WriteHeader(500)
			return
		}
		if r.URL.Query().Get("days") != "90" || r.URL.Query().Get("future") != "true" {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(`{"payload":{"releases":[
{"artist_credit_name":"Fresh Act","artist_mbids":["` + artNew + `"],"caa_id":42,"caa_release_mbid":"d0000000-0000-4000-8000-000000000000","release_date":"2026-10-19","release_group_mbid":"` + rgFresh + `","release_group_primary_type":"Album","release_name":"Fresh One","listen_count":9},
{"artist_credit_name":"Soon Act","release_date":"2026-11-02","release_group_mbid":"` + rgSoon + `","release_name":"Soon","release_group_primary_type":"EP"},
{"artist_credit_name":"Bad","release_date":"2026-10-19","release_group_mbid":"nope","release_name":"x"}]}}`))
	}))
}

// fixedSource is a new-release source answering fixed lists.
func fixedSource(name string, albums, tracks []NewItem, calls *int) NewSource {
	return NewSource{Name: name, Read: func(context.Context) ([]NewItem, []NewItem, error) {
		*calls++
		return albums, tracks, nil
	}}
}

// Each window comes from the first source in the order spotify81, Apple
// Music, ListenBrainz that has items for it; every item carries its source.
func TestNewMusicSourceOrder(t *testing.T) {
	var agent atomic.Value
	srv := freshServer(t, &agent, nil)
	defer srv.Close()
	now := func() time.Time { return time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC) }
	n, err := OpenNew(filepath.Join(t.TempDir(), "new.sqlite"), NewOptions{FreshURL: srv.URL + "/lb/", UserAgent: "polite-agent", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var sc, ac int
	// spotify81: one album this week, no songs. Apple: a recent album, a
	// week album (not used: spotify81 had the week) and a song chart.
	n.SetSources(
		fixedSource(SrcLists, []NewItem{{MBID: rgUPC, ArtistMBID: artNew, Title: "Spot Week", ReleaseDate: "2026-10-20"}}, nil, &sc),
		fixedSource(SrcApple, []NewItem{{MBID: rgName, ArtistMBID: artNew, Title: "Apple Recent", ReleaseDate: "2026-10-01"}, {MBID: rgFresh, Title: "Apple Week", ReleaseDate: "2026-10-19"}},
			[]NewItem{{ArtistMBID: artNew, Title: "Hit", Track: "Hit", ReleaseDate: "2026-10-18"}, {Title: "No artist id"}}, &ac),
	)
	if err := n.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sc != 1 || ac != 1 || agent.Load() != "polite-agent" {
		t.Fatalf("reads %d %d agent %v", sc, ac, agent.Load())
	}
	week := window(t, n, WindowWeek)
	if len(week.Items) != 1 || week.Items[0].Title != "Spot Week" || week.Items[0].Source != SrcLists {
		t.Fatalf("week %+v", week.Items)
	}
	recent := window(t, n, WindowRecent)
	if len(recent.Items) != 1 || recent.Items[0].Source != SrcApple {
		t.Fatalf("recent %+v", recent.Items)
	}
	tracks := window(t, n, WindowTracks)
	if len(tracks.Items) != 1 || tracks.Items[0].Source != SrcApple || tracks.Items[0].Track != "Hit" {
		t.Fatalf("tracks %+v", tracks.Items)
	}
	// Only ListenBrainz has the coming releases.
	soon := window(t, n, WindowUpcoming)
	if len(soon.Items) != 1 || soon.Items[0].MBID != rgSoon || soon.Items[0].Source != SrcListenBrainz || soon.Items[0].Type != "ep" {
		t.Fatalf("upcoming %+v", soon.Items)
	}
	b, _, _ := n.Window(WindowWeek)
	if !strings.Contains(string(b), `"source":"spotify81"`) {
		t.Fatalf("window body without its source: %s", b)
	}
}

// With spotify81 and Apple empty, ListenBrainz fills the windows.
func TestNewMusicListenBrainzBackup(t *testing.T) {
	var agent atomic.Value
	srv := freshServer(t, &agent, nil)
	defer srv.Close()
	now := func() time.Time { return time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC) }
	n, err := OpenNew(filepath.Join(t.TempDir(), "new.sqlite"), NewOptions{FreshURL: srv.URL + "/lb/", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	var sc, ac int
	n.SetSources(fixedSource(SrcLists, nil, nil, &sc), fixedSource(SrcApple, nil, nil, &ac))
	if err := n.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	week := window(t, n, WindowWeek)
	if len(week.Items) != 1 || week.Items[0].MBID != rgFresh || week.Items[0].Source != SrcListenBrainz || !strings.Contains(week.Items[0].Cover, "coverartarchive.org/release/") {
		t.Fatalf("week %+v", week.Items)
	}
}

// Windows stored before the source tags are not served after a restart.
func TestNewMusicDropsUntaggedStoredWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.sqlite")
	n, err := OpenNew(path, NewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = n.db.Exec(`INSERT INTO windows (window, body) VALUES ('week', '{"schema_version":1,"items":[{"mbid":"x"}]}')`)
	n.Close()
	n2, err := OpenNew(path, NewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer n2.Close()
	if _, _, ok := n2.Window(WindowWeek); ok {
		t.Fatal("old untagged window served")
	}
}

type newBody struct {
	Window string    `json:"window"`
	Items  []NewItem `json:"items"`
}

func window(t *testing.T, n *NewMusic, w string) newBody {
	t.Helper()
	b, etag, ok := n.Window(w)
	if !ok || etag == "" {
		t.Fatalf("%s missing", w)
	}
	var out newBody
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNewMusicKeepsLastWindowsWhenSourcesFail(t *testing.T) {
	var agent atomic.Value
	var fail atomic.Bool
	srv := freshServer(t, &agent, &fail)
	defer srv.Close()
	now := func() time.Time { return time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC) }
	n, err := OpenNew(filepath.Join(t.TempDir(), "new.sqlite"), NewOptions{FreshURL: srv.URL + "/lb/", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if err := n.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, before, _ := n.Window(WindowWeek)
	fail.Store(true)
	if err := n.Refresh(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if _, after, ok := n.Window(WindowWeek); !ok || after != before {
		t.Fatal("lost last good window")
	}
}

// M6: a window is tagged with the source its body names, and rows stamped
// listenbrainz before are stamped again with their real source.
func TestNewMusicWindowsTaggedWithRealSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.sqlite")
	n, err := OpenNew(path, NewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = n.db.Exec(`INSERT INTO windows (window, body) VALUES ('week', '{"source":"spotify81","items":[]}'), ('month', '{"items":[]}')`)
	var src string
	_ = n.db.QueryRow(`SELECT src FROM windows WHERE window='week'`).Scan(&src)
	if src != "spotify81" {
		t.Fatalf("new write tagged %q", src)
	}
	// An old row stamped listenbrainz by the earlier trigger.
	_, _ = n.db.Exec(`UPDATE windows SET src='listenbrainz' WHERE window='week'`)
	n.Close()
	n2, err := OpenNew(path, NewOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer n2.Close()
	_ = n2.db.QueryRow(`SELECT src FROM windows WHERE window='week'`).Scan(&src)
	if src != "spotify81" {
		t.Fatalf("not re-stamped: %q", src)
	}
	_ = n2.db.QueryRow(`SELECT src FROM windows WHERE window='month'`).Scan(&src)
	if src != "listenbrainz" {
		t.Fatalf("no source in body: %q", src)
	}
}

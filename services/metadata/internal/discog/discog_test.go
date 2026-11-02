package discog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/fetchq"
	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

const (
	artistID = "11111111-1111-1111-1111-111111111111"
	albumID  = "22222222-2222-2222-2222-222222222222"
)

func gid(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

// fakeMB answers release-group browses for an artist with n groups and
// release browses / lookups for one album.
type fakeMB struct {
	groups   int
	releases []map[string]any
	tracks   map[string]any
	calls    []string
	fail     error
}

func (f *fakeMB) get(_ context.Context, path string, v url.Values, out any) error {
	f.calls = append(f.calls, path+"?"+v.Encode())
	if f.fail != nil {
		return f.fail
	}
	var body any
	switch path {
	case "/release-group":
		off, _ := strconv.Atoi(v.Get("offset"))
		lim, _ := strconv.Atoi(v.Get("limit"))
		var list []map[string]any
		for i := off; i < min(off+lim, f.groups); i++ {
			list = append(list, map[string]any{"id": gid(i), "title": "G" + strconv.Itoa(i), "primary-type": "Album", "secondary-types": []string{}, "first-release-date": fmt.Sprintf("2000-01-%02d", i%28+1)})
		}
		body = map[string]any{"release-group-count": f.groups, "release-group-offset": off, "release-groups": list}
	case "/release":
		off, _ := strconv.Atoi(v.Get("offset"))
		lim, _ := strconv.Atoi(v.Get("limit"))
		body = map[string]any{"release-count": len(f.releases), "releases": f.releases[min(off, len(f.releases)):min(off+lim, len(f.releases))]}
	default:
		body = f.tracks
	}
	b, _ := json.Marshal(body)
	return json.Unmarshal(b, out)
}

func open(t *testing.T, f *fakeMB, o Options) *Store {
	t.Helper()
	o.Get = f.get
	o.Logf = func(string, ...any) {}
	s, err := Open(filepath.Join(t.TempDir(), "d.sqlite"), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// walk runs a job until it stops asking for more steps.
func walk(t *testing.T, s *Store, key string, p int) (steps int, err error) {
	t.Helper()
	for steps = 1; steps < 100; steps++ {
		if err = s.Run(context.Background(), key, p); !errors.Is(err, fetchq.ErrMore) {
			return steps, err
		}
	}
	t.Fatal("walk never ended")
	return
}

func TestArtistWalkStoresEveryGroup(t *testing.T) {
	f := &fakeMB{groups: 259}
	s := open(t, f, Options{})
	steps, err := walk(t, s, ArtistKey(artistID), fetchq.P0Waiting)
	if err != nil || steps != 3 || len(f.calls) != 3 {
		t.Fatalf("steps %d calls %v err %v", steps, f.calls, err)
	}
	a, ok := s.Artist(context.Background(), artistID)
	if !ok || len(a.Groups) != 259 || a.Count != 259 || !a.Complete {
		t.Fatalf("stored %v %d %d %v", ok, len(a.Groups), a.Count, a.Complete)
	}
	for i := 1; i < len(a.Groups); i++ {
		if a.Groups[i-1].FirstReleaseDate > a.Groups[i].FirstReleaseDate {
			t.Fatal("not oldest first")
		}
	}
	// Fresh: the next job makes no call.
	if err := s.Run(context.Background(), ArtistKey(artistID), fetchq.P3Refresh); err != nil || len(f.calls) != 3 {
		t.Fatalf("fresh list walked again: %v %d", err, len(f.calls))
	}
}

func TestArtistWalkCap(t *testing.T) {
	f := &fakeMB{groups: 450}
	s := open(t, f, Options{MaxGroups: 200})
	if _, err := walk(t, s, ArtistKey(artistID), fetchq.P0Waiting); err != nil {
		t.Fatal(err)
	}
	a, _ := s.Artist(context.Background(), artistID)
	if len(a.Groups) != 200 || a.Count != 450 || a.Complete || len(f.calls) != 2 {
		t.Fatalf("capped %d %d %v calls %d", len(a.Groups), a.Count, a.Complete, len(f.calls))
	}
}

func TestAdoptsLeanArtistAnswer(t *testing.T) {
	f := &fakeMB{groups: 3}
	stored := fmt.Sprintf(`{"source_data":{"country":"US","release-group-count":2,"release-groups":[{"id":%q,"title":"A"},{"id":%q,"title":"B"}]}}`, gid(1), gid(2))
	s := open(t, f, Options{Stored: func(context.Context, string, string) ([]byte, bool) { return []byte(stored), true }})
	if err := s.Run(context.Background(), ArtistKey(artistID), fetchq.P1Finish); err != nil || len(f.calls) != 0 {
		t.Fatalf("adopt: %v calls %v", err, f.calls)
	}
	a, ok := s.Artist(context.Background(), artistID)
	if !ok || len(a.Groups) != 2 || !a.Complete || a.Groups[0].SecondaryTypes == nil {
		t.Fatalf("adopted %+v", a)
	}
	// An old (pre-lean) answer of 25 with no count is walked instead.
	s2 := open(t, f, Options{Stored: func(context.Context, string, string) ([]byte, bool) {
		return []byte(`{"source_data":{"release-groups":[{"id":"` + gid(1) + `"}]}}`), true
	}})
	if _, err := walk(t, s2, ArtistKey(artistID), fetchq.P1Finish); err != nil || len(f.calls) != 1 {
		t.Fatalf("old answer: %v calls %v", err, f.calls)
	}
}

func TestCheckAndFeedMarks(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	f := &fakeMB{groups: 150}
	s := open(t, f, Options{Now: func() time.Time { return now }})
	walk(t, s, ArtistKey(artistID), fetchq.P0Waiting)
	// A week on: one first-page call, nothing new, list kept.
	now = now.Add(8 * 24 * time.Hour)
	f.calls = nil
	if err := s.Run(context.Background(), ArtistKey(artistID), fetchq.P3Refresh); err != nil || len(f.calls) != 1 {
		t.Fatalf("check %v %v", err, f.calls)
	}
	if a, _ := s.Artist(context.Background(), artistID); s.ArtistDue(a) {
		t.Fatal("still due after check")
	}
	// A new release group: the check walks the whole list.
	now = now.Add(8 * 24 * time.Hour)
	f.groups, f.calls = 151, nil
	if _, err := walk(t, s, ArtistKey(artistID), fetchq.P3Refresh); err != nil || len(f.calls) != 2 {
		t.Fatalf("rewalk %v %v", err, f.calls)
	}
	if a, _ := s.Artist(context.Background(), artistID); len(a.Groups) != 151 {
		t.Fatalf("new group missing: %d", len(a.Groups))
	}
	// The feed marks the list when one of its release groups changes.
	if n := s.MarkChanged(context.Background(), []string{gid(7), "99999999-9999-9999-9999-999999999999"}); n != 1 {
		t.Fatalf("marked %d", n)
	}
	a, _ := s.Artist(context.Background(), artistID)
	if !a.Stale || !s.ArtistDue(a) {
		t.Fatal("not stale")
	}
	if ar, _ := s.Due(context.Background(), 10); len(ar) != 1 {
		t.Fatalf("due %v", ar)
	}
}

func rel(id, status, country, format string, tracks int, date string) map[string]any {
	return map[string]any{"id": id, "title": "Globalization", "status": status, "country": country, "date": date,
		"media":         []any{map[string]any{"position": 1, "format": format, "track-count": tracks}},
		"artist-credit": []any{map[string]any{"name": "Pitbull", "artist": map[string]any{"id": artistID, "name": "Pitbull"}}},
		"release-group": map[string]any{"title": "Globalization", "primary-type": "Album", "secondary-types": []string{}, "first-release-date": "2014-11-21"}}
}

func TestAlbumWalkChoosesAndStoresTracks(t *testing.T) {
	chosen := "33333333-3333-3333-3333-333333333333"
	f := &fakeMB{releases: []map[string]any{
		rel("44444444-4444-4444-4444-444444444444", "Official", "US", "CD", 16, "2014-11-21"), // deluxe
		rel(chosen, "Official", "US", "Digital Media", 12, "2014-11-21"),
		rel("55555555-5555-5555-5555-555555555555", "Official", "JP", "CD", 12, "2014-11-20"),
		rel("66666666-6666-6666-6666-666666666666", "Bootleg", "US", "CD", 12, "2014-01-01"),
	}, tracks: map[string]any{"id": chosen, "artist-credit": []any{map[string]any{"name": "Pitbull"}}, "media": []any{map[string]any{"position": 1, "format": "Digital Media", "tracks": []any{
		map[string]any{"position": 1, "number": "1", "title": "Fireball", "length": 235000, "recording": map[string]any{"id": "77777777-7777-7777-7777-777777777777"}, "artist-credit": []any{map[string]any{"name": "Pitbull", "joinphrase": " feat. "}, map[string]any{"name": "John Ryan"}}},
		map[string]any{"position": 2, "number": "2", "title": "Wild Wild Love", "length": nil, "recording": map[string]any{"id": "88888888-8888-8888-8888-888888888888", "length": 199000}},
	}}}}}
	s := open(t, f, Options{Stored: func(_ context.Context, kind, id string) ([]byte, bool) {
		return []byte(`{"source_data":{"country":"US"}}`), kind == "artist" && id == artistID
	}})
	steps, err := walk(t, s, AlbumKey(albumID), fetchq.P0Waiting)
	if err != nil || steps != 2 || len(f.calls) != 2 {
		t.Fatalf("steps %d %v %v", steps, err, f.calls)
	}
	a, ok := s.Album(context.Background(), albumID)
	if !ok || a.Edition == nil || a.Edition.ID != chosen || a.Title != "Globalization" || len(a.Editions) != 4 || a.EditionCount != 4 || !a.EditionsComplete {
		t.Fatalf("album %+v", a)
	}
	if len(a.Tracks) != 2 || a.Tracks[0].ArtistCredit != "Pitbull feat. John Ryan" || a.Tracks[1].ArtistCredit != "" || a.Tracks[1].LengthMS != 199000 || a.LengthMS != 434000 || a.DiscCount != 1 || a.Tracks[0].Disc != 1 {
		t.Fatalf("tracks %+v", a)
	}
	if s.MarkChanged(context.Background(), []string{chosen}) != 1 {
		t.Fatal("chosen edition change not marked")
	}
}

func TestChoose(t *testing.T) {
	e := func(id, status, country string, n int, date string, formats ...string) Edition {
		return Edition{ID: id, Status: status, Countries: []string{country}, TrackCount: n, Date: date, Formats: formats}
	}
	cases := []struct {
		eds  []Edition
		home string
		want string
	}{
		{nil, "", ""},
		{[]Edition{e("a", "Promotion", "US", 3, "")}, "", "a"},
		// Worldwide digital beats an earlier vinyl from elsewhere.
		{[]Edition{e("a", "Official", "DE", 10, "2000", "12\" Vinyl"), e("b", "Official", "XW", 10, "2001", "Digital Media")}, "US", "b"},
		// Home country counts like worldwide; earliest wins.
		{[]Edition{e("b", "Official", "GB", 10, "2001", "CD"), e("a", "Official", "GB", 10, "2000", "CD"), e("c", "Official", "JP", 10, "1999", "CD")}, "GB", "a"},
		// The most common track count wins over the region.
		{[]Edition{e("a", "Official", "US", 14, "2000", "CD"), e("b", "Official", "JP", 12, "2000", "CD"), e("c", "Official", "DE", 12, "2000", "CD")}, "US", "b"},
	}
	for i, c := range cases {
		if got := Choose(c.eds, c.home); got != c.want {
			t.Fatalf("case %d: %q want %q", i, got, c.want)
		}
	}
}

func TestPausesBackgroundAfterFailures(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	f := &fakeMB{groups: 5, fail: &model.FetchError{Code: "upstream_unavailable", Status: 503}}
	s := open(t, f, Options{Now: func() time.Time { return now }})
	for i := range 3 {
		if err := s.Run(context.Background(), ArtistKey(gid(i)), fetchq.P2Shared); !errors.Is(err, fetchq.ErrThrottled) {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	if p, _ := s.Paused(); !p {
		t.Fatal("not paused")
	}
	n := len(f.calls)
	if err := s.Run(context.Background(), ArtistKey(gid(9)), fetchq.P3Refresh); !errors.Is(err, fetchq.ErrConnection) || len(f.calls) != n {
		t.Fatalf("background ran while paused: %v", err)
	}
	// A page someone waits on still asks (at the shared pace).
	f.fail = nil
	if _, err := walk(t, s, ArtistKey(gid(9)), fetchq.P0Waiting); err != nil || len(f.calls) != n+1 {
		t.Fatalf("waiting ask blocked: %v", err)
	}
	now = now.Add(16 * time.Minute)
	if p, _ := s.Paused(); p {
		t.Fatal("still paused")
	}
}

func TestBudgetSpentWaits(t *testing.T) {
	f := &fakeMB{groups: 5}
	s := open(t, f, Options{Take: func() bool { return false }})
	if err := s.Run(context.Background(), ArtistKey(artistID), fetchq.P0Waiting); !errors.Is(err, fetchq.ErrThrottled) || len(f.calls) != 0 {
		t.Fatalf("spent budget: %v %v", err, f.calls)
	}
}

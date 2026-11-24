package mediaservers

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestPickWatched(t *testing.T) {
	now := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	old, recent := now.AddDate(0, 0, -40), now.AddDate(0, 0, -5)
	ana, bo, cy := watchAccount{1, "ana"}, watchAccount{1, "bo"}, watchAccount{2, "cy"}
	movie := cleanupTitle{id: 1, kind: "movie", name: "Dune", bytes: 9000}
	show := cleanupTitle{id: 2, kind: "show", name: "Severance", bytes: 5000, eps: []seenKey{{2, 1, 1}, {2, 1, 2}}}
	asked := cleanupTitle{id: 3, kind: "movie", name: "Arrival", bytes: 100, askers: []int64{7}}
	noFiles := cleanupTitle{id: 4, kind: "show", name: "Empty"}
	all := []cleanupTitle{movie, show, asked, noFiles}
	at := func(ks ...any) map[seenKey]time.Time {
		m := map[seenKey]time.Time{}
		for i := 0; i < len(ks); i += 2 {
			m[ks[i].(seenKey)] = ks[i+1].(time.Time)
		}
		return m
	}
	m1, m3, e1, e2 := seenKey{title: 1}, seenKey{title: 3}, seenKey{2, 1, 1}, seenKey{2, 1, 2}
	for _, c := range []struct {
		name   string
		seen   map[watchAccount]map[seenKey]time.Time
		chosen []watchAccount
		askers map[int64][]watchAccount
		want   []int64
	}{
		{name: "nobody chosen lists nothing", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m1, old)}, want: []int64{}},
		{name: "one person finished the movie long ago", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m1, old)}, chosen: []watchAccount{ana}, want: []int64{1}},
		{name: "finished too recently", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m1, recent)}, chosen: []watchAccount{ana}, want: []int64{}},
		{name: "everyone must finish", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m1, old)}, chosen: []watchAccount{ana, bo}, want: []int64{}},
		{name: "the latest finish counts", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m1, old), cy: at(m1, recent)}, chosen: []watchAccount{ana, cy}, want: []int64{}},
		{name: "an unchosen person is not waited for", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m1, old)}, chosen: []watchAccount{ana}, want: []int64{1}},
		{name: "show half watched", seen: map[watchAccount]map[seenKey]time.Time{ana: at(e1, old)}, chosen: []watchAccount{ana}, want: []int64{}},
		{name: "show every episode with a file watched", seen: map[watchAccount]map[seenKey]time.Time{ana: at(e1, old, e2, old)}, chosen: []watchAccount{ana}, want: []int64{2}},
		{name: "show finished on two servers", seen: map[watchAccount]map[seenKey]time.Time{ana: at(e1, old, e2, old), cy: at(e1, old, e2, old)}, chosen: []watchAccount{ana, cy}, want: []int64{2}},
		{name: "asker has not watched", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m3, old)}, chosen: []watchAccount{ana}, askers: map[int64][]watchAccount{7: {bo}}, want: []int64{}},
		{name: "asker with no account stays protected", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m3, old)}, chosen: []watchAccount{ana}, want: []int64{}},
		{name: "asker watched on their own account", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m3, old), bo: at(m3, recent)}, chosen: []watchAccount{ana}, askers: map[int64][]watchAccount{7: {bo}}, want: []int64{3}},
		{name: "biggest first kept", seen: map[watchAccount]map[seenKey]time.Time{ana: at(m1, old, e1, old, e2, old)}, chosen: []watchAccount{ana}, want: []int64{1, 2}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := []int64{}
			for _, w := range pickWatched(all, c.seen, c.chosen, c.askers, now.AddDate(0, 0, -30)) {
				got = append(got, w.ID)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestParseCleanupAsk(t *testing.T) {
	for _, c := range []struct {
		days, people string
		wantDays     int
		wantPeople   map[string]bool
	}{
		{"", "", 30, nil},
		{"0", " , ", 0, nil},
		{"7", "1:22,2:u1", 7, map[string]bool{"1:22": true, "2:u1": true}},
		{"-1", "1:a:b", 30, map[string]bool{"1:a:b": true}},
		{"99999", "", 30, nil},
		{"abc", "", 30, nil},
	} {
		d, p := parseCleanupAsk(c.days, c.people)
		if d != c.wantDays || !reflect.DeepEqual(p, c.wantPeople) {
			t.Fatalf("%q %q: got %d %v", c.days, c.people, d, p)
		}
	}
}

// withMovieFile gives the movie one present file of size bytes, protected
// when asked.
func withMovieFile(t *testing.T, w *world, title int64, size int64, protected bool) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr, lf int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		_ = tx.QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&lf)
		v := &mediafiles.Version{TitleID: title, Label: "HD", PickRulesID: pr, LibraryFolderID: lf, FolderName: "x", IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		f := &mediafiles.File{SlotIDs: []int64{}, VersionID: v.ID, RelativePath: "x/x.mkv", Size: size, Languages: []string{}, MediaInfo: map[string]any{},
			Origin: "imported", PlacedBy: "none", Role: "main", State: "present", Protected: protected}
		_, err := database.Insert(tx, "files", f, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The list read from the database: a movie everyone watched long ago shows
// with its size, everyone is chosen by default, and a protected copy or an
// ask by someone who has not watched keeps it off.
func TestWatchedCleanupList(t *testing.T) {
	w, movie, _ := mineWorld(t)
	ctx := context.Background()
	now := time.Now().UTC()
	withMovieFile(t, w, movie, 4096, false)
	seen(t, w, "Plex", "22", "ana", movie, 0, 0)
	seen(t, w, "Jelly", "u1", "ana", movie, 0, 0)
	got, err := WatchedCleanupList(ctx, w.s.Read(), 0, nil, nil, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Found) != 1 || got.Found[0].ID != movie || got.Found[0].Bytes != 4096 || len(got.People) != 2 || !got.People[0].Chosen {
		t.Fatalf("got %+v", got)
	}
	if got, _ := WatchedCleanupList(ctx, w.s.Read(), 30, nil, nil, now); len(got.Found) != 0 {
		t.Fatalf("watched just now must wait 30 days: %+v", got.Found)
	}
	// Bo asked for it and has no account: protected until he watched.
	if got, _ := WatchedCleanupList(ctx, w.s.Read(), 0, nil, map[int64][]int64{movie: {9}}, now.Add(time.Hour)); len(got.Found) != 0 {
		t.Fatalf("an asker who has not watched keeps it: %+v", got.Found)
	}
	// Watch history off: nothing listed and nobody named.
	watchOn(t, w, false)
	if got, _ := WatchedCleanupList(ctx, w.s.Read(), 0, nil, nil, now); got.Enabled || len(got.People) != 0 {
		t.Fatalf("off: %+v", got)
	}
}

func TestWatchedCleanupSkipsProtectedCopies(t *testing.T) {
	w, movie, _ := mineWorld(t)
	withMovieFile(t, w, movie, 4096, true)
	seen(t, w, "Plex", "22", "ana", movie, 0, 0)
	got, err := WatchedCleanupList(context.Background(), w.s.Read(), 0, nil, nil, time.Now().UTC().Add(time.Hour))
	if err != nil || len(got.Found) != 0 {
		t.Fatalf("got %+v %v", got.Found, err)
	}
}

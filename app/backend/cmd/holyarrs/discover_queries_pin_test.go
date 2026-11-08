package main

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

var rdpNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

const rdpStamp = "2026-10-13T12:00:00Z"

// rdpTitle inserts a title the way the Library leaves it. tmdb "" stores no
// TMDB id (json_extract gives NULL).
func rdpTitle(t *testing.T, st *database.Store, kind, name, tmdb, state, created string) int64 {
	t.Helper()
	ids := "{}"
	if tmdb != "" {
		ids = fmt.Sprintf(`{"tmdb":"%s"}`, tmdb)
	}
	var id int64
	err := st.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, genres, metadata_source, why_added, state)
			VALUES (?, '{}', ?, '{}', ?, ?, ?, 2020, ?, '["Drama"]', 'tmdb', 'import', ?)`, created, created, kind, name, name, ids, state)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func rdpEpisode(t *testing.T, st *database.Store, title int64, air string, dropped bool) {
	t.Helper()
	err := st.Write(context.Background(), func(tx *sql.Tx) error {
		var drop any
		if dropped {
			drop = rdpStamp
		}
		_, err := tx.Exec(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, air_date, dropped_at) VALUES (?, '{}', ?, '{}', ?, 'episode', ?, ?)`,
			rdpStamp, rdpStamp, title, air, drop)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func rdpExec(t *testing.T, st *database.Store, q string, args ...any) {
	t.Helper()
	if err := st.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// exploreReturning: Library shows whose next episode (the earliest one after
// now) airs 7 to 60 days out, soonest first, at most 40. Shows with no TMDB
// id, removed shows, dropped episodes and movies are left out.
func TestRDPExploreReturningOverALargeLibrary(t *testing.T) {
	r := newAskRig(t)
	ctx := context.Background()
	day := func(n int) string { return database.FormatTime(rdpNow.AddDate(0, 0, n)) }
	// 60 shows in the window, the later ones airing later.
	for i := 0; i < 60; i++ {
		id := rdpTitle(t, r.st, "show", fmt.Sprintf("In %02d", i), fmt.Sprint(5000+i), "active", rdpStamp)
		rdpEpisode(t, r.st, id, day(8+i/2*1), false)
	}
	// 100 shows outside it: too soon, too far, or already over.
	for i := 0; i < 100; i++ {
		id := rdpTitle(t, r.st, "show", fmt.Sprintf("Out %03d", i), fmt.Sprint(6000+i), "active", rdpStamp)
		switch i % 3 {
		case 0:
			rdpEpisode(t, r.st, id, day(3), false)
		case 1:
			rdpEpisode(t, r.st, id, day(90), false)
		default:
			rdpEpisode(t, r.st, id, day(-5), false)
		}
	}
	// A show with an episode in 3 days and another in 20: its next one is
	// the 3-day one, so it is out (the quirk is pinned).
	two := rdpTitle(t, r.st, "show", "Two Soon", "7001", "active", rdpStamp)
	rdpEpisode(t, r.st, two, day(3), false)
	rdpEpisode(t, r.st, two, day(20), false)
	// No TMDB id, removed, dropped episode, and a movie with an episode row.
	rdpEpisode(t, r.st, rdpTitle(t, r.st, "show", "No Id", "", "active", rdpStamp), day(10), false)
	rdpEpisode(t, r.st, rdpTitle(t, r.st, "show", "Gone", "7002", "removed", rdpStamp), day(10), false)
	rdpEpisode(t, r.st, rdpTitle(t, r.st, "show", "Dropped", "7003", "active", rdpStamp), day(10), true)
	rdpEpisode(t, r.st, rdpTitle(t, r.st, "movie", "Film", "7004", "active", rdpStamp), day(10), false)

	got, err := discover.ExploreReturning(ctx, r.st.Read(), rdpNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 40 {
		t.Fatalf("%d returning shows, want the 40 cap", len(got))
	}
	for i, it := range got {
		if it.Kind != "show" || !it.Owned || it.TMDBID != fmt.Sprint(5000+i) || it.Name != fmt.Sprintf("In %02d", i) {
			t.Fatalf("row %d: %+v", i, it)
		}
		if i > 0 && it.Released < got[i-1].Released {
			t.Fatalf("row %d not soonest first: %s after %s", i, it.Released, got[i-1].Released)
		}
	}
	if got[0].Released != rdpNow.AddDate(0, 0, 8).Format("2006-01-02") {
		t.Fatalf("first airs %s", got[0].Released)
	}
}

// exploreCollections: TMDB collections with an owned (not removed) movie,
// most owned first then by id; local collections and ones with nothing
// owned are left out.
func TestRDPExploreCollectionsCountOwnedMovies(t *testing.T) {
	r := newAskRig(t)
	ctx := context.Background()
	mk := func(source, name string) int64 {
		var id int64
		err := r.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO movie_collections (source_id, name) VALUES (?, ?)", source, name)
			id, _ = res.LastInsertId()
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	put := func(col int64, title int64) {
		rdpExec(t, r.st, "INSERT INTO movie_collection_titles (collection_id, title_id) VALUES (?, ?)", col, title)
	}
	a, b, c, d, empty := mk("tmdb:10", "Alpha"), mk("tmdb:20", "Beta"), mk("local:30", "Local"), mk("tmdb:40", "Delta"), mk("tmdb:50", "Nothing owned")
	for i := 0; i < 150; i++ { // a crowd of other movies in no collection
		rdpTitle(t, r.st, "movie", fmt.Sprintf("Solo %03d", i), fmt.Sprint(9000+i), "active", rdpStamp)
	}
	m := func(n, state string) int64 { return rdpTitle(t, r.st, "movie", n, "", state, rdpStamp) }
	put(a, m("A1", "active"))
	put(a, m("A2", "active"))
	put(a, m("A3", "removed"))
	put(b, m("B1", "active"))
	put(c, m("C1", "active"))
	put(d, m("D1", "active"))
	put(d, m("D2", "active"))
	put(empty, m("E1", "removed"))
	got, err := discover.ExploreCollections(ctx, r.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	var s string
	for _, x := range got {
		s += fmt.Sprintf("%s:%s:%d ", x.TMDBID, x.Name, x.Owned)
	}
	if s != "10:Alpha:2 40:Delta:2 20:Beta:1 " {
		t.Fatalf("collections %q", s)
	}
}

// libraryJoin looks the item up by kind and TMDB id: a removed title or one
// of the other kind does not count, a never-add entry hides the item, the
// open ask shows with the asker's own flag.
func TestRDPLibraryJoinAndNeverAdd(t *testing.T) {
	r := newAskRig(t)
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		rdpTitle(t, r.st, "movie", fmt.Sprintf("Crowd %03d", i), fmt.Sprint(8000+i), "active", rdpStamp)
	}
	movie := rdpTitle(t, r.st, "movie", "Here", "123", "active", rdpStamp)
	rdpTitle(t, r.st, "show", "Same Number Show", "124", "active", rdpStamp)
	rdpTitle(t, r.st, "movie", "Removed", "125", "removed", rdpStamp)
	never := func(kind, ids string) {
		rdpExec(t, r.st, `INSERT INTO never_add (created_at, created_by, updated_at, updated_by, kind, source_ids, name) VALUES (?, '{}', ?, '{}', ?, ?, 'n')`, rdpStamp, rdpStamp, kind, ids)
	}
	never("movie", `{"tmdb":"126"}`)
	never("show", `{"tmdb":"127"}`)
	never("movie", `{}`) // no TMDB id: never matches anything
	never("artist", `{"tmdb":"128"}`)

	v, hidden, err := r.api.disc.LibraryJoin(ctx, metadata.DiscoverItem{Kind: "movie", TMDBID: "123"}, r.ana.id)
	if err != nil || hidden || !v.InLibrary || v.TitleID != movie || v.Ask != nil || v.Genres == nil {
		t.Fatalf("in library: %+v hidden=%v %v", v, hidden, err)
	}
	// Right number, wrong kind: not in the Library.
	if v, _, _ := r.api.disc.LibraryJoin(ctx, metadata.DiscoverItem{Kind: "movie", TMDBID: "124"}, r.ana.id); v.InLibrary || v.TitleID != 0 {
		t.Fatalf("movie 124 found a show: %+v", v)
	}
	if v, _, _ := r.api.disc.LibraryJoin(ctx, metadata.DiscoverItem{Kind: "show", TMDBID: "124"}, r.ana.id); !v.InLibrary {
		t.Fatalf("show 124 not found: %+v", v)
	}
	// Removed: not in the Library.
	if v, _, _ := r.api.disc.LibraryJoin(ctx, metadata.DiscoverItem{Kind: "movie", TMDBID: "125"}, r.ana.id); v.InLibrary {
		t.Fatalf("removed title found: %+v", v)
	}
	for _, c := range []struct {
		kind, id string
		hide     bool
	}{{"movie", "126", true}, {"show", "126", false}, {"show", "127", true}, {"movie", "128", false}, {"movie", "", false}} {
		if _, h, err := r.api.disc.LibraryJoin(ctx, metadata.DiscoverItem{Kind: c.kind, TMDBID: c.id}, r.ana.id); err != nil || h != c.hide {
			t.Fatalf("never-add %s %s: hidden %v want %v (%v)", c.kind, c.id, h, c.hide, err)
		}
	}
	// dropNeverAdd keeps the order and leaves out movie 126 and show 127 only.
	items := []metadata.DiscoverItem{{Kind: "movie", TMDBID: "126"}, {Kind: "movie", TMDBID: "1"}, {Kind: "show", TMDBID: "127"}, {Kind: "show", TMDBID: "126"}, {Kind: "movie", TMDBID: "128"}}
	kept, err := r.api.disc.DropNeverAdd(ctx, items)
	if err != nil || len(kept) != 3 || kept[0].TMDBID != "1" || kept[1].TMDBID != "126" || kept[2].TMDBID != "128" {
		t.Fatalf("dropNeverAdd %+v %v", kept, err)
	}
	// An open ask: ana asked, so Mine for ana and not for bo.
	if _, err := r.asks.Ask(ctx, requests.Person{UserID: r.ana.id, Name: "ana"}, "693134"); err != nil {
		t.Fatal(err)
	}
	for who, want := range map[int64]bool{r.ana.id: true, r.bo.id: false} {
		v, _, err := r.api.disc.LibraryJoin(ctx, metadata.DiscoverItem{Kind: "movie", TMDBID: "693134"}, who)
		if err != nil || v.Ask == nil || v.Ask.State != "asked" || v.Ask.Mine != want || v.Ask.People != 1 {
			t.Fatalf("open ask for %d: %+v %v", who, v.Ask, err)
		}
	}
}

// exploreInput reads the Library (titles with a TMDB id, not removed) and
// the asks (not turned down) into the Explore signals.
func TestRDPExploreInputReadsLibraryAndAsks(t *testing.T) {
	r := newAskRig(t)
	ctx := context.Background()
	for i := 0; i < 250; i++ {
		rdpTitle(t, r.st, "movie", fmt.Sprintf("Crowd %03d", i), fmt.Sprint(8000+i), "active", rdpStamp)
	}
	rdpTitle(t, r.st, "show", "No Id", "", "active", rdpStamp)
	rdpTitle(t, r.st, "show", "Gone", "7777", "removed", rdpStamp)
	if _, err := r.asks.Ask(ctx, requests.Person{UserID: r.ana.id, Name: "ana"}, "693134"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.asks.Ask(ctx, requests.Person{UserID: r.bo.id, Name: "bo"}, "900002"); err != nil {
		t.Fatal(err)
	}
	m, err := r.api.explore.LoadMember(ctx, r.ana.id)
	if err != nil || m.Kid {
		t.Fatalf("member %+v %v", m, err)
	}
	in, err := r.api.explore.Input(ctx, m, "", rdpNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Owned) != 250 || in.Owned["show:7777"] || !in.Owned["movie:8000"] {
		t.Fatalf("owned %d", len(in.Owned))
	}
	if len(in.MyAsks) != 1 || in.MyAsks[0].TMDBID != "693134" || in.MyAsks[0].Kind != "movie" {
		t.Fatalf("my asks %+v", in.MyAsks)
	}
	// A turned-down ask drops out of the signals.
	rdpExec(t, r.st, "UPDATE asks SET state = 'turned_down' WHERE tmdb_id = '693134'")
	in, err = r.api.explore.Input(ctx, m, "", rdpNow)
	if err != nil || len(in.MyAsks) != 0 {
		t.Fatalf("after turn down: %+v %v", in.MyAsks, err)
	}
	if _, err := r.api.explore.LoadMember(ctx, 99999); err == nil {
		t.Fatal("unknown member loaded")
	}
}

// stripTasteRefs weighs a member's own asks 2.0 (fading), follows 1.0 and,
// for anyone but a Kid, the newest 200 Library titles 0.25.
func TestRDPStripTasteRefsWeights(t *testing.T) {
	r := newAskRig(t)
	ctx := context.Background()
	for i := 0; i < 260; i++ {
		rdpTitle(t, r.st, "movie", fmt.Sprintf("Crowd %03d", i), fmt.Sprint(8000+i), "active", fmt.Sprintf("2026-01-01T00:%02d:%02dZ", i/60, i%60))
	}
	f1 := rdpTitle(t, r.st, "show", "Followed", "4242", "active", rdpStamp)
	rdpTitle(t, r.st, "show", "FollowedNoId", "", "active", rdpStamp)
	noID := rdpTitle(t, r.st, "movie", "NoIdFollowed", "", "active", rdpStamp)
	rdpExec(t, r.st, "INSERT INTO follows (user_id, title_id, created_at) VALUES (?, ?, ?), (?, ?, ?)", r.ana.id, f1, rdpStamp, r.ana.id, noID, rdpStamp)
	rdpExec(t, r.st, "INSERT INTO tmdb_follows (user_id, kind, tmdb_id, created_at) VALUES (?, 'show', '5151', ?)", r.ana.id, rdpStamp)
	rdpExec(t, r.st, "INSERT INTO tmdb_movie_follows (user_id, tmdb_id, created_at) VALUES (?, '5252', ?)", r.ana.id, rdpStamp)
	if _, err := r.asks.Ask(ctx, requests.Person{UserID: r.ana.id, Name: "ana"}, "693134"); err != nil {
		t.Fatal(err)
	}
	count := func(kid bool) (ask, follow, lib int) {
		weights, err := r.api.disc.StripTasteWeights(ctx, r.ana.id, kid, rdpNow)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range weights {
			switch {
			case w > 1.5:
				ask++
			case w == 1:
				follow++
			case w == 0.25:
				lib++
			default:
				t.Fatalf("odd weight %v", w)
			}
		}
		return
	}
	// asks: 1; follows: the followed show, the two TMDB follows (a follow of
	// a title with no TMDB id is skipped); Library: newest 200 of 261 with ids.
	if a, f, l := count(false); a != 1 || f != 3 || l != 200 {
		t.Fatalf("adult: asks %d follows %d library %d", a, f, l)
	}
	if a, f, l := count(true); a != 1 || f != 3 || l != 0 {
		t.Fatalf("kid: asks %d follows %d library %d", a, f, l)
	}
}

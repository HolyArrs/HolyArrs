package sources

import (
	"context"
	"database/sql"
	"net/url"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// searchedAt is when the Item of a Slot was last searched for (nil for never).
func searchedAt(t *testing.T, f fixture, slotID int64) *string {
	t.Helper()
	var at *string
	if err := f.s.Read().QueryRow("SELECT i.last_searched_at FROM items i JOIN slots s ON s.item_id = i.id WHERE s.id = ?", slotID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

// A search that asked nobody because its folder is kept apart still counts as
// the title's turn: the nightly search moves on to the titles after it.
func TestSearchMissing_KeptApartTitleTakesItsTurn(t *testing.T) {
	general := &liveFeed{}
	general.set(feedOf())
	f := seed(t, general.url(t))
	quiet := &liveFeed{}
	quiet.set(feedOf())
	addTaggedSource(t, f, "Anime", quiet.url(t), []string{"anime"}, func(src *domain.Source) { src.UseFor.AutoSearch = false })
	ctx := context.Background()
	// A second movie, in a plain folder.
	folder := plainFolder(t, f)
	var sl mediafiles.Slot
	var ver mediafiles.Version
	var it mediafiles.Item
	var ti mediafiles.Title
	for _, get := range []func() error{
		func() error { return database.Get(ctx, f.s.Read(), "slots", f.slotID, &sl) },
		func() error { return database.Get(ctx, f.s.Read(), "versions", sl.VersionID, &ver) },
		func() error { return database.Get(ctx, f.s.Read(), "items", sl.ItemID, &it) },
		func() error { return database.Get(ctx, f.s.Read(), "titles", it.TitleID, &ti) },
	} {
		if err := get(); err != nil {
			t.Fatal(err)
		}
	}
	var other int64
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		ti.ID, ti.Name, ti.SortName, ti.SourceIDs = 0, "Other Movie", "other movie", map[string]string{"tmdb": "999001"}
		if _, err := database.Insert(tx, "titles", &ti, domain.System); err != nil {
			return err
		}
		it.ID, it.TitleID = 0, ti.ID
		if _, err := database.Insert(tx, "items", &it, domain.System); err != nil {
			return err
		}
		ver.ID, ver.TitleID, ver.LibraryFolderID, ver.FolderName = 0, ti.ID, folder, "Other Movie"
		if _, err := database.Insert(tx, "versions", &ver, domain.System); err != nil {
			return err
		}
		sl.ID, sl.ItemID, sl.VersionID = 0, it.ID, ver.ID
		_, err := database.Insert(tx, "slots", &sl, domain.System)
		other = sl.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1 WHERE label != 'Plain'`)
	exec(t, f, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('what_to_pick', 'bulk_search_cap', '1', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`)
	// One turn of one search: which Slot it picked.
	turn := func() int64 {
		t.Helper()
		exec(t, f, "DELETE FROM jobs WHERE kind = 'search_slot'")
		runKind(t, f, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, map[string]any{"which": "missing"}, func(q *jobs.Queue) {
			q.Handle(jobs.KindSearchSlot, func(context.Context, *jobs.Run) error { return nil })
		})
		js, _ := database.Select[domain.Job](ctx, f.s.Read(), "jobs", "kind = 'search_slot' ORDER BY id")
		if len(js) != 1 || js[0].Subject == nil {
			t.Fatalf("%d searches queued", len(js))
		}
		return js[0].Subject.ID
	}
	if got := turn(); got != f.slotID {
		t.Fatalf("the first turn picked Slot %d, want the kept-apart one %d", got, f.slotID)
	}
	exec(t, f, "DELETE FROM jobs")
	runSearch(t, f, &Searcher{Store: f.s})
	if searchedAt(t, f, f.slotID) == nil {
		t.Fatal("the kept-apart search left its Item as never searched")
	}
	if got := turn(); got != other {
		t.Fatalf("the second turn picked Slot %d again, want the plain title's %d", got, other)
	}
}

// The season and the album search that ask nobody also count as searched.
func TestKeptApart_SeasonAndAlbumCountAsSearched(t *testing.T) {
	gen := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, gen.start(t), nil)
	an := &tvServer{answer: func(url.Values) []string { return nil }}
	addTaggedSource(t, f.fixture, "Anime", an.start(t), []string{"anime"}, func(src *domain.Source) { src.UseFor.AutoSearch = false })
	exec(t, f.fixture, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if j.State != "done" || len(gen.queries) != 0 || searchedAt(t, f.fixture, f.slots["S01E01"]) == nil || searchedAt(t, f.fixture, f.slots["S01E03"]) == nil {
		t.Fatalf("season: job %s, general asked %d, searched %v %v", j.State, len(gen.queries), searchedAt(t, f.fixture, f.slots["S01E01"]), searchedAt(t, f.fixture, f.slots["S01E03"]))
	}
	mg := &musicServer{answer: func(url.Values) []string { return nil }}
	m := seedArtist(t, mg.start(t))
	ma := &musicServer{answer: func(url.Values) []string { return nil }}
	addTaggedSource(t, m.fixture, "Anime", ma.start(t), []string{"anime"}, func(src *domain.Source) { src.State = "paused" })
	exec(t, m.fixture, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	aj := m.albumJob(t, &Searcher{Store: m.s}, lta, nil)
	if aj.State != "done" || len(mg.queries) != 0 || searchedAt(t, m.fixture, m.slots[ltaTracks[0]]) == nil {
		t.Fatalf("album: job %s, general asked %d, searched %v", aj.State, len(mg.queries), searchedAt(t, m.fixture, m.slots[ltaTracks[0]]))
	}
}

// A Source the copy takes through its Pick rules' tags is asked, also while
// the folder's own tagged Source cannot be searched: the search goes on.
func TestHandle_FolderKeptApartStillAsksThePickRulesSource(t *testing.T) {
	general := &liveFeed{}
	general.set(duneFeed)
	f := seed(t, general.url(t))
	anime := &liveFeed{}
	anime.set(duneFeed)
	addTaggedSource(t, f, "Anime", anime.url(t), []string{"anime"}, func(src *domain.Source) { src.UseFor.AutoSearch = false })
	hd := &liveFeed{}
	hd.set(duneFeed)
	addTaggedSource(t, f, "HD", hd.url(t), []string{"hd"}, nil)
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('["hd"]'))`)
	runSearch(t, f, &Searcher{Store: f.s})
	if feedHits(hd) == 0 || feedHits(general) != 0 || feedHits(anime) != 0 {
		t.Fatalf("HD asked %d, general asked %d, anime asked %d", feedHits(hd), feedHits(general), feedHits(anime))
	}
}

// The line gives each Source its own cause and names the folder's tags only.
func TestKeptApartLine(t *testing.T) {
	mk := func(name string, tags []string, change func(*domain.Source)) domain.Source {
		s := domain.Source{Name: name, Tags: tags, Format: "torznab", State: "enabled", Kinds: []string{"movie"}}
		s.UseFor.AutoSearch, s.UseFor.HandSearch = true, true
		if change != nil {
			change(&s)
		}
		return s
	}
	srcs := []domain.Source{
		mk("A", []string{"anime"}, func(s *domain.Source) { s.Format = "rss" }),
		mk("B", []string{"anime"}, func(s *domain.Source) { s.State = "paused" }),
		mk("C", []string{"anime", "subs"}, func(s *domain.Source) { s.UseFor.AutoSearch = false }),
		mk("Kids", []string{"kids"}, func(s *domain.Source) { s.UseFor.AutoSearch = false }),
		mk("Plain", nil, nil),
	}
	cp := copyOf([]string{"kids"}, []string{"subs", "anime"}, kindTags(srcs, "movie"))
	want := "This folder only takes Sources tagged anime, subs. None can be searched right now (A only has a feed; B is paused; C has search turned off), so nothing was asked."
	if got := cp.keptApartLine(srcs, "movie", false, time.Now()); got != want {
		t.Fatalf("line %q", got)
	}
	// By hand C can be searched: the line is not for this search.
	if got := cp.keptApartLine(srcs, "movie", true, time.Now()); got != "" {
		t.Fatalf("by hand: %q", got)
	}
}

// One Source with the folder's tag can be searched but the Pick rules bar it,
// another is paused: the search is not called kept apart by the paused one
// alone, it says the Pick rules stand in the way.
func TestHandle_FolderTaggedSourceBarredByPickRules(t *testing.T) {
	general := &liveFeed{}
	general.set(duneFeed)
	f := seed(t, general.url(t))
	a := &liveFeed{}
	a.set(duneFeed)
	addTaggedSource(t, f, "A", a.url(t), []string{"anime"}, nil)
	b := &liveFeed{}
	b.set(duneFeed)
	addTaggedSource(t, f, "B", b.url(t), []string{"anime"}, func(src *domain.Source) {
		until := time.Now().Add(time.Hour)
		src.State, src.PausedUntil = "paused", &until
	})
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('["hd"]'))`)
	runSearch(t, f, &Searcher{Store: f.s})
	sl := slotOf(t, f, f.slotID)
	want := "no Source takes this title's tags: A: its tags (anime) are not in the Pick rules' tags (hd); B: its tags (anime) are not in the Pick rules' tags (hd)"
	if feedHits(general)+feedHits(a)+feedHits(b) != 0 || sl.LastReason == nil || *sl.LastReason != want {
		t.Fatalf("asked %d %d %d, reason %v", feedHits(general), feedHits(a), feedHits(b), sl.LastReason)
	}
}

// A Source with the folder's tag that a person paused still keeps the folder
// apart: the search asks nobody and says who paused it, and the feed does not
// hand the title a general Source's Release.
func TestKeptApart_TurnedOffSourceStillKeepsTheFolderApart(t *testing.T) {
	general := &liveFeed{}
	general.set(feedOf(item(web1080, time.Now().Add(-2*time.Hour), 50, "")))
	f := seed(t, general.url(t))
	feedOn(t, f)
	anime := &liveFeed{}
	anime.set(feedOf())
	addTaggedSource(t, f, "Anime", anime.url(t), []string{"anime"}, func(src *domain.Source) { src.State = "disabled" })
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anime"]', only_tagged_sources = 1`)
	runSearch(t, f, &Searcher{Store: f.s})
	sl := slotOf(t, f, f.slotID)
	want := "This folder only takes Sources tagged anime. None can be searched right now (Anime is paused by you), so nothing was asked."
	if feedHits(general) != 0 || feedHits(anime) != 0 || sl.LastReason == nil || *sl.LastReason != want {
		t.Fatalf("search: general asked %d, anime asked %d, reason %v", feedHits(general), feedHits(anime), sl.LastReason)
	}
	exec(t, f, `UPDATE slots SET state = 'wanted'`)
	runKind(t, f, &Searcher{Store: f.s}, jobs.KindFeedCheck, nil, nil)
	if rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ?", web1080); len(rs) != 0 {
		t.Fatalf("feed: the general Source's Release reached the title %d times", len(rs))
	}
}

// Whether untagged Sources stay out is decided by the folder's tags alone: a
// folder tag no Source has falls back and says so, even when the title has a
// tag of its own that a Source shares. That Source is asked as it always was.
func TestHandle_FolderTagNoSourceHasFallsBackBesideATitleTag(t *testing.T) {
	general := &liveFeed{}
	general.set(duneFeed)
	f := seed(t, general.url(t))
	uhd := &liveFeed{}
	uhd.set(duneFeed)
	addTaggedSource(t, f, "UHD", uhd.url(t), []string{"4k"}, nil)
	exec(t, f, `UPDATE titles SET tags = '["4k"]'`)
	exec(t, f, `UPDATE library_folders SET new_title_tags = '["anmie"]', only_tagged_sources = 1`)
	runSearch(t, f, &Searcher{Store: f.s})
	evs, _, err := f.s.Events(context.Background(), database.EventFilter{Limit: 1, Kinds: []string{EventSearchDone, EventSearchFailed, EventReleaseChosen, EventReleaseHeld}})
	if err != nil || len(evs) == 0 {
		t.Fatalf("no search Event: %v", err)
	}
	if note, _ := evs[0].Data["sources_note"].(string); feedHits(general) == 0 || feedHits(uhd) == 0 || note != tagsPassedOver {
		t.Fatalf("general asked %d, 4k Source asked %d, note %q", feedHits(general), feedHits(uhd), note)
	}
}

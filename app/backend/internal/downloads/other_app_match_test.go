package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// addUsenet adds a SABnzbd Downloader with the given category and returns
// its id.
func addUsenet(t *testing.T, s *database.Store, name, cat, shows string) int64 {
	t.Helper()
	d := &domain.Downloader{Writable: true, Name: name, Adapter: "sabnzbd", Protocol: "usenet", URL: "http://sab-" + name, Category: cat, CategoryShows: shows,
		Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "downloaders", d, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return d.ID
}

// A shared SABnzbd: jobs in other apps' categories are never matched and
// never offered in Confirm match, as Sonarr and Radarr leave other
// categories alone. Own categories, torrent clients and a Usenet client
// with no category of its own match as before.
func TestUsenetOtherCategoriesNotMatched(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	ctx := context.Background()
	sab := addUsenet(t, x.s, "S", "holyarrs", "holyarrs-tv")
	open := addUsenet(t, x.s, "O", "", "")
	x.dl.Match.SearchMovies = func(context.Context, string, int) ([]metadata.Movie, error) { return nil, nil }
	x.dl.Match.SearchShows = func(context.Context, string) ([]metadata.Show, error) { return nil, nil }

	out, _, err := x.dl.MatchLinkOnly(ctx, sab, "SABnzbd_nzo_sonarr1", "QI.XL.S24E02.720p.WEB", "tv")
	if err != nil || out != matchOtherApp {
		t.Fatalf("other app's job: %q %v", out, err)
	}
	var n int
	_ = x.s.Read().QueryRow("SELECT COUNT(*) FROM client_links WHERE downloader_id = ?", sab).Scan(&n)
	if n != 0 {
		t.Fatalf("other app's job saved %d links", n)
	}
	for _, c := range []struct {
		id  int64
		cat string
	}{{sab, "holyarrs"}, {sab, "HolyArrs-TV"}, {open, "tv"}, {1, "tv"}} {
		if out, _, err := x.dl.MatchLinkOnly(ctx, c.id, "k-"+c.cat, "QI.XL.S24E02.720p.WEB", c.cat); err != nil || out != LinkUnsure {
			t.Fatalf("downloader %d category %q: %q %v", c.id, c.cat, out, err)
		}
	}

	// Rows already waiting: the other app's one is left out of the list.
	exec41(t, x.s, `DELETE FROM client_links`)
	for _, r := range []struct {
		id       int64
		key, cat string
	}{{sab, "sabnzbd_nzo_a", "tv"}, {sab, "sabnzbd_nzo_b", "holyarrs"}, {1, "eeee", "tv"}} {
		exec41(t, x.s, `INSERT INTO client_links (downloader_id, hash, name, state, slot_ids, covers, candidates, how, matched_at) VALUES (?, ?, ?, 'unsure', '[]', '', '[]', 'unsure', '')`, r.id, r.key, r.key)
		exec41(t, x.s, `INSERT INTO dl_match_queue (downloader_id, key, name, category, priority, next_try_at, outcome) VALUES (?, ?, ?, ?, 0, '', 'unsure')`, r.id, strings.ToUpper(r.key), r.key, r.cat)
	}
	var got struct {
		Matches []MatchView    `json:"matches"`
		Counts  map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/matches", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, m := range got.Matches {
		names[m.Name] = true
	}
	if len(got.Matches) != 2 || !names["sabnzbd_nzo_b"] || !names["eeee"] || got.Counts[LinkUnsure] != 2 {
		t.Fatalf("matches %v counts %v", names, got.Counts)
	}

	// Match now looks at them again (the categories may have changed).
	exec41(t, x.s, `INSERT INTO dl_match_queue (downloader_id, key, name, category, priority, next_try_at, outcome) VALUES (?, 'x', 'x', 'tv', 0, '', ?)`, sab, matchOtherApp)
	if n := x.dl.Rematch(ctx); n != 1 {
		t.Fatalf("rematch %d", n)
	}
}

func TestOtherApp(t *testing.T) {
	t.Parallel()
	sab := domain.Downloader{Protocol: "usenet", Category: "holyarrs", CategoryMovies: "holyarrs-movies"}
	for _, c := range []struct {
		d    domain.Downloader
		cat  string
		want bool
	}{
		{sab, "holyarrs", false},
		{sab, "HOLYARRS-MOVIES", false},
		{sab, "tv", true},
		{sab, "", true},
		{domain.Downloader{Protocol: "usenet"}, "tv", false},
		{domain.Downloader{Protocol: "usenet", Category: "*"}, "tv", false},
		{domain.Downloader{Protocol: "torrent", Category: "holyarrs"}, "tv", false},
	} {
		if got := otherApp(c.d, c.cat); got != c.want {
			t.Errorf("%+v %q: %v", c.d, c.cat, got)
		}
	}
}

// A job moved into HolyArrs's own category in a shared SABnzbd is matched
// on the next look, as Sonarr picks it up on its next poll.
func TestUsenetJobMovedIntoOwnCategoryIsMatched(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	ctx := context.Background()
	sab := addUsenet(t, x.s, "S", "holyarrs", "")
	var lookups int
	x.dl.Match.SearchMovies = func(context.Context, string, int) ([]metadata.Movie, error) { lookups++; return nil, nil }
	x.dl.Match.SearchShows = func(context.Context, string) ([]metadata.Show, error) { lookups++; return nil, nil }
	p := &persister{db: x.s, now: time.Now}
	s := &Sync{Store: &Store{}, DB: x.s, Match: x.dl.MatchLinkOnly}
	s.init()
	job := Job{ID: "SABnzbd_nzo_m", Name: "QI.XL.S24E02.720p.WEB", Category: "tv"}
	p.note(sab, Changes{Added: []string{job.ID}}, nil, map[string]Job{job.ID: job}, "")
	if err := p.flush(ctx); err != nil {
		t.Fatal(err)
	}
	s.matchStep(ctx)
	outcome := func() (out, cat string) {
		_ = x.s.Read().QueryRow("SELECT outcome, category FROM dl_match_queue WHERE downloader_id = ? AND key = ?", sab, job.ID).Scan(&out, &cat)
		return
	}
	if out, _ := outcome(); out != matchOtherApp || lookups != 0 {
		t.Fatalf("job in tv: outcome %q lookups %d", out, lookups)
	}

	moved := job
	moved.Category = "holyarrs"
	p.note(sab, Changes{Changed: []FieldChange{{ID: job.ID, Fields: []string{"Category"}}}}, map[string]Job{job.ID: job}, map[string]Job{job.ID: moved}, "")
	if err := p.flush(ctx); err != nil {
		t.Fatal(err)
	}
	s.matchStep(ctx)
	if out, cat := outcome(); out == matchOtherApp || cat != "holyarrs" || lookups == 0 {
		t.Fatalf("job now in own category: outcome %q queue category %q lookups %d", out, cat, lookups)
	}
}

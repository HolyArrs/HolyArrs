package requests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// labShows answers show lookups by any id of a made-up show.
type labShows map[string]metadata.Show

func (f labShows) SearchShows(context.Context, string) ([]metadata.Show, error) { return nil, nil }

// day2 parses a YYYY-MM-DD date as the end of that day.
func day2(s string) *time.Time {
	t := day(s).Add(24*time.Hour - time.Second)
	return &t
}

func (f labShows) Show(_ context.Context, id string) (metadata.Show, error) {
	if sh, ok := f[id]; ok {
		return sh, nil
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

// newShow makes a lab show with two aired seasons and one still to come.
func newShow(name, tmdb, tvdb, imdb string) metadata.Show {
	ids := map[string]string{"tmdb": tmdb, "tvdb": tvdb, "imdb": imdb}
	ep := func(season, n int, day string) metadata.Episode {
		return metadata.Episode{Season: season, Number: n, Name: name, AirDate: day2(day), AirDay: day}
	}
	return metadata.Show{Source: "tmdb", Name: name, Year: 2024, Status: "running", IDs: ids,
		Seasons:  []metadata.Season{{Number: 1}, {Number: 2}, {Number: 3}},
		Episodes: []metadata.Episode{ep(1, 1, "2024-01-07"), ep(1, 2, "2024-01-14"), ep(2, 1, "2025-02-02"), ep(3, 1, "2099-01-04")}}
}

// fakeShowLists serves TMDB list 61 (two shows, one movie, one show on the
// Never-add list) and the Trakt list kim/shows and kim's show watchlist.
func fakeShowLists(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/list/61":
			if r.Header.Get("Authorization") != "Bearer "+fakeTMDBToken {
				w.WriteHeader(401)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 61, "total_pages": 1, "items": []map[string]any{
				{"id": 700001, "media_type": "tv", "name": "Ferry Lights"},
				{"id": 700002, "media_type": "tv", "name": "Gull Watch"},
				{"id": 900002, "media_type": "movie", "title": "Paper Harbour"},
				{"id": 700003, "media_type": "tv", "name": "Not Here"},
			}})
		case "/users/kim/lists/shows/items/show", "/users/kim/watchlist/shows":
			if r.Header.Get("trakt-api-key") != fakeTraktClient {
				w.WriteHeader(403)
				return
			}
			w.Header().Set("X-Pagination-Page-Count", "1")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"type": "show", "show": map[string]any{"title": "Ferry Lights", "ids": map[string]any{"tmdb": 700001, "tvdb": 710001}}},
				{"type": "show", "show": map[string]any{"title": "Reed Signal", "ids": map[string]any{"tvdb": 710004}}},
				{"type": "show", "show": map[string]any{"title": "No Ids", "ids": map[string]any{}}},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// showEnv is setup plus show details, a show Library folder and the show
// list stand-ins.
func showEnv(t *testing.T) (*env, mediafiles.LibraryFolder) {
	t.Helper()
	e := setup(t)
	src := labShows{}
	for _, sh := range []metadata.Show{
		newShow("Ferry Lights", "700001", "710001", "tt7000010"),
		newShow("Gull Watch", "700002", "710002", "tt7000020"),
		newShow("Not Here", "700003", "710003", "tt7000030"),
		newShow("Reed Signal", "700004", "710004", "tt7000040"),
		lateStart(),
		onlySpecials(),
	} {
		for k, v := range sh.IDs {
			src[k+":"+v] = sh
		}
	}
	e.svc.Library.Shows = metadata.ShowSources{"tmdb": src}
	srv := fakeShowLists(t)
	e.svc.TMDBBase, e.svc.TraktBase = srv.URL, srv.URL
	lf, err := library.AddFolder(context.Background(), e.s, library.FolderInput{Path: t.TempDir(), Kind: "show", Label: "Shows"}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	return e, lf
}

// lateStart is a show with Specials and seasons 2 and 3, but no season 1.
func lateStart() metadata.Show {
	sh := newShow("Late Start", "700005", "710005", "tt7000050")
	sh.Seasons = []metadata.Season{{Number: 0}, {Number: 2}, {Number: 3}}
	sh.Episodes = []metadata.Episode{{Season: 0, Number: 1, Name: "Late Start", AirDate: day2("2023-12-01"), AirDay: "2023-12-01"},
		{Season: 2, Number: 1, Name: "Late Start", AirDate: day2("2024-01-07"), AirDay: "2024-01-07"},
		{Season: 3, Number: 1, Name: "Late Start", AirDate: day2("2025-01-05"), AirDay: "2025-01-05"}}
	return sh
}

// onlySpecials is a show with nothing but Specials (season 0).
func onlySpecials() metadata.Show {
	sh := newShow("Only Extras", "700006", "710006", "tt7000060")
	sh.Seasons = []metadata.Season{{Number: 0}}
	sh.Episodes = []metadata.Episode{{Season: 0, Number: 1, Name: "Only Extras", AirDate: day2("2024-02-04"), AirDay: "2024-02-04"}}
	return sh
}

func TestFirstSeason_SpecialsCountOnlyWhenAlone(t *testing.T) {
	e, lf := showEnv(t)
	id, _, err := shows.New(e.svc.Library).AddShow(context.Background(), "tmdb:700006", shows.ShowAddOptions{LibraryFolderID: lf.ID, FirstSeason: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := keptSeasons(t, e.s, id); !got[0] {
		t.Fatalf("kept %+v, want Specials", got)
	}
}

func TestFirstSeason_NoSeasonOneUsesEarliestNumbered(t *testing.T) {
	e, lf := showEnv(t)
	id, _, err := shows.New(e.svc.Library).AddShow(context.Background(), "tmdb:700005", shows.ShowAddOptions{LibraryFolderID: lf.ID, FirstSeason: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := keptSeasons(t, e.s, id); got[0] || !got[2] || got[3] {
		t.Fatalf("kept %+v, want only season 2", got)
	}
}

// keptSeasons lists a show's seasons kept up to date.
func keptSeasons(t *testing.T, s *database.Store, titleID int64) map[int]bool {
	t.Helper()
	ss, err := shows.Seasons(context.Background(), s.Read(), titleID, now)
	if err != nil {
		t.Fatal(err)
	}
	out := map[int]bool{}
	for _, x := range ss {
		out[x.Number] = x.KeepUpToDate
	}
	return out
}

func TestAdd_ShowInputChecks(t *testing.T) {
	e, lf := showEnv(t)
	ctx := context.Background()
	bad := []OutsideListInput{
		{Name: "x", Kind: ListTMDB, Ref: "61", TitleKind: "artist"},
		{Name: "x", Kind: ListTMDB, Ref: "61", TitleKind: "show", Seasons: "pilot"},
		{Name: "x", Kind: ListTMDB, Ref: "61", TitleKind: "show", NewSeasons: "some"},
		{Name: "x", Kind: ListTMDB, Ref: "61", TitleKind: "show", LibraryFolderID: e.lf.ID}, // a movie folder
		{Name: "x", Kind: ListTMDB, Ref: "61", LibraryFolderID: lf.ID},                      // a show folder for movies
	}
	for _, in := range bad {
		if _, err := AddOutsideList(ctx, e.s, in, domain.System); err == nil {
			t.Errorf("accepted %+v", in)
		}
	}
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Shows", Kind: ListTMDB, Ref: "61", TitleKind: "show", ReleaseGate: library.GateReleased}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if l.TitleKind != "show" || l.LibraryFolderID != lf.ID || l.Seasons != library.SeasonsAll || l.NewSeasons != "all" || l.ReleaseGate != "" {
		t.Fatalf("show defaults %+v", l)
	}
	m, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Movies", Kind: ListTMDB, Ref: "61", Seasons: "first"}, domain.System)
	if err != nil || m.TitleKind != "movie" || m.Seasons != "" || m.LibraryFolderID != e.lf.ID {
		t.Fatalf("movie list %+v %v", m, err)
	}
	// Turning a list from movies to shows clears its last sync.
	if _, err := e.svc.Sync(ctx, m.ID, domain.System); err != nil {
		t.Fatal(err)
	}
	m, err = EditOutsideList(ctx, e.s, m.ID, OutsideListInput{Name: "Movies", Kind: ListTMDB, Ref: "61", TitleKind: "show", Seasons: "latest"}, domain.System)
	if err != nil || m.LastSyncedAt != nil || m.Seasons != library.SeasonsLatest || m.LibraryFolderID != lf.ID {
		t.Fatalf("edit to shows %+v %v", m, err)
	}
	// An edit without title_kind keeps a show list a show list.
	m, err = EditOutsideList(ctx, e.s, m.ID, OutsideListInput{Name: "Movies", Kind: ListTMDB, Ref: "61", Seasons: library.SeasonsFuture, NewSeasons: "none"}, domain.System)
	if err != nil || m.TitleKind != "show" || m.Seasons != library.SeasonsFuture || m.NewSeasons != "all" {
		t.Fatalf("edit without title_kind %+v %v", m, err)
	}
	// Pick rules must be made for what the list adds.
	movieOnly, err := library.CreatePickRules(ctx, e.s, library.PickRulesInput{Name: "Movies only", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	showOnly, _ := library.CreatePickRules(ctx, e.s, library.PickRulesInput{Name: "Shows only", Kinds: []string{"show"}, Body: mediafiles.GoodEveryday()}, domain.System)
	if _, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Wrong rules", Kind: ListTMDB, Ref: "61", TitleKind: "show", PickRulesID: movieOnly.ID}, domain.System); !errors.Is(err, ErrRulesKind) {
		t.Fatalf("movie rules on a show list: %v", err)
	}
	if _, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Wrong rules", Kind: ListTMDB, Ref: "61", PickRulesID: showOnly.ID}, domain.System); !errors.Is(err, ErrRulesKind) {
		t.Fatalf("show rules on a movie list: %v", err)
	}
	if _, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Right rules", Kind: ListTMDB, Ref: "61", TitleKind: "show", PickRulesID: showOnly.ID}, domain.System); err != nil {
		t.Fatalf("show rules on a show list: %v", err)
	}
	// No show folder yet: a show list cannot be saved.
	e2 := setup(t)
	if _, err := AddOutsideList(ctx, e2.s, OutsideListInput{Name: "Shows", Kind: ListTMDB, Ref: "61", TitleKind: "show"}, domain.System); !errors.Is(err, library.ErrNoShowFolder) {
		t.Fatalf("no show folder: %v", err)
	}
}

func TestSync_TMDBShowListUsesItsSeasonChoice(t *testing.T) {
	e, lf := showEnv(t)
	ctx := context.Background()
	if _, err := library.AddNeverAdd(ctx, e.s, mediafiles.NeverAdd{Kind: "show", SourceIDs: map[string]string{"tmdb": "700003"}, Name: "Not Here"}, domain.System); err != nil {
		t.Fatal(err)
	}
	// Gull Watch is in the Library already, added by hand.
	if _, _, err := shows.New(e.svc.Library).AddShow(ctx, "tmdb:700002", shows.ShowAddOptions{LibraryFolderID: lf.ID}, domain.System); err != nil {
		t.Fatal(err)
	}
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Shows", Kind: ListTMDB, Ref: "61", TitleKind: "show", Seasons: SeasonsFirst, NewSeasons: "none", SearchOnAdd: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Sync(ctx, l.ID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != 3 || res.Added != 1 || res.Already != 1 || res.NeverAdd != 1 || res.Failed != 0 || len(res.Names) != 1 || res.Names[0] != "Ferry Lights" {
		t.Fatalf("result %+v", res)
	}
	ts := titles(t, e.s)
	ferry := ts["Ferry Lights"]
	if ferry.Kind != "show" || ferry.WhyAdded != "list" || ferry.NewPartsPolicy != "none" || ts["Paper Harbour"].ID != 0 || ts["Not Here"].ID != 0 {
		t.Fatalf("titles %+v", ts)
	}
	if got := keptSeasons(t, e.s, ferry.ID); !got[1] || got[2] || got[3] {
		t.Fatalf("first season only: %+v", got)
	}
	// With search on add, the folder scan carries the whole-show search.
	var params string
	if err := e.s.Read().QueryRow("SELECT params FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') = ?", library.KindScanTitle, ferry.ID).Scan(&params); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(params)) || !strings.Contains(params, "search_show") {
		t.Fatalf("scan params %s", params)
	}
	// Second run: nothing new.
	res, _ = e.svc.Sync(ctx, l.ID, domain.System)
	if res.Added != 0 || res.Already != 2 || res.NeverAdd != 1 {
		t.Fatalf("second %+v", res)
	}
}

func TestSync_TraktShowListAndWatchlist(t *testing.T) {
	e, _ := showEnv(t)
	ctx := context.Background()
	l, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Trakt shows", Kind: ListTrakt, Ref: "kim/shows", TitleKind: "show", Seasons: library.SeasonsLatest}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.svc.Sync(ctx, l.ID, domain.System)
	if err != nil || res.Found != 3 || res.Added != 2 || res.Skipped != 1 {
		t.Fatalf("result %+v %v", res, err) // Reed Signal has only a TVDB id; No Ids has none
	}
	ts := titles(t, e.s)
	reed := ts["Reed Signal"]
	if reed.ID == 0 || reed.WhyAdded != "list" {
		t.Fatalf("titles %+v", ts)
	}
	if got := keptSeasons(t, e.s, reed.ID); got[1] || !got[3] {
		t.Fatalf("latest season only: %+v", got)
	}
	var jobs int
	_ = e.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND params LIKE '%search_show%'", library.KindScanTitle).Scan(&jobs)
	if jobs != 0 {
		t.Fatalf("searched without search on add: %d", jobs)
	}
	w, err := AddOutsideList(ctx, e.s, OutsideListInput{Name: "Show watchlist", Kind: ListTrakt, Ref: "kim/watchlist", TitleKind: "show"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := e.svc.Sync(ctx, w.ID, domain.System); err != nil || res.Already != 2 || res.Added != 0 {
		t.Fatalf("watchlist %+v %v", res, err)
	}
}

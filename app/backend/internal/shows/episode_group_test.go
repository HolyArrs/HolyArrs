package shows

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const trappedCut = "64a0f1c2b7d8e9001c2a3b4c"

// fakeGroups is TMDB's episode orders for Trapped: one cut of season 3
// with six of its eight episodes, the third and fourth swapped.
type fakeGroups struct{ err error }

func (f *fakeGroups) EpisodeGroups(_ context.Context, tmdbID string) ([]metadata.EpisodeGroupInfo, error) {
	if tmdbID != "64485" {
		return []metadata.EpisodeGroupInfo{}, nil
	}
	return []metadata.EpisodeGroupInfo{{ID: trappedCut, Name: "International cut", Type: 6, Seasons: 3, Episodes: 6}}, nil
}

func (f *fakeGroups) EpisodeGroup(_ context.Context, id string) (metadata.EpisodeGroup, error) {
	if f.err != nil {
		return metadata.EpisodeGroup{}, f.err
	}
	if id != trappedCut {
		return metadata.EpisodeGroup{}, metadata.ErrNotFound
	}
	s3 := metadata.GroupSeason{Name: "Season 3", Order: 2}
	for i, n := range []int{0, 1, 3, 2, 4, 5} {
		s3.Episodes = append(s3.Episodes, metadata.GroupEpisode{ID: strconv.Itoa(2000000 + n), Order: i})
	}
	return metadata.EpisodeGroup{ID: id, Name: "International cut", Type: 6,
		Seasons: []metadata.GroupSeason{{Name: "Season 1", Order: 0}, {Name: "Season 2", Order: 1}, s3}}, nil
}

func trappedShow() metadata.Show {
	sh := metadata.Show{Source: "tmdb", Name: "Trapped", Year: 2015, Status: "ended", IDs: map[string]string{"tmdb": "64485"},
		Seasons: []metadata.Season{{Number: 3, Name: "Season 3"}}}
	for i := 0; i < 8; i++ {
		day := time.Date(2019, 12, 1+i, 0, 0, 0, 0, time.UTC)
		sh.Episodes = append(sh.Episodes, metadata.Episode{Season: 3, Number: i + 1, Name: "Episode " + strconv.Itoa(i+1), ID: strconv.Itoa(2000000 + i),
			AirDate: &day, AirDay: day.Format("2006-01-02")})
	}
	return sh
}

type groupedRow struct {
	ID, Slot int64
	Alt      [2]int
}

func groupedRows(t *testing.T, s *database.Store, titleID int64) map[int]groupedRow {
	t.Helper()
	its, err := database.Select[mediafiles.Item](context.Background(), s.Read(), "items", "title_id = ? AND kind = 'episode'", titleID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[int]groupedRow{}
	for _, it := range its {
		r := groupedRow{ID: it.ID}
		if it.Numbers.AltSeason != nil {
			r.Alt = [2]int{*it.Numbers.AltSeason, *it.Numbers.AltEpisode}
		}
		sl, _ := database.Select[mediafiles.Slot](context.Background(), s.Read(), "slots", "item_id = ?", it.ID)
		if len(sl) == 1 {
			r.Slot = sl[0].ID
		}
		out[*it.Numbers.Episode] = r
	}
	return out
}

func season3(t *testing.T, s *database.Store, ti mediafiles.Title) SeasonSummary {
	t.Helper()
	ss, err := Seasons(context.Background(), s.Read(), ti.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range ss {
		if x.Number == 3 {
			return x
		}
	}
	t.Fatalf("no season 3 in %+v", ss)
	return SeasonSummary{}
}

func TestEpisodeGroupRenumbersAndBack(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	src := &fakeShows{}
	sh := trappedShow()
	src.set(sh, "tmdb:64485")
	ti := seedShow(t, s, sh, library.SeasonsAll)
	groups := &fakeGroups{}
	svc := showService(s, metadata.ShowSources{"tmdb": src})
	svc.EpisodeGroups = groups
	before := groupedRows(t, s, ti.ID)
	if got := season3(t, s, ti); got.Episodes != 8 {
		t.Fatalf("TMDB season 3: %+v", got)
	}

	bad := "64a0f1c2b7d8e9001c2a3b4d"
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &bad}, false, domain.System); !errors.Is(err, library.ErrBadEpisodeGroup) {
		t.Fatalf("unknown group: %v", err)
	}
	cut := trappedCut
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &cut}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	after := groupedRows(t, s, ti.ID)
	want := map[int][2]int{1: {3, 1}, 2: {3, 2}, 3: {3, 4}, 4: {3, 3}, 5: {3, 5}, 6: {3, 6}, 7: {}, 8: {}}
	for n, r := range after {
		if r.ID != before[n].ID || r.Slot != before[n].Slot || r.Slot == 0 {
			t.Fatalf("E%d row or Slot changed: %+v, was %+v", n, r, before[n])
		}
		if r.Alt != want[n] {
			t.Fatalf("E%d numbers %v, want %v", n, r.Alt, want[n])
		}
	}
	if got := season3(t, s, ti); got.Episodes != 6 {
		t.Fatalf("season 3 in the cut: %+v", got)
	}
	// The calendar shows the order's numbers.
	from := time.Date(2019, 12, 4, 0, 0, 0, 0, time.UTC)
	es, err := library.Calendar(ctx, s.Read(), from, from.AddDate(0, 0, 1))
	if err != nil || len(es) != 1 || *es[0].Season != 3 || *es[0].Episode != 3 || es[0].EpisodeName != "Episode 4" {
		t.Fatalf("calendar %+v %v", es, err)
	}

	// Release names in either numbering find their episode.
	var refs []mediafiles.EpisodeRef
	for n, r := range after {
		refs = append(refs, mediafiles.EpisodeRef{ItemID: r.ID, Season: 3, Episode: n, AltSeason: r.Alt[0], AltEpisode: r.Alt[1]})
	}
	for name, tmdbEp := range map[string]int{"Trapped.S03E03.1080p.WEB.H264-GRP": 4, "Trapped.S03E04.1080p.WEB.H264-GRP": 3, "Trapped.S03E07.1080p.WEB.H264-GRP": 7} {
		got, why := mediafiles.MatchEpisodes(releasename.Parse(name, releasename.Hints{Show: true}), library.OrderAired, refs)
		if len(got) != 1 || got[0].ItemID != after[tmdbEp].ID {
			t.Fatalf("%s: %+v %s, want TMDB E%d", name, got, why, tmdbEp)
		}
	}

	// A failed read of the order keeps the numbers.
	groups.err = errors.New("TMDB down")
	if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	if got := groupedRows(t, s, ti.ID); got[3].Alt != [2]int{3, 4} {
		t.Fatalf("after a failed read: %+v", got)
	}
	groups.err = nil

	none := ""
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &none}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	for n, r := range groupedRows(t, s, ti.ID) {
		if r.ID != before[n].ID || r.Slot != before[n].Slot || r.Alt != [2]int{} {
			t.Fatalf("back to TMDB E%d: %+v", n, r)
		}
	}
	if got := season3(t, s, ti); got.Episodes != 8 {
		t.Fatalf("season 3 back on TMDB: %+v", got)
	}
}

func TestGroupAbsolutes(t *testing.T) {
	sh := trappedShow()
	g, _ := (&fakeGroups{}).EpisodeGroup(context.Background(), trappedCut)
	g.SetNumbers(sh.Episodes)
	got := groupAbsolutes(sh.Episodes)
	want := map[[2]int]int{{3, 1}: 1, {3, 2}: 2, {3, 4}: 3, {3, 3}: 4, {3, 5}: 5, {3, 6}: 6}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%v = %d, want %d", k, got[k], v)
		}
	}
}

func TestEpisodeGroupsRoute(t *testing.T) {
	s := open(t)
	src := &fakeShows{}
	sh := trappedShow()
	src.set(sh, "tmdb:64485")
	ti := seedShow(t, s, sh, library.SeasonsAll)
	svc := showService(s, metadata.ShowSources{"tmdb": src})
	svc.EpisodeGroups = &fakeGroups{}
	cut := trappedCut
	if _, err := svc.Edit(context.Background(), ti.ID, library.TitleChange{EpisodeGroupID: &cut}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	(&SeasonHandlers{Store: s, Lib: svc.Service, Fail: func(w http.ResponseWriter, err error) { ShowError(w, err, nil) },
		TitleID: func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
			var x mediafiles.Title
			err := database.Get(r.Context(), s.Read(), "titles", ti.ID, &x)
			return x, err == nil
		}}).Routes(m)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/titles/"+strconv.FormatInt(ti.ID, 10)+"/episode-groups", nil))
	want := `{"chosen":"` + trappedCut + `","groups":[{"id":"` + trappedCut + `","name":"International cut","type":6,"seasons":3,"episodes":6}]}`
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}

	// An unknown order is a bad request.
	rec = httptest.NewRecorder()
	ShowError(rec, library.ErrBadEpisodeGroup, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown order answered %d", rec.Code)
	}
}

func TestEpisodeListByOrder(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	src := &fakeShows{}
	sh := trappedShow()
	src.set(sh, "tmdb:64485")
	ti := seedShow(t, s, sh, library.SeasonsAll)
	svc := showService(s, metadata.ShowSources{"tmdb": src})
	svc.EpisodeGroups = &fakeGroups{}
	cut := trappedCut
	if _, err := svc.Edit(ctx, ti.ID, library.TitleChange{EpisodeGroupID: &cut}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RefreshShow(ctx, ti.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	m := http.NewServeMux()
	(&EpisodeHandlers{Store: s, Lib: svc.Service, Fail: func(w http.ResponseWriter, err error) { ShowError(w, err, nil) },
		Trim: func(context.Context, map[int64][]EpisodeSlot) {},
		TitleID: func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
			var x mediafiles.Title
			err := database.Get(r.Context(), s.Read(), "titles", ti.ID, &x)
			return x, err == nil
		}}).Routes(m)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/titles/"+strconv.FormatInt(ti.ID, 10)+"/episodes?season=3", nil))
	var body struct {
		Episodes []EpisodeView `json:"episodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body.Episodes) != 8 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var got []string
	for _, e := range body.Episodes {
		g := key(e.Season, e.Episode)
		if e.TMDBEpisode != nil {
			g += "<" + key(*e.TMDBSeason, *e.TMDBEpisode)
		}
		if e.OutsideOrder {
			g += " out"
		}
		got = append(got, g)
	}
	want := "S03E01 S03E02 S03E03<S03E04 S03E04<S03E03 S03E05 S03E06 S03E07 out S03E08 out"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
}

package requests

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// fakeShows answers one show by any of its ids.
type fakeShows struct{ shows map[string]metadata.Show }

func (f *fakeShows) SearchShows(context.Context, string) ([]metadata.Show, error) { return nil, nil }

func (f *fakeShows) Show(_ context.Context, id string) (metadata.Show, error) {
	if sh, ok := f.shows[id]; ok {
		return sh, nil
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

func at(s string) *time.Time { t, _ := time.Parse(time.RFC3339, s); return &t }

// askShow is a show with specials, an aired first season and a second
// season with one aired episode and one far ahead.
func askShow() metadata.Show {
	return metadata.Show{Source: "tmdb", Name: "Lab Show", Year: 2025, Status: "running",
		IDs:     map[string]string{"tmdb": "100", "tvdb": "200", "imdb": "tt0100"},
		Seasons: []metadata.Season{{Number: 0, Name: "Specials"}, {Number: 1, Name: "Season 1"}, {Number: 2, Name: "Season 2"}},
		Episodes: []metadata.Episode{
			{Season: 0, Number: 1, Name: "Extra", AirDate: at("2025-03-01T12:00:00Z"), AirDay: "2025-03-01"},
			{Season: 1, Number: 1, Name: "Pilot", AirDate: at("2025-01-06T02:00:00Z"), AirDay: "2025-01-05"},
			{Season: 1, Number: 2, Name: "Second", AirDate: at("2025-01-13T02:00:00Z"), AirDay: "2025-01-12"},
			{Season: 2, Number: 1, Name: "Return", AirDate: at("2026-01-05T02:00:00Z"), AirDay: "2026-01-04"},
			{Season: 2, Number: 2, Name: "Far", AirDate: at("2099-01-05T02:00:00Z"), AirDay: "2099-01-04"},
		}}
}

// showRig is the asks rig with a show Library folder and the fake source.
func showRig(t *testing.T) *rig {
	t.Helper()
	r := newRig(t)
	sh := askShow()
	r.svc.Library.Shows = metadata.ShowSources{"tmdb": &fakeShows{shows: map[string]metadata.Show{"tmdb:100": sh, "tvdb:200": sh, "imdb:tt0100": sh}}}
	if _, err := library.AddFolder(context.Background(), r.st, library.FolderInput{Path: t.TempDir(), Kind: "show", Label: "Shows"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	return r
}

type seasonRow struct {
	keep bool
	n    int
}

func seasonKeeps(t *testing.T, st *database.Store, titleID int64) map[int]bool {
	t.Helper()
	rows, err := st.Read().Query("SELECT number, keep_up_to_date FROM parts WHERE title_id = ? AND kind = 'season'", titleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var s seasonRow
		if err := rows.Scan(&s.n, &s.keep); err != nil {
			t.Fatal(err)
		}
		out[s.n] = s.keep
	}
	return out
}

func TestShowAskSeasonsJoinApproveAndAskAgain(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()

	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	a := res.Ask
	if a.Kind != "show" || a.State != domain.AskAsked || a.Name != "Lab Show" || len(a.Seasons) != 1 || a.Seasons[0] != 1 || a.Ref != "tmdb:100" {
		t.Fatalf("ask %+v", a)
	}
	if e := hasEvent(r.events(t), domain.EventAskMade); e == nil || e.Summary != "ana asked for Lab Show (2025), season 1" {
		t.Fatalf("event %+v", e)
	}
	// An unknown season is refused.
	if _, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{7}); !errors.Is(err, ErrNoSuchSeason) {
		t.Fatalf("season 7: %v", err)
	}
	// bo asks for the same season by the TVDB id: he joins ana's ask.
	res, err = r.svc.AskShow(ctx, r.bo, "tvdb:200", []int{1})
	if err != nil || !res.Joined || res.Ask.ID != a.ID {
		t.Fatalf("join %+v %v", res, err)
	}
	// bo also wants season 2 while the ask waits: the ask grows, no new one.
	res, err = r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{2})
	if err != nil || res.Ask.ID != a.ID || len(res.Ask.Seasons) != 2 || !res.Already {
		t.Fatalf("wider %+v %v", res, err)
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM asks").Scan(&n)
	if n != 1 {
		t.Fatalf("%d asks, want 1", n)
	}

	got, err := r.svc.Approve(ctx, a.ID, r.luis, ApproveOptions{Note: "enjoy"})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.AskApproved || got.TitleID == nil {
		t.Fatalf("approved %+v", got)
	}
	keeps := seasonKeeps(t, r.st, *got.TitleID)
	if keeps[0] || !keeps[1] || !keeps[2] {
		t.Fatalf("season switches after approval %v", keeps)
	}
	var why string
	_ = r.st.Read().QueryRow("SELECT why_added FROM titles WHERE id = ?", *got.TitleID).Scan(&why)
	if why != "ask" {
		t.Fatalf("why added %q", why)
	}
	e := hasEvent(r.events(t), domain.EventAskApproved)
	if e == nil || e.Data["approved_by"] != "luis" || !strings.Contains(e.Summary, "seasons 1 and 2") {
		t.Fatalf("approved event %+v", e)
	}
	if asked, _ := e.Data["asked_by"].([]any); len(asked) != 2 {
		t.Fatalf("asked_by %v", e.Data["asked_by"])
	}
	var msgs int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM messages WHERE kind = 'approved'").Scan(&msgs)
	if msgs != 2 {
		t.Fatalf("%d approved messages, want 2", msgs)
	}

	// The whole show is now kept: asking for it says it is in the Library.
	var in *InLibraryError
	if _, err := r.svc.AskShow(ctx, r.ana, "imdb:tt0100", nil); !errors.As(err, &in) || !in.Show || in.TitleID != *got.TitleID {
		t.Fatalf("in library: %v", err)
	}
	// Season 1 asked again while the approved ask is open joins it.
	res, err = r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil || res.Ask.ID != a.ID {
		t.Fatalf("join approved %+v %v", res, err)
	}
}

func TestShowAskForSeasonsNotKeptYet(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	// luis adds the show with only the latest season kept.
	tid, _, err := shows.New(r.svc.Library).AddShow(ctx, "tmdb:100", shows.ShowAddOptions{Seasons: "latest"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// A whole-show ask becomes an ask for the seasons not kept yet.
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ask.Seasons) != 1 || res.Ask.Seasons[0] != 1 {
		t.Fatalf("seasons asked %v", res.Ask.Seasons)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if k := seasonKeeps(t, r.st, tid); !k[1] || !k[2] || k[0] {
		t.Fatalf("season switches %v", k)
	}
}

func TestShowAskReadyWhenEveryAiredEpisodeIsIn(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	setSeason := func(season int, state string) {
		err := r.st.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE slots SET state = ? WHERE item_id IN (SELECT i.id FROM items i JOIN parts p ON p.id = i.part_id
				WHERE p.title_id = ? AND p.number = ?)`, state, *a.TitleID, season)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// One of two aired episodes in: not ready yet.
	err = r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE slots SET state = 'in_library' WHERE item_id = (SELECT i.id FROM items i JOIN parts p ON p.id = i.part_id
			WHERE p.title_id = ? AND p.number = 1 ORDER BY i.id LIMIT 1)`, *a.TitleID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.svc.CheckReady(ctx); err != nil || n != 0 {
		t.Fatalf("ready with half a season: %d %v", n, err)
	}
	setSeason(1, "in_library")
	if n, err := r.svc.CheckReady(ctx); err != nil || n != 1 {
		t.Fatalf("ready: %d %v", n, err)
	}
	r.svc.Wait()
	if m := r.fake.last(); m.Moment != alerts.MomentReady || !strings.Contains(m.Text, "Lab Show (2025), season 1") || len(m.People) != 1 || m.People[0].Name != "ana" {
		t.Fatalf("ready alert %+v", m)
	}
	if e := hasEvent(r.events(t), domain.EventAskAvailable); e == nil || !strings.Contains(e.Summary, "ready for ana") {
		t.Fatalf("ready event %+v", e)
	}
	got, _ := get(ctx, r.st.Read(), a.ID)
	if got.State != domain.AskAvailable {
		t.Fatalf("state %s", got.State)
	}
}

func TestShowAsksCountAgainstTheShowLimit(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	// bo has used his one show ask this week: a new show ask is refused,
	// and the message names the show limit.
	set, _ := ReadSettings(ctx, r.st.Read())
	set.ShowsPerWeek = 1
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	useAsk(t, r, r.bo.UserID, "show")
	var le *LimitError
	if _, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{2}); !errors.As(err, &le) || le.Kind != KindShows ||
		!strings.Contains(err.Error(), "1 show in the last 7 days, which is your show limit") {
		t.Fatalf("show over the show limit: %v", err)
	}
	// His movie limit is untouched.
	if _, err := r.svc.Ask(ctx, r.bo, "900003"); err != nil {
		t.Fatalf("movie at the show limit: %v", err)
	}
	// A whole show is one ask, whatever its seasons; her own show limit
	// wins over the default.
	two := 2
	if err := SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{Shows: &two}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", nil); err != nil {
		t.Fatal(err)
	}
	l, _ := r.svc.LimitOf(ctx, r.ana)
	if l.Shows.PerWeek != 2 || l.Shows.Used != 1 || l.Shows.Left != 1 || l.Movies.Used != 0 || l.Movies.Left != DefaultPerWeek || l.Music.PerWeek != DefaultPerWeek {
		t.Fatalf("limits %+v", l)
	}
	// Joining her waiting ask is free for bo, even at his limit.
	if res, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{2}); err != nil || !res.Joined {
		t.Fatalf("join at the limit: %+v %v", res, err)
	}
	// 0 is no limit for shows.
	set.ShowsPerWeek = 0
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	if l, _ := r.svc.LimitOf(ctx, r.bo); !l.Shows.NoLimit || l.Shows.Used != 1 {
		t.Fatalf("bo limits %+v", l)
	}
}

// useAsk writes one open ask of a kind for a person, as if they had asked
// just now (for a limit already used up).
func useAsk(t *testing.T, r *rig, userID int64, kind string) {
	t.Helper()
	ctx := context.Background()
	err := r.st.Write(ctx, func(tx *sql.Tx) error {
		a := domain.Ask{Kind: kind, TMDBID: "used-" + kind, Ref: "tmdb:used-" + kind, SourceIDs: map[string]string{"tmdb": "used-" + kind}, Seasons: []int{},
			Name: "Used", State: domain.AskAsked, AskedBy: userID, AskedAt: r.clock}
		if kind == "artist" {
			a.SourceIDs = map[string]string{"musicbrainz": "used-artist"}
			a.TMDBID = ""
		}
		if _, err := database.Insert(tx, "asks", &a, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 1)", a.ID, userID, database.FormatTime(r.clock))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutoApproveAddsTheWholeShow(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.luis, "tmdb:100", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Ask.State != domain.AskApproved || !res.Ask.AutoApproved || res.Ask.TitleID == nil {
		t.Fatalf("auto %+v", res.Ask)
	}
	if k := seasonKeeps(t, r.st, *res.Ask.TitleID); k[0] || !k[1] || !k[2] {
		t.Fatalf("whole show seasons %v", k)
	}
}

func TestShowNeverAddAndTurnDown(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.TurnDown(ctx, res.Ask.ID, r.luis, "not this one"); err != nil {
		t.Fatal(err)
	}
	if e := hasEvent(r.events(t), domain.EventAskTurnedDown); e == nil || e.Data["turned_down_by"] != "luis" {
		t.Fatalf("turned down event %+v", e)
	}
	err = r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO never_add (created_at, created_by, updated_at, updated_by, kind, name, source_ids)
			VALUES ('2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', 'show', 'Lab Show', '{"tvdb":"200"}')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", nil); !errors.Is(err, shows.ErrShowNeverAdd) {
		t.Fatalf("never add: %v", err)
	}
}

func TestSeasonWords(t *testing.T) {
	for _, c := range []struct {
		in   []int
		want string
	}{{nil, "the whole show"}, {[]int{2}, "season 2"}, {[]int{1, 2}, "seasons 1 and 2"}, {[]int{1, 2, 4}, "seasons 1, 2 and 4"}} {
		if got := seasonWords(c.in); got != c.want {
			t.Errorf("%v: %q, want %q", c.in, got, c.want)
		}
	}
	if !covers(nil, []int{3}) || covers([]int{1}, nil) || !covers([]int{1, 2}, []int{2}) || covers([]int{1}, []int{1, 2}) {
		t.Error("covers")
	}
	if w := widen([]int{2}, []int{1}); len(w) != 2 || w[0] != 1 {
		t.Errorf("widen %v", w)
	}
}

// An approved ask that does not cover the seasons asked for never blocks a
// new ask: another member's other season, or the whole show, is asked anew.
func TestShowAskBesideAnApprovedPartialAsk(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	two, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{2})
	if err != nil {
		t.Fatalf("season 2 beside an approved season 1: %v", err)
	}
	if two.Ask.ID == res.Ask.ID || two.Joined || len(two.Ask.Seasons) != 1 || two.Ask.Seasons[0] != 2 {
		t.Fatalf("new ask %+v", two)
	}
	// The whole show while only approved and waiting partial asks exist
	// joins the waiting one that now covers what is not kept.
	whole, err := r.svc.AskShow(ctx, r.ana, "tvdb:200", nil)
	if err != nil || whole.Ask.ID != two.Ask.ID {
		t.Fatalf("whole show %+v %v", whole, err)
	}
}

func TestWholeShowAskBesideAnApprovedPartialAsk(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	whole, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", nil)
	if err != nil {
		t.Fatalf("whole show beside an approved season 1: %v", err)
	}
	if whole.Ask.ID == res.Ask.ID || len(whole.Ask.Seasons) != 1 || whole.Ask.Seasons[0] != 2 {
		t.Fatalf("new ask %+v", whole)
	}
}

// Joining a waiting show ask with more seasons is a new ask for the
// joiner: it is counted against their show limit, and at the limit the ask
// is not widened.
func TestWideningAShowAskCountsAgainstTheLimit(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	one := 1
	if err := SetPersonLimit(ctx, r.st, r.bo.UserID, PersonLimit{Shows: &one}); err != nil {
		t.Fatal(err)
	}
	if err := SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{Shows: &one}); err != nil {
		t.Fatal(err)
	}
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	// A movie does not use bo's show limit; another show does.
	if _, err := r.svc.Ask(ctx, r.bo, "900002"); err != nil {
		t.Fatal(err)
	}
	useAsk(t, r, r.bo.UserID, "show")
	var le *LimitError
	if _, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{2}); !errors.As(err, &le) {
		t.Fatalf("widening at the limit: %v", err)
	}
	got, _ := get(ctx, r.st.Read(), res.Ask.ID)
	if len(got.Seasons) != 1 {
		t.Fatalf("widened past the limit: %v", got.Seasons)
	}
	// A third member with room widens it and is counted.
	cy := Person{UserID: r.luis.UserID, Name: "luis"}
	w, err := r.svc.AskShow(ctx, cy, "tmdb:100", []int{2})
	if err != nil || !w.Joined || len(w.Ask.Seasons) != 2 {
		t.Fatalf("widen %+v %v", w, err)
	}
	var counted int
	_ = r.st.Read().QueryRow("SELECT counted FROM ask_people WHERE ask_id = ? AND user_id = ?", res.Ask.ID, cy.UserID).Scan(&counted)
	if counted != 1 {
		t.Fatalf("widening joiner counted %d, want 1", counted)
	}
}

// Asks of several kinds sent at once: each kind keeps its own limit, and a
// full movie limit never holds back a show.
func TestParallelAsksKeepEachKindLimit(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	if err := SaveSettings(ctx, r.st, Settings{MoviesPerWeek: 1, ShowsPerWeek: 1, MusicPerWeek: 1}, domain.System); err != nil {
		t.Fatal(err)
	}
	movies := []string{"693134", "900002", "900003", "900005"}
	errs := make([]error, len(movies)+1)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, id := range movies {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, errs[i] = r.svc.Ask(ctx, r.ana, id) }()
	}
	wg.Add(1)
	go func() { defer wg.Done(); <-start; _, errs[len(movies)] = r.svc.AskShow(ctx, r.ana, "tmdb:100", nil) }()
	close(start)
	wg.Wait()
	if errs[len(movies)] != nil {
		t.Fatalf("the show was held back: %v", errs[len(movies)])
	}
	ok := 0
	for _, err := range errs[:len(movies)] {
		var le *LimitError
		switch {
		case err == nil:
			ok++
		case !errors.As(err, &le) || le.Kind != KindMovies:
			t.Fatalf("unexpected %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d movie asks got through a limit of 1: %v", ok, errs)
	}
	if l, _ := r.svc.LimitOf(ctx, r.ana); l.Movies.Used != 1 || l.Shows.Used != 1 || l.Music.Used != 0 {
		t.Fatalf("limits %+v", l)
	}
}

// TestShowAskFutureSeasonsOnly: a Future seasons only ask is stored as
// such, a whole-show ask does not join it, and approving adds the show
// with only what is still to air wanted.
func TestShowAskFutureSeasonsOnly(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShowFutureOnly(ctx, r.ana, "tmdb:100")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Ask.FutureOnly || len(res.Ask.Seasons) != 0 {
		t.Fatalf("ask %+v", res.Ask)
	}
	// bo asks for the future too: he joins.
	if j, err := r.svc.AskShowFutureOnly(ctx, r.bo, "tvdb:200"); err != nil || j.Ask.ID != res.Ask.ID {
		t.Fatalf("join %+v %v", j, err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if k := seasonKeeps(t, r.st, *got.TitleID); k[1] || !k[2] {
		t.Fatalf("season switches %v", k)
	}
	var choice string
	var past, ahead bool
	_ = r.st.Read().QueryRow("SELECT season_choice FROM titles WHERE id = ?", *got.TitleID).Scan(&choice)
	_ = r.st.Read().QueryRow("SELECT keep_up_to_date FROM items WHERE title_id = ? AND name = 'Return'", *got.TitleID).Scan(&past)
	_ = r.st.Read().QueryRow("SELECT keep_up_to_date FROM items WHERE title_id = ? AND name = 'Far'", *got.TitleID).Scan(&ahead)
	if choice != "future" || past || !ahead {
		t.Fatalf("choice %q past %v ahead %v", choice, past, ahead)
	}
	// Asking for the future again joins the open approved ask.
	if j, err := r.svc.AskShowFutureOnly(ctx, r.ana, "tmdb:100"); err != nil || j.Ask.ID != res.Ask.ID {
		t.Fatalf("again: %+v %v", j, err)
	}
}

// TestShowAskFutureOnlyForShowInLibrary: a show in the Library that keeps
// nothing is switched to Future seasons only on approval.
func TestShowAskFutureOnlyForShowInLibrary(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	tid, _, err := shows.New(r.svc.Library).AddShow(ctx, "tmdb:100", shows.ShowAddOptions{Seasons: "none", NewSeasons: "none"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// A whole-show ask waits; a future ask must not join it as the whole
	// show, and a whole-show ask must not join a future one.
	res, err := r.svc.AskShowFutureOnly(ctx, r.ana, "tmdb:100")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if k := seasonKeeps(t, r.st, tid); k[1] || !k[2] {
		t.Fatalf("season switches %v", k)
	}
	var policy string
	_ = r.st.Read().QueryRow("SELECT new_parts_policy FROM titles WHERE id = ?", tid).Scan(&policy)
	if policy != "all" {
		t.Fatalf("policy %q", policy)
	}
	whole, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", nil)
	if err != nil || whole.Ask.ID == res.Ask.ID || whole.Ask.FutureOnly {
		t.Fatalf("whole %+v %v", whole, err)
	}
}

// an approved show ask is looked for at once (scan, then a search of
// what the title wants), not at the next missing search hours away; with
// Search on approve off it waits for that search as before.
func TestApprovedShowAskSearchesAtOnce(t *testing.T) {
	queued := func(r *rig, tid int64) int {
		var n int
		_ = r.st.Read().QueryRow("SELECT count(*) FROM jobs WHERE dedupe_key = ?", "scan_then_search:"+strconv.FormatInt(tid, 10)).Scan(&n)
		return n
	}
	r := showRig(t)
	ctx := context.Background()
	res, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	if n := queued(r, *got.TitleID); n != 1 {
		t.Fatalf("scan then search queued %d times, want 1", n)
	}

	off := showRig(t)
	set, _ := ReadSettings(ctx, off.st.Read())
	set.SearchOnApprove = false
	if err := SaveSettings(ctx, off.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	res, _ = off.svc.AskShow(ctx, off.ana, "tmdb:100", []int{1})
	got, err = off.svc.Approve(ctx, res.Ask.ID, off.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve off %+v %v", got, err)
	}
	if n := queued(off, *got.TitleID); n != 0 {
		t.Fatalf("Search on approve off still queued %d", n)
	}
}

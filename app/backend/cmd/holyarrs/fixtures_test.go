package main

// Fixtures copied from internal/api's tests for the cross-module tests
// that run here on the program's real routes.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/api"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// signedIn is one person's browser: cookies and the CSRF token.
type signedIn struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
	id   int64
}

func (s *signedIn) call(method, path, body string, want int) []byte {
	s.t.Helper()
	req, _ := http.NewRequest(method, s.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accounts.CSRFHeader, s.csrf)
	resp, err := s.c.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		s.t.Fatalf("%s %s: %d, want %d: %s", method, path, resp.StatusCode, want, b)
	}
	return b
}

type askRig struct {
	srv   *httptest.Server
	st    *database.Store
	owner *signedIn
	ana   *signedIn
	bo    *signedIn
}

// newAskRig runs the real API with sign-in: the owner luis and the members
// ana and bo (members may ask, nothing more).
func newAskRig(t *testing.T) *askRig {
	t.Helper()
	dir := t.TempDir()
	st, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, insertFolder(t)); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := accounts.New(st, accounts.Options{Vault: v})
	md := &metadata.Static{Movies: []metadata.Movie{metadata.Lab().Movies[0],
		{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "900002"}},
		{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900003"}}}}
	q := jobs.New(st, jobs.Options{Workers: 1})
	a := api.New(api.Server{Store: st, Queue: q, Metadata: md, Version: "test", Auth: au, Secrets: v, Live: webserver.NewLiveHub(webserver.LiveOptions{})})
	srv := httptest.NewServer(routes(a, config{}))
	t.Cleanup(srv.Close)
	r := &askRig{srv: srv, st: st}

	browser := func() *signedIn {
		jar, _ := cookiejar.New(nil)
		return &signedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	}
	read := func(b []byte) (string, string) {
		var x struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &x)
		return x.CSRF, x.Path
	}
	r.owner = browser()
	r.owner.csrf, _ = read(r.owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, 201))
	join := func(name string) *signedIn {
		_, link := read(r.owner.call("POST", "/api/v1/users/invites", `{}`, 201))
		b := browser()
		b.csrf, _ = read(b.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(link, "/invite/"), `{"name":"`+name+`","password":"a long test passphrase"}`, 201))
		var me struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(b.call("GET", "/api/v1/auth/me", "", 200), &me)
		b.id = me.ID
		return b
	}
	r.ana, r.bo = join("ana"), join("bo")
	return r
}

func insertFolder(t *testing.T) func(tx *sql.Tx) error {
	dir := t.TempDir()
	return func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: dir, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	}
}

// fakeShowSource answers fixed shows by "<kind>:<id>".
type fakeShowSource struct {
	mu    sync.Mutex
	shows map[string]metadata.Show
	err   error
}

func (f *fakeShowSource) SearchShows(_ context.Context, q string) ([]metadata.Show, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	seen := map[string]bool{}
	out := []metadata.Show{}
	for _, sh := range f.shows {
		if strings.Contains(strings.ToLower(sh.Name), strings.ToLower(q)) && !seen[sh.Name] {
			seen[sh.Name] = true
			out = append(out, sh)
		}
	}
	return out, nil
}

func (f *fakeShowSource) Show(_ context.Context, id string) (metadata.Show, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return metadata.Show{}, f.err
	}
	if sh, ok := f.shows[id]; ok {
		return sh, nil
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

func at(s string) *time.Time { t, _ := time.Parse(time.RFC3339, s); return &t }

// apiShow is a show with a finished first season and a second one far in
// the future, so the test does not depend on today's date.
func apiShow(source, id string) metadata.Show {
	return metadata.Show{Source: source, Name: "Lab Show", Year: 2025, Status: "running", Network: "Lab TV",
		IDs:     map[string]string{source: id, "tvdb": "200", "imdb": "tt0100"},
		Seasons: []metadata.Season{{Number: 0, Name: "Specials"}, {Number: 1, Name: "Season 1"}, {Number: 2, Name: "Season 2"}},
		Episodes: []metadata.Episode{
			{Season: 0, Number: 1, Name: "Behind the Lab", AirDate: at("2025-03-01T12:00:00Z"), Timed: true, AirDay: "2025-03-01"},
			{Season: 1, Number: 1, Name: "Pilot", AirDate: at("2025-01-06T02:00:00Z"), Timed: true, AirDay: "2025-01-05", ID: "e101"},
			{Season: 1, Number: 2, Name: "Second", AirDate: at("2025-01-13T02:00:00Z"), Timed: true, AirDay: "2025-01-12", ID: "e102"},
			{Season: 2, Number: 1, Name: "Return", AirDate: at("2099-01-05T02:00:00Z"), Timed: true, AirDay: "2099-01-04", ID: "e201"},
			{Season: 2, Number: 2, Name: "TBA", ID: "e202"},
		}}
}

type addedShow struct {
	Title struct {
		ID    int64  `json:"id"`
		Kind  string `json:"kind"`
		Slots []any  `json:"slots"`
		Show  struct {
			DetailsSource string                `json:"details_source"`
			EpisodeOrder  string                `json:"episode_order"`
			SeasonChoice  string                `json:"season_choice"`
			NewSeasons    string                `json:"new_seasons"`
			Seasons       []shows.SeasonSummary `json:"seasons"`
			Episodes      int                   `json:"episodes"`
			Missing       int                   `json:"missing"`
			NotOutYet     int                   `json:"not_out_yet"`
		} `json:"show"`
	} `json:"title"`
	JobID int64 `json:"job_id"`
}

// The music fixtures the api tests that span music and other modules
// share (the music tests keep their own copy in internal/music).

// fakeArtists is MusicBrainz over fixed answers.
type fakeArtists struct {
	mu        sync.Mutex
	artists   map[string]metadata.Artist
	pressings map[string][]metadata.Pressing
	err       error
	// block holds SearchArtists until closed; entered says one came in;
	// members records whether each search was a member's.
	block   chan struct{}
	entered chan struct{}
	members []bool
}

func (f *fakeArtists) SearchArtists(ctx context.Context, q string) ([]metadata.Artist, error) {
	f.mu.Lock()
	f.members = append(f.members, metadata.IsMemberLookup(ctx))
	block, entered := f.block, f.entered
	f.mu.Unlock()
	if block != nil {
		entered <- struct{}{}
		<-block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	out := []metadata.Artist{}
	for _, a := range f.artists {
		if strings.Contains(strings.ToLower(a.Name), strings.ToLower(q)) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeArtists) Artist(_ context.Context, mbid string) (metadata.Artist, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return metadata.Artist{}, f.err
	}
	if a, ok := f.artists[mbid]; ok {
		return a, nil
	}
	return metadata.Artist{}, metadata.ErrArtistNotFound
}

func (f *fakeArtists) Pressings(_ context.Context, mbid string) ([]metadata.Pressing, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pressings[mbid], f.err
}

// AlbumArtist finds the album among the fixed artists' albums.
func (f *fakeArtists) AlbumArtist(_ context.Context, album string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, a := range f.artists {
		for _, al := range a.Albums {
			if al.MBID == album {
				return id, nil
			}
		}
	}
	return "", metadata.ErrArtistNotFound
}

const (
	apiArtist = "a0000000-0000-0000-0000-0000000000aa"
	apiAlbum  = "10000000-0000-0000-0000-0000000000aa"
	apiLive   = "10000000-0000-0000-0000-0000000000ab"
	apiSoon   = "10000000-0000-0000-0000-0000000000ac"
	apiRelA   = "20000000-0000-0000-0000-0000000000aa"
	apiRelB   = "20000000-0000-0000-0000-0000000000ab"
)

func apiMusic() *fakeArtists {
	tr := func(p string, n int) []metadata.Track {
		out := []metadata.Track{}
		for i := 1; i <= n; i++ {
			out = append(out, metadata.Track{ID: fmt.Sprintf("%s-%d", p, i), RecordingID: fmt.Sprintf("rec-%d", i), Disc: 1, Number: i, Position: i,
				Name: fmt.Sprintf("Song %d", i), LengthMS: 180000})
		}
		return out
	}
	return &fakeArtists{
		artists: map[string]metadata.Artist{apiArtist: {MBID: apiArtist, Name: "Lab Band", Type: "group", Status: "running", Albums: []metadata.Album{
			{MBID: apiAlbum, Name: "First Light", PrimaryType: "album", ReleaseDate: "2020-03-06"},
			{MBID: apiLive, Name: "Live Night", PrimaryType: "album", SecondaryTypes: []string{"live"}, ReleaseDate: "2021"},
			{MBID: apiSoon, Name: "Soon", PrimaryType: "ep", ReleaseDate: "2099-01-01"},
		}}},
		pressings: map[string][]metadata.Pressing{apiAlbum: {
			{MBID: apiRelA, Name: "First Light", Status: "official", Country: "XW", Date: "2020-03-06", Formats: []string{"Digital Media"}, Tracks: tr("a", 2)},
			{MBID: apiRelB, Name: "First Light", Status: "official", Country: "US", Date: "2020", Disambiguation: "deluxe", Formats: []string{"CD"}, Tracks: tr("b", 3)},
		}},
	}
}

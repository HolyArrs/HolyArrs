package main

// Helpers for the full-server migrate, arr API
// and AI tests. Names carry an arr prefix so they never clash with other
// helpers in this package.

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
)

func arrItoa(n int64) string { return strconv.FormatInt(n, 10) }

func arrSetup(t *testing.T) (*httptest.Server, *database.Store, *jobs.Queue) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	hub := webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second})
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Live: hub, Metadata: metadata.Lab(), Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, s, q
}
func arrDo(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}

type arrSignedIn struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
	id   int64
}

func (s *arrSignedIn) call(method, path, body string, want int) []byte {
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

type arrAskRig struct {
	srv   *httptest.Server
	st    *database.Store
	owner *arrSignedIn
	ana   *arrSignedIn
	bo    *arrSignedIn
	api   *handlers
}

func newArrAskRig(t *testing.T) *arrAskRig {
	t.Helper()
	dir := t.TempDir()
	st, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	if err := st.Write(ctx, arrInsertFolder(t)); err != nil {
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
	a := buildHandlers(services{Store: st, Queue: q, Metadata: md, Version: "test", Auth: au, Secrets: v, Live: webserver.NewLiveHub(webserver.LiveOptions{})})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	r := &arrAskRig{srv: srv, st: st, api: a}

	browser := func() *arrSignedIn {
		jar, _ := cookiejar.New(nil)
		return &arrSignedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
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
	join := func(name string) *arrSignedIn {
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
func arrInsertFolder(t *testing.T) func(tx *sql.Tx) error {
	dir := t.TempDir()
	return func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: dir, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	}
}

type arrCtlFix struct {
	srv     *httptest.Server
	s       *database.Store
	pc      *parts.Control
	reg     *jobs.Registry
	api     *handlers
	runs    atomic.Int64 // passes of the test loops
	mu      sync.Mutex
	exits   []string
	exitErr error
}

func newArrCtlMode(t *testing.T, withAuth, safe bool) *arrCtlFix {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	f := &arrCtlFix{s: s, reg: jobs.NewRegistry()}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	f.pc = &parts.Control{Store: s, Reg: f.reg, Jobs: q, Drain: time.Second, SafeMode: safe, Logf: func(string, ...any) {}}
	if err := f.pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	q.SetGate(f.pc)
	for _, name := range []string{"gates", "backups"} {
		f.pc.Add(jobs.Loop{Name: name, Label: name, Every: 20 * time.Millisecond, Run: func(context.Context) error { f.runs.Add(1); return nil }}, name == "gates")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); q.Stop(); f.reg.Wait() })
	f.pc.Start(ctx)
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	srvOpts := services{Store: s, Queue: q, Metadata: metadata.Lab(), Version: "test", Parts: f.pc, States: f.reg, SafeMode: safe,
		ExitApp: func(how string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.exitErr != nil {
				return f.exitErr
			}
			f.exits = append(f.exits, how)
			return nil
		}}
	if withAuth {
		srvOpts.Auth = accounts.New(s, accounts.Options{})
	}
	f.api = buildHandlers(srvOpts)
	f.srv = httptest.NewServer(f.api.Handler())
	t.Cleanup(f.srv.Close)
	return f
}
func arrMoveSetup(t *testing.T, safe bool) (*httptest.Server, *database.Store, int64, string) {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	films := filepath.Join(t.TempDir(), "films")
	if err := os.MkdirAll(filepath.Join(films, "Dune Part Two (2024)"), 0o755); err != nil {
		t.Fatal(err)
	}
	q := jobs.New(st, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	lib := &library.Service{Store: st, Queue: q, Metadata: metadata.Lab()}
	mv := &migrate.Mover{Store: st, Library: lib, Queue: q}
	mv.Register(q)
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(buildHandlers(services{Store: st, Queue: q, Library: lib, Metadata: metadata.Lab(), Version: "test", Mover: mv, SafeMode: safe}).Handler())
	t.Cleanup(srv.Close)
	snap := &migrate.Snapshot{Kind: migrate.Radarr, Via: migrate.ViaBackup, Version: "6.4.4",
		Folders: []migrate.Folder{{Path: "/movies"}},
		Titles: []migrate.Title{{SourceID: 1, Kind: "movie", Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134"},
			Path: "/movies/Dune Part Two (2024)", Monitored: true}},
		Tags:         []migrate.Tag{{ID: 1, Label: "kids"}},
		Indexers:     []migrate.Provider{{Name: "Nyaa", Kind: "Torznab", Enabled: true, Settings: map[string]any{"baseUrl": "http://prowlarr:9696/1/"}, NeedsReentry: []string{"apiKey"}}},
		NeedsReentry: []string{`indexer "Nyaa": apiKey`},
	}
	saved, err := migrate.Save(context.Background(), st, 1, snap)
	if err != nil {
		t.Fatal(err)
	}
	return srv, st, saved.ID, filepath.ToSlash(films)
}

type arrShowSource struct {
	mu    sync.Mutex
	shows map[string]metadata.Show
	err   error
}

func (f *arrShowSource) SearchShows(_ context.Context, q string) ([]metadata.Show, error) {
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
func (f *arrShowSource) Show(_ context.Context, id string) (metadata.Show, error) {
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
func arrShow(source, id string) metadata.Show {
	return metadata.Show{Source: source, Name: "Lab Show", Year: 2025, Status: "running", Network: "Lab TV",
		IDs:     map[string]string{source: id, "tvdb": "200", "imdb": "tt0100"},
		Seasons: []metadata.Season{{Number: 0, Name: "Specials"}, {Number: 1, Name: "Season 1"}, {Number: 2, Name: "Season 2"}},
		Episodes: []metadata.Episode{
			{Season: 0, Number: 1, Name: "Behind the Lab", AirDate: arrAt("2025-03-01T12:00:00Z"), Timed: true, AirDay: "2025-03-01"},
			{Season: 1, Number: 1, Name: "Pilot", AirDate: arrAt("2025-01-06T02:00:00Z"), Timed: true, AirDay: "2025-01-05", ID: "e101"},
			{Season: 1, Number: 2, Name: "Second", AirDate: arrAt("2025-01-13T02:00:00Z"), Timed: true, AirDay: "2025-01-12", ID: "e102"},
			{Season: 2, Number: 1, Name: "Return", AirDate: arrAt("2099-01-05T02:00:00Z"), Timed: true, AirDay: "2099-01-04", ID: "e201"},
			{Season: 2, Number: 2, Name: "TBA", ID: "e202"},
		}}
}

const arrSourceCaps = `<?xml version="1.0"?><caps><limits max="50"/><searching><search available="yes" supportedParams="q"/>
<movie-search available="yes" supportedParams="q,imdbid"/></searching><categories><category id="2000"/></categories></caps>`

func arrAt(s string) *time.Time { t, _ := time.Parse(time.RFC3339, s); return &t }

func newArrCtl(t *testing.T, withAuth bool) *arrCtlFix {
	t.Helper()
	return newArrCtlMode(t, withAuth, false)
}

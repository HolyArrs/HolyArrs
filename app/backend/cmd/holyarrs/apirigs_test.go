package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/api"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
	"github.com/HolyArrs/holyarrs/app/backend/internal/titleview"
)

// Rigs for the tests that cross modules: the whole api with sign-in, the
// Library and the Downloads reader.

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
	asks  *requests.Service
	owner *signedIn
	ana   *signedIn
	bo    *signedIn
	api   *api.API
	md    metadata.Provider
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
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	r := &askRig{srv: srv, st: st, api: a, md: md}

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

func day(s string) *time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return &t
}

// libSetup is an API with two TMDB-like movies, no Library folder yet and a
// running queue with the library's handlers and a fake search.
func libSetup(t *testing.T) (*httptest.Server, *database.Store, chan int64) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	md := &metadata.Static{Movies: []metadata.Movie{
		{Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134", "imdb": "tt15239678"},
			CinemaDate: day("2024-03-01"), DigitalDate: day("2024-04-16")},
		{Name: "The Lantern Keepers", Year: 2031, IDs: map[string]string{"tmdb": "900001"}, CinemaDate: day("2031-02-01")},
	}}
	q := jobs.New(s, jobs.Options{Workers: 2, PollInterval: 5 * time.Millisecond})
	svc := &library.Service{Store: s, Queue: q, Metadata: md}
	svc.Register(q)
	searched := make(chan int64, 10)
	q.Handle(library.KindSearchSlot, func(ctx context.Context, r *jobs.Run) error { searched <- r.Job.Subject.ID; return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(api.New(api.Server{Store: s, Queue: q, Metadata: md, Version: "test", Library: svc}).Handler())
	t.Cleanup(srv.Close)
	return srv, s, searched
}

// organiseWorld adds a movie Library folder and Dune with one imported file.
func organiseWorld(t *testing.T) (url string, s *database.Store, lib string, titleID, fileID int64) {
	t.Helper()
	srv, s, _ := libSetup(t)
	lib = t.TempDir()
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, lib), 201)
	var added struct {
		Title titleview.TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201), &added)
	titleID = added.Title.ID
	rel := "Dune - Part Two (2024)/Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv"
	if err := os.MkdirAll(filepath.Join(lib, "Dune - Part Two (2024)"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, filepath.FromSlash(rel)), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	release := "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS"
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var vid, sid int64
		if err := tx.QueryRow("SELECT v.id, s.id FROM versions v JOIN slots s ON s.version_id = v.id WHERE v.title_id = ?", titleID).Scan(&vid, &sid); err != nil {
			return err
		}
		f := &mediafiles.File{SlotIDs: []int64{sid}, VersionID: vid, RelativePath: rel, Size: 5, AddedAt: time.Now(), Languages: []string{},
			MediaInfo: map[string]any{}, ReleaseName: &release, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		id, err := database.Insert(tx, "files", f, domain.System)
		fileID = id
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv.URL, s, lib, titleID, fileID
}

// feedStore does one background read by hand: every torrent Downloader's
// whole list goes into the reader's Store (made when the API has none),
// the way the reader's pollers fill it. The Downloads view reads only it.
func feedStore(t *testing.T, a *api.API) *downloads.Sync {
	t.Helper()
	if a.DownloadsWeb().DLSync == nil {
		a.DownloadsWeb().DLSync = &downloads.Sync{Store: &downloads.Store{}, DB: a.DownloadsWeb().Store}
	}
	ctx := context.Background()
	ds, err := database.Select[domain.Downloader](ctx, a.DownloadsWeb().Store.Read(), "downloaders", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		if d.Removed || d.Protocol != "torrent" {
			continue
		}
		var jobs []downloads.Job
		err := a.DownloadsWeb().Manager().ControlEngine(ctx, d, func(e downloads.Engine) error {
			al, ok := e.(downloads.AllLister)
			if !ok {
				return downloads.ErrNotSupported
			}
			var err error
			jobs, err = al.ListAll(ctx)
			return err
		})
		if errors.Is(err, downloads.ErrNotSupported) {
			err = nil
		}
		if err != nil {
			a.DownloadsWeb().DLSync.Store.Fail(d.ID, 0)
			continue
		}
		a.DownloadsWeb().DLSync.Store.Update(d.ID, 0, jobs, time.Now())
	}
	return a.DownloadsWeb().DLSync
}

// matchStored runs the reader's match lookup (MatchLinkOnly) once for every
// outside item in the Store, as its match queue would.
func matchStored(t *testing.T, a *api.API) {
	t.Helper()
	ctx := context.Background()
	ds, err := database.Select[domain.Downloader](ctx, a.DownloadsWeb().Store.Read(), "downloaders", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		for _, j := range a.DownloadsWeb().DLSync.Store.Current(d.ID).Items {
			if _, _, err := a.DownloadsWeb().MatchLinkOnly(ctx, d.ID, j.ID, j.Name, j.Category); err != nil {
				t.Fatalf("match %q: %v", j.Name, err)
			}
		}
	}
}

func body(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

type viewRows struct {
	Downloads []downloads.DownloadRow `json:"downloads"`
}

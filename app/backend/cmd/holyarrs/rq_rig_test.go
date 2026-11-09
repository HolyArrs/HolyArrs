package main

// Rigs for the request-side tests that span several modules (reports,
// reviews, title defaults, the waiting download row). They run the real
// New server.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func rqItoa(n int64) string { return strconv.FormatInt(n, 10) }

func rqDo(t *testing.T, method, url, body string, want int) []byte {
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

func rqExec(t *testing.T, s *database.Store, q string, args ...any) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// rqBrowser is one person's browser: cookies and the CSRF token.
type rqBrowser struct {
	t    *testing.T
	base string
	c    *http.Client
	csrf string
	id   int64
}

func (s *rqBrowser) call(method, path, body string, want int) []byte {
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

type rqAskRig struct {
	st             *database.Store
	owner, ana, bo *rqBrowser
}

// rqNewAskRig runs the real API with sign-in: the owner luis and the
// members ana and bo.
func rqNewAskRig(t *testing.T) *rqAskRig {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	dir := t.TempDir()
	if err := st.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: dir, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	v, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := accounts.New(st, accounts.Options{Vault: v})
	md := &metadata.Static{Movies: []metadata.Movie{metadata.Lab().Movies[0]}}
	q := jobs.New(st, jobs.Options{Workers: 1})
	a := buildHandlers(services{Store: st, Queue: q, Metadata: md, Version: "test", Auth: au, Secrets: v, Live: webserver.NewLiveHub(webserver.LiveOptions{})})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	r := &rqAskRig{st: st}
	browser := func() *rqBrowser {
		jar, _ := cookiejar.New(nil)
		return &rqBrowser{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
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
	join := func(name string) *rqBrowser {
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

// rqMovieCopy puts one movie with a file in the Library.
func rqMovieCopy(t *testing.T, s *database.Store) (mediafiles.File, mediafiles.Version) {
	t.Helper()
	var f mediafiles.File
	var v mediafiles.Version
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var folderID int64
		if err := tx.QueryRow("SELECT id FROM library_folders ORDER BY id LIMIT 1").Scan(&folderID); err != nil {
			return err
		}
		rules := &mediafiles.PickRules{Name: "Copy search", Kinds: []string{"movie"}, Revision: 1, Body: mediafiles.PickRulesBody{}}
		if _, err := database.Insert(tx, "pick_rules", rules, domain.System); err != nil {
			return err
		}
		title := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Arrival", SortName: "arrival", MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		if _, err := database.Insert(tx, "titles", title, domain.System); err != nil {
			return err
		}
		item := &mediafiles.Item{TitleID: title.ID, Kind: "movie", Numbers: mediafiles.Numbers{}, KeepUpToDate: true, SourceIDs: map[string]string{}}
		if _, err := database.Insert(tx, "items", item, domain.System); err != nil {
			return err
		}
		v = mediafiles.Version{TitleID: title.ID, Label: "4K", PickRulesID: rules.ID, LibraryFolderID: folderID, FolderName: "Arrival", KeepUpToDate: true, IsMain: true, AudioLanguages: []string{"en"}}
		if _, err := database.Insert(tx, "versions", &v, domain.System); err != nil {
			return err
		}
		f = mediafiles.File{SlotIDs: []int64{}, VersionID: v.ID, RelativePath: "Arrival/Arrival.mkv", Size: 10, AddedAt: time.Now().UTC(), Quality: mediafiles.Quality{Codec: "av1"}, Languages: []string{}, MediaInfo: map[string]any{"inspection_status": "complete", "container": "matroska", "video_codec": "hevc", "audio_languages": []string{"en"}, "audio_tracks": 1}, Origin: "imported", PlacedBy: "move", Role: "main", State: "present"}
		if _, err := database.Insert(tx, "files", &f, domain.System); err != nil {
			return err
		}
		slot := mediafiles.Slot{ItemID: item.ID, VersionID: v.ID, State: mediafiles.SlotInLibrary, FileID: &f.ID}
		if _, err := database.Insert(tx, "slots", &slot, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE files SET slot_ids=json_array(?) WHERE id=?", slot.ID, f.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return f, v
}

// rqSeedDownload makes one movie, its Release, a fake qBittorrent and a
// Download in dlState, and serves the real API.
func rqSeedDownload(t *testing.T, slotState mediafiles.SlotState, dlState string) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var mu sync.Mutex
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { mu.Lock(); mu.Unlock() }))
	t.Cleanup(qb.Close)
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		y := 2020
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Salt Choir", SortName: "salt choir", Year: &y, Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		ver := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Salt Choir", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", ver, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: ver.ID, State: slotState}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		rel := &domain.Release{SourceID: src.ID, Name: "Salt.Choir.2020.1080p.WEB.H264-HOLYARRS", LinkRef: "http://x/a.torrent", Protocol: "torrent", Size: 1000,
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Writable: true, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: qb.URL, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		eta := int64(90)
		dl := &domain.Download{DownloaderID: dr.ID, ReleaseID: &rel.ID, SlotIDs: []int64{sl.ID}, ExternalID: "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd", State: dlState, Progress: 0.3,
			Speed: 2048, ETA: &eta, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", dl.ID, sl.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: time.Hour})
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Live: webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second}), Metadata: metadata.Lab(), Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

// rqLibSetup is an API with two TMDB-like movies, no Library folder yet
// and a running queue with the library's handlers and a fake search.
func rqLibSetup(t *testing.T) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	day := func(d string) *time.Time { x, _ := time.Parse("2006-01-02", d); return &x }
	md := &metadata.Static{Movies: []metadata.Movie{
		{Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134", "imdb": "tt15239678"},
			CinemaDate: day("2024-03-01"), DigitalDate: day("2024-04-16")},
		{Name: "The Lantern Keepers", Year: 2031, IDs: map[string]string{"tmdb": "900001"}, CinemaDate: day("2031-02-01")},
	}}
	q := jobs.New(s, jobs.Options{Workers: 2, PollInterval: 5 * time.Millisecond})
	svc := &library.Service{Store: s, Queue: q, Metadata: md}
	svc.Register(q)
	q.Handle(library.KindSearchSlot, func(ctx context.Context, r *jobs.Run) error { return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Metadata: md, Version: "test", Library: svc}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

// rqShows answers fixed shows by "<kind>:<id>".
type rqShows struct {
	shows map[string]metadata.Show
	err   error
}

func (f *rqShows) SearchShows(context.Context, string) ([]metadata.Show, error) {
	return nil, f.err
}

func (f *rqShows) Show(_ context.Context, id string) (metadata.Show, error) {
	if f.err != nil {
		return metadata.Show{}, f.err
	}
	if sh, ok := f.shows[id]; ok {
		return sh, nil
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

func rqShowSetup(t *testing.T) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	at := func(v string) *time.Time { x, _ := time.Parse(time.RFC3339, v); return &x }
	show := metadata.Show{Source: "tmdb", Name: "Lab Show", Year: 2025, Status: "running", Network: "Lab TV",
		IDs:     map[string]string{"tmdb": "100", "tvdb": "200", "imdb": "tt0100"},
		Seasons: []metadata.Season{{Number: 0, Name: "Specials"}, {Number: 1, Name: "Season 1"}, {Number: 2, Name: "Season 2"}},
		Episodes: []metadata.Episode{
			{Season: 0, Number: 1, Name: "Behind the Lab", AirDate: at("2025-03-01T12:00:00Z"), Timed: true, AirDay: "2025-03-01"},
			{Season: 1, Number: 1, Name: "Pilot", AirDate: at("2025-01-06T02:00:00Z"), Timed: true, AirDay: "2025-01-05", ID: "e101"},
			{Season: 1, Number: 2, Name: "Second", AirDate: at("2025-01-13T02:00:00Z"), Timed: true, AirDay: "2025-01-12", ID: "e102"},
			{Season: 2, Number: 1, Name: "Return", AirDate: at("2099-01-05T02:00:00Z"), Timed: true, AirDay: "2099-01-04", ID: "e201"},
			{Season: 2, Number: 2, Name: "TBA", ID: "e202"},
		}}
	q := jobs.New(s, jobs.Options{})
	lib := &library.Service{Store: s, Queue: q, Shows: metadata.ShowSources{"tmdb": &rqShows{shows: map[string]metadata.Show{"tmdb:100": show}},
		"tvmaze": &rqShows{}, "tvdb": &rqShows{err: metadata.ErrNoTVDBKey}}}
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Library: lib, Metadata: &metadata.Static{}}).Handler())
	t.Cleanup(srv.Close)
	rqDo(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"show","label":"Shows"}`, t.TempDir()), 201)
	return srv, s
}

func rqRuleJSON(t *testing.T, name string, kinds []string, b mediafiles.PickRulesBody) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"name": name, "kinds": kinds, "body": b})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func rqRuleByName(t *testing.T, base, name string) downloads.PickRulesView {
	t.Helper()
	var got struct {
		PickRules []downloads.PickRulesView `json:"pick_rules"`
	}
	_ = json.Unmarshal(rqDo(t, "GET", base+"/api/v1/pick-rules", "", 200), &got)
	for _, p := range got.PickRules {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no Pick rules named %q in %+v", name, got)
	return downloads.PickRulesView{}
}

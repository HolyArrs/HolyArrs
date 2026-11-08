package api

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
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const dlPinLib = 1200

// dlPinWant is what one seeded download must show in the view.
type dlPinWant struct {
	dlID, titleID        int64
	title, version, kind string
	year, titleYear      int
	name, source, owner  string
	album, covers, phase string
	size                 int64
	decision, hasRelease bool
	asked                bool
	episodes             int
}

type dlPinWorld struct {
	srv    *httptest.Server
	s      *database.Store
	owner  *http.Client
	member *http.Client
	csrf   string
	mcsrf  string
	ana    int64
	want   map[int64]*dlPinWant // by download id
	order  []int64              // download ids, oldest first
}

func (w *dlPinWorld) get(t *testing.T, c *http.Client, path string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", w.srv.URL+path, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func (w *dlPinWorld) send(t *testing.T, c *http.Client, csrf, method, path, body string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, w.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accounts.CSRFHeader, csrf)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// newDlPinWorld starts the real API with sign-in on, an owner and a member
// (ana), and seeds dlPinLib downloads over movies, shows and artists.
func newDlPinWorld(t *testing.T) *dlPinWorld {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := accounts.New(s, accounts.Options{Vault: v})
	srv := httptest.NewServer(New(Server{Store: s, Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v,
		Live: webserver.NewLiveHub(webserver.LiveOptions{})}).Handler())
	t.Cleanup(srv.Close)
	w := &dlPinWorld{srv: srv, s: s, want: map[int64]*dlPinWant{}}
	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	csrfOf := func(b []byte) string {
		var o struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		_ = json.Unmarshal(b, &o)
		return o.CSRF + o.Path
	}
	w.owner, w.member = newClient(), newClient()
	st, b := w.send(t, w.owner, "", "POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("setup %d %s", st, b)
	}
	w.csrf = csrfOf(b)
	st, b = w.send(t, w.owner, w.csrf, "POST", "/api/v1/users/invites", `{}`)
	if st != 201 {
		t.Fatalf("invite %d %s", st, b)
	}
	st, b = w.send(t, w.member, "", "POST", "/api/v1/auth/invites/"+strings.TrimPrefix(csrfOf(b), "/invite/"), `{"name":"ana","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("join %d %s", st, b)
	}
	w.mcsrf = csrfOf(b)
	if err := s.Read().QueryRow("SELECT id FROM users WHERE name = 'ana'").Scan(&w.ana); err != nil {
		t.Fatal(err)
	}
	w.seed(t)
	return w
}

func (w *dlPinWorld) seed(t *testing.T) {
	t.Helper()
	err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Writable: true, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		for i := 0; i < dlPinLib; i++ {
			kind := []string{"movie", "show", "artist", "movie"}[i%4]
			year := 1950 + i%70
			owner := ""
			by := domain.System
			if i%3 == 0 {
				by = domain.Actor{Kind: "user", ID: w.ana}
				owner = "ana"
			}
			name := fmt.Sprintf("Title %04d", i)
			ti := &mediafiles.Title{Kind: kind, GateDriver: "none", Name: name, SortName: strings.ToLower(name), Year: &year, Aliases: []string{}, SourceIDs: map[string]string{},
				MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
			if _, err := database.Insert(tx, "titles", ti, by); err != nil {
				return err
			}
			it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
			album, covers := "", ""
			wantYear := year
			switch kind {
			case "show":
				se, ep := i%3+1, i%10+1
				it.Kind, it.Numbers = "episode", mediafiles.Numbers{Season: &se, Episode: &ep}
				n := fmt.Sprintf("Ep %d", i)
				it.Name = &n
				covers = fmt.Sprintf("S%02dE%02d %s", se, ep, n)
			case "artist":
				wantYear = 0 // an artist row drops the title year unless an album gives one
				tr := i%9 + 1
				it.Kind, it.Numbers = "track", mediafiles.Numbers{Track: &tr}
				if i%8 == 2 { // a track with an album; i%8 == 6 has no album (NULL part)
					d := time.Date(2001, 5, 1, 0, 0, 0, 0, time.UTC)
					pt := &mediafiles.Part{TitleID: ti.ID, Kind: "album", Name: fmt.Sprintf("Album %d", i), ReleaseDate: &d, KeepUpToDate: true,
						SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}}
					if _, err := database.Insert(tx, "parts", pt, domain.System); err != nil {
						return err
					}
					it.PartID = &pt.ID
					album, wantYear = pt.Name, 2001
				}
			}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			ver := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: name, IsMain: true, KeepUpToDate: true}
			if _, err := database.Insert(tx, "versions", ver, domain.System); err != nil {
				return err
			}
			state := mediafiles.SlotDownloading
			if i%10 == 5 {
				state = mediafiles.SlotNeedsADecision
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: ver.ID, State: state}
			if state == mediafiles.SlotNeedsADecision {
				r := "the download failed and no other Release passed the Pick rules"
				sl.LastReason = &r
			}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			pw := &dlPinWant{title: name, titleID: ti.ID, version: "HD", kind: kind, year: wantYear, titleYear: year, owner: owner, album: album, covers: covers, decision: state == mediafiles.SlotNeedsADecision}
			if kind == "show" || kind == "artist" {
				pw.episodes = 1
			}
			dl := &domain.Download{DownloaderID: dr.ID, SlotIDs: []int64{sl.ID}, ExternalID: fmt.Sprintf("%040x", i+1), State: "downloading", Progress: 0.5,
				Speed: 100, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
			if i%4 != 3 { // a download with no Release (NULL partner)
				rel := &domain.Release{SourceID: src.ID, Name: fmt.Sprintf("Rel.%04d.1080p", i), LinkRef: "http://x/a.torrent", Protocol: "torrent", Size: int64(1000 + i),
					Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
				if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
					return err
				}
				dl.ReleaseID = &rel.ID
				pw.name, pw.size, pw.source, pw.hasRelease = rel.Name, rel.Size, "Lab", true
			} else {
				pw.name = dl.ExternalID
			}
			if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
				return err
			}
			if _, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", dl.ID, sl.ID); err != nil {
				return err
			}
			// Every fifth title is asked for by ana (the member's own downloads).
			if i%5 == 0 {
				ask := &domain.Ask{Kind: "movie", TMDBID: fmt.Sprint(1000 + i), Name: name, State: "approved", TitleID: &ti.ID, AskedBy: w.ana, AskedAt: time.Now().UTC(), Ref: fmt.Sprintf("tmdb:%d", 1000+i)}
				if _, err := database.Insert(tx, "asks", ask, domain.System); err != nil {
					return err
				}
				if _, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 1)", ask.ID, w.ana, time.Now().UTC().Format(database.TimeLayout)); err != nil {
					return err
				}
				pw.asked = true
			}
			pw.dlID = dl.ID
			w.want[dl.ID] = pw
			w.order = append(w.order, dl.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type dlPinView struct {
	Downloads []downloads.DownloadRow  `json:"downloads"`
	Totals    downloads.DownloadTotals `json:"totals"`
}

func dlPinCheckRow(t *testing.T, r downloads.DownloadRow, w *dlPinWant, manager bool) {
	t.Helper()
	if r.TitleID != w.titleID || r.Title != w.title || r.Version != w.version || r.Kind != w.kind {
		t.Errorf("download %d: title %d %q %q %q, want %d %q %q %q", r.ID, r.TitleID, r.Title, r.Version, r.Kind, w.titleID, w.title, w.version, w.kind)
	}
	if w.year == 0 && r.Year != nil || w.year != 0 && (r.Year == nil || *r.Year != w.year) {
		t.Errorf("download %d: year %v, want %d", r.ID, r.Year, w.year)
	}
	if r.Name != w.name || r.Size != w.size {
		t.Errorf("download %d: name %q size %d, want %q %d", r.ID, r.Name, r.Size, w.name, w.size)
	}
	if r.Album != w.album {
		t.Errorf("download %d: album %q, want %q", r.ID, r.Album, w.album)
	}
	if len(r.Episodes) != w.episodes {
		t.Errorf("download %d: %d episode lines, want %d", r.ID, len(r.Episodes), w.episodes)
	}
	if w.kind == "show" && r.Covers != w.covers && !strings.HasPrefix(r.Covers, strings.SplitN(w.covers, " ", 2)[0]) {
		t.Errorf("download %d: covers %q, want %q", r.ID, r.Covers, w.covers)
	}
	wantSlot := "downloading"
	if w.decision {
		wantSlot = "needs_a_decision"
	}
	if r.SlotState != wantSlot {
		t.Errorf("download %d: slot state %q, want %q", r.ID, r.SlotState, wantSlot)
	}
	if manager {
		if r.RequestedBy != w.owner || r.SourceName != w.source {
			t.Errorf("download %d: requested by %q source %q, want %q %q", r.ID, r.RequestedBy, r.SourceName, w.owner, w.source)
		}
		if w.hasRelease && r.SourceID == 0 || !w.hasRelease && r.SourceID != 0 {
			t.Errorf("download %d: source id %d with release %v", r.ID, r.SourceID, w.hasRelease)
		}
		if !r.CanAct && r.Active {
			t.Errorf("download %d: a manager can act on an active row", r.ID)
		}
	} else if r.RequestedBy != "" || r.SourceName != "" || r.Path != "" || r.CanAct {
		t.Errorf("download %d: a member sees manager fields: %+v", r.ID, r)
	}
}

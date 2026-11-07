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
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Pins for the download list reads (slotInfos, releaseNames, rowOwners, the
// member filter and decisions). They hold today's results on a large library
// with NULL partners, so a later rewrite of the cross-module JOINs can be
// checked against them.

const dlPinLib = 1200

// dlPinWant is what one seeded download must show in the view.
type dlPinWant struct {
	dlID, titleID        int64
	title, version, kind string
	year, titleYear int
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
	order  []int64            // download ids, oldest first
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
	req.Header.Set(auth.CSRFHeader, csrf)
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
	au := auth.New(s, auth.Options{Vault: v})
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
		lf := &domain.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
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
			ti := &domain.Title{Kind: kind, GateDriver: "none", Name: name, SortName: strings.ToLower(name), Year: &year, Aliases: []string{}, SourceIDs: map[string]string{},
				MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
			if _, err := database.Insert(tx, "titles", ti, by); err != nil {
				return err
			}
			it := &domain.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
			album, covers := "", ""
			wantYear := year
			switch kind {
			case "show":
				se, ep := i%3+1, i%10+1
				it.Kind, it.Numbers = "episode", domain.Numbers{Season: &se, Episode: &ep}
				n := fmt.Sprintf("Ep %d", i)
				it.Name = &n
				covers = fmt.Sprintf("S%02dE%02d %s", se, ep, n)
			case "artist":
				wantYear = 0 // an artist row drops the title year unless an album gives one
				tr := i%9 + 1
				it.Kind, it.Numbers = "track", domain.Numbers{Track: &tr}
				if i%8 == 2 { // a track with an album; i%8 == 6 has no album (NULL part)
					d := time.Date(2001, 5, 1, 0, 0, 0, 0, time.UTC)
					pt := &domain.Part{TitleID: ti.ID, Kind: "album", Name: fmt.Sprintf("Album %d", i), ReleaseDate: &d, KeepUpToDate: true,
						SourceIDs: map[string]string{}, Pressings: []domain.Pressing{}, SecondaryTypes: []string{}}
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
			ver := &domain.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: name, IsMain: true, KeepUpToDate: true}
			if _, err := database.Insert(tx, "versions", ver, domain.System); err != nil {
				return err
			}
			state := domain.SlotDownloading
			if i%10 == 5 {
				state = domain.SlotNeedsADecision
			}
			sl := &domain.Slot{ItemID: it.ID, VersionID: ver.ID, State: state}
			if state == domain.SlotNeedsADecision {
				r := "the download failed and no other Release passed the Pick rules"
				sl.LastReason = &r
			}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			pw := &dlPinWant{title: name, titleID: ti.ID, version: "HD", kind: kind, year: wantYear, titleYear: year, owner: owner, album: album, covers: covers, decision: state == domain.SlotNeedsADecision}
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
	Downloads []DownloadRow  `json:"downloads"`
	Totals    DownloadTotals `json:"totals"`
}

func dlPinCheckRow(t *testing.T, r DownloadRow, w *dlPinWant, manager bool) {
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

// Characterization: the Downloads view over a large library. The newest 500
// rows come back in id-descending order; Release, Version, album and owner
// partners are NULL for some rows and those rows keep their shape.
// route: GET /api/v1/downloads/view
func TestDlPinDownloadsViewLargeLibrary(t *testing.T) {
	w := newDlPinWorld(t)
	st, b := w.get(t, w.owner, "/api/v1/downloads/view?limit=500")
	if st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	var v dlPinView
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Downloads) != 500 {
		t.Fatalf("%d rows, want 500", len(v.Downloads))
	}
	for i, r := range v.Downloads {
		wantID := w.order[len(w.order)-1-i]
		if r.ID != wantID {
			t.Fatalf("row %d is download %d, want %d (newest first)", i, r.ID, wantID)
		}
		dlPinCheckRow(t, r, w.want[r.ID], true)
	}
	// Default page is 100 rows.
	_, b = w.get(t, w.owner, "/api/v1/downloads/view")
	var d dlPinView
	_ = json.Unmarshal(b, &d)
	if len(d.Downloads) != 100 {
		t.Fatalf("default page %d rows, want 100", len(d.Downloads))
	}
	// Totals count the downloading speed of every row.
	if v.Totals.Speed != 500*100 {
		t.Fatalf("totals %+v", v.Totals)
	}
}

// Characterization: a member sees only the downloads of titles they asked
// for, without manager fields (Source, Requested by, Path).
// route: GET /api/v1/downloads/view
func TestDlPinDownloadsViewMemberFilter(t *testing.T) {
	w := newDlPinWorld(t)
	st, b := w.get(t, w.member, "/api/v1/downloads/view?limit=500")
	if st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	var v dlPinView
	_ = json.Unmarshal(b, &v)
	want := 0
	for _, pw := range w.want {
		if pw.asked {
			want++
		}
	}
	if want != dlPinLib/5 || len(v.Downloads) != want {
		t.Fatalf("member sees %d rows, want %d", len(v.Downloads), want)
	}
	prev := int64(1 << 62)
	for _, r := range v.Downloads {
		pw := w.want[r.ID]
		if pw == nil || !pw.asked {
			t.Fatalf("member sees download %d that is not theirs", r.ID)
		}
		if r.ID >= prev {
			t.Fatalf("rows not newest first at %d", r.ID)
		}
		prev = r.ID
		dlPinCheckRow(t, r, pw, false)
	}
	// ?asked=1 is for people who approve; a member still gets their own rows.
	_, b = w.get(t, w.member, "/api/v1/downloads/view?asked=1")
	var a dlPinView
	_ = json.Unmarshal(b, &a)
	if len(a.Downloads) != want {
		t.Fatalf("member ?asked=1 sees %d rows, want %d", len(a.Downloads), want)
	}
	// ?asked=1 for the owner reads every download, with no row cap.
	_, b = w.get(t, w.owner, "/api/v1/downloads/view?asked=1")
	var o dlPinView
	_ = json.Unmarshal(b, &o)
	if len(o.Downloads) != want {
		// asked=1 narrows to titles with an approved ask (the Requests page).
		t.Fatalf("owner ?asked=1 sees %d rows, want %d", len(o.Downloads), want)
	}
}

// Characterization: Needs a decision over a large library: the slots in that
// state, newest first, with the version count, the Release name (NULL for
// downloads without a Release), the album (NULL part) and the episode line.
// route: GET /api/v1/needs-decision
func TestDlPinNeedsDecisionLargeLibrary(t *testing.T) {
	w := newDlPinWorld(t)
	st, b := w.get(t, w.owner, "/api/v1/needs-decision")
	if st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	var out struct {
		Items []DecisionView `json:"items"`
		Count int            `json:"count"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	wantN := 0
	for _, pw := range w.want {
		if pw.decision {
			wantN++
		}
	}
	if wantN != dlPinLib/10 || out.Count != wantN || len(out.Items) != wantN {
		t.Fatalf("count %d items %d, want %d", out.Count, len(out.Items), wantN)
	}
	ids := make([]int64, 0, wantN)
	for _, it := range out.Items {
		pw := w.want[*it.DownloadID]
		if pw == nil || !pw.decision {
			t.Fatalf("item %+v is not a decision row", it)
		}
		ids = append(ids, it.SlotID)
		if it.TitleID != pw.titleID || it.Title != pw.title || it.Version != "HD" || it.Versions != 1 || it.Kind != pw.kind || it.Why != "ran_out" {
			t.Errorf("item %+v, want %+v", it, pw)
		}
		if !strings.HasPrefix(it.Reason, "The download failed") || !strings.HasSuffix(it.Reason, ".") || it.HasCopy || it.Since.IsZero() {
			t.Errorf("item reason/copy/since: %+v", it)
		}
		if pw.hasRelease && (it.ReleaseID == nil || it.ReleaseName != pw.name) || !pw.hasRelease && (it.ReleaseID != nil || it.ReleaseName != "") {
			t.Errorf("item release %v %q, want has=%v %q", it.ReleaseID, it.ReleaseName, pw.hasRelease, pw.name)
		}
		if it.Album != pw.album {
			t.Errorf("item album %q, want %q", it.Album, pw.album)
		}
		if (pw.kind == "show" || pw.kind == "artist") != (it.Episode != nil) {
			t.Errorf("item %d episode line %v for kind %s", it.SlotID, it.Episode, pw.kind)
		}
		if it.Year == nil || *it.Year != pw.titleYear {
			t.Errorf("item %d year %v, want %d", it.SlotID, it.Year, pw.titleYear)
		}
	}
	// Order is updated_at DESC, id DESC; all rows were made in one run, so
	// ties are common and the id breaks them.
	if !sort.SliceIsSorted(ids, func(i, j int) bool { return ids[i] > ids[j] }) {
		// updated_at may differ by milliseconds, so only the set is pinned then.
		seen := map[int64]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		if len(seen) != wantN {
			t.Fatalf("duplicate slots in the decision list")
		}
	}
}

// Characterization: the title owner name (users joined to titles by the
// created_by json) in the client detail route and in the view.
// route: GET /api/v1/downloads/view
func TestDlPinRowOwnersNullPartner(t *testing.T) {
	w := newDlPinWorld(t)
	var n, owned int
	_, b := w.get(t, w.owner, "/api/v1/downloads/view?limit=500")
	var v dlPinView
	_ = json.Unmarshal(b, &v)
	for _, r := range v.Downloads {
		n++
		if r.RequestedBy == "ana" {
			owned++
		}
		// A system-made title has no requester, and a HolyArrs row never
		// says "Added in <client>" (that is for outside rows).
		if r.RequestedBy != "" && r.RequestedBy != "ana" {
			t.Fatalf("download %d requested by %q", r.ID, r.RequestedBy)
		}
	}
	// Every third title was added by ana: of the newest 500 ids.
	want := 0
	for i := dlPinLib - 500; i < dlPinLib; i++ {
		if i%3 == 0 {
			want++
		}
	}
	if owned != want {
		t.Fatalf("%d of %d rows owned by ana, want %d", owned, n, want)
	}
}

// Route pins: status and auth level for the member and the signed-out caller.
// route: GET /api/v1/downloads/view
// route: GET /api/v1/downloads/speeds
// route: GET /api/v1/needs-decision
// route: POST /api/v1/needs-decision/{id}/decide
// route: POST /api/v1/downloads/{id}/remove
// route: GET /api/v1/downloaders
// route: GET /api/v1/tags
func TestDlPinDownloadRouteAccess(t *testing.T) {
	w := newDlPinWorld(t)
	type probe struct {
		method, path string
		member       int // status for a signed-in member
		owner        int // status for the owner
	}
	probes := []probe{
		{"GET", "/api/v1/downloads/view", 200, 200},
		{"GET", "/api/v1/downloads/speeds", 403, 200},
		{"GET", "/api/v1/needs-decision", 403, 200},
		{"POST", "/api/v1/needs-decision/1/decide", 403, 404},
		{"POST", "/api/v1/downloads/1/remove", 403, 0}, // owner outcome depends on the client; checked below
		{"GET", "/api/v1/downloaders", 403, 200},
		{"GET", "/api/v1/tags", 403, 200},
		{"GET", "/api/v1/downloads", 403, 200},
		{"GET", "/api/v1/downloads/clients", 403, 200},
		{"GET", "/api/v1/downloads/matches", 403, 200},
		{"GET", "/api/v1/turned-down", 403, 200},
		{"GET", "/api/v1/pick-rules", 403, 200},
		{"GET", "/api/v1/own-traits", 403, 200},
		{"GET", "/api/v1/settings/downloaders", 403, 200},
		{"PUT", "/api/v1/settings/downloaders", 403, 0},
		{"POST", "/api/v1/downloaders", 403, 0},
		{"GET", "/api/v1/downloaders/1/attempts", 403, 0},
	}
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, p := range probes {
		body := "{}"
		if st, b := w.send(t, w.member, w.mcsrf, p.method, p.path, body); st != p.member {
			t.Errorf("member %s %s: %d, want %d: %s", p.method, p.path, st, p.member, b)
		}
		if st, _ := w.send(t, anon, "", p.method, p.path, body); st != 401 && st != 403 {
			t.Errorf("signed out %s %s: %d, want 401 or 403", p.method, p.path, st)
		}
		if p.owner != 0 {
			if st, b := w.send(t, w.owner, w.csrf, p.method, p.path, body); st != p.owner {
				t.Errorf("owner %s %s: %d, want %d: %s", p.method, p.path, st, p.owner, b)
			}
		}
	}
	// Unknown ids answer 404 to the owner.
	if st, b := w.send(t, w.owner, w.csrf, "POST", "/api/v1/needs-decision/999999/decide", `{"action":"search"}`); st != 404 {
		t.Errorf("decide unknown: %d %s", st, b)
	}
}

// Response shape of the view: the top-level keys and one row's keys.
// route: GET /api/v1/downloads/view
func TestDlPinDownloadsViewShape(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	var top map[string]json.RawMessage
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &top)
	keys := []string{}
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if got := strings.Join(keys, ","); got != "clients,downloads,next_cursor,outside_total,totals" {
		t.Fatalf("top-level keys %s", got)
	}
	var rows []map[string]any
	_ = json.Unmarshal(top["downloads"], &rows)
	if len(rows) != 1 {
		t.Fatalf("%d rows", len(rows))
	}
	rk := []string{}
	for k := range rows[0] {
		rk = append(rk, k)
	}
	sort.Strings(rk)
	const want = "active,can_act,downloader,downloader_id,downloader_kind,eta,hash,id,kind,label,live_key,name,peers,phase,progress,protocol,seeds,size,slot_state,source_id,source_name,speed,state,text,title,title_id,up_speed,updated_at,version,year"
	if got := strings.Join(rk, ","); got != want {
		t.Fatalf("row keys\n got %s\nwant %s", got, want)
	}
}

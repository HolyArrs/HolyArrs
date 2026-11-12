package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"runtime/metrics"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const (
	ownHash   = "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"
	outHash   = "1111111111111111111111111111111111111111"
	seedHash  = "2222222222222222222222222222222222222222"
	pauseHash = "3333333333333333333333333333333333333333"
)

type allClient struct {
	webapi string // app/webapiVersion; "" answers 2.15.1
	mu     sync.Mutex
	calls  []string
}

func (f *allClient) saw() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, " ")
}

func (f *allClient) server(t *testing.T) *httptest.Server {
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		path := strings.TrimPrefix(r.URL.Path, "/api/v2/")
		f.mu.Lock()
		f.calls = append(f.calls, path+"?"+r.Form.Encode())
		f.mu.Unlock()
		switch path {
		case "torrents/info":
			if r.Form.Get("category") != "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[
{"hash":"` + strings.ToUpper(ownHash) + `","name":"Salt.Choir","category":"holyarrs","size":1000,"progress":0.3,"state":"downloading","added_on":1700000000},
{"hash":"` + outHash + `","name":"Linux.ISO","category":"linux","size":5000,"progress":0.5,"dlspeed":100,"upspeed":5,"num_seeds":3,"num_leechs":4,"eta":60,"state":"downloading","added_on":1700000100},
{"hash":"` + seedHash + `","name":"Old.Album","category":"music","size":300,"progress":1,"upspeed":9,"state":"uploading","ratio":1.5},
{"hash":"` + pauseHash + `","name":"Done.Thing","category":"","size":200,"progress":1,"state":"stoppedUP"}]`))
		case "torrents/properties":
			_, _ = w.Write([]byte(`{"total_size":5000,"total_downloaded":2500,"seeds":3,"seeds_total":9,"save_path":"/data/linux"}`))
		case "torrents/files":
			_, _ = w.Write([]byte(`[{"index":0,"name":"linux.iso","size":5000,"progress":0.5,"priority":1}]`))
		case "torrents/trackers":
			_, _ = w.Write([]byte(`[{"url":"http://t.lab/announce?passkey=x","tier":0,"status":2}]`))
		case "app/preferences":
			_, _ = w.Write([]byte(`{"save_path":"/data/downloads"}`))
		case "torrents/categories":
			_, _ = w.Write([]byte(`{"linux":{"name":"linux","savePath":"/data/linux"},"music":{"name":"music","savePath":""}}`))
		case "sync/torrentPeers":
			_, _ = w.Write([]byte(`{"peers":{}}`))
		case "app/webapiVersion":
			v := f.webapi
			if v == "" {
				v = "2.15.1"
			}
			_, _ = w.Write([]byte(v))
		}
	}))
	t.Cleanup(qb.Close)
	return qb
}

func countDownloads(t *testing.T, s *database.Store) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT COUNT(*) FROM downloads").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// dlStreams opens /live streams on the API's hub as a manager (uid 0) or
// as non-manager people, without the sign-in layer.
func dlStreams(t *testing.T, ctx context.Context, hub *webserver.LiveHub, uids ...int64) (map[int64]<-chan liveFrame, func(uid int64, last string) <-chan liveFrame) {
	t.Helper()
	srv := httptest.NewServer(hub.Handler(func(r *http.Request) webserver.LiveViewer {
		id, _ := strconv.ParseInt(r.URL.Query().Get("u"), 10, 64)
		return webserver.LiveViewer{UserID: id, All: id == 0}
	}))
	t.Cleanup(srv.Close)
	x := liveSetup{srv: srv}
	out := map[int64]<-chan liveFrame{}
	for _, id := range uids {
		out[id] = x.streamURL(t, ctx, srv.URL+"/live?u="+strconv.FormatInt(id, 10))
	}
	for hub.Clients() != len(uids) {
		time.Sleep(5 * time.Millisecond)
	}
	resume := func(uid int64, last string) <-chan liveFrame {
		return x.streamURL(t, ctx, srv.URL+"/live?u="+strconv.FormatInt(uid, 10)+"&last="+last)
	}
	return out, resume
}

func dlPatches(t *testing.T, fs []liveFrame) []downloads.Patch {
	t.Helper()
	var out []downloads.Patch
	for _, f := range fs {
		if f.topic != webserver.LiveTopicDL {
			continue
		}
		var p downloads.Patch
		if err := json.Unmarshal([]byte(f.data), &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func manageBase(t *testing.T) (x *dl41, f *allClient, base string) {
	t.Helper()
	x = seed41(t, mediafiles.SlotDownloading, "downloading")
	f = &allClient{}
	exec41(t, x.s, "UPDATE downloaders SET url = ?", f.server(t).URL)
	var did int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders").Scan(&did)
	return x, f, x.srv.URL + "/api/v1/downloaders/" + dlItoa(did)
}

func fetch(t *testing.T, url string) string {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func gzipLen(s string) int {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return buf.Len()
}

// cpuSeconds is the process's Go CPU time (user + GC), from runtime/metrics.
func cpuSeconds() float64 {
	s := []metrics.Sample{{Name: "/cpu/classes/total:cpu-seconds"}, {Name: "/cpu/classes/idle:cpu-seconds"}}
	metrics.Read(s)
	return s[0].Value.Float64() - s[1].Value.Float64()
}

// realisticTorrents is n outside torrents with the fields a real client
// sends (long names, tracker, speeds, limits), 10% downloading.
func realisticTorrents(n int) string {
	parts := make([]string, n)
	for i := range n {
		state, prog, dl := "uploading", 1.0, 0
		if i%10 == 0 {
			state, prog, dl = "downloading", 0.37, 2_400_000
		}
		parts[i] = fmt.Sprintf(`{"hash":"%040x","name":"Some.Show.Name.S%02dE%02d.1080p.WEB-DL.DDP5.1.H.264-GROUP%d","category":"tv-sonarr","save_path":"/data/downloads/complete/tv","content_path":"/data/downloads/complete/tv/Some.Show.Name.S%02dE%02d","size":%d,"progress":%g,"dlspeed":%d,"upspeed":%d,"num_seeds":%d,"num_leechs":%d,"num_complete":120,"num_incomplete":14,"eta":%d,"state":"%s","added_on":%d,"ratio":1.73,"completion_on":%d,"priority":0,"tracker":"https://tracker.example.org:443/announce","trackers_count":2,"dl_limit":0,"up_limit":0,"last_activity":%d,"seeding_time":86400,"seen_complete":%d,"uploaded":%d,"time_active":90000,"ratio_limit":-2,"seeding_time_limit":-2,"inactive_seeding_time_limit":-2}`,
			i+1, i%30+1, i%24+1, i, i%30+1, i%24+1, 1_500_000_000+int64(i)*1000, prog, dl, 30_000+i, i%20, i%7, 8640000, state, 1700000000+i, 1700003600+i, 1730000000+i, 1730000000+i, 2_500_000_000+int64(i))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

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
	srv := httptest.NewServer(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "test", Auth: au, Secrets: v,
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
				pw.name = name // no Release and no files seen: the row is named by its title
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

// viewAs reads the Downloads rows as one person, past the sign-in.
func viewAs(t *testing.T, x *dl41, who webserver.Caller) []downloads.DownloadRow {
	t.Helper()
	return viewAt(t, x, who, "/api/v1/downloads/view")
}

// viewAt is viewAs at a given address (?asked=1 for the Requests page).
func viewAt(t *testing.T, x *dl41, who webserver.Caller, url string) []downloads.DownloadRow {
	t.Helper()
	a := buildHandlers(services{Store: x.s, Queue: x.q, Live: webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second}), Metadata: metadata.Lab(), Version: "test"})
	r := httptest.NewRequest("GET", url, nil)
	r = r.WithContext(webserver.ContextWith(r.Context(), who))
	w := httptest.NewRecorder()
	a.DownloadsWeb().DownloadsView(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Downloads []downloads.DownloadRow `json:"downloads"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.Downloads
}

// get answers one GET for p, whatever its status.
func (s *signedIn) get(path string) (int, string) {
	s.t.Helper()
	req, _ := http.NewRequest("GET", s.base+path, nil)
	resp, err := s.c.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// openStream opens /live for u and says when the server ends it.
func (x liveSetup) openStream(t *testing.T, ctx context.Context, u liveUser) <-chan struct{} {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, x.srv.URL+"/live", nil)
	resp, err := u.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("live: %d", resp.StatusCode)
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		close(done)
	}()
	return done
}

func (x liveSetup) waitClients(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for x.hub.Clients() != n {
		if time.Now().After(deadline) {
			t.Fatalf("clients %d, want %d", x.hub.Clients(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type liveUser struct {
	c    *http.Client
	csrf string
	id   int64
}

type liveSetup struct {
	srv  *httptest.Server
	hub  *webserver.LiveHub
	sy   *downloads.Sync
	own  liveUser
	mem  liveUser
	mem2 liveUser
	st   *database.Store
}

func (x liveSetup) call(t *testing.T, u liveUser, method, path, body string, csrf bool) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, x.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf {
		req.Header.Set(accounts.CSRFHeader, u.csrf)
	}
	c := u.c
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (x liveSetup) stream(t *testing.T, ctx context.Context, u liveUser, last string) <-chan liveFrame {
	t.Helper()
	path := x.srv.URL + "/live"
	if last != "" {
		path += "?last=" + last
	}
	return x.streamWith(t, ctx, u.c, path)
}

// streamURL opens a stream at url without a signed-in client.
func (x liveSetup) streamURL(t *testing.T, ctx context.Context, url string) <-chan liveFrame {
	t.Helper()
	return x.streamWith(t, ctx, http.DefaultClient, url)
}

// rdpSend sends the call as one person and returns status and sorted JSON names.
func (r *askRig) rdpSend(t *testing.T, who *signedIn, req string) (int, string) {
	t.Helper()
	m, rest, _ := strings.Cut(req, " ")
	path, body, _ := strings.Cut(rest, " ")
	hr, _ := http.NewRequest(m, r.srv.URL+path, strings.NewReader(body))
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set(accounts.CSRFHeader, who.csrf)
	resp, err := who.c.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var x map[string]any
	_ = json.Unmarshal(b, &x)
	var ks []string
	for k := range x {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return resp.StatusCode, strings.Join(ks, " ")
}

func dlItoa(n int64) string { return strconv.FormatInt(n, 10) }

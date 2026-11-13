//go:build lab

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// The last beta lab checks (lossless protection is in
// labbeta43_test.go), from a throwaway container in the lab network with
// the lab downloads folder mounted (labclients.py run beta):
//
//	HOLYARRS_LAB_SELF=<container> go test -tags lab -run 'TestLabBeta' ./cmd/holyarrs/
//
// Every test runs the app as cmd/holyarrs wires it (search, judge, grab,
// Downloader sync, import) against the lab fake indexer, our own Dune test
// torrent (seeded by qbt-seed) and our own small torrents made here, never
// anything else. Torrents, payloads and Library copies are removed after.

const (
	labDuneHash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"
	labDune1080 = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"
	labDuneSize = 29588180
	labDune720  = "Dune.Part.Two.2024.720p.WEB.H264-HOLYARRS"
)

// labHere is where this container sees a path the lab qBittorrent names
// under /data/downloads (HOLYARRS_LAB_DL_HERE; the same path by default).
func labHere(p string) string {
	if h := os.Getenv("HOLYARRS_LAB_DL_HERE"); h != "" {
		return h + strings.TrimPrefix(p, "/data/downloads")
	}
	return p
}

// labFeedItem is one listing the feed adds to the fake indexer's answer.
type labFeedItem struct {
	name, link, hash string
	size             int64
}

func (it labFeedItem) xml(pub string) string {
	h := ""
	if it.hash != "" {
		h = fmt.Sprintf(`<torznab:attr name="infohash" value="%s"/>`, it.hash)
	}
	return fmt.Sprintf(`<item><title>%s</title><guid isPermaLink="false">%s</guid><pubDate>%s</pubDate><size>%d</size><link>%s</link>`+
		`<category>2000</category><category>2040</category><enclosure url="%s" length="%d" type="application/x-bittorrent"/>`+
		`<torznab:attr name="category" value="2000"/><torznab:attr name="seeders" value="5"/><torznab:attr name="peers" value="5"/>`+
		`<torznab:attr name="size" value="%d"/>%s</item>`, it.name, it.link, pub, it.size, it.link, it.link, it.size, it.size, h)
}

// labFeed answers as the lab fake indexer does, with three test changes:
// our Dune test file's real size (the fake indexer lists 8.4 GB, which the
// fake check rightly refuses), only the listings in keep (all when keep is
// nil), and the listings in extra. fresh dates every listing now, so a wait
// counts from the search.
type labFeed struct {
	mu    sync.Mutex
	keep  map[string]bool
	extra []labFeedItem
	fresh bool
	URL   string
}

var labItemRe = regexp.MustCompile(`(?s)<item>.*?</item>`)
var labTitleRe = regexp.MustCompile(`<title>([^<]*)</title>`)
var labPubRe = regexp.MustCompile(`<pubDate>[^<]*</pubDate>`)

func newLabFeed(t *testing.T) *labFeed {
	t.Helper()
	up := os.Getenv("HOLYARRS_LAB_SOURCE")
	if up == "" {
		up = "http://fake-indexer/api"
	}
	f := &labFeed{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := up
		if r.URL.RawQuery != "" {
			u += "?" + r.URL.RawQuery
		}
		resp, err := http.Get(u)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		out := string(b)
		if r.URL.Query().Get("t") != "caps" {
			f.mu.Lock()
			pub := time.Now().UTC().Format(time.RFC1123Z)
			out = labItemRe.ReplaceAllStringFunc(out, func(it string) string {
				m := labTitleRe.FindStringSubmatch(it)
				if m == nil || (f.keep != nil && !f.keep[m[1]]) {
					return ""
				}
				if m[1] == labDune1080 {
					it = strings.ReplaceAll(it, "8400000000", fmt.Sprint(labDuneSize))
				}
				if f.fresh {
					it = labPubRe.ReplaceAllString(it, "<pubDate>"+pub+"</pubDate>")
				}
				return it
			})
			var add strings.Builder
			for _, it := range f.extra {
				add.WriteString(it.xml(pub))
			}
			f.mu.Unlock()
			out = strings.Replace(out, "</channel>", add.String()+"</channel>", 1)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	f.URL = srv.URL + "/api"
	return f
}

// set says which fake indexer listings stay and which listings are added.
func (f *labFeed) set(keep []string, extra ...labFeedItem) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if keep == nil {
		f.keep = nil
	} else {
		f.keep = map[string]bool{}
		for _, k := range keep {
			f.keep[k] = true
		}
	}
	f.extra = extra
}

// labOwnMovie makes our own single-file Dune torrent named name, serves it
// to the lab qBittorrent from this test and puts its payload where that
// qBittorrent will save it, so it finishes from what it has. It returns the
// listing for the feed.
func labOwnMovie(t *testing.T, cat, name string, seed byte) labFeedItem {
	t.Helper()
	return labOwnFile(t, cat, name, name+".mkv", seed)
}

// labOwnFile is labOwnMovie with the file inside named file.
func labOwnFile(t *testing.T, cat, name, file string, seed byte) labFeedItem {
	t.Helper()
	// 24 MB: over the fake check's floor for a video of unknown length (20 MB).
	files := []labFile{{name: file, data: video(seed, 24<<20)}}
	tor, hash := labTorrent("", files)
	base := labServe(t, map[string][]byte{"own-" + fmt.Sprint(seed) + ".torrent": tor})
	dir := labHere(filepath.Join("/data/downloads/incomplete", cat))
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, files[0].name), files[0].data, 0o666); err != nil {
		t.Fatal(err)
	}
	return labFeedItem{name: name, link: base + "/own-" + fmt.Sprint(seed) + ".torrent", hash: hash, size: int64(len(files[0].data))}
}

// labMovie is Dune: Part Two with one movie Slot in a Version, a Source,
// the lab qBittorrent, and the job queue wired as cmd/holyarrs wires it.
type labMovie struct {
	s      *database.Store
	srv    *httptest.Server
	q      *jobs.Queue
	m      *downloads.Manager
	lib    *library.Service
	vault  *secrets.Vault
	cat    string
	movies string
	// mayFail names Releases whose Download is meant to fail on the way.
	mayFail map[string]bool
	title   int64
	slot    int64
	ver     int64
}

type labMovieOpts struct {
	cat     string
	movies  string                          // the Library folder; a temp folder when ""
	feed    string                          // the Source (Torznab); none when ""
	rules   func(*mediafiles.PickRulesBody) // changes to Standard
	pathMap []domain.PathMap
	notOut  bool // the digital release is a month away: the gate is closed
}

func newLabMovie(t *testing.T, o labMovieOpts) *labMovie {
	t.Helper()
	ctx := context.Background()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	labNoSizeFloors(t, s)
	vault, err := secrets.Open(ctx, s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	x := &labMovie{s: s, vault: vault, cat: o.cat, movies: o.movies}
	if x.movies == "" {
		x.movies = t.TempDir()
	} else {
		if err := os.MkdirAll(x.movies, 0o777); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(x.movies) })
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		body := mediafiles.GoodEveryday()
		if o.rules != nil {
			o.rules(&body)
		}
		pr := &mediafiles.PickRules{Name: "Lab " + t.Name(), Kinds: []string{"movie"}, Revision: 1, Body: body}
		if _, err := database.Insert(tx, "pick_rules", pr, domain.System); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: x.movies, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		y := 2024
		digital := time.Date(2024, 4, 16, 0, 0, 0, 0, time.UTC)
		state, status := mediafiles.SlotWanted, "released"
		if o.notOut {
			digital, state, status = time.Now().UTC().AddDate(0, 1, 0), mediafiles.SlotNotOutYet, "announced"
		}
		ti := &mediafiles.Title{Kind: "movie", Name: "Dune: Part Two", SortName: "dune part two", Year: &y, Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{},
			WhyAdded: "by_hand", Artwork: []string{}, Status: status, State: "active", KeepUpToDate: true,
			DigitalDate: &digital, GateDriver: "digital", GateOpensAt: &digital}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr.ID, LibraryFolderID: lf.ID, FolderName: "Dune Part Two (2024)", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: state}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		x.title, x.slot, x.ver = ti.ID, sl.ID, v.ID
		if o.feed != "" {
			so := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: o.feed, AllowLocal: true,
				UseFor: domain.SourceUse{Feed: true, AutoSearch: true, HandSearch: true}, Kinds: []string{"movie"}, Categories: map[string][]int{"movie": {2000}},
				Priority: 25, State: "enabled", Extra: map[string]string{}}
			if _, err := database.Insert(tx, "sources", so, domain.System); err != nil {
				return err
			}
		}
		pm := o.pathMap
		if pm == nil {
			pm = []domain.PathMap{}
		}
		d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: labQbit(), Category: o.cat, Priority: 1,
			PathMap: pm, AfterImport: "leave", State: "ok", AllowLocal: true}
		_, err := database.Insert(tx, "downloaders", d, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// Wired as cmd/holyarrs wires it.
	qctx, cancel := context.WithCancel(ctx)
	x.q = jobs.New(s, jobs.Options{Workers: 2, BaseBackoff: time.Hour, PollInterval: 50 * time.Millisecond})
	md := metadata.Lab()
	x.lib = &library.Service{Store: s, Queue: x.q, Metadata: md}
	x.m = &downloads.Manager{Store: s, SavePath: filepath.Join("/data/downloads/incomplete", o.cat)}
	x.m.Link = func(ctx context.Context, rel domain.Release) (string, error) {
		return sources.GrabLink(ctx, s, vault, rel)
	}
	im := &importer.Importer{Store: s}
	q := x.q
	sr := &sources.Searcher{Store: s, Queue: q, Secrets: vault, OnChosen: func(ctx context.Context, slotID, releaseID int64) {
		_, _, _ = q.Enqueue(ctx, downloads.KindGrab, &domain.Subject{Type: "slot", ID: slotID}, map[string]any{"release_id": releaseID}, downloads.GrabKey(slotID, releaseID), domain.System)
	}}
	x.m.OnFailed = func(ctx context.Context, slotID int64) { _ = sr.QueueAfterFailure(ctx, slotID, domain.System) }
	x.m.OnFinished = func(ctx context.Context, id int64) {
		_, _, _ = q.Enqueue(ctx, importer.KindImport, &domain.Subject{Type: "download", ID: id}, nil, fmt.Sprintf("import:%d", id), domain.System)
	}
	sr.Register(q)
	q.Handle(downloads.KindGrab, x.m.HandleGrab)
	q.Handle(importer.KindImport, im.HandleJob)
	if err := q.Start(qctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); q.Stop() })
	t.Cleanup(func() { _ = os.RemoveAll(labHere(filepath.Join("/data/downloads/incomplete", o.cat))) })
	x.srv = httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: md, Version: "lab", Downloads: x.m, Queue: q, Library: x.lib, Secrets: vault, Search: sr}), config{}))
	t.Cleanup(x.srv.Close)
	return x
}

// remove takes the given torrents out of the lab qBittorrent afterwards.
func (x *labMovie) remove(t *testing.T, hashes ...string) {
	t.Cleanup(func() {
		if e, err := x.m.EngineFor(context.Background(), domain.Downloader{Adapter: "qbittorrent", URL: labQbit(), AllowLocal: true}); err == nil {
			for _, h := range hashes {
				_ = e.Remove(context.Background(), h, true)
			}
		}
	})
}

// waitJob waits for a Job to end and returns its state and result.
func (x *labMovie) waitJob(t *testing.T, id int64) (string, string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		var st, res string
		_ = x.s.Read().QueryRow("SELECT state, COALESCE(result, '') || ' ' || COALESCE(error, '') FROM jobs WHERE id = ?", id).Scan(&st, &res)
		if st == "done" || st == "failed" || st == "dead" {
			return st, res
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %d still %s: %s", id, st, x.dump())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// autoSearch runs the search the app runs by itself (not by hand).
func (x *labMovie) autoSearch(t *testing.T) string {
	t.Helper()
	id, _, err := x.q.Enqueue(context.Background(), jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: x.slot}, nil, fmt.Sprintf("search_slot:%d:%d", x.slot, time.Now().UnixNano()), domain.System)
	if err != nil {
		t.Fatal(err)
	}
	st, res := x.waitJob(t, id)
	if st != "done" {
		t.Fatalf("search %s: %s", st, res)
	}
	return res
}

// handSearch is Search on the title page; choose false only shows the Releases.
func (x *labMovie) handSearch(t *testing.T, choose bool) {
	t.Helper()
	var out struct {
		JobIDs []int64 `json:"job_ids"`
	}
	_ = json.Unmarshal(do(t, "POST", x.srv.URL+"/api/v1/titles/"+labItoa(x.title)+"/search", fmt.Sprintf(`{"slot_id":%d,"choose":%v}`, x.slot, choose), 202), &out)
	if len(out.JobIDs) == 0 {
		t.Fatal("no search job")
	}
	for _, id := range out.JobIDs {
		if st, res := x.waitJob(t, id); st != "done" {
			t.Fatalf("hand search %s: %s", st, res)
		}
	}
}

type labRel struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	State      string     `json:"state"`
	FoundBy    string     `json:"found_by"`
	SourceName string     `json:"source_name"`
	Outcome    string     `json:"outcome"`
	Rank       int        `json:"rank"`
	Step       string     `json:"step"`
	NeedsForce bool       `json:"needs_force"`
	HoldUntil  *time.Time `json:"hold_until"`
	Reasons    []struct {
		Code   string `json:"code"`
		Effect string `json:"effect"`
		Text   string `json:"text"`
	} `json:"reasons"`
}

func (r labRel) has(code string) string {
	for _, x := range r.Reasons {
		if x.Code == code {
			return x.Text
		}
	}
	return ""
}

// releases is the Releases list on the title page for the Slot.
func (x *labMovie) releases(t *testing.T) map[string]labRel {
	t.Helper()
	var out struct {
		Releases []labRel `json:"releases"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/titles/"+labItoa(x.title)+"/releases?slot_id="+labItoa(x.slot)+"&limit=100", "", 200), &out)
	m := map[string]labRel{}
	for _, r := range out.Releases {
		m[r.Name] = r
	}
	return m
}

func (x *labMovie) slotState() string {
	var st string
	_ = x.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", x.slot).Scan(&st)
	return st
}

// placed is the Slot's File once it is in the Library.
func (x *labMovie) placed() (mediafiles.File, bool) {
	var f mediafiles.File
	var fid sql.NullInt64
	_ = x.s.Read().QueryRow("SELECT file_id FROM slots WHERE id = ? AND state IN ('in_library', 'better_wanted')", x.slot).Scan(&fid)
	if !fid.Valid {
		return f, false
	}
	if err := database.Get(context.Background(), x.s.Read(), "files", fid.Int64, &f); err != nil {
		return f, false
	}
	return f, f.State == "present"
}

// untilPlaced syncs the Downloader every second until the Slot's file is in
// the Library and its row says Done.
func (x *labMovie) untilPlaced(t *testing.T) (mediafiles.File, downloads.DownloadRow) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Minute)
	var phases []string
	for {
		_, _ = x.m.SyncOnce(context.Background())
		rows := labRows(t, x.srv.URL)
		for _, r := range rows {
			if r.Active || len(rows) == 1 {
				if len(phases) == 0 || phases[len(phases)-1] != r.Phase {
					phases = append(phases, r.Phase)
				}
				switch r.Phase {
				case "failed", "vanished", "import_refused", "needs_a_decision":
					if !x.mayFail[r.Name] {
						t.Fatalf("download %s: %+v; %s", r.Phase, r, x.dump())
					}
				}
			}
		}
		if f, ok := x.placed(); ok {
			for _, r := range rows {
				if r.Phase == "done" {
					t.Logf("phases %v", phases)
					return f, r
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("not placed; phases %v; %s", phases, x.dump())
		}
		time.Sleep(time.Second)
	}
}

// dump is the jobs, Releases and Slot for a failure message.
func (x *labMovie) dump() string {
	var b strings.Builder
	b.WriteString("slot " + x.slotState() + "; jobs:")
	if rows, err := x.s.Read().Query("SELECT kind || ' ' || state || ' ' || COALESCE(result, '') || ' ' || COALESCE(error, '') FROM jobs ORDER BY id"); err == nil {
		for rows.Next() {
			var j string
			_ = rows.Scan(&j)
			b.WriteString(" | " + j)
		}
		rows.Close()
	}
	b.WriteString(" || folder " + labHere(filepath.Join("/data/downloads/incomplete", x.cat)) + ":")
	if ents, err := os.ReadDir(labHere(filepath.Join("/data/downloads/incomplete", x.cat))); err == nil {
		for _, e := range ents {
			b.WriteString(" " + e.Name())
		}
	} else {
		b.WriteString(" " + err.Error())
	}
	if resp, err := http.Get(labQbit() + "/api/v2/torrents/info?category=" + x.cat); err == nil {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		b.WriteString(" || qbit: " + string(raw))
	}
	b.WriteString(" || imports:")
	if rows, err := x.s.Read().Query("SELECT state || ' from ' || COALESCE(from_path, '') || ' plan ' || COALESCE(plan, '') || ' result ' || COALESCE(result, '') FROM imports ORDER BY id"); err == nil {
		for rows.Next() {
			var r string
			_ = rows.Scan(&r)
			b.WriteString(" | " + r)
		}
		rows.Close()
	}
	b.WriteString(" || releases:")
	if rows, err := x.s.Read().Query("SELECT name || ' ' || state || ' ' || COALESCE(link_ref, '') || ' ' || COALESCE(info_hash, '-') FROM releases ORDER BY id"); err == nil {
		for rows.Next() {
			var r string
			_ = rows.Scan(&r)
			b.WriteString(" | " + labKeyless(r))
		}
		rows.Close()
	}
	return b.String()
}

var labKeyRe = regexp.MustCompile(`(?i)(apikey|jackett_apikey)=[^&\s]*`)

// labKeyless hides any key in a link before it is printed.
func labKeyless(s string) string { return labKeyRe.ReplaceAllString(s, "$1=…") }

// labDownloads is how many Downloads HolyArrs sent, by Release name.
func (x *labMovie) labDownloads() map[string]int {
	out := map[string]int{}
	if rows, err := x.s.Read().Query("SELECT r.name FROM downloads d JOIN releases r ON r.id = d.release_id"); err == nil {
		for rows.Next() {
			var n string
			_ = rows.Scan(&n)
			out[n]++
		}
		rows.Close()
	}
	return out
}

// Pick a release yourself: Search shows the lab's Dune listings with
// their verdicts and moves nothing; choosing the program behind a video name
// needs force; choosing our Dune test torrent by hand sends it, and it is
// downloaded and imported.
func TestLabBeta41PickReleaseYourself(t *testing.T) {
	feed := newLabFeed(t)
	x := newLabMovie(t, labMovieOpts{cat: "holyarrs-lab-b41", feed: feed.URL})
	x.remove(t, labDuneHash)
	x.handSearch(t, false)
	rels := x.releases(t)
	dune, ok := rels[labDune1080]
	exe, okExe := rels[labDune1080+".mkv.exe"]
	if !ok || !okExe {
		t.Fatalf("releases %v", rels)
	}
	if x.slotState() != string(mediafiles.SlotWanted) || len(x.labDownloads()) != 0 {
		t.Fatalf("showing moved the Slot (%s) or sent %v", x.slotState(), x.labDownloads())
	}
	if dune.State != "judged" || dune.FoundBy != "hand_search" || dune.Rank != 1 {
		t.Fatalf("our Dune: %+v", dune)
	}
	if exe.Outcome != "refused" || !exe.NeedsForce {
		t.Fatalf("the .exe: %+v", exe)
	}
	do(t, "POST", x.srv.URL+"/api/v1/releases/"+labItoa(exe.ID)+"/choose", fmt.Sprintf(`{"slot_id":%d}`, x.slot), 409)
	do(t, "POST", x.srv.URL+"/api/v1/releases/"+labItoa(dune.ID)+"/choose", fmt.Sprintf(`{"slot_id":%d}`, x.slot), 200)
	f, row := x.untilPlaced(t)
	if got := x.labDownloads(); len(got) != 1 || got[labDune1080] != 1 {
		t.Fatalf("sent %v", got)
	}
	var byHand int
	_ = x.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = 'release_chosen_by_hand'").Scan(&byHand)
	if byHand != 1 || x.releases(t)[labDune1080].State != "chosen" {
		t.Fatalf("chosen by hand events %d, release %+v", byHand, x.releases(t)[labDune1080])
	}
	t.Logf("#41: %d listings shown, the .exe needs force (%s), our Dune chosen by hand, imported as %s (%s); row %s", len(rels), exe.has("hard.safety:executable")+exe.has("hard.never"), f.RelativePath, f.PlacedBy, row.Label)
}

// The whole chain: a program file behind a video name, chosen by hand
// with force, is sent; when qBittorrent lists its files it fails as
// dangerous, is turned down, and the Slot is searched again, which sends our
// Dune test torrent, imported. Clearing the Turned down list afterwards
// leaves the program file refused: picking it again needs force again.
func TestLabBeta41ForcedProgramFileChain(t *testing.T) {
	const cat = "holyarrs-lab-b41c"
	// A program file behind a video name, said in the listing (as the
	// lab fake indexer's own decoy says it), with our own file behind it.
	const exeName = "Dune.Part.Two.2024.1080p.WEB.H264-LABEXE.mkv.exe"
	feed := newLabFeed(t)
	exe := labOwnFile(t, cat, exeName, exeName, 41)
	feed.set([]string{labDune1080}, exe)
	x := newLabMovie(t, labMovieOpts{cat: cat, feed: feed.URL})
	x.mayFail = map[string]bool{exeName: true}
	x.remove(t, exe.hash, labDuneHash)
	x.handSearch(t, false)
	r := x.releases(t)[exeName]
	if r.Outcome != "refused" || !r.NeedsForce || r.has("hard.safety:executable")+r.has("hard.never") == "" {
		t.Fatalf("the program file: %+v", r)
	}
	do(t, "POST", x.srv.URL+"/api/v1/releases/"+labItoa(r.ID)+"/choose", fmt.Sprintf(`{"slot_id":%d}`, x.slot), 409)
	do(t, "POST", x.srv.URL+"/api/v1/releases/"+labItoa(r.ID)+"/choose", fmt.Sprintf(`{"slot_id":%d,"force":true}`, x.slot), 200)
	f, _ := x.untilPlaced(t)
	var failed, why string
	_ = x.s.Read().QueryRow("SELECT d.state, COALESCE(d.state_reason, '') FROM downloads d JOIN releases r ON r.id = d.release_id WHERE r.name = ?", exeName).Scan(&failed, &why)
	if failed != "failed" || !strings.Contains(why, "dangerous") {
		t.Fatalf("the program file's Download is %s (%s)", failed, why)
	}
	// Turned down for this Slot: on the list (state turned_down), or judged
	// again by the next search with that as a failed reason.
	td := x.releases(t)[exeName]
	down := td.State == "turned_down"
	for _, r := range td.Reasons {
		down = down || r.Code == "hard.turned_down" && r.Effect == "fail"
	}
	if !down || !td.NeedsForce {
		t.Fatalf("the program file after failing: %+v", td)
	}
	if got := x.labDownloads(); got[exeName] != 1 || got[labDune1080] != 1 {
		t.Fatalf("sent %v", got)
	}
	if strings.Contains(f.RelativePath, ".exe") {
		t.Fatalf("placed %s", f.RelativePath)
	}
	do(t, "DELETE", x.srv.URL+"/api/v1/turned-down", "", 200)
	if r := x.releases(t)[exeName]; r.State != "refused" || !r.NeedsForce {
		t.Fatalf("after clearing the list: %+v", r)
	}
	do(t, "POST", x.srv.URL+"/api/v1/releases/"+labItoa(r.ID)+"/choose", fmt.Sprintf(`{"slot_id":%d}`, x.slot), 409)
	t.Logf("#41 chain: forced %s failed as dangerous (%s), turned down, our Dune sent next and imported as %s; after clearing the list it still needs force", exeName, why, f.RelativePath)
}

// Pick your quality: with Standard the 1080p copy ranks over our
// own 720p copy; with Pick rules that allow only 720p, the 1080p copy is
// refused as not allowed and the 720p copy is grabbed and imported.
func TestLabBeta46PickYourQuality(t *testing.T) {
	const cat = "holyarrs-lab-b46"
	feed := newLabFeed(t)
	own := labOwnMovie(t, cat, labDune720, 46)
	feed.set([]string{labDune1080}, own)

	def := newLabMovie(t, labMovieOpts{cat: cat + "-default", feed: feed.URL})
	def.handSearch(t, false)
	dr := def.releases(t)
	if dr[labDune1080].Rank != 1 || dr[labDune720].Rank != 2 || dr[labDune1080].Step != "1080p_web" || dr[labDune720].Step != "720p_web" {
		t.Fatalf("Standard: 1080p %+v, 720p %+v", dr[labDune1080], dr[labDune720])
	}

	x := newLabMovie(t, labMovieOpts{cat: cat, feed: feed.URL, rules: func(b *mediafiles.PickRulesBody) {
		for i := range b.Steps {
			b.Steps[i].Allowed = b.Steps[i].Quality == "720p_web"
		}
		b.GoodEnoughAt = "720p_web"
	}})
	x.remove(t, own.hash, labDuneHash)
	x.autoSearch(t)
	rels := x.releases(t)
	if why := rels[labDune1080].has("step.not_allowed"); rels[labDune1080].Outcome != "refused" || why == "" {
		t.Fatalf("1080p under 720p-only rules: %+v", rels[labDune1080])
	}
	if rels[labDune720].State != "chosen" {
		t.Fatalf("720p: %+v; %s", rels[labDune720], x.dump())
	}
	f, _ := x.untilPlaced(t)
	if got := x.labDownloads(); len(got) != 1 || got[labDune720] != 1 {
		t.Fatalf("sent %v", got)
	}
	if !strings.Contains(f.RelativePath, "720p") && (f.ReleaseName == nil || *f.ReleaseName != labDune720) {
		t.Fatalf("placed %s (%v)", f.RelativePath, f.ReleaseName)
	}
	t.Logf("#46: Standard ranks 1080p_web over 720p_web; 720p-only rules refuse 1080p (%s) and import %s", rels[labDune1080].has("step.not_allowed"), f.RelativePath)
}

// Wait for a better release: with a 60 minute wait the first copy found
// (our 720p) is held, not sent; a better copy found during the wait (our
// 1080p Dune) takes its place; when the wait is over the better copy is
// sent and imported, and the held one never was.
func TestLabBeta48WaitForBetter(t *testing.T) {
	const cat = "holyarrs-lab-b48"
	feed := newLabFeed(t)
	feed.fresh = true
	own := labOwnMovie(t, cat, labDune720, 48)
	feed.set([]string{}, own)
	x := newLabMovie(t, labMovieOpts{cat: cat, feed: feed.URL, rules: func(b *mediafiles.PickRulesBody) {
		b.Protocol.WaitMinutesTorrent = 60
		b.Protocol.SkipWaitAtGoodEnough = false
	}})
	x.remove(t, own.hash, labDuneHash)
	x.autoSearch(t)
	r := x.releases(t)[labDune720]
	if x.slotState() != "held" || r.State != "held" || r.HoldUntil == nil || r.HoldUntil.Before(time.Now().Add(50*time.Minute)) {
		t.Fatalf("first copy: slot %s, %+v", x.slotState(), r)
	}
	time.Sleep(3 * time.Second)
	if n := len(x.labDownloads()); n != 0 {
		t.Fatalf("sent during the wait: %v", x.labDownloads())
	}
	// A better copy shows up in the Source's feed during the wait (the app's
	// feed check, as the old apps' RSS sync, is what sees it).
	feed.set([]string{labDune1080}, own)
	fc, _, err := x.q.Enqueue(context.Background(), jobs.KindFeedCheck, nil, nil, "schedule:feed_check", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if st, res := x.waitJob(t, fc); st != "done" {
		t.Fatalf("feed check %s %s", st, res)
	}
	var held sql.NullInt64
	_ = x.s.Read().QueryRow("SELECT held_release_id FROM slots WHERE id = ?", x.slot).Scan(&held)
	rels := x.releases(t)
	var heldName, heldState, foundBy string
	_ = x.s.Read().QueryRow("SELECT name, state, found_by FROM releases WHERE id = ?", held.Int64).Scan(&heldName, &heldState, &foundBy)
	if x.slotState() != "held" || heldName != labDune1080 || heldState != "held" || rels[labDune720].State != "passed_over" || len(x.labDownloads()) != 0 {
		t.Fatalf("better copy during the wait: slot %s held %v (%s %s %s); 720p %+v; sent %v", x.slotState(), held, heldName, heldState, foundBy, rels[labDune720], x.labDownloads())
	}
	t.Logf("during the wait the feed check (%s) held %s; 720p %s (%s)", foundBy, heldName, rels[labDune720].State, rels[labDune720].has("rank.lost"))
	// The wait is over: the scheduled check chooses what was held.
	if err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET next_try_at = ? WHERE id = ?", database.FormatTime(time.Now().Add(-time.Minute)), x.slot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id, _, err := x.q.Enqueue(context.Background(), jobs.KindGrabHeld, nil, nil, "schedule:grab_held:after", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if st, res := x.waitJob(t, id); st != "done" || !strings.Contains(res, `"chosen":1`) {
		t.Fatalf("grab_held %s %s", st, res)
	}
	f, _ := x.untilPlaced(t)
	if got := x.labDownloads(); len(got) != 1 || got[labDune1080] != 1 {
		t.Fatalf("sent %v", got)
	}
	t.Logf("#48: 720p held until %s, replaced by 1080p during the wait, 1080p sent when the wait ended and imported as %s", r.HoldUntil.Format(time.RFC3339), f.RelativePath)
}

// Start when it's out: a movie whose digital release is a month away is
// not searched by the app (the Slot waits as not out yet); when the date
// passes, the gate opens, the next search grabs our Dune test torrent and it
// is imported.
func TestLabBeta52StartWhenOut(t *testing.T) {
	feed := newLabFeed(t)
	// Posted now, after the (moved) digital date: a copy posted before a
	// movie is out is refused as early, which is right.
	feed.fresh = true
	feed.set([]string{labDune1080})
	x := newLabMovie(t, labMovieOpts{cat: "holyarrs-lab-b52", feed: feed.URL, notOut: true})
	x.remove(t, labDuneHash)
	if st := x.slotState(); st != string(mediafiles.SlotNotOutYet) {
		t.Fatalf("slot %s", st)
	}
	res := x.autoSearch(t)
	if !strings.Contains(res, "not_out_yet") || len(x.releases(t)) != 0 || len(x.labDownloads()) != 0 {
		t.Fatalf("searched before it was out: %s; releases %v", res, x.releases(t))
	}
	if n, err := x.lib.OpenGates(context.Background()); err != nil || n != 0 {
		t.Fatalf("gate opened early: %d %v", n, err)
	}
	// The digital date passes.
	past := database.FormatTime(time.Now().Add(-time.Hour))
	if err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET digital_date = ?, gate_opens_at = ?, status = 'released' WHERE id = ?", past, past, x.title)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := x.lib.OpenGates(context.Background()); err != nil || n != 1 || x.slotState() != string(mediafiles.SlotWanted) {
		t.Fatalf("gate: %d %v, slot %s", n, err, x.slotState())
	}
	x.autoSearch(t)
	f, _ := x.untilPlaced(t)
	if got := x.labDownloads(); len(got) != 1 || got[labDune1080] != 1 {
		t.Fatalf("sent %v", got)
	}
	t.Logf("#52: not searched while not out (%s); gate opened, grabbed and imported as %s", strings.TrimSpace(res), f.RelativePath)
}

// Hardlinks: with the Library on the same lab volume as the downloads,
// the import links the file (same inode, two links) and qBittorrent keeps
// seeding the original.
func TestLabBeta49Hardlink(t *testing.T) {
	feed := newLabFeed(t)
	feed.set([]string{labDune1080})
	lib := labHere("/data/downloads/holyarrs-lab-b49-library")
	x := newLabMovie(t, labMovieOpts{cat: "holyarrs-lab-b49", feed: feed.URL, movies: lib})
	x.remove(t, labDuneHash)
	x.autoSearch(t)
	f, _ := x.untilPlaced(t)
	if f.PlacedBy != "hardlink" || f.FellBack {
		t.Fatalf("placed by %s (fell back %v)", f.PlacedBy, f.FellBack)
	}
	placed := filepath.Join(lib, filepath.FromSlash(f.RelativePath))
	src := labHere(filepath.Join("/data/downloads/incomplete", x.cat, labDune1080+".mkv"))
	a, err := os.Stat(placed)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, b) || labLinks(a) != 2 || a.Size() != labDuneSize {
		t.Fatalf("library %s and download %s: same file %v, links %d, size %d", placed, src, os.SameFile(a, b), labLinks(a), a.Size())
	}
	st := labQbitState(t, labDuneHash)
	if !strings.Contains(strings.ToLower(st), "up") {
		t.Fatalf("qBittorrent state %q after import, want it seeding", st)
	}
	t.Logf("#49: %s and %s are one file (inode %d, %d links); qBittorrent %s", f.RelativePath, src, labInode(a), labLinks(a), st)
}

// Folder mapping: this container sees the lab downloads folder at
// another path than qBittorrent does (HOLYARRS_LAB_DL_HERE), and the
// Library is on a second mount. The Downloader's folder mapping turns
// qBittorrent's path into ours; the file is found, copied across the
// mounts and imported.
func TestLabBeta50FolderMapping(t *testing.T) {
	here := os.Getenv("HOLYARRS_LAB_DL_HERE")
	lib := os.Getenv("HOLYARRS_LAB_OTHER_MOUNT")
	if here == "" || lib == "" {
		t.Skip("HOLYARRS_LAB_DL_HERE and HOLYARRS_LAB_OTHER_MOUNT name the two mounts")
	}
	if _, err := os.Stat("/data/downloads"); !os.IsNotExist(err) {
		t.Fatalf("/data/downloads must not exist here, so only the mapping can find the file (%v)", err)
	}
	feed := newLabFeed(t)
	feed.set([]string{labDune1080})
	x := newLabMovie(t, labMovieOpts{cat: "holyarrs-lab-b50", feed: feed.URL, movies: filepath.Join(lib, "movies"),
		pathMap: []domain.PathMap{{Theirs: "/data/downloads", Ours: here}}})
	x.remove(t, labDuneHash)
	x.autoSearch(t)
	f, row := x.untilPlaced(t)
	var save string
	_ = x.s.Read().QueryRow("SELECT save_path FROM downloads LIMIT 1").Scan(&save)
	if !strings.HasPrefix(save, "/data/downloads/") {
		t.Fatalf("save path %q: want qBittorrent's own path", save)
	}
	got, err := os.ReadFile(filepath.Join(x.movies, filepath.FromSlash(f.RelativePath)))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(here, strings.TrimPrefix(save, "/data/downloads/"), labDune1080+".mkv"))
	if err != nil {
		want, err = os.ReadFile(filepath.Join(here, "incomplete", x.cat, labDune1080+".mkv"))
	}
	if err != nil || len(got) != labDuneSize || string(got) != string(want) {
		t.Fatalf("library copy %d bytes, download %d bytes (%v)", len(got), len(want), err)
	}
	if f.PlacedBy != "copy" || !f.FellBack {
		t.Fatalf("across two mounts: placed by %s, fell back %v", f.PlacedBy, f.FellBack)
	}
	t.Logf("#50: qBittorrent path %s mapped to %s; imported across mounts as %s (%s, fell back %v); row %s", save, here, f.RelativePath, f.PlacedBy, f.FellBack, row.Label)
}

// Jackett bridge: connect the lab Jackett (it holds only our own site
// definition, which reads the fake indexer); its site becomes a Source; the
// app's own search finds our Dune test torrent through Jackett, sends it to
// the lab qBittorrent through Jackett's download link, and imports it.
func TestLabBeta47JackettGrab(t *testing.T) {
	key := os.Getenv("HOLYARRS_LAB_JACKETT_KEY")
	if key == "" {
		t.Skip("HOLYARRS_LAB_JACKETT_KEY not set")
	}
	jackett := envOr("HOLYARRS_LAB_JACKETT_URL", "http://jackett:9117")
	x := newLabMovie(t, labMovieOpts{cat: "holyarrs-lab-b47"})
	x.remove(t, labDuneHash)
	var v sources.BridgeView
	_ = json.Unmarshal(do(t, "POST", x.srv.URL+"/api/v1/bridges", fmt.Sprintf(`{"kind":"jackett","url":%q,"key":%q}`, jackett, key), 201), &v)
	if v.Sources < 1 {
		t.Fatalf("bridge %+v", v)
	}
	var srcURL string
	_ = x.s.Read().QueryRow("SELECT url FROM sources LIMIT 1").Scan(&srcURL)
	x.autoSearch(t)
	rels := x.releases(t)
	r, ok := rels[labDune1080]
	if !ok || r.State != "chosen" {
		t.Fatalf("through Jackett: %v; %s", rels, x.dump())
	}
	f, row := x.untilPlaced(t)
	var link string
	_ = x.s.Read().QueryRow("SELECT link_ref FROM releases WHERE id = ?", r.ID).Scan(&link)
	if !strings.HasPrefix(link, jackett+"/dl/") {
		t.Fatalf("link %s: want Jackett's own download link", labKeyless(link))
	}
	var ext string
	_ = x.s.Read().QueryRow("SELECT external_id FROM downloads LIMIT 1").Scan(&ext)
	if !strings.EqualFold(ext, labDuneHash) {
		t.Fatalf("followed as %q", ext)
	}
	t.Logf("#47: Jackett %s (%d sites) -> Source %s; %s chosen from %s, sent through Jackett's link, followed by hash, imported as %s; row %s", v.URL, v.Sites, labKeyless(srcURL), r.Name, r.SourceName, f.RelativePath, row.Label)
}

// labQbitState is the lab qBittorrent's state for one torrent.
func labQbitState(t *testing.T, hash string) string {
	t.Helper()
	resp, err := http.Get(labQbit() + "/api/v2/torrents/info?hashes=" + hash)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []struct {
		State    string  `json:"state"`
		Progress float64 `json:"progress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out) != 1 {
		t.Fatalf("torrent %s in qBittorrent: %v (%d)", hash, err, len(out))
	}
	return fmt.Sprintf("%s %.0f%%", out[0].State, out[0].Progress*100)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func labItoa(n int64) string { return strconv.FormatInt(n, 10) }

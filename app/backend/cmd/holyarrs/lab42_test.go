//go:build lab

package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Stage 4.2 on the lab, inside the lab network, from a container that the
// lab qBittorrent can reach by name:
//
//	HOLYARRS_LAB_SELF=<this container's name> go test -tags lab -run 'TestLab(SeasonPack|BrokenEpisode)' ./cmd/holyarrs/
//
// Both tests make our own tiny torrents here (a season pack of three
// episode files, and single episodes), serve them from this test, and put
// the payload in the lab downloads folder first, so the lab qBittorrent
// checks the files it already has and finishes without any peer. Nothing is
// fetched from outside; the torrents and their files are removed after.
const lab42Cat = "holyarrs-lab42"

func bencode(b *bytes.Buffer, v any) {
	switch x := v.(type) {
	case int:
		fmt.Fprintf(b, "i%de", x)
	case int64:
		fmt.Fprintf(b, "i%de", x)
	case string:
		fmt.Fprintf(b, "%d:%s", len(x), x)
	case []byte:
		fmt.Fprintf(b, "%d:", len(x))
		b.Write(x)
	case []any:
		b.WriteByte('l')
		for _, e := range x {
			bencode(b, e)
		}
		b.WriteByte('e')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('d')
		for _, k := range keys {
			bencode(b, k)
			bencode(b, x[k])
		}
		b.WriteByte('e')
	default:
		panic(fmt.Sprintf("bencode %T", v))
	}
}

type labFile struct {
	name string // inside the torrent's folder (or the single file's name)
	data []byte
}

// labTorrent builds a private torrent: one file when folder is "", else a
// folder of files. It returns the .torrent and its info hash.
func labTorrent(folder string, files []labFile) ([]byte, string) {
	const piece = 256 << 10
	var all []byte
	for _, f := range files {
		all = append(all, f.data...)
	}
	var pieces []byte
	for i := 0; i < len(all); i += piece {
		h := sha1.Sum(all[i:min(i+piece, len(all))])
		pieces = append(pieces, h[:]...)
	}
	info := map[string]any{"piece length": piece, "pieces": pieces, "private": 1}
	if folder == "" {
		info["name"], info["length"] = files[0].name, len(files[0].data)
	} else {
		info["name"] = folder
		var list []any
		for _, f := range files {
			list = append(list, map[string]any{"length": len(f.data), "path": []any{f.name}})
		}
		info["files"] = list
	}
	var ib bytes.Buffer
	bencode(&ib, info)
	sum := sha1.Sum(ib.Bytes())
	var tb bytes.Buffer
	bencode(&tb, map[string]any{"announce": "http://qbt-seed:9000/announce", "created by": "holyarrs lab test", "info": info})
	return tb.Bytes(), hex.EncodeToString(sum[:])
}

func video(seed byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = seed + byte(i%251)
	}
	return b
}

// labShow is Copper Valley with three aired episodes of season 1 in one
// Version, each Slot in state, a Source and the lab qBittorrent.
type labShow struct {
	s       *database.Store
	srcID   int64
	slots   []int64
	shows   string
	partID  int64
	verID   int64
	titleID int64
}

// labNoSizeFloors turns the import size floors off in a lab test's own
// database: its payloads are a few hundred KB on purpose, which the fake
// guard would rightly refuse as too small to be the full video or track.
func labNoSizeFloors(t *testing.T, s *database.Store) {
	t.Helper()
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', 'import_size_floors', 'false', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func newLabShow(t *testing.T, qb string, state mediafiles.SlotState) labShow {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	labNoSizeFloors(t, s)
	x := labShow{s: s, shows: t.TempDir()}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: x.shows, Kind: "show", Label: "Shows", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		y, order := 2025, "aired"
		ti := &mediafiles.Title{Kind: "show", GateDriver: "none", Name: "Copper Valley", SortName: "copper valley", Year: &y, Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", EpisodeOrder: &order, MusicTypes: []string{},
			WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active", KeepUpToDate: true}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		season := 1
		p := &mediafiles.Part{TitleID: ti.ID, Kind: "season", Number: &season, Name: "Season 1", KeepUpToDate: true, SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: ti.ID, Label: "Main", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Copper Valley (2025)", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		x.titleID, x.partID, x.verID = ti.ID, p.ID, v.ID
		for n, name := range []string{"The Mill", "Flood", "Ash"} {
			ep, name := n+1, name
			it := &mediafiles.Item{TitleID: ti.ID, PartID: &p.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: &season, Episode: &ep}, Name: &name, KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: state}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			x.slots = append(x.slots, sl.ID)
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://fake-indexer/api", AllowLocal: true /* the lab indexer is at home */, Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		x.srcID = src.ID
		d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: qb, Category: lab42Cat, Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		_, err := database.Insert(tx, "downloaders", d, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return x
}

// release records a Release chosen for slots (a pack: all of them).
func (x labShow) release(t *testing.T, name, link, hash string, size int64, slots []int64) int64 {
	t.Helper()
	h := hash
	r := &domain.Release{SourceID: x.srcID, Name: name, LinkRef: link, Protocol: "torrent", Size: size, InfoHash: &h, Flags: []string{}, Parsed: map[string]any{},
		MatchedSlotIDs: slots, Verdict: map[string]any{"pick_slot_ids": slots}, FoundBy: "auto_search", State: "chosen"}
	err := x.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		for _, id := range slots {
			if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", r.ID, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

// labServe serves the torrents from this test on a port the lab
// qBittorrent can reach; it returns the base URL.
func labServe(t *testing.T, torrents map[string][]byte) string {
	t.Helper()
	self := os.Getenv("HOLYARRS_LAB_SELF")
	if self == "" {
		self = "holyarrs-labtest"
	}
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := torrents[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(b)
	})}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return fmt.Sprintf("http://%s:%d", self, l.Addr().(*net.TCPAddr).Port)
}

func labQbit() string {
	if qb := os.Getenv("HOLYARRS_LAB_QBIT"); qb != "" {
		return qb
	}
	return "http://qbittorrent:8090"
}

func labRemove(t *testing.T, m *downloads.Manager, qb string, hashes ...string) {
	t.Cleanup(func() {
		if e, err := m.EngineFor(context.Background(), domain.Downloader{Adapter: "qbittorrent", URL: qb, AllowLocal: true}); err == nil {
			for _, h := range hashes {
				_ = e.Remove(context.Background(), h, true)
			}
		}
		_ = os.RemoveAll(filepath.Join("/data/downloads/incomplete", lab42Cat))
	})
}

func labRows(t *testing.T, url string) []downloads.DownloadRow {
	var out struct {
		Downloads []downloads.DownloadRow `json:"downloads"`
	}
	_ = json.Unmarshal(do(t, "GET", url+"/api/v1/downloads/view", "", 200), &out)
	return out.Downloads
}

// 4.2.1 done when: a lab season pack (our own torrent with three tiny
// episode files) imports into all its episodes.
func TestLabSeasonPackImports(t *testing.T) {
	qb := labQbit()
	ctx := context.Background()
	x := newLabShow(t, qb, mediafiles.SlotChosen)
	const pack = "Copper.Valley.S01.1080p.WEB.H264-HOLYARRS"
	var files []labFile
	for n := 1; n <= 3; n++ {
		files = append(files, labFile{name: fmt.Sprintf("Copper.Valley.S01E0%d.1080p.WEB.H264-HOLYARRS.mkv", n), data: video(byte(n), 300<<10)})
	}
	tor, hash := labTorrent(pack, files)
	base := labServe(t, map[string][]byte{"pack.torrent": tor})
	save := filepath.Join("/data/downloads/incomplete", lab42Cat)
	for _, f := range files {
		p := filepath.Join(save, pack, f.name)
		if err := os.MkdirAll(filepath.Dir(p), 0o777); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, f.data, 0o666); err != nil {
			t.Fatal(err)
		}
	}
	relID := x.release(t, pack, base+"/pack.torrent", hash, 900<<10, x.slots)
	m := &downloads.Manager{Store: x.s, SavePath: save}
	labRemove(t, m, qb, hash)
	im := &importer.Importer{Store: x.s}
	srv := httptest.NewServer(routes(buildHandlers(services{Store: x.s, Metadata: metadata.Lab(), Version: "lab", Downloads: m}), config{}))
	t.Cleanup(srv.Close)
	// 5.2.5: a webhook Alert for grab and import, received here.
	var hookMu sync.Mutex
	var hooks []alerts.Payload
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p alerts.Payload
		_ = json.NewDecoder(r.Body).Decode(&p)
		hookMu.Lock()
		hooks = append(hooks, p)
		hookMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(hook.Close)
	vault, err := secrets.Open(ctx, x.s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if _, err := alerts.Add(ctx, x.s, vault, alerts.Input{Name: "Lab hook", Kind: "webhook", URL: hook.URL, AllowLocal: &yes,
		Moments: []string{alerts.MomentGrab, alerts.MomentImport}}, domain.System); err != nil {
		t.Fatal(err)
	}
	snd := &alerts.Sender{Store: x.s, Vault: vault}
	snd.Watch(ctx)

	id, err := m.Grab(ctx, x.slots[0], relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if d := labRows(t, srv.URL); len(d) != 1 || d[0].Covers != "S01E01-E03, 3 episodes" || len(d[0].Episodes) != 3 {
		t.Fatalf("row %+v", d)
	}
	seen := []string{}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if _, err := m.SyncOnce(ctx); err != nil {
			t.Fatal(err)
		}
		r := labRows(t, srv.URL)[0]
		if len(seen) == 0 || seen[len(seen)-1] != r.Phase {
			seen = append(seen, r.Phase)
		}
		if r.Phase == "waiting_to_import" {
			break
		}
		if r.Phase == "failed" || r.Phase == "vanished" || time.Now().After(deadline) {
			t.Fatalf("phases %v, last %+v", seen, r)
		}
		time.Sleep(time.Second)
	}
	imp, err := im.Import(ctx, id, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	r := labRows(t, srv.URL)[0]
	seen = append(seen, r.Phase)
	if imp.State != "done" || r.Phase != "done" || r.Text != "3 of 3 episodes in the library." {
		t.Fatalf("import %s %+v; row %+v; phases %v", imp.State, imp.Result, r, seen)
	}
	for n, sid := range x.slots {
		var st, rel string
		_ = x.s.Read().QueryRow("SELECT s.state, f.relative_path FROM slots s JOIN files f ON f.id = s.file_id WHERE s.id = ?", sid).Scan(&st, &rel)
		want := fmt.Sprintf("Copper Valley (2025)/Season 1/Copper Valley - S01E0%d - %s 1080p WEB.mkv", n+1, []string{"The Mill", "Flood", "Ash"}[n])
		if (st != "in_library" && st != "better_wanted") || rel != want {
			t.Fatalf("S01E0%d: %s %q", n+1, st, rel)
		}
		if b, err := os.ReadFile(filepath.Join(x.shows, filepath.FromSlash(rel))); err != nil || !bytes.Equal(b, files[n].data) {
			t.Fatalf("S01E0%d on disk: %v", n+1, err)
		}
	}
	// One Alert for the grab and one for the pack import, naming the show
	// and its episodes; History rows for both name the episodes too.
	snd.Wait()
	hookMu.Lock()
	got := append([]alerts.Payload{}, hooks...)
	hookMu.Unlock()
	subjects := map[string]bool{}
	for _, p := range got {
		subjects[p.Subject] = true
		if p.Show == nil || p.Show.Name != "Copper Valley" || len(p.Episodes) != 3 {
			t.Fatalf("alert %+v", p)
		}
	}
	if len(got) != 2 || !subjects["Sent to download: Copper Valley S01E01 to S01E03 (3 episodes)"] || !subjects["In the Library: Copper Valley S01E01 to S01E03 (3 episodes)"] {
		t.Fatalf("alerts %v", subjects)
	}
	var hist struct {
		Events []shows.EventView `json:"events"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/events?kind=download_sent,import_*", "", 200), &hist)
	if len(hist.Events) != 2 || hist.Events[0].Episode != "S01E01-E03, 3 episodes" || hist.Events[1].Episode != "S01E01-E03, 3 episodes" {
		t.Fatalf("history %+v", hist.Events)
	}
	t.Logf("season pack through the lab qBittorrent: phases %v, import %s, %s; alerts %v", seen, imp.State, r.Text, subjects)
}

// 4.2.2 done when: a broken lab episode download ends with the next Release
// grabbed. The broken one has no payload and no peer; Mark failed (from the
// Downloads screen's route) turns it down and the next Release (sent by
// OnFailed, standing in for the search) finishes and imports.
func TestLabBrokenEpisodeNextGrabbed(t *testing.T) {
	qb := labQbit()
	ctx := context.Background()
	x := newLabShow(t, qb, mediafiles.SlotChosen)
	ep := x.slots[1] // S01E02
	const broken, next = "Copper.Valley.S01E02.1080p.WEB.H264-BROKEN", "Copper.Valley.S01E02.720p.WEB.H264-HOLYARRS"
	bf := labFile{name: broken + ".mkv", data: video(9, 400<<10)}
	nf := labFile{name: next + ".mkv", data: video(7, 300<<10)}
	bt, bh := labTorrent("", []labFile{bf})
	nt, nh := labTorrent("", []labFile{nf})
	base := labServe(t, map[string][]byte{"broken.torrent": bt, "next.torrent": nt})
	save := filepath.Join("/data/downloads/incomplete", lab42Cat)
	if err := os.MkdirAll(save, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(save, nf.name), nf.data, 0o666); err != nil {
		t.Fatal(err)
	}
	nextID := x.release(t, next, base+"/next.torrent", nh, 300<<10, []int64{ep})
	brokenID := x.release(t, broken, base+"/broken.torrent", bh, 400<<10, []int64{ep})
	if err := x.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE releases SET state = 'judged' WHERE id = ?", nextID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m := &downloads.Manager{Store: x.s, SavePath: save}
	labRemove(t, m, qb, bh, nh)
	var nextDL int64
	m.OnFailed = func(ctx context.Context, slotID int64) {
		if err := x.s.Write(ctx, func(tx *sql.Tx) error {
			if err := mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotSearching, "searching", domain.System); err != nil {
				return err
			}
			if err := mediafiles.MoveSlotTx(tx, slotID, mediafiles.SlotChosen, "chose the next", domain.System); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", nextID, slotID)
			return err
		}); err != nil {
			t.Errorf("choose next Release: %v", err)
			return
		}
		id, err := m.Grab(ctx, slotID, nextID, domain.System)
		if err != nil {
			t.Errorf("next grab: %v", err)
		}
		nextDL = id
	}
	srv := httptest.NewServer(routes(buildHandlers(services{Store: x.s, Metadata: metadata.Lab(), Version: "lab", Downloads: m}), config{}))
	t.Cleanup(srv.Close)
	first, err := m.Grab(ctx, ep, brokenID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// The broken one never moves: no peer has it.
	for i := 0; i < 5; i++ {
		_, _ = m.SyncOnce(ctx)
		time.Sleep(time.Second)
	}
	var d domain.Download
	_ = database.Get(ctx, x.s.Read(), "downloads", first, &d)
	if d.Progress >= 1 || (d.State != "downloading" && d.State != "sent" && d.State != "queued" && d.State != "stalled") {
		t.Fatalf("broken download %s %.2f", d.State, d.Progress)
	}
	do(t, "POST", srv.URL+"/api/v1/downloads/"+labItoa(first)+"/mark-failed", "", 200)
	if nextDL == 0 {
		t.Fatal("the next Release was not grabbed")
	}
	var rs string
	_ = x.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", brokenID).Scan(&rs)
	if rs != "turned_down" {
		t.Fatalf("broken release %s", rs)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		_, _ = m.SyncOnce(ctx)
		_ = database.Get(ctx, x.s.Read(), "downloads", nextDL, &d)
		if d.State == "finished" || d.State == "seeding" {
			break
		}
		if d.State == "failed" || time.Now().After(deadline) {
			t.Fatalf("next download %s", d.State)
		}
		time.Sleep(time.Second)
	}
	imp, err := (&importer.Importer{Store: x.s}).Import(ctx, nextDL, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %v %+v", err, imp)
	}
	var st string
	_ = x.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", ep).Scan(&st)
	if st != "in_library" && st != "better_wanted" {
		t.Fatalf("S01E02 %s", st)
	}
	rows := labRows(t, srv.URL)
	t.Logf("broken episode replaced: %d rows, first %s, next %s, S01E02 %s", len(rows), rows[1].Label, rows[0].Label, st)
}

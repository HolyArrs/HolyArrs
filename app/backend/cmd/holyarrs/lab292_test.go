//go:build lab

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// labQbitTorrent is one torrent as the lab qBittorrent reports it.
type labQbitTorrent struct {
	Hash     string `json:"hash"`
	Category string `json:"category"`
	State    string `json:"state"`
}

func labQbitCall(t *testing.T, qb, method, path string, form url.Values) []byte {
	t.Helper()
	var req *http.Request
	var err error
	if method == http.MethodGet {
		req, err = http.NewRequest(method, qb+"/api/v2/"+path+"?"+form.Encode(), nil)
	} else {
		req, err = http.NewRequest(method, qb+"/api/v2/"+path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := res.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if res.StatusCode/100 != 2 { // qBittorrent 5.2 answers 202 while it fetches a link
		t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, b.String())
	}
	return []byte(b.String())
}

func labQbitList(t *testing.T, qb string, form url.Values) []labQbitTorrent {
	var out []labQbitTorrent
	if err := json.Unmarshal(labQbitCall(t, qb, http.MethodGet, "torrents/info", form), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// On a shared qBittorrent: another app's torrent sits in its own
// category next to HolyArrs's. HolyArrs grabs and imports our Dune test
// torrent, moves it to the after-import category once, and never touches the
// other app's torrent (not by the move, not by Pause all):
//
//	go test -tags lab -run TestLabCategoryAfterImportSharedQbit ./cmd/holyarrs/
func TestLabCategoryAfterImportSharedQbit(t *testing.T) {
	qb := os.Getenv("HOLYARRS_LAB_QBIT")
	if qb == "" {
		qb = "http://qbittorrent:8090"
	}
	const hash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"
	const cat, done, other = "holyarrs-lab292", "holyarrs-lab292-done", "sonarr-lab292"
	ctx := context.Background()

	// The other app's torrent: our own Arcane test torrent, in its category.
	labQbitCall(t, qb, http.MethodPost, "torrents/add", url.Values{"urls": {"http://fake-indexer/arcane-s01e01.torrent"},
		"category": {other}, "savepath": {"/data/downloads/incomplete/" + other}})
	var otherHash string
	for i := 0; i < 30 && otherHash == ""; i++ {
		if l := labQbitList(t, qb, url.Values{"category": {other}}); len(l) == 1 {
			otherHash = l[0].Hash
		} else {
			time.Sleep(time.Second)
		}
	}
	if otherHash == "" {
		t.Fatal("the other app's torrent never showed in qBittorrent")
	}
	t.Cleanup(func() {
		labQbitCall(t, qb, http.MethodPost, "torrents/delete", url.Values{"hashes": {otherHash + "|" + hash}, "deleteFiles": {"true"}})
		labQbitCall(t, qb, http.MethodPost, "torrents/removeCategories", url.Values{"categories": {other + "\n" + cat + "\n" + done}})
	})
	otherBefore := labQbitList(t, qb, url.Values{"hashes": {otherHash}})[0]

	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	movies := t.TempDir()
	var slotID, relID int64
	var dl domain.Downloader
	err = s.Write(ctx, func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		y := 2024
		lf := &mediafiles.LibraryFolder{Path: movies, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Dune: Part Two", SortName: "dune part two", Year: &y, Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{},
			WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
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
		v := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Dune Part Two (2024)", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: v.ID, State: mediafiles.SlotChosen}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://fake-indexer/api", AllowLocal: true, Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		h := hash
		rel := &domain.Release{SourceID: src.ID, Name: "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", LinkRef: "http://fake-indexer/dune-part-two.torrent",
			Protocol: "torrent", Size: 29588180, InfoHash: &h, Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID},
			Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", rel.ID, sl.ID); err != nil {
			return err
		}
		dl = domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: qb, Category: cat, Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true, Writable: true, CategoryAfterImport: done, OwnOnly: true}
		if _, err := database.Insert(tx, "downloaders", &dl, domain.System); err != nil {
			return err
		}
		slotID, relID = sl.ID, rel.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m := &downloads.Manager{Store: s, SavePath: "/data/downloads/incomplete/" + cat}
	im := &importer.Importer{Store: s}
	im.OnImported = func(ctx context.Context, id int64) { m.MoveAfterImport(ctx, id) } // as main.go wires it
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "lab", Downloads: m}), config{}))
	t.Cleanup(srv.Close)

	id, err := m.Grab(ctx, slotID, relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if _, err := m.SyncOnce(ctx); err != nil {
			t.Fatal(err)
		}
		var out struct {
			Downloads []downloads.DownloadRow `json:"downloads"`
		}
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/downloads/view", "", 200), &out)
		if len(out.Downloads) != 1 {
			t.Fatalf("Downloads rows %+v (the other app's torrent must not show)", out.Downloads)
		}
		if out.Downloads[0].Phase == "waiting_to_import" {
			break
		}
		if p := out.Downloads[0].Phase; p == "failed" || p == "vanished" || time.Now().After(deadline) {
			t.Fatalf("phase %s", p)
		}
		time.Sleep(time.Second)
	}
	if got := labQbitList(t, qb, url.Values{"hashes": {hash}})[0].Category; got != cat {
		t.Fatalf("before import our torrent is in %q, want %q", got, cat)
	}
	// A subtitle file that came with the download is brought along:
	// our own test file, dropped beside the finished video.
	saveDir := "/data/downloads/incomplete/" + cat
	t.Cleanup(func() { _ = os.RemoveAll(saveDir); _ = os.RemoveAll("/data/downloads/incomplete/" + other) })
	if _, err := importer.SaveSideFiles(ctx, s, importer.SideFiles{On: true, Types: []string{"srt"}}, domain.System); err != nil {
		t.Fatal(err)
	}
	vids, _ := filepath.Glob(saveDir + "/*.mkv")
	if len(vids) != 1 {
		t.Fatalf("downloaded videos in %s: %v", saveDir, vids)
	}
	if err := os.WriteFile(strings.TrimSuffix(vids[0], ".mkv")+".en.srt", []byte("1\n00:00:01,000 --> 00:00:02,000\nlab\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	imp, err := im.Import(ctx, id, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var placedVideo, placedSub string
	_ = filepath.WalkDir(movies, func(p string, d os.DirEntry, _ error) error {
		switch {
		case d == nil || d.IsDir():
		case strings.HasSuffix(p, ".mkv"):
			placedVideo = p
		case strings.HasSuffix(p, ".srt"):
			placedSub = p
		}
		return nil
	})
	if placedVideo == "" || placedSub != strings.TrimSuffix(placedVideo, ".mkv")+".en.srt" {
		t.Fatalf("after import the Library has video %q and subtitle %q; want the subtitle beside the video as <video name>.en.srt", placedVideo, placedSub)
	}
	ours := labQbitList(t, qb, url.Values{"hashes": {hash}})
	if len(ours) != 1 || ours[0].Category != done {
		t.Fatalf("after import %s our torrent is %+v, want category %q", imp.State, ours, done)
	}
	var lines int
	_ = s.ReadEvents().QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'download_changed' AND download_id = ?", id).Scan(&lines)

	// A second import run must not move it again.
	m.MoveAfterImport(ctx, id)
	var lines2 int
	_ = s.ReadEvents().QueryRow("SELECT COUNT(*) FROM events WHERE kind = 'download_changed' AND download_id = ?", id).Scan(&lines2)
	if lines < 1 || lines2 != lines {
		t.Fatalf("History lines for the move went from %d to %d on a second run", lines, lines2)
	}

	// Pause all on HolyArrs's side stops our torrent only.
	e, err := m.EngineFor(ctx, dl)
	if err != nil {
		t.Fatal(err)
	}
	pa, ok := e.(interface {
		PauseAll(context.Context) error
		ResumeAll(context.Context) error
	})
	if !ok {
		t.Fatal("qBittorrent engine has no PauseAll")
	}
	if err := pa.PauseAll(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	otherAfter := labQbitList(t, qb, url.Values{"hashes": {otherHash}})
	oursPaused := labQbitList(t, qb, url.Values{"hashes": {hash}})[0]
	_ = pa.ResumeAll(ctx)
	if len(otherAfter) != 1 || otherAfter[0].Category != other {
		t.Fatalf("other app's torrent changed: before %+v, after %+v", otherBefore, otherAfter)
	}
	if strings.HasPrefix(otherAfter[0].State, "stopped") || strings.HasPrefix(otherAfter[0].State, "paused") {
		t.Fatalf("Pause all stopped the other app's torrent: %+v", otherAfter[0])
	}
	if !strings.HasPrefix(oursPaused.State, "stopped") && !strings.HasPrefix(oursPaused.State, "paused") {
		t.Fatalf("Pause all left our torrent running: %+v", oursPaused)
	}
	t.Logf("subtitle came along as %s; import %s; ours moved %s -> %s (History lines %d); other %s stayed %s/%s", filepath.Base(placedSub), imp.State, cat, done, lines, otherHash[:8], otherAfter[0].Category, otherAfter[0].State)
}

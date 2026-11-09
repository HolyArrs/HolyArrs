//go:build lab

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// in the lab, as TestLabMovieZeroToDone but with our own RAR
// test torrent (research/lab/make_rar_torrent.py): a compressed RAR 5 set in
// four volumes with an .nfo and a Sample clip. The lab qBittorrent downloads
// it from qbt-seed, the importer unpacks it with the built-in decoder next to
// the download and puts the video in the Library.
//
//	go test -tags lab -run TestLabRarMovieZeroToDone ./cmd/holyarrs/
func TestLabRarMovieZeroToDone(t *testing.T) {
	qb := os.Getenv("HOLYARRS_LAB_QBIT")
	if qb == "" {
		qb = "http://qbittorrent:8090"
	}
	const hash = "de60c8b2410fa13e3dfdeb49094e736b57a1c2bc"
	const cat = "holyarrs-lab54"
	ctx := context.Background()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	movies := t.TempDir()
	var slotID, relID int64
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
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://fake-indexer/api", AllowLocal: true /* the lab indexer is at home */, Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		h := hash
		rel := &domain.Release{SourceID: src.ID, Name: "Dune.Part.Two.2024.2160p.WEB-DL.H265-HOLYARRS", LinkRef: "http://fake-indexer/dune-rar.torrent",
			Protocol: "torrent", Size: 30306558, InfoHash: &h, Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID},
			Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", rel.ID, sl.ID); err != nil {
			return err
		}
		d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: qb, Category: cat, Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		slotID, relID = sl.ID, rel.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A 29 MB "2160p" lab video would trip the size floors meant for real ones.
	labNoSizeFloors(t, s)
	im := &importer.Importer{Store: s}
	m := &downloads.Manager{Store: s, SavePath: "/data/downloads/incomplete/" + cat}
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "lab", Downloads: m}), config{}))
	t.Cleanup(srv.Close)
	view := func() downloads.DownloadRow {
		var out struct {
			Downloads []downloads.DownloadRow `json:"downloads"`
		}
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/downloads/view", "", 200), &out)
		if len(out.Downloads) != 1 {
			t.Fatalf("rows %+v", out.Downloads)
		}
		return out.Downloads[0]
	}

	id, err := m.Grab(ctx, slotID, relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e, err := m.EngineFor(ctx, domain.Downloader{Adapter: "qbittorrent", URL: qb, AllowLocal: true}); err == nil {
			_ = e.Remove(context.Background(), hash, true)
		}
	})
	seen := []string{view().Phase}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if _, err := m.SyncOnce(ctx); err != nil {
			t.Fatal(err)
		}
		r := view()
		if seen[len(seen)-1] != r.Phase {
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
	r := view()
	seen = append(seen, r.Phase)
	var st string
	_ = s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&st)
	if r.Phase != "done" || r.Label != "Done" || (st != string(mediafiles.SlotInLibrary) && st != string(mediafiles.SlotBetterWanted)) {
		t.Fatalf("after import %s: slot %s, row %+v, phases %v", imp.State, st, r, seen)
	}
	// The movie in the Library is the video from inside the RAR set, not the Sample.
	var got []string
	_ = filepath.WalkDir(movies, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil && fi.Size() == 29588180 {
				got = append(got, p)
			}
		}
		return nil
	})
	if len(got) != 1 {
		t.Fatalf("library files of the video's size: %v", got)
	}
	t.Logf("0 -> done through the lab qBittorrent with a compressed RAR 5 set: phases %v, import %s, slot %s, library file %s", seen, imp.State, st, got[0])
}

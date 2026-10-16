//go:build lab

package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/importer"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Stage 4.1: one real run from nothing to done, inside the lab network:
//
//	go test -tags lab -run TestLabMovieZeroToDone ./internal/api/
//
// The lab qBittorrent fetches our own Dune test torrent from the lab's
// fake indexer and seeder; HolyArrs follows it on the Downloads screen
// (GET /api/v1/downloads/view) until the importer places the file and the
// row says Done. /data/downloads must be the lab downloads folder, as
// qBittorrent sees it. The torrent and its files are removed afterwards.
func TestLabMovieZeroToDone(t *testing.T) {
	qb := os.Getenv("HOLYARRS_LAB_QBIT")
	if qb == "" {
		qb = "http://qbittorrent:8090"
	}
	const hash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"
	const cat = "holyarrs-lab41"
	ctx := context.Background()
	s, err := store.Open(t.TempDir(), store.Options{})
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
		lf := &domain.LibraryFolder{Path: movies, Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		ti := &domain.Title{Kind: "movie", GateDriver: "none", Name: "Dune: Part Two", SortName: "dune part two", Year: &y, Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{},
			WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
		if _, err := store.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		if _, err := store.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		it := &domain.Item{TitleID: ti.ID, Kind: "movie", KeepUpToDate: true}
		if _, err := store.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		v := &domain.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Dune Part Two (2024)", IsMain: true, KeepUpToDate: true}
		if _, err := store.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		sl := &domain.Slot{ItemID: it.ID, VersionID: v.ID, State: domain.SlotChosen}
		if _, err := store.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://fake-indexer/api", Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}}
		if _, err := store.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		h := hash
		rel := &domain.Release{SourceID: src.ID, Name: "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", LinkRef: "http://fake-indexer/dune-part-two.torrent",
			Protocol: "torrent", Size: 29588180, InfoHash: &h, Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID},
			Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := store.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		d := &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent", URL: qb, Category: cat, Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := store.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		slotID, relID = sl.ID, rel.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	im := &importer.Importer{Store: s}
	m := &downloader.Manager{Store: s, SavePath: "/data/downloads/incomplete/" + cat}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: metadata.Lab(), Version: "lab", Downloads: m}).Handler())
	t.Cleanup(srv.Close)
	view := func() DownloadRow {
		var out struct {
			Downloads []DownloadRow `json:"downloads"`
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
	if r.Phase != "done" || r.Label != "Done" || (st != string(domain.SlotInLibrary) && st != string(domain.SlotBetterWanted)) {
		t.Fatalf("after import %s: slot %s, row %+v, phases %v", imp.State, st, r, seen)
	}
	t.Logf("0 -> done through the lab qBittorrent: phases %v, import %s, slot %s, progress %.2f", seen, imp.State, st, r.Progress)
}

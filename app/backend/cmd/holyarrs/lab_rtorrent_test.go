//go:build lab

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// TestLabRTorrentMovieZeroToDone follows the lab's own Dune test torrent from
// a grab to Done through a real rTorrent:
//
//	HOLYARRS_LAB_RTORRENT          the XML-RPC address
//	HOLYARRS_LAB_RTORRENT_USER     and HOLYARRS_LAB_RTORRENT_PASSWORD_FILE for a sign-in
//	HOLYARRS_LAB_RTORRENT_HERE     rTorrent's /downloads as this test sees it (a path pair)
//	HOLYARRS_LAB_RTORRENT_TORRENT  the lab's dune-part-two.torrent
//
// Its payload must already be in HERE/temp, so rTorrent finishes it by
// checking the file. The torrent and its files are removed afterwards.
func TestLabRTorrentMovieZeroToDone(t *testing.T) {
	base, here, file := os.Getenv("HOLYARRS_LAB_RTORRENT"), os.Getenv("HOLYARRS_LAB_RTORRENT_HERE"), os.Getenv("HOLYARRS_LAB_RTORRENT_TORRENT")
	if base == "" || here == "" || file == "" {
		t.Skip("HOLYARRS_LAB_RTORRENT, _HERE and _TORRENT are not set")
	}
	torrent, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	pass := ""
	if f := os.Getenv("HOLYARRS_LAB_RTORRENT_PASSWORD_FILE"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		pass = strings.TrimSpace(string(b))
	}
	const hash = "29be7bfa8759643f2d5dac9f94d516c3378696ed"
	const cat = "holyarrs-lab-rtorrent"
	payload := filepath.Join(here, "temp", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv")
	if _, err := os.Stat(payload); err != nil {
		t.Fatalf("the payload is not in place: %v", err)
	}
	indexer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(torrent) }))
	t.Cleanup(indexer.Close)

	ctx := context.Background()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	vault := map[int64]string{}
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
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: indexer.URL + "/api", AllowLocal: true, Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		h := hash
		rel := &domain.Release{SourceID: src.ID, Name: "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", LinkRef: indexer.URL + "/dune-part-two.torrent",
			Protocol: "torrent", Size: 29588180, InfoHash: &h, Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID},
			Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", rel.ID, sl.ID); err != nil {
			return err
		}
		dl = domain.Downloader{Name: "Lab rTorrent", Adapter: "rtorrent", Protocol: "torrent", URL: base, Username: os.Getenv("HOLYARRS_LAB_RTORRENT_USER"),
			Category: cat, Priority: 1, PathMap: []domain.PathMap{{Theirs: "/downloads", Ours: here}}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if pass != "" {
			ref := int64(1)
			dl.SecretRef, vault[ref] = &ref, pass
		}
		if _, err := database.Insert(tx, "downloaders", &dl, domain.System); err != nil {
			return err
		}
		slotID, relID = sl.ID, rel.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	im := &importer.Importer{Store: s}
	m := &downloads.Manager{Store: s, SavePath: "/data/downloads", FromVault: func(_ context.Context, id int64) (string, error) { return vault[id], nil }}
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
	e, err := m.EngineFor(ctx, dl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Remove(context.Background(), hash, true) })
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
	placed := 0
	_ = filepath.WalkDir(movies, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".mkv") {
			placed++
		}
		return nil
	})
	if placed != 1 {
		t.Errorf("%d video files in the Library folder", placed)
	}
	t.Logf("0 -> done through the lab rTorrent: phases %v, import %s, slot %s, progress %.2f", seen, imp.State, st, r.Progress)
}

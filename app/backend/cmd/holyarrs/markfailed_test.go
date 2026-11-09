package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// markServer is the whole program without sign-in: marking a Download
// failed shows on the title page (downloads and titleview).
func markServer(t *testing.T) (*httptest.Server, *database.Store) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "test", Secrets: v}), config{}))
	t.Cleanup(srv.Close)
	return srv, s
}

// fakeQbitAPI is enough qBittorrent for a delete.
type fakeQbitAPI struct {
	mu      sync.Mutex
	deleted []string
}

// route: POST /api/v1/downloads/{id}/mark-failed
func TestMarkFailedRoute(t *testing.T) {
	t.Parallel()
	srv, s := markServer(t)
	f := &fakeQbitAPI{}
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/api/v2/torrents/delete" {
			f.deleted = append(f.deleted, r.PostForm.Get("hashes"))
		}
	}))
	t.Cleanup(qb.Close)
	var dlID, slotID int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Salt Choir", SortName: "salt choir", Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		for _, x := range []struct {
			table string
			v     any
		}{{"library_folders", lf}, {"titles", ti}} {
			if _, err := database.Insert(tx, x.table, x.v, domain.System); err != nil {
				return err
			}
		}
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		ver := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Salt Choir", IsMain: true}
		if _, err := database.Insert(tx, "versions", ver, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: ver.ID, State: mediafiles.SlotDownloading}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		rel := &domain.Release{SourceID: src.ID, Name: "Salt.Choir.2020.1080p.WEB.H264-HOLYARRS", LinkRef: "http://x/a.torrent", Protocol: "torrent", Size: 1,
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Writable: true, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: qb.URL, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: dr.ID, ReleaseID: &rel.ID, SlotIDs: []int64{sl.ID}, ExternalID: "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd", State: "downloading", Progress: 0.3,
			FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", dl.ID, sl.ID); err != nil {
			return err
		}
		dlID, slotID = dl.ID, sl.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The movie page shows the Download in plain words.
	var tv discover.TitleView
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles/1", "", 200), &tv)
	if len(tv.Slots) != 1 || tv.Slots[0].Download == nil || tv.Slots[0].Download.Label != "Downloading" || !tv.Slots[0].Download.Active {
		t.Fatalf("slot %+v", tv.Slots)
	}
	var dv downloads.DownloadView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloads/"+itoa(dlID)+"/mark-failed", "", 200), &dv)
	if dv.State != "failed" || dv.Active || !strings.HasPrefix(dv.Text, "Marked failed") {
		t.Fatalf("marked %+v", dv)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd" {
		t.Fatalf("not removed from the Downloader: %v", f.deleted)
	}
	var st string
	_ = s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&st)
	if st != string(mediafiles.SlotFailed) {
		t.Fatalf("slot %s", st)
	}
	do(t, "POST", srv.URL+"/api/v1/downloads/"+itoa(dlID)+"/mark-failed", "", 409)
	do(t, "POST", srv.URL+"/api/v1/downloads/999/mark-failed", "", 404)
}

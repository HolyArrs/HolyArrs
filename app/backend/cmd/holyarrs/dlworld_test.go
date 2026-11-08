package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type dl41 struct {
	srv                 *httptest.Server
	s                   *database.Store
	q                   *jobs.Queue
	dlID, slotID, relID int64
	mu                  sync.Mutex
	deleted             []string
	api                 *handlers
}

// seed41 makes one movie with a Slot in slotState, its Release, a fake
// qBittorrent and a Download in dlState.
func seed41(t *testing.T, slotState mediafiles.SlotState, dlState string) *dl41 {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	x := &dl41{s: s, q: jobs.New(s, jobs.Options{Workers: 1, PollInterval: time.Hour})}
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path == "/api/v2/torrents/delete" {
			x.mu.Lock()
			x.deleted = append(x.deleted, r.PostForm.Get("hashes")+"|"+r.PostForm.Get("deleteFiles"))
			x.mu.Unlock()
		}
	}))
	t.Cleanup(qb.Close)
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr); err != nil {
			return err
		}
		lf := &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		y := 2020
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Salt Choir", SortName: "salt choir", Year: &y, Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
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
		ver := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Salt Choir", IsMain: true, KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", ver, domain.System); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: it.ID, VersionID: ver.ID, State: slotState}
		if slotState == mediafiles.SlotNeedsADecision {
			r := "the download failed and no other Release passed the Pick rules (3 found, 3 refused)"
			sl.LastReason = &r
		}
		if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		rel := &domain.Release{SourceID: src.ID, Name: "Salt.Choir.2020.1080p.WEB.H264-HOLYARRS", LinkRef: "http://x/a.torrent", Protocol: "torrent", Size: 1000,
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Writable: true, Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: qb.URL, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		eta := int64(90)
		dl := &domain.Download{DownloaderID: dr.ID, ReleaseID: &rel.ID, SlotIDs: []int64{sl.ID}, ExternalID: "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd", State: dlState, Progress: 0.3,
			Speed: 2048, ETA: &eta, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", dl, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id = ?", dl.ID, sl.ID); err != nil {
			return err
		}
		x.dlID, x.slotID, x.relID = dl.ID, sl.ID, rel.ID
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	x.api = buildHandlers(services{Store: s, Queue: x.q, Live: webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second}), Metadata: metadata.Lab(), Version: "test"})
	x.srv = httptest.NewServer(x.api.Handler())
	t.Cleanup(x.srv.Close)
	return x
}

func (x *dl41) slotState(t *testing.T) string {
	t.Helper()
	var st string
	_ = x.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", x.slotID).Scan(&st)
	return st
}

func (x *dl41) removed() []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	return append([]string(nil), x.deleted...)
}

func exec41(t *testing.T, s *database.Store, q string, args ...any) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

func count(t *testing.T, st *database.Store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := st.Read().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// noClient is a reader Manager that never reaches a client: the store only
// holds what the test loads.
type noClient struct{ m *downloads.Manager }

func (noClient) Reader(context.Context, domain.Downloader) (downloads.Engine, uint64, error) {
	return nil, 0, errors.New("no client in this test")
}
func (noClient) ReportRead(context.Context, domain.Downloader, error) {}
func (noClient) Generation(int64) uint64                              { return 0 }
func (n noClient) Interval(a int) time.Duration                       { return n.m.Interval(a) }

// qbList is a fake qBittorrent that lists the given torrents.
func qbList(t *testing.T, torrents string) *httptest.Server {
	t.Helper()
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch strings.TrimPrefix(r.URL.Path, "/api/v2/") {
		case "torrents/info":
			if r.Form.Get("category") != "" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(torrents))
		case "app/webapiVersion":
			_, _ = w.Write([]byte("2.15.1"))
		}
	}))
	t.Cleanup(qb.Close)
	return qb
}

// pointQB moves the seeded Downloader to url and drops its cached engine.
func pointQB(t *testing.T, x *dl41, url string) {
	t.Helper()
	exec41(t, x.s, "UPDATE downloaders SET url = ?", url)
	var id int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&id)
	x.api.DownloadsWeb().Manager().Forget(id)
}

type viewAnswer struct {
	Downloads    []downloads.DownloadRow  `json:"downloads"`
	Totals       downloads.DownloadTotals `json:"totals"`
	Clients      []downloads.ClientHealth `json:"clients"`
	OutsideTotal int                      `json:"outside_total"`
	NextCursor   string                   `json:"next_cursor"`
}

func view(t *testing.T, url string) viewAnswer {
	t.Helper()
	var v viewAnswer
	if err := json.Unmarshal(do(t, "GET", url, "", 200), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

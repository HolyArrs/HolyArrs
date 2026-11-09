package downloads

import (
	"context"
	"database/sql"
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
	dl                  *Web
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
	x.dl = &Web{Store: s, Queue: x.q, Live: webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second}), Metadata: metadata.Lab()}
	x.srv = webRig(t, x.dl)
	return x
}

func exec41(t *testing.T, s *database.Store, q string, args ...any) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// feedStore does one background read by hand: every torrent Downloader's
// whole list goes into the reader's Store (made when the API has none),
// the way the reader's pollers fill it. The Downloads view reads only it.
func feedStore(t *testing.T, a *Web) *Sync {
	t.Helper()
	if a.DLSync == nil {
		a.DLSync = &Sync{Store: &Store{}, DB: a.Store}
	}
	ctx := context.Background()
	ds, err := database.Select[domain.Downloader](ctx, a.Store.Read(), "downloaders", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		if d.Removed || d.Protocol != "torrent" {
			continue
		}
		var jobs []Job
		err := a.Manager().ControlEngine(ctx, d, func(e Engine) error {
			al, ok := e.(AllLister)
			if !ok {
				return ErrNotSupported
			}
			var err error
			jobs, err = al.ListAll(ctx)
			return err
		})
		if errors.Is(err, ErrNotSupported) {
			err = nil
		}
		if err != nil {
			a.DLSync.Store.Fail(d.ID, 0)
			continue
		}
		a.DLSync.Store.Update(d.ID, 0, jobs, time.Now())
	}
	return a.DLSync
}

// in other categories; it records every call.
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
{"hash":"` + strings.ToUpper(routeOwnHash) + `","name":"Salt.Choir","category":"holyarrs","size":1000,"progress":0.3,"state":"downloading","added_on":1700000000},
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

const (
	pauseHash = "3333333333333333333333333333333333333333"
)

type viewRows struct {
	Downloads []DownloadRow `json:"downloads"`
}

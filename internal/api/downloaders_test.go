package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func body(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func dlServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(Server{Store: s, Metadata: metadata.Lab(), Version: "test", Secrets: v}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

// fakeSAB answers the version, queue and categories calls with one key.
func fakeSAB(t *testing.T, key string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("mode") == "version":
			_, _ = w.Write([]byte(`{"version":"5.1.3"}`))
		case q.Get("apikey") != key:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("API Key Incorrect"))
		case q.Get("mode") == "get_cats":
			_, _ = w.Write([]byte(`{"categories":["*","movies"]}`))
		default:
			_, _ = w.Write([]byte(`{"queue":{"status":"Idle","slots":[]}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// route: GET /api/v1/downloaders
// route: POST /api/v1/downloaders
// route: PUT /api/v1/downloaders/{id}
// route: DELETE /api/v1/downloaders/{id}
// route: POST /api/v1/downloaders/{id}/test
// route: POST /api/v1/downloaders/{id}/pause
// route: POST /api/v1/downloaders/{id}/resume
func TestDownloadersAddTestPauseDelete(t *testing.T) {
	srv, s := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab := fakeSAB(t, key)

	// SABnzbd needs its key.
	do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"Lab SABnzbd","kind":"sabnzbd","url":"`+sab.URL+`","allow_local_network":true}`, 400)
	do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"Bad","kind":"unsupported","url":"http://x"}`, 400)
	do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"Bad","kind":"sabnzbd","url":"http://user:pw@x","secret":"k"}`, 400)

	var d DownloaderView
	raw, _ := json.Marshal(map[string]any{"name": "Lab SABnzbd", "kind": "sabnzbd", "url": sab.URL, "secret": key, "category": "movies", "allow_local_network": true, "priority": 2})
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", string(raw), 201), &d)
	if d.ID == 0 || d.Protocol != "usenet" || d.Secret != secrets.Masked || d.Priority != 2 || d.Health.State != "working" {
		t.Fatalf("added %+v", d)
	}
	do(t, "POST", srv.URL+"/api/v1/downloaders", string(raw), 409) // name used
	list := string(do(t, "GET", srv.URL+"/api/v1/downloaders", "", 200))
	if strings.Contains(list, key) || !strings.Contains(list, "Lab SABnzbd") {
		t.Fatalf("list %s", list)
	}
	id := itoa(d.ID)

	var res struct {
		OK     bool   `json:"ok"`
		Text   string `json:"text"`
		Health struct {
			State string `json:"state"`
		} `json:"health"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if !res.OK || res.Text != "It answered: sabnzbd 5.1.3." {
		t.Fatalf("test %+v", res)
	}
	// The masked key keeps the stored one; a wrong key is said in plain words.
	keep, wrong := secrets.Masked, "not-the-key"
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, body(map[string]any{"name": "Lab SABnzbd", "secret": keep, "category": "movies"}), 200)
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, body(map[string]any{"name": "Lab SABnzbd", "secret": wrong}), 200)
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if res.OK || !strings.Contains(res.Text, "turned down the key") || res.Health.State != "sign_in_refused" {
		t.Fatalf("wrong key test %+v", res)
	}
	// A new host needs the key again.
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Lab SABnzbd","url":"http://10.9.9.9:8080"}`, 409)
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Lab SABnzbd","secret":"`+key+`"}`, 200)
	// A category SABnzbd lacks.
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, `{"name":"Lab SABnzbd","category":"holyarrs"}`, 200)
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if res.OK || !strings.Contains(res.Text, "category does not exist") {
		t.Fatalf("category test %+v", res)
	}

	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/pause", "", 200), &d)
	if d.Health.State != "paused" {
		t.Fatalf("paused %+v", d.Health)
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/resume", "", 200), &d)
	if d.Health.State != "working" {
		t.Fatalf("resumed %+v", d.Health)
	}

	// The key never reaches the database in the clear.
	var n int
	_ = s.Read().QueryRow("SELECT count(*) FROM secrets WHERE value LIKE ?", "%"+key+"%").Scan(&n)
	if n != 0 {
		t.Fatal("key stored in the clear")
	}
	do(t, "DELETE", srv.URL+"/api/v1/downloaders/"+id, "", 204)
	do(t, "DELETE", srv.URL+"/api/v1/downloaders/"+id, "", 404)
	do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 404)
}

func TestDownloaderOnTheLocalNetworkNeedsTheSwitch(t *testing.T) {
	srv, _ := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab := fakeSAB(t, key)
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", `{"name":"S","kind":"sabnzbd","url":"`+sab.URL+`","secret":"`+key+`","category":"movies"}`, 201), &d)
	var res struct {
		OK   bool   `json:"ok"`
		Text string `json:"text"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+itoa(d.ID)+"/test", "", 200), &res)
	if res.OK || !strings.Contains(res.Text, "May reach the local network") {
		t.Fatalf("test %+v", res)
	}
}

// fakeQbitAPI is enough qBittorrent for a delete.
type fakeQbitAPI struct {
	mu      sync.Mutex
	deleted []string
}

// route: POST /api/v1/downloads/{id}/mark-failed
func TestMarkFailedRoute(t *testing.T) {
	srv, s := dlServer(t)
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
		lf := &domain.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		ti := &domain.Title{Kind: "movie", GateDriver: "none", Name: "Salt Choir", SortName: "salt choir", Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active"}
		for _, x := range []struct {
			table string
			v     any
		}{{"library_folders", lf}, {"titles", ti}} {
			if _, err := store.Insert(tx, x.table, x.v, domain.System); err != nil {
				return err
			}
		}
		it := &domain.Item{TitleID: ti.ID, Kind: "movie"}
		if _, err := store.Insert(tx, "items", it, domain.System); err != nil {
			return err
		}
		ver := &domain.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Salt Choir", IsMain: true}
		if _, err := store.Insert(tx, "versions", ver, domain.System); err != nil {
			return err
		}
		sl := &domain.Slot{ItemID: it.ID, VersionID: ver.ID, State: domain.SlotDownloading}
		if _, err := store.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := store.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		rel := &domain.Release{SourceID: src.ID, Name: "Salt.Choir.2020.1080p.WEB.H264-HOLYARRS", LinkRef: "http://x/a.torrent", Protocol: "torrent", Size: 1,
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := store.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: qb.URL, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := store.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		dl := &domain.Download{DownloaderID: dr.ID, ReleaseID: &rel.ID, SlotIDs: []int64{sl.ID}, ExternalID: "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd", State: "downloading", Progress: 0.3,
			FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		if _, err := store.Insert(tx, "downloads", dl, domain.System); err != nil {
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
	var tv TitleView
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles/1", "", 200), &tv)
	if len(tv.Slots) != 1 || tv.Slots[0].Download == nil || tv.Slots[0].Download.Label != "Downloading" || !tv.Slots[0].Download.Active {
		t.Fatalf("slot %+v", tv.Slots)
	}
	var dv DownloadView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloads/"+itoa(dlID)+"/mark-failed", "", 200), &dv)
	if dv.State != "failed" || dv.Active || !strings.HasPrefix(dv.Text, "Marked failed") {
		t.Fatalf("marked %+v", dv)
	}
	if len(f.deleted) != 1 || f.deleted[0] != "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd" {
		t.Fatalf("not removed from the Downloader: %v", f.deleted)
	}
	var st string
	_ = s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", slotID).Scan(&st)
	if st != string(domain.SlotFailed) {
		t.Fatalf("slot %s", st)
	}
	do(t, "POST", srv.URL+"/api/v1/downloads/"+itoa(dlID)+"/mark-failed", "", 409)
	do(t, "POST", srv.URL+"/api/v1/downloads/999/mark-failed", "", 404)
}

// route: GET /api/v1/settings/downloaders
// route: PUT /api/v1/settings/downloaders
func TestDownloaderSettings(t *testing.T) {
	srv, _ := dlServer(t)
	var st DownloaderSettings
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/settings/downloaders", "", 200), &st)
	if st.StalledAfterHours != 6 || st.StalledActHours != 24 || st.GrabSpacingSeconds != 2 || !st.RemoveFailed || !st.FailDangerous || !st.ReplaceVanished || !st.TryNextDownloader {
		t.Fatalf("defaults %+v", st)
	}
	body := `{"stalled_after_hours":3,"stalled_act_hours":12,"remove_failed":false,"fail_dangerous":true,"replace_vanished":false,"grab_spacing_seconds":5,"try_next_downloader":false}`
	_ = json.Unmarshal(do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", body, 200), &st)
	if st.StalledAfterHours != 3 || st.StalledActHours != 12 || st.RemoveFailed || st.ReplaceVanished || st.GrabSpacingSeconds != 5 || st.TryNextDownloader {
		t.Fatalf("saved %+v", st)
	}
	do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{"stalled_after_hours":10,"stalled_act_hours":5,"grab_spacing_seconds":2}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{"stalled_after_hours":0,"stalled_act_hours":5,"grab_spacing_seconds":2}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/downloaders", `{"stalled_after_hours":6,"stalled_act_hours":24,"grab_spacing_seconds":99}`, 400)
}

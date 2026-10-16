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
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/live"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

type dl41 struct {
	srv                 *httptest.Server
	s                   *store.Store
	q                   *jobs.Queue
	dlID, slotID, relID int64
	mu                  sync.Mutex
	deleted             []string
}

// seed41 makes one movie with a Slot in slotState, its Release, a fake
// qBittorrent and a Download in dlState.
func seed41(t *testing.T, slotState domain.SlotState, dlState string) *dl41 {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
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
		lf := &domain.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		y := 2020
		ti := &domain.Title{Kind: "movie", GateDriver: "none", Name: "Salt Choir", SortName: "salt choir", Year: &y, Aliases: []string{}, SourceIDs: map[string]string{},
			MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "released", State: "active", KeepUpToDate: true}
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
		ver := &domain.Version{TitleID: ti.ID, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "Salt Choir", IsMain: true, KeepUpToDate: true}
		if _, err := store.Insert(tx, "versions", ver, domain.System); err != nil {
			return err
		}
		sl := &domain.Slot{ItemID: it.ID, VersionID: ver.ID, State: slotState}
		if slotState == domain.SlotNeedsADecision {
			r := "the download failed and no other Release passed the Pick rules (3 found, 3 refused)"
			sl.LastReason = &r
		}
		if _, err := store.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}
		if _, err := store.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		rel := &domain.Release{SourceID: src.ID, Name: "Salt.Choir.2020.1080p.WEB.H264-HOLYARRS", LinkRef: "http://x/a.torrent", Protocol: "torrent", Size: 1000,
			Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: []int64{sl.ID}, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := store.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		dr := &domain.Downloader{Name: "Q", Adapter: "qbittorrent", Protocol: "torrent", URL: qb.URL, Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if _, err := store.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		eta := int64(90)
		dl := &domain.Download{DownloaderID: dr.ID, ReleaseID: &rel.ID, SlotIDs: []int64{sl.ID}, ExternalID: "abcdabcdabcdabcdabcdabcdabcdabcdabcdabcd", State: dlState, Progress: 0.3,
			Speed: 2048, ETA: &eta, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		if _, err := store.Insert(tx, "downloads", dl, domain.System); err != nil {
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
	x.srv = httptest.NewServer(New(Server{Store: s, Queue: x.q, Live: live.NewHub(live.Options{Heartbeat: time.Second}), Metadata: metadata.Lab(), Version: "test"}).Handler())
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

func exec41(t *testing.T, s *store.Store, q string, args ...any) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// route: GET /api/v1/downloads/view
func TestDownloadsView(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	var out struct {
		Downloads []DownloadRow `json:"downloads"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &out)
	if len(out.Downloads) != 1 {
		t.Fatalf("%+v", out)
	}
	r := out.Downloads[0]
	if r.Phase != "downloading" || r.Label != "Downloading" || r.Title != "Salt Choir" || r.Year == nil || *r.Year != 2020 || r.Version != "HD" ||
		r.Downloader != "Q" || r.Name != "Salt.Choir.2020.1080p.WEB.H264-HOLYARRS" || r.Speed != 2048 || r.ETA == nil || !r.CanAct {
		t.Fatalf("row %+v", r)
	}
	// Once the Downloader is done, the Slot says where it is.
	for _, c := range []struct{ dl, slot, phase, label string }{
		{"finished", "downloading", "waiting_to_import", "Waiting to import"},
		{"finished", "importing", "importing", "Importing"},
		{"seeding", "in_library", "done", "Done"},
		{"finished", "needs_a_decision", "needs_a_decision", "Needs a decision"},
		{"vanished", "failed", "vanished", "Vanished"},
		{"stalled", "stalled", "stalled", "Stalled"},
		{"failed", "failed", "failed", "Failed"},
		{"queued", "downloading", "waiting", "Waiting its turn"},
	} {
		exec41(t, x.s, "UPDATE downloads SET state = ?", c.dl)
		exec41(t, x.s, "UPDATE slots SET state = ?", c.slot)
		_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view?limit=10", "", 200), &out)
		if r := out.Downloads[0]; r.Phase != c.phase || r.Label != c.label || r.CanAct != (c.dl == "stalled" || c.dl == "queued") {
			t.Errorf("%s/%s: %s %q %v", c.dl, c.slot, r.Phase, r.Label, r.CanAct)
		}
	}
}

// route: POST /api/v1/downloads/{id}/remove
func TestRemoveDownloadRoute(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	do(t, "POST", x.srv.URL+"/api/v1/downloads/"+itoa(x.dlID)+"/remove", `{"delete_files":"yes"}`, 400)
	// Defaults: delete files, keep the Release, the movie goes back to wanted.
	var v DownloadView
	_ = json.Unmarshal(do(t, "POST", x.srv.URL+"/api/v1/downloads/"+itoa(x.dlID)+"/remove", "", 200), &v)
	if v.State != "cleaned" || v.Label != "Removed" || v.Active || !strings.HasPrefix(v.Text, "Removed") {
		t.Fatalf("removed %+v", v)
	}
	if d := x.removed(); len(d) != 1 || !strings.HasSuffix(d[0], "|true") {
		t.Fatalf("deleted %v", d)
	}
	if st := x.slotState(t); st != "wanted" {
		t.Fatalf("slot %s", st)
	}
	do(t, "POST", x.srv.URL+"/api/v1/downloads/"+itoa(x.dlID)+"/remove", "", 409)
	do(t, "POST", x.srv.URL+"/api/v1/downloads/999/remove", "", 404)

	// Choices given: keep the files, turn the Release down, no search.
	y := seed41(t, domain.SlotDownloading, "stalled")
	do(t, "POST", y.srv.URL+"/api/v1/downloads/"+itoa(y.dlID)+"/remove", `{"delete_files":false,"turn_down":true,"search_again":false}`, 200)
	var rs string
	_ = y.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", y.relID).Scan(&rs)
	if d := y.removed(); rs != "turned_down" || len(d) != 1 || !strings.HasSuffix(d[0], "|false") || y.slotState(t) != "wanted" {
		t.Fatalf("release %s deleted %v slot %s", rs, d, y.slotState(t))
	}
}

// route: GET /api/v1/needs-decision
func TestNeedsDecisionList(t *testing.T) {
	x := seed41(t, domain.SlotNeedsADecision, "failed")
	var out struct {
		Items []DecisionView `json:"items"`
		Count int            `json:"count"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/needs-decision", "", 200), &out)
	if out.Count != 1 || len(out.Items) != 1 {
		t.Fatalf("%+v", out)
	}
	it := out.Items[0]
	if it.SlotID != x.slotID || it.Title != "Salt Choir" || it.Version != "HD" || it.Versions != 1 || it.Why != "ran_out" ||
		!strings.HasPrefix(it.Reason, "The download failed") || it.ReleaseID == nil || it.ReleaseName == "" || it.HasCopy || it.Since.IsZero() {
		t.Fatalf("item %+v", it)
	}
	exec41(t, x.s, "UPDATE slots SET state = 'wanted'")
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/needs-decision", "", 200), &out)
	if out.Count != 0 || len(out.Items) != 0 {
		t.Fatalf("%+v", out)
	}
}

// route: POST /api/v1/needs-decision/{id}/decide
func TestDecideRoute(t *testing.T) {
	x := seed41(t, domain.SlotNeedsADecision, "failed")
	u := x.srv.URL + "/api/v1/needs-decision/" + itoa(x.slotID) + "/decide"
	do(t, "POST", u, `{"action":"dance"}`, 400)
	do(t, "POST", x.srv.URL+"/api/v1/needs-decision/999/decide", `{"action":"search"}`, 404)
	var out struct {
		JobID int64 `json:"job_id"`
	}
	_ = json.Unmarshal(do(t, "POST", u, `{"action":"search"}`, 202), &out)
	var params string
	_ = x.s.Read().QueryRow("SELECT params FROM jobs WHERE id = ?", out.JobID).Scan(&params)
	if out.JobID == 0 || !strings.Contains(params, "by_hand") {
		t.Fatalf("job %d %s", out.JobID, params)
	}
	// Turn down: the last Release is turned down and a search queued.
	do(t, "POST", u, `{"action":"turn_down"}`, 202)
	var rs string
	_ = x.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", x.relID).Scan(&rs)
	if rs != "turned_down" {
		t.Fatalf("release %s", rs)
	}
	// Stop looking: Keep up to date off for the Version, the Slot rests.
	do(t, "POST", u, `{"action":"stop"}`, 200)
	var keep bool
	_ = x.s.Read().QueryRow("SELECT keep_up_to_date FROM versions LIMIT 1").Scan(&keep)
	if keep || x.slotState(t) != "wanted" {
		t.Fatalf("keep %v slot %s", keep, x.slotState(t))
	}
	do(t, "POST", u, `{"action":"stop"}`, 404)
	// No Release to turn down.
	y := seed41(t, domain.SlotNeedsADecision, "failed")
	exec41(t, y.s, "UPDATE downloads SET release_id = NULL")
	do(t, "POST", y.srv.URL+"/api/v1/needs-decision/"+itoa(y.slotID)+"/decide", `{"action":"turn_down"}`, 409)
}

// 4.1.3 minor 8: Stop looking on a Slot that moved on meanwhile is refused
// and leaves Keep up to date as it was.
func TestDecideStopRefusesSlotThatMovedOn(t *testing.T) {
	x := seed41(t, domain.SlotNeedsADecision, "failed")
	exec41(t, x.s, "UPDATE versions SET keep_up_to_date = 1")
	beforeDecide = func() { exec41(t, x.s, "UPDATE slots SET state = 'searching'") }
	t.Cleanup(func() { beforeDecide = nil })
	do(t, "POST", x.srv.URL+"/api/v1/needs-decision/"+itoa(x.slotID)+"/decide", `{"action":"stop"}`, 409)
	var keep bool
	_ = x.s.Read().QueryRow("SELECT keep_up_to_date FROM versions LIMIT 1").Scan(&keep)
	if !keep || x.slotState(t) != "searching" {
		t.Fatalf("keep %v slot %s", keep, x.slotState(t))
	}
}

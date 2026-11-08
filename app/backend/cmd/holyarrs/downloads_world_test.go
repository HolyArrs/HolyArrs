package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// The view reads the reader's snapshot only: no
// client call, no database write, no matcher start; it carries the revs
// and pages outside rows as before.
func TestDownloadsViewFromStore(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	var calls atomic.Int64
	qb := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	t.Cleanup(qb.Close)
	pointQB(t, x, qb.URL)
	var dlr int64
	_ = x.s.Read().QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&dlr)
	exec41(t, x.s, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('links', 'downloads_sync', 'on', '', '{}', '', '{}')`)

	sy := &downloads.Sync{Store: &downloads.Store{}, DB: x.s, Manager: noClient{x.api.DownloadsWeb().Manager()}}
	x.api.DownloadsWeb().DLSync = sy
	var jobs []downloads.Job
	for i := range 250 {
		jobs = append(jobs, downloads.Job{ID: fmt.Sprintf("%040x", i+1), Name: fmt.Sprintf("Outside.%d", i), Category: "linux", Size: 100, Progress: 1,
			UpSpeed: 10, State: downloads.StateFinished, Detail: "uploading", AddedAt: int64(1700000000 + i), Ratio: 1.5, LastActivity: int64(1700100000 + i)})
	}
	sy.Store.Load(dlr, jobs)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { sy.Around(ctx, func(ctx context.Context) { <-ctx.Done() }); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	for !sy.On() {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond) // let the reader's first (failed) read pass

	conn, err := x.s.Read().Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	version := func() int64 {
		var v int64
		if err := conn.QueryRowContext(context.Background(), "PRAGMA data_version").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	calls.Store(0)
	before := version()
	var first struct {
		viewAnswer
		Revs map[string]uint64 `json:"revs"`
	}
	start := time.Now()
	for i := range 5 {
		b := do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200)
		if i == 0 {
			if err := json.Unmarshal(b, &first); err != nil {
				t.Fatal(err)
			}
		}
	}
	if took := time.Since(start) / 5; took > 300*time.Millisecond {
		t.Fatalf("a view took %v", took)
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("the view made %d client calls", n)
	}
	if after := version(); after != before {
		t.Fatalf("the view wrote to the database (data_version %d -> %d)", before, after)
	}
	if n := count(t, x.s, "SELECT COUNT(*) FROM dl_match_queue"); n != 0 {
		t.Fatalf("the view queued %d matches", n)
	}
	if first.OutsideTotal != 250 || first.NextCursor != "200" || len(first.Downloads) != 201 {
		t.Fatalf("outside %d cursor %q rows %d", first.OutsideTotal, first.NextCursor, len(first.Downloads))
	}
	if first.Revs[fmt.Sprint(dlr)] == 0 {
		t.Fatalf("revs %v", first.Revs)
	}
	if len(first.Clients) != 1 || first.Clients[0].Health != "checking" || !first.Clients[0].Stale {
		t.Fatalf("client health %+v, want checking (last known rows)", first.Clients)
	}
	if first.Totals.UpSpeed != 0 {
		// Last-known rows carry no speeds: they are not current.
		t.Fatalf("upload total %d, want 0", first.Totals.UpSpeed)
	}
	if !sy.Store.Viewer() {
		t.Fatal("a view did not count as a viewer")
	}
	// The next page from the cursor.
	v := view(t, x.srv.URL+"/api/v1/downloads/view?cursor=200")
	if len(v.Downloads) != 51 || v.NextCursor != "" {
		t.Fatalf("page 2: rows %d cursor %q", len(v.Downloads), v.NextCursor)
	}
	// One client's page only (a rev gap on that client): its rows and its rev.
	var one struct {
		viewAnswer
		Revs map[string]uint64 `json:"revs"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+fmt.Sprintf("/api/v1/downloads/view?client=%d", dlr), "", 200), &one)
	if len(one.Downloads) != 201 || len(one.Revs) != 1 || one.Revs[fmt.Sprint(dlr)] != first.Revs[fmt.Sprint(dlr)] {
		t.Fatalf("client page: rows %d revs %v", len(one.Downloads), one.Revs)
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view?client=999999", "", 200), &one)
	if len(one.Downloads) != 0 {
		t.Fatalf("other client page: rows %d, want 0", len(one.Downloads))
	}
	// The torrent detail's last activity comes from the reader's snapshot,
	// not the empty list cache.
	var td downloads.TorrentDetail
	x.api.DownloadsWeb().TorrentTimeline(context.Background(), dlr, fmt.Sprintf("%040X", 3), &td, nil)
	if td.LastActivity == nil || td.LastActivity.Unix() != 1700100002 {
		t.Fatalf("last activity %v, want from the reader", td.LastActivity)
	}
	// The health page names the client the reader is checking again.
	var hc struct {
		Sync []downloads.ClientHealth `json:"sync"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/health/checks", "", 200), &hc)
	if len(hc.Sync) != 1 || hc.Sync[0].ID != dlr || hc.Sync[0].Health != "checking" {
		t.Fatalf("health sync %+v, want checking", hc.Sync)
	}
	sy.Store.SetFallback(dlr, true)
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/health/checks", "", 200), &hc)
	if len(hc.Sync) != 1 || hc.Sync[0].Health != "degraded" {
		t.Fatalf("health sync %+v, want degraded", hc.Sync)
	}
}

// route: DELETE /api/v1/turned-down/{id}
// A failed download turns its copy down on its own; the
// copy is on the Turned down list with why, and Undo lets it be picked again.
func TestFailedCopyTurnedDownThenUndone(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	do(t, "POST", x.srv.URL+"/api/v1/downloads/"+itoa(x.dlID)+"/mark-failed", "", 200)
	// The search itself is the Manager's OnFailed (wired in main; covered in
	// internal/downloader); here the Slot is handed to it as failed.
	if st := x.slotState(t); st != string(mediafiles.SlotFailed) {
		t.Fatalf("slot %s", st)
	}
	var list struct {
		Releases []sources.ReleaseView `json:"releases"`
		Total    int                   `json:"total"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/turned-down", "", 200), &list)
	if list.Total != 1 || list.Releases[0].ID != x.relID || list.Releases[0].Name == "" {
		t.Fatalf("turned down %+v", list)
	}
	why := false
	for _, r := range list.Releases[0].Reasons {
		why = why || r.Code == "turned_down.reason"
	}
	if !why {
		t.Fatalf("no reason shown: %+v", list.Releases[0].Reasons)
	}
	do(t, "DELETE", x.srv.URL+"/api/v1/turned-down/"+itoa(x.relID), "", 200)
	var st string
	_ = x.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", x.relID).Scan(&st)
	if st == "turned_down" {
		t.Fatal("still turned down after Undo")
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/turned-down", "", 200), &list)
	if list.Total != 0 {
		t.Fatalf("still listed %+v", list)
	}
}

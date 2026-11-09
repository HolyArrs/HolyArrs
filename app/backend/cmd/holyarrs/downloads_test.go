package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestDownloadsView(t *testing.T) {
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	var out struct {
		Downloads []downloads.DownloadRow `json:"downloads"`
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
	// Upload speed, seeds and peers come through, and the totals add up what is moving.
	exec41(t, x.s, "UPDATE downloads SET up_speed = 512, seeds = 40, peers = 7")
	var rates struct {
		Downloads []downloads.DownloadRow  `json:"downloads"`
		Totals    downloads.DownloadTotals `json:"totals"`
	}
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &rates)
	if rr := rates.Downloads[0]; rr.UpSpeed != 512 || rr.Seeds != 40 || rr.Peers != 7 || rates.Totals.Speed != 2048 || rates.Totals.UpSpeed != 512 {
		t.Fatalf("rates %+v totals %+v", rates.Downloads[0], rates.Totals)
	}
	// Once the Downloader is done, the Slot says where it is.
	for _, c := range []struct{ dl, slot, phase, label string }{
		{"finished", "downloading", "waiting_to_import", "Waiting to import"},
		{"finished", "importing", "importing", "Importing"},
		{"seeding", "in_library", "done", "Done"},
		{"finished", "needs_a_decision", "needs_a_decision", "Review"},
		{"vanished", "failed", "vanished", "Failed"},
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

func TestRemoveDownloadRoute(t *testing.T) {
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	do(t, "POST", x.srv.URL+"/api/v1/downloads/"+dlItoa(x.dlID)+"/remove", `{"delete_files":"yes"}`, 400)
	// Defaults: delete files, keep the Release, the movie goes back to wanted.
	var v downloads.DownloadView
	_ = json.Unmarshal(do(t, "POST", x.srv.URL+"/api/v1/downloads/"+dlItoa(x.dlID)+"/remove", "", 200), &v)
	if v.State != "cleaned" || v.Label != "Removed" || v.Active || !strings.HasPrefix(v.Text, "Removed") {
		t.Fatalf("removed %+v", v)
	}
	if d := x.removed(); len(d) != 1 || !strings.HasSuffix(d[0], "|true") {
		t.Fatalf("deleted %v", d)
	}
	if st := x.slotState(t); st != "wanted" {
		t.Fatalf("slot %s", st)
	}
	do(t, "POST", x.srv.URL+"/api/v1/downloads/"+dlItoa(x.dlID)+"/remove", "", 409)
	do(t, "POST", x.srv.URL+"/api/v1/downloads/999/remove", "", 404)

	// Choices given: keep the files, turn the Release down, no search.
	y := seed41(t, mediafiles.SlotDownloading, "stalled")
	do(t, "POST", y.srv.URL+"/api/v1/downloads/"+dlItoa(y.dlID)+"/remove", `{"delete_files":false,"turn_down":true,"search_again":false}`, 200)
	var rs string
	_ = y.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", y.relID).Scan(&rs)
	if d := y.removed(); rs != "turned_down" || len(d) != 1 || !strings.HasSuffix(d[0], "|false") || y.slotState(t) != "wanted" {
		t.Fatalf("release %s deleted %v slot %s", rs, d, y.slotState(t))
	}
}

// route: GET /api/v1/needs-decision
func TestNeedsDecisionList(t *testing.T) {
	x := seed41(t, mediafiles.SlotNeedsADecision, "failed")
	var out struct {
		Items []downloads.DecisionView `json:"items"`
		Count int                      `json:"count"`
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

func TestDecideRoute(t *testing.T) {
	x := seed41(t, mediafiles.SlotNeedsADecision, "failed")
	u := x.srv.URL + "/api/v1/needs-decision/" + dlItoa(x.slotID) + "/decide"
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
	y := seed41(t, mediafiles.SlotNeedsADecision, "failed")
	exec41(t, y.s, "UPDATE downloads SET release_id = NULL")
	do(t, "POST", y.srv.URL+"/api/v1/needs-decision/"+dlItoa(y.slotID)+"/decide", `{"action":"turn_down"}`, 409)
}

// 4.1.3 minor 8: Stop looking on a Slot that moved on meanwhile is refused
// and leaves Keep up to date as it was.
func TestDecideStopRefusesSlotThatMovedOn(t *testing.T) {
	x := seed41(t, mediafiles.SlotNeedsADecision, "failed")
	exec41(t, x.s, "UPDATE versions SET keep_up_to_date = 1")
	downloads.BeforeDecide = func() { exec41(t, x.s, "UPDATE slots SET state = 'searching'") }
	t.Cleanup(func() { downloads.BeforeDecide = nil })
	do(t, "POST", x.srv.URL+"/api/v1/needs-decision/"+dlItoa(x.slotID)+"/decide", `{"action":"stop"}`, 409)
	var keep bool
	_ = x.s.Read().QueryRow("SELECT keep_up_to_date FROM versions LIMIT 1").Scan(&keep)
	if !keep || x.slotState(t) != "searching" {
		t.Fatalf("keep %v slot %s", keep, x.slotState(t))
	}
}

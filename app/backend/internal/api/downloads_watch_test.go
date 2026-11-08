package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// viewAs reads the Downloads rows as one person, past the sign-in.
func viewAs(t *testing.T, x *dl41, who auth.Who) []DownloadRow {
	t.Helper()
	return viewAt(t, x, who, "/api/v1/downloads/view")
}

// viewAt is viewAs at a given address (?asked=1 for the Requests page).
func viewAt(t *testing.T, x *dl41, who auth.Who, url string) []DownloadRow {
	t.Helper()
	a := New(Server{Store: x.s, Queue: x.q, Live: webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second}), Metadata: metadata.Lab(), Version: "test"})
	r := httptest.NewRequest("GET", url, nil)
	r = r.WithContext(auth.ContextWith(r.Context(), who))
	w := httptest.NewRecorder()
	a.downloadsView(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var out struct {
		Downloads []DownloadRow `json:"downloads"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out.Downloads
}

// Everyone signed in may open Downloads; someone who may not manage the
// Library sees only the downloads for titles they asked for, with no actions.
func TestDownloadsViewOwnAsksOnly(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	exec41(t, x.s, `INSERT INTO users(id, created_at, created_by, updated_at, updated_by, name, role, password_hash) VALUES
		(1, 't', '{}', 't', '{}', 'owner', 'owner', 'x'), (2, 't', '{}', 't', '{}', 'ana', 'member', 'x'), (3, 't', '{}', 't', '{}', 'bo', 'member', 'x')`)
	exec41(t, x.s, `INSERT INTO asks (id, created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, title_id, asked_by, asked_at)
		SELECT 1, 't', '{}', 't', '{}', 'movie', '1', 'Salt Choir', 'approved', t.id, 2, 't' FROM titles t LIMIT 1`)
	exec41(t, x.s, `INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (1, 2, 't')`)

	if got := auth.RoutePerms["GET /api/v1/downloads/view"]; !(auth.Who{Role: auth.RoleMember, Perms: []auth.Perm{auth.PermAsk}}).Has(got) {
		t.Fatalf("a member cannot open Downloads (%q)", got)
	}
	member := func(id int64) auth.Who {
		return auth.Who{UserID: id, Role: auth.RoleMember, Perms: []auth.Perm{auth.PermAsk}, Via: "session"}
	}
	mine := viewAs(t, x, member(2))
	if len(mine) != 1 || mine[0].CanAct {
		t.Fatalf("asker rows %+v", mine)
	}
	if other := viewAs(t, x, member(3)); len(other) != 0 {
		t.Fatalf("someone else's download shown: %+v", other)
	}
	admin := viewAs(t, x, auth.Who{UserID: 1, Role: auth.RoleAdmin, Perms: []auth.Perm{auth.PermManageLibrary}, Via: "session"})
	if len(admin) != 1 || !admin[0].CanAct {
		t.Fatalf("admin rows %+v", admin)
	}
	// #139: someone who approves (but does not run the Library) reads every
	// approved ask's downloads on the Requests page, as plain rows with no actions.
	approver := auth.Who{UserID: 3, Role: auth.RoleMember, Perms: []auth.Perm{auth.PermAsk, auth.PermApprove}, Via: "session"}
	if rows := viewAt(t, x, approver, "/api/v1/downloads/view?asked=1"); len(rows) != 1 || rows[0].CanAct {
		t.Fatalf("approver's Requests rows %+v", rows)
	}
	if rows := viewAs(t, x, approver); len(rows) != 0 {
		t.Fatalf("approver's own Downloads page shows someone else's download: %+v", rows)
	}
	// Every action and the swarm stay with people who manage the Library.
	for _, p := range []string{"POST /api/v1/downloads/{id}/mark-failed", "POST /api/v1/downloads/{id}/remove", "POST /api/v1/downloads/{id}/pause",
		"POST /api/v1/downloads/{id}/resume", "POST /api/v1/downloads/{id}/top", "POST /api/v1/downloads/{id}/recheck", "POST /api/v1/downloads/pause-all",
		"POST /api/v1/downloads/resume-all", "GET /api/v1/downloads/{id}/swarm", "DELETE /api/v1/turned-down/{id}", "POST /api/v1/needs-decision/{id}/decide"} {
		if member(2).Has(auth.RoutePerms[p]) {
			t.Errorf("a member may use %s", p)
		}
	}
}

// A failed copy whose Slot moved on (searching again, or another copy on
// its way) is no problem any more; one with nothing left still is.
func TestDownloadsViewFailedReplaced(t *testing.T) {
	x := seed41(t, domain.SlotSearching, "failed")
	owner := auth.Who{UserID: 1, Role: auth.RoleOwner, Via: "session"}
	if r := viewAs(t, x, owner); len(r) != 1 || r[0].Phase != "failed" || !r[0].Replaced {
		t.Fatalf("searching again: %+v", r)
	}
	exec41(t, x.s, "UPDATE slots SET state = 'needs_a_decision'")
	if r := viewAs(t, x, owner); len(r) != 1 || r[0].Replaced {
		t.Fatalf("nothing left: %+v", r)
	}
	exec41(t, x.s, "UPDATE slots SET state = 'failed'")
	if r := viewAs(t, x, owner); r[0].Replaced {
		t.Fatalf("just failed: %+v", r)
	}
}

// route: DELETE /api/v1/turned-down/{id}
// A failed download turns its copy down on its own; the
// copy is on the Turned down list with why, and Undo lets it be picked again.
func TestFailedCopyTurnedDownThenUndone(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	do(t, "POST", x.srv.URL+"/api/v1/downloads/"+itoa(x.dlID)+"/mark-failed", "", 200)
	// The search itself is the Manager's OnFailed (wired in main; covered in
	// internal/downloader); here the Slot is handed to it as failed.
	if st := x.slotState(t); st != string(domain.SlotFailed) {
		t.Fatalf("slot %s", st)
	}
	var list struct {
		Releases []ReleaseView `json:"releases"`
		Total    int           `json:"total"`
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

// route: GET /api/v1/downloads/speeds
func TestDownloadsSpeedsRoute(t *testing.T) {
	x := seed41(t, domain.SlotDownloading, "downloading")
	var out struct {
		Samples []map[string]any `json:"samples"`
	}
	if err := json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/speeds", "", 200), &out); err != nil {
		t.Fatal(err)
	}
}

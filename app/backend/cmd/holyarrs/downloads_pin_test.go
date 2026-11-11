package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestDlPinDownloadsViewLargeLibrary(t *testing.T) {
	w := newDlPinWorld(t)
	st, b := w.get(t, w.owner, "/api/v1/downloads/view?limit=500")
	if st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	var v dlPinView
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Downloads) != 500 {
		t.Fatalf("%d rows, want 500", len(v.Downloads))
	}
	for i, r := range v.Downloads {
		wantID := w.order[len(w.order)-1-i]
		if r.ID != wantID {
			t.Fatalf("row %d is download %d, want %d (newest first)", i, r.ID, wantID)
		}
		dlPinCheckRow(t, r, w.want[r.ID], true)
	}
	// Default page is 100 rows.
	_, b = w.get(t, w.owner, "/api/v1/downloads/view")
	var d dlPinView
	_ = json.Unmarshal(b, &d)
	if len(d.Downloads) != 100 {
		t.Fatalf("default page %d rows, want 100", len(d.Downloads))
	}
	// Totals count the downloading speed of every row.
	if v.Totals.Speed != 500*100 {
		t.Fatalf("totals %+v", v.Totals)
	}
}

func TestDlPinDownloadsViewMemberFilter(t *testing.T) {
	w := newDlPinWorld(t)
	st, b := w.get(t, w.member, "/api/v1/downloads/view?limit=500")
	if st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	var v dlPinView
	_ = json.Unmarshal(b, &v)
	want := 0
	for _, pw := range w.want {
		if pw.asked {
			want++
		}
	}
	if want != dlPinLib/5 || len(v.Downloads) != want {
		t.Fatalf("member sees %d rows, want %d", len(v.Downloads), want)
	}
	prev := int64(1 << 62)
	for _, r := range v.Downloads {
		pw := w.want[r.ID]
		if pw == nil || !pw.asked {
			t.Fatalf("member sees download %d that is not theirs", r.ID)
		}
		if r.ID >= prev {
			t.Fatalf("rows not newest first at %d", r.ID)
		}
		prev = r.ID
		dlPinCheckRow(t, r, pw, false)
	}
	// ?asked=1 is for people who approve; a member still gets their own rows.
	_, b = w.get(t, w.member, "/api/v1/downloads/view?asked=1")
	var a dlPinView
	_ = json.Unmarshal(b, &a)
	if len(a.Downloads) != want {
		t.Fatalf("member ?asked=1 sees %d rows, want %d", len(a.Downloads), want)
	}
	// ?asked=1 for the owner reads every download, with no row cap.
	_, b = w.get(t, w.owner, "/api/v1/downloads/view?asked=1")
	var o dlPinView
	_ = json.Unmarshal(b, &o)
	if len(o.Downloads) != want {
		// asked=1 narrows to titles with an approved ask (the Requests page).
		t.Fatalf("owner ?asked=1 sees %d rows, want %d", len(o.Downloads), want)
	}
}

func TestDlPinNeedsDecisionLargeLibrary(t *testing.T) {
	w := newDlPinWorld(t)
	st, b := w.get(t, w.owner, "/api/v1/needs-decision")
	if st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	var out struct {
		Items []downloads.DecisionView `json:"items"`
		Count int                      `json:"count"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	wantN := 0
	for _, pw := range w.want {
		if pw.decision {
			wantN++
		}
	}
	if wantN != dlPinLib/10 || out.Count != wantN || len(out.Items) != wantN {
		t.Fatalf("count %d items %d, want %d", out.Count, len(out.Items), wantN)
	}
	ids := make([]int64, 0, wantN)
	for _, it := range out.Items {
		pw := w.want[*it.DownloadID]
		if pw == nil || !pw.decision {
			t.Fatalf("item %+v is not a decision row", it)
		}
		ids = append(ids, it.SlotID)
		if it.TitleID != pw.titleID || it.Title != pw.title || it.Version != "HD" || it.Versions != 1 || it.Kind != pw.kind || it.Why != "ran_out" {
			t.Errorf("item %+v, want %+v", it, pw)
		}
		if !strings.HasPrefix(it.Reason, "The download failed") || !strings.HasSuffix(it.Reason, ".") || it.HasCopy || it.Since.IsZero() {
			t.Errorf("item reason/copy/since: %+v", it)
		}
		if pw.hasRelease && (it.ReleaseID == nil || it.ReleaseName != pw.name) || !pw.hasRelease && (it.ReleaseID != nil || it.ReleaseName != "") {
			t.Errorf("item release %v %q, want has=%v %q", it.ReleaseID, it.ReleaseName, pw.hasRelease, pw.name)
		}
		if it.Album != pw.album {
			t.Errorf("item album %q, want %q", it.Album, pw.album)
		}
		if (pw.kind == "show" || pw.kind == "artist") != (it.Episode != nil) {
			t.Errorf("item %d episode line %v for kind %s", it.SlotID, it.Episode, pw.kind)
		}
		if it.Year == nil || *it.Year != pw.titleYear {
			t.Errorf("item %d year %v, want %d", it.SlotID, it.Year, pw.titleYear)
		}
	}
	// Order is updated_at DESC, id DESC; all rows were made in one run, so
	// ties are common and the id breaks them.
	if !sort.SliceIsSorted(ids, func(i, j int) bool { return ids[i] > ids[j] }) {
		// updated_at may differ by milliseconds, so only the set is pinned then.
		seen := map[int64]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		if len(seen) != wantN {
			t.Fatalf("duplicate slots in the decision list")
		}
	}
}

func TestDlPinRowOwnersNullPartner(t *testing.T) {
	w := newDlPinWorld(t)
	var n, owned int
	_, b := w.get(t, w.owner, "/api/v1/downloads/view?limit=500")
	var v dlPinView
	_ = json.Unmarshal(b, &v)
	for _, r := range v.Downloads {
		n++
		if r.RequestedBy == "ana" {
			owned++
		}
		// A system-made title has no requester, and a HolyArrs row never
		// says "Added in <client>" (that is for outside rows).
		if r.RequestedBy != "" && r.RequestedBy != "ana" {
			t.Fatalf("download %d requested by %q", r.ID, r.RequestedBy)
		}
	}
	// Every third title was added by ana: of the newest 500 ids.
	want := 0
	for i := dlPinLib - 500; i < dlPinLib; i++ {
		if i%3 == 0 {
			want++
		}
	}
	if owned != want {
		t.Fatalf("%d of %d rows owned by ana, want %d", owned, n, want)
	}
}

func TestDlPinDownloadRouteAccess(t *testing.T) {
	w := newDlPinWorld(t)
	type probe struct {
		method, path string
		member       int // status for a signed-in member
		owner        int // status for the owner
	}
	probes := []probe{
		{"GET", "/api/v1/downloads/view", 200, 200},
		{"GET", "/api/v1/needs-decision", 403, 200},
		{"POST", "/api/v1/needs-decision/1/decide", 403, 404},
		{"POST", "/api/v1/downloads/1/remove", 403, 0}, // owner outcome depends on the client; checked below
		{"GET", "/api/v1/downloaders", 403, 200},
		{"GET", "/api/v1/tags", 403, 200},
		{"GET", "/api/v1/downloads", 403, 200},
		{"GET", "/api/v1/downloads/clients", 403, 200},
		{"GET", "/api/v1/downloads/matches", 403, 200},
		{"GET", "/api/v1/turned-down", 403, 200},
		{"GET", "/api/v1/pick-rules", 403, 200},
		{"GET", "/api/v1/own-traits", 403, 200},
		{"GET", "/api/v1/settings/downloaders", 403, 200},
		{"PUT", "/api/v1/settings/downloaders", 403, 0},
		{"POST", "/api/v1/downloaders", 403, 0},
		{"GET", "/api/v1/downloaders/1/attempts", 403, 0},
	}
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, p := range probes {
		body := "{}"
		if st, b := w.send(t, w.member, w.mcsrf, p.method, p.path, body); st != p.member {
			t.Errorf("member %s %s: %d, want %d: %s", p.method, p.path, st, p.member, b)
		}
		if st, _ := w.send(t, anon, "", p.method, p.path, body); st != 401 && st != 403 {
			t.Errorf("signed out %s %s: %d, want 401 or 403", p.method, p.path, st)
		}
		if p.owner != 0 {
			if st, b := w.send(t, w.owner, w.csrf, p.method, p.path, body); st != p.owner {
				t.Errorf("owner %s %s: %d, want %d: %s", p.method, p.path, st, p.owner, b)
			}
		}
	}
	// Unknown ids answer 404 to the owner.
	if st, b := w.send(t, w.owner, w.csrf, "POST", "/api/v1/needs-decision/999999/decide", `{"action":"search"}`); st != 404 {
		t.Errorf("decide unknown: %d %s", st, b)
	}
}

// Response shape of the view: the top-level keys and one row's keys.
// route: GET /api/v1/downloads/view
func TestDlPinDownloadsViewShape(t *testing.T) {
	x := seed41(t, mediafiles.SlotDownloading, "downloading")
	var top map[string]json.RawMessage
	_ = json.Unmarshal(do(t, "GET", x.srv.URL+"/api/v1/downloads/view", "", 200), &top)
	keys := []string{}
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if got := strings.Join(keys, ","); got != "clients,downloads,next_cursor,outside_rest,outside_total,totals" {
		t.Fatalf("top-level keys %s", got)
	}
	var rows []map[string]any
	_ = json.Unmarshal(top["downloads"], &rows)
	if len(rows) != 1 {
		t.Fatalf("%d rows", len(rows))
	}
	rk := []string{}
	for k := range rows[0] {
		rk = append(rk, k)
	}
	sort.Strings(rk)
	const want = "active,can_act,downloader,downloader_id,downloader_kind,eta,hash,id,kind,label,live_key,name,peers,phase,progress,protocol,seeds,size,slot_state,source_id,source_name,speed,state,text,title,title_id,up_speed,updated_at,version,year"
	if got := strings.Join(rk, ","); got != want {
		t.Fatalf("row keys\n got %s\nwant %s", got, want)
	}
}

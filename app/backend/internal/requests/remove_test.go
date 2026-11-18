package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

func askCount(t *testing.T, r *rig, where string) int {
	t.Helper()
	var n int
	if err := r.st.Read().QueryRow("SELECT count(*) FROM asks WHERE " + where).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func setState(t *testing.T, r *rig, id int64, state string) {
	t.Helper()
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE asks SET state = ? WHERE id = ?", state, id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRemoveAnyStateKeepsTitleAndWritesHistory(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	_, _ = r.svc.Ask(ctx, r.bo, "693134")
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	var titles int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM titles").Scan(&titles)
	if err := r.svc.Remove(ctx, res.Ask.ID, r.luis); err != nil {
		t.Fatal(err)
	}
	var after int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM titles").Scan(&after)
	if titles == 0 || after != titles {
		t.Fatalf("titles %d -> %d", titles, after)
	}
	if askCount(t, r, "id = "+strconv.FormatInt(res.Ask.ID, 10)) != 0 {
		t.Fatal("ask still there")
	}
	var people int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM ask_people WHERE ask_id = ?", res.Ask.ID).Scan(&people)
	if people != 0 {
		t.Fatalf("people left %d", people)
	}
	found := false
	for _, e := range r.events(t) {
		if e.Kind == domain.EventAskWithdrawn && strings.Contains(e.Summary, "luis removed the request for") {
			found = true
		}
	}
	if !found {
		t.Fatal("no History line")
	}
	if err := r.svc.Remove(ctx, res.Ask.ID, r.luis); err == nil {
		t.Fatal("removed twice")
	}
}

func TestRemoveEachState(t *testing.T) {
	for _, state := range []string{"asked", "approved", "turned_down", "available"} {
		r := newRig(t)
		ctx := context.Background()
		res, _ := r.svc.Ask(ctx, r.ana, "693134")
		setState(t, r, res.Ask.ID, state)
		if err := r.svc.Remove(ctx, res.Ask.ID, r.luis); err != nil {
			t.Fatalf("%s: %v", state, err)
		}
	}
}

func TestMemberCannotWithdrawAfterAnswer(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	setState(t, r, res.Ask.ID, "turned_down")
	if err := r.svc.Withdraw(ctx, res.Ask.ID, r.ana, false); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("own answered: %v", err)
	}
	if err := r.svc.Withdraw(ctx, res.Ask.ID, r.bo, false); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("other's answered: %v", err)
	}
	if askCount(t, r, "1=1") != 1 {
		t.Fatal("ask gone")
	}
}

func TestRemoveManyByIDsAndFilters(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	var ids []int64
	for _, tmdb := range []string{"693134", "900002", "900003", "900005"} {
		res, err := r.svc.Ask(ctx, r.ana, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, res.Ask.ID)
	}
	setState(t, r, ids[0], "approved") // no title: failed
	setState(t, r, ids[1], "turned_down")
	setState(t, r, ids[2], "available")
	n, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFailed)
	if err != nil || n != 1 || askCount(t, r, "state = 'asked'") != 1 || askCount(t, r, "1=1") != 3 {
		t.Fatalf("failed: %d %v", n, err)
	}
	n, err = r.svc.RemoveMany(ctx, r.luis, nil, RemoveFinished)
	if err != nil || n != 2 || askCount(t, r, "1=1") != 1 {
		t.Fatalf("finished: %d %v", n, err)
	}
	n, err = r.svc.RemoveMany(ctx, r.luis, []int64{ids[3], 9999}, "")
	if err != nil || n != 1 || askCount(t, r, "1=1") != 0 {
		t.Fatalf("ids: %d %v", n, err)
	}
	if _, err := r.svc.RemoveMany(ctx, r.luis, nil, "nope"); err == nil {
		t.Fatal("bad filter accepted")
	}
}

// The "failed" filter takes every ask the Requests page shows as Failed
// (approved with no title, or approved and its title deleted from the
// Library since) and nothing else; its count is the number it removed.
func TestRemoveFailedMatchesTheFailedCards(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	ask := func(tmdb string) int64 {
		res, err := r.svc.Ask(ctx, r.ana, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		return res.Ask.ID
	}
	gone, kept, noTitle, waiting := ask("693134"), ask("900002"), ask("900003"), ask("900005")
	for _, id := range []int64{gone, kept} {
		if a, err := r.svc.Approve(ctx, id, r.luis, ApproveOptions{}); err != nil || a.TitleID == nil {
			t.Fatalf("approve %d: %+v %v", id, a, err)
		}
	}
	setState(t, r, noTitle, "approved")
	vs, _ := r.svc.Views(ctx, "a.id = ?", gone)
	if _, err := r.svc.Library.Remove(ctx, *vs[0].TitleID, library.RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	// The page's rule: approved, and no title or a reason it will not arrive.
	all, err := r.svc.Views(ctx, "1 = 1")
	if err != nil {
		t.Fatal(err)
	}
	want := map[int64]bool{}
	for _, v := range all {
		if v.State == domain.AskApproved && (v.TitleID == nil || v.Problem != "") {
			want[v.ID] = true
		}
	}
	if len(want) != 2 || !want[gone] || !want[noTitle] {
		t.Fatalf("failed cards %v", want)
	}
	titles := askCount(t, r, "title_id IS NOT NULL")
	n, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFailed)
	if err != nil || n != len(want) {
		t.Fatalf("removed %d of %d: %v", n, len(want), err)
	}
	for _, v := range all {
		left := askCount(t, r, "id = "+strconv.FormatInt(v.ID, 10)) == 1
		if left == want[v.ID] {
			t.Fatalf("ask %d (%s) left=%v", v.ID, v.Name, left)
		}
	}
	if askCount(t, r, "id = "+strconv.FormatInt(kept, 10)+" OR id = "+strconv.FormatInt(waiting, 10)) != 2 || titles != 2 {
		t.Fatalf("kept asks or titles changed (%d)", titles)
	}
	var lib int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM titles WHERE state != 'removed'").Scan(&lib)
	if lib != 1 {
		t.Fatalf("titles in the Library %d", lib)
	}
	// Again: nothing left to take.
	if n, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFailed); err != nil || n != 0 {
		t.Fatalf("second run %d %v", n, err)
	}
}

// Clear failed sends the ids it showed. The server keeps only those still
// Failed at that moment: a title added back meanwhile, or an ask now
// waiting, stays. Ids the page saw fail by their download go while the ask
// is still approved. The answer counts what really went.
func TestRemoveFailedIDsKeepsOnlyStillFailed(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	ask := func(tmdb string) int64 {
		res, err := r.svc.Ask(ctx, r.ana, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		return res.Ask.ID
	}
	gone, healed, dl, waiting := ask("693134"), ask("900002"), ask("900003"), ask("900005")
	titles := map[int64]int64{}
	for _, id := range []int64{gone, healed, dl} {
		a, err := r.svc.Approve(ctx, id, r.luis, ApproveOptions{})
		if err != nil || a.TitleID == nil {
			t.Fatalf("approve %d: %+v %v", id, a, err)
		}
		titles[id] = *a.TitleID
	}
	for _, id := range []int64{gone, healed} {
		if _, err := r.svc.Library.Remove(ctx, titles[id], library.RemoveChoice{}, true, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	// The page showed gone, healed and waiting as Failed, and dl as a failed
	// download. Then healed's title came back and waiting was never approved.
	err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'active' WHERE id = ?", titles[healed])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	n, err := r.svc.RemoveMany(ctx, r.luis, []int64{gone, healed, waiting, 9999}, RemoveFailed, dl)
	if err != nil || n != 2 {
		t.Fatalf("removed %d, want 2: %v", n, err)
	}
	for id, left := range map[int64]bool{gone: false, dl: false, healed: true, waiting: true} {
		if got := askCount(t, r, "id = "+strconv.FormatInt(id, 10)) == 1; got != left {
			t.Fatalf("ask %d left=%v, want %v", id, got, left)
		}
	}
	// A failed-download id for an ask that is no longer approved stays.
	if n, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFailed, waiting); err != nil || n != 0 {
		t.Fatalf("waiting by download: %d %v", n, err)
	}
	if _, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFinished, waiting); err == nil {
		t.Fatal("failed downloads accepted with the finished filter")
	}
}

// Clear finished with the page's ids (a kind or "Asked by" filter on) takes
// only those, and only while they are still finished.
func TestRemoveFinishedIDsKeepsOthers(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	var ids []int64
	for _, tmdb := range []string{"693134", "900002", "900003"} {
		res, _ := r.svc.Ask(ctx, r.ana, tmdb)
		ids = append(ids, res.Ask.ID)
		setState(t, r, res.Ask.ID, "turned_down")
	}
	setState(t, r, ids[1], "asked") // answered again since the page read it
	n, err := r.svc.RemoveMany(ctx, r.luis, []int64{ids[0], ids[1]}, RemoveFinished)
	if err != nil || n != 1 || askCount(t, r, "1=1") != 2 {
		t.Fatalf("removed %d (left %d): %v", n, askCount(t, r, "1=1"), err)
	}
}

// The weekly limit counts a person's own asks that were not turned down.
// Removing a waiting ask frees its place, as taking it back or turning it
// down does.
func TestRemoveWaitingFreesTheWeeklyPlaceLikeDecline(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	used := func() int {
		t.Helper()
		l, err := r.svc.LimitOf(ctx, r.ana)
		if err != nil {
			t.Fatal(err)
		}
		return l.Movies.Used
	}
	a, _ := r.svc.Ask(ctx, r.ana, "693134")
	b, _ := r.svc.Ask(ctx, r.ana, "900002")
	if used() != 2 {
		t.Fatalf("used %d, want 2", used())
	}
	if _, err := r.svc.TurnDown(ctx, a.Ask.ID, r.luis, ""); err != nil {
		t.Fatal(err)
	}
	if used() != 1 {
		t.Fatalf("after turning down: used %d, want 1", used())
	}
	if n, err := r.svc.RemoveMany(ctx, r.luis, []int64{b.Ask.ID}, ""); err != nil || n != 1 {
		t.Fatalf("remove %d %v", n, err)
	}
	if used() != 0 {
		t.Fatalf("after removing the waiting ask: used %d, want 0", used())
	}
}

// Clear failed with no type or person filter on sends the filter and the
// failed-download ids but no card ids: it takes every Failed ask plus those
// downloads, however many cards the page had loaded.
func TestRemoveFailedFilterWithDownloadsTakesAll(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	var ids []int64
	for _, tmdb := range []string{"693134", "900002", "900003", "900005"} {
		res, err := r.svc.Ask(ctx, r.ana, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, res.Ask.ID)
	}
	setState(t, r, ids[0], "approved") // no title: failed
	setState(t, r, ids[1], "approved") // no title: failed
	if _, err := r.svc.Approve(ctx, ids[2], r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	n, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFailed, ids[2])
	if err != nil || n != 3 || askCount(t, r, "1=1") != 1 {
		t.Fatalf("removed %d: %v", n, err)
	}
}

func decidedAgo(t *testing.T, r *rig, id int64, ago time.Duration) {
	t.Helper()
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE asks SET decided_at = ? WHERE id = ?", database.FormatTime(r.clock.Add(-ago)), id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// An approved ask has no title until the add finishes (up to approveAddLimit),
// so Clear failed leaves one approved 10 seconds ago and takes one approved
// 10 minutes ago, and the page is told which is still being added.
func TestFailedWaitsForTheTitleToBeAdded(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	var ids []int64
	for _, tmdb := range []string{"693134", "900002"} {
		res, err := r.svc.Ask(ctx, r.ana, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		setState(t, r, res.Ask.ID, "approved")
		ids = append(ids, res.Ask.ID)
	}
	decidedAgo(t, r, ids[0], 10*time.Second)
	decidedAgo(t, r, ids[1], 10*time.Minute)
	vs, _ := r.svc.Views(ctx, "1 = 1")
	for _, v := range vs {
		if v.Adding != (v.ID == ids[0]) || v.Failed() != (v.ID == ids[1]) {
			t.Fatalf("ask %d adding=%v failed=%v", v.ID, v.Adding, v.Failed())
		}
	}
	if n, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFailed); err != nil || n != 1 || askCount(t, r, "id = "+strconv.FormatInt(ids[0], 10)) != 1 {
		t.Fatalf("removed %d: %v", n, err)
	}
	// Once the limit has passed it is failed like the others.
	r.clock = r.clock.Add(approveAddLimit)
	if n, err := r.svc.RemoveMany(ctx, r.luis, nil, RemoveFailed); err != nil || n != 1 {
		t.Fatalf("later removed %d: %v", n, err)
	}
}

func askRoutes(r *rig) *http.ServeMux {
	m := http.NewServeMux()
	(&AskHandlers{Svc: r.svc, Person: func(*http.Request) Person { return r.luis }}).Routes(m)
	return m
}

// The list says the true totals of every tab and the failed ones however many
// it lists, and reading it writes nothing.
func TestAskListCarriesTheTotals(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	var ids []int64
	for _, tmdb := range []string{"693134", "900002", "900003", "900005"} {
		res, err := r.svc.Ask(ctx, r.ana, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, res.Ask.ID)
	}
	setState(t, r, ids[0], "approved")
	setState(t, r, ids[1], "approved")
	setState(t, r, ids[2], "available")
	decidedAgo(t, r, ids[0], time.Hour)
	decidedAgo(t, r, ids[1], time.Hour)
	rec := httptest.NewRecorder()
	askRoutes(r).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/asks?limit=1", nil))
	var got struct {
		Asks   []View         `json:"asks"`
		Counts map[string]int `json:"counts"`
		Total  int            `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"pending": 1, "processing": 2, "done": 1, "failed": 2}
	if len(got.Asks) != 1 || got.Total != 4 || len(got.Counts) != 4 {
		t.Fatalf("list %d total %d counts %v", len(got.Asks), got.Total, got.Counts)
	}
	for k, v := range want {
		if got.Counts[k] != v {
			t.Fatalf("counts %v, want %v", got.Counts, want)
		}
	}
	if askCount(t, r, "1=1") != 4 {
		t.Fatal("reading the list removed something")
	}
}

// An older page (or a newer one) that sends a dry_run flag does not get a
// count: the remove route knows no such flag, so the body is a plain
// remove. This keeps a count from ever riding on the remove route again.
func TestRemoveRouteHasNoDryRun(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	setState(t, r, res.Ask.ID, "available")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/asks/remove", strings.NewReader(`{"filter":"finished","dry_run":true}`))
	req.Header.Set("Content-Type", "application/json")
	askRoutes(r).ServeHTTP(rec, req)
	if rec.Code == http.StatusOK && askCount(t, r, "1=1") != 0 {
		t.Fatal("200 but nothing removed: dry_run acted as a count")
	}
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), `"removed":1`) {
		t.Fatalf("answer %s", rec.Body.String())
	}
}

func postRemove(r *rig, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/asks/remove", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	askRoutes(r).ServeHTTP(rec, req)
	return rec
}

// With a type or person filter on the page sends only the shown ids and
// "only" (the kind of clear), with no filter: this server takes just those of
// them still failed; a 1.1.4 server ignores "only" and takes exactly those ids,
// where the old {"filter","ids"} body made it take every failed ask.
func TestRemoveOnlyNarrowsTheIDsSent(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	var ids []int64
	for _, tmdb := range []string{"693134", "900002", "900003", "900005"} {
		res, err := r.svc.Ask(ctx, r.ana, tmdb)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, res.Ask.ID)
	}
	setState(t, r, ids[0], "approved") // failed, shown
	setState(t, r, ids[1], "approved") // failed, not shown
	setState(t, r, ids[2], "available") // was shown as failed, finished meanwhile
	decidedAgo(t, r, ids[0], time.Hour)
	decidedAgo(t, r, ids[1], time.Hour)
	body := `{"ids":[` + strconv.FormatInt(ids[0], 10) + `,` + strconv.FormatInt(ids[2], 10) + `],"only":"failed"}`
	if rec := postRemove(r, body); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"removed":1`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if askCount(t, r, "1=1") != 3 || askCount(t, r, "id = "+strconv.FormatInt(ids[1], 10)) != 1 || askCount(t, r, "id = "+strconv.FormatInt(ids[2], 10)) != 1 {
		t.Fatal("removed more than the shown failed one")
	}
	for _, bad := range []string{`{"only":"failed"}`, `{"filter":"failed","only":"failed","ids":[1]}`, `{"ids":[1],"only":"nope"}`} {
		if rec := postRemove(r, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", bad, rec.Code)
		}
	}
	if askCount(t, r, "1=1") != 3 {
		t.Fatal("a refused body removed something")
	}
}

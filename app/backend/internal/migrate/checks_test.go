package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// checkRoutes serves a lab's migrate routes as the owner.
func checkRoutes(t *testing.T, l *moveLab) (*Handlers, func(method, path, body string) *httptest.ResponseRecorder) {
	t.Helper()
	r := webserver.NewRouter()
	h := &Handlers{Store: l.st, Mover: func() *Mover { return l.mv }, SafeMode: func() bool { return false }}
	h.Routes(r)
	owner := webserver.ContextWith(httptest.NewRequest("GET", "/", nil).Context(), webserver.Caller{UserID: 1, Name: "owner", Role: webserver.RoleOwner, Via: "session"})
	return h, func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(owner)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
}

func labBody(l *moveLab) string {
	b, _ := json.Marshal(MoveInput{Snapshots: l.snaps, Choices: Choices{PathRules: l.rules, People: l.people}})
	return string(b)
}

// followCheck polls a check over HTTP until it is no longer running.
func followCheck(t *testing.T, do func(method, path, body string) *httptest.ResponseRecorder, id string, max time.Duration) (map[string]any, int) {
	t.Helper()
	end := time.Now().Add(max)
	for time.Now().Before(end) {
		start := time.Now()
		rec := do("GET", "/api/v1/migrate/checks/"+id, "")
		if took := time.Since(start); took > time.Second {
			t.Fatalf("GET of a check took %v", took)
		}
		var v map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		if rec.Code != http.StatusOK || v["state"] != "running" {
			return v, rec.Code
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("check %s still running after %v", id, max)
	return nil, 0
}

func startCheckHTTP(t *testing.T, do func(method, path, body string) *httptest.ResponseRecorder, what, body string) string {
	t.Helper()
	start := time.Now()
	rec := do("POST", "/api/v1/migrate/"+what, body)
	if took := time.Since(start); rec.Code != http.StatusAccepted || took > time.Second {
		t.Fatalf("POST %s: %d after %v: %s", what, rec.Code, took, rec.Body.String())
	}
	var v struct {
		ID string `json:"check_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if v.ID == "" {
		t.Fatalf("no check id: %s", rec.Body.String())
	}
	return v.ID
}

// A metadata server that never answers cannot hold the test run's
// request. It answers at once; each lookup is cut off after dryCallLimit,
// and that show is still being checked, never "not found".
func TestCheckHungMetadata(t *testing.T) {
	old := dryCallLimit
	dryCallLimit = 200 * time.Millisecond
	t.Cleanup(func() { dryCallLimit = old })
	l, _ := cunkSetup(t, cunkSonarr, []metadata.Show{cunkOn()}, cunkFiles...)
	l.mv.Library.Shows = metadata.ShowSources{"tmdb": &stuckShows{manyShows: manyShows{list: []metadata.Show{cunkOn()}}}, "tvmaze": &stuckSearch{}, "tvdb": &stuckSearch{}}
	_, do := checkRoutes(t, l)
	id := startCheckHTTP(t, do, "dry-run", labBody(l))
	v, code := followCheck(t, do, id, 10*time.Second)
	if code != http.StatusOK || v["state"] != "done" {
		t.Fatalf("check ended %d %v", code, v)
	}
	b, _ := json.Marshal(v["report"])
	var rep Report
	_ = json.Unmarshal(b, &rep)
	if rep.Checking != 1 || lineOf(t, &rep, cunkKey).Result == ResFailed || v["plan_hash"] == "" {
		t.Fatalf("hung show: %d still being checked, line %+v", rep.Checking, lineOf(t, &rep, cunkKey))
	}
}

// A slow disk (or any slow step) never holds the request either: the page
// sees the step it is on, and the same input joins the running check.
func TestCheckSlowDisk(t *testing.T) {
	l := newMoveLab(t)
	h, do := checkRoutes(t, l)
	release := make(chan struct{})
	in := MoveInput{Snapshots: l.snaps}
	by := domain.Actor{Kind: "user", ID: 1}
	slow := func(ctx context.Context, c *bgCheck) (map[string]any, error) {
		c.setStep("folders")
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return map[string]any{"plan_hash": "x"}, nil
	}
	c := h.checks.start("plan", in, by, slow)
	if again := h.checks.start("plan", in, by, slow); again != c {
		t.Fatal("the same input started a second check")
	}
	time.Sleep(50 * time.Millisecond)
	rec := do("GET", "/api/v1/migrate/checks/"+c.id, "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"step":"folders"`) || !strings.Contains(rec.Body.String(), `"state":"running"`) {
		t.Fatalf("slow check: %d %s", rec.Code, rec.Body.String())
	}
	// Changed choices stop the older check of the same kind.
	other := h.checks.start("plan", MoveInput{Snapshots: append([]int64{}, l.snaps...), Choices: Choices{Skip: []string{"alerts"}}}, by, slow)
	if v, _ := followCheck(t, do, c.id, 5*time.Second); v["error"] == nil {
		t.Fatalf("older check not stopped: %v", v)
	}
	close(release)
	if v, code := followCheck(t, do, other.id, 5*time.Second); code != 200 || v["state"] != "done" {
		t.Fatalf("newer check: %d %v", code, v)
	}
}

// A restart forgets checks: GET answers 410 so the page starts again, and
// an unknown snapshot answers 404 as before (the page then reads again).
func TestCheckAfterRestart(t *testing.T) {
	l := newMoveLab(t)
	_, do := checkRoutes(t, l)
	if rec := do("GET", "/api/v1/migrate/checks/abc", ""); rec.Code != http.StatusGone {
		t.Fatalf("unknown check: %d", rec.Code)
	}
	id := startCheckHTTP(t, do, "plan", `{"snapshots":[999]}`)
	if _, code := followCheck(t, do, id, 5*time.Second); code != http.StatusNotFound {
		t.Fatalf("missing snapshot answered %d", code)
	}
	if rec := do("POST", "/api/v1/migrate/plan", `{"snapshots":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no snapshot picked: %d", rec.Code)
	}
}

// 10,000 titles: the request answers at once, progress counts up to the
// total, and memory stays modest.
func TestCheckTenThousandTitles(t *testing.T) {
	const n = 10000
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		base := radarr.Titles[0]
		for i, extra := 0, n-len(radarr.Titles); i < extra; i++ {
			tt := base
			tt.SourceID = int64(1000 + i)
			tt.Name = fmt.Sprintf("Film %d", i)
			tt.IDs = map[string]string{"tmdb": fmt.Sprint(900000 + i)}
			tt.Path = base.Path + fmt.Sprintf(" copy %d", i)
			tt.Files = nil
			radarr.Titles = append(radarr.Titles, tt)
		}
	})
	_, do := checkRoutes(t, l)
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	id := startCheckHTTP(t, do, "dry-run", labBody(l))
	sawProgress := false
	end := time.Now().Add(20 * time.Minute)
	var v map[string]any
	for time.Now().Before(end) {
		rec := do("GET", "/api/v1/migrate/checks/"+id, "")
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		if rec.Code != 200 {
			t.Fatalf("check failed: %d %s", rec.Code, rec.Body.String())
		}
		if d, _ := v["done"].(float64); v["state"] == "running" && d > 0 && d < n {
			sawProgress = true
		}
		if v["state"] != "running" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if v["state"] != "done" || v["total"].(float64) < n-50 || v["done"] != v["total"] {
		t.Fatalf("10k check: %v %v of %v", v["state"], v["done"], v["total"])
	}
	if !sawProgress {
		t.Log("the check finished before a poll saw it midway")
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if grew := int64(after.HeapAlloc) - int64(before.HeapAlloc); grew > 256<<20 {
		t.Fatalf("heap grew %d MB for 10k titles", grew>>20)
	}
}

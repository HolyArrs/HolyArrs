package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"
)

// moveSetup serves the API with a running move Job and one kept Radarr
// snapshot whose movie folder is on this machine under another path.
func moveSetup(t *testing.T, safe bool) (*httptest.Server, *database.Store, int64, string) {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	films := filepath.Join(t.TempDir(), "films")
	if err := os.MkdirAll(filepath.Join(films, "Dune Part Two (2024)"), 0o755); err != nil {
		t.Fatal(err)
	}
	q := jobs.New(st, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	lib := &library.Service{Store: st, Queue: q, Metadata: metadata.Lab()}
	mv := &migrate.Mover{Store: st, Library: lib, Queue: q}
	mv.Register(q)
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(buildHandlers(services{Store: st, Queue: q, Library: lib, Metadata: metadata.Lab(), Version: "test", Mover: mv, SafeMode: safe}).Handler())
	t.Cleanup(srv.Close)
	snap := &migrate.Snapshot{Kind: migrate.Radarr, Via: migrate.ViaBackup, Version: "6.4.4",
		Folders: []migrate.Folder{{Path: "/movies"}},
		Titles: []migrate.Title{{SourceID: 1, Kind: "movie", Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134"},
			Path: "/movies/Dune Part Two (2024)", Monitored: true}},
		Tags:         []migrate.Tag{{ID: 1, Label: "kids"}},
		Indexers:     []migrate.Provider{{Name: "Nyaa", Kind: "Torznab", Enabled: true, Settings: map[string]any{"baseUrl": "http://prowlarr:9696/1/"}, NeedsReentry: []string{"apiKey"}}},
		NeedsReentry: []string{`indexer "Nyaa": apiKey`},
	}
	saved, err := migrate.Save(context.Background(), st, 1, snap)
	if err != nil {
		t.Fatal(err)
	}
	return srv, st, saved.ID, filepath.ToSlash(films)
}

func moveBody(snap int64, to string, skip ...string) string {
	return moveBodyHash(snap, to, "", skip...)
}

// moveBodyHash is a move body bound to a dry run's plan_hash.
func moveBodyHash(snap int64, to, hash string, skip ...string) string {
	b, _ := json.Marshal(map[string]any{"snapshots": []int64{snap}, "path_rules": []map[string]string{{"from": "/movies", "to": to}}, "skip": skip, "plan_hash": hash})
	return string(b)
}

// route: POST /api/v1/migrate/plan
// route: POST /api/v1/migrate/dry-run
// route: GET /api/v1/migrate/moves
// route: POST /api/v1/migrate/moves
// route: GET /api/v1/migrate/moves/{id}
// route: GET /api/v1/migrate/moves/{id}/report
// route: POST /api/v1/migrate/moves/{id}/undo
func TestMoveRoutes(t *testing.T) {
	t.Parallel()
	srv, st, snap, films := moveSetup(t, false)
	base := srv.URL + "/api/v1/migrate/"

	// The plan: groups with counts, not-moved reasons, and a suggestion
	// when no rule is given and the old path is not here.
	b := lbDo(t, "POST", base+"plan", moveBody(snap, films), 200)
	var plan struct {
		Plan   migrate.Plan   `json:"plan"`
		Counts map[string]int `json:"counts"`
	}
	lbDecodeJSON(t, b, &plan)
	if plan.Counts["titles"] != 1 || plan.Counts["sources"] != 1 || plan.Counts["not_moved"] == 0 || !strings.Contains(string(b), "HolyArrs has no tags") {
		t.Fatalf("plan %s", b)
	}
	lbDo(t, "POST", base+"plan", `{"snapshots":[`+lbItoa(snap)+`],"skip":["nothing"]}`, 400)
	lbDo(t, "POST", base+"plan", `{"snapshots":[`+lbItoa(snap)+`],"path_rules":[{"from":"/movies/../etc","to":"/x"}]}`, 400)
	lbDo(t, "POST", base+"plan", `{"snapshots":[]}`, 400)
	lbDo(t, "POST", base+"plan", `{"snapshots":[999]}`, 404)
	lbDo(t, "POST", base+"plan", `not json`, 400)

	// The dry run says what would happen and changes nothing.
	b = lbDo(t, "POST", base+"dry-run", moveBody(snap, films), 200)
	if !strings.Contains(string(b), `"result":"adopt"`) || !strings.Contains(string(b), `"dry":true`) {
		t.Fatalf("dry run %s", b)
	}
	var dry struct {
		PlanHash string `json:"plan_hash"`
	}
	lbDecodeJSON(t, b, &dry)
	if len(dry.PlanHash) != 64 {
		t.Fatalf("dry run without a plan hash: %s", b)
	}
	var n int
	_ = st.Read().QueryRow("SELECT count(*) FROM titles").Scan(&n)
	if n != 0 {
		t.Fatal("the dry run added a title")
	}

	// Approve: bound to the dry run (no hash, or another plan's: 409), then
	// a supervised Job with a report.
	lbDo(t, "POST", base+"moves", moveBody(snap, films), 409)
	if b = lbDo(t, "POST", base+"moves", moveBodyHash(snap, films, dry.PlanHash, "titles"), 409); !strings.Contains(string(b), "dry run again") {
		t.Fatalf("changed plan %s", b)
	}
	b = lbDo(t, "POST", base+"moves", moveBodyHash(snap, films, dry.PlanHash), 202)
	var started struct {
		Move  migrate.Move `json:"move"`
		JobID int64        `json:"job_id"`
	}
	lbDecodeJSON(t, b, &started)
	if started.JobID == 0 || started.Move.State != "queued" {
		t.Fatalf("start %s", b)
	}
	id := lbItoa(started.Move.ID)
	var got struct {
		Move migrate.Move `json:"move"`
	}
	for i := 0; i < 500 && got.Move.State != "done"; i++ {
		lbDecodeJSON(t, lbDo(t, "GET", base+"moves/"+id, "", 200), &got)
		time.Sleep(10 * time.Millisecond)
	}
	if got.Move.State != "done" || got.Move.Report.Counts[migrate.ResAdopted] != 1 {
		t.Fatalf("move %+v", got.Move)
	}
	if b = lbDo(t, "GET", base+"moves", "", 200); !strings.Contains(string(b), `"state":"done"`) {
		t.Fatalf("list %s", b)
	}
	resp, err := http.Get(base + "moves/" + id + "/report")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/markdown") ||
		!strings.Contains(resp.Header.Get("Content-Disposition"), "holyarrs-move-"+id+".md") {
		t.Fatalf("report %d %v", resp.StatusCode, resp.Header)
	}
	lbDo(t, "GET", base+"moves/999", "", 404)
	lbDo(t, "GET", base+"moves/x/report", "", 404)

	// Undo: a supervised Job of its own, once.
	if b = lbDo(t, "POST", base+"moves/"+id+"/undo", "", 202); !strings.Contains(string(b), `"undo_job_id"`) {
		t.Fatalf("undo %s", b)
	}
	for i := 0; i < 500 && got.Move.State != "undone"; i++ {
		lbDecodeJSON(t, lbDo(t, "GET", base+"moves/"+id, "", 200), &got)
		time.Sleep(10 * time.Millisecond)
	}
	if got.Move.State != "undone" || got.Move.Report.Counts[migrate.ResRemoved] == 0 {
		t.Fatalf("undo %+v", got.Move)
	}
	lbDo(t, "POST", base+"moves/"+id+"/undo", "", 409)
	lbDo(t, "POST", base+"moves/999/undo", "", 404)
}

// TestMoveSafeModeRefuses: safe mode keeps the Library still.
func TestMoveSafeModeRefuses(t *testing.T) {
	t.Parallel()
	srv, _, snap, films := moveSetup(t, true)
	base := srv.URL + "/api/v1/migrate/"
	lbDo(t, "POST", base+"dry-run", moveBody(snap, films), 200)
	lbDo(t, "POST", base+"moves", moveBody(snap, films), 409)
}

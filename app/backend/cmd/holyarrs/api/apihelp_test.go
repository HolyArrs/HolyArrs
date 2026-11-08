package api

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func setup(t *testing.T) (*httptest.Server, *database.Store, *jobs.Queue) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	hub := webserver.NewLiveHub(webserver.LiveOptions{Heartbeat: time.Second})
	srv := httptest.NewServer(New(Server{Store: s, Queue: q, Live: hub, Metadata: metadata.Lab(), Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, s, q
}

func do(t *testing.T, method, url, body string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("%s %s: %d, want %d: %s", method, url, resp.StatusCode, want, b)
	}
	return b
}

// routePackages are the packages that register /api/v1 routes, seen from
// this folder (cmd/holyarrs keeps the same list for the spec tests).
var routePackages = []string{".", "../../../internal/accounts", "../../../internal/backups", "../../../internal/alerts", "../../../internal/health", "../../../internal/updates", "../../../internal/mediaservers", "../../../internal/metadata", "../../../internal/sources", "../../../internal/subtitles", "../../../internal/ai", "../../../internal/arrapi", "../../../internal/migrate", "../../../internal/movies", "../../../internal/shows", "../../../internal/library", "../../../internal/mediafiles", "../../../internal/datapack", "../../../internal/importer", "../../../internal/setup", "../../../internal/downloads", "../../../internal/requests", "../../../internal/music", "../../../internal/discover", "../../../internal/core/webserver"}

// The jobs and imports lists answer JSON (their end-to-end test runs in
// cmd/holyarrs; this keeps the routes served here too).
//
// route: GET /api/v1/imports
// route: GET /api/v1/jobs
func TestJobsAndImportsServed(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	for _, p := range []string{"/api/v1/jobs", "/api/v1/imports"} {
		if b := do(t, "GET", srv.URL+p, "", 200); len(b) == 0 || b[0] != '{' && b[0] != '[' {
			t.Fatalf("%s: %s", p, b)
		}
	}
}

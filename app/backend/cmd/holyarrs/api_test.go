package main

import (
	"strconv"

	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
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
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Queue: q, Live: hub, Metadata: metadata.Lab(), Version: "test"}), config{}))
	t.Cleanup(srv.Close)
	return srv, s, q
}

// route: GET /api/v1/titles
// route: POST /api/v1/titles/{id}/search
// route: GET /api/v1/titles/{id}/releases
func TestAddListSearch(t *testing.T) {
	t.Parallel()
	srv, _, q := setup(t)
	// Nothing yet.
	var list struct {
		Titles []TitleView `json:"titles"`
	}
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles", "", 200), &list)
	if len(list.Titles) != 0 {
		t.Fatalf("titles %+v", list)
	}
	b := do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	var resp struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(b, &resp)
	added := resp.Title
	if added.Name != "Dune: Part Two" || len(added.Slots) != 1 || added.Slots[0].State != "wanted" {
		t.Fatalf("added %s", b)
	}
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 409) // already there
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:1"}`, 404)
	do(t, "POST", srv.URL+"/api/v1/titles", `not json`, 400)
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles", "", 200), &list)
	if len(list.Titles) != 1 || list.Titles[0].Slots[0].StateLabel != "Missing" {
		t.Fatalf("titles %+v", list)
	}

	ran := make(chan int64, 1)
	q.Handle(jobs.KindSearchSlot, func(ctx context.Context, r *jobs.Run) error { ran <- r.Job.Subject.ID; return nil })
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer q.Stop()
	b = do(t, "POST", srv.URL+"/api/v1/titles/"+strconv.FormatInt(added.ID, 10)+"/search", "", 202)
	if !strings.Contains(string(b), "job_ids") {
		t.Fatalf("search answer %s", b)
	}
	select {
	case id := <-ran:
		if id != added.Slots[0].ID {
			t.Fatalf("searched slot %d", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("search job never ran")
	}
	do(t, "POST", srv.URL+"/api/v1/titles/999/search", "", 404)
	do(t, "GET", srv.URL+"/api/v1/titles/"+strconv.FormatInt(added.ID, 10)+"/releases", "", 200)
}

// route: GET /api/v1/health
// route: GET /api/v1/downloads
// route: GET /api/v1/imports
// route: GET /api/v1/jobs
func TestReadEndpoints(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	for _, p := range []string{"/api/v1/downloads", "/api/v1/events", "/api/v1/jobs", "/api/v1/imports"} {
		b := do(t, "GET", srv.URL+p, "", 200)
		if !json.Valid(b) {
			t.Fatalf("%s: not JSON: %s", p, b)
		}
	}
	var h map[string]any
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/health", "", 200), &h)
	if h["ok"] != true || h["write_wait_p95_ms"] == nil {
		t.Fatalf("health %+v", h)
	}
}

func TestWebApp(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	page := string(do(t, "GET", srv.URL+"/", "", 200))
	if !strings.Contains(page, "/_app/immutable/") {
		t.Fatalf("page does not load the app: %s", page)
	}
	// App links fall back to the app page; unknown files and API routes do not.
	if got := string(do(t, "GET", srv.URL+"/settings/backups", "", 200)); got != page {
		t.Fatal("app link did not get the app page")
	}
	do(t, "GET", srv.URL+"/nope.js", "", 404)
	do(t, "GET", srv.URL+"/api/v1/nope", "", 404)
	do(t, "GET", srv.URL+"/brand/favicon.svg", "", 200)
}

func TestLiveStreamOpens(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/live", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("live: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestSignInGuardsRoutes(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "test", Auth: accounts.New(s, accounts.Options{})}), config{}))
	t.Cleanup(srv.Close)
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noFollow.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("/: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	do(t, "GET", srv.URL+"/api/v1/health", "", 401)
	do(t, "GET", srv.URL+"/api/v1/titles", "", 401)
	do(t, "GET", srv.URL+"/live", "", 401)
	do(t, "GET", srv.URL+"/healthz", "", 200)
	do(t, "GET", srv.URL+"/readyz", "", 200)
	if b := do(t, "GET", srv.URL+"/login", "", 200); !strings.Contains(string(b), "/_app/immutable/") {
		t.Fatal("sign-in page missing")
	}
	do(t, "GET", srv.URL+"/brand/favicon.svg", "", 200)
}

func TestSourcesShowMaskedKey(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	ref := int64(42)
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "sources", &domain.Source{Name: "S", Protocol: "torrent", Format: "torznab", URL: "http://x/api",
			SecretRef: &ref, Kinds: []string{"movie"}, Categories: map[string][]int{}, State: "enabled", Extra: map[string]string{}}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	b := string(do(t, "GET", srv.URL+"/api/v1/sources", "", 200))
	if !strings.Contains(b, `"api_key":"••••••••"`) || strings.Contains(b, "secret_ref") {
		t.Fatalf("sources: %s", b)
	}
}

// route: GET /api/v1/metadata/search
func TestMetadataSearchAndAttribution(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t)
	b := string(do(t, "GET", srv.URL+"/api/v1/metadata/search?q=dune+part+two&year=2024", "", 200))
	if !strings.Contains(b, `"tmdb":"693134"`) {
		t.Fatalf("search: %s", b)
	}
	do(t, "GET", srv.URL+"/api/v1/metadata/search", "", 400)
	page := string(do(t, "GET", srv.URL+"/", "", 200))
	// Credits live on GitHub, not in the app.
	if strings.Contains(page, "This product uses the TMDB API") {
		t.Fatal("TMDB credit line should not be on the page")
	}
}

// route: PUT /api/v1/settings/tmdb-token
// route: GET /api/v1/settings/tmdb-token
func TestNoTMDBKeyRoutes(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	srv := httptest.NewServer(routes(buildHandlers(services{Store: s, Metadata: metadata.Lab(), Version: "test"}), config{}))
	t.Cleanup(srv.Close)
	for _, m := range []string{"GET", "PUT"} {
		req, _ := http.NewRequest(m, srv.URL+"/api/v1/settings/tmdb-token", strings.NewReader(`{"token":"TOKEN_FOR_TEST_ONLY"}`))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == 200 {
			t.Fatalf("%s tmdb-token still served", m)
		}
	}
}

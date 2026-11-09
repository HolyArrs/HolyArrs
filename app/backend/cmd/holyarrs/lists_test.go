package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// listSetup is an API with Outside lists pointed at a stand-in TMDB that
// serves list 7 (Dune and The Lantern Keepers), and a secrets vault.
func listSetup(t *testing.T) (*httptest.Server, *database.Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	key, _ := secrets.LoadKey(dir, "")
	v, err := secrets.Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	md := &metadata.Static{Movies: []metadata.Movie{
		{Name: "Dune: Part Two", Year: 2024, IDs: map[string]string{"tmdb": "693134"}, CinemaDate: day("2024-03-01"), DigitalDate: day("2024-04-16")},
		{Name: "The Lantern Keepers", Year: 2031, IDs: map[string]string{"tmdb": "900001"}, CinemaDate: day("2031-02-01")},
	}}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer TOKEN_FOR_TEST_ONLY" || r.URL.Path != "/list/7" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"total_pages":1,"items":[{"id":693134,"media_type":"movie","title":"Dune: Part Two"},{"id":900001,"media_type":"movie","title":"The Lantern Keepers"}]}`))
	}))
	t.Cleanup(fake.Close)
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond})
	lib := &library.Service{Store: s, Queue: q, Metadata: md}
	lib.Register(q)
	q.Handle(library.KindSearchSlot, func(context.Context, *jobs.Run) error { return nil })
	ls := &requests.OutsideLists{Store: s, Queue: q, Library: lib, Metadata: md, TMDBBase: fake.URL,
		TMDBToken:   func(context.Context) (string, error) { return "TOKEN_FOR_TEST_ONLY", nil },
		TraktClient: func(ctx context.Context) (string, error) { return v.Setting(ctx, "links", "trakt_client_id") }}
	ls.Register(q)
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Metadata: md, Version: "test", Library: lib, Lists: ls, Secrets: v}).Handler())
	t.Cleanup(srv.Close)
	return srv, s
}

// route: GET /api/v1/outside-lists
// route: POST /api/v1/outside-lists
// route: PUT /api/v1/outside-lists/{id}
// route: DELETE /api/v1/outside-lists/{id}
// route: POST /api/v1/outside-lists/{id}/sync
// route: GET /api/v1/settings/trakt-key
// route: PUT /api/v1/settings/trakt-key
func TestOutsideListsAPI(t *testing.T) {
	t.Parallel()
	srv, s := listSetup(t)
	ctx := context.Background()
	movies := filepath.Join(t.TempDir(), "movies")
	_ = os.MkdirAll(movies, 0o755)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie"}`, movies), 201)
	do(t, "POST", srv.URL+"/api/v1/never-add", `{"tmdb_id":"900001","name":"The Lantern Keepers"}`, 201)

	// Trakt key: stored, shown masked only.
	do(t, "PUT", srv.URL+"/api/v1/settings/trakt-key", `{}`, 400)
	do(t, "PUT", srv.URL+"/api/v1/settings/trakt-key", `{"key":"TRAKT_FOR_TEST_ONLY"}`, 200)
	b := string(do(t, "GET", srv.URL+"/api/v1/settings/trakt-key", "", 200))
	if strings.Contains(b, "TRAKT_FOR_TEST_ONLY") || !strings.Contains(b, secrets.Masked) {
		t.Fatalf("trakt key shown: %s", b)
	}

	do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"x","kind":"imdb","ref":"1"}`, 400)
	do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"x","kind":"trakt_list","ref":"no slash"}`, 400)
	// Saving a list reads it once straight away.
	var l requests.ListView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"Lab list","kind":"tmdb_list","ref":"https://www.themoviedb.org/list/7-lab"}`, 201), &l)
	if l.ID == 0 || l.Ref != "7" || l.SyncEveryHours != 12 || l.JobID == 0 {
		t.Fatalf("list %+v", l)
	}
	do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"LAB LIST","kind":"tmdb_list","ref":"8"}`, 409)
	deadline := time.Now().Add(5 * time.Second)
	for {
		ts, _ := database.Select[mediafiles.Title](ctx, s.Read(), "titles", "1=1")
		if len(ts) == 1 {
			if ts[0].WhyAdded != "list" || ts[0].Name != "Dune: Part Two" {
				t.Fatalf("title %+v", ts[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("list never added Dune; titles %+v", ts)
		}
		time.Sleep(10 * time.Millisecond)
	}
	var all struct {
		Lists []requests.ListView `json:"outside_lists"`
	}
	for time.Now().Before(deadline) {
		_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/outside-lists", "", 200), &all)
		if len(all.Lists) == 1 && all.Lists[0].LastSyncedAt != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(all.Lists) != 1 || !strings.Contains(all.Lists[0].LastResult, "1 on the Never-add list") {
		t.Fatalf("lists %+v", all)
	}

	b = string(do(t, "PUT", srv.URL+"/api/v1/outside-lists/"+itoa(l.ID), `{"name":"Lab list","kind":"tmdb_list","ref":"7","enabled":false,"sync_every_hours":24}`, 200))
	if !strings.Contains(b, `"enabled":false`) || !strings.Contains(b, `"sync_every_hours":24`) {
		t.Fatalf("edit %s", b)
	}
	do(t, "PUT", srv.URL+"/api/v1/outside-lists/999", `{"name":"y","kind":"tmdb_list","ref":"7"}`, 404)
	b = string(do(t, "POST", srv.URL+"/api/v1/outside-lists/"+itoa(l.ID)+"/sync", "", 202))
	if !strings.Contains(b, `"job_id"`) {
		t.Fatalf("sync %s", b)
	}
	do(t, "POST", srv.URL+"/api/v1/outside-lists/999/sync", "", 404)
	do(t, "DELETE", srv.URL+"/api/v1/outside-lists/"+itoa(l.ID), "", 204)
	do(t, "DELETE", srv.URL+"/api/v1/outside-lists/"+itoa(l.ID), "", 404)

	// Show lists: they need a show folder, and keep their season choice.
	off := `,"enabled":false`
	do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"Shows","kind":"tmdb_list","ref":"7","title_kind":"show"`+off+`}`, 409)
	do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"Shows","kind":"tmdb_list","ref":"7","title_kind":"album"`+off+`}`, 400)
	shows := filepath.Join(t.TempDir(), "shows")
	_ = os.MkdirAll(shows, 0o755)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"show"}`, shows), 201)
	do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"Shows","kind":"tmdb_list","ref":"7","title_kind":"show","seasons":"pilot"`+off+`}`, 400)
	var sl requests.ListView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/outside-lists", `{"name":"Shows","kind":"tmdb_list","ref":"7","title_kind":"show","seasons":"first","new_seasons":"none"`+off+`}`, 201), &sl)
	if sl.TitleKind != "show" || sl.Seasons != "first" || sl.NewSeasons != "none" {
		t.Fatalf("show list %+v", sl)
	}
	// Pick rules made only for movies do not fit a show list.
	var pr struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", `{"name":"Movies only","kinds":["movie"],"body":`+goodEverydayBody(t)+`}`, 201), &pr)
	do(t, "POST", srv.URL+"/api/v1/outside-lists", fmt.Sprintf(`{"name":"Shows 2","kind":"tmdb_list","ref":"7","title_kind":"show","pick_rules_id":%d%s}`, pr.ID, off), 400)
	// A show goes on the Never-add list by its TMDB number.
	b = string(do(t, "POST", srv.URL+"/api/v1/never-add", `{"kind":"show","tmdb_id":"700003","name":"Not Here"}`, 201))
	if !strings.Contains(b, `"kind":"show"`) {
		t.Fatalf("never-add show %s", b)
	}
	do(t, "POST", srv.URL+"/api/v1/never-add", `{"kind":"artist","tmdb_id":"1","name":"x"}`, 400)
}

// goodEverydayBody is the Standard rules body as JSON, for new rules.
func goodEverydayBody(t *testing.T) string {
	t.Helper()
	b, err := json.Marshal(mediafiles.GoodEveryday())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func detailedMovie() metadata.Movie {
	return metadata.Movie{Name: "Five", Year: 2024, Runtime: 121, IDs: map[string]string{"tmdb": "5"},
		Backdrop: "https://img.test/w1280/b.jpg", Logo: "https://img.test/w500/l.png",
		Genres: []string{"Drama", "Adventure"}, Rating: "PG-13", People: []string{"A1", "D1"}}
}

func detailsRig(t *testing.T, p metadata.Provider) (*httptest.Server, *database.Store, *library.Service) {
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
	q := jobs.New(s, jobs.Options{Workers: 1})
	lib := &library.Service{Store: s, Queue: q, Metadata: p}
	srv := httptest.NewServer(buildHandlers(services{Store: s, Queue: q, Metadata: p, Library: lib, Version: "test"}).Handler())
	t.Cleanup(srv.Close)
	return srv, s, lib
}

func TestTitleViewCarriesDetailsOnAdd(t *testing.T) {
	t.Parallel()
	srv, _, _ := detailsRig(t, &metadata.Static{Movies: []metadata.Movie{detailedMovie()}})
	var resp struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:5"}`, 201), &resp)
	b := string(do(t, "GET", srv.URL+"/api/v1/titles/"+itoa(resp.Title.ID), "", 200))
	for _, want := range []string{`"backdrop":"https://img.test/w1280/b.jpg"`, `"logo":"https://img.test/w500/l.png"`,
		`"genres":["Drama","Adventure"]`, `"runtime_minutes":121`, `"rating":"PG-13"`, `"people":["A1","D1"]`,
		`"artwork":[]`, `"name":"Five"`} {
		if !strings.Contains(b, want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
	list := string(do(t, "GET", srv.URL+"/api/v1/titles", "", 200))
	if !strings.Contains(list, `"genres":["Drama","Adventure"]`) {
		t.Errorf("list %s", list)
	}
}

func TestTitleDetailsBackfilledOnRefresh(t *testing.T) {
	t.Parallel()
	bare := detailedMovie()
	bare.Backdrop, bare.Logo, bare.Genres, bare.Rating, bare.People, bare.Runtime = "", "", nil, "", nil, 0
	p := &metadata.Static{Movies: []metadata.Movie{bare}}
	srv, _, lib := detailsRig(t, p)
	var resp struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:5"}`, 201), &resp)
	b := string(do(t, "GET", srv.URL+"/api/v1/titles/"+itoa(resp.Title.ID), "", 200))
	if !strings.Contains(b, `"genres":[]`) || !strings.Contains(b, `"people":[]`) || strings.Contains(b, `"backdrop"`) || strings.Contains(b, `"rating"`) {
		t.Fatalf("bare title %s", b)
	}
	p.Movies[0] = detailedMovie()
	if _, err := lib.RefreshTitle(context.Background(), resp.Title.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	var got TitleView
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles/"+itoa(resp.Title.ID), "", 200), &got)
	if got.Backdrop != "https://img.test/w1280/b.jpg" || got.Logo == "" || got.Rating != "PG-13" || got.RuntimeMinutes != 121 ||
		!reflect.DeepEqual(got.Genres, []string{"Drama", "Adventure"}) || !reflect.DeepEqual(got.People, []string{"A1", "D1"}) {
		t.Fatalf("refreshed %+v", got)
	}
}

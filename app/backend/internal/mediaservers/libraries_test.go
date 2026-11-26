package mediaservers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// route: GET /api/v1/media-servers/{id}/libraries
// route: PUT /api/v1/media-servers/{id}/libraries
func TestLibrariesPickAndScan(t *testing.T) {
	w := newWorld(t)
	srv, calls := fakeServer(t, "plex", "/movies")
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "Plex", Kind: "plex", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{Store: w.s, Vault: w.v, Linker: &Linker{Store: w.s, Vault: w.v}}
	mux := http.NewServeMux()
	h.Routes(mux)
	do := func(method, body string) (int, string) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/media-servers/"+strconv.FormatInt(m.ID, 10)+"/libraries", strings.NewReader(body)))
		return rec.Code, rec.Body.String()
	}
	code, out := do("GET", "")
	if code != 200 || !strings.Contains(out, `"title":"Movies"`) || !strings.Contains(out, `"paths":["/movies"]`) || strings.Contains(out, `"on":true`) {
		t.Fatalf("list: %d %s", code, out)
	}
	code, out = do("PUT", `{"keys":["2"," 2",""],"scan":true}`)
	if code != 200 || !strings.Contains(out, `"keys":["2"]`) {
		t.Fatalf("pick: %d %s", code, out)
	}
	if _, out = do("GET", ""); !strings.Contains(out, `"key":"2","title":"TV","kind":"show","paths":["/tv"],"on":true`) || !strings.Contains(out, `"key":"1","title":"Movies","kind":"movie","paths":["/movies"],"on":false`) {
		t.Fatalf("after pick: %s", out)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		found := false
		for _, c := range calls() {
			if c.path == "/library/sections/2/refresh" {
				found = true
			}
			if c.path == "/library/sections/1/refresh" {
				t.Fatal("scanned a library that is off")
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no scan")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code, _ := do("PUT", `nope`); code != 400 {
		t.Fatalf("bad body: %d", code)
	}
}

func TestJellyfinLibraries(t *testing.T) {
	w := newWorld(t)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Library/VirtualFolders":
			_, _ = rw.Write([]byte(`[{"Name":"Films","CollectionType":"movies","Locations":["/data/films"],"ItemId":"a1"},{"Name":"Books","CollectionType":"books","ItemId":"b"},{"Name":"Shows","CollectionType":"tvshows","ItemId":"c2"}]`))
		case r.URL.Path == "/Items/c2/Refresh" && r.Method == http.MethodPost:
			rw.WriteHeader(http.StatusNoContent)
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "J", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	libs, err := l.Libraries(context.Background(), m)
	if err != nil || len(libs) != 2 || libs[0].Kind != "movie" || libs[1].Key != "c2" || libs[0].Paths[0] != "/data/films" {
		t.Fatalf("libs %+v %v", libs, err)
	}
	if err := l.Scan(context.Background(), m, []string{"c2"}); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryCounts(t *testing.T) {
	w := newWorld(t)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/Library/VirtualFolders":
			_, _ = rw.Write([]byte(`[{"Name":"Films","CollectionType":"movies","ItemId":"a1"},{"Name":"Shows","CollectionType":"tvshows","ItemId":"c2"},{"Name":"Tunes","CollectionType":"music","ItemId":"m3"}]`))
		case r.URL.Path == "/Items" && r.URL.Query().Get("ParentId") == "a1" && r.URL.Query().Get("IncludeItemTypes") == "Movie":
			_, _ = rw.Write([]byte(`{"Items":[],"TotalRecordCount":1284}`))
		case r.URL.Path == "/Items" && r.URL.Query().Get("ParentId") == "m3" && r.URL.Query().Get("IncludeItemTypes") == "MusicArtist":
			_, _ = rw.Write([]byte(`{"Items":[],"TotalRecordCount":96}`))
		default:
			rw.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "J", Kind: "jellyfin", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{Store: w.s, Vault: w.v, Linker: &Linker{Store: w.s, Vault: w.v}}
	mux := http.NewServeMux()
	h.Routes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/media-servers/"+strconv.FormatInt(m.ID, 10)+"/libraries", nil))
	out := rec.Body.String()
	// A failed count (Shows) is left out; the list still answers.
	if rec.Code != 200 || !strings.Contains(out, `"key":"a1","title":"Films","kind":"movie","paths":[],"on":false,"count":1284`) || !strings.Contains(out, `"count":96`) || !strings.Contains(out, `"key":"c2","title":"Shows","kind":"show","paths":[],"on":false}`) {
		t.Fatalf("counts: %d %s", rec.Code, out)
	}
}

func TestPlexLibraryCount(t *testing.T) {
	w := newWorld(t)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/sections/7/all" && r.URL.Query().Get("X-Plex-Container-Size") == "0" {
			_, _ = rw.Write([]byte(`<MediaContainer size="0" totalSize="312"></MediaContainer>`))
			return
		}
		rw.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	m, err := Add(context.Background(), w.s, w.v, Input{Name: "P", Kind: "plex", URL: srv.URL, Token: tok}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	l := &Linker{Store: w.s, Vault: w.v}
	libs := []Library{{Key: "7", Kind: "show"}, {Key: "8", Kind: "movie"}}
	l.countLibraries(context.Background(), m, libs)
	if libs[0].Count == nil || *libs[0].Count != 312 || libs[1].Count != nil {
		t.Fatalf("plex counts: %+v", libs)
	}
}

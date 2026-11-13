package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Pins of the routes owned by sources (incl. search, held, bridges),
// mediaservers and the metadata routes that move out of api: the auth level
// in the permission table, who gets which status, and the top-level JSON keys.

type pinSMRoute struct {
	route  string
	perm   string // the entry in accounts.RoutePerms
	anon   int
	member int
	owner  int
	keys   string // top-level JSON keys of the owner's answer ("" when not JSON)
}

var pinSMRoutes = []pinSMRoute{
	{"GET /api/v1/sources", "manage_library", 401, 403, 200, "sources"},
	{"GET /api/v1/sources/1/stats", "manage_library", 401, 403, 404, "error"},
	{"GET /api/v1/sources/sites", "manage_library", 401, 403, 200, "sites"},
	{"POST /api/v1/sources", "settings", 401, 403, 400, "error"},
	{"PUT /api/v1/sources/1", "settings", 401, 403, 404, "error"},
	{"DELETE /api/v1/sources/1", "settings", 401, 403, 404, "error"},
	{"POST /api/v1/sources/1/test", "settings", 401, 403, 404, "error"},
	{"POST /api/v1/sources/1/pause", "settings", 401, 403, 404, "error"},
	{"POST /api/v1/sources/1/resume", "settings", 401, 403, 404, "error"},
	{"POST /api/v1/sources/1/try", "settings", 401, 403, 404, "error"},
	{"POST /api/v1/sources/check", "settings", 401, 403, 200, "kind query sources"},
	{"POST /api/v1/titles/1/search", "", 401, 403, 404, "error"},
	{"GET /api/v1/titles/1/releases", "", 401, 403, 404, "error"},
	{"POST /api/v1/releases/1/choose", "", 401, 403, 404, "error"},
	{"POST /api/v1/releases/1/turn-down", "", 401, 403, 404, "error"},
	{"GET /api/v1/held", "manage_library", 401, 403, 200, "limit offset releases total"},
	{"DELETE /api/v1/held", "manage_library", 401, 403, 200, "removed"},
	{"DELETE /api/v1/held/1", "manage_library", 401, 403, 404, "error"},
	{"GET /api/v1/turned-down", "manage_library", 401, 403, 200, "limit offset releases total"},
	{"DELETE /api/v1/turned-down", "manage_library", 401, 403, 200, "removed"},
	{"DELETE /api/v1/turned-down/1", "manage_library", 401, 403, 404, "error"},
	{"POST /api/v1/search/missing", "manage_library", 401, 403, 202, "job_ids"},
	{"POST /api/v1/search/better", "manage_library", 401, 403, 202, "job_ids"},
	{"POST /api/v1/search/feed-check", "manage_library", 401, 403, 202, "job_ids"},
	{"GET /api/v1/searches", "manage_library", 401, 403, 200, "recent running"},
	{"GET /api/v1/bridges", "manage_library", 401, 403, 200, "bridges home_ip"},
	{"POST /api/v1/bridges", "settings", 401, 403, 400, "error field"},
	{"POST /api/v1/bridges/find", "settings", 401, 403, 200, "found"},
	{"POST /api/v1/bridges/test", "settings", 401, 403, 400, "error field"},
	{"POST /api/v1/bridges/1/check", "settings", 401, 403, 404, "error"},
	{"DELETE /api/v1/bridges/1", "settings", 401, 403, 404, "error"},
	{"GET /api/v1/media-servers", "settings", 401, 403, 200, "media_servers"},
	{"POST /api/v1/media-servers", "settings", 401, 403, 400, "error"},
	{"PUT /api/v1/media-servers/1", "settings", 401, 403, 404, "error"},
	{"DELETE /api/v1/media-servers/1", "settings", 401, 403, 404, "error"},
	{"POST /api/v1/media-servers/1/test", "settings", 401, 403, 404, "error"},
	// Metadata routes that move out of api: signed-in people may read them.
	{"GET /api/v1/metadata/health", "signed_in", 401, 200, 200, "up"},
	{"GET /api/v1/metadata/movie/693134/reviews", "signed_in", 401, 404, 404, "error"},
	{"GET /api/v1/trailer/batch", "signed_in", 401, 400, 400, "error"},
	{"GET /api/v1/trailer/movie/693134", "signed_in", 401, 404, 404, "error key"},
	{"GET /api/v1/trailer/stream/x/manifest.m3u8", "signed_in", 401, 404, 404, "error"},
	{"GET /api/v1/trailer/stream/x/seg/1", "signed_in", 401, 404, 404, "error"},
	{"GET /api/v1/extras/movie/693134", "signed_in", 401, 200, 200, "scores"},
	{"GET /api/v1/wiki/movie/693134", "signed_in", 401, 200, 200, "awards badges bio tallies"},
	{"GET /api/v1/settings/trakt-token", "owner", 401, 403, 200, "token"},
	{"PUT /api/v1/settings/trakt-token", "owner", 401, 403, 400, "error"},
}

// pinSMPattern turns a probed route back into its registered pattern.
func pinSMPattern(route string) string {
	r := strings.NewReplacer("/movie/693134/reviews", "/{kind}/{tmdb_id}/reviews", "/trailer/movie/693134", "/trailer/{kind}/{tmdb_id}",
		"/trailer/stream/x/manifest.m3u8", "/trailer/stream/{id}/manifest.m3u8", "/trailer/stream/x/seg/1", "/trailer/stream/{id}/seg/{n}",
		"/extras/movie/693134", "/extras/{kind}/{id}", "/wiki/movie/693134", "/wiki/{kind}/{id}")
	route = r.Replace(route)
	return strings.NewReplacer("/1/", "/{id}/", "/1 ", "/{id} ").Replace(strings.TrimSuffix(route, "/1") + map[bool]string{true: "/{id}", false: ""}[strings.HasSuffix(route, "/1")])
}

func pinSMCall(t *testing.T, s *signedIn, method, path string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, s.base+path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(accounts.CSRFHeader, s.csrf)
	resp, err := s.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var v map[string]any
	keys := []string{}
	if json.Unmarshal(b, &v) == nil {
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	return resp.StatusCode, strings.Join(keys, " ")
}

// route: GET /api/v1/sources
// route: POST /api/v1/sources
// route: GET /api/v1/held
// route: GET /api/v1/bridges
// route: GET /api/v1/media-servers
// route: GET /api/v1/extras/{kind}/{id}
func TestPinSourcesMediaServersMetadataRoutes(t *testing.T) {
	r := newAskRig(t)
	for _, rt := range pinSMRoutes {
		method, path, _ := strings.Cut(rt.route, " ")
		req, _ := http.NewRequest(method, r.srv.URL+path, strings.NewReader("{}"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != rt.anon {
			t.Errorf("%s anonymous: %d, want %d", rt.route, resp.StatusCode, rt.anon)
		}
		if st, _ := pinSMCall(t, r.ana, method, path); st != rt.member {
			t.Errorf("%s member: %d, want %d", rt.route, st, rt.member)
		}
		st, keys := pinSMCall(t, r.owner, method, path)
		if st != rt.owner || keys != rt.keys {
			t.Errorf("%s owner: %d [%s], want %d [%s]", rt.route, st, keys, rt.owner, rt.keys)
		}
	}
}

// The permission table names the same level for every route the three modules own.
func TestPinSourcesMediaServersMetadataPermLevels(t *testing.T) {
	for _, rt := range pinSMRoutes {
		if rt.perm == "" {
			continue
		}
		got, ok := accounts.RoutePerms[pinSMPattern(rt.route)]
		if !ok || fmt.Sprint(got) != rt.perm {
			t.Errorf("%s: perm %v (listed %v), want %s", rt.route, got, ok, rt.perm)
		}
	}
	// Title search and release choice carry a permission too (not pinned by name here).
	for _, route := range []string{"POST /api/v1/titles/{id}/search", "GET /api/v1/titles/{id}/releases", "POST /api/v1/releases/{id}/choose", "POST /api/v1/releases/{id}/turn-down"} {
		if _, ok := accounts.RoutePerms[route]; !ok {
			t.Errorf("%s missing from the permission table", route)
		}
	}
}

func pinSMGolden(t *testing.T, name string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE") == "1" {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != string(got) {
		t.Fatalf("%s changed:\n%s", name, got)
	}
}

// The JSON of a source and a media server as the routes return them.
func TestPinSourceAndMediaServerViewGolden(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ref := int64(7)
	seed := 5
	src := domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://idx.example/api",
		UseFor: domain.SourceUse{Feed: true, AutoSearch: true, HandSearch: true}, Kinds: []string{"movie", "tv"},
		Categories: map[string][]int{"movie": {2000}, "tv": {5000}}, MinSeeders: &seed, Priority: 10, State: "enabled",
		SecretRef: &ref, AllowLocal: true, ProxyURL: "", Tags: nil}
	src.ID = 3
	pinSMGolden(t, "pin_source_view.golden.json", sources.SourceViewOf(src, sources.Summary{}))
	ms := domain.MediaServer{Name: "Jellyfin", Kind: "jellyfin", URL: "http://jf:8096", TokenSecretID: &ref, Enabled: true, OnImport: true,
		OnRename: true, OnDelete: false, PathFrom: "/data", PathTo: "/media", AllowLocal: true, LastOKAt: &at, LastError: "slow"}
	ms.ID = 2
	pinSMGolden(t, "pin_media_server_view.golden.json", mediaservers.ViewOf(ms))
}

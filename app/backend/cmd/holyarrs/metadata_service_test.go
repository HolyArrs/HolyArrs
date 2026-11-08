package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const (
	metadataTestInstallation = "METADATA_TEST_INSTALLATION"
	serviceArtist            = "00000000-0000-4000-8000-000000000002"
	serviceAlbum             = "00000000-0000-4000-8000-000000000003"
	serviceRecording         = "00000000-0000-4000-8000-000000000004"
	servicePressing          = "00000000-0000-4000-8000-000000000005"
	serviceTrack             = "00000000-0000-4000-8000-000000000006"
)

type metadataContractFixture struct {
	mu     sync.Mutex
	token  string
	routes map[string]int
	srv    *httptest.Server
}

func newMetadataContractFixture(t *testing.T) *metadataContractFixture {
	t.Helper()
	f := &metadataContractFixture{token: metadataTestInstallation, routes: map[string]int{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *metadataContractFixture) revoke() {
	f.mu.Lock()
	f.token = "revoked"
	f.mu.Unlock()
}

func (f *metadataContractFixture) routeCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.routes[path]
}

func (f *metadataContractFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	token := f.token
	f.routes[r.URL.Path]++
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+token || token == "revoked" {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "installation_access_refused"}})
		return
	}

	var source, sourceID string
	var data any
	switch {
	case r.URL.Path == "/v1/search":
		source = r.URL.Query().Get("provider")
		data = fixtureSearchData(source, r.URL.Query().Get("kind"), mustInt(r.URL.Query().Get("page")), mustInt(r.URL.Query().Get("limit")))
	case r.URL.Path == "/v1/movies/tmdb/550" || r.URL.Path == "/v1/movies/tmdb/551":
		source, sourceID = "tmdb", strings.TrimPrefix(r.URL.Path, "/v1/movies/tmdb/")
		data = fixtureMovie(sourceID)
	case r.URL.Path == "/v1/shows/tmdb/1396":
		source, sourceID, data = "tmdb", "1396", fixtureTMDBShow()
	case strings.HasPrefix(r.URL.Path, "/v1/shows/tmdb/1396/seasons/"):
		source, sourceID = "tmdb", "1396"
		season := strings.TrimPrefix(r.URL.Path, "/v1/shows/tmdb/1396/seasons/")
		data = fixtureTMDBSeason(season)
	case r.URL.Path == "/v1/shows/tvmaze/1":
		source, sourceID, data = "tvmaze", "1", fixtureTVMazeShow()
	case strings.HasPrefix(r.URL.Path, "/v1/music/musicbrainz/"):
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/music/musicbrainz/"), "/")
		if len(parts) != 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		source, sourceID = "musicbrainz", parts[1]
		data = fixtureMusic(parts[0], parts[1])
	default:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
		return
	}
	if data == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": "not_found"}})
		return
	}
	f.writeEnvelope(w, source, sourceID, data)
}

func (f *metadataContractFixture) writeEnvelope(w http.ResponseWriter, source, sourceID string, data any) {
	now := time.Now().UTC()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schema_version": 1,
		"source":         source,
		"source_id":      sourceID,
		"fetched_at":     now.Format(time.RFC3339Nano),
		"expires_at":     now.Add(time.Hour).Format(time.RFC3339Nano),
		"completeness":   "complete",
		"stale":          false,
		"data":           data,
	})
}

func fixtureSearchData(source, kind string, page, limit int) map[string]any {
	var item map[string]any
	switch source + "/" + kind {
	case "tmdb/movie":
		item = fixtureMovie("550")
	case "tmdb/show":
		item = fixtureTMDBShow()
	case "tvmaze/show":
		item = fixtureTVMazeShow()
	case "musicbrainz/artist":
		item = fixtureMusic("artist", serviceArtist)
	}
	items := []any{}
	if item != nil {
		items = append(items, item)
	}
	return map[string]any{"items": items, "pagination": map[string]any{"page": page, "total_pages": 1, "total_results": len(items), "offset": (page - 1) * limit, "limit": limit}}
}

func fixtureMovie(id string) map[string]any {
	return map[string]any{
		"source": "tmdb", "source_id": id, "kind": "movie", "title": "Synthetic Movie " + id,
		"summary": "Synthetic resume from the private fixture.",
		"source_data": map[string]any{"id": mustInt(id), "title": "Synthetic Movie " + id, "original_title": "Synthetic Movie " + id,
			"overview": "Synthetic resume from the private fixture.", "original_language": "en", "release_date": "2020-01-02", "runtime": 93},
	}
}

func fixtureTMDBShow() map[string]any {
	return map[string]any{
		"source": "tmdb", "source_id": "1396", "kind": "show", "title": "Synthetic TMDB Show", "summary": "Plain show summary.",
		"source_data": map[string]any{"id": 1396, "name": "Synthetic TMDB Show", "original_name": "Synthetic TMDB Show",
			"overview": "Plain show summary.", "original_language": "en", "first_air_date": "2020-01-02", "last_air_date": "2020-02-02",
			"number_of_seasons": 1, "number_of_episodes": 2, "status": "Ended", "seasons": []any{
				map[string]any{"id": 139600, "season_number": 0, "name": "Specials", "episode_count": 1},
				map[string]any{"id": 139601, "season_number": 1, "name": "Season 1", "episode_count": 1},
			}},
	}
}

func fixtureTMDBSeason(number string) map[string]any {
	n := mustInt(number)
	return map[string]any{
		"source": "tmdb", "source_id": fmt.Sprintf("%d", 139600+n), "kind": "show", "title": fmt.Sprintf("Season %d", n), "summary": "",
		"source_data": map[string]any{"id": 139600 + n, "name": fmt.Sprintf("Season %d", n), "season_number": n, "air_date": "2020-01-02",
			"episodes": []any{map[string]any{"id": 1396000 + n, "episode_number": 1, "season_number": n,
				"name": fmt.Sprintf("Episode %d", n), "air_date": "2020-01-02", "runtime": 24}}},
	}
}

func fixtureTVMazeShow() map[string]any {
	return map[string]any{
		"source": "tvmaze", "source_id": "1", "kind": "show", "title": "Synthetic TVMaze Show", "summary": "<p>Untrusted summary text.</p>",
		"source_data": map[string]any{"id": 1, "name": "Synthetic TVMaze Show", "summary": "<p>Untrusted summary text.</p>", "language": "English",
			"premiered": "2020-01-02", "ended": "2020-02-02", "status": "Ended", "runtime": 24,
			"_embedded": map[string]any{"episodes": []any{
				map[string]any{"id": 10, "name": "Special", "season": 0, "number": 1, "airdate": "2020-01-01"},
				map[string]any{"id": 11, "name": "Episode One", "season": 1, "number": 1, "airdate": "2020-01-02"},
			}}},
	}
}

func fixtureMusic(kind, id string) map[string]any {
	artist := map[string]any{"id": serviceArtist, "name": "Synthetic Artist", "sort-name": "Synthetic Artist"}
	credit := []any{map[string]any{"name": "Synthetic Artist", "artist": artist}}
	group := map[string]any{"id": serviceAlbum, "title": "Synthetic Album", "primary-type": "Album", "first-release-date": "2020-01-02"}
	recording := map[string]any{"id": serviceRecording, "title": "Synthetic Track", "length": 120000, "artist-credit": credit}
	var raw map[string]any
	switch kind {
	case "artist":
		if id != serviceArtist {
			return nil
		}
		raw = map[string]any{"id": id, "name": "Synthetic Artist", "sort-name": "Synthetic Artist", "type": "Group", "country": "US", "release-groups": []any{group}}
	case "release-group":
		if id != serviceAlbum {
			return nil
		}
		raw = map[string]any{"id": id, "title": "Synthetic Album", "primary-type": "Album", "first-release-date": "2020-01-02",
			"releases": []any{map[string]any{"id": servicePressing, "title": "Synthetic Edition", "date": "2020-01-02", "country": "US", "status": "Official"}}}
	case "release":
		if id != servicePressing {
			return nil
		}
		raw = map[string]any{"id": id, "title": "Synthetic Edition", "date": "2020-01-02", "country": "US", "status": "Official", "release-group": group,
			"artist-credit": credit, "media": []any{map[string]any{"position": 1, "format": "CD", "track-count": 1,
				"tracks": []any{map[string]any{"id": "00000000-0000-4000-8000-000000000006", "number": "1", "position": 1,
					"title": "Synthetic Track", "length": 120000, "recording": recording}}}}}
	case "recording":
		if id != serviceRecording {
			return nil
		}
		raw = recording
	default:
		return nil
	}
	return map[string]any{"source": "musicbrainz", "source_id": id, "kind": kind, "title": fmt.Sprint(raw["title"]), "summary": "", "source_data": raw}
}

func mustInt(v string) int {
	n := 0
	_, _ = fmt.Sscanf(v, "%d", &n)
	return n
}

type metadataAppRig struct {
	t      *testing.T
	store  *database.Store
	lib    *library.Service
	owner  *signedIn
	member *signedIn
	close  func()
}

func newMetadataAppRig(t *testing.T, service *metadata.Service, directMusic ...metadata.ArtistProvider) *metadataAppRig {
	t.Helper()
	previousLog := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(previousLog) })
	dir := t.TempDir()
	st, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	for _, kind := range []string{"movie", "show", "music"} {
		path := t.TempDir()
		if err := st.Write(ctx, func(tx *sql.Tx) error {
			_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: path, Kind: kind, Label: strings.ToUpper(kind[:1]) + kind[1:], Health: "ok", SameDiskAs: []int64{}}, domain.System)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	vault, err := secrets.Open(ctx, st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	au := accounts.New(st, accounts.Options{Vault: vault})
	q := jobs.New(st, jobs.Options{})
	var artists metadata.ArtistProvider
	if len(directMusic) > 0 {
		artists = directMusic[0]
	}
	lib := &library.Service{Store: st, Queue: q, Metadata: service.Movies(nil), Shows: metadata.ShowSources{
		"tmdb": service.Shows("tmdb", nil), "tvmaze": service.Shows("tvmaze", nil),
	}, Artists: service.Artists(artists)}
	a := buildHandlers(services{Store: st, Queue: q, Metadata: lib.Metadata, Library: lib, Auth: au, Secrets: vault, Version: "test"})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)

	browser := func() *signedIn {
		jar, _ := cookiejar.New(nil)
		return &signedIn{t: t, base: srv.URL, c: &http.Client{Jar: jar}}
	}
	read := func(b []byte) (string, string) {
		var v struct {
			CSRF string `json:"csrf"`
			Path string `json:"path"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		return v.CSRF, v.Path
	}
	owner := browser()
	owner.csrf, _ = read(owner.call("POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`, http.StatusCreated))
	var me struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(owner.call("GET", "/api/v1/auth/me", "", http.StatusOK), &me); err != nil {
		t.Fatal(err)
	}
	owner.id = me.ID
	_, invite := read(owner.call("POST", "/api/v1/users/invites", `{}`, http.StatusCreated))
	member := browser()
	member.csrf, _ = read(member.call("POST", "/api/v1/auth/invites/"+strings.TrimPrefix(invite, "/invite/"), `{"name":"ana","password":"a long test passphrase"}`, http.StatusCreated))
	me.ID = 0
	if err := json.Unmarshal(member.call("GET", "/api/v1/auth/me", "", http.StatusOK), &me); err != nil {
		t.Fatal(err)
	}
	member.id = me.ID
	return &metadataAppRig{t: t, store: st, lib: lib, owner: owner, member: member, close: srv.Close}
}

func newMetadataServiceForTest(t *testing.T, base string, access func(context.Context) (string, error)) *metadata.Service {
	t.Helper()
	svc, err := metadata.NewService(metadata.ServiceOptions{URL: base, Access: access, HTTP: &http.Client{}, AllowLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestMetadataServiceAuthenticatedAppPersistsProviderIdentities(t *testing.T) {
	fixture := newMetadataContractFixture(t)
	svc := newMetadataServiceForTest(t, fixture.srv.URL, func(context.Context) (string, error) { return metadataTestInstallation, nil })
	r := newMetadataAppRig(t, svc)

	var movies struct {
		Movies []discover.LookupView `json:"movies"`
	}
	if err := json.Unmarshal(r.member.call("GET", "/api/v1/titles/lookup?term=synthetic", "", http.StatusOK), &movies); err != nil {
		t.Fatal(err)
	}
	if len(movies.Movies) != 1 || movies.Movies[0].IDs["tmdb"] != "550" {
		t.Fatalf("movie lookup %+v", movies)
	}
	beforeRoute := fixture.routeCount("/v1/movies/tmdb/550")
	r.member.call("POST", "/api/v1/titles", `{"id":"tmdb:550"}`, http.StatusForbidden)
	if fixture.routeCount("/v1/movies/tmdb/550") != beforeRoute {
		t.Fatal("member movie mutation reached metadata service")
	}
	var movieAdded struct {
		Title TitleView `json:"title"`
	}
	if err := json.Unmarshal(r.owner.call("POST", "/api/v1/titles", `{"id":"tmdb:550"}`, http.StatusCreated), &movieAdded); err != nil {
		t.Fatal(err)
	}
	assertStoredIdentity(t, r.store, movieAdded.Title.ID, "movie", "tmdb", "550")
	assertTitleAddedBy(t, r.store, movieAdded.Title.ID, r.owner.id)

	for _, tc := range []struct {
		source, id string
		episodes   int
	}{{"tmdb", "1396", 2}, {"tvmaze", "1", 2}} {
		var found struct {
			Shows  []ShowResult `json:"shows"`
			Source string       `json:"source"`
		}
		path := "/api/v1/shows/lookup?term=synthetic&source=" + tc.source
		if err := json.Unmarshal(r.member.call("GET", path, "", http.StatusOK), &found); err != nil {
			t.Fatal(err)
		}
		if found.Source != tc.source || len(found.Shows) != 1 || found.Shows[0].IDs[tc.source] != tc.id {
			t.Fatalf("%s lookup %+v", tc.source, found)
		}
		detailPath := "/v1/shows/" + tc.source + "/" + tc.id
		beforeRoute = fixture.routeCount(detailPath)
		r.member.call("POST", "/api/v1/shows", fmt.Sprintf(`{"id":"%s:%s","details_source":"%s"}`, tc.source, tc.id, tc.source), http.StatusForbidden)
		if fixture.routeCount(detailPath) != beforeRoute {
			t.Fatalf("member %s show mutation reached metadata service", tc.source)
		}
		var added addedShow
		if err := json.Unmarshal(r.owner.call("POST", "/api/v1/shows", fmt.Sprintf(`{"id":"%s:%s","details_source":"%s"}`, tc.source, tc.id, tc.source), http.StatusCreated), &added); err != nil {
			t.Fatal(err)
		}
		assertStoredIdentity(t, r.store, added.Title.ID, "show", tc.source, tc.id)
		assertTitleAddedBy(t, r.store, added.Title.ID, r.owner.id)
		var episodes int
		if err := r.store.Read().QueryRow("SELECT count(*) FROM items WHERE title_id = ? AND kind = 'episode'", added.Title.ID).Scan(&episodes); err != nil {
			t.Fatal(err)
		}
		if episodes != tc.episodes {
			t.Fatalf("%s stored %d episodes, want %d", tc.source, episodes, tc.episodes)
		}
	}

	var artists struct {
		Artists []music.ArtistResult `json:"artists"`
		Source  string               `json:"source"`
	}
	if err := json.Unmarshal(r.member.call("GET", "/api/v1/artists/lookup?term=synthetic", "", http.StatusOK), &artists); err != nil {
		t.Fatal(err)
	}
	if artists.Source != "musicbrainz" || len(artists.Artists) != 1 || artists.Artists[0].MBID != serviceArtist {
		t.Fatalf("artist lookup %+v", artists)
	}
	beforeRoute = fixture.routeCount("/v1/music/musicbrainz/artist/" + serviceArtist)
	r.member.call("POST", "/api/v1/artists", `{"id":"mbid:`+serviceArtist+`"}`, http.StatusForbidden)
	if fixture.routeCount("/v1/music/musicbrainz/artist/"+serviceArtist) != beforeRoute {
		t.Fatal("member artist mutation reached metadata service")
	}
	var artistAdded struct {
		Title struct {
			ID int64 `json:"id"`
		} `json:"title"`
	}
	if err := json.Unmarshal(r.owner.call("POST", "/api/v1/artists", `{"id":"mbid:`+serviceArtist+`","albums":"all"}`, http.StatusCreated), &artistAdded); err != nil {
		t.Fatal(err)
	}
	assertStoredIdentity(t, r.store, artistAdded.Title.ID, "artist", "musicbrainz", serviceArtist)
	assertTitleAddedBy(t, r.store, artistAdded.Title.ID, r.owner.id)
	if _, err := r.lib.RefreshArtist(context.Background(), artistAdded.Title.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	assertMusicRows(t, r.store, artistAdded.Title.ID)

	if fixture.routeCount("/v1/movies/tmdb/550") == 0 || fixture.routeCount("/v1/shows/tmdb/1396/seasons/1") == 0 ||
		fixture.routeCount("/v1/music/musicbrainz/release/"+servicePressing) == 0 {
		t.Fatalf("missing app detail routes: movie=%d season=%d release=%d", fixture.routeCount("/v1/movies/tmdb/550"),
			fixture.routeCount("/v1/shows/tmdb/1396/seasons/1"), fixture.routeCount("/v1/music/musicbrainz/release/"+servicePressing))
	}
}

func TestMetadataServiceRevocationPreservesUsersAsksAndLibrary(t *testing.T) {
	fixture := newMetadataContractFixture(t)
	svc := newMetadataServiceForTest(t, fixture.srv.URL, func(context.Context) (string, error) { return metadataTestInstallation, nil })
	r := newMetadataAppRig(t, svc)

	ask := askOf(t, r.member.call("POST", "/api/v1/asks", `{"id":"tmdb:551"}`, http.StatusCreated))
	var added struct {
		Title TitleView `json:"title"`
	}
	if err := json.Unmarshal(r.owner.call("POST", "/api/v1/titles", `{"id":"tmdb:550"}`, http.StatusCreated), &added); err != nil {
		t.Fatal(err)
	}
	before := metadataStateCounts(t, r.store)
	fixture.revoke()

	r.owner.call("GET", "/api/v1/titles/lookup?term=synthetic", "", http.StatusServiceUnavailable)
	r.owner.call("GET", "/api/v1/shows/lookup?term=synthetic&source=tmdb", "", http.StatusServiceUnavailable)
	r.owner.call("GET", "/api/v1/artists/lookup?term=synthetic", "", http.StatusServiceUnavailable)
	r.owner.call("POST", "/api/v1/titles", `{"id":"tmdb:551"}`, http.StatusServiceUnavailable)
	r.member.call("POST", "/api/v1/titles", `{"id":"tmdb:551"}`, http.StatusForbidden)
	assertLegacyMetadataRevoked(t, r)

	after := metadataStateCounts(t, r.store)
	if before != after {
		t.Fatalf("revoked access changed stored state: before=%+v after=%+v", before, after)
	}
	assertAskIdentity(t, r.store, ask.ID, r.member.id, "tmdb:551")
	assertStoredIdentity(t, r.store, added.Title.ID, "movie", "tmdb", "550")
}

func TestMetadataServiceErrorsAreSafeForTheAPI(t *testing.T) {
	secret := "installation-secret-must-not-escape"
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"access", metadata.ErrServiceAccess, http.StatusServiceUnavailable},
		{"unavailable", metadata.ErrServiceUnavailable, http.StatusServiceUnavailable},
		{"busy", metadata.ErrServiceBusy, http.StatusServiceUnavailable},
		{"incomplete", metadata.ErrServiceIncomplete, http.StatusServiceUnavailable},
		{"invalid", metadata.ErrServiceInvalid, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			wrapped := fmt.Errorf("transport included %s: %w", secret, tc.err)
			if !library.ServiceError(w, wrapped) {
				t.Fatal("service error was not handled")
			}
			if w.Code != tc.status || strings.Contains(w.Body.String(), secret) || !strings.Contains(w.Body.String(), tc.err.Error()) {
				t.Fatalf("service error response: status=%d body=%q", w.Code, w.Body.String())
			}
		})
	}
	w := httptest.NewRecorder()
	if library.ServiceError(w, errors.New("direct provider error")) || w.Body.Len() != 0 {
		t.Fatalf("direct error was intercepted: status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestMetadataServicePrivateBuild7AuthenticatedApp(t *testing.T) {
	base := strings.TrimSpace(os.Getenv("HOLYARRS_METADATA_ACCEPTANCE_URL"))
	tokenFile := strings.TrimSpace(os.Getenv("HOLYARRS_METADATA_ACCEPTANCE_TOKEN_FILE"))
	if base == "" || tokenFile == "" {
		t.Skip("private build-7 endpoint and credential file are not set")
	}
	access := func(context.Context) (string, error) {
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	svc := newMetadataServiceForTest(t, base, access)
	r := newMetadataAppRig(t, svc, acceptanceDirectMusic())

	var movies struct {
		Movies []discover.LookupView `json:"movies"`
	}
	if err := json.Unmarshal(r.owner.call("GET", "/api/v1/titles/lookup?term=synthetic", "", http.StatusOK), &movies); err != nil {
		t.Fatal(err)
	}
	if len(movies.Movies) == 0 || movies.Movies[0].IDs["tmdb"] == "" {
		t.Fatalf("private movie lookup returned no TMDB identity")
	}
	movieID := movies.Movies[0].IDs["tmdb"]
	var movieAdded struct {
		Title TitleView `json:"title"`
	}
	if err := json.Unmarshal(r.owner.call("POST", "/api/v1/titles", `{"id":"tmdb:`+movieID+`"}`, http.StatusCreated), &movieAdded); err != nil {
		t.Fatal(err)
	}
	assertStoredIdentity(t, r.store, movieAdded.Title.ID, "movie", "tmdb", movieID)
	assertTitleAddedBy(t, r.store, movieAdded.Title.ID, r.owner.id)

	showIDs := make(map[string]acceptanceShowIdentity)
	for _, source := range []string{"tmdb", "tvmaze"} {
		var found struct {
			Shows []ShowResult `json:"shows"`
		}
		if err := json.Unmarshal(r.owner.call("GET", "/api/v1/shows/lookup?term=synthetic&source="+source, "", http.StatusOK), &found); err != nil {
			t.Fatal(err)
		}
		if len(found.Shows) == 0 || found.Shows[0].IDs[source] == "" {
			t.Fatalf("private %s show lookup returned no identity", source)
		}
		id := found.Shows[0].IDs[source]
		if source == "tmdb" {
			if fresh := os.Getenv("HOLYARRS_METADATA_ACCEPTANCE_TMDB_SHOW_ID"); fresh != "" {
				n, err := strconv.ParseInt(fresh, 10, 64)
				if err != nil || n <= 0 || strconv.FormatInt(n, 10) != fresh {
					t.Fatal("private TMDB show override must be a canonical positive integer")
				}
				id = fresh
			}
		}
		var added addedShow
		if err := json.Unmarshal(r.owner.call("POST", "/api/v1/shows", fmt.Sprintf(`{"id":"%s:%s","details_source":"%s"}`, source, id, source), http.StatusCreated), &added); err != nil {
			t.Fatal(err)
		}
		assertStoredIdentity(t, r.store, added.Title.ID, "show", source, id)
		assertTitleAddedBy(t, r.store, added.Title.ID, r.owner.id)
		if added.Title.Show.Episodes == 0 {
			t.Fatalf("private %s show stored no episodes", source)
		}
		showIDs[source] = acceptanceShowIdentity{titleID: added.Title.ID, sourceID: id}
	}

	var artists struct {
		Artists []music.ArtistResult `json:"artists"`
		Source  string               `json:"source"`
	}
	if err := json.Unmarshal(r.member.call("GET", "/api/v1/artists/lookup?term=synthetic", "", http.StatusOK), &artists); err != nil {
		t.Fatal(err)
	}
	artistFound := false
	for _, artist := range artists.Artists {
		artistFound = artistFound || artist.MBID == serviceArtist
	}
	if artists.Source != "musicbrainz" || !artistFound {
		t.Fatal("private artist lookup returned no matching MusicBrainz identity")
	}

	var artistAdded struct {
		Title struct {
			ID int64 `json:"id"`
		} `json:"title"`
	}
	if err := json.Unmarshal(r.owner.call("POST", "/api/v1/artists", `{"id":"mbid:`+serviceArtist+`","albums":"all"}`, http.StatusCreated), &artistAdded); err != nil {
		t.Fatal(err)
	}
	if _, err := r.lib.RefreshArtist(context.Background(), artistAdded.Title.ID, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	assertStoredIdentity(t, r.store, artistAdded.Title.ID, "artist", "musicbrainz", serviceArtist)
	assertTitleAddedBy(t, r.store, artistAdded.Title.ID, r.owner.id)
	assertMusicRows(t, r.store, artistAdded.Title.ID)
	var artistName string
	if err := r.store.Read().QueryRow("SELECT name FROM titles WHERE id = ?", artistAdded.Title.ID).Scan(&artistName); err != nil {
		t.Fatal(err)
	}
	if artistName != "Synthetic Artist Reconciled" {
		t.Fatalf("private music details did not reconcile with direct catalog: %q", artistName)
	}
	t.Logf("private build-7 app proof: movie=tmdb:%s shows=tmdb:%s,tvmaze:%s music=%s/%s/%s/%s (service path plus direct complete-catalog reconciliation)", movieID, showIDs["tmdb"].sourceID, showIDs["tvmaze"].sourceID, serviceArtist, serviceAlbum, servicePressing, serviceRecording)

	revokeSignal := strings.TrimSpace(os.Getenv("HOLYARRS_METADATA_ACCEPTANCE_REVOKE_SIGNAL"))
	if revokeSignal != "" {
		provePrivateMetadataRevocation(t, r, revokeSignal, movieAdded.Title.ID, movieID, showIDs, artistAdded.Title.ID)
	}
}

type acceptanceShowIdentity struct {
	titleID  int64
	sourceID string
}

func provePrivateMetadataRevocation(t *testing.T, r *metadataAppRig, signal string, movieTitleID int64, movieID string, showIDs map[string]acceptanceShowIdentity, artistTitleID int64) {
	t.Helper()
	ask := askOf(t, r.member.call("POST", "/api/v1/asks", `{"id":"tmdb:551"}`, http.StatusCreated))
	before := metadataStateCounts(t, r.store)

	if err := os.MkdirAll(filepath.Dir(signal), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signal, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	readyInfo, err := os.Stat(signal)
	if err != nil {
		t.Fatal(err)
	}
	revokedMarker := signal + ".revoked"
	deadline := time.Now().Add(5 * time.Minute)
	for {
		if info, statErr := os.Stat(revokedMarker); statErr == nil && info.ModTime().After(readyInfo.ModTime()) {
			break
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			t.Fatal(statErr)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for coordinated private metadata revocation")
		}
		time.Sleep(time.Second)
	}

	r.owner.call("GET", "/api/v1/titles/lookup?term=synthetic", "", http.StatusServiceUnavailable)
	r.owner.call("GET", "/api/v1/shows/lookup?term=synthetic&source=tmdb", "", http.StatusServiceUnavailable)
	r.owner.call("GET", "/api/v1/artists/lookup?term=synthetic", "", http.StatusServiceUnavailable)
	r.owner.call("POST", "/api/v1/titles", `{"id":"tmdb:552"}`, http.StatusServiceUnavailable)
	r.owner.call("POST", "/api/v1/shows", `{"id":"tmdb:1397","details_source":"tmdb"}`, http.StatusServiceUnavailable)
	r.owner.call("POST", "/api/v1/artists", `{"id":"mbid:00000000-0000-4000-8000-000000000008","albums":"all"}`, http.StatusServiceUnavailable)
	r.member.call("POST", "/api/v1/titles", `{"id":"tmdb:552"}`, http.StatusForbidden)
	assertLegacyMetadataRevoked(t, r)

	after := metadataStateCounts(t, r.store)
	if before != after {
		t.Fatalf("private revocation changed stored state: before=%+v after=%+v", before, after)
	}
	assertAskIdentity(t, r.store, ask.ID, r.member.id, "tmdb:551")
	assertStoredIdentity(t, r.store, movieTitleID, "movie", "tmdb", movieID)
	for source, show := range showIDs {
		assertStoredIdentity(t, r.store, show.titleID, "show", source, show.sourceID)
	}
	assertStoredIdentity(t, r.store, artistTitleID, "artist", "musicbrainz", serviceArtist)
	t.Log("private build-7 revocation proof: authenticated metadata reads and mutations returned 503; member mutation returned 403; stored state survived")
}

func acceptanceDirectMusic() metadata.ArtistProvider {
	return &fakeArtists{
		artists: map[string]metadata.Artist{serviceArtist: {
			MBID: serviceArtist, Name: "Synthetic Artist Reconciled", Type: "group", Status: "running",
			Albums: []metadata.Album{{MBID: serviceAlbum, Name: "Synthetic Album", PrimaryType: "album", ReleaseDate: "2020-01-02"}},
		}},
		pressings: map[string][]metadata.Pressing{serviceAlbum: {{
			MBID: servicePressing, Name: "Synthetic Edition", Status: "official", Country: "US", Date: "2020-01-02", Formats: []string{"CD"},
			Tracks: []metadata.Track{{ID: serviceTrack, RecordingID: serviceRecording, Disc: 1, Number: 1, Position: 1, Name: "Synthetic Track", LengthMS: 120000}},
		}}},
	}
}

func assertStoredIdentity(t *testing.T, st *database.Store, titleID int64, kind, source, want string) {
	t.Helper()
	var gotKind, got string
	if err := st.Read().QueryRow("SELECT kind, json_extract(source_ids, '$.'||?) FROM titles WHERE id = ?", source, titleID).Scan(&gotKind, &got); err != nil {
		t.Fatal(err)
	}
	if gotKind != kind || got != want {
		t.Fatalf("title %d identity: kind=%q %s=%q, want %q %s=%q", titleID, gotKind, source, got, kind, source, want)
	}
}

func assertTitleAddedBy(t *testing.T, st *database.Store, titleID, userID int64) {
	t.Helper()
	var actorKind string
	var actorID int64
	if err := st.ReadEvents().QueryRow("SELECT json_extract(actor, '$.kind'), json_extract(actor, '$.id') FROM events WHERE kind = ? AND title_id = ? ORDER BY id DESC LIMIT 1", domain.EventTitleAdded, titleID).Scan(&actorKind, &actorID); err != nil {
		t.Fatal(err)
	}
	if actorKind != "user" || actorID != userID {
		t.Fatalf("title %d added actor = %s:%d, want user:%d", titleID, actorKind, actorID, userID)
	}
}

func assertLegacyMetadataRevoked(t *testing.T, r *metadataAppRig) {
	t.Helper()
	if strings.EqualFold(strings.TrimSpace(os.Getenv("HOLYARRS_METADATA_ACCEPTANCE_WIRING")), "true") {
		r.owner.call("GET", "/api/v1/metadata/search?q=synthetic", "", http.StatusServiceUnavailable)
	}
}

func assertAskIdentity(t *testing.T, st *database.Store, askID, askedBy int64, ref string) {
	t.Helper()
	var gotAskedBy int64
	var gotRef, tmdbID string
	if err := st.Read().QueryRow("SELECT asked_by, ref, json_extract(source_ids, '$.tmdb') FROM asks WHERE id = ?", askID).Scan(&gotAskedBy, &gotRef, &tmdbID); err != nil {
		t.Fatal(err)
	}
	wantTMDB := strings.TrimPrefix(ref, "tmdb:")
	if gotAskedBy != askedBy || gotRef != ref || tmdbID != wantTMDB {
		t.Fatalf("ask %d identity changed: asked_by=%d ref=%q tmdb=%q, want %d %q %q", askID, gotAskedBy, gotRef, tmdbID, askedBy, ref, wantTMDB)
	}
}

func assertMusicRows(t *testing.T, st *database.Store, titleID int64) {
	t.Helper()
	var album, pressing, recording string
	if err := st.Read().QueryRow("SELECT json_extract(source_ids, '$.musicbrainz') FROM parts WHERE title_id = ? AND kind = 'album'", titleID).Scan(&album); err != nil {
		t.Fatal(err)
	}
	if err := st.Read().QueryRow("SELECT mbid FROM pressings WHERE part_id IN (SELECT id FROM parts WHERE title_id = ?)", titleID).Scan(&pressing); err != nil {
		t.Fatal(err)
	}
	if err := st.Read().QueryRow("SELECT recording_id FROM items WHERE title_id = ? AND kind = 'track'", titleID).Scan(&recording); err != nil {
		t.Fatal(err)
	}
	if album != serviceAlbum || pressing != servicePressing || recording != serviceRecording {
		t.Fatalf("music identities album=%q pressing=%q recording=%q", album, pressing, recording)
	}
}

type metadataCounts struct {
	Users, Asks, Titles, Parts, Items int
	Data                              [sha256.Size]byte
}

func metadataStateCounts(t *testing.T, st *database.Store) metadataCounts {
	t.Helper()
	var out metadataCounts
	for _, q := range []struct {
		sql string
		dst *int
	}{
		{"SELECT count(*) FROM users", &out.Users}, {"SELECT count(*) FROM asks", &out.Asks}, {"SELECT count(*) FROM titles", &out.Titles},
		{"SELECT count(*) FROM parts", &out.Parts}, {"SELECT count(*) FROM items", &out.Items},
	} {
		if err := st.Read().QueryRow(q.sql).Scan(q.dst); err != nil {
			t.Fatal(err)
		}
	}
	// Compare complete core rows in memory; never print account or library data.
	// Auth sessions are excluded because normal requests update last-seen times.
	h := sha256.New()
	for _, table := range []string{"users", "asks", "titles", "parts", "items", "pressings"} {
		rows, err := st.Read().Query("SELECT * FROM " + table + " ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		_, _ = fmt.Fprintln(h, table)
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			data, err := json.Marshal(values)
			if err != nil {
				_ = rows.Close()
				t.Fatal("cannot encode private core-row comparison")
			}
			_, _ = h.Write(data)
			_, _ = h.Write([]byte{'\n'})
		}
		readErr := rows.Err()
		_ = rows.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
	}
	copy(out.Data[:], h.Sum(nil))
	return out
}

package migrate

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// seerrAPI pages people and requests the way the manual describes (take,
// skip, pageInfo, results), 100 at a time; 130 people prove the paging.
func seerrAPI(t *testing.T, key string) string {
	var users []any
	users = append(users, map[string]any{"id": 1, "email": "luis@example.com", "username": "", "plexUsername": "luis", "plexId": "plex-9", "userType": 1,
		"permissions": 2, "avatar": "https://plex.example/avatar/1", "createdAt": "2024-01-01T00:00:00.000Z", "plexToken": "S3CR3T_PLEX"})
	users = append(users, map[string]any{"id": 2, "email": "ana@example.com", "displayName": "Ana", "jellyfinUserId": "jellyfin-2", "userType": 3,
		"permissions": 32 | 128, "movieQuotaLimit": 5, "movieQuotaDays": 7, "tvQuotaLimit": 2, "tvQuotaDays": 14})
	for i := 3; i <= 130; i++ {
		users = append(users, map[string]any{"id": i, "email": "u" + strconv.Itoa(i) + "@example.com", "username": "u" + strconv.Itoa(i), "userType": 2, "permissions": 32})
	}
	page := func(all []any, r *http.Request) any {
		take, _ := strconv.Atoi(r.URL.Query().Get("take"))
		skip, _ := strconv.Atoi(r.URL.Query().Get("skip"))
		end := min(skip+take, len(all))
		return map[string]any{"pageInfo": map[string]any{"pages": (len(all) + take - 1) / take, "page": skip/take + 1, "results": len(all)},
			"results": all[min(skip, len(all)):end]}
	}
	reqs := []any{
		map[string]any{"id": 10, "status": 1, "is4k": false, "createdAt": "2026-10-01T00:00:00.000Z", "updatedAt": "2026-10-01T00:00:00.000Z",
			"media": map[string]any{"mediaType": "movie", "tmdbId": 603}, "requestedBy": map[string]any{"id": 2, "plexToken": "S3CR3T_IN_REQUEST"}},
		map[string]any{"id": 11, "status": 2, "media": map[string]any{"mediaType": "tv", "tmdbId": 1396, "tvdbId": 81189},
			"seasons": []any{map[string]any{"seasonNumber": 1}, map[string]any{"seasonNumber": 2}}, "requestedBy": map[string]any{"id": 2}, "modifiedBy": map[string]any{"id": 1}},
		map[string]any{"id": 12, "status": 5, "media": map[string]any{"mediaType": "movie", "tmdbId": 680}, "requestedBy": map[string]any{"id": 1}},
		map[string]any{"id": 13, "status": 3, "media": map[string]any{"mediaType": "movie", "tmdbId": 681}, "requestedBy": map[string]any{"id": 2}},
	}
	srv := fakeAPI(t, "X-API-Key", key, map[string]any{
		"/api/v1/status":        map[string]any{"version": "3.5.0", "commitTag": "x"},
		"/api/v1/user":          func(r *http.Request) any { return page(users, r) },
		"/api/v1/request":       func(r *http.Request) any { return page(reqs, r) },
		"/api/v1/settings/main": map[string]any{"apiKey": "S3CR3T_MAIN", "applicationTitle": "Ask here", "defaultPermissions": 32, "defaultQuotas": map[string]any{"movie": map[string]any{"quotaLimit": 3, "quotaDays": 7}, "tv": map[string]any{}}},
	})
	return srv.URL
}

func TestSeerrAPI(t *testing.T) {
	key := "S3CR3T_SEERR"
	u := seerrAPI(t, key)
	info, err := Probe(context.Background(), api(Seerr, u, key))
	if err != nil || info.Version != "3.5.0" {
		t.Fatalf("probe %+v %v", info, err)
	}
	s := read(t, api(Seerr, u, key))
	if len(s.Users) != 130 {
		t.Fatalf("users %d (paging)", len(s.Users))
	}
	luis, ana := s.Users[0], s.Users[1]
	if luis.Name != "luis" || !luis.Admin || luis.Kind != "plex" || luis.ExternalID != "plex-9" || luis.Email != "luis@example.com" || !slices.Contains(luis.Permissions, "ADMIN") {
		t.Fatalf("luis %+v", luis)
	}
	if ana.Name != "Ana" || ana.Admin || ana.Kind != "jellyfin" || ana.ExternalID != "jellyfin-2" || ana.MovieLimit != 5 || ana.MovieDays != 7 || ana.ShowLimit != 2 || ana.ShowDays != 14 ||
		!slices.Equal(ana.Permissions, []string{"REQUEST", "AUTO_APPROVE"}) {
		t.Fatalf("ana %+v", ana)
	}
	if len(s.Requests) != 2 || !hasNote(s, "2 finished, failed or turned-down") {
		t.Fatalf("requests %+v %v", s.Requests, s.Notes)
	}
	m, tv := s.Requests[0], s.Requests[1]
	if m.Kind != "movie" || m.IDs["tmdb"] != "603" || m.State != "pending" || m.By != 2 || m.ByName != "Ana" || m.Created == "" {
		t.Fatalf("movie request %+v", m)
	}
	if tv.Kind != "show" || tv.IDs["tvdb"] != "81189" || tv.State != "approved" || !slices.Equal(tv.Seasons, []int{1, 2}) || tv.DecidedBy != 1 {
		t.Fatalf("show request %+v", tv)
	}
	if s.Settings["default_movie_limit"] != int64(3) || s.Settings["applicationTitle"] != "Ask here" {
		t.Fatalf("defaults %+v", s.Settings)
	}
	if _, ok := s.Settings["default_show_limit"]; ok {
		t.Fatalf("empty show quota became explicit: %+v", s.Settings)
	}
}

func TestSeerrDefaultQuotaPresenceSurvivesReaderAndPlan(t *testing.T) {
	missing := seerrDefaults(map[string]any{"defaultQuotas": map[string]any{"movie": map[string]any{}, "tv": map[string]any{}}})
	for _, key := range []string{"default_movie_limit", "default_movie_days", "default_show_limit", "default_show_days"} {
		if _, ok := missing[key]; ok {
			t.Fatalf("missing quota became explicit %s: %+v", key, missing)
		}
	}
	zero := seerrDefaults(map[string]any{"defaultQuotas": map[string]any{"movie": map[string]any{"quotaLimit": 0, "quotaDays": 7}}})
	p := BuildPlan([]Input{{ID: 1, Snap: &Snapshot{Kind: Seerr, Settings: zero}}}, Choices{})
	it := item(t, p, GroupRequests, "wishlist_defaults")
	if it.AskSettings == nil || it.AskSettings.MoviesPerWeek == nil || *it.AskSettings.MoviesPerWeek != 0 || it.AskSettings.ShowsPerWeek != nil {
		t.Fatalf("explicit unlimited/default presence lost: %+v", it.AskSettings)
	}
	u := seerrUser(map[string]any{"id": 2, "username": "zero", "movieQuotaLimit": 0, "movieQuotaDays": 7})
	up := BuildPlan([]Input{{ID: 2, Snap: &Snapshot{Kind: Seerr, Users: []User{u}}}}, Choices{})
	if _, ok := notMoved(up, "seerr users[0].movie_quota"); !ok {
		t.Fatal("explicit per-person zero was silently treated as inherited")
	}
}

// seerrDB is a Seerr or Overseerr database as the manual lists it.
func seerrDB(seerr bool) []string {
	out := []string{
		`CREATE TABLE "user" (id INTEGER PRIMARY KEY, email TEXT, username TEXT, plexUsername TEXT, jellyfinUsername TEXT, userType INTEGER, plexId INTEGER, jellyfinUserId TEXT, permissions INTEGER, avatar TEXT, movieQuotaLimit INTEGER, movieQuotaDays INTEGER, tvQuotaLimit INTEGER, tvQuotaDays INTEGER, password TEXT, resetPasswordGuid TEXT, plexToken TEXT, createdAt TEXT)`,
		`INSERT INTO "user" VALUES (1, 'luis@example.com', NULL, 'luis', NULL, 1, 9, NULL, 2, '/avatar', NULL, NULL, NULL, NULL, 'S3CR3T_HASH', 'S3CR3T_GUID', 'S3CR3T_PLEX', '2024-01-01 00:00:00')`,
		`INSERT INTO "user" VALUES (2, 'bo@example.com', 'bo', NULL, NULL, 2, NULL, NULL, 32, NULL, 1, 7, NULL, NULL, 'S3CR3T_HASH2', NULL, NULL, '2024-02-01 00:00:00')`,
		`CREATE TABLE user_settings (id INTEGER PRIMARY KEY, pgpKey TEXT, pushbulletAccessToken TEXT)`,
		`INSERT INTO user_settings VALUES (1, 'S3CR3T_PGP', 'S3CR3T_PB')`,
		`CREATE TABLE media (id INTEGER PRIMARY KEY, mediaType TEXT, tmdbId INTEGER, tvdbId INTEGER, imdbId TEXT, status INTEGER)`,
		`INSERT INTO media VALUES (1, 'movie', 603, NULL, 'tt0133093', 2), (2, 'tv', 1396, 81189, NULL, 3), (3, 'movie', 680, NULL, NULL, 5)`,
		`CREATE TABLE media_request (id INTEGER PRIMARY KEY, status INTEGER, type TEXT, is4k INTEGER, mediaId INTEGER, requestedById INTEGER, modifiedById INTEGER, createdAt TEXT, updatedAt TEXT)`,
		`INSERT INTO media_request VALUES (10, 1, 'movie', 0, 1, 2, NULL, '2026-10-01', '2026-10-01'), (11, 2, 'tv', 1, 2, 2, 1, '2026-10-02', '2026-10-03'), (12, 5, 'movie', 0, 3, 1, 1, '2026-01-01', '2026-01-02')`,
		`CREATE TABLE season_request (id INTEGER PRIMARY KEY, seasonNumber INTEGER, status INTEGER, requestId INTEGER)`,
		`INSERT INTO season_request VALUES (1, 3, 1, 11), (2, 1, 1, 11)`,
		`CREATE TABLE session (id TEXT, expiredAt INTEGER, json TEXT)`, `INSERT INTO session VALUES ('S3CR3T_SID', 0, '{"S3CR3T":1}')`,
	}
	if seerr {
		out = append(out, `CREATE TABLE blocklist (id INTEGER PRIMARY KEY)`, `CREATE TABLE watchlist (id INTEGER PRIMARY KEY)`)
	}
	return out
}

func TestSeerrBackup(t *testing.T) {
	settings := []byte(`{"clientId":"S3CR3T_CLIENT","main":{"apiKey":"S3CR3T_MAIN","defaultPermissions":32,"defaultQuotas":{"movie":{"quotaLimit":2,"quotaDays":7},"tv":{"quotaLimit":1,"quotaDays":7}}},"radarr":[{"apiKey":"S3CR3T_RADARR"}]}`)
	for _, seerr := range []bool{true, false} {
		kind := Overseerr
		if seerr {
			kind = Seerr
		}
		dir := t.TempDir()
		zp := filepath.Join(dir, "config.zip")
		makeZip(t, zp, map[string][]byte{"config/db/db.sqlite3": dbBytes(t, seerrDB(seerr)...), "config/settings.json": settings})
		info, err := Probe(context.Background(), file(kind, zp))
		if err != nil || info.Kind != kind {
			t.Fatalf("%s probe %+v %v", kind, info, err)
		}
		other := Seerr
		if seerr {
			other = Overseerr
		}
		if _, err := Read(context.Background(), file(other, zp)); !errors.Is(err, ErrWrongApp) {
			t.Fatalf("%s read as %s: %v", kind, other, err)
		}
		s := read(t, file(kind, zp))
		if len(s.Users) != 2 || s.Users[0].Name != "luis" || !s.Users[0].Admin || s.Users[0].ExternalID != "9" || s.Users[1].Name != "bo" || s.Users[1].MovieLimit != 1 || s.Users[1].MovieDays != 7 {
			t.Fatalf("%s users %+v", kind, s.Users)
		}
		if len(s.Requests) != 2 || s.Requests[1].State != "approved" || !s.Requests[1].FourK || !slices.Equal(s.Requests[1].Seasons, []int{1, 3}) ||
			s.Requests[0].IDs["imdb"] != "tt0133093" || s.Requests[0].ByName != "bo" || s.Requests[1].DecidedBy != 1 {
			t.Fatalf("%s requests %+v", kind, s.Requests)
		}
		if s.Settings["default_movie_limit"] != int64(2) || s.Settings["default_show_days"] != int64(7) || !slices.Equal(anyStrings(s.Settings["default_permissions"]), []string{"REQUEST"}) {
			t.Fatalf("%s settings %+v", kind, s.Settings)
		}
	}
	// A folder laid out like the app's config folder reads the same.
	dir := t.TempDir()
	makeDB(t, filepath.Join(mkdir(t, filepath.Join(dir, "db")), "db.sqlite3"), seerrDB(true)...)
	if s := read(t, file(Seerr, dir)); len(s.Users) != 2 {
		t.Fatalf("folder %+v", s.Users)
	}
	// A database without the request tables is not Seerr's.
	bad := filepath.Join(t.TempDir(), "db.sqlite3")
	makeDB(t, bad, `CREATE TABLE "user" (id INTEGER)`)
	if _, err := Read(context.Background(), file(Seerr, bad)); !errors.Is(err, ErrWrongApp) {
		t.Fatalf("not seerr: %v", err)
	}
}

func anyStrings(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, e := range l {
		s, _ := e.(string)
		out = append(out, s)
	}
	if out == nil {
		if ss, ok := v.([]string); ok {
			return ss
		}
	}
	return out
}

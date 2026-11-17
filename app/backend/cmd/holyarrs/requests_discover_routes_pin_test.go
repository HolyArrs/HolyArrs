package main

import (
	"net/http"
	"net/http/cookiejar"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
)

// rdpRoutes is what each route of requests, discover, migrate, the arr
// API settings and the AI connection needs today: {permission, key scope}.
// The table must stay true as routes move between modules.
var rdpRoutes = map[string][2]string{
	"DELETE /api/v1/asks/{id}":                             {"ask", "write"},
	"DELETE /api/v1/explore/strip/counts":                  {"signed_in", "write"},
	"DELETE /api/v1/metadata/album/{mbid}/follow":          {"signed_in", "write"},
	"DELETE /api/v1/metadata/movie/{tmdb_id}/follow":       {"signed_in", "write"},
	"DELETE /api/v1/metadata/show/{tmdb_id}/follow":        {"signed_in", "write"},
	"DELETE /api/v1/migrate/saved-keys":                    {"owner", "admin"},
	"DELETE /api/v1/migrate/snapshots/{id}":                {"users", "admin"},
	"DELETE /api/v1/outside-lists/{id}":                    {"settings", "write"},
	"DELETE /api/v1/settings/old-app-api/keys/{side}":      {"settings", "admin"},
	"DELETE /api/v1/titles/{id}/follow":                    {"signed_in", "write"},
	"GET /.well-known/oauth-authorization-server":          {"public", "admin"},
	"GET /.well-known/oauth-protected-resource":            {"public", "admin"},
	"GET /.well-known/oauth-protected-resource/{path...}":  {"public", "admin"},
	"GET /api/v1/ask-limits":                               {"users", "admin"},
	"GET /api/v1/asked-by":                                 {"signed_in", "read"},
	"GET /api/v1/asks":                                     {"signed_in", "read"},
	"GET /api/v1/asks/limit":                               {"ask", "read"},
	"GET /api/v1/asks/{id}":                                {"signed_in", "read"},
	"GET /api/v1/asks/{id}/parts":                          {"signed_in", "read"},
	"GET /api/v1/calendar":                                 {"signed_in", "read"},
	"GET /api/v1/discover":                                 {"signed_in", "read"},
	"GET /api/v1/explore/browse":                           {"signed_in", "read"},
	"GET /api/v1/explore/browse/category":                  {"signed_in", "read"},
	"GET /api/v1/explore/browse/section":                   {"signed_in", "read"},
	"GET /api/v1/explore/genre":                            {"signed_in", "read"},
	"GET /api/v1/explore/genre-row":                        {"signed_in", "read"},
	"GET /api/v1/explore/head":                             {"signed_in", "read"},
	"GET /api/v1/explore/music":                            {"signed_in", "read"},
	"GET /api/v1/explore/music/rising":                     {"signed_in", "read"},
	"GET /api/v1/explore/new":                              {"signed_in", "read"},
	"GET /api/v1/explore/new/music":                        {"signed_in", "read"},
	"GET /api/v1/explore/new/service":                      {"signed_in", "read"},
	"GET /api/v1/explore/new/services":                     {"signed_in", "read"},
	"GET /api/v1/explore/service-top":                      {"signed_in", "read"},
	"GET /api/v1/explore/strip":                            {"signed_in", "read"},
	"GET /api/v1/explore/tail":                             {"signed_in", "read"},
	"GET /api/v1/explore/title-stats":                      {"owner", "admin"},
	"GET /api/v1/explore/top":                              {"signed_in", "read"},
	"GET /api/v1/mcp":                                      {"signed_in", "mcp"},
	"GET /api/v1/metadata/album/{mbid}/follow":             {"signed_in", "read"},
	"GET /api/v1/metadata/health":                          {"signed_in", "read"},
	"GET /api/v1/metadata/movie/{tmdb_id}/follow":          {"signed_in", "read"},
	"GET /api/v1/metadata/search":                          {"signed_in", "read"},
	"GET /api/v1/metadata/show/{tmdb_id}/follow":           {"signed_in", "read"},
	"GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}": {"signed_in", "read"},
	"GET /api/v1/metadata/{kind}/{tmdb_id}":                {"signed_in", "read"},
	"GET /api/v1/metadata/{kind}/{tmdb_id}/anime":          {"signed_in", "read"},
	"GET /api/v1/metadata/{kind}/{tmdb_id}/more-from":      {"signed_in", "read"},
	"GET /api/v1/metadata/{kind}/{tmdb_id}/reviews":        {"signed_in", "read"},
	"GET /api/v1/migrate/moves":                            {"users", "admin"},
	"GET /api/v1/migrate/moves/{id}":                       {"users", "admin"},
	"GET /api/v1/migrate/moves/{id}/report":                {"users", "admin"},
	"GET /api/v1/migrate/saved-keys":                       {"owner", "admin"},
	"GET /api/v1/migrate/snapshots":                        {"users", "admin"},
	"GET /api/v1/migrate/snapshots/{id}":                   {"users", "admin"},
	"GET /api/v1/my/follows":                               {"signed_in", "read"},
	"GET /api/v1/my/plex-watchlist":                        {"signed_in", "read"},
	"GET /api/v1/outside-lists":                            {"settings", "read"},
	"GET /api/v1/people/popular":                           {"signed_in", "read"},
	"GET /api/v1/people/search":                            {"signed_in", "read"},
	"GET /api/v1/people/shelves":                           {"signed_in", "read"},
	"GET /api/v1/people/shelves/{id}":                      {"signed_in", "read"},
	"GET /api/v1/people/{id}":                              {"signed_in", "read"},
	"GET /api/v1/reports":                                  {"signed_in", "read"},
	"GET /api/v1/search":                                   {"signed_in", "read"},
	"GET /api/v1/search/suggest":                           {"signed_in", "read"},
	"GET /api/v1/searches":                                 {"manage_library", "read"},
	"GET /api/v1/settings/ai":                              {"signed_in", "admin"},
	"GET /api/v1/settings/asks":                            {"settings", "read"},
	"GET /api/v1/settings/old-app-api":                     {"settings", "admin"},
	"GET /api/v1/settings/title-defaults":                  {"manage_library", "read"},
	"GET /api/v1/titles/{id}/follow":                       {"signed_in", "read"},
	"GET /api/v1/watch-services":                           {"signed_in", "read"},
	"GET /mcp":                                             {"signed_in", "mcp"},
	"GET /oauth/authorize":                                 {"public", "admin"},
	"POST /api/v1/asks":                                    {"ask", "write"},
	"POST /api/v1/asks/remove":                            {"approve", "write"},
	"POST /api/v1/asks/{id}/approve":                       {"approve", "write"},
	"POST /api/v1/asks/{id}/turn-down":                     {"approve", "write"},
	"POST /api/v1/auth/setup/done":                         {"owner", "admin"},
	"POST /api/v1/explore/counts":                          {"signed_in", "write"},
	"POST /api/v1/explore/strip/counts":                    {"signed_in", "write"},
	"POST /api/v1/mcp":                                     {"signed_in", "mcp"},
	"POST /api/v1/migrate/dry-run":                         {"users", "admin"},
	"POST /api/v1/migrate/moves":                           {"users", "admin"},
	"POST /api/v1/migrate/moves/{id}/undo":                 {"users", "admin"},
	"POST /api/v1/migrate/plan":                            {"users", "admin"},
	"POST /api/v1/migrate/probe":                           {"users", "admin"},
	"POST /api/v1/migrate/read":                            {"users", "admin"},
	"POST /api/v1/my/plex-watchlist/check":                 {"signed_in", "write"},
	"POST /api/v1/outside-lists":                           {"settings", "write"},
	"POST /api/v1/outside-lists/{id}/sync":                 {"settings", "write"},
	"POST /api/v1/people/{id}/movies/review":               {"signed_in", "read"},
	"POST /api/v1/reports":                                 {"signed_in", "write"},
	"POST /api/v1/reports/{id}/fix":                        {"manage_library", "write"},
	"POST /api/v1/reports/{id}/reopen":                     {"manage_library", "write"},
	"POST /api/v1/search/better":                           {"manage_library", "write"},
	"POST /api/v1/search/feed-check":                       {"manage_library", "write"},
	"POST /api/v1/search/missing":                          {"manage_library", "write"},
	"POST /api/v1/settings/import-secrets":                 {"owner", "admin"},
	"POST /api/v1/settings/old-app-api/keys/{side}":        {"settings", "admin"},
	"POST /mcp":                                   {"signed_in", "mcp"},
	"POST /oauth/register":                        {"public", "admin"},
	"POST /oauth/token":                           {"public", "admin"},
	"PUT /api/v1/ask-limits/{id}":                 {"users", "admin"},
	"PUT /api/v1/metadata/album/{mbid}/follow":    {"signed_in", "write"},
	"PUT /api/v1/metadata/movie/{tmdb_id}/follow": {"signed_in", "write"},
	"PUT /api/v1/metadata/show/{tmdb_id}/follow":  {"signed_in", "write"},
	"PUT /api/v1/my/plex-watchlist":               {"signed_in", "write"},
	"PUT /api/v1/outside-lists/{id}":              {"settings", "write"},
	"PUT /api/v1/settings/ai":                     {"owner", "admin"},
	"PUT /api/v1/settings/asks":                   {"settings", "write"},
	"PUT /api/v1/settings/old-app-api":            {"settings", "write"},
	"PUT /api/v1/settings/title-defaults":         {"settings", "write"},
	"PUT /api/v1/titles/{id}/follow":              {"signed_in", "write"},
}

func TestRDPRoutePermsAndScopes(t *testing.T) {
	for p, want := range rdpRoutes {
		need, ok := accounts.RoutePerms[p]
		if !ok {
			t.Errorf("%s: no entry in accounts.RoutePerms", p)
			continue
		}
		if string(need) != want[0] {
			t.Errorf("%s: needs %q, pinned %q", p, need, want[0])
		}
		if got := accounts.RouteScope(p); got != want[1] {
			t.Errorf("%s: scope %q, pinned %q", p, got, want[1])
		}
	}
	if len(rdpRoutes) < 100 {
		t.Fatalf("pin table has %d routes", len(rdpRoutes))
	}
}

// rdpCall is one request and what the owner (luis) and a member (ana) get.
// Keys are the sorted top-level JSON names of the answer ("" when the body
// is not pinned).
type rdpCall struct {
	req        string // "METHOD /path body"
	owner, mem int
	ownerKeys  string
	memberKeys string
}

var rdpReads = []rdpCall{
	{"GET /api/v1/asks", 200, 200, "asks", "asks"},
	{"GET /api/v1/asks/limit", 200, 200, "four_k movies music no_limit qualities shows", "four_k movies music no_limit qualities shows"},
	{"GET /api/v1/asks/1", 404, 404, "error", "error"},
	{"GET /api/v1/asks/1/parts", 404, 404, "error", "error"},
	{"GET /api/v1/settings/asks", 200, 403, "four_k_library_folder_id four_k_pick_rules_id library_folder_id movies_per_week music_library_folder_id music_per_week music_pick_rules_id pick_rules_id plex_friends plex_watchlists plex_watchlists_everyone qualities search_on_approve show_four_k_library_folder_id show_four_k_pick_rules_id show_library_folder_id show_pick_rules_id shows_per_week", "error"},
	{"GET /api/v1/ask-limits", 200, 403, "limits", "error"},
	{"GET /api/v1/my/follows", 200, 200, "follows", "follows"},
	{"GET /api/v1/titles/1/follow", 404, 404, "error", "error"},
	{"GET /api/v1/metadata/show/100/follow", 200, 200, "following tmdb_id", "following tmdb_id"},
	{"GET /api/v1/metadata/movie/693134/follow", 200, 200, "following tmdb_id", "following tmdb_id"},
	{"GET /api/v1/metadata/album/abc/follow", 400, 400, "error", "error"},
	{"GET /api/v1/reports", 200, 200, "open reports", "reports"},
	{"GET /api/v1/outside-lists", 200, 403, "outside_lists", "error"},
	{"GET /api/v1/settings/title-defaults", 200, 403, "movie show", "error"},
	{"GET /api/v1/my/plex-watchlist", 200, 200, "can_read chosen everyone items linked may_ask movies on read_by_owner shows", "can_read chosen everyone items linked may_ask movies on read_by_owner shows"},
	{"GET /api/v1/asked-by?tmdb_id=693134&kind=movie", 200, 200, "titles", "titles"},
	{"GET /api/v1/calendar", 200, 200, "dates from my_follows people titles to", "dates from my_follows people titles to"},
	{"GET /api/v1/discover", 400, 400, "error", "error"},
	{"GET /api/v1/search?q=dune", 200, 200, "more query titles", "more query titles"},
	{"GET /api/v1/search/suggest?q=du", 200, 200, "completions items q", "completions items q"},
	{"GET /api/v1/explore/head", 200, 200, "building head hero seeds tail_cap tail_rows taste type", "building head hero seeds tail_cap tail_rows taste type"},
	{"GET /api/v1/explore/title-stats", 200, 403, "days layers templates", "error"},
	{"GET /api/v1/people/search?q=a", 503, 503, "error", "error"},
	{"GET /api/v1/people/popular", 503, 503, "error", "error"},
	{"GET /api/v1/people/shelves", 503, 503, "error", "error"},
	{"GET /api/v1/mcp", 401, 401, "error", "error"},
	{"GET /api/v1/settings/ai", 200, 200, "enabled path versions", "enabled path versions"},
	{"GET /api/v1/settings/old-app-api", 200, 403, "delete_files enabled pushed sides versions", "error"},
	{"GET /api/v1/migrate/snapshots", 200, 403, "snapshots", "error"},
	{"GET /api/v1/migrate/moves", 200, 403, "moves", "error"},
	{"GET /api/v1/migrate/saved-keys", 200, 403, "saved", "error"},
	{"GET /api/v1/explore/browse", 200, 200, "sections tiles type", "sections tiles type"},
	{"GET /api/v1/explore/new", 200, 200, "building hero more page rail rows services_set type", "building hero more page rail rows services_set type"},
	{"GET /api/v1/explore/music", 200, 200, "cold genre genres hero hero_alts kid next partial region regions rows total v2 v7", "cold genre genres hero hero_alts kid next partial region regions rows total v2 v7"},
	{"GET /api/v1/migrate/moves/999", 404, 403, "error", "error"},
	{"GET /api/v1/migrate/moves/999/report", 404, 403, "error", "error"},
	{"GET /api/v1/migrate/snapshots/999", 404, 403, "error", "error"},
	{"GET /api/v1/watch-services", 400, 400, "error", "error"},
	{"GET /api/v1/metadata/movie/693134", 501, 501, "error", "error"},
	{"GET /api/v1/metadata/show/100/seasons/1", 501, 501, "error", "error"},
}

var rdpWrites = []rdpCall{
	{"POST /api/v1/asks {}", 400, 400, "error", "error"},
	{"DELETE /api/v1/asks/999 ", 404, 404, "error", "error"},
	{"POST /api/v1/asks/999/approve {}", 404, 403, "error", "error"},
	{"POST /api/v1/asks/999/turn-down {}", 404, 403, "error", "error"},
	{"PUT /api/v1/ask-limits/999 {}", 404, 403, "error", "error"},
	{"PUT /api/v1/titles/999/follow {}", 404, 404, "error", "error"},
	{"DELETE /api/v1/titles/999/follow ", 404, 404, "error", "error"},
	{"PUT /api/v1/metadata/show/100/follow {}", 200, 200, "following tmdb_id", "following tmdb_id"},
	{"PUT /api/v1/metadata/movie/693134/follow {}", 200, 200, "following tmdb_id", "following tmdb_id"},
	{"PUT /api/v1/metadata/album/abc/follow {}", 400, 400, "error", "error"},
	{"POST /api/v1/reports {}", 404, 404, "error", "error"},
	{"POST /api/v1/reports/999/fix {}", 404, 403, "error", "error"},
	{"POST /api/v1/reports/999/reopen {}", 404, 403, "error", "error"},
	{"POST /api/v1/outside-lists {}", 400, 403, "error", "error"},
	{"PUT /api/v1/outside-lists/999 {}", 404, 403, "error", "error"},
	{"DELETE /api/v1/outside-lists/999 ", 404, 403, "error", "error"},
	{"POST /api/v1/outside-lists/999/sync {}", 404, 403, "error", "error"},
	{"PUT /api/v1/settings/title-defaults {}", 200, 403, "movie show", "error"},
	{"PUT /api/v1/my/plex-watchlist {}", 200, 200, "can_read chosen everyone items linked may_ask movies on read_by_owner shows", "can_read chosen everyone items linked may_ask movies on read_by_owner shows"},
	{"POST /api/v1/my/plex-watchlist/check {}", 409, 409, "error", "error"},
	{"POST /api/v1/explore/counts {}", 400, 400, "error", "error"},
	{"POST /api/v1/explore/strip/counts {}", 400, 400, "error", "error"},
	{"DELETE /api/v1/explore/strip/counts ", 204, 204, "", ""},
	{"POST /api/v1/people/999/movies/review {}", 400, 400, "error", "error"},
	{"POST /api/v1/mcp {}", 401, 401, "error", "error"},
	{"PUT /api/v1/settings/ai {}", 200, 403, "enabled path versions", "error"},
	{"PUT /api/v1/settings/old-app-api {}", 200, 403, "delete_files enabled pushed sides versions", "error"},
	{"POST /api/v1/settings/old-app-api/keys/sonarr {}", 201, 403, "key note path side", "error"},
	{"DELETE /api/v1/settings/old-app-api/keys/sonarr ", 204, 403, "", "error"},
	{"POST /api/v1/migrate/probe {}", 400, 403, "error", "error"},
	{"POST /api/v1/migrate/read {}", 400, 403, "error", "error"},
	{"POST /api/v1/migrate/plan {}", 400, 403, "error", "error"},
	{"POST /api/v1/migrate/dry-run {}", 400, 403, "error", "error"},
	{"POST /api/v1/migrate/moves {}", 400, 403, "error", "error"},
	{"POST /api/v1/migrate/moves/999/undo {}", 404, 403, "error", "error"},
	{"DELETE /api/v1/migrate/snapshots/999 ", 404, 403, "error", "error"},
	{"DELETE /api/v1/migrate/saved-keys ", 200, 403, "forgotten", "error"},
	{"POST /api/v1/settings/import-secrets {}", 400, 403, "error", "error"},
}

// Status, auth level and response shape of these routes, as the owner and
// as a plain member see them.
func TestRDPRoutesAsOwnerAndMember(t *testing.T) {
	r := newAskRig(t)
	for _, set := range [][]rdpCall{rdpReads, rdpWrites} {
		for _, c := range set {
			if st, keys := r.rdpSend(t, r.owner, c.req); st != c.owner || (c.ownerKeys != "" && keys != c.ownerKeys) {
				t.Errorf("owner %s: %d [%s], pinned %d [%s]", c.req, st, keys, c.owner, c.ownerKeys)
			}
			if st, keys := r.rdpSend(t, r.ana, c.req); st != c.mem || (c.memberKeys != "" && keys != c.memberKeys) {
				t.Errorf("member %s: %d [%s], pinned %d [%s]", c.req, st, keys, c.mem, c.memberKeys)
			}
		}
	}
}

// Nobody signed in: every one of those routes is refused with 401 before
// it looks at the request.
func TestRDPRoutesNeedSignIn(t *testing.T) {
	r := newAskRig(t)
	jar, _ := cookiejar.New(nil)
	nobody := &signedIn{t: t, base: r.srv.URL, c: &http.Client{Jar: jar}}
	n := 0
	for _, set := range [][]rdpCall{rdpReads, rdpWrites} {
		for _, c := range set {
			if st, _ := r.rdpSend(t, nobody, c.req); st != 401 {
				t.Errorf("no session %s: %d, want 401", c.req, st)
			}
			n++
		}
	}
	if n < 70 {
		t.Fatalf("only %d calls", n)
	}
}

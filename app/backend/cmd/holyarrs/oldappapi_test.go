package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/arrapi"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// oldAppFix is a signed-in app with a show and a movie Library folder,
// the lab show and movie to add, and the arr API turned on with a key
// per side. Its Job queue never runs, so every Job a call queues stays
// queued and can be checked.
type oldAppFix struct {
	t                     *testing.T
	srv                   *httptest.Server
	s                     *database.Store
	pc                    *parts.Control
	own                   *http.Client
	csrf                  string
	showDir, movieDir     string
	sonarrKey, radarrKey  string
	showFolder, movieFold int64
	base                  string // the URL base, "" for none
	art                   *library.ArtworkCache
	vault                 *secrets.Vault // keeps the keys of the Sources Prowlarr writes
}

func (f *oldAppFix) call(c *http.Client, csrf, method, path, body string, hdr ...string) (int, http.Header, string) {
	f.t.Helper()
	req, _ := http.NewRequest(method, f.srv.URL+f.base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(accounts.CSRFHeader, csrf)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

// session calls as the signed-in owner.
func (f *oldAppFix) session(method, path, body string) (int, string) {
	f.t.Helper()
	st, _, b := f.call(f.own, f.csrf, method, path, body)
	return st, b
}

// arr calls the arr API the way Seerr does: the key in the address.
func (f *oldAppFix) arr(key, method, path, body string) (int, http.Header, string) {
	f.t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return f.call(http.DefaultClient, "", method, path+sep+url.Values{"apikey": {key}}.Encode(), body)
}

// arrOK calls and wants the status; it returns the decoded answer.
func (f *oldAppFix) arrOK(key, method, path, body string, want int) any {
	f.t.Helper()
	st, hd, b := f.arr(key, method, path, body)
	if st != want {
		f.t.Fatalf("%s %s: %d, want %d: %s", method, path, st, want, b)
	}
	if v := hd.Get("X-Application-Version"); v != arrapi.SonarrVersion && v != arrapi.RadarrVersion {
		f.t.Fatalf("%s %s: version header %q", method, path, v)
	}
	var v any
	if err := json.Unmarshal([]byte(b), &v); err != nil {
		f.t.Fatalf("%s %s: %v: %s", method, path, err, b)
	}
	return v
}

// newOldAppFix serves the app, under base when one is given.
func newOldAppFix(t *testing.T, base ...string) *oldAppFix {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	q := jobs.New(s, jobs.Options{})
	pc := &parts.Control{Store: s, Reg: jobs.NewRegistry(), Jobs: q, Drain: time.Second, Logf: func(string, ...any) {}}
	if err := pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	tmdb := &arrShowSource{shows: map[string]metadata.Show{"tmdb:100": arrShow("tmdb", "100"), "tvdb:200": arrShow("tmdb", "100")}}
	// The artwork cache allows no address, so nothing is ever fetched.
	art := &library.ArtworkCache{Dir: t.TempDir()}
	lib := &library.Service{Store: s, Queue: q, Shows: metadata.ShowSources{"tmdb": tmdb, "tvdb": &arrShowSource{err: metadata.ErrNoTVDBKey}}, Artwork: art}
	vault, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	h := buildHandlers(services{Store: s, Queue: q, Library: lib, Metadata: metadata.Lab(), Version: "test",
		Auth: accounts.New(s, accounts.Options{}), Parts: pc, Secrets: vault}).Handler()
	f := &oldAppFix{t: t, s: s, pc: pc, showDir: t.TempDir(), movieDir: t.TempDir(), art: art, vault: vault}
	if len(base) > 0 {
		h, f.base = webserver.UnderURLBase(base[0], h), base[0]
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	f.srv = srv
	jar, _ := cookiejar.New(nil)
	f.own = &http.Client{Jar: jar}
	st, _, body := f.call(f.own, "", "POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	f.csrf = aiField(body, "csrf")
	for _, lf := range []struct {
		kind, path string
		id         *int64
	}{{"show", f.showDir, &f.showFolder}, {"movie", f.movieDir, &f.movieFold}} {
		st, body := f.session("POST", "/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":%q,"label":"x"}`, lf.path, lf.kind))
		if st != 201 {
			t.Fatalf("folder %d %s", st, body)
		}
		var v struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal([]byte(body), &v)
		*lf.id = v.ID
	}
	return f
}

// turnOn switches the arr API on and makes both keys.
func (f *oldAppFix) turnOn() {
	f.t.Helper()
	if st, body := f.session("PUT", "/api/v1/settings/old-app-api", `{"enabled":true}`); st != 200 || !strings.Contains(body, `"enabled":true`) {
		f.t.Fatalf("turn on: %d %s", st, body)
	}
	for _, side := range []string{"sonarr", "radarr"} {
		st, body := f.session("POST", "/api/v1/settings/old-app-api/keys/"+side, "")
		if st != 201 {
			f.t.Fatalf("key %s: %d %s", side, st, body)
		}
		k := aiField(body, "key")
		if len(k) != 32 || strings.Trim(k, "0123456789abcdef") != "" {
			f.t.Fatalf("key %q is not 32 lowercase hex characters", k)
		}
		if side == "sonarr" {
			f.sonarrKey = k
		} else {
			f.radarrKey = k
		}
	}
}

// fields checks a decoded object has every name in want and none in not.
func fields(t *testing.T, where string, v any, want []string, not ...string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: not an object: %v", where, v)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("%s: no %q in %v", where, k, m)
		}
	}
	for _, k := range not {
		if _, ok := m[k]; ok {
			t.Errorf("%s: %q should be left out: %v", where, k, m[k])
		}
	}
	return m
}

func arrList(t *testing.T, where string, v any, n int) []any {
	t.Helper()
	l, ok := v.([]any)
	if !ok || (n >= 0 && len(l) != n) {
		t.Fatalf("%s: want a list of %d: %v", where, n, v)
	}
	return l
}

func num(v any) int64 { f, _ := v.(float64); return int64(f) }

// route: GET /api/v1/settings/old-app-api
// route: PUT /api/v1/settings/old-app-api
// route: POST /api/v1/settings/old-app-api/keys/{side}
// route: DELETE /api/v1/settings/old-app-api/keys/{side}
func TestOldAppSwitchAndKeys(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	// Off by default: both prefixes answer 404 whatever is sent.
	if st, _, _ := f.arr("whatever", "GET", "/sonarr/api/v3/system/status", ""); st != 404 {
		t.Fatalf("off: %d", st)
	}
	if st, _, _ := f.call(http.DefaultClient, "", "POST", "/radarr/api/v3/movie", `{}`); st != 404 {
		t.Fatalf("off: %d", st)
	}
	st, body := f.session("GET", "/api/v1/settings/old-app-api", "")
	if st != 200 || !strings.Contains(body, `"enabled":false`) || !strings.Contains(body, `"delete_files":false`) || !strings.Contains(body, `"path":"/sonarr"`) {
		t.Fatalf("settings: %d %s", st, body)
	}
	f.turnOn()
	// A new key replaces the old one; the settings show only a hint.
	old := f.sonarrKey
	st, body = f.session("POST", "/api/v1/settings/old-app-api/keys/sonarr", "")
	if st != 201 {
		t.Fatalf("new key: %d %s", st, body)
	}
	f.sonarrKey = aiField(body, "key")
	if st, _, _ := f.arr(old, "GET", "/sonarr/api/v3/system/status", ""); st != 401 {
		t.Fatalf("replaced key: %d", st)
	}
	f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/system/status", "", 200)
	st, body = f.session("GET", "/api/v1/settings/old-app-api", "")
	if st != 200 || strings.Contains(body, f.sonarrKey) || !strings.Contains(body, "…"+f.sonarrKey[28:]) || !strings.Contains(body, `"last_used_at"`) {
		t.Fatalf("settings with keys: %d %s", st, body)
	}
	if st, _ := f.session("POST", "/api/v1/settings/old-app-api/keys/lidarr", ""); st != 404 {
		t.Fatalf("unknown side: %d", st)
	}
	// An arr key cannot be made as a HolyArrs API key.
	if st, body := f.session("POST", "/api/v1/auth/keys", `{"label":"x","scopes":["compat"]}`); st != 400 {
		t.Fatalf("compat scope on an API key: %d %s", st, body)
	}
	// An admin-only API key cannot make an arr key, which can change
	// the Library; a key holding write too can.
	for scopes, want := range map[string]int{`["admin"]`: 403, `["all"]`: 201} {
		st, hak := f.session("POST", "/api/v1/auth/keys", `{"label":"x","scopes":`+scopes+`}`)
		if st != 201 {
			t.Fatalf("hak key %s: %d %s", scopes, st, hak)
		}
		if st, _, b := f.call(http.DefaultClient, "", "POST", "/api/v1/settings/old-app-api/keys/sonarr", "", "X-Api-Key", aiField(hak, "key")); st != want {
			t.Fatalf("key made by a %s API key: %d %s, want %d", scopes, st, b, want)
		}
	}
	st, body = f.session("POST", "/api/v1/settings/old-app-api/keys/sonarr", "")
	if st != 201 {
		t.Fatalf("new key: %d %s", st, body)
	}
	f.sonarrKey = aiField(body, "key")
	// Removing a key: it stops working at once; a second remove is 404.
	if st, _ := f.session("DELETE", "/api/v1/settings/old-app-api/keys/radarr", ""); st != 204 {
		t.Fatalf("remove: %d", st)
	}
	if st, _, _ := f.arr(f.radarrKey, "GET", "/radarr/api/v3/system/status", ""); st != 401 {
		t.Fatalf("removed key: %d", st)
	}
	if st, _ := f.session("DELETE", "/api/v1/settings/old-app-api/keys/radarr", ""); st != 404 {
		t.Fatalf("remove again: %d", st)
	}
	// Turning it off answers 404 again, even with a good key.
	if st, _ := f.session("PUT", "/api/v1/settings/old-app-api", `{"enabled":false}`); st != 200 {
		t.Fatal(st)
	}
	if st, _, _ := f.arr(f.sonarrKey, "GET", "/sonarr/api/v3/system/status", ""); st != 404 {
		t.Fatalf("off again: %d", st)
	}
}

// The key: query first, then X-Api-Key, then Authorization (with or
// without Bearer); only the first place found counts. Wrong, missing,
// other-side and HolyArrs keys get a bare 401.
func TestOldAppKeyPlaces(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	const path = "/sonarr/api/v3/system/status"
	get := func(q string, hdr ...string) (int, string) {
		st, _, b := f.call(http.DefaultClient, "", "GET", path+q, "", hdr...)
		return st, b
	}
	k := f.sonarrKey
	for name, c := range map[string]struct {
		q   string
		hdr []string
	}{
		"query":          {"?apikey=" + k, nil},
		"query any case": {"?ApiKey=" + k, nil},
		"header":         {"", []string{"X-Api-Key", k}},
		"bearer":         {"", []string{"Authorization", "Bearer " + k}},
		"bare":           {"", []string{"Authorization", k}},
	} {
		if st, b := get(c.q, c.hdr...); st != 200 || !strings.Contains(b, `"urlBase":"/sonarr"`) {
			t.Errorf("%s: %d %s", name, st, b)
		}
	}
	st, hak := f.session("POST", "/api/v1/auth/keys", `{"label":"all","scopes":["all"]}`)
	if st != 201 {
		t.Fatalf("hak key: %d %s", st, hak)
	}
	for name, c := range map[string]struct {
		q   string
		hdr []string
	}{
		"none":                      {"", nil},
		"wrong":                     {"?apikey=" + strings.Repeat("0", 32), nil},
		"wrong query, right header": {"?apikey=nope", []string{"X-Api-Key", k}},
		"empty query, right header": {"?apikey=", []string{"X-Api-Key", k}},
		"other side":                {"?apikey=" + f.radarrKey, nil},
		"holyarrs key":              {"", []string{"X-Api-Key", aiField(hak, "key")}},
		"access_token":              {"?access_token=" + k, nil},
	} {
		st, _, b := f.call(http.DefaultClient, "", "GET", path+c.q, "", c.hdr...)
		if st != 401 || b != "" {
			t.Errorf("%s: %d %q, want a bare 401", name, st, b)
		}
	}
	// The arr key reaches nothing else.
	if st, _, _ := f.call(http.DefaultClient, "", "GET", "/api/v1/titles", "", "X-Api-Key", k); st != 401 {
		t.Errorf("arr key on /api/v1: %d", st)
	}
	// Writes from a Docker neighbour (another host name, no Origin) pass:
	// a key is no browser credential.
	if st, _, b := f.call(http.DefaultClient, "", "POST", "/sonarr/api/v3/tag?apikey="+k, `{"label":"x"}`, "Host", "holyarrs:18730"); st != 201 {
		t.Errorf("write by host name: %d %s", st, b)
	}
}

// A key made by someone who may no longer manage the Library is refused
// (403), reads and writes alike; making a key needs that permission.
func TestOldAppMemberKeyRefused(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	st, body := f.session("POST", "/api/v1/users/invites", `{}`)
	if st != 201 {
		t.Fatalf("invite %d %s", st, body)
	}
	jar, _ := cookiejar.New(nil)
	mem := &http.Client{Jar: jar}
	st, _, body = f.call(mem, "", "POST", "/api/v1/auth/invites/"+strings.TrimPrefix(aiField(body, "path"), "/invite/"), `{"name":"ana","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("join %d %s", st, body)
	}
	memCSRF := aiField(body, "csrf")
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","settings"]' WHERE name = 'ana'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if st, _, b := f.call(mem, memCSRF, "POST", "/api/v1/settings/old-app-api/keys/sonarr", ""); st != 403 {
		t.Fatalf("member makes a key: %d %s", st, b)
	}
	// The owner's key handed to the member's account.
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE compat_keys SET owner_user_id = (SELECT id FROM users WHERE name = 'ana')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/sonarr/api/v3/series", ""},
		{"POST", "/sonarr/api/v3/series", `{"tvdbId":200}`},
		{"PUT", "/sonarr/api/v3/series", `{"id":1}`},
		{"DELETE", "/sonarr/api/v3/series/1", ""},
		{"POST", "/sonarr/api/v3/command", `{"name":"RefreshMonitoredDownloads"}`},
	} {
		if st, _, b := f.arr(f.sonarrKey, c.method, c.path, c.body); st != 403 {
			t.Errorf("member key %s %s: %d %s", c.method, c.path, st, b)
		}
	}
}

// A side whose part is off answers the part gate's 409; the other works.
func TestOldAppPartOff(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	if err := f.pc.Set(context.Background(), parts.Shows, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if st, _, b := f.arr(f.sonarrKey, "GET", "/sonarr/api/v3/series", ""); st != 409 || !strings.Contains(b, "this part is off") {
		t.Fatalf("shows off: %d %s", st, b)
	}
	f.arrOK(f.radarrKey, "GET", "/radarr/api/v3/movie", "", 200)
}

// The calls of the server test, field by field, and paths in any case.
func TestOldAppServerTestFields(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	for _, side := range []struct{ name, key, version, folder string }{
		{"sonarr", f.sonarrKey, arrapi.SonarrVersion, f.showDir}, {"radarr", f.radarrKey, arrapi.RadarrVersion, f.movieDir}} {
		p := "/" + side.name + "/api/v3/"
		st, hd, b := f.arr(side.key, "GET", p+"system/status", "")
		if st != 200 || hd.Get("X-Application-Version") != side.version {
			t.Fatalf("status: %d %v %s", st, hd, b)
		}
		var status any
		_ = json.Unmarshal([]byte(b), &status)
		m := fields(t, side.name+" status", status, []string{"appName", "version", "urlBase", "isDocker"})
		// The app name is the old app's, which Pulsarr checks; HolyArrs is the instance name.
		if m["appName"] != map[string]string{"sonarr": "Sonarr", "radarr": "Radarr"}[side.name] || m["instanceName"] != "HolyArrs" || m["version"] != side.version || m["urlBase"] != "/"+side.name {
			t.Errorf("status: %v", m)
		}
		// Seerr calls the quality profiles with a capital P.
		profiles := arrList(t, "qualityProfile", f.arrOK(side.key, "GET", p+"qualityProfile", "", 200), -1)
		if len(profiles) == 0 {
			t.Fatal("no quality profiles")
		}
		for _, pr := range profiles {
			fields(t, "profile", pr, []string{"id", "name"})
		}
		roots := arrList(t, "rootfolder", f.arrOK(side.key, "GET", p+"rootfolder", "", 200), 1)
		if r := fields(t, "rootfolder", roots[0], []string{"id", "path", "accessible", "unmappedFolders"}); r["path"] != side.folder {
			t.Errorf("rootfolder: %v", r)
		}
		tg := fields(t, "tag", f.arrOK(side.key, "POST", p+"tag", `{"label":"1 - `+side.name+`"}`, 201), []string{"id", "label"})
		again := fields(t, "tag again", f.arrOK(side.key, "POST", p+"tag", `{"label":"1 - `+strings.ToUpper(side.name)+`"}`, 201), []string{"id"})
		if again["id"] != tg["id"] {
			t.Errorf("the same label made a second tag: %v %v", tg, again)
		}
		found := false
		for _, x := range arrList(t, "tag", f.arrOK(side.key, "GET", p+"TAG", "", 200), -1) {
			found = found || fields(t, "tag", x, []string{"id", "label"})["id"] == tg["id"]
		}
		if !found {
			t.Errorf("tag %v not listed", tg)
		}
		f.arrOK(side.key, "PUT", p+"tag/"+arrItoa(num(tg["id"])), `{"id":1,"label":"1-`+side.name+`"}`, 202)
		// The id may come in the body alone.
		if r := f.arrOK(side.key, "PUT", p+"tag", `{"id":`+arrItoa(num(tg["id"]))+`,"label":"1 - `+side.name+`"}`, 202).(map[string]any); r["label"] != "1 - "+side.name {
			t.Errorf("put tag: %v", r)
		}
		if st, _, _ := f.arr(side.key, "PUT", p+"tag/999", `{"label":"x"}`); st != 404 {
			t.Errorf("unknown tag: %d", st)
		}
		if st, _, _ := f.arr(side.key, "DELETE", p+"tag/999", ""); st != 404 {
			t.Errorf("unknown tag delete: %d", st)
		}
		if st, _, _ := f.arr(side.key, "GET", "/"+side.name+"/api/system/status", ""); st != 404 {
			t.Errorf("no /api/v3: %d", st)
		}
	}
	// Overseerr always asks for language profiles.
	lp := arrList(t, "languageprofile", f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/languageprofile", "", 200), 1)
	fields(t, "languageprofile", lp[0], []string{"id", "name"})
	if st, _, _ := f.arr(f.radarrKey, "GET", "/radarr/api/v3/languageprofile", ""); st != 404 {
		t.Errorf("radarr languageprofile: %d", st)
	}
}

// slotsOf lists a title's main-Version Slots by season and episode
// (0, 0 for a movie).
func (f *oldAppFix) slotsOf(titleID int64) map[[2]int]mediafiles.Slot {
	f.t.Helper()
	out := map[[2]int]mediafiles.Slot{}
	rows, err := f.s.Read().Query(`SELECT s.id, s.version_id, COALESCE(json_extract(i.numbers, '$.season'), 0), COALESCE(json_extract(i.numbers, '$.episode'), 0)
		FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id AND v.is_main = 1 WHERE i.title_id = ?`, titleID)
	if err != nil {
		f.t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var sl mediafiles.Slot
		var se, ep int
		if err := rows.Scan(&sl.ID, &sl.VersionID, &se, &ep); err != nil {
			f.t.Fatal(err)
		}
		out[[2]int{se, ep}] = sl
	}
	return out
}

// putFile puts a file in the Library for a Slot, as an import would.
func (f *oldAppFix) putFile(sl mediafiles.Slot, rel string, res int) {
	f.t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		fl := mediafiles.File{SlotIDs: []int64{sl.ID}, VersionID: sl.VersionID, RelativePath: rel, Size: 1000, AddedAt: time.Now().UTC(),
			Quality: mediafiles.Quality{Resolution: res, Source: "web_dl"}, Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported",
			PlacedBy: "hardlink", Role: "main", State: "present"}
		id, err := database.Insert(tx, "files", &fl, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", id, sl.ID)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *oldAppFix) jobs(kind string) []domain.Job {
	f.t.Helper()
	js, err := database.Select[domain.Job](context.Background(), f.s.Read(), "jobs", "kind = ? ORDER BY id", kind)
	if err != nil {
		f.t.Fatal(err)
	}
	return js
}

// TestOldAppSeerrFlow makes Seerr's and Overseerr's calls in their order:
// a show asked for with season 1, then season 2 on a second request, a
// movie, the queue poll, the daily scans seeing files arrive, and Seerr's
// remove. Bodies follow the Seerr manual (C4, C5).
func TestOldAppSeerrFlow(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	son, rad := f.sonarrKey, f.radarrKey
	s := "/sonarr/api/v3/"
	profile := num(arrList(t, "profiles", f.arrOK(son, "GET", s+"qualityProfile", "", 200), -1)[0].(map[string]any)["id"])
	tagID := num(f.arrOK(son, "POST", s+"tag", `{"label":"1 - luis"}`, 201).(map[string]any)["id"])

	// A show not in the Library: the lookup has no id, all its seasons and
	// zero counts.
	look := arrList(t, "lookup", f.arrOK(son, "GET", s+"series/lookup?term=tvdb:200", "", 200), 1)[0]
	lm := fields(t, "series lookup", look, []string{"title", "tvdbId", "titleSlug", "seasons", "monitored", "qualityProfileId", "tags", "added", "statistics", "images"}, "id", "path")
	if num(lm["tvdbId"]) != 200 || len(lm["seasons"].([]any)) != 3 || lm["added"] != "0001-01-01T00:00:00Z" {
		t.Fatalf("lookup: %v", lm)
	}
	arrList(t, "free text lookup", f.arrOK(son, "GET", s+"series/lookup?term=lab", "", 200), 1)
	arrList(t, "bad tvdb lookup", f.arrOK(son, "GET", s+"series/lookup?term=tvdb:abc", "", 200), 0)
	arrList(t, "unknown tvdb lookup", f.arrOK(son, "GET", s+"series/lookup?term=tvdb:999", "", 200), 0)
	arrList(t, "series by tvdb, none", f.arrOK(son, "GET", s+"series?tvdbId=200", "", 200), 0)

	// Seerr adds it with season 1 asked for.
	add := fmt.Sprintf(`{"tvdbId":200,"title":"Lab Show","qualityProfileId":%d,"languageProfileId":1,"seasons":[{"seasonNumber":0,"monitored":false},
		{"seasonNumber":1,"monitored":true},{"seasonNumber":2,"monitored":false}],"tags":[%d],"seasonFolder":true,"monitored":true,
		"monitorNewItems":"all","rootFolderPath":%q,"seriesType":"standard","addOptions":{"ignoreEpisodesWithFiles":true,"searchForMissingEpisodes":true}}`,
		profile, tagID, f.showDir+"/")
	st, hd, b := f.arr(son, "POST", s+"series", add)
	if st != 201 {
		t.Fatalf("add show: %d %s", st, b)
	}
	var added map[string]any
	_ = json.Unmarshal([]byte(b), &added)
	fields(t, "added series", added, []string{"id", "titleSlug", "monitored", "seasons", "statistics", "path", "rootFolderPath", "qualityProfileId"})
	showID := num(added["id"])
	if !strings.HasSuffix(hd.Get("Location"), "/sonarr/api/v3/series/"+arrItoa(showID)) || added["titleSlug"] != arrItoa(showID) || num(added["qualityProfileId"]) != profile {
		t.Fatalf("added: %v %v", hd.Get("Location"), added)
	}
	seasonOn := func(m map[string]any) map[int64]bool {
		out := map[int64]bool{}
		for _, x := range m["seasons"].([]any) {
			se := x.(map[string]any)
			out[num(se["seasonNumber"])] = se["monitored"].(bool)
		}
		return out
	}
	if on := seasonOn(added); on[0] || !on[1] || on[2] {
		t.Fatalf("seasons after add: %v", on)
	}
	var keep1, keep2 int
	_ = f.s.Read().QueryRow(`SELECT count(*) FROM items i JOIN parts p ON p.id = i.part_id WHERE p.title_id = ? AND p.number = 1 AND i.keep_up_to_date = 1`, showID).Scan(&keep1)
	_ = f.s.Read().QueryRow(`SELECT count(*) FROM items i JOIN parts p ON p.id = i.part_id WHERE p.title_id = ? AND p.number = 2 AND i.keep_up_to_date = 1`, showID).Scan(&keep2)
	if keep1 != 2 || keep2 != 0 {
		t.Fatalf("episodes kept: season 1 %d, season 2 %d", keep1, keep2)
	}
	// The search waits for the folder scan (it is the scan's next step).
	if js := f.jobs(library.KindScanTitle); len(js) != 1 || !strings.Contains(fmt.Sprint(js[0].Params["then"]), "search_show") {
		t.Fatalf("scan job: %+v", js)
	}
	// Adding it again is the old apps' 400 list on TvdbId.
	st, _, b = f.arr(son, "POST", s+"series", add)
	var dup []map[string]any
	if st != 400 || json.Unmarshal([]byte(b), &dup) != nil || len(dup) != 1 || dup[0]["propertyName"] != "TvdbId" || dup[0]["severity"] != "error" {
		t.Fatalf("duplicate add: %d %s", st, b)
	}
	if st, _, b := f.arr(son, "POST", s+"series", `{"tvdbId":0}`); st != 400 || !strings.Contains(b, "TvdbId") {
		t.Fatalf("no tvdb id: %d %s", st, b)
	}
	if st, _, b := f.arr(son, "POST", s+"series", `{"tvdbId":201,"rootFolderPath":"/nowhere"}`); st != 400 || !strings.Contains(b, "RootFolderPath") {
		t.Fatalf("unknown folder: %d %s", st, b)
	}

	// A second request, for season 2: the lookup now gives the Library's
	// show with its id; Seerr sends it back with season 2 on (no id in the
	// path), lists the episodes and switches on any still off.
	look = arrList(t, "lookup again", f.arrOK(son, "GET", s+"series/lookup?term=tvdb:200", "", 200), 1)[0]
	lm = fields(t, "lookup in library", look, []string{"id", "monitored", "seasons", "qualityProfileId", "path", "tags"})
	if num(lm["id"]) != showID {
		t.Fatalf("lookup id %v, want %d", lm["id"], showID)
	}
	for _, x := range lm["seasons"].([]any) {
		if se := x.(map[string]any); num(se["seasonNumber"]) == 2 {
			se["monitored"] = true
		}
	}
	lm["tags"] = []int64{tagID}
	put, _ := json.Marshal(lm)
	putAns := fields(t, "put series", f.arrOK(son, "PUT", s+"series", string(put), 202), []string{"id", "monitored"})
	if on := seasonOn(putAns); !on[1] || !on[2] {
		t.Fatalf("seasons after put: %v", on)
	}
	f.arrOK(son, "PUT", s+"series/"+arrItoa(showID), string(put), 202)
	if st, _, _ := f.arr(son, "GET", s+"episode", ""); st != 400 {
		t.Fatalf("episodes without seriesId: %d", st)
	}
	eps := arrList(t, "episodes", f.arrOK(son, "GET", s+"episode?seriesid="+arrItoa(showID), "", 200), 5)
	var special int64
	for _, x := range eps {
		e := fields(t, "episode", x, []string{"id", "seriesId", "seasonNumber", "episodeNumber", "monitored", "hasFile", "episodeFileId"})
		if num(e["seasonNumber"]) == 2 && !e["monitored"].(bool) {
			t.Errorf("season 2 episode still off: %v", e)
		}
		if num(e["seasonNumber"]) == 0 {
			special = num(e["id"])
		}
	}
	arrList(t, "season 1 episodes", f.arrOK(son, "GET", s+"episode?seriesId="+arrItoa(showID)+"&seasonNumber=1", "", 200), 2)
	mon := arrList(t, "monitor", f.arrOK(son, "PUT", s+"episode/monitor", fmt.Sprintf(`{"episodeIds":[%d],"monitored":true}`, special), 202), 1)
	if !mon[0].(map[string]any)["monitored"].(bool) {
		t.Fatalf("monitor: %v", mon)
	}
	if st, _, _ := f.arr(son, "PUT", s+"episode/monitor", `{"episodeIds":[999999],"monitored":true}`); st != 404 {
		t.Fatalf("monitor unknown episode: %d", st)
	}
	// Seerr's search, then Overseerr's: hand searches as Jobs.
	cmd := fields(t, "command", f.arrOK(son, "POST", s+"command", fmt.Sprintf(`{"name":"MissingEpisodeSearch","seriesId":%d}`, showID), 201),
		[]string{"id", "name", "commandName", "status", "queued", "body", "trigger", "priority", "result"})
	if num(cmd["id"]) == 2147483647 || cmd["status"] != "queued" || cmd["commandName"] != "Missing Episode Search" {
		t.Fatalf("command: %v", cmd)
	}
	poll := fields(t, "poll", f.arrOK(son, "GET", s+"command/"+arrItoa(num(cmd["id"])), "", 200), []string{"id", "status"})
	if poll["status"] != "queued" {
		t.Fatalf("poll: %v", poll)
	}
	f.arrOK(son, "POST", s+"command", fmt.Sprintf(`{"name":"seriessearch","seriesId":%d}`, showID), 201)
	if js := f.jobs("search_season"); len(js) == 0 || js[0].Params["by_hand"] != true {
		t.Fatalf("season searches: %+v", js)
	}
	if st, _, b := f.arr(son, "POST", s+"command", `{"name":"Backup"}`); st != 500 || !strings.Contains(b, "Unknown command") {
		t.Fatalf("unknown command: %d %s", st, b)
	}
	if st, _, b := f.arr(son, "POST", s+"command", `{}`); st != 400 || !strings.Contains(b, "propertyName") {
		t.Fatalf("no name: %d %s", st, b)
	}

	// A movie, through the Radarr side.
	r := "/radarr/api/v3/"
	ml := fields(t, "movie lookup", arrList(t, "movie lookup", f.arrOK(rad, "GET", r+"movie/lookup?term=tmdb:693134", "", 200), 1)[0],
		[]string{"title", "tmdbId", "titleSlug", "monitored", "minimumAvailability", "tags", "year"}, "id", "hasFile", "statistics")
	if ml["monitored"] != false || ml["minimumAvailability"] != "tba" || ml["titleSlug"] != "693134" {
		t.Fatalf("movie lookup: %v", ml)
	}
	arrList(t, "movie by name", f.arrOK(rad, "GET", r+"movie/lookup?term=dune", "", 200), 1)
	addMovie := fmt.Sprintf(`{"title":"Dune: Part Two","qualityProfileId":%d,"profileId":%d,"titleSlug":"693134","minimumAvailability":"released",
		"tmdbId":693134,"year":2024,"rootFolderPath":%q,"monitored":true,"tags":[],"addOptions":{"searchForMovie":true}}`, profile, profile, f.movieDir)
	st, hd, b = f.arr(rad, "POST", r+"movie", addMovie)
	if st != 201 {
		t.Fatalf("add movie: %d %s", st, b)
	}
	var mv map[string]any
	_ = json.Unmarshal([]byte(b), &mv)
	fields(t, "added movie", mv, []string{"id", "hasFile", "monitored", "statistics", "sizeOnDisk", "movieFileId", "isAvailable"})
	movieID := num(mv["id"])
	if mv["hasFile"] != false || !strings.HasSuffix(hd.Get("Location"), "/radarr/api/v3/movie/"+arrItoa(movieID)) {
		t.Fatalf("added movie: %v", mv)
	}
	st, _, b = f.arr(rad, "POST", r+"movie", addMovie)
	if st != 400 || !strings.Contains(b, `"errorCode":"MovieExistsValidator"`) || !strings.Contains(b, `"propertyName":"TmdbId"`) {
		t.Fatalf("duplicate movie: %d %s", st, b)
	}
	// In the Library, a lookup gives the id but no hasFile, like Radarr.
	ml = fields(t, "movie lookup in library", arrList(t, "movie lookup", f.arrOK(rad, "GET", r+"movie/lookup?term=tmdb:693134", "", 200), 1)[0],
		[]string{"id", "monitored", "qualityProfileId", "tags", "path"}, "hasFile", "statistics")
	ml["monitored"] = false
	put, _ = json.Marshal(ml)
	if a := f.arrOK(rad, "PUT", r+"movie", string(put), 202).(map[string]any); a["monitored"] != false {
		t.Fatalf("put movie off: %v", a)
	}
	ml["monitored"], ml["minimumAvailability"] = true, "inCinemas"
	put, _ = json.Marshal(ml)
	if a := f.arrOK(rad, "PUT", r+"movie/"+arrItoa(movieID), string(put), 202).(map[string]any); a["monitored"] != true || a["minimumAvailability"] != "inCinemas" {
		t.Fatalf("put movie on: %v", a)
	}
	f.arrOK(rad, "POST", r+"command", fmt.Sprintf(`{"name":"MoviesSearch","movieIds":[%d]}`, movieID), 201)
	if js := f.jobs("search_slot"); len(js) != 1 || js[0].Params["by_hand"] != true {
		t.Fatalf("movie search: %+v", js)
	}

	// The queue poll every minute: RefreshMonitoredDownloads writes
	// nothing, and polling it says completed.
	slots := f.slotsOf(showID)
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		dr := domain.Downloader{Adapter: "qbittorrent", Name: "Lab qBit", Protocol: "torrent", URL: "http://lab.invalid", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", &dr, domain.System); err != nil {
			return err
		}
		eta := int64(125)
		d := domain.Download{DownloaderID: dr.ID, ExternalID: "ABCDEF", SlotIDs: []int64{slots[[2]int{1, 1}].ID, slots[[2]int{1, 2}].ID},
			State: "downloading", Progress: 0.5, ETA: &eta, SavePath: "/downloads/lab", FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		_, err := database.Insert(tx, "downloads", &d, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := f.s.WriteCount()
	refresh := fields(t, "refresh", f.arrOK(son, "POST", s+"command", `{"name":"RefreshMonitoredDownloads"}`, 201), []string{"id", "status"})
	if num(refresh["id"]) != 2147483647 || refresh["status"] != "completed" {
		t.Fatalf("refresh: %v", refresh)
	}
	if p := fields(t, "refresh poll", f.arrOK(son, "GET", s+"command/2147483647", "", 200), []string{"status"}); p["status"] != "completed" {
		t.Fatalf("refresh poll: %v", p)
	}
	q := fields(t, "queue", f.arrOK(son, "GET", s+"queue?includeEpisode=true", "", 200), []string{"page", "pageSize", "sortKey", "sortDirection", "totalRecords", "records"})
	if f.s.WriteCount() != before {
		t.Fatalf("the queue poll wrote %d times", f.s.WriteCount()-before)
	}
	if num(q["totalRecords"]) != 2 || num(q["pageSize"]) < 1000 || q["sortDirection"] != "descending" {
		t.Fatalf("queue: %v", q)
	}
	for _, x := range q["records"].([]any) {
		row := fields(t, "queue row", x, []string{"id", "seriesId", "episodeId", "episode", "size", "sizeleft", "timeleft", "estimatedCompletionTime",
			"status", "title", "downloadId", "protocol", "trackedDownloadStatus", "trackedDownloadState", "statusMessages", "downloadClient", "outputPath"}, "movieId")
		ep := fields(t, "queue episode", row["episode"], []string{"seasonNumber", "episodeNumber"})
		if num(row["seriesId"]) != showID || row["downloadId"] != "ABCDEF" || row["status"] != "downloading" || row["timeleft"] != "00:02:05" ||
			row["protocol"] != "torrent" || row["trackedDownloadState"] != "downloading" || num(ep["seasonNumber"]) != 1 {
			t.Errorf("queue row: %v", row)
		}
	}
	if p := f.arrOK(son, "GET", s+"queue?pageSize=1&page=9&sortKey=nope", "", 200).(map[string]any); num(p["page"]) != 2 || len(p["records"].([]any)) != 1 || p["sortKey"] != "timeleft" {
		t.Fatalf("paged queue: %v", p)
	}
	arrList(t, "radarr queue", f.arrOK(rad, "GET", r+"queue", "", 200).(map[string]any)["records"], 0)

	// Seerr's daily scans: season 1 becomes available once every episode
	// in it has a file (files reach the total), the movie once it has one.
	seasonCounts := func() map[int64]map[string]any {
		all := arrList(t, "series", f.arrOK(son, "GET", s+"series", "", 200), 1)
		sm := fields(t, "series", all[0], []string{"id", "tvdbId", "titleSlug", "title", "seasons", "statistics", "monitored"})
		fields(t, "series counts", sm["statistics"], []string{"seasonCount", "episodeFileCount", "episodeCount", "totalEpisodeCount", "sizeOnDisk", "percentOfEpisodes"})
		out := map[int64]map[string]any{}
		for _, x := range sm["seasons"].([]any) {
			se := x.(map[string]any)
			out[num(se["seasonNumber"])] = fields(t, "season counts", se["statistics"], []string{"episodeFileCount", "totalEpisodeCount", "episodeCount", "percentOfEpisodes"})
		}
		return out
	}
	if c := seasonCounts()[1]; num(c["episodeFileCount"]) != 0 || num(c["totalEpisodeCount"]) != 2 || num(c["episodeCount"]) != 2 {
		t.Fatalf("season 1 before files: %v", c)
	}
	f.putFile(slots[[2]int{1, 1}], "Lab Show/S01E01.mkv", 1080)
	f.putFile(slots[[2]int{1, 2}], "Lab Show/S01E02.mkv", 1080)
	counts := seasonCounts()
	if c := counts[1]; num(c["episodeFileCount"]) != 2 || num(c["totalEpisodeCount"]) != 2 || c["percentOfEpisodes"] != 100.0 {
		t.Fatalf("season 1 with files: %v", c)
	}
	if c := counts[2]; num(c["episodeFileCount"]) != 0 || num(c["totalEpisodeCount"]) != 2 {
		t.Fatalf("season 2: %v", c)
	}
	one := fields(t, "one series", f.arrOK(son, "GET", s+"series/"+arrItoa(showID), "", 200), []string{"statistics"})
	if num(one["statistics"].(map[string]any)["episodeFileCount"]) != 2 {
		t.Fatalf("series counts: %v", one["statistics"])
	}
	if e := arrList(t, "episodes", f.arrOK(son, "GET", s+"episode?seriesId="+arrItoa(showID)+"&seasonNumber=1", "", 200), 2)[0].(map[string]any); e["hasFile"] != true || num(e["episodeFileId"]) == 0 {
		t.Fatalf("episode with file: %v", e)
	}
	f.putFile(f.slotsOf(movieID)[[2]int{0, 0}], "Dune Part Two (2024)/Dune.mkv", 2160)
	ms := arrList(t, "movies", f.arrOK(rad, "GET", r+"movie", "", 200), 1)
	m := fields(t, "movie", ms[0], []string{"id", "tmdbId", "titleSlug", "title", "hasFile", "monitored", "movieFile", "statistics"})
	mf := fields(t, "movie file", m["movieFile"], []string{"id", "path", "size", "quality", "mediaInfo"})
	if m["hasFile"] != true || mf["mediaInfo"].(map[string]any)["resolution"] != "3840x2160" || num(m["statistics"].(map[string]any)["movieFileCount"]) != 1 {
		t.Fatalf("movie with file: %v", m)
	}
	// Library pictures point at this side's covers; remoteUrl keeps the
	// original address of the wide picture.
	const wide = "https://image.tmdb.org/t/p/w1280/lab-wide.jpg"
	f.exec("UPDATE titles SET backdrop = ? WHERE id = ?", wide, movieID)
	ims := fields(t, "movie", f.arrOK(rad, "GET", r+"movie/"+arrItoa(movieID), "", 200), []string{"images"})["images"].([]any)
	if len(ims) == 0 {
		t.Fatal("no images")
	}
	for _, x := range ims {
		im := fields(t, "image", x, []string{"coverType", "url", "remoteUrl"})
		if !strings.HasPrefix(im["url"].(string), "/radarr/MediaCover/"+arrItoa(movieID)+"/") || (im["coverType"] == "fanart" && im["remoteUrl"] != wide) {
			t.Errorf("image: %v", im)
		}
	}
	arrList(t, "movie by tmdb", f.arrOK(rad, "GET", r+"movie?tmdbId=693134", "", 200), 1)
	arrList(t, "movie by other tmdb", f.arrOK(rad, "GET", r+"movie?tmdbId=1", "", 200), 0)

	// Seerr's remove: the title goes; with the delete-files switch off the
	// files stay. A gone or wrong-kind id answers 404 everywhere.
	if st, _, b := f.arr(son, "DELETE", s+"series/"+arrItoa(showID)+"?deleteFiles=true&addImportExclusion=false", ""); st != 200 {
		t.Fatalf("delete show: %d %s", st, b)
	}
	var present int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'present' AND relative_path LIKE 'Lab Show/%'").Scan(&present)
	if present != 2 {
		t.Fatalf("files kept: %d, want 2", present)
	}
	for _, c := range []struct{ key, method, path, body string }{
		{son, "DELETE", s + "series/" + arrItoa(showID), ""},
		{son, "GET", s + "series/" + arrItoa(showID), ""},
		{son, "PUT", s + "series", fmt.Sprintf(`{"id":%d,"monitored":true}`, showID)},
		{son, "GET", s + "episode?seriesId=" + arrItoa(showID), ""},
		{son, "GET", s + "series/" + arrItoa(movieID), ""},
		{rad, "GET", r + "movie/" + arrItoa(showID), ""},
		{rad, "DELETE", r + "movie/" + arrItoa(showID), ""},
	} {
		if st, _, b := f.arr(c.key, c.method, c.path, c.body); st != 404 {
			t.Errorf("%s %s: %d %s, want 404", c.method, c.path, st, b)
		}
	}
	if l := arrList(t, "lookup after delete", f.arrOK(son, "GET", s+"series/lookup?term=tvdb:200", "", 200), 1)[0].(map[string]any); l["id"] != nil {
		t.Fatalf("lookup after delete still has an id: %v", l)
	}
	arrList(t, "series after delete", f.arrOK(son, "GET", s+"series", "", 200), 0)
	f.arrOK(rad, "DELETE", r+"movie/"+arrItoa(movieID)+"?deleteFiles=false&addImportExclusion=true", "", 200)
	if st, _, _ := f.arr(rad, "GET", r+"movie/"+arrItoa(movieID), ""); st != 404 {
		t.Fatalf("movie after delete: %d", st)
	}
	var never int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM never_add WHERE kind = 'movie'").Scan(&never)
	if never != 1 {
		t.Fatalf("import exclusion: never-add rows %d", never)
	}
}

// Under a URL base the status answer's urlBase, which Seerr keeps as its
// base path, and the Location of an add both carry it once.
// Seerr's "open in" link for admins lands on the HolyArrs title page.
func TestOldAppOpenLink(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t, "/holyarrs")
	f.turnOn()
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for path, want := range map[string]int{"/sonarr/series/12": 302, "/radarr/movie/7": 302, "/sonarr/series/arcane": 401, "/radarr/api/v3/movie/7": 401} {
		st, hd, _ := f.call(noFollow, "", "GET", path, "")
		if st != want {
			t.Fatalf("%s: %d, want %d", path, st, want)
		}
		if loc := hd.Get("Location"); want == 302 && loc != "/holyarrs/library/12" && loc != "/holyarrs/title/movie/7" {
			t.Fatalf("%s: Location %q", path, hd.Get("Location"))
		}
	}
}

func TestOldAppUnderURLBase(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t, "/holyarrs")
	f.turnOn()
	if m := f.arrOK(f.radarrKey, "GET", "/radarr/api/v3/system/status", "", 200).(map[string]any); m["urlBase"] != "/holyarrs/radarr" {
		t.Fatalf("urlBase: %v", m["urlBase"])
	}
	st, hd, b := f.arr(f.radarrKey, "POST", "/radarr/api/v3/movie", fmt.Sprintf(`{"tmdbId":693134,"rootFolderPath":%q,"monitored":true}`, f.movieDir))
	if st != 201 || !strings.HasPrefix(hd.Get("Location"), "/holyarrs/radarr/api/v3/movie/") {
		t.Fatalf("add under a base: %d %q %s", st, hd.Get("Location"), b)
	}
	if st, hd, _ := f.arr(f.radarrKey, "POST", "/radarr/api/v3/command", `{"name":"RefreshMonitoredDownloads"}`); st != 201 || hd.Get("Location") != "/holyarrs/radarr/api/v3/command/2147483647" {
		t.Fatalf("command under a base: %d %q", st, hd.Get("Location"))
	}
}

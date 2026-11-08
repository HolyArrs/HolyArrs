package api

// Helpers kept for the api tests that still use the old-app, migrate and AI
// fixtures; the tests themselves moved to cmd/holyarrs.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/arrapi"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parts"
	"image"
	"image/color"
	_ "image/jpeg" // the smaller poster copy is a JPEG
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// keyCall sends one call with an API key in its header.
func keyCall(t *testing.T, base, key, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, base+path, strings.NewReader(body))
	req.Header.Set(accounts.APIKeyHeader, key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// aiFix is a signed-in app with an owner, a member and their keys.
type aiFix struct {
	*ctlFix
	t                      *testing.T
	own, mem               *http.Client
	csrf, memCSRF          string
	ownKey, memKey, allKey string
}

func (f *aiFix) call(c *http.Client, csrf, key, method, path, body string, hdr ...string) (int, string) {
	f.t.Helper()
	req, _ := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set(accounts.CSRFHeader, csrf)
	}
	if key != "" {
		req.Header.Set(accounts.APIKeyHeader, key)
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
	return resp.StatusCode, string(b)
}

func aiField(body, name string) string {
	var v map[string]any
	_ = json.Unmarshal([]byte(body), &v)
	s, _ := v[name].(string)
	return s
}

func newAIFix(t *testing.T) *aiFix {
	t.Helper()
	f := &aiFix{ctlFix: newCtl(t, true), t: t}
	client := func() *http.Client { jar, _ := cookiejar.New(nil); return &http.Client{Jar: jar} }
	f.own, f.mem = client(), client()
	st, body := f.call(f.own, "", "", "POST", "/api/v1/auth/setup", `{"name":"luis","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("setup %d %s", st, body)
	}
	f.csrf = aiField(body, "csrf")
	st, body = f.call(f.own, f.csrf, "", "POST", "/api/v1/users/invites", `{}`)
	if st != 201 {
		t.Fatalf("invite %d %s", st, body)
	}
	st, body = f.call(f.mem, "", "", "POST", "/api/v1/auth/invites/"+strings.TrimPrefix(aiField(body, "path"), "/invite/"), `{"name":"ana","password":"a long test passphrase"}`)
	if st != 201 {
		t.Fatalf("join %d %s", st, body)
	}
	f.memCSRF = aiField(body, "csrf")
	// Making keys needs the settings permission.
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE users SET perms = '["ask","settings"]' WHERE name = 'ana'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	mk := func(c *http.Client, csrf, scopes string) string {
		st, body := f.call(c, csrf, "", "POST", "/api/v1/auth/keys", `{"label":"claude","scopes":`+scopes+`}`)
		if st != 201 {
			t.Fatalf("key %s: %d %s", scopes, st, body)
		}
		return aiField(body, "key")
	}
	f.ownKey = mk(f.own, f.csrf, `["mcp"]`)
	f.memKey = mk(f.mem, f.memCSRF, `["mcp"]`)
	f.allKey = mk(f.own, f.csrf, `["all"]`)
	return f
}

func (f *aiFix) turnOn(on bool) {
	f.t.Helper()
	b, _ := json.Marshal(map[string]bool{"enabled": on})
	if st, body := f.call(f.own, f.csrf, "", "PUT", "/api/v1/settings/ai", string(b)); st != 200 {
		f.t.Fatalf("turn on: %d %s", st, body)
	}
}

// rpc sends one JSON-RPC request with key and returns the status and answer.
func (f *aiFix) rpc(key, method, params string) (int, map[string]any) {
	f.t.Helper()
	if params == "" {
		params = "{}"
	}
	st, body := f.call(&http.Client{}, "", key, "POST", "/api/v1/mcp", `{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":`+params+`}`)
	var v map[string]any
	_ = json.Unmarshal([]byte(body), &v)
	return st, v
}

// tool calls a tool and returns its structured answer, or its error text.
func (f *aiFix) tool(key, name, args string) (map[string]any, string) {
	f.t.Helper()
	st, v := f.rpc(key, "tools/call", `{"name":"`+name+`","arguments":`+args+`}`)
	if st != 200 {
		f.t.Fatalf("%s: %d %v", name, st, v)
	}
	res, _ := v["result"].(map[string]any)
	if res["isError"] == true {
		c, _ := res["content"].([]any)
		first, _ := c[0].(map[string]any)
		s, _ := first["text"].(string)
		return nil, s
	}
	m, _ := res["structuredContent"].(map[string]any)
	return m, ""
}

func (f *aiFix) toolNames(key string) []string {
	f.t.Helper()
	_, v := f.rpc(key, "tools/list", "")
	res, _ := v["result"].(map[string]any)
	ts, _ := res["tools"].([]any)
	var out []string
	for _, t := range ts {
		m, _ := t.(map[string]any)
		out = append(out, m["name"].(string))
	}
	return out
}

func aiEvents(t *testing.T, s *database.Store) []domain.Event {
	t.Helper()
	es, _, err := s.Events(context.Background(), database.EventFilter{Kinds: []string{"ai_*"}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

// fakeRadarr answers the few Radarr calls the read makes (written from the
// Radarr manual), with secrets that must never come back.
func fakeRadarr(t *testing.T, key string) string {
	t.Helper()
	routes := map[string]string{
		"/api/v3/system/status": `{"appName":"Radarr","version":"6.4.4","migrationVersion":242}`,
		"/api/v3/movie":         `[{"id":1,"tmdbId":603,"title":"The Matrix","year":1999,"path":"/movies/The Matrix (1999)","monitored":true,"qualityProfileId":1,"tags":[]}]`,
		"/api/v3/indexer":       `[{"id":1,"name":"Nyaa","implementation":"Torznab","fields":[{"name":"baseUrl","value":"http://x/"},{"name":"apiKey","value":"SECRET_IN_RADARR"}]}]`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		b, ok := routes[r.URL.Path]
		if !ok {
			b = "[]"
			if strings.HasSuffix(r.URL.Path, "/naming") {
				b = "{}"
			}
		}
		_, _ = w.Write([]byte(b))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func readBody(key, url string) string {
	b, _ := json.Marshal(map[string]any{"kind": "radarr", "url": url, "key": key, "allow_local_network": true})
	return string(b)
}

func (f *oldAppFix) exec(q string, args ...any) {
	f.t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q, args...)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
}

func (f *oldAppFix) profile() int64 {
	f.t.Helper()
	return num(arrList(f.t, "profiles", f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/qualityprofile", "", 200), -1)[0].(map[string]any)["id"])
}

// addLabShow adds the lab show as Seerr would, with only season 1 kept up
// to date: its two episodes aired in 2025, the special aired but is off,
// season 2 has not aired.
func (f *oldAppFix) addLabShow() int64 {
	f.t.Helper()
	body := fmt.Sprintf(`{"tvdbId":200,"title":"Lab Show","qualityProfileId":%d,"seasons":[{"seasonNumber":0,"monitored":false},
		{"seasonNumber":1,"monitored":true},{"seasonNumber":2,"monitored":false}],"monitored":true,"monitorNewItems":"all",
		"rootFolderPath":%q,"seriesType":"standard"}`, f.profile(), f.showDir)
	return num(f.arrOK(f.sonarrKey, "POST", "/sonarr/api/v3/series", body, 201).(map[string]any)["id"])
}

func (f *oldAppFix) addLabMovie() int64 {
	f.t.Helper()
	body := fmt.Sprintf(`{"tmdbId":693134,"qualityProfileId":%d,"rootFolderPath":%q,"monitored":true,"minimumAvailability":"released"}`, f.profile(), f.movieDir)
	return num(f.arrOK(f.radarrKey, "POST", "/radarr/api/v3/movie", body, 201).(map[string]any)["id"])
}

// putFileAs puts fl in the Library for a Slot, as an import would.
func (f *oldAppFix) putFileAs(sl mediafiles.Slot, fl mediafiles.File) int64 {
	f.t.Helper()
	fl.SlotIDs, fl.VersionID, fl.AddedAt = []int64{sl.ID}, sl.VersionID, time.Now().UTC()
	fl.Origin, fl.PlacedBy, fl.Role, fl.State = "imported", "hardlink", "main", "present"
	if fl.Languages == nil {
		fl.Languages = []string{}
	}
	if fl.MediaInfo == nil {
		fl.MediaInfo = map[string]any{}
	}
	var id int64
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var err error
		if id, err = database.Insert(tx, "files", &fl, domain.System); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library', active_download_id = NULL WHERE id = ?", id, sl.ID)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// itemOf is the id of a show's episode.
func (f *oldAppFix) itemOf(showID int64, season, ep int) int64 {
	f.t.Helper()
	var id int64
	if err := f.s.Read().QueryRow(`SELECT id FROM items WHERE title_id = ? AND json_extract(numbers, '$.season') = ? AND json_extract(numbers, '$.episode') = ?`,
		showID, season, ep).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// storePoster puts a 400x600 poster in the artwork cache and on the title.
func (f *oldAppFix) storePoster(titleID int64, from string) []byte {
	f.t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, 600))
	for x := 0; x < 400; x++ {
		img.Set(x, x, color.RGBA{200, 20, 20, 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		f.t.Fatal(err)
	}
	name := library.ArtworkName(from)
	if err := os.WriteFile(filepath.Join(f.art.Dir, name), b.Bytes(), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.exec("UPDATE titles SET artwork = ? WHERE id = ?", `["poster:`+name+`"]`, titleID)
	return b.Bytes()
}

// storeWide puts a wide picture's address on the title and its copy in the
// artwork cache, as if fetched once before.
func (f *oldAppFix) storeWide(titleID int64, from string) []byte {
	f.t.Helper()
	b := []byte("wide picture " + from)
	if err := os.WriteFile(filepath.Join(f.art.Dir, library.ArtworkName(from)), b, 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.exec("UPDATE titles SET backdrop = ? WHERE id = ?", from, titleID)
	return b
}

// idSQL writes ids as an SQL list (test helper).
func idSQL(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = itoa(id)
	}
	return "(" + strings.Join(parts, ",") + ")"
}

// prowlarrKey is the fake Prowlarr's own key, written into every entry.
const prowlarrKey = "0123456789abcdef0123456789abcdef"

// prowlarrFeed is a fake Prowlarr on this machine: every /<id>/api answers
// caps for its key. It notes the paths asked.
type prowlarrFeed struct {
	*httptest.Server
	mu    sync.Mutex
	asked []string
}

func newProwlarrFeed(t *testing.T) *prowlarrFeed {
	t.Helper()
	p := &prowlarrFeed{}
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != prowlarrKey || r.URL.Query().Get("t") != "caps" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		p.mu.Lock()
		p.asked = append(p.asked, r.URL.Path)
		p.mu.Unlock()
		_, _ = w.Write([]byte(sourceCaps))
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *prowlarrFeed) calls() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.asked)
}

// fillIndexer makes the entry Prowlarr writes from a template: the name,
// its address for one indexer, its key and the categories, plus any
// other field values given.
func fillIndexer(t *testing.T, tmpl any, name, base string, set map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(tmpl)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["name"] = name
	vals := map[string]any{"baseUrl": base, "apiPath": "/api", "apiKey": prowlarrKey}
	for k, v := range set {
		vals[k] = v
	}
	for _, x := range m["fields"].([]any) {
		f := x.(map[string]any)
		if v, ok := vals[f["name"].(string)]; ok {
			f["value"] = v
		}
	}
	return m
}

func jsonOf(v any) string { b, _ := json.Marshal(v); return string(b) }

// prowlarrManaged are the values Prowlarr compares before it writes again.
func prowlarrManaged(m map[string]any) string {
	out := map[string]any{}
	for _, k := range []string{"name", "implementation", "priority", "enableRss", "enableAutomaticSearch", "enableInteractiveSearch"} {
		out[k] = m[k]
	}
	for _, x := range m["fields"].([]any) {
		f := x.(map[string]any)
		out["field "+f["name"].(string)] = f["value"]
	}
	return jsonOf(out)
}

func (f *oldAppFix) source(id int64) (domain.Source, error) {
	var src domain.Source
	err := database.Get(context.Background(), f.s.Read(), "sources", id, &src)
	return src, err
}

func (f *oldAppFix) mustSource(id int64) domain.Source {
	f.t.Helper()
	src, err := f.source(id)
	if err != nil {
		f.t.Fatalf("source %d: %v", id, err)
	}
	return src
}

// prowlarrGolden compares an answered indexer with its golden file, the
// fake Prowlarr's address and the id made the same every run.
func prowlarrGolden(t *testing.T, name string, v any, feed string) {
	t.Helper()
	m := v.(map[string]any)
	m["id"] = 1
	b, _ := json.MarshalIndent(m, "", "  ")
	got := strings.ReplaceAll(string(b), feed, "http://prowlarr:9696") + "\n"
	path := filepath.Join("testdata", name)
	if os.Getenv("HOLYARRS_WRITE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Fatalf("%s changed:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

// oldAppFix is a signed-in app with a show and a movie Library folder,
// the lab show and movie to add, and the old-app API turned on with a key
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

// arr calls the old-app API the way Seerr does: the key in the address.
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
	tmdb := &fakeShowSource{shows: map[string]metadata.Show{"tmdb:100": apiShow("tmdb", "100"), "tvdb:200": apiShow("tmdb", "100")}}
	// The artwork cache allows no address, so nothing is ever fetched.
	art := &library.ArtworkCache{Dir: t.TempDir()}
	lib := &library.Service{Store: s, Queue: q, Shows: metadata.ShowSources{"tmdb": tmdb, "tvdb": &fakeShowSource{err: metadata.ErrNoTVDBKey}}, Artwork: art}
	vault, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	h := New(Server{Store: s, Queue: q, Library: lib, Metadata: metadata.Lab(), Version: "test",
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

// turnOn switches the old-app API on and makes both keys.
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

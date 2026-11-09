package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Prowlarr's app sync: Prowlarr writes its indexers
// into HolyArrs's /sonarr and /radarr sides as Sources, in the order its
// manual gives (status, templates, test, add, read back, rewrite only on a
// difference, delete).

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
		_, _ = w.Write([]byte(arrSourceCaps))
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

// The whole sync for each side, in Prowlarr's order.
func TestOldAppProwlarrFlow(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	feed := newProwlarrFeed(t)
	for _, side := range []struct {
		name, key, impl, kind string
		set                   map[string]any
	}{
		{"sonarr", f.sonarrKey, "Torznab", "tv", map[string]any{"categories": []any{5030, 5040}, "animeCategories": []any{5070},
			"minimumSeeders": 2, "seedCriteria.seedRatio": 1.5, "seedCriteria.seedTime": 120, "seedCriteria.seasonPackSeedTime": 240}},
		{"radarr", f.radarrKey, "Newznab", "movie", map[string]any{"categories": []any{2040, 2000}}},
	} {
		p := "/" + side.name + "/api/v3/"
		// The version check.
		if st, hd, b := f.arr(side.key, "GET", p+"system/status", ""); st != 200 || hd.Get("X-Application-Version") == "" {
			t.Fatalf("status: %d %s", st, b)
		}
		// The templates: Prowlarr takes the one of the indexer's kind.
		var tmpl any
		for _, x := range arrList(t, "schema", f.arrOK(side.key, "GET", p+"indexer/schema", "", 200), 2) {
			if x.(map[string]any)["implementation"] == side.impl {
				tmpl = x
			}
		}
		if tmpl == nil {
			t.Fatalf("%s: no %s template", side.name, side.impl)
		}
		// Prowlarr's test sends an entry for its id 0; HolyArrs asks it for
		// caps on the local network.
		f.arrOK(side.key, "POST", p+"indexer/test", jsonOf(fillIndexer(t, tmpl, "Test", feed.URL+"/0/", side.set)), 200)
		if c := feed.calls(); len(c) == 0 || c[len(c)-1] != "/0/api" {
			t.Fatalf("%s: the test did not reach Prowlarr: %v", side.name, c)
		}
		// Add: tested first, then kept as a paused Source that may reach
		// the local network, with the key in the vault.
		sent := fillIndexer(t, tmpl, "Lab Tracker (Prowlarr)", feed.URL+"/1/", side.set)
		sent["priority"], sent["enableRss"], sent["tags"] = 20, true, []any{}
		added := f.arrOK(side.key, "POST", p+"indexer", jsonOf(sent), 201).(map[string]any)
		id := num(added["id"])
		src := f.mustSource(id)
		if src.State != "disabled" || !src.AllowLocal || src.URL != feed.URL+"/1/api" || !slices.Equal(src.Kinds, []string{side.kind}) ||
			src.Priority != 20 || src.Name != "Lab Tracker (Prowlarr)" || src.SecretRef == nil {
			t.Fatalf("%s: source %+v", side.name, src)
		}
		if k, err := f.vault.Get(context.Background(), *src.SecretRef); err != nil || k != prowlarrKey {
			t.Fatalf("%s: stored key %q %v", side.name, k, err)
		}
		if strings.Contains(jsonOf(src.Extra), prowlarrKey) {
			t.Fatalf("%s: the key is kept in the clear: %v", side.name, src.Extra)
		}
		wantCats := []int{2000, 2040}
		if side.name == "sonarr" {
			wantCats = []int{5030, 5040, 5070}
			if src.MinSeeders == nil || *src.MinSeeders != 2 {
				t.Errorf("seeders: %v", src.MinSeeders)
			}
		}
		if !slices.Equal(src.Categories[side.kind], wantCats) {
			t.Errorf("%s categories: %v", side.name, src.Categories)
		}
		// Read back: the list and the entry compare equal with what was
		// sent, the key masked.
		want := map[string]any{}
		_ = json.Unmarshal([]byte(jsonOf(sent)), &want)
		for _, x := range want["fields"].([]any) {
			if x.(map[string]any)["name"] == "apiKey" {
				x.(map[string]any)["value"] = "********"
			}
		}
		list := arrList(t, "indexers", f.arrOK(side.key, "GET", p+"indexer", "", 200), 1)
		one := f.arrOK(side.key, "GET", p+"indexer/"+arrItoa(id), "", 200)
		for _, got := range []any{list[0], one, added} {
			if g, w := prowlarrManaged(got.(map[string]any)), prowlarrManaged(want); g != w {
				t.Fatalf("%s reads back differently:\n%s\nwant\n%s", side.name, g, w)
			}
		}
		prowlarrGolden(t, "oldapp_prowlarr_"+side.name+".golden", one, feed.URL)
		// An update with nothing changed (the key sent in the clear again)
		// writes nothing.
		before := f.mustSource(id)
		sent["id"] = id
		f.arrOK(side.key, "PUT", p+"indexer/"+arrItoa(id), jsonOf(sent), 202)
		if after := f.mustSource(id); !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Fatalf("%s: an unchanged update wrote the Source", side.name)
		}
		// Nor does one with the key masked, as read back.
		f.arrOK(side.key, "PUT", p+"indexer/"+arrItoa(id), jsonOf(one), 202)
		if after := f.mustSource(id); !after.UpdatedAt.Equal(before.UpdatedAt) {
			t.Fatalf("%s: a read-back update wrote the Source", side.name)
		}
		// A changed one is written; it stays paused, its key stays.
		sent["priority"], sent["name"] = 10, "Lab Tracker 2 (Prowlarr)"
		f.arrOK(side.key, "PUT", p+"indexer/"+arrItoa(id), jsonOf(sent), 202)
		src = f.mustSource(id)
		if src.Priority != 10 || src.Name != "Lab Tracker 2 (Prowlarr)" || src.State != "disabled" || src.SecretRef == nil {
			t.Fatalf("%s: changed source %+v", side.name, src)
		}
		// Resumed by a person, it stays on through a change of priority,
		// and is paused again when its address moves.
		if st, b := f.session("POST", "/api/v1/sources/"+arrItoa(id)+"/resume", ""); st != 200 {
			t.Fatalf("resume: %d %s", st, b)
		}
		sent["priority"] = 11
		f.arrOK(side.key, "PUT", p+"indexer/"+arrItoa(id), jsonOf(sent), 202)
		if src = f.mustSource(id); src.State != "enabled" {
			t.Fatalf("%s: a priority change paused it: %s", side.name, src.State)
		}
		moved := fillIndexer(t, tmpl, "Lab Tracker 2 (Prowlarr)", feed.URL+"/2/", side.set)
		moved["priority"] = 11
		f.arrOK(side.key, "PUT", p+"indexer/"+arrItoa(id), jsonOf(moved), 202)
		if src = f.mustSource(id); src.State != "disabled" || src.URL != feed.URL+"/2/api" {
			t.Fatalf("%s: moved source %+v", side.name, src)
		}
		if c := feed.calls(); c[len(c)-1] != "/2/api" {
			t.Errorf("%s: the moved address was not tested: %v", side.name, c)
		}
		// Delete removes the Source.
		f.arrOK(side.key, "DELETE", p+"indexer/"+arrItoa(id), "", 200)
		if _, err := f.source(id); err == nil {
			t.Fatalf("%s: the Source is still there", side.name)
		}
		if st, _, _ := f.arr(side.key, "GET", p+"indexer/"+arrItoa(id), ""); st != 404 {
			t.Errorf("%s: deleted indexer: %d", side.name, st)
		}
		arrList(t, "indexers", f.arrOK(side.key, "GET", p+"indexer", "", 200), 0)
	}
}

// What the routes keep to themselves: each side sees only the Sources it
// made, a Source made by hand is never touched, a failed test blocks a
// save unless Prowlarr forces it, and the key's maker must be allowed to
// change Sources.
func TestOldAppProwlarrRules(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	feed := newProwlarrFeed(t)
	tmpl := func(key, side, impl string) any {
		for _, x := range arrList(t, "schema", f.arrOK(key, "GET", "/"+side+"/api/v3/indexer/schema", "", 200), 2) {
			if x.(map[string]any)["implementation"] == impl {
				return x
			}
		}
		t.Fatal("no template")
		return nil
	}
	son := fillIndexer(t, tmpl(f.sonarrKey, "sonarr", "Torznab"), "Both (Prowlarr)", feed.URL+"/5/", nil)
	rad := fillIndexer(t, tmpl(f.radarrKey, "radarr", "Torznab"), "Both (Prowlarr)", feed.URL+"/5/", nil)
	sid := num(f.arrOK(f.sonarrKey, "POST", "/sonarr/api/v3/indexer", jsonOf(son), 201).(map[string]any)["id"])
	// The same indexer on the other side gets its own Source, named with
	// the side's word, and still reads back with the name sent.
	got := f.arrOK(f.radarrKey, "POST", "/radarr/api/v3/indexer", jsonOf(rad), 201).(map[string]any)
	rid := num(got["id"])
	if got["name"] != "Both (Prowlarr)" || f.mustSource(rid).Name != "Both (Prowlarr) (movies)" {
		t.Errorf("second side: %v %q", got["name"], f.mustSource(rid).Name)
	}
	for _, c := range []struct{ method, path string }{{"GET", "indexer/"}, {"PUT", "indexer/"}, {"DELETE", "indexer/"}} {
		if st, _, b := f.arr(f.radarrKey, c.method, "/radarr/api/v3/"+c.path+arrItoa(sid), jsonOf(rad)); st != 404 {
			t.Errorf("radarr %s of a sonarr Source: %d %s", c.method, st, b)
		}
	}
	arrList(t, "radarr indexers", f.arrOK(f.radarrKey, "GET", "/radarr/api/v3/indexer", "", 200), 1)
	// A Source made by hand is not listed and cannot be changed or deleted
	// here.
	st, body := f.session("POST", "/api/v1/sources", `{"name":"By hand","format":"torznab","url":"https://example.invalid/api"}`)
	if st != 201 {
		t.Fatalf("hand source: %d %s", st, body)
	}
	hid := num(func() any { var v map[string]any; _ = json.Unmarshal([]byte(body), &v); return v["id"] }())
	arrList(t, "sonarr indexers", f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/indexer", "", 200), 1)
	for _, m := range []string{"GET", "PUT", "DELETE"} {
		if st, _, _ := f.arr(f.sonarrKey, m, "/sonarr/api/v3/indexer/"+arrItoa(hid), jsonOf(son)); st != 404 {
			t.Errorf("%s of a hand Source: %d", m, st)
		}
	}
	if _, err := f.source(hid); err != nil {
		t.Fatal("the hand Source is gone")
	}
	// An indexer that does not answer: 400 with the reason, then saved
	// when Prowlarr sends it again with forceSave.
	dead := fillIndexer(t, tmpl(f.sonarrKey, "sonarr", "Newznab"), "Dead (Prowlarr)", feed.URL+"/9/", map[string]any{"apiKey": strings.Repeat("w", 8)})
	st, _, b := f.arr(f.sonarrKey, "POST", "/sonarr/api/v3/indexer", jsonOf(dead))
	if st != 400 || !strings.Contains(b, "errorMessage") {
		t.Fatalf("dead indexer: %d %s", st, b)
	}
	did := num(f.arrOK(f.sonarrKey, "POST", "/sonarr/api/v3/indexer?forceSave=true", jsonOf(dead), 201).(map[string]any)["id"])
	if src := f.mustSource(did); src.State != "disabled" || src.Format != "newznab" || src.Protocol != "usenet" || src.MinSeeders != nil {
		t.Errorf("forced source: %+v", src)
	}
	// Another kind of indexer is refused, never pretended.
	other := fillIndexer(t, tmpl(f.sonarrKey, "sonarr", "Torznab"), "Other", feed.URL+"/6/", nil)
	other["implementation"] = "HDBits"
	if st, _, _ := f.arr(f.sonarrKey, "POST", "/sonarr/api/v3/indexer?forceSave=true", jsonOf(other)); st != 400 {
		t.Errorf("another kind: %d", st)
	}
	// A masked key sent with a new host is refused: the stored key never
	// goes to a new host by itself, through the test or an update.
	feed2 := newProwlarrFeed(t)
	away := f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/indexer/"+arrItoa(sid), "", 200).(map[string]any)
	for _, x := range away["fields"].([]any) {
		if x.(map[string]any)["name"] == "baseUrl" {
			x.(map[string]any)["value"] = feed2.URL + "/5/"
		}
	}
	for _, c := range []struct{ method, path string }{{"POST", "indexer/test"}, {"PUT", "indexer/" + arrItoa(sid)}} {
		if st, _, b := f.arr(f.sonarrKey, c.method, "/sonarr/api/v3/"+c.path, jsonOf(away)); st != 400 || !strings.Contains(b, "ApiKey") {
			t.Errorf("masked key to a new host, %s %s: %d %s", c.method, c.path, st, b)
		}
	}
	if c := feed2.calls(); len(c) != 0 {
		t.Fatalf("the stored key went to a new host: %v", c)
	}
	// A long name in letters of more than one byte is cut to fit.
	long := fillIndexer(t, tmpl(f.sonarrKey, "sonarr", "Newznab"), strings.Repeat("Ж", 45)+" (Prowlarr)", feed.URL+"/7/", nil)
	lr := f.arrOK(f.sonarrKey, "POST", "/sonarr/api/v3/indexer", jsonOf(long), 201).(map[string]any)
	if n := f.mustSource(num(lr["id"])).Name; lr["name"] != long["name"] || len(n) > 80 || !strings.HasPrefix(n, "ЖЖЖ") {
		t.Errorf("long name: %v %q", lr["name"], n)
	}
	// Local network turned off by a person and turned on again by Prowlarr
	// pauses the Source until a person resumes it.
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE sources SET allow_local_network = 0, state = 'enabled' WHERE id = ?`, rid)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	rad["priority"] = 30
	f.arrOK(f.radarrKey, "PUT", "/radarr/api/v3/indexer/"+arrItoa(rid), jsonOf(rad), 202)
	if src := f.mustSource(rid); !src.AllowLocal || src.State != "disabled" {
		t.Errorf("local network back on: %v %s", src.AllowLocal, src.State)
	}
	// A key made by someone who may manage the Library but not change
	// Sources reads the list but writes nothing.
	st, body = f.session("POST", "/api/v1/users/invites", `{}`)
	if st != 201 {
		t.Fatalf("invite %d %s", st, body)
	}
	if st, _, b := f.call(http.DefaultClient, "", "POST", "/api/v1/auth/invites/"+strings.TrimPrefix(aiField(body, "path"), "/invite/"),
		`{"name":"ana","password":"a long test passphrase"}`); st != 201 {
		t.Fatalf("join %d %s", st, b)
	}
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET perms = '["ask","manage_library"]' WHERE name = 'ana'`); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE compat_keys SET owner_user_id = (SELECT id FROM users WHERE name = 'ana')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	arrList(t, "member list", f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/indexer", "", 200), 3)
	for _, c := range []struct{ method, path string }{{"POST", "indexer?forceSave=true"}, {"POST", "indexer/test"}, {"PUT", "indexer/" + arrItoa(sid)},
		{"DELETE", "indexer/" + arrItoa(sid)}} {
		if st, _, b := f.arr(f.sonarrKey, c.method, "/sonarr/api/v3/"+c.path, jsonOf(son)); st != 403 {
			t.Errorf("member key %s %s: %d %s", c.method, c.path, st, b)
		}
	}
	if _, err := f.source(sid); err != nil {
		t.Fatal("the member key removed a Source")
	}
}

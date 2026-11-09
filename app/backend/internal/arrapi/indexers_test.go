package arrapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// golden compares got with testdata/name. HOLYARRS_WRITE_GOLDEN=1 rewrites
// the file.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("HOLYARRS_WRITE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
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

// The templates Prowlarr reads before it writes are Newznab and Torznab,
// with the fields it fills and the ones it copies, per side.
func TestIndexerSchema_Templates(t *testing.T) {
	for _, side := range []string{"sonarr", "radarr"} {
		h := &handler{side: side}
		rec := httptest.NewRecorder()
		h.indexerSchema(rec, httptest.NewRequest("GET", "/indexer/schema", nil))
		var v any
		if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		golden(t, "indexer_schema_"+side+".golden", string(b)+"\n")
	}
}

// prowlarrBody fills a template the way Prowlarr does, with its own
// address and key in the fields it manages.
func prowlarrBody(h *handler, format string, set map[string]any) map[string]any {
	b, _ := json.Marshal(h.template(format))
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["name"] = "Lab Tracker (Prowlarr)"
	vals := map[string]any{"baseUrl": "http://prowlarr:9696/3/", "apiPath": "/api", "apiKey": strings.Repeat("p1", 16), "categories": []any{5030.0, 5040.0}}
	for k, v := range set {
		vals[k] = v
	}
	for _, f := range fieldRows(m) {
		if v, ok := vals[fieldName(f)]; ok {
			f["value"] = v
		}
	}
	return m
}

func readBody(t *testing.T, h *handler, m map[string]any) push {
	t.Helper()
	b, _ := json.Marshal(m)
	rec := httptest.NewRecorder()
	p, ok := h.readPush(rec, httptest.NewRequest("POST", "/indexer", strings.NewReader(string(b))))
	if !ok {
		t.Fatalf("push refused: %d %s", rec.Code, rec.Body)
	}
	return p
}

// sourceOf returns the Source a push becomes.
func sourceOf(h *handler, p push) domain.Source {
	in := p.input(h.sourceKind(), p.name)
	src := domain.Source{Name: in.Name, Format: in.Format, Protocol: in.Protocol, URL: in.URL, UseFor: *in.UseFor, Kinds: in.Kinds,
		Categories: in.Categories, MinSeeders: in.MinSeeders, Priority: *in.Priority, AllowLocal: true, Extra: h.extras(nil, p, p.name)}
	ref := int64(1)
	src.SecretRef = &ref
	src.ID = 9
	return src
}

// managed returns the values Prowlarr compares before it writes again.
func managed(m map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"name", "implementation", "priority", "enableRss", "enableAutomaticSearch", "enableInteractiveSearch"} {
		out[k] = m[k]
	}
	for _, f := range fieldRows(m) {
		out["field "+fieldName(f)] = f["value"]
	}
	b, _ := json.Marshal(out)
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	return v
}

// What Prowlarr wrote reads back the same with its key masked, so it does
// not write again. A value changed in HolyArrs reads back changed.
func TestIndexerReadBack_EqualAfterWrite(t *testing.T) {
	for _, c := range []struct{ side, format string }{{"sonarr", "torznab"}, {"sonarr", "newznab"}, {"radarr", "torznab"}, {"radarr", "newznab"}} {
		h := &handler{side: c.side}
		sent := prowlarrBody(h, c.format, map[string]any{"minimumSeeders": 3.0, "seedCriteria.seedRatio": 1.5, "seedCriteria.seedTime": 60.0,
			"categories": []any{5040.0, 5030.0, 2000.0}})
		sent["priority"], sent["enableRss"], sent["tags"], sent["downloadClientId"] = 12.0, false, []any{4.0}, 2.0
		p := readBody(t, h, sent)
		if p.url != "http://prowlarr:9696/3/api" || p.priority != 12 || p.use.Feed || !p.use.AutoSearch || strings.Contains(p.keep, "prowlarrkey") {
			t.Fatalf("%s %s push: %+v", c.side, c.format, p)
		}
		if c.format == "torznab" && (p.seeders == nil || *p.seeders != 3) || c.format == "newznab" && p.seeders != nil {
			t.Errorf("%s %s seeders: %v", c.side, c.format, p.seeders)
		}
		src := sourceOf(h, p)
		got := h.indexerOf(src)
		want := managed(sent)
		want["field apiKey"] = maskedKey
		if g := managed(got); !mapsEqual(g, want) {
			t.Errorf("%s %s reads back:\n%v\nwant\n%v", c.side, c.format, g, want)
		}
		if got["id"] != int64(9) || !slices.Equal(got["tags"].([]any), []any{4.0}) || got["downloadClientId"] != 2.0 {
			t.Errorf("%s %s kept values: %v", c.side, c.format, got)
		}
		// After a change in HolyArrs, the address and categories read back
		// as HolyArrs has them. A name with the side's word after it reads
		// back as sent.
		src.URL, src.Categories = "http://prowlarr:9696/4/api", map[string][]int{h.sourceKind(): {5000}}
		src.Name, src.Extra[extraName] = "Lab Tracker (Prowlarr) (shows)", "Lab Tracker (Prowlarr) (shows)"
		g := managed(h.indexerOf(src))
		if g["field baseUrl"] != "http://prowlarr:9696/4/api" || g["field apiPath"] != "" || !slices.Equal(g["field categories"].([]any), []any{5000.0}) ||
			g["name"] != "Lab Tracker (Prowlarr)" {
			t.Errorf("%s %s changed: %v", c.side, c.format, g)
		}
		src.Name = "Renamed"
		if g := managed(h.indexerOf(src)); g["name"] != "Renamed" {
			t.Errorf("renamed in HolyArrs: %v", g["name"])
		}
	}
}

func mapsEqual(a, b map[string]any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// A body HolyArrs cannot keep as a Source gets a 400 that says what is
// wrong.
func TestIndexerWrite_BadBodyRefused(t *testing.T) {
	h := &handler{side: "sonarr"}
	for name, change := range map[string]func(m map[string]any){
		"another kind":   func(m map[string]any) { m["implementation"] = "HDBits" },
		"no name":        func(m map[string]any) { m["name"] = " " },
		"no address":     func(m map[string]any) { fieldRows(m)[0]["value"] = "" },
		"query":          func(m map[string]any) { fieldRows(m)[0]["value"] = "http://prowlarr:9696/3/?apikey=x" },
		"priority":       func(m map[string]any) { m["priority"] = 60.0 },
		"categories":     func(m map[string]any) { fieldRows(m)[3]["value"] = "5000" },
		"seeders":        func(m map[string]any) { fieldRows(m)[7]["value"] = -1.0 },
		"not valid JSON": nil,
	} {
		body := "{"
		if change != nil {
			m := prowlarrBody(h, "torznab", nil)
			change(m)
			b, _ := json.Marshal(m)
			body = string(b)
		}
		rec := httptest.NewRecorder()
		if _, ok := h.readPush(rec, httptest.NewRequest("POST", "/indexer", strings.NewReader(body))); ok || rec.Code != 400 {
			t.Errorf("%s: %v %d %s", name, ok, rec.Code, rec.Body)
		}
	}
	if joinFeed("http://p:9696/3/", "/api") != "http://p:9696/3/api" || joinFeed("http://p:9696/3/api/", "") != "http://p:9696/3/api" {
		t.Error("joinFeed")
	}
	if cutName(strings.Repeat("é", 90), 80) != strings.Repeat("é", 40) || cutName("a"+strings.Repeat("é", 90), 80) != "a"+strings.Repeat("é", 39) {
		t.Error("cutName")
	}
}

// A write from Prowlarr reaches the rest of the app. A saved Source goes
// to the searcher and a removed one stops being searched.
func TestIndexerWriteTellsTheApp(t *testing.T) {
	st := settingsStore(t)
	v, err := secrets.Open(context.Background(), st, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	var saved, removed []int64
	d := Deps{Store: st, Secrets: v,
		SourceSaved:   func(s domain.Source) { saved = append(saved, s.ID) },
		SourceRemoved: func(id int64) { removed = append(removed, id) }}
	srv := Sonarr(d)
	ctx := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 1, Via: "api_key", Role: webserver.RoleOwner,
		Perms: webserver.DefaultPerms(webserver.RoleOwner), Scopes: []string{webserver.ScopeCompat}})
	send := func(method, path string, body map[string]any) *httptest.ResponseRecorder {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(method, path, rd).WithContext(ctx))
		return rec
	}
	body := prowlarrBody(&handler{side: "sonarr"}, "torznab", nil)
	rec := send("POST", "/sonarr/api/v3/indexer?forceSave=true", body)
	if rec.Code != 201 || len(saved) != 1 {
		t.Fatalf("add: %d %s saved %v", rec.Code, rec.Body, saved)
	}
	var made struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &made); err != nil || made.ID == 0 || saved[0] != made.ID {
		t.Fatalf("id: %v %s saved %v", err, rec.Body, saved)
	}
	id := strconv.FormatInt(made.ID, 10)
	body["name"] = "Lab Tracker renamed"
	if rec = send("PUT", "/sonarr/api/v3/indexer/"+id+"?forceSave=true", body); rec.Code != 202 || len(saved) != 2 {
		t.Fatalf("put: %d %s saved %v", rec.Code, rec.Body, saved)
	}
	if len(removed) != 0 {
		t.Fatalf("removed too early: %v", removed)
	}
	if rec = send("DELETE", "/sonarr/api/v3/indexer/"+id, nil); rec.Code != 200 || !slices.Equal(removed, []int64{made.ID}) {
		t.Fatalf("delete: %d %s removed %v", rec.Code, rec.Body, removed)
	}
}

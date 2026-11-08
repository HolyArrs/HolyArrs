package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/titles/owned
func TestTitlesOwned(t *testing.T) {
	srv, _, _ := setup(t)
	do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201)
	var got struct {
		Items []OwnedItem `json:"items"`
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/titles/owned", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Kind != "movie" || got.Items[0].TMDB != "693134" || got.Items[0].ID == 0 || got.Items[0].State != "wanted" {
		t.Fatalf("owned %+v", got)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/titles/owned", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	tag := resp.Header.Get("ETag")
	if tag == "" || resp.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("headers %v", resp.Header)
	}
	req.Header.Set("If-None-Match", tag)
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

// The SQL prefilter never drops a row foldMatch keeps.
func TestFoldPrefilterKeepsMatches(t *testing.T) {
	_, st, _ := setup(t)
	db := st.Read()
	for _, c := range []struct {
		name, q string
	}{{"Amélie", "amel"}, {"The Dark Knight", "dark kni"}, {"Ünïcode Song", "unicode"}, {"AC/DC Live", "ac dc"}} {
		words := foldWords(c.q)
		pre, args := foldPrefilter("n", words)
		var n int
		if err := db.QueryRow("SELECT count(*) FROM (SELECT ? AS n) WHERE 1 = 1"+pre, append([]any{c.name}, args...)...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if foldMatch(c.name, words) != (n == 1) {
			t.Fatalf("%q %q: sql %d", c.name, c.q, n)
		}
	}
}

// Kept outside answers survive a restart (they live in SQLite).
func TestKeptOnDisk(t *testing.T) {
	_, st, _ := setup(t)
	a := New(Server{Store: st, Version: "test"})
	keepOnDisk(context.Background(), a, "extras|movie:1", TitleExtras{Logo: "x", Scores: []OutsideScore{}})
	b := New(Server{Store: st, Version: "test"})
	k, ok := loadKept[TitleExtras](context.Background(), b, "extras|movie:1")
	if !ok || k.v.Logo != "x" || time.Since(k.at) > time.Minute {
		t.Fatalf("%+v %v", k, ok)
	}
}

// A member sees "requested" only for their own asks (as titleAsks).
func TestTitlesOwnedAsksArePerPerson(t *testing.T) {
	r := newAskRig(t)
	a := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	r.owner.call("POST", "/api/v1/asks/"+itoa(a.ID)+"/approve", `{}`, 200)
	// Members cannot read the Library; a viewer without the right to approve can.
	state := func(who webserver.Caller) string {
		req := httptest.NewRequest("GET", "/api/v1/titles/owned", nil)
		req = req.WithContext(webserver.ContextWith(req.Context(), who))
		rec := httptest.NewRecorder()
		r.api.titlesOwned(rec, req)
		var got struct {
			Items []OwnedItem `json:"items"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		for _, it := range got.Items {
			if it.TMDB == "693134" {
				return it.State
			}
		}
		return "missing"
	}
	if s := state(webserver.Caller{UserID: r.ana.id}); s != "requested" {
		t.Fatalf("ana %s", s)
	}
	if s := state(webserver.Caller{UserID: r.bo.id}); s != "wanted" {
		t.Fatalf("bo %s", s)
	}
	if s := state(webserver.Caller{UserID: r.bo.id, Perms: []webserver.Perm{webserver.PermApprove}}); s != "requested" {
		t.Fatalf("approver %s", s)
	}
}

// The wiki copy on disk is keyed without the caller's name, and each kind
// of kept answer is capped.
func TestKeptKeysBounded(t *testing.T) {
	_, st, _ := setup(t)
	old := keptPerPrefix
	keptPerPrefix = 40
	t.Cleanup(func() { keptPerPrefix = old })
	a := New(Server{Store: st, Version: "test"})
	ctx := context.Background()
	for i := range keptPerPrefix + 5 {
		keepOnDisk(ctx, a, "extras|movie:"+itoa(int64(i+1)), TitleExtras{Scores: []OutsideScore{}})
	}
	var n int
	if err := st.Read().QueryRow("SELECT count(*) FROM lookup_cache WHERE key LIKE 'extras|%'").Scan(&n); err != nil || n != keptPerPrefix {
		t.Fatalf("kept %d %v", n, err)
	}
}

// A bogus name cannot change what another caller gets for a person.
func TestWikiNameCannotPoison(t *testing.T) {
	_, st, _ := setup(t)
	a := New(Server{Store: st, Version: "test", WikiPerson: func(_ context.Context, id, name string) (metadata.WikiEntity, error) {
		return metadata.WikiEntity{Born: "by " + name}, nil
	}})
	get := func(q string) WikiFacts {
		req := httptest.NewRequest("GET", "/api/v1/wiki/person/5"+q, nil)
		req.SetPathValue("kind", "person")
		req.SetPathValue("id", "5")
		rec := httptest.NewRecorder()
		a.wikiFacts(rec, req)
		var v WikiFacts
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		return v
	}
	if v := get("?name=Bogus"); v.Born != "by Bogus" {
		t.Fatalf("named %+v", v)
	}
	if v := get(""); v.Born != "by " {
		t.Fatalf("shared answer poisoned: %+v", v)
	}
	// A fresh process reads the disk copy: still the name-free one.
	wikiFactsCache.mu.Lock()
	wikiFactsCache.seen = map[string]wikiEntry{}
	wikiFactsCache.mu.Unlock()
	b := New(Server{Store: st, Version: "test"})
	if k, ok := loadKept[WikiFacts](context.Background(), b, "wiki|person:5"); !ok || k.v.Born != "by " {
		t.Fatalf("disk %+v %v", k, ok)
	}
}

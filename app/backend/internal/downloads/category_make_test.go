package downloads

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"database/sql"
)

// sabCats is a SABnzbd that keeps its categories and makes one on set_config.
func sabCats(t *testing.T, key string, cats ...string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	cats = append([]string{"*"}, cats...)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		defer mu.Unlock()
		switch {
		case q.Get("mode") == "version":
			_, _ = w.Write([]byte(`{"version":"5.1.3"}`))
		case q.Get("apikey") != key:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("API Key Incorrect"))
		case q.Get("mode") == "get_cats":
			b, _ := json.Marshal(map[string]any{"categories": cats})
			_, _ = w.Write(b)
		case q.Get("mode") == "set_config" && q.Get("section") == "categories" && q.Get("name") != "":
			cats = append(cats, strings.ToLower(q.Get("name")))
			_, _ = w.Write([]byte(`{"config":{"categories":[]}}`))
		default:
			_, _ = w.Write([]byte(`{"queue":{"status":"Idle","slots":[]}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(cats) }
}

type categoryAnswer struct {
	OK       bool   `json:"ok"`
	Text     string `json:"text"`
	Category string `json:"category"`
	CanMake  bool   `json:"can_make"`
	MakeOff  bool   `json:"make_off"`
}

// route: POST /api/v1/downloaders/{id}/category
func TestMissingCategoryMadeOnClick(t *testing.T) {
	t.Parallel()
	srv, s := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab, cats := sabCats(t, key)
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "SAB", "kind": "sabnzbd", "url": sab.URL, "secret": key,
		"category": "movies", "allow_local_network": true, "writable": false}), 201), &d)
	id := itoa(d.ID)
	var res categoryAnswer
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if res.OK || res.Category != "movies" || res.CanMake || !res.MakeOff {
		t.Fatalf("read only test %+v", res)
	}
	// Read only: HolyArrs says why it cannot make it.
	do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/category", `{"name":"movies"}`, 409)
	do(t, "PUT", srv.URL+"/api/v1/downloaders/"+id, body(map[string]any{"name": "SAB", "writable": true}), 200)
	// Saving a Downloader HolyArrs may change makes its category there.
	if !slices.Contains(cats(), "movies") {
		t.Fatalf("not made on save: %v", cats())
	}
	// One that goes missing later is offered on Test.
	if err := s.Write(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET category = 'films' WHERE id = ?", d.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if res.OK || res.Category != "films" || !res.CanMake || res.MakeOff {
		t.Fatalf("test %+v", res)
	}
	// Only one of its own categories.
	do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/category", `{"name":"other"}`, 400)
	do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/category", `{"name":"films"}`, 204)
	if !slices.Contains(cats(), "films") {
		t.Fatalf("cats %v", cats())
	}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if !res.OK {
		t.Fatalf("after make %+v", res)
	}
}

// A category the import set is made on the first check, when HolyArrs may
// change the Downloader; one typed here is not.
func TestImportedCategoryMadeOnCheck(t *testing.T) {
	t.Parallel()
	srv, s := dlServer(t)
	key := "0123456789abcdef0123456789abcdef"
	sab, cats := sabCats(t, key)
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "SAB", "kind": "sabnzbd", "url": sab.URL, "secret": key,
		"category": "tv-sonarr", "allow_local_network": true}), 201), &d)
	set := func(q string, args ...any) {
		if err := s.Write(t.Context(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	set("UPDATE downloaders SET category_movies = 'typo', categories_imported = '[\"tv-sonarr\"]' WHERE id = ?", d.ID)
	id := itoa(d.ID)
	var res categoryAnswer
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if res.OK || res.Category != "typo" || !slices.Contains(cats(), "tv-sonarr") || slices.Contains(cats(), "typo") {
		t.Fatalf("test %+v cats %v", res, cats())
	}
	// Read only: nothing is made.
	set("UPDATE downloaders SET category_movies = '', categories_imported = '[\"radarr\"]', category = 'radarr', writable = 0 WHERE id = ?", d.ID)
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if res.OK || res.Category != "radarr" || slices.Contains(cats(), "radarr") {
		t.Fatalf("read only %+v cats %v", res, cats())
	}
}

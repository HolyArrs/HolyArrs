package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// slskdAnswers are two users' answers: one shares the album as an artist
// folder with the album inside, the other an album folder of MP3s next to a
// locked folder and a cover.
const slskdAnswers = `[
 {"username":"alice","hasFreeUploadSlot":true,"queueLength":0,"files":[
  {"filename":"@@music\\Lab Artist\\Lab Album (2024)\\01 - One.flac","size":30000000,"extension":"flac","bitDepth":24,"sampleRate":96000},
  {"filename":"@@music\\Lab Artist\\Lab Album (2024)\\02 - Two.flac","size":20000000,"extension":"flac","bitDepth":24,"sampleRate":96000},
  {"filename":"@@music\\Lab Artist\\Lab Album (2024)\\cover.jpg","size":100000,"extension":"jpg"}]},
 {"username":"bob","hasFreeUploadSlot":false,"queueLength":12,"files":[
  {"filename":"share\\Lab Artist - Lab Album [2024]\\01.mp3","size":8000000,"extension":"","bitRate":320,"isVariableBitRate":false},
  {"filename":"share\\Lab Artist - Lab Album [2024]\\02.mp3","size":7000000,"extension":"","bitRate":320,"isVariableBitRate":false},
  {"filename":"share\\Locked\\03.mp3","size":7000000,"extension":"mp3","isLocked":true}]}
]`

func TestSlskdSearch(t *testing.T) {
	old := slskdPoll
	slskdPoll.gap = 10 * time.Millisecond
	t.Cleanup(func() { slskdPoll = old })
	var mu sync.Mutex
	var id, text string
	var wait float64
	polls, deleted := 0, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("X-API-Key") != "slskd-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v0/searches":
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			id, _ = b["id"].(string)
			text, _ = b["searchText"].(string)
			wait, _ = b["searchTimeout"].(float64)
			_, _ = w.Write([]byte(`{"id":"` + id + `","state":"InProgress","isComplete":false}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v0/searches/"+id:
			polls++
			_, _ = w.Write([]byte(`{"isComplete":` + map[bool]string{true: "true", false: "false"}[polls > 1] + `}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v0/searches/"+id+"/responses":
			_, _ = w.Write([]byte(slskdAnswers))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v0/searches/"+id:
			deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	items, err := siteClient(srv.URL, "slskd", "slskd-key").siteSearch(context.Background(), Query{Music: true, Artist: "Lab Artist", Album: "Lab Album"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	// slskd counts searchTimeout in milliseconds; 15 meant 15 ms and every search ended at once.
	if wait != 15000 {
		t.Errorf("searchTimeout %v, want 15000 ms", wait)
	}
	if text != "Lab Artist Lab Album" || len(id) != 36 || polls != 2 || !deleted {
		t.Errorf("search %q id %q polls %d deleted %v", text, id, polls, deleted)
	}
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	a, b := items[0], items[1]
	if a.Title != "Lab Artist - Lab Album (2024) [FLAC 24bit]" || a.Size != 50000000 || a.Files != 2 || a.Protocol != "soulseek" || *a.Seeders != 1 {
		t.Errorf("alice %+v", a)
	}
	if b.Title != "Lab Artist - Lab Album [2024] [MP3 320]" || b.Size != 15000000 || *b.Seeders != 0 || *b.Peers != 12 {
		t.Errorf("bob %+v", b)
	}
	rel, err := ParseSlskdLink(a.Link)
	if err != nil || rel.User != "alice" || len(rel.Files) != 2 || rel.Files[0].Name != `@@music\Lab Artist\Lab Album (2024)\01 - One.flac` || rel.Files[0].Size != 30000000 {
		t.Errorf("link %+v %v", rel, err)
	}
}

func TestSlskdNoFeed(t *testing.T) {
	items, err := siteClient("http://127.0.0.1:1", "slskd", "k").siteSearch(context.Background(), Query{feed: true}, nil)
	if err != nil || items != nil {
		t.Errorf("feed: %v %v", items, err)
	}
}

func TestSlskdTest(t *testing.T) {
	state := "Connected, LoggedIn"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v0/application" || r.Header.Get("X-API-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"server":{"state":"` + state + `"}}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := siteClient(srv.URL, "slskd", "k").Test(context.Background()); err != nil {
		t.Errorf("signed in: %v", err)
	}
	state = "Disconnected"
	if _, err := siteClient(srv.URL, "slskd", "k").Test(context.Background()); err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Errorf("signed out: %v", err)
	}
	if _, err := siteClient(srv.URL, "slskd", "wrong").Test(context.Background()); err == nil {
		t.Error("wrong key passed")
	}
	if _, err := siteClient(srv.URL, "slskd", "").Test(context.Background()); err == nil {
		t.Error("no key passed")
	}
}

func TestSlskdLinkDamaged(t *testing.T) {
	for _, l := range []string{"magnet:?x", "slskd:!!", SlskdScheme + "e30", SlskdLink(SlskdRelease{User: "u"}),
		SlskdLink(SlskdRelease{User: "u", Files: []SlskdFile{{Name: `a\x.flac`, Size: 1}, {Name: `b\y.flac`, Size: 1}}})} {
		if _, err := ParseSlskdLink(l); err == nil {
			t.Errorf("%q passed", l)
		}
	}
}

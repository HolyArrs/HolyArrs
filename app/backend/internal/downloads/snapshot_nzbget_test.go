package downloads

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// nzbRPC answers listgroups, history and status like NZBGet and counts the calls.
type nzbRPC struct {
	mu      sync.Mutex
	calls   map[string]int
	groups  []map[string]any
	history []map[string]any
}

func (f *nzbRPC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Method string `json:"method"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[in.Method]++
	var res any = []map[string]any{}
	switch in.Method {
	case "listgroups":
		res = f.groups
	case "history":
		res = f.history
	case "status":
		res = map[string]any{"DownloadRateLo": 1024}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"version": "1.1", "result": res})
}

// The reader must list every NZBGet job through the queue and history reads, never List. With the outside setting off only the Downloader's category shows.
func TestSync_NzbgetListsEveryJob(t *testing.T) {
	f := &nzbRPC{calls: map[string]int{},
		groups: []map[string]any{
			{"NZBID": 1, "NZBName": "A", "Category": "movies", "FileSizeLo": 100, "RemainingSizeLo": 50, "Status": "DOWNLOADING"},
			{"NZBID": 2, "NZBName": "B", "Category": "tv_shows", "FileSizeLo": 100, "RemainingSizeLo": 100, "Status": "PAUSED"},
		},
		history: []map[string]any{
			{"NZBID": 3, "NZBName": "C", "Category": "movies", "Status": "FAILURE/BAD", "MarkStatus": "BAD"},
			{"NZBID": 4, "NZBName": "D", "Category": "music", "Status": "SUCCESS/ALL"},
		}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	for _, c := range []struct {
		setting string
		want    int
	}{{"", 4}, {"false", 2}} {
		e := NewNZBGet(srv.URL, "u", "p", nil)
		s, d := nzbRig(t, e, c.setting)
		if err := s.read(context.Background(), d); err != nil {
			t.Fatal(err)
		}
		items := s.Store.Current(d.ID).Items
		if len(items) != c.want {
			t.Fatalf("setting %q: %d items, want %d", c.setting, len(items), c.want)
		}
		if c.setting == "" && (items["nzbid-3"].State != StateFailed || items["nzbid-2"].State != StatePaused || items["nzbid-1"].Speed != 1024) {
			t.Fatalf("items %+v", items)
		}
	}
	if f.calls["listgroups"] == 0 || f.calls["history"] == 0 || f.calls["status"] == 0 {
		t.Fatalf("calls %v", f.calls)
	}
	// With no change counter the history is read in full each time, and the
	// short read budget applies, since only SABnzbd's first read is long.
	if readBudget(domain.Downloader{ID: 9, Adapter: "nzbget"}, &SabCursors{}) != callTimeout {
		t.Fatal("budget")
	}
	if Cadence("nzbget", 0, true, false, 5*time.Second, 30*time.Second) != Cadence("sabnzbd", 0, true, false, 5*time.Second, 30*time.Second) {
		t.Fatal("NZBGet follows SABnzbd's cadence")
	}
}

// nzbRig builds a reader over a fake NZBGet.
func nzbRig(t *testing.T, e Engine, outside string) (*Sync, domain.Downloader) {
	t.Helper()
	s, d := sabRig(t, nil, outside)
	d.Adapter = "nzbget"
	d.Category = "movies"
	s.Manager = &Manager{Store: s.DB, Active: time.Second, Idle: time.Second, HealthEvery: time.Hour,
		Engine: func(domain.Downloader) (Engine, error) { return e, nil }}
	return s, d
}

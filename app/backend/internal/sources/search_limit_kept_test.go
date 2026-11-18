package sources

import (
	"net/http"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Source saved through the API with a limit but no timeout keeps one Client,
// so its search limit holds across searches.
func TestSearchLimitKeptWithoutTimeout(t *testing.T) {
	var mu sync.Mutex
	n := 0
	srv := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write(file(t, "caps.xml"))
			return
		}
		mu.Lock()
		n++
		mu.Unlock()
		_, _ = w.Write([]byte(tvRSS()))
	})
	f := seedShow(t, srv.URL+"/api", nil)
	exec(t, f.fixture, `UPDATE sources SET limits = ?`, `{"searches_per_window":5,"window_seconds":86400}`)
	s := &Searcher{Store: f.s}
	var first *Client
	for i := 0; i < 40; i++ {
		runKind(t, f.fixture, s, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E02"]}, map[string]any{"by_hand": true})
		for _, c := range s.clients {
			if first == nil {
				first = c
			} else if c != first {
				t.Fatalf("search %d made a new client", i)
			}
		}
		exec(t, f.fixture, `UPDATE slots SET state = 'wanted' WHERE id = ?`, f.slots["S01E02"])
	}
	mu.Lock()
	defer mu.Unlock()
	if n > 5 {
		t.Fatalf("source got %d requests, limit is 5", n)
	}
}

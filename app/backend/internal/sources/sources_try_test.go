package sources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// route: POST /api/v1/sources/{id}/try
func TestSourceTryNow(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(context.Background(), s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	srv := routesServer(t, s, v)
	var down atomic.Bool
	idx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(sourceCaps))
	}))
	t.Cleanup(idx.Close)
	var x SourceView
	raw, _ := json.Marshal(map[string]any{"name": "Lab", "format": "torznab", "url": idx.URL + "/api", "allow_local_network": true})
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources", string(raw), 201), &x)
	id := fmt.Sprint(x.ID)
	pause := func() {
		err := s.Write(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE sources SET state = 'paused', pause_reason = 'rate_limited', paused_until = '2099-01-01T00:00:00Z' WHERE id = ?`, x.ID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	type answer struct {
		OK     bool `json:"ok"`
		Health struct {
			State, Reason, Until string
		} `json:"health"`
	}

	// Still failing: the back-off pause comes back with a fresh wait.
	pause()
	down.Store(true)
	var res answer
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources/"+id+"/try", "", 200), &res)
	if res.OK || res.Health.State != "unreachable" || res.Health.Until == "" || res.Health.Until == "2099-01-01T00:00:00Z" {
		t.Fatalf("failed try %+v", res)
	}

	// Answering again: the Source is back to working.
	pause()
	down.Store(false)
	res = answer{}
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/sources/"+id+"/try", "", 200), &res)
	if !res.OK || res.Health.State != "working" || res.Health.Reason != "" {
		t.Fatalf("good try %+v", res)
	}
}

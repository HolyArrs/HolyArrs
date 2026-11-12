package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type testAnswer struct {
	OK     bool   `json:"ok"`
	Text   string `json:"text"`
	Health struct {
		State string `json:"state"`
	} `json:"health"`
}

// A restarted NZBGet must not lock HolyArrs out. While the breaker holds
// calls back, those refusals never reached the Downloader, so they do not
// lengthen its rest; and once it answers again, the owner's Test goes
// through at once and the good answer ends the rest.
func TestOwnersTestGoesThroughTheBreakerAndEndsTheRest(t *testing.T) {
	ctx := context.Background()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	v, err := secrets.Open(ctx, s, make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	a := &Web{Store: s, Metadata: metadata.Lab(), Secrets: v}
	srv := webRig(t, a)

	f := &fakeNZBGet{user: "u", pass: "p"}
	var down atomic.Bool
	nz := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusBadGateway) // restarting
			return
		}
		f.ServeHTTP(w, r)
	}))
	t.Cleanup(nz.Close)
	var d DownloaderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders", body(map[string]any{"name": "NZBGet", "kind": "nzbget", "url": nz.URL,
		"username": "u", "secret": "p", "category": "movies", "allow_local_network": true}), 201), &d)
	id := itoa(d.ID)

	var dr domain.Downloader
	if err := database.Get(ctx, s.Read(), "downloaders", d.ID, &dr); err != nil {
		t.Fatal(err)
	}
	// Background checks while it restarts open the breaker.
	down.Store(true)
	for range 3 {
		if _, err := a.Manager().Check(ctx, dr); err == nil {
			t.Fatal("check while down answered")
		}
	}
	fails := func() (n int, resting bool) {
		var until sql.NullString
		if err := s.Read().QueryRow("SELECT fail_count, backoff_until FROM downloaders WHERE id = ?", d.ID).Scan(&n, &until); err != nil {
			t.Fatal(err)
		}
		return n, until.Valid
	}
	before, _ := fails()

	down.Store(false)
	if _, err := a.Manager().Check(ctx, dr); !errors.Is(err, outbound.ErrSuppressed) {
		t.Fatalf("background check with the breaker open: %v", err)
	}
	if n, _ := fails(); n != before {
		t.Fatalf("a refusal that never reached the Downloader counted: %d failures, was %d", n, before)
	}

	var res testAnswer
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/downloaders/"+id+"/test", "", 200), &res)
	if !res.OK || res.Health.State != "working" {
		t.Fatalf("owner's test once it is back %+v", res)
	}
	if n, resting := fails(); n != 0 || resting {
		t.Fatalf("rest not ended: %d failures, resting %v", n, resting)
	}
	if _, err := a.Manager().Check(ctx, dr); err != nil {
		t.Fatalf("background check after the owner's test: %v", err)
	}
}

package accounts

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Sign-in succeeds while another write is held up on purpose, and while
// more ordinary writes wait behind it.
func TestSignInWhileAWriteIsStalled(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{WriteCap: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c := newClock()
	a := New(s, Options{Now: c.now, fastHash: true})
	m := http.NewServeMux()
	m.HandleFunc("GET /{$}", okHandler)
	a.Routes(m)
	srv := httptest.NewServer(a.Middleware(webserver.RecordRouteHits(m)))
	t.Cleanup(srv.Close)
	f := &fixture{srv: srv, a: a, s: s, clock: c}
	setupOwner(t, f, client(t))

	release := make(chan struct{})
	var wg sync.WaitGroup
	// Runs before the store closes: every held write has returned.
	t.Cleanup(func() { close(release); wg.Wait() })
	held := make(chan struct{})
	wg.Go(func() {
		_ = s.Write(context.Background(), func(*sql.Tx) error { close(held); <-release; return nil })
	})
	<-held
	// Ordinary writes queue up behind the stalled one; each is held too.
	for range 3 {
		wg.Go(func() {
			_ = s.Write(context.Background(), func(*sql.Tx) error { <-release; return nil })
		})
	}
	time.Sleep(50 * time.Millisecond)
	began := time.Now()
	resp, body := call(t, client(t), "POST", srv.URL+"/api/v1/auth/login", `{"name":"luis","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("sign-in under a stalled write: %d %s", resp.StatusCode, body)
	}
	t.Logf("sign-in took %v behind a stalled write (cap 300ms)", time.Since(began))
	if d := time.Since(began); d > 5*time.Second {
		t.Fatalf("sign-in took %v", d)
	}
}

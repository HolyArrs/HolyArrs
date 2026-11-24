package downloads

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rdt-client stands in for qBittorrent on .NET: every answer carries
// "Server: Kestrel" and a lapsed session gets an empty 403. HolyArrs must
// sign in again and read the list, not call it another app.
func TestQbitRdtClientLapsedSessionSignsInAgain(t *testing.T) {
	logins := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Kestrel")
		switch r.URL.Path {
		case "/api/v2/auth/login":
			logins++
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "s", Path: "/"})
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/info", "/api/v2/sync/maindata":
			if c, err := r.Cookie("SID"); err != nil || c.Value != "s" || logins < 2 {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if r.URL.Path == "/api/v2/sync/maindata" {
				_, _ = w.Write([]byte(`{"rid":1,"full_update":true,"torrents":{}}`))
				return
			}
			_, _ = w.Write([]byte(`[{"hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","name":"x","category":"holyarrs","state":"pausedUP","progress":1,"size":10,"completed":0,"amount_left":10,"completion_on":1794951961}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	q := NewQbittorrentLogin(srv.URL, "u", "p", nil)
	// First sign-in, then pretend the session lapsed (rdt-client restarted).
	_, _ = q.call(context.Background(), http.MethodGet, "torrents/info", nil)
	if _, err := q.info(context.Background(), map[string][]string{"category": {"holyarrs"}}); err != nil {
		t.Fatalf("list after a lapsed session: %v", err)
	}
	if logins < 2 {
		t.Fatalf("signed in %d times, want a fresh sign-in after the 403", logins)
	}
}

// Sonarr on the qBittorrent port still reads as another app: it answers 401.
func TestQbitSonarrOnThePortIsStillAnotherApp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Kestrel")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := NewQbittorrent(srv.URL, nil).Health(context.Background())
	if err == nil {
		t.Fatal("Health passed against Sonarr")
	}
}

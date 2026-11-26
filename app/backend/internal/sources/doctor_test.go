package sources

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestSourceTestNamesEachCause(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/key/"):
			w.WriteHeader(http.StatusUnauthorized)
		case strings.HasPrefix(r.URL.Path, "/radarr/"):
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("<html><head><title>Radarr</title></head></html>"))
		case strings.HasPrefix(r.URL.Path, "/nginx/"):
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("The plain HTTP request was sent to HTTPS port"))
		case strings.HasPrefix(r.URL.Path, "/page/"):
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!doctype html><html><body>Some other app</body></html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer plain.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }))
	defer secure.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(3 * time.Second):
		}
	}))
	defer slow.Close()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := "http://" + l.Addr().String()
	l.Close()

	cases := []struct {
		name  string
		url   string
		local bool
		want  doctor.Cause
	}{
		// The trailing dot makes the name absolute: a DNS search domain with a
		// wildcard record would otherwise turn nas.invalid into a name that resolves.
		{"name not found", "http://nas.invalid.:9117/api", true, doctor.DNS},
		{"refused", closed + "/api", true, doctor.Refused},
		{"timeout", slow.URL + "/api", true, doctor.Timeout},
		{"self-signed certificate", secure.URL + "/api", true, doctor.TLS},
		{"https on an http port", strings.Replace(plain.URL, "http://", "https://", 1) + "/api", true, doctor.UseHTTP},
		{"http on an https port", strings.Replace(secure.URL, "https://", "http://", 1) + "/api", true, doctor.UseHTTPS},
		{"key refused", plain.URL + "/key/api", true, doctor.Auth},
		{"wrong path", plain.URL + "/nothing/api", true, doctor.NotFound},
		{"another app", plain.URL + "/page/api", true, doctor.WrongApp},
		{"another app turns down the key", plain.URL + "/radarr/api", true, doctor.WrongApp},
		{"nginx https port", plain.URL + "/nginx/api", true, doctor.UseHTTPS},
		{"local network policy", plain.URL + "/api", false, doctor.Blocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := New(Config{URL: tc.url, APIKey: "k", Protocol: "torrent", AllowLocal: tc.local, Timeout: 500 * time.Millisecond}).Test(ctx)
			if err == nil {
				t.Fatal("the test passed")
			}
			if got := CauseOf(err); got != tc.want {
				t.Fatalf("cause %q, want %q (error %v)", got, tc.want, err)
			}
			if got := Explain(err); got != doctor.Line(tc.want) {
				t.Fatalf("text %q", got)
			}
			// The health row of a Source whose test failed says the same line.
			paused := "paused"
			kind := KindDown
			src := domain.Source{State: paused, PauseReason: &kind, Extra: map[string]string{"cause": string(CauseOf(err))}}
			if tc.want == doctor.Auth {
				kind = KindLoginRefused
			}
			if h := HealthOf(src); h.Text != doctor.Line(tc.want) || h.Cause != string(tc.want) {
				t.Fatalf("health %+v", h)
			}
		})
	}
}

func TestSourceThroughBridgeNamesTheBridge(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	_, err = New(Config{URL: "http://" + addr + "/1/api", APIKey: "k", AllowLocal: true, Timeout: time.Second}).Test(context.Background())
	if got := ExplainVia("http://"+addr+"/1/api", err, nil); !strings.HasPrefix(got, "Prowlarr isn't answering") {
		t.Fatalf("got %q", got)
	}
}

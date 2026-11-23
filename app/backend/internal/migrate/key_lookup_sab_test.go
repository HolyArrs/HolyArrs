package migrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSabKeyFromGeneralPage(t *testing.T) {
	key := "0123456789abcdef0123456789abcdef"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/config/general/" {
			_, _ = w.Write([]byte(`<input type="text" id="apikey" name="apikey" value="` + key + `" readonly>`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	if got := LookupKeyWith(context.Background(), SABnzbd, srv.URL, true, KeyLogin{}); got.Key != key {
		t.Fatalf("key = %q, want %q", got.Key, key)
	}
}

func TestSabKeyAsksForLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login/", http.StatusSeeOther)
	}))
	defer srv.Close()
	if got := LookupKeyWith(context.Background(), SABnzbd, srv.URL, true, KeyLogin{}); !got.NeedLogin || got.Key != "" {
		t.Fatalf("got %+v, want a login ask", got)
	}
}

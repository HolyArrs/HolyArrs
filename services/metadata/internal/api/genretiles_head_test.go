package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Security 17: a stored tile link that is not https is never re-checked
// (no plain-http requests to wherever a tampered row points).
func TestGenreTilesHeadOKNeedsHTTPS(t *testing.T) {
	asked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked = true }))
	defer srv.Close()
	g := &GenreTiles{HTTP: srv.Client()}
	if g.headOK(context.Background(), srv.URL+"/x.jpg") || asked {
		t.Fatalf("plain http link checked")
	}
}

package wikidata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// OutsideIDs reads the Apple Music (P2850), Deezer (P2722) and Spotify
// (P1902) artist ids of items 50 at a time and keeps the answers: a second
// ask, found or not, makes no call.
func TestOutsideIDsBatchedAndKept(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("action") != "wbgetentities" || r.URL.Query().Get("props") != "claims" {
			t.Errorf("asked %s", r.URL.RawQuery)
		}
		if strings.Count(r.URL.Query().Get("ids"), "|") != 1 {
			t.Errorf("not batched: %s", r.URL.Query().Get("ids"))
		}
		w.Write([]byte(`{"entities":{"Q5":{"id":"Q5","claims":{
"P2850":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"1052638496"}}}],
"P2722":[{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"67898492"}}}],
"P1902":[{"rank":"deprecated","mainsnak":{"snaktype":"value","datavalue":{"value":"old"}}},{"rank":"normal","mainsnak":{"snaktype":"value","datavalue":{"value":"sp5"}}}]}},
"Q6":{"id":"Q6","claims":{}}}}`))
	}))
	defer srv.Close()
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	s := open(t, srv.URL, &now)
	ctx := context.Background()
	got, err := s.OutsideIDs(ctx, []string{"Q5", "Q6"})
	if err != nil {
		t.Fatal(err)
	}
	if g := got["Q5"]; g.AppleMusic != "1052638496" || g.Deezer != "67898492" || g.Spotify != "sp5" {
		t.Fatalf("Q5 %+v", g)
	}
	if g, ok := got["Q6"]; !ok || g != (OutsideIDs{}) {
		t.Fatalf("Q6 %+v %v", g, ok)
	}
	before := calls.Load()
	if again, err := s.OutsideIDs(ctx, []string{"Q6", "Q5"}); err != nil || again["Q5"].Spotify != "sp5" || calls.Load() != before {
		t.Fatalf("kept answers asked again: %v %d", err, calls.Load()-before)
	}
}

package requests

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// The list reads on from ?offset=, so a long one can be read page by page
// with nothing missed or repeated.
func TestAskListPagesWithOffset(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	for _, tmdb := range []string{"693134", "900002", "900003", "900005"} {
		if _, err := r.svc.Ask(ctx, r.ana, tmdb); err != nil {
			t.Fatal(err)
		}
	}
	page := func(q string) []View {
		rec := httptest.NewRecorder()
		askRoutes(r).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/asks?"+q, nil))
		var got struct {
			Asks  []View `json:"asks"`
			Total int    `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Total != 4 {
			t.Fatalf("%s: total %d", q, got.Total)
		}
		return got.Asks
	}
	seen := map[int64]bool{}
	for _, q := range []string{"limit=3", "limit=3&offset=3"} {
		for _, v := range page(q) {
			if seen[v.ID] {
				t.Fatalf("%s repeats %d", q, v.ID)
			}
			seen[v.ID] = true
		}
	}
	if len(seen) != 4 {
		t.Fatalf("read %d of 4", len(seen))
	}
	if n := len(page("offset=9")); n != 0 {
		t.Fatalf("past the end gave %d", n)
	}
	if n := len(page("offset=-2")); n != 4 {
		t.Fatalf("a bad offset gave %d", n)
	}
}

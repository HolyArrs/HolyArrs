package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// route: GET /api/v1/explore/row-stats
// Opens and asks sent from Explore rows come back summed per row, busiest
// first, with the row's plain name and no word on who did what.
func TestExploreRowStatsRoute(t *testing.T) {
	t.Parallel()
	a, _, _ := exploreRig(t)
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	for _, body := range []string{
		`{"row_type":"hidden","event":"open"}`,
		`{"row_type":"hidden","event":"open"}`,
		`{"row_type":"hidden","event":"ask"}`,
		`{"row_type":"classics","event":"open"}`,
		`{"row_type":"hero","event":"ask"}`,
	} {
		resp, err := http.Post(srv.URL+"/api/v1/explore/counts", "application/json", strings.NewReader(body))
		if err != nil || resp.StatusCode != http.StatusNoContent {
			t.Fatalf("count %s: %v %v", body, err, resp.StatusCode)
		}
		resp.Body.Close()
	}
	resp, err := http.Get(srv.URL + "/api/v1/explore/row-stats")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("row-stats: %v %v", err, resp.StatusCode)
	}
	defer resp.Body.Close()
	var out struct {
		Days int              `json:"days"`
		Rows []map[string]any `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Days != 30 || len(out.Rows) != 3 {
		t.Fatalf("got %+v", out)
	}
	cases := []struct {
		row, title  string
		opens, asks float64
	}{
		{"hidden", "Hidden gems", 2, 1},
		{"classics", "Classics", 1, 0},
		{"hero", "", 0, 1},
	}
	for i, c := range cases {
		r := out.Rows[i]
		if r["row_type"] != c.row || r["opens"] != c.opens || r["asks"] != c.asks || (r["title"] != nil && r["title"] != c.title) || (c.title != "" && r["title"] != c.title) {
			t.Fatalf("row %d = %v, want %+v", i, r, c)
		}
		for k := range r {
			if k == "member_id" || k == "user_id" || k == "name" {
				t.Fatalf("row %d says who: %v", i, r)
			}
		}
	}
}

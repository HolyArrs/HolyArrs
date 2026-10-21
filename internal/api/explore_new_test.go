package api

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

func newServiceTopFake(reads *int) func(context.Context, string) (metadata.ServiceTop, error) {
	return func(_ context.Context, region string) (metadata.ServiceTop, error) {
		*reads++
		var top []metadata.ServiceTopItem
		var fresh []metadata.ServiceNewItem
		for i := 0; i < 12; i++ {
			kind := "movie"
			if i%2 == 0 {
				kind = "tv"
			}
			top = append(top, metadata.ServiceTopItem{Rank: i + 1, TMDBID: int64(3000 + i), Kind: kind, Title: "Top " + strconv.Itoa(i)})
			fresh = append(fresh, metadata.ServiceNewItem{TMDBID: int64(4000 + i), Kind: kind, Title: "New " + strconv.Itoa(i), AddedAt: "2026-10-18T00:00:00Z"})
		}
		return metadata.ServiceTop{Region: region, Services: []metadata.ServiceTopList{{Service: "netflix", Name: "Netflix", Source: "streaming-availability", Items: top, New: fresh}}}, nil
	}
}

// route: GET /api/v1/explore/new
// New & Popular: new on the member's services first (each card with its
// service), today's top 10s, then the TMDB rows; a title once per page,
// rows under 6 left out, paged 5 then 4; the service lists read once.
func TestExploreNewPage(t *testing.T) {
	a, s, _ := exploreRig(t)
	reads := 0
	a.s.ServiceTop = newServiceTopFake(&reads)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO explore_prefs (member_id, services, region) VALUES (1, '8', 'US')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	base := srv.URL + "/api/v1/explore/new"
	seen := map[string]bool{}
	var ids []string
	for p := 0; p < 6; p++ {
		pg := getJSON(t, base+"?type=all&page="+strconv.Itoa(p), 200)
		for _, r := range pg["rows"].([]any) {
			row := r.(map[string]any)
			ids = append(ids, row["id"].(string))
			items := row["items"].([]any)
			if len(items) < 6 {
				t.Fatalf("row %s has %d", row["id"], len(items))
			}
			for _, it := range items {
				m := it.(map[string]any)
				k := m["kind"].(string) + m["tmdb_id"].(string)
				if seen[k] {
					t.Fatalf("%s twice", k)
				}
				seen[k] = true
				if row["id"] == "np_services" && m["streaming"].(map[string]any)["service"] != float64(8) {
					t.Fatalf("no service on %v", m)
				}
			}
		}
		if pg["more"] != true {
			break
		}
	}
	if len(ids) < 4 || ids[0] != "np_services" || ids[1] != "np_top_shows" || ids[2] != "np_top_movies" {
		t.Fatalf("rows %v", ids)
	}
	if reads != 1 {
		t.Fatalf("service lists read %d times", reads)
	}
	for _, bad := range []string{"?type=x", "?page=-1", "?page=a"} {
		getJSON(t, base+bad, 400)
	}
}

// route: GET /api/v1/explore/new/services
// The services row alone, for Explore, split by tab.
func TestExploreNewServicesRow(t *testing.T) {
	a, s, _ := exploreRig(t)
	reads := 0
	a.s.ServiceTop = newServiceTopFake(&reads)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO explore_prefs (member_id, services, region) VALUES (1, '8', 'US')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	row := getJSON(t, srv.URL+"/api/v1/explore/new/services?type=shows", 200)["row"].(map[string]any)
	items := row["items"].([]any)
	if len(items) != 6 || items[0].(map[string]any)["kind"] != "show" {
		t.Fatalf("row %v", row)
	}
	getJSON(t, srv.URL+"/api/v1/explore/new/services?type=x", 400)
}

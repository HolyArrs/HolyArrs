package main

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// newNow is the rig's clock: the fake TMDB dates every title 1 September 2026.
var newNow = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

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
			// Half the chart is titles TMDB lists (new), half old unknowns.
			id := int64(3000 + i)
			if i%3 == 0 {
				id = int64(1000 + i*13)
			}
			top = append(top, metadata.ServiceTopItem{Rank: i + 1, TMDBID: id, Kind: kind, Title: "Top " + strconv.Itoa(i)})
			fresh = append(fresh, metadata.ServiceNewItem{TMDBID: int64(4000 + i), Kind: kind, Title: "New " + strconv.Itoa(i), AddedAt: "2026-09-03T00:00:00Z"})
		}
		return metadata.ServiceTop{Region: region, Services: []metadata.ServiceTopList{{Service: "netflix", Name: "Netflix", Source: "streaming-availability", Items: top, New: fresh}}}, nil
	}
}

func newRig(t *testing.T) (*handlers, *httptest.Server, *int) {
	t.Helper()
	a, s, _ := exploreRig(t)
	a.explore.Clock = func() time.Time { return newNow }
	reads := 0
	a.s.ServiceTop = newServiceTopFake(&reads)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO explore_prefs (member_id, services, region) VALUES (1, '8', 'US')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	discover.ResetNewPages()
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return a, srv, &reads
}

// route: GET /api/v1/explore/new
// New & Popular: fresh on the member's services first (each card with its
// service), the hot and new charts, then the TMDB rows; a title once per
// page (plus the rail), rows under 6 left out, paged 5 then 4; the service
// lists read once; page 0 carries the This week rail.
func TestExploreNewPage(t *testing.T) {
	_, srv, reads := newRig(t)
	base := srv.URL + "/api/v1/explore/new"
	seen := map[string]bool{}
	var ids []string
	var rail []any
	for p := 0; p < 6; p++ {
		pg := getJSON(t, base+"?type=all&page="+strconv.Itoa(p), 200)
		if p == 0 {
			rail, _ = pg["rail"].([]any)
			if pg["services_set"] != true {
				t.Fatalf("services_set %v", pg["services_set"])
			}
		} else if _, ok := pg["rail"]; ok {
			t.Fatal("rail on a later page")
		}
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
				if m["fresh"] == nil {
					t.Fatalf("no newness strip on %v in %s", m, row["id"])
				}
			}
		}
		if pg["more"] != true {
			break
		}
	}
	// The top of the page: the Top 10s, then the critics' best (everyone's
	// watching needs charts past their top 10, which this rig has not), then
	// the rest by score, Fresh on your services among them.
	if len(ids) < 4 || ids[0] != "np_top_shows" || ids[1] != "np_top_movies" || ids[2] != "np_acclaimed" || !slices.Contains(ids, "np_services") {
		t.Fatalf("rows %v", ids)
	}
	for _, gone := range []string{"np_week", "np_people", "np_returning"} {
		for _, id := range ids {
			if id == gone {
				t.Fatalf("%s still on the page", gone)
			}
		}
	}
	if len(rail) == 0 {
		t.Fatal("no rail")
	}
	// The rail: dated days, a title once in it.
	inRail := map[string]bool{}
	for _, d := range rail {
		day := d.(map[string]any)
		if _, err := time.Parse("2006-01-02", day["day"].(string)); err != nil {
			t.Fatal(err)
		}
		for _, it := range day["items"].([]any) {
			m := it.(map[string]any)
			k := m["kind"].(string) + m["tmdb_id"].(string)
			if inRail[k] {
				t.Fatalf("%s twice on the rail", k)
			}
			inRail[k] = true
		}
	}
	if *reads != 1 {
		t.Fatalf("service lists read %d times", *reads)
	}
	for _, bad := range []string{"?type=x", "?page=-1", "?page=a"} {
		getJSON(t, base+bad, 400)
	}
}

// The acceptance checks on the built page: most of the top 20 are new in
// the last 30 days, every title is inside its kind's window, no title is in
// more than 2 places, and a cached first page answers fast.
func TestExploreNewAcceptance(t *testing.T) {
	a, srv, _ := newRig(t)
	r := httptest.NewRequest("GET", "/api/v1/explore/new", nil)
	r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Role: webserver.RoleAdmin}))
	var pg discover.NewPage
	for i := 0; i < 40; i++ {
		var building bool
		if pg, building = a.disc.NewRows(r.Context(), r, 1, "all"); !building {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	places := map[string]int{}
	var top []string
	for _, row := range pg.Rows {
		for _, it := range row.Items {
			places[it.Key()]++
			top = append(top, it.Key())
			f := pg.Facts[it.Key()]
			if f.Kind == engine.NewUpcoming || !engine.InWindow(f, newNow) {
				t.Fatalf("%s in %s outside its window: %+v", it.Key(), row.Type, f)
			}
		}
	}
	for _, d := range pg.Rail {
		for _, it := range d.Items {
			places[it.Key()]++
		}
	}
	for k, n := range places {
		if n > 2 {
			t.Fatalf("%s in %d places", k, n)
		}
	}
	top = top[:min(20, len(top))]
	fresh := 0
	for _, k := range top {
		f := pg.Facts[k]
		if f.Kind == engine.NewUpcoming || newNow.Sub(f.At) <= 30*24*time.Hour {
			fresh++
		}
	}
	if len(top) < 20 || fresh*100 < 80*len(top) {
		t.Fatalf("%d of %d top titles new in 30 days", fresh, len(top))
	}
	start := time.Now()
	getJSON(t, srv.URL+"/api/v1/explore/new?type=all&page=0", 200)
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Fatalf("cached first page took %v", d)
	}
}

// The New hero's titles are outside Explore's pools: the ones it picks from
// are noted for the trailer pass (in the page's language), none for a Kid.
func TestExploreNewHeroNotesTrailers(t *testing.T) {
	a, _, _ := newRig(t)
	r := httptest.NewRequest("GET", "/api/v1/explore/new?language=de", nil)
	r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Role: webserver.RoleAdmin}))
	var pg discover.NewPage
	for i := 0; i < 40; i++ {
		var building bool
		if pg, building = a.disc.NewRows(r.Context(), r, 1, "all"); !building {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	kid := r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: 1, Role: webserver.RoleMember, MaxRating: "PG-13"}))
	if _, err := a.disc.NewHero(kid, pg, 1); err != nil {
		t.Fatal(err)
	}
	if got := a.trailers.Asks(time.Now()); len(got) != 0 {
		t.Fatalf("a Kid's hero noted %v", got)
	}
	if _, err := a.disc.NewHero(r, pg, 1); err != nil {
		t.Fatal(err)
	}
	got := a.trailers.Asks(time.Now())
	if len(got) == 0 || len(got) > 24 {
		t.Fatalf("noted %d", len(got))
	}
	for _, j := range got {
		if j[2] != "de" || pg.Scores[j[0]+":"+j[1]] <= 0 {
			t.Fatalf("noted %v", j)
		}
	}
}

// route: GET /api/v1/explore/new/services
// The best of the services row, for Explore: at most 12, at most 3 per
// service, all new, split by tab.
func TestExploreNewServicesRow(t *testing.T) {
	_, srv, _ := newRig(t)
	row := getJSON(t, srv.URL+"/api/v1/explore/new/services?type=shows", 200)["row"].(map[string]any)
	items := row["items"].([]any)
	if len(items) == 0 || len(items) > 12 {
		t.Fatalf("row %v", row)
	}
	per := map[float64]int{}
	for _, it := range items {
		m := it.(map[string]any)
		if m["kind"] != "show" || m["fresh"] == nil {
			t.Fatalf("item %v", m)
		}
		per[m["streaming"].(map[string]any)["service"].(float64)]++
	}
	for s, n := range per {
		if n > 3 {
			t.Fatalf("service %v has %d", s, n)
		}
	}
	getJSON(t, srv.URL+"/api/v1/explore/new/services?type=x", 400)
}

func TestNewFreshText(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC) // a Wednesday
	cases := map[string]engine.NewFact{
		"Added 2d ago":    {Kind: engine.NewService, At: now.AddDate(0, 0, -2)},
		"Added today":     {Kind: engine.NewService, At: now.Add(-time.Hour)},
		"Out Nov 4":       {Kind: engine.NewUpcoming, At: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)},
		"Season 3":        {Kind: engine.NewSeason, At: now.AddDate(0, 0, -4), Season: 3},
		"New episode Fri": {Kind: engine.NewEpisode, At: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)},
		"New series":      {Kind: engine.NewSeries, At: now.AddDate(0, 0, -9)},
		"Out Oct 14":      {Kind: engine.NewRelease, At: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)},
	}
	for want, f := range cases {
		if got := discover.NewFresh(f, now); got == nil || got.Text != want {
			t.Fatalf("%+v: %v, want %s", f, got, want)
		}
	}
	if discover.NewFresh(engine.NewFact{}, now) != nil {
		t.Fatal("no fact, no strip")
	}
}

// A Kid's New & Popular never shows a title over their limit, rail
// included.
func TestKidExploreNew(t *testing.T) {
	k := newKidRig(t)
	discover.ResetNewPages()
	code, b := k.kai.get("/api/v1/explore/new?type=all")
	if code != 200 && code != 403 {
		t.Fatalf("kid new page %d", code)
	}
	if hit := mentionsBlocked(string(b)); hit != "" {
		t.Fatalf("kid sees %s: %s", hit, b)
	}
	if !strings.Contains(string(b), "rows") && code == 200 {
		t.Fatalf("answer %s", b)
	}
}

// route: GET /api/v1/explore/new/service
// A service chip's own list: up to 20 new titles of that service (more
// than its share of the mixed row), every card on that service, Library
// titles last; the hot row is that service's chart, new only; bad asks 400.
func TestExploreNewServiceList(t *testing.T) {
	a, srv, reads := newRig(t)
	ts := database.FormatTime(newNow)
	if err := a.s.Store.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state)
			VALUES (?, 1, ?, 1, 'show', 'S1612', 'S1612', '{"tmdb":"1612"}', 'tmdb', 'by_hand', 'active')`, ts, ts)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	base := srv.URL + "/api/v1/explore/new/service?id=8&type=all"
	var row map[string]any
	for i := 0; i < 40; i++ {
		body := getJSON(t, base, 200)
		row = body["row"].(map[string]any)
		if body["building"] != true {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	items := row["items"].([]any)
	if len(items) < 12 || len(items) > 20 {
		t.Fatalf("service list has %d", len(items))
	}
	lib := false
	for _, it := range items {
		m := it.(map[string]any)
		if m["streaming"].(map[string]any)["service"] != float64(8) || m["fresh"] == nil {
			t.Fatalf("item %v", m)
		}
		if m["in_library"] == true {
			lib = true
		} else if lib {
			t.Fatalf("a title after a Library title: %v", m)
		}
	}
	// S1612 leads the list without the Library; owned, it goes last (and
	// past the 20 when the service has 20 others).
	if first := items[0].(map[string]any); first["tmdb_id"] == "1612" {
		t.Fatal("the Library title still leads")
	}
	hot := getJSON(t, srv.URL+"/api/v1/explore/new/service?id=8&type=shows&row=hot_shows", 200)["row"].(map[string]any)
	for i, it := range hot["items"].([]any) {
		m := it.(map[string]any)
		if m["kind"] != "show" || m["streaming"].(map[string]any)["rank"] != float64(i+1) {
			t.Fatalf("hot item %v", m)
		}
	}
	if n := len(getJSON(t, srv.URL+"/api/v1/explore/new/service?id=999&type=all", 200)["row"].(map[string]any)["items"].([]any)); n != 0 {
		t.Fatalf("unknown service has %d", n)
	}
	if *reads != 1 {
		t.Fatalf("service lists read %d times", *reads)
	}
	for _, bad := range []string{"?type=all", "?id=x", "?id=8&row=nope", "?id=8&type=x"} {
		getJSON(t, srv.URL+"/api/v1/explore/new/service"+bad, 400)
	}
}

// A Kid's service list never shows a title over their limit.
func TestKidExploreNewService(t *testing.T) {
	k := newKidRig(t)
	discover.ResetNewPages()
	code, b := k.kai.get("/api/v1/explore/new/service?id=8&type=all")
	if code != 200 && code != 403 {
		t.Fatalf("kid service list %d", code)
	}
	if hit := mentionsBlocked(string(b)); hit != "" {
		t.Fatalf("kid sees %s: %s", hit, b)
	}
}

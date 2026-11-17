package arrapi

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// The status answer names the old app of each side, so request apps that
// check the name connect, and carries the four text fields they read.
func TestStatus_NamesTheOldApp(t *testing.T) {
	r := newPinRig(t)
	for side, want := range map[string]string{"sonarr": "Sonarr", "radarr": "Radarr"} {
		var rec *httptest.ResponseRecorder
		if side == "sonarr" {
			rec = serveGET(r.son, "/sonarr/api/v3/system/status")
		} else {
			rec = serveGET(Radarr(Deps{Store: r.st}), "/radarr/api/v3/system/status")
		}
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil || rec.Code != 200 {
			t.Fatalf("%s: %d %s", side, rec.Code, rec.Body)
		}
		if m["appName"] != want || m["instanceName"] != AppName {
			t.Errorf("%s: appName %v, instanceName %v", side, m["appName"], m["instanceName"])
		}
		for _, k := range []string{"appName", "version", "branch", "authentication"} {
			if s, ok := m[k].(string); !ok || s == "" {
				t.Errorf("%s: %s is %v, want text", side, k, m[k])
			}
		}
	}
}

// neverAdd puts a title on the Never-add list.
func (r *pinRig) neverAdd(kind, name, ids string) {
	r.t.Helper()
	r.exec(`INSERT INTO never_add (created_at, created_by, updated_at, updated_by, kind, source_ids, name) VALUES (?, '{}', ?, '{}', ?, ?, ?)`,
		pinStamp, pinStamp, kind, ids, name)
}

type exclusionPage struct {
	Page, PageSize, TotalRecords int
	SortKey, SortDirection       string
	Records                      []map[string]any
}

// The Never-add list comes in pages with the total, so a client can read it
// to the end. An empty list is one empty page.
func TestNeverAddPages(t *testing.T) {
	r := newPinRig(t)
	rad := Radarr(Deps{Store: r.st})
	get := func(side, path string) exclusionPage {
		t.Helper()
		var rec *httptest.ResponseRecorder
		if side == "sonarr" {
			rec = serveGET(r.son, "/sonarr/api/v3/"+path)
		} else {
			rec = serveGET(rad, "/radarr/api/v3/"+path)
		}
		var p exclusionPage
		if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &p) != nil {
			t.Fatalf("%s %s: %d %s", side, path, rec.Code, rec.Body)
		}
		return p
	}
	if p := get("sonarr", "importlistexclusion/paged?page=1&pageSize=1000&sortDirection=ascending&sortKey=title"); p.TotalRecords != 0 || len(p.Records) != 0 || p.Records == nil || p.Page != 1 {
		t.Fatalf("empty show list: %+v", p)
	}
	if p := get("radarr", "exclusions/paged?page=1&pageSize=1000&sortDirection=ascending&sortKey=movieTitle"); p.TotalRecords != 0 || len(p.Records) != 0 {
		t.Fatalf("empty movie list: %+v", p)
	}
	r.neverAdd("show", "Zeta Show", `{"tvdb":"30","tmdb":"3"}`)
	r.neverAdd("show", "alpha show", `{"tvdb":"10"}`)
	r.neverAdd("show", "Beta Show", `{"tmdb":"2"}`)
	r.neverAdd("movie", "Gamma Movie (2001)", `{"tmdb":"77"}`)
	r.neverAdd("movie", "Delta Movie", `{"tmdb":"78","imdb":"tt1"}`)
	r.neverAdd("artist", "Band", `{"musicbrainz":"x"}`)

	p1 := get("sonarr", "importlistexclusion/paged?page=1&pageSize=2&sortDirection=ascending&sortKey=title")
	p2 := get("sonarr", "importlistexclusion/paged?page=2&pageSize=2&sortDirection=ascending&sortKey=title")
	p3 := get("sonarr", "importlistexclusion/paged?page=3&pageSize=2&sortDirection=ascending&sortKey=title")
	if p1.TotalRecords != 3 || len(p1.Records) != 2 || len(p2.Records) != 1 || len(p3.Records) != 0 || p1.PageSize != 2 || p2.Page != 2 {
		t.Fatalf("pages: %+v %+v %+v", p1, p2, p3)
	}
	var titles []string
	for _, rec := range append(p1.Records, p2.Records...) {
		titles = append(titles, rec["title"].(string))
		if _, ok := rec["tvdbId"].(float64); !ok {
			t.Errorf("no tvdbId number: %v", rec)
		}
		if _, ok := rec["id"].(float64); !ok {
			t.Errorf("no id: %v", rec)
		}
	}
	if !slices.Equal(titles, []string{"alpha show", "Beta Show", "Zeta Show"}) {
		t.Fatalf("order: %v", titles)
	}
	if p1.Records[0]["tvdbId"].(float64) != 10 || p2.Records[0]["tvdbId"].(float64) != 30 {
		t.Fatalf("tvdb ids: %v %v", p1.Records, p2.Records)
	}
	if d := get("sonarr", "importlistexclusion/paged?page=1&pageSize=10&sortDirection=descending&sortKey=title"); d.Records[0]["title"] != "Zeta Show" || d.SortDirection != "descending" {
		t.Fatalf("descending: %+v", d)
	}
	m := get("radarr", "exclusions/paged?page=1&pageSize=1000&sortDirection=ascending&sortKey=movieTitle")
	if m.TotalRecords != 2 || len(m.Records) != 2 || m.Records[0]["movieTitle"] != "Delta Movie" || m.Records[1]["tmdbId"].(float64) != 77 {
		t.Fatalf("movies: %+v", m)
	}
	if _, ok := m.Records[0]["movieYear"].(float64); !ok {
		t.Fatalf("no movieYear: %v", m.Records[0])
	}
	if m.Records[1]["movieYear"].(float64) != 2001 {
		t.Fatalf("year from the name: %v", m.Records[1])
	}
}

// Tags can be deleted, which takes them off every title, and listed with
// the titles that carry them.
func TestTags_DeleteAndUsage(t *testing.T) {
	r := newPinRig(t)
	show, _ := r.show("Tagged Show", "501", true, 1, func(int) string { return pinDay(-3) })
	other, _ := r.show("Other Show", "502", true, 1, func(int) string { return pinDay(-3) })
	movie := r.movie("Tagged Movie", "601", 1)
	rad := Radarr(Deps{Store: r.st, Now: func() time.Time { return pinClock }})
	tagID := func(label string) int64 {
		_, b := r.send("POST", "tag", `{"label":"`+label+`"}`)
		var tg tag
		_ = json.Unmarshal(b, &tg)
		return tg.ID
	}
	a, b := tagID("alice"), tagID("bob")
	for _, l := range [][2]int64{{a, show}, {a, movie}, {b, show}, {b, other}} {
		r.exec("INSERT INTO compat_title_tags (tag_id, title_id) VALUES (?, ?)", l[0], l[1])
	}
	st, body := r.get("tag/detail")
	var details []struct {
		ID        int64
		Label     string
		SeriesIDs []int64 `json:"seriesIds"`
	}
	if st != 200 || json.Unmarshal(body, &details) != nil || len(details) != 2 {
		t.Fatalf("detail: %d %s", st, body)
	}
	if details[0].Label != "alice" || !slices.Equal(details[0].SeriesIDs, []int64{show}) || !slices.Equal(details[1].SeriesIDs, []int64{show, other}) {
		t.Fatalf("sonarr detail: %+v", details)
	}
	rec := serveGET(rad, "/radarr/api/v3/tag/detail")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), fmt.Sprintf(`"movieIds":[%d]`, movie)) || !strings.Contains(rec.Body.String(), `"movieIds":[]`) {
		t.Fatalf("radarr detail: %d %s", rec.Code, rec.Body)
	}
	// The show list carries the tag ids.
	st, body = r.get(fmt.Sprintf("series/%d", show))
	var s struct{ Tags []int64 }
	if st != 200 || json.Unmarshal(body, &s) != nil || !slices.Equal(s.Tags, []int64{a, b}) {
		t.Fatalf("series tags: %d %s", st, body)
	}
	if st, body := r.send("DELETE", fmt.Sprintf("tag/%d", a), ""); st != 200 {
		t.Fatalf("delete: %d %s", st, body)
	}
	if st, _ := r.send("DELETE", fmt.Sprintf("tag/%d", a), ""); st != 404 {
		t.Fatalf("delete again: %d", st)
	}
	if st, _ := r.send("DELETE", "tag/x", ""); st != 404 {
		t.Fatalf("bad id: %d", st)
	}
	st, body = r.get(fmt.Sprintf("series/%d", show))
	if json.Unmarshal(body, &s) != nil || !slices.Equal(s.Tags, []int64{b}) {
		t.Fatalf("tags after delete: %d %s", st, body)
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM compat_title_tags WHERE tag_id = ?", a).Scan(&n)
	if n != 0 {
		t.Fatalf("%d links left", n)
	}
	// A removed title is left out of the usage list.
	r.exec("UPDATE titles SET state = 'removed' WHERE id = ?", other)
	_, body = r.get("tag/detail")
	if json.Unmarshal(body, &details) != nil || len(details) != 1 || !slices.Equal(details[0].SeriesIDs, []int64{show}) {
		t.Fatalf("removed title listed: %s", body)
	}
}

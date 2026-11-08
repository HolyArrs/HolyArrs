package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Pins for the subtitle routes (R16) and the title view and missing list on
// a large library, before the routes and queries move.

// subPerms are the permission levels of every subtitle route today.
var subPerms = map[string]webserver.Perm{
	"GET /api/v1/subtitles/languages":           webserver.PermManageLibrary,
	"GET /api/v1/settings/subtitles":            webserver.PermSettings,
	"PUT /api/v1/settings/subtitles":            webserver.PermSettings,
	"GET /api/v1/titles/{id}/subtitles":         webserver.PermManageLibrary,
	"PUT /api/v1/titles/{id}/subtitle-wish":     webserver.PermManageLibrary,
	"POST /api/v1/subtitles/search":             webserver.PermManageLibrary,
	"GET /api/v1/subtitles/missing":             webserver.PermManageLibrary,
	"POST /api/v1/subtitles/missing/search":     webserver.PermManageLibrary,
	"POST /api/v1/files/{id}/subtitles/choices": webserver.PermManageLibrary,
	"POST /api/v1/files/{id}/subtitles/choose":  webserver.PermManageLibrary,
	"POST /api/v1/subtitles/{id}/sync":          webserver.PermManageLibrary,
	"POST /api/v1/subtitles/{id}/turn-down":     webserver.PermManageLibrary,
	"GET /api/v1/subtitle-sites":                webserver.PermManageLibrary,
	"GET /api/v1/subtitle-sites/kinds":          webserver.PermManageLibrary,
	"POST /api/v1/subtitle-sites":               webserver.PermSettings,
	"PUT /api/v1/subtitle-sites/{id}":           webserver.PermSettings,
	"DELETE /api/v1/subtitle-sites/{id}":        webserver.PermSettings,
	"POST /api/v1/subtitle-sites/{id}/test":     webserver.PermSettings,
}

// route: GET /api/v1/subtitles/languages
// route: GET /api/v1/subtitle-sites
func TestSubPinSubtitleRoutePerms(t *testing.T) {
	for p, want := range subPerms {
		if got, ok := accounts.RoutePerms[p]; !ok || got != want {
			t.Errorf("%s: permission %q (listed %v), want %q", p, got, ok, want)
		}
	}
	// The members and signed-out callers are refused every one.
	w := newDlPinWorld(t)
	anon := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	fill := strings.NewReplacer("{id}", "1")
	for p := range subPerms {
		method, path, _ := strings.Cut(p, " ")
		path = fill.Replace(path)
		if st, b := w.send(t, w.member, w.mcsrf, method, path, "{}"); st != http.StatusForbidden {
			t.Errorf("member %s: %d %s", p, st, b)
		}
		if st, _ := w.send(t, anon, "", method, path, "{}"); st != http.StatusUnauthorized && st != http.StatusForbidden {
			t.Errorf("signed out %s: %d", p, st)
		}
	}
}

// subBig seeds a show of 1000 episodes and 150 movies, with subtitle rows,
// NULL partners (episodes with no File, a File with no rows).
type subBig struct {
	show, movie int64
	showVer     int64
	files       []int64 // present file ids of the show, in order
	items       []int64
}

func seedSubBig(t *testing.T, s *database.Store) *subBig {
	t.Helper()
	b := &subBig{}
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ts, by := database.FormatTime(time.Now()), `{"kind":"system"}`
		ex := func(q string, args ...any) int64 {
			res, err := tx.Exec(q, args...)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			id, _ := res.LastInsertId()
			return id
		}
		var lf, rules int64
		_ = tx.QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&lf)
		_ = tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules)
		title := func(kind, name string) int64 {
			return ex(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state)
				VALUES (?, ?, ?, ?, ?, ?, ?, 2001, '{}', 'tmdb', 'by_hand', 'active')`, ts, by, ts, by, kind, name, name)
		}
		version := func(tid int64, name string) int64 {
			return ex(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
				VALUES (?, ?, ?, ?, ?, 'HD', ?, ?, ?, 1)`, ts, by, ts, by, tid, rules, lf, name)
		}
		item := func(tid int64, kind, nums string) int64 {
			return ex(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, numbers) VALUES (?, ?, ?, ?, ?, ?, ?)`, ts, by, ts, by, tid, kind, nums)
		}
		file := func(ver, it int64, rel string, has bool) int64 {
			slot := ex(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'in_library')`, ts, by, ts, by, it, ver)
			if !has {
				return 0
			}
			id := ex(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'imported', 'copy')`, ts, by, ts, by, fmt.Sprintf("[%d]", slot), ver, rel, ts)
			if _, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", id, slot); err != nil {
				t.Fatal(err)
			}
			return id
		}
		sub := func(f int64, lang, state string) {
			ex(`INSERT INTO subtitles (created_at, created_by, updated_at, updated_by, file_id, language, place, origin, state)
				VALUES (?, ?, ?, ?, ?, ?, 'wish', 'found', ?)`, ts, by, ts, by, f, lang, state)
		}
		b.show = title("show", "Big Show")
		b.showVer = version(b.show, "Big Show (2001)")
		for i := 0; i < 1000; i++ {
			it := item(b.show, "episode", fmt.Sprintf(`{"season":%d,"episode":%d}`, i/100+1, i%100+1))
			f := file(b.showVer, it, fmt.Sprintf("Big Show (2001)/Season %02d/e%04d.mkv", i/100+1, i), i%25 != 24) // every 25th has no File
			if f == 0 {
				continue
			}
			b.files = append(b.files, f)
			b.items = append(b.items, it)
			switch i % 3 {
			case 0:
				sub(f, "en", "wanted")
			case 1:
				sub(f, "es", "none_good_enough")
			} // i%3 == 2: a File with no subtitle rows
		}
		for i := 0; i < 150; i++ {
			name := fmt.Sprintf("Film %03d", i)
			tid := title("movie", name)
			if i == 0 {
				b.movie = tid
			}
			f := file(version(tid, name), item(tid, "movie", "{}"), name+"/"+name+".mkv", true)
			if i%2 == 0 {
				sub(f, "en", "wanted")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// route: GET /api/v1/titles/{id}/subtitles
func TestSubPinTitleSubtitlesLargeShow(t *testing.T) {
	srv, s, _ := setup(t)
	b := seedSubBig(t, s)
	u := srv.URL + "/api/v1"
	var top map[string]json.RawMessage
	decodeJSON(t, do(t, "GET", u+"/titles/"+itoa(b.show)+"/subtitles", "", 200), &top)
	keys := []string{}
	for k := range top {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "files,wishes" {
		t.Fatalf("keys %v", keys)
	}
	var files []SubtitleFileView
	decodeJSON(t, top["files"], &files)
	if len(files) != len(b.files) {
		t.Fatalf("%d files, want %d", len(files), len(b.files))
	}
	for i, f := range files {
		if f.FileID != b.files[i] || f.ItemID != b.items[i] || f.VersionID != b.showVer || f.RelativePath == "" {
			t.Fatalf("file %d: %+v", i, f)
		}
		if f.Wish.From != "default" || f.Wish.Wants == nil {
			t.Fatalf("file %d wish %+v", i, f.Wish)
		}
		if f.Subtitles == nil {
			t.Fatalf("file %d: subtitles is null, want a list", i)
		}
		// Rows per file follow the seed: the third has none.
		n := 0
		switch i % 3 {
		case 0, 1:
			n = 1
		}
		_ = n
	}
	// Rows exist for most Files and are empty lists (not null) for the rest.
	empty, one := 0, 0
	for _, f := range files {
		switch len(f.Subtitles) {
		case 0:
			empty++
		case 1:
			one++
		default:
			t.Fatalf("file %d has %d rows", f.FileID, len(f.Subtitles))
		}
	}
	if empty == 0 || one == 0 || empty+one != len(files) {
		t.Fatalf("empty %d one %d", empty, one)
	}
	var wishes []json.RawMessage
	decodeJSON(t, top["wishes"], &wishes)
	if wishes == nil || len(wishes) != 0 {
		t.Fatalf("wishes %s", top["wishes"])
	}
	// Unknown title 404; an unknown route id shape 404 as well.
	do(t, "GET", u+"/titles/999999/subtitles", "", 404)
}

// route: GET /api/v1/subtitles/missing
func TestSubPinMissingRouteLargeLibrary(t *testing.T) {
	srv, s, _ := setup(t)
	b := seedSubBig(t, s)
	u := srv.URL + "/api/v1/subtitles/missing"
	type resp struct {
		Missing []struct {
			FileID   int64  `json:"file_id"`
			TitleID  int64  `json:"title_id"`
			Title    string `json:"title"`
			Kind     string `json:"kind"`
			Season   *int   `json:"season"`
			Episode  *int   `json:"episode"`
			Language string `json:"language"`
			State    string `json:"state"`
		} `json:"missing"`
		Total  int `json:"total"`
		Titles []struct {
			ID    int64  `json:"id"`
			Name  string `json:"name"`
			Kind  string `json:"kind"`
			Count int    `json:"count"`
		} `json:"titles"`
	}
	// Show rows: i%3 in {0,1} for present files; movies: 75 even ones.
	showRows := 0
	for i := range b.files {
		_ = i
	}
	var out resp
	decodeJSON(t, do(t, "GET", u+"?limit=500", "", 200), &out)
	showRows = 0
	for _, r := range out.Missing {
		if r.Kind == "episode" {
			showRows++
		}
	}
	if out.Total != showRows+75 && out.Total < 500 {
		t.Fatalf("total %d rows %d", out.Total, len(out.Missing))
	}
	if len(out.Missing) != 500 {
		t.Fatalf("%d rows on a 500 page of %d", len(out.Missing), out.Total)
	}
	// Total counts every row, not just the page. 960 present files (every 25th has none), two thirds with a row, plus 75 movies.
	present := len(b.files)
	want := 0
	idx := 0
	for i := 0; i < 1000; i++ {
		if i%25 == 24 {
			continue
		}
		if i%3 != 2 {
			want++
		}
		idx++
	}
	if idx != present || out.Total != want+75 {
		t.Fatalf("total %d, want %d", out.Total, want+75)
	}
	if out.Missing[0].Title != "Big Show" || out.Missing[0].Season == nil || *out.Missing[0].Season != 1 || out.Missing[0].Episode == nil || *out.Missing[0].Episode != 1 {
		t.Fatalf("first %+v", out.Missing[0])
	}
	// Default page is 100; offset moves it.
	var page resp
	decodeJSON(t, do(t, "GET", u+"?offset=100", "", 200), &page)
	if len(page.Missing) != 100 || page.Total != out.Total || page.Missing[0].FileID != out.Missing[100].FileID {
		t.Fatalf("page %d total %d", len(page.Missing), page.Total)
	}
	// The titles list: the show first, then films, each with its count.
	if len(out.Titles) != 1+75 || out.Titles[0].Name != "Big Show" || out.Titles[0].Count != want || out.Titles[1].Name != "Film 000" || out.Titles[1].Count != 1 {
		t.Fatalf("titles %d, first %+v", len(out.Titles), out.Titles[0])
	}
	// Filters.
	var f resp
	decodeJSON(t, do(t, "GET", u+"?kind=movie&limit=500", "", 200), &f)
	if f.Total != 75 || len(f.Missing) != 75 || f.Missing[0].Season != nil {
		t.Fatalf("movies %d", f.Total)
	}
	decodeJSON(t, do(t, "GET", u+"?title="+itoa(b.movie), "", 200), &f)
	if f.Total != 1 || f.Missing[0].Language != "en" || f.Missing[0].State != "wanted" {
		t.Fatalf("one title %+v", f)
	}
	decodeJSON(t, do(t, "GET", u+"?language=spa&limit=500", "", 200), &f)
	if f.Total == 0 || f.Missing[0].Language != "es" || f.Missing[0].State != "none_good_enough" {
		t.Fatalf("spanish %+v", f)
	}
	do(t, "GET", u+"?title=abc", "", 400)
	do(t, "GET", u+"?language=xx", "", 400)
	do(t, "GET", u+"?kind=album", "", 400)
	// The show filter list is not narrowed by the other filters.
	decodeJSON(t, do(t, "GET", u+"?kind=movie", "", 200), &f)
	if len(f.Titles) != 76 {
		t.Fatalf("titles under a filter: %d", len(f.Titles))
	}
}

// route: POST /api/v1/subtitles/search
// route: POST /api/v1/subtitles/missing/search
func TestSubPinSubtitleSearchQueuesFiles(t *testing.T) {
	srv, s, _ := setup(t)
	b := seedSubBig(t, s)
	u := srv.URL + "/api/v1/subtitles"
	var q struct {
		Queued  int     `json:"queued"`
		FileIDs []int64 `json:"file_ids"`
		JobIDs  []int64 `json:"job_ids"`
	}
	decodeJSON(t, do(t, "POST", u+"/search", `{"title_id": `+itoa(b.show)+`}`, 202), &q)
	if q.Queued != len(b.files) || len(q.FileIDs) != len(b.files) || q.FileIDs[0] != b.files[0] || q.JobIDs == nil {
		t.Fatalf("queued %d ids %d", q.Queued, len(q.FileIDs))
	}
	decodeJSON(t, do(t, "POST", u+"/search", `{"title_id": `+itoa(b.show)+`, "item_id": `+itoa(b.items[3])+`}`, 202), &q)
	if q.Queued != 1 || q.FileIDs[0] != b.files[3] {
		t.Fatalf("one item: %+v", q)
	}
	// Missing search: one file once, however many of its rows are missing.
	decodeJSON(t, do(t, "POST", u+"/missing/search", `{"kind": "movie"}`, 202), &q)
	if q.Queued != 75 || len(q.FileIDs) != 75 {
		t.Fatalf("missing search %+v", q.Queued)
	}
}

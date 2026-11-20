package main

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites/fakesite"
)

// subtitleMovie adds a movie with one File on disk; it returns the title,
// the File and the Library folder.
func subtitleMovie(t *testing.T, s *database.Store) (int64, int64, string) {
	t.Helper()
	var titleID, fileID int64
	var lib string
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ts, by := database.FormatTime(time.Now()), `{"kind":"system"}`
		var lf, rules int64
		if err := tx.QueryRow("SELECT id, path FROM library_folders LIMIT 1").Scan(&lf, &lib); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules); err != nil {
			return err
		}
		ex := func(q string, args ...any) int64 {
			res, err := tx.Exec(q, args...)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			id, _ := res.LastInsertId()
			return id
		}
		titleID = ex(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state)
			VALUES (?, ?, ?, ?, 'movie', 'Big Buck Bunny', 'Big Buck Bunny', 2008, '{"tmdb":"10378"}', 'tmdb', 'by_hand', 'active')`, ts, by, ts, by)
		ver := ex(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
			VALUES (?, ?, ?, ?, ?, 'HD', ?, ?, 'Big Buck Bunny (2008)', 1)`, ts, by, ts, by, titleID, rules, lf)
		item := ex(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind) VALUES (?, ?, ?, ?, ?, 'movie')`, ts, by, ts, by, titleID)
		slot := ex(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'in_library')`, ts, by, ts, by, item, ver)
		fileID = ex(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, release_name, origin, placed_by)
			VALUES (?, ?, ?, ?, ?, ?, 'Big Buck Bunny (2008)/Big Buck Bunny (2008).mkv', ?, 'Big.Buck.Bunny.2008.1080p.BluRay.x264-LAB', 'imported', 'copy')`,
			ts, by, ts, by, "["+itoa(slot)+"]", ver, ts)
		_, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", fileID, slot)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(lib, "Big Buck Bunny (2008)")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Big Buck Bunny (2008).mkv"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	return titleID, fileID, lib
}

// route: GET /api/v1/subtitles/languages
// route: GET /api/v1/settings/subtitles
// route: PUT /api/v1/settings/subtitles
// route: GET /api/v1/titles/{id}/subtitles
// route: PUT /api/v1/titles/{id}/subtitle-wish
// route: GET /api/v1/subtitle-sites
// route: GET /api/v1/subtitle-sites/kinds
// route: POST /api/v1/subtitle-sites
// route: PUT /api/v1/subtitle-sites/{id}
// route: DELETE /api/v1/subtitle-sites/{id}
// route: POST /api/v1/subtitles/search
// route: GET /api/v1/subtitles/missing
// route: POST /api/v1/subtitles/missing/search
// route: POST /api/v1/files/{id}/subtitles/choices
// route: POST /api/v1/files/{id}/subtitles/choose
// route: POST /api/v1/subtitles/{id}/sync
// route: POST /api/v1/subtitles/{id}/turn-down
func TestSubtitleRoutes(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	u := srv.URL + "/api/v1"
	titleID, fileID, lib := subtitleMovie(t, s)
	fake := httptest.NewServer(fakesite.Lab())
	t.Cleanup(fake.Close)

	var langs struct {
		Languages []struct{ Code string } `json:"languages"`
	}
	decodeJSON(t, do(t, "GET", u+"/subtitles/languages", "", 200), &langs)
	if len(langs.Languages) < 50 {
		t.Fatalf("languages %d", len(langs.Languages))
	}
	// Settings: defaults, a change, a refusal.
	var set map[string]any
	decodeJSON(t, do(t, "GET", u+"/settings/subtitles", "", 200), &set)
	if set["pass_mark_movie"] != float64(70) || set["pass_mark_episode"] != float64(90) {
		t.Fatalf("%v", set)
	}
	do(t, "PUT", u+"/settings/subtitles", `{"pass_mark_movie": 101}`, 400)
	do(t, "PUT", u+"/settings/subtitles", `{"languages": [{"language": "eng"}], "search_hours": 12}`, 200)
	decodeJSON(t, do(t, "GET", u+"/settings/subtitles", "", 200), &set)
	if set["search_hours"] != float64(12) {
		t.Fatalf("%v", set)
	}
	// Wishes.
	do(t, "PUT", u+"/titles/"+itoa(titleID)+"/subtitle-wish", `{"languages": [{"language": "klingon"}]}`, 400)
	do(t, "PUT", u+"/titles/"+itoa(titleID)+"/subtitle-wish", `{"version_id": 999, "languages": []}`, 404)
	do(t, "PUT", u+"/titles/9999/subtitle-wish", `{"languages": []}`, 404)
	do(t, "PUT", u+"/titles/"+itoa(titleID)+"/subtitle-wish", `{"languages": [{"language": "en"}, {"language": "es", "hearing_impaired": "either"}]}`, 200)
	// Sites.
	var kinds struct {
		Kinds []struct{ Kind string } `json:"kinds"`
	}
	decodeJSON(t, do(t, "GET", u+"/subtitle-sites/kinds", "", 200), &kinds)
	if len(kinds.Kinds) != 11 {
		t.Fatalf("%v", kinds)
	}
	do(t, "POST", u+"/subtitle-sites", `{"kind": "nope"}`, 400)
	do(t, "POST", u+"/subtitle-sites", `{"kind": "addic7ed", "username": "a"}`, 409)           // needs the password too
	do(t, "POST", u+"/subtitle-sites", `{"kind": "opensubtitles", "api_key": "made-up"}`, 409) // needs the sign-in too
	do(t, "POST", u+"/subtitle-sites", `{"kind": "fake", "url": "ftp://x"}`, 400)
	var site struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
	}
	decodeJSON(t, do(t, "POST", u+"/subtitle-sites", `{"kind": "podnapisi", "enabled": false}`, 201), &site)
	do(t, "PUT", u+"/subtitle-sites/"+itoa(site.ID), `{"kind": "gestdown"}`, 400)
	do(t, "PUT", u+"/subtitle-sites/9999", `{}`, 404)
	do(t, "DELETE", u+"/subtitle-sites/"+itoa(site.ID), "", 204)
	do(t, "DELETE", u+"/subtitle-sites/"+itoa(site.ID), "", 404)
	decodeJSON(t, do(t, "POST", u+"/subtitle-sites", `{"kind": "fake", "url": "`+fake.URL+`", "allow_local_network": true, "limits": {"gap_ms": 1, "per_window": 100, "window_seconds": 3600}}`, 201), &site)
	decodeJSON(t, do(t, "PUT", u+"/subtitle-sites/"+itoa(site.ID), `{"name": "Lab", "url": "`+fake.URL+`", "allow_local_network": true, "enabled": true, "limits": {"gap_ms": 1, "per_window": 100, "window_seconds": 3600}}`, 200), &site)
	var list struct {
		Sites []struct{ Name string } `json:"sites"`
	}
	decodeJSON(t, do(t, "GET", u+"/subtitle-sites", "", 200), &list)
	if len(list.Sites) != 1 || list.Sites[0].Name != "Lab" {
		t.Fatalf("%+v", list)
	}
	// Searches are queued; the lab movie gets its subtitles when one runs.
	do(t, "POST", u+"/subtitles/search", `{}`, 400)
	do(t, "POST", u+"/subtitles/search", `{"title_id": 9999}`, 404)
	var q struct{ Queued int }
	decodeJSON(t, do(t, "POST", u+"/subtitles/search", `{"title_id": `+itoa(titleID)+`}`, 202), &q)
	if q.Queued != 1 {
		t.Fatalf("%+v", q)
	}
	// Choices list both languages; choosing saves one by hand.
	var ch struct {
		Outcomes []struct {
			Want    struct{ Language string }
			Choices []struct {
				Site, Key string
				Score     struct{ Total int }
			}
		}
	}
	decodeJSON(t, do(t, "POST", u+"/files/"+itoa(fileID)+"/subtitles/choices", "", 200), &ch)
	// Review 4.16 (3): looking at choices reaches sites and writes, so it is
	// never a GET (a cross-site link cannot start it).
	do(t, "GET", u+"/files/"+itoa(fileID)+"/subtitles/choices", "", 404) // not served
	if len(ch.Outcomes) != 2 || len(ch.Outcomes[0].Choices) == 0 {
		t.Fatalf("%+v", ch)
	}
	do(t, "POST", u+"/files/9999/subtitles/choices", "", 404)
	do(t, "POST", u+"/files/"+itoa(fileID)+"/subtitles/choose", `{"site": "Lab", "key": "nope"}`, 404)
	c := ch.Outcomes[0].Choices[0]
	var saved struct{ Path string }
	decodeJSON(t, do(t, "POST", u+"/files/"+itoa(fileID)+"/subtitles/choose", `{"site": "`+c.Site+`", "key": "`+c.Key+`"}`, 200), &saved)
	if !strings.HasSuffix(saved.Path, ".en.srt") {
		t.Fatalf("%+v", saved)
	}
	if _, err := os.Stat(filepath.Join(lib, filepath.FromSlash(saved.Path))); err != nil {
		t.Fatal(err)
	}
	// The missing list has Spanish; search all queues it.
	var miss struct {
		Missing []struct{ Language string } `json:"missing"`
		Total   int                         `json:"total"`
	}
	decodeJSON(t, do(t, "GET", u+"/subtitles/missing?language=spa&kind=movie", "", 200), &miss)
	if miss.Total != 1 || miss.Missing[0].Language != "es" {
		t.Fatalf("%+v", miss)
	}
	do(t, "GET", u+"/subtitles/missing?language=xx", "", 400)
	do(t, "GET", u+"/subtitles/missing?kind=album", "", 400)
	do(t, "POST", u+"/subtitles/missing/search", `{"kind": "album"}`, 400)
	decodeJSON(t, do(t, "POST", u+"/subtitles/missing/search", `{"language": "es"}`, 202), &q)
	// The title view lists the File and its rows.
	var tv struct {
		Wishes []any `json:"wishes"`
		Files  []struct {
			Subtitles []struct {
				ID    int64  `json:"id"`
				Place string `json:"place"`
				State string `json:"state"`
			} `json:"subtitles"`
		} `json:"files"`
	}
	decodeJSON(t, do(t, "GET", u+"/titles/"+itoa(titleID)+"/subtitles", "", 200), &tv)
	if len(tv.Wishes) != 1 || len(tv.Files) != 1 {
		t.Fatalf("%+v", tv)
	}
	var subID int64
	for _, r := range tv.Files[0].Subtitles {
		if r.Place == "beside" && r.State == "present" {
			subID = r.ID
		}
	}
	// Timing fix and turn down.
	do(t, "POST", u+"/subtitles/"+itoa(subID)+"/sync", `{"offset_ms": 99999999}`, 409)
	var nr struct{ ID int64 }
	decodeJSON(t, do(t, "POST", u+"/subtitles/"+itoa(subID)+"/sync", `{"offset_ms": 1000}`, 200), &nr)
	do(t, "POST", u+"/subtitles/9999/sync", `{"offset_ms": 1000}`, 404)
	do(t, "POST", u+"/subtitles/"+itoa(subID)+"/turn-down", `{}`, 409) // the old row is replaced
	do(t, "POST", u+"/subtitles/"+itoa(nr.ID)+"/turn-down", `{"reason": "wrong film"}`, 204)
	do(t, "GET", u+"/titles/9999/subtitles", "", 404)
}

package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// shapeOf turns decoded JSON into its structure. Object keys stay in
// order, each leaf becomes its type and a list becomes its first element.
func shapeOf(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := map[string]any{}
		for _, k := range keys {
			out[k] = shapeOf(x[k])
		}
		return out
	case []any:
		if len(x) == 0 {
			return []any{}
		}
		return []any{shapeOf(x[0])}
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	}
	return fmt.Sprintf("%T", v)
}

func pinGoldenShape(t *testing.T, name string, body []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("%s: %v: %s", name, err, body)
	}
	got, _ := json.MarshalIndent(shapeOf(v), "", "  ")
	path := filepath.Join("testdata", name+".shape.golden.json")
	if os.Getenv("PIN_WRITE") == "1" {
		_ = os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("%s shape changed:\n got %s\nwant %s", name, got, want)
	}
}

func (r *pinRig) movie(name, tmdb string, i int) int64 {
	r.t.Helper()
	var folder int64
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		return tx.QueryRow("SELECT id FROM library_folders WHERE kind = 'movie' ORDER BY id LIMIT 1").Scan(&folder)
	})
	if err != nil {
		id := int64(0)
		_ = r.st.Write(context.Background(), func(tx *sql.Tx) error {
			var e error
			id, e = database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: r.t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System)
			return e
		})
		folder = id
	}
	title := r.exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state, cinema_date, digital_date)
		VALUES (?, '{}', ?, '{}', 'movie', ?, ?, ?, ?, 'tmdb', 'import', 'active', '2020-01-01T00:00:00Z', ?)`, pinStamp, pinStamp, name, strings.ToLower(name), 2000+i%25,
		fmt.Sprintf(`{"tmdb":"%s","imdb":"tt%07d"}`, tmdb, i), "2020-03-01T00:00:00Z")
	ver := r.exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
		VALUES (?, '{}', ?, '{}', ?, 'Main', ?, ?, ?, 1)`, pinStamp, pinStamp, title, r.rules, folder, name)
	it := r.exec(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind) VALUES (?, '{}', ?, '{}', ?, 'movie')`, pinStamp, pinStamp, title)
	r.exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, '{}', ?, '{}', ?, ?, 'wanted')`, pinStamp, pinStamp, it, ver)
	return title
}

func (r *pinRig) radarr(path string) (int, []byte) {
	r.t.Helper()
	rec := httptest.NewRecorder()
	Radarr(Deps{Store: r.st, Now: func() time.Time { return pinClock }}).ServeHTTP(rec, httptest.NewRequest("GET", "/radarr/api/v3/"+path, nil))
	return rec.Code, rec.Body.Bytes()
}

// The movie list reads titles, main versions, slots and files over a
// large library, ordered by sort name.
func TestPinRadarrMovieList(t *testing.T) {
	r := newPinRig(t)
	for i := 0; i < 150; i++ {
		r.movie(fmt.Sprintf("Movie %03d", i), fmt.Sprint(100000+i), i)
	}
	removed := r.movie("Removed", "999", 999)
	r.exec("UPDATE titles SET state = 'removed' WHERE id = ?", removed)
	st, b := r.radarr("movie")
	var list []struct {
		ID      int64
		Title   string
		TmdbID  int64
		HasFile bool
	}
	if st != 200 || json.Unmarshal(b, &list) != nil || len(list) != 150 {
		t.Fatalf("list: %d %d %.200s", st, len(list), b)
	}
	if list[0].Title != "Movie 000" || list[149].Title != "Movie 149" || list[0].TmdbID != 100000 || list[0].HasFile {
		t.Fatalf("order or fields: %+v ... %+v", list[0], list[149])
	}
	st, b = r.radarr("movie?tmdbId=100007")
	var one []struct{ Title string }
	_ = json.Unmarshal(b, &one)
	if st != 200 || len(one) != 1 || one[0].Title != "Movie 007" {
		t.Fatalf("tmdbId filter: %d %s", st, b)
	}
	if st, b = r.radarr("movie?tmdbId=424242"); st != 200 || strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("no match: %d %s", st, b)
	}
	if st, _ = r.radarr("movie/999999"); st != 404 {
		t.Fatalf("missing movie: %d", st)
	}
	pinGoldenShape(t, "radarr_movie", func() []byte { _, b := r.radarr("movie?tmdbId=100007"); return b }())
}

// The answers the old apps read keep the same shape.
func TestPinArrAnswerShapes(t *testing.T) {
	r := newPinRig(t)
	r.show("Shape Show", "777", true, 2, func(k int) string { return pinDay(-k) })
	r.send("POST", "tag", `{"label":"x"}`)
	for name, path := range map[string]string{
		"sonarr_series":        "series",
		"sonarr_episode":       "episode?seriesId=1",
		"sonarr_missing":       "wanted/missing?includeSeries=true",
		"sonarr_system_status": "system/status",
		"sonarr_rootfolder":    "rootfolder",
		"sonarr_quality":       "qualityprofile",
		"sonarr_language":      "languageprofile",
		"sonarr_tag":           "tag",
		"sonarr_queue":         "queue",
	} {
		st, b := r.get(path)
		if st != 200 {
			t.Fatalf("%s: %d %s", path, st, b)
		}
		// The show's paths and id hold test folders, so only the structure is compared.
		pinGoldenShape(t, name, b)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/sonarr/api/v3/command", strings.NewReader(`{"name":"RefreshMonitoredDownloads"}`))
	req.Header.Set("Content-Type", "application/json")
	r.son.ServeHTTP(rec, req)
	if rec.Code != 201 {
		t.Fatalf("command: %d %s", rec.Code, rec.Body.String())
	}
	pinGoldenShape(t, "sonarr_command", rec.Body.Bytes())
}

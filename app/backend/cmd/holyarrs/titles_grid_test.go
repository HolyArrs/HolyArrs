package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// route: GET /api/v1/titles
// The Library grid list carries every title of the full list, in the same
// order, with everything the Library page draws, sorts and filters by, and
// leaves out what only a title's own page reads.
func TestTitlesGridKeepsWhatTheLibraryReads(t *testing.T) {
	t.Parallel()
	t.Run("movie with a file", func(t *testing.T) {
		t.Parallel()
		srv, s, _ := setup(t)
		var add struct {
			Title TitleView `json:"title"`
		}
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/titles", `{"id":"tmdb:693134"}`, 201), &add)
		err := s.Write(context.Background(), func(tx *sql.Tx) error {
			name := "Movie.mkv"
			slot := add.Title.Slots[0]
			id, err := database.Insert(tx, "files", &mediafiles.File{SlotIDs: []int64{slot.ID}, VersionID: slot.VersionID, RelativePath: name, Size: 4 << 30, AddedAt: time.Now().UTC(),
				Languages: []string{"en"}, MediaInfo: map[string]any{}, ReleaseName: &name, Quality: mediafiles.Quality{Resolution: 1080, Source: "bluray", Codec: "x265"}, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}, domain.System)
			if err != nil {
				return err
			}
			_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", id, slot.ID)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		checkGrid(t, srv, 1)
	})
	t.Run("show with seasons", func(t *testing.T) {
		t.Parallel()
		srv, _, _, _ := showSetup(t)
		do(t, "POST", srv.URL+"/api/v1/shows", `{"id":"tmdb:100","more_versions":[{"label":"4K"}]}`, 201)
		checkGrid(t, srv, 1)
	})
}

func checkGrid(t *testing.T, srv *httptest.Server, count int) {
	t.Helper()
	rows := func(path string) []map[string]json.RawMessage {
		var w struct {
			Titles []map[string]json.RawMessage `json:"titles"`
		}
		if err := json.Unmarshal(do(t, "GET", srv.URL+path, "", 200), &w); err != nil {
			t.Fatal(err)
		}
		return w.Titles
	}
	full, grid := rows("/api/v1/titles"), rows("/api/v1/titles?grid=1")
	if len(full) != count || len(grid) != len(full) {
		t.Fatalf("full %d titles, grid %d", len(full), len(grid))
	}
	same := []string{"id", "kind", "name", "year", "overview", "ids", "keep_up_to_date", "artwork", "backdrop", "logo", "genres", "runtime_minutes",
		"rating", "score", "votes", "added_at", "size_bytes", "folder_path", "library_folder_id", "pick_rules_id", "pick_rules_name",
		"copy", "last_activity", "asks", "cinema_date", "digital_date", "physical_date"}
	for i, f := range full {
		g := grid[i]
		for _, k := range same {
			if !bytes.Equal(f[k], g[k]) {
				t.Errorf("title %d key %s: full %s, grid %s", i, k, f[k], g[k])
			}
		}
		for _, k := range []string{"gate_reason", "gate_open", "gate_opens_at", "gate_delay_days", "gate_driver", "release_gate", "why_added", "stage", "tags", "people", "no_downloader"} {
			if _, ok := g[k]; ok {
				t.Errorf("title %d: grid still carries %s", i, k)
			}
		}
		var fv, gv []map[string]json.RawMessage
		_ = json.Unmarshal(f["versions"], &fv)
		_ = json.Unmarshal(g["versions"], &gv)
		if len(gv) != len(fv) {
			t.Fatalf("title %d: %d versions in grid, %d in full", i, len(gv), len(fv))
		}
		for j := range fv {
			if !bytes.Equal(fv[j]["label"], gv[j]["label"]) || len(gv[j]) > 3 {
				t.Errorf("title %d version %d: grid %v, full label %s", i, j, gv[j], fv[j]["label"])
			}
		}
		var fs, gs []map[string]json.RawMessage
		_ = json.Unmarshal(f["slots"], &fs)
		_ = json.Unmarshal(g["slots"], &gs)
		if len(gs) != len(fs) {
			t.Fatalf("title %d: %d slots in grid, %d in full", i, len(gs), len(fs))
		}
		for j := range fs {
			for _, k := range []string{"id", "state", "state_label", "reason", "download"} {
				if !bytes.Equal(fs[j][k], gs[j][k]) {
					t.Errorf("title %d slot %d key %s: full %s, grid %s", i, j, k, fs[j][k], gs[j][k])
				}
			}
			var ff, gf map[string]json.RawMessage
			_ = json.Unmarshal(fs[j]["file"], &ff)
			_ = json.Unmarshal(gs[j]["file"], &gf)
			if (ff == nil) != (gf == nil) || !bytes.Equal(ff["State"], gf["State"]) || !bytes.Equal(ff["Quality"], gf["Quality"]) || len(gf) > 3 {
				t.Errorf("title %d slot %d file: grid %v, full state %s", i, j, gf, ff["State"])
			}
		}
		if f["show"] != nil {
			var fshow, gshow map[string]json.RawMessage
			_ = json.Unmarshal(f["show"], &fshow)
			_ = json.Unmarshal(g["show"], &gshow)
			for k, v := range fshow {
				if k != "seasons" && !bytes.Equal(v, gshow[k]) {
					t.Errorf("show key %s: full %s, grid %s", k, v, gshow[k])
				}
			}
			var fse, gse []map[string]json.RawMessage
			_ = json.Unmarshal(fshow["seasons"], &fse)
			_ = json.Unmarshal(gshow["seasons"], &gse)
			if len(gse) != len(fse) {
				t.Fatalf("show: %d seasons in grid, %d in full", len(gse), len(fse))
			}
			for j := range fse {
				for _, k := range []string{"number", "keep_up_to_date", "aired", "dropped"} {
					if !bytes.Equal(fse[j][k], gse[j][k]) {
						t.Errorf("season %d key %s: full %s, grid %s", j, k, fse[j][k], gse[j][k])
					}
				}
			}
		}
	}
	fullRaw, gridRaw := do(t, "GET", srv.URL+"/api/v1/titles", "", 200), do(t, "GET", srv.URL+"/api/v1/titles?grid=1", "", 200)
	if len(gridRaw)*10 > len(fullRaw)*7 {
		t.Errorf("grid %d bytes, full %d: want under 70 percent", len(gridRaw), len(fullRaw))
	}
}

package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Album search over HTTP (stage 3.3): the search routes take an album, a
// track or the whole artist; Releases list per album with the album named;
// Home and the Turned down list say "Artist – Album"; music search settings.
//
// route: POST /api/v1/titles/{id}/search
// route: GET /api/v1/titles/{id}/releases
// route: POST /api/v1/releases/{id}/turn-down
// route: GET /api/v1/turned-down
// route: GET /api/v1/searches
// route: GET /api/v1/settings/search
// route: PUT /api/v1/settings/search
func TestAlbumSearchOverHTTP(t *testing.T) {
	t.Parallel()
	srv, s, lib, _ := musicSetup(t)
	u := srv.URL + "/api/v1"
	ctx := context.Background()
	var added struct {
		Title struct {
			ID int64 `json:"id"`
		} `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", u+"/artists", `{"id":"mbid:`+apiArtist+`","albums":"latest"}`, 201), &added)
	id := added.Title.ID
	if _, err := lib.RefreshArtist(ctx, id, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	tu := fmt.Sprintf("%s/titles/%d", u, id)
	var albums struct {
		Albums []library.AlbumSummary `json:"albums"`
	}
	_ = json.Unmarshal(do(t, "GET", tu+"/albums", "", 200), &albums)
	var first, live int64
	for _, a := range albums.Albums {
		switch a.MBID {
		case apiAlbum:
			first = a.ID
		case apiLive:
			live = a.ID
		}
	}
	// The artist got music Pick rules.
	var rules string
	_ = s.Read().QueryRow("SELECT p.name FROM versions v JOIN pick_rules p ON p.id = v.pick_rules_id WHERE v.title_id = ?", id).Scan(&rules)
	if rules != mediafiles.LosslessFirstName {
		t.Fatalf("artist Pick rules %q", rules)
	}
	jobOf := func(body []byte) []domain.Job {
		var out struct {
			JobIDs []int64 `json:"job_ids"`
		}
		_ = json.Unmarshal(body, &out)
		var js []domain.Job
		for _, j := range out.JobIDs {
			var job domain.Job
			if err := database.Get(ctx, s.Read(), "jobs", j, &job); err != nil {
				t.Fatal(err)
			}
			js = append(js, job)
		}
		return js
	}
	// The whole artist: First Light only (the one kept up to date).
	js := jobOf(do(t, "POST", tu+"/search", "", 202))
	if len(js) != 1 || js[0].Kind != jobs.KindSearchAlbum || js[0].Subject.ID != first || js[0].Params["kept_only"] != true {
		t.Fatalf("whole artist %+v", js)
	}
	albumJob := js[0]
	// Every album looking: the searches are spread out, and one click queues
	// at most artist_search_cap of them and says how many it left out.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('what_to_pick', 'artist_search_cap', '2', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Two more Versions of the artist: First Light by three Versions.
	all := 3
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, label := range []string{"MP3", "Vinyl"} {
			res, err := tx.Exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, keep_up_to_date, is_main)
				SELECT created_at, created_by, updated_at, updated_by, title_id, ?, pick_rules_id, library_folder_id, folder_name || ' ' || ?, 1, 0 FROM versions WHERE title_id = ? LIMIT 1`, label, label, id)
			if err != nil {
				return err
			}
			v, _ := res.LastInsertId()
			if _, err := tx.Exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state)
				SELECT s.created_at, s.created_by, s.updated_at, s.updated_by, s.item_id, ?, 'wanted' FROM slots s JOIN items i ON i.id = s.item_id
				WHERE i.part_id = ? AND s.version_id = (SELECT min(id) FROM versions WHERE title_id = ?)`, v, first, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec("UPDATE jobs SET state = 'done' WHERE id = ?", albumJob.ID) // so the whole-artist click queues anew
		return err
	}); err != nil {
		t.Fatal(err)
	}
	body := do(t, "POST", tu+"/search", "", 202)
	var capped struct {
		LeftOut int `json:"left_out"`
	}
	_ = json.Unmarshal(body, &capped)
	js = jobOf(body)
	if all < 3 || len(js) != 2 || capped.LeftOut != all-2 || !js[1].RunAfter.After(js[0].RunAfter.Add(sources.BulkStagger/2)) {
		t.Fatalf("capped artist search: %d albums, %d jobs, left out %d", all, len(js), capped.LeftOut)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{"DELETE FROM settings WHERE key = 'artist_search_cap'", "DELETE FROM jobs WHERE id > " + fmt.Sprint(albumJob.ID),
			"DELETE FROM slots WHERE version_id IN (SELECT id FROM versions WHERE is_main = 0)", "DELETE FROM versions WHERE is_main = 0"} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// One album to choose from, even one not kept up to date.
	js = jobOf(do(t, "POST", tu+"/search", fmt.Sprintf(`{"album_id":%d,"choose":false}`, live), 202))
	if len(js) != 1 || js[0].Subject.ID != live || js[0].Params["choose"] != false {
		t.Fatalf("album list %+v", js)
	}
	// A track searches its album.
	var trackID, slotID int64
	_ = s.Read().QueryRow("SELECT i.id, s.id FROM items i JOIN slots s ON s.item_id = i.id WHERE i.part_id = ? ORDER BY i.id LIMIT 1", first).Scan(&trackID, &slotID)
	js = jobOf(do(t, "POST", tu+"/search", fmt.Sprintf(`{"item_id":%d}`, trackID), 202))
	if len(js) != 1 || js[0].Kind != jobs.KindSearchAlbum || js[0].Subject.ID != first {
		t.Fatalf("track %+v", js)
	}
	do(t, "POST", tu+"/search", `{"choose":false}`, 400)
	do(t, "POST", tu+"/search", `{"season":1}`, 400)
	do(t, "POST", tu+"/search", `{"album_id":99999}`, 404)
	do(t, "POST", tu+"/search", `{"item_id":99999}`, 404)

	// A Release the album search found, judged for both tracks.
	var slots []int64
	rows, _ := s.Read().Query("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.part_id = ? ORDER BY s.id", first)
	for rows.Next() {
		var x int64
		_ = rows.Scan(&x)
		slots = append(slots, x)
	}
	rows.Close()
	var relID int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		src := &domain.Source{Name: "Lab", Protocol: "torrent", Format: "torznab", URL: "http://x", Kinds: []string{}, Categories: map[string][]int{},
			State: "enabled", Extra: map[string]string{}}
		if _, err := database.Insert(tx, "sources", src, domain.System); err != nil {
			return err
		}
		rel := &domain.Release{SourceID: src.ID, Name: "Lab Band - First Light (2020) [FLAC]", LinkRef: "http://x/1.torrent", Protocol: "torrent",
			Size: 300 << 20, Flags: []string{}, Parsed: map[string]any{}, MatchedSlotIDs: slots, FoundBy: "hand_search", JobID: &albumJob.ID,
			State: "judged", Verdict: map[string]any{"outcome": "chosen", "passed": true, "rank": 1, "reasons": []any{
				map[string]any{"step": "match", "rule": "album", "effect": "pass", "detail": "holds First Light"}}}}
		if _, err := database.Insert(tx, "releases", rel, domain.System); err != nil {
			return err
		}
		relID = rel.ID
		_, err := tx.Exec("UPDATE jobs SET state = 'done', result = '{\"releases\":1,\"refused\":0}', ended_at = ? WHERE id = ?",
			database.FormatTime(albumJob.CreatedAt), albumJob.ID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Releases []sources.ReleaseView `json:"releases"`
		Total    int                   `json:"total"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/releases?album_id=%d", tu, first), "", 200), &list)
	if list.Total != 1 || list.Releases[0].Album != "First Light" || list.Releases[0].TitleName != "Lab Band" || list.Releases[0].Episode != "" {
		t.Fatalf("album list %+v", list)
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/releases?album_id=%d", tu, live), "", 200), &list)
	if list.Total != 0 {
		t.Fatalf("other album %+v", list)
	}
	var v sources.ReleaseView
	_ = json.Unmarshal(do(t, "POST", fmt.Sprintf("%s/releases/%d/turn-down", u, relID), `{"reason":"bad rip"}`, 200), &v)
	if v.State != "turned_down" || v.Album != "First Light" {
		t.Fatalf("turned down %+v", v)
	}
	_ = json.Unmarshal(do(t, "GET", u+"/turned-down", "", 200), &list)
	if list.Total != 1 || list.Releases[0].Album != "First Light" {
		t.Fatalf("turned down list %+v", list)
	}
	var home struct {
		Recent []sources.SearchView `json:"recent"`
	}
	_ = json.Unmarshal(do(t, "GET", u+"/searches", "", 200), &home)
	found := false
	for _, x := range home.Recent {
		found = found || x.JobID == albumJob.ID && x.Album == "First Light" && x.TitleName == "Lab Band" && x.Episode == ""
	}
	if !found {
		t.Fatalf("searches %+v", home.Recent)
	}

	// Music search settings.
	var st sources.SearchSettings
	_ = json.Unmarshal(do(t, "PUT", u+"/settings/search", `{"missing_search_music":false,"music_discography_packs":true,"album_track_tolerance":2}`, 200), &st)
	if st.MissingSearchMusic || !st.MusicDiscographyPacks || st.AlbumTrackTolerance != 2 || st.FeedCheckMinutes != 15 {
		t.Fatalf("music settings %+v", st)
	}
	do(t, "PUT", u+"/settings/search", `{"album_track_tolerance":-2}`, 400)
	do(t, "PUT", u+"/settings/search", `{"album_track_tolerance":51}`, 400)
	_ = json.Unmarshal(do(t, "PUT", u+"/settings/search", `{"album_extra_files":3,"artist_search_cap":50}`, 200), &st)
	if st.AlbumExtraFiles != 3 || st.ArtistSearchCap != 50 {
		t.Fatalf("music settings %+v", st)
	}
	do(t, "PUT", u+"/settings/search", `{"album_extra_files":21}`, 400)
	do(t, "PUT", u+"/settings/search", `{"artist_search_cap":0}`, 400)

	// Movie Pick rules are refused for a new artist.
	var good int64
	_ = s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&good)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	do(t, "POST", u+"/artists", fmt.Sprintf(`{"id":"mbid:%s","pick_rules_id":%d}`, apiArtist, good), 400)
}

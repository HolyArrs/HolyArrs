package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// 4.8.11: a show is added with a 4K Version, gets one Slot per episode per
// Version, takes and drops a third Version, refuses a movie folder for one,
// and History names the Version of an Event.
// route: POST /api/v1/shows
// route: POST /api/v1/titles/{id}/versions
// route: PUT /api/v1/titles/{id}/versions/{vid}
// route: DELETE /api/v1/titles/{id}/versions/{vid}
// route: GET /api/v1/events
func TestShowVersionsAPI(t *testing.T) {
	t.Parallel()
	srv, s, _, _ := showSetup(t)
	u := srv.URL + "/api/v1"
	var movies library.FolderView
	_ = json.Unmarshal(do(t, "POST", u+"/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"Movies"}`, t.TempDir()), 201), &movies)
	do(t, "POST", u+"/shows", `{"id":"tmdb:100","more_versions":[{"label":""}]}`, 400)
	do(t, "POST", u+"/shows", fmt.Sprintf(`{"id":"tmdb:100","more_versions":[{"label":"4K","library_folder_id":%d}]}`, movies.ID), 400)
	var add struct {
		Title TitleView `json:"title"`
	}
	_ = json.Unmarshal(do(t, "POST", u+"/shows", `{"id":"tmdb:100","more_versions":[{"label":"4K"}]}`, 201), &add)
	tv := add.Title
	if len(tv.Versions) != 2 || !tv.Versions[0].Main || tv.Versions[1].Label != "4K" || !strings.HasSuffix(tv.Versions[1].FolderPath, "[4K]") {
		t.Fatalf("added %+v", tv.Versions)
	}
	var eps struct {
		Episodes []EpisodeView `json:"episodes"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/titles/%d/episodes", u, tv.ID), "", 200), &eps)
	if len(eps.Episodes) == 0 {
		t.Fatal("no episodes")
	}
	for _, e := range eps.Episodes {
		if len(e.Slots) != 2 {
			t.Fatalf("episode %d has %d Slots", e.ID, len(e.Slots))
		}
	}
	base := fmt.Sprintf("%s/titles/%d/versions", u, tv.ID)
	do(t, "POST", base, fmt.Sprintf(`{"label":"Remux","library_folder_id":%d}`, movies.ID), 400)
	var v VersionView
	_ = json.Unmarshal(do(t, "POST", base, `{"label":"Remux"}`, 201), &v)
	do(t, "PUT", fmt.Sprintf("%s/%d", base, v.ID), `{"keep_up_to_date":false}`, 200)
	do(t, "DELETE", fmt.Sprintf("%s/%d", base, tv.Versions[0].ID), "", 409)
	do(t, "DELETE", fmt.Sprintf("%s/%d", base, v.ID), "", 204)

	// History names the Version of an Event on a 4K Slot.
	var slot4K int64
	_ = s.Read().QueryRow("SELECT id FROM slots WHERE version_id = ? LIMIT 1", tv.Versions[1].ID).Scan(&slot4K)
	addEvent(t, s, tv.ID, slot4K)
	var hist struct {
		Events []shows.EventView `json:"events"`
	}
	_ = json.Unmarshal(do(t, "GET", fmt.Sprintf("%s/events?title=%d&kind=test_version", u, tv.ID), "", 200), &hist)
	if len(hist.Events) != 1 || hist.Events[0].Version != "4K" {
		t.Fatalf("history %+v", hist.Events)
	}
}

// 4.8.11 review: a Version takes only Pick rules that fit its title (music
// rules never for a show or a movie), its label is plain text that names a
// folder safely, and its folder never lands on another title's folder.
// route: POST /api/v1/titles/{id}/versions
// route: PUT /api/v1/titles/{id}/versions/{vid}
func TestShowVersionRulesLabelsAndFolders(t *testing.T) {
	t.Parallel()
	srv, s, _, _ := showSetup(t)
	u := srv.URL + "/api/v1"
	var music, movie int64
	_ = s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.LosslessFirstName).Scan(&music)
	_ = s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.GoodEverydayName).Scan(&movie)
	if music == 0 || movie == 0 {
		t.Fatal("no presets")
	}
	var add struct {
		Title TitleView `json:"title"`
	}
	b := do(t, "POST", u+"/shows", fmt.Sprintf(`{"id":"tmdb:100","more_versions":[{"label":"4K","pick_rules_id":%d}]}`, music), 400)
	if !strings.Contains(string(b), "for music") {
		t.Fatalf("music rules on add: %s", b)
	}
	_ = json.Unmarshal(do(t, "POST", u+"/shows", `{"id":"tmdb:100"}`, 201), &add)
	base := fmt.Sprintf("%s/titles/%d/versions", u, add.Title.ID)
	do(t, "POST", base, fmt.Sprintf(`{"label":"4K","pick_rules_id":%d}`, music), 400)
	var v VersionView
	_ = json.Unmarshal(do(t, "POST", base, fmt.Sprintf(`{"label":"4K","pick_rules_id":%d}`, movie), 201), &v)
	do(t, "PUT", fmt.Sprintf("%s/%d", base, v.ID), fmt.Sprintf(`{"pick_rules_id":%d}`, music), 400)
	do(t, "PUT", fmt.Sprintf("%s/%d", base, add.Title.Versions[0].ID), `{"label":"CON"}`, 400)

	for _, bad := range []string{"a\nb", "a\tb", "4K‮", "4​K", "HD.", "CON", "lpt1", "nul.txt", "[x]", strings.Repeat("x", 41), "a b"} {
		body, _ := json.Marshal(map[string]string{"label": bad})
		do(t, "POST", base, string(body), 400)
	}

	// Another title whose main folder is "<this show> [Remux]" in the same
	// Library folder: a Remux Version here would land on it.
	var lf int64
	var folder string
	_ = s.Read().QueryRow("SELECT library_folder_id, folder_name FROM versions WHERE id = ?", add.Title.Versions[0].ID).Scan(&lf, &folder)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "show", GateDriver: "none", Name: "Other", SortName: "other", Aliases: []string{},
			SourceIDs: map[string]string{}, MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released",
			MusicTypes: []string{}, WhyAdded: "by_hand", Artwork: []string{}, Status: "running", State: "active"}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		ov := &mediafiles.Version{TitleID: ti.ID, Label: "HD", IsMain: true, PickRulesID: movie, LibraryFolderID: lf, FolderName: folder + " [Remux]"}
		_, err := database.Insert(tx, "versions", ov, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	b = do(t, "POST", base, `{"label":"remux"}`, 409)
	if !strings.Contains(string(b), "another title") {
		t.Fatalf("folder clash: %s", b)
	}
}

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type pickRulesDetail struct {
	ID       int64                    `json:"id"`
	Name     string                   `json:"name"`
	Kinds    []string                 `json:"kinds"`
	Preset   bool                     `json:"preset"`
	Revision int                      `json:"revision"`
	Body     mediafiles.PickRulesBody `json:"body"`
	InUse    struct {
		Versions       int `json:"versions"`
		LibraryFolders int `json:"library_folders"`
		OutsideLists   int `json:"outside_lists"`
		Wishlist       int `json:"wishlist"`
		TitleDefaults  int `json:"title_defaults"`
	} `json:"in_use"`
}

func ruleJSON(t *testing.T, name string, kinds []string, b mediafiles.PickRulesBody) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"name": name, "kinds": kinds, "body": b})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func ruleByName(t *testing.T, base, name string) downloads.PickRulesView {
	t.Helper()
	var got struct {
		PickRules []downloads.PickRulesView `json:"pick_rules"`
	}
	_ = json.Unmarshal(do(t, "GET", base+"/api/v1/pick-rules", "", 200), &got)
	for _, p := range got.PickRules {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no Pick rules named %q in %+v", name, got)
	return downloads.PickRulesView{}
}

// recheckJobs counts the good enough re-check Jobs queued for a Pick rules set.
func recheckJobs(t *testing.T, s *database.Store, id int64) int {
	t.Helper()
	js, err := database.Select[domain.Job](context.Background(), s.Read(), "jobs", "kind = ?", library.KindRecheckPickRules)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, j := range js {
		if j.Subject != nil && j.Subject.ID == id {
			n++
		}
	}
	return n
}

// route: POST /api/v1/pick-rules
// route: GET /api/v1/pick-rules/{id}
// route: PUT /api/v1/pick-rules/{id}
// route: DELETE /api/v1/pick-rules/{id}
func TestCreateEditDeletePickRules(t *testing.T) {
	t.Parallel()
	srv, s, _ := libSetup(t)
	var p pickRulesDetail
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, "Small files", []string{"movie"}, mediafiles.GoodEveryday()), 201), &p)
	if p.ID == 0 || p.Name != "Small files" || p.Revision != 1 || p.Preset || len(p.Body.Steps) == 0 {
		t.Fatalf("create %+v", p)
	}
	path := fmt.Sprintf("%s/api/v1/pick-rules/%d", srv.URL, p.ID)
	var got pickRulesDetail
	_ = json.Unmarshal(do(t, "GET", path, "", 200), &got)
	if got.ID != p.ID || got.Name != "Small files" || got.InUse.Versions != 0 {
		t.Fatalf("get %+v", got)
	}
	b := mediafiles.GoodEveryday()
	b.MinPoints = 25
	_ = json.Unmarshal(do(t, "PUT", path, ruleJSON(t, "Smaller files", []string{"movie", "show"}, b), 200), &got)
	if got.Name != "Smaller files" || got.Revision != 2 || got.Body.MinPoints != 25 || len(got.Kinds) != 2 {
		t.Fatalf("put %+v", got)
	}
	if v := ruleByName(t, srv.URL, "Smaller files"); v.InUse != 0 {
		t.Fatalf("list in_use %+v", v)
	}
	// Saving queues the good enough re-check of the titles that use the set.
	if n := recheckJobs(t, s, p.ID); n != 1 {
		t.Fatalf("re-check jobs after a save: %d, want 1", n)
	}
	do(t, "DELETE", path, "", 204)
	do(t, "GET", path, "", 404)
	do(t, "PUT", path, ruleJSON(t, "x", []string{"movie"}, b), 404)
	do(t, "DELETE", path, "", 404)
	do(t, "GET", srv.URL+"/api/v1/pick-rules/abc", "", 400)
}

func TestPickRulesFieldErrors(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	cases := []struct {
		name  string
		kinds []string
		edit  func(*mediafiles.PickRulesBody)
		field string
	}{
		{"", []string{"movie"}, nil, "name"},
		{"x", []string{"book"}, nil, "kinds"},
		{"x", []string{"movie"}, func(b *mediafiles.PickRulesBody) { b.GoodEnoughAt = "cam" }, "body.good_enough_at"},
		{"x", []string{"movie"}, func(b *mediafiles.PickRulesBody) { b.Steps = nil }, "body.steps"},
		{"x", []string{"movie"}, func(b *mediafiles.PickRulesBody) { b.Traits = []mediafiles.TraitPoints{{Trait: "glitter"}} }, "body.traits"},
		{"x", []string{"movie"}, func(b *mediafiles.PickRulesBody) { b.Protocol.Prefer = "ftp" }, "body.protocol.prefer"},
		{"x", []string{"movie"}, func(b *mediafiles.PickRulesBody) { b.Protocol.WaitMinutesTorrent = -1 }, "body.protocol.wait_minutes_torrent"},
		{"x", []string{"movie"}, func(b *mediafiles.PickRulesBody) {
			b.SizeLimits = map[string]mediafiles.SizeLimit{"*": {Min: 2, Max: 1}}
		}, "body.size_limits"},
		{"x", []string{"movie"}, func(b *mediafiles.PickRulesBody) { b.ProperPolicy.MaxAgeDays = -1 }, "body.proper_policy.max_age_days"},
	}
	for _, c := range cases {
		b := mediafiles.GoodEveryday()
		if c.edit != nil {
			c.edit(&b)
		}
		var e struct {
			Error string `json:"error"`
			Field string `json:"field"`
		}
		_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, c.name, c.kinds, b), 400), &e)
		if e.Field != c.field || e.Error == "" {
			t.Errorf("%s: got %+v", c.field, e)
		}
	}
	do(t, "POST", srv.URL+"/api/v1/pick-rules", `{"name":`, 400)
}

func TestPickRulesNameTaken(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	b := string(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, "standARD", []string{"movie"}, mediafiles.GoodEveryday()), 409))
	if !strings.Contains(b, `"field":"name"`) {
		t.Fatalf("taken: %s", b)
	}
}

func TestPresetKeepsNameAndKinds(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	ge := ruleByName(t, srv.URL, mediafiles.GoodEverydayName)
	path := fmt.Sprintf("%s/api/v1/pick-rules/%d", srv.URL, ge.ID)
	b := mediafiles.GoodEveryday()
	b.MinPoints = 3
	do(t, "PUT", path, ruleJSON(t, "Renamed", ge.Kinds, b), 409)
	do(t, "PUT", path, ruleJSON(t, ge.Name, []string{"movie"}, b), 409)
	var got pickRulesDetail
	_ = json.Unmarshal(do(t, "PUT", path, ruleJSON(t, ge.Name, ge.Kinds, b), 200), &got)
	if got.Body.MinPoints != 3 || !got.Preset {
		t.Fatalf("preset body %+v", got)
	}
}

func TestPresetCannotBeRemoved(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	ge := ruleByName(t, srv.URL, mediafiles.GoodEverydayName)
	b := string(do(t, "DELETE", fmt.Sprintf("%s/api/v1/pick-rules/%d", srv.URL, ge.ID), "", 409))
	if !strings.Contains(b, "ready-made") {
		t.Fatalf("delete preset: %s", b)
	}
}

func TestPickRulesInUseCountsEveryReference(t *testing.T) {
	t.Parallel()
	srv, st, _ := libSetup(t)
	var p pickRulesDetail
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, "Mine", []string{"movie", "show"}, mediafiles.GoodEveryday()), 201), &p)
	var lf library.FolderView
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"Movies","default_pick_rules_id":%d}`, t.TempDir(), p.ID), 201), &lf)
	do(t, "POST", srv.URL+"/api/v1/titles", fmt.Sprintf(`{"id":"tmdb:693134","pick_rules_id":%d}`, p.ID), 201)
	do(t, "PUT", srv.URL+"/api/v1/settings/asks", fmt.Sprintf(`{"pick_rules_id":%d,"show_pick_rules_id":%d}`, p.ID, p.ID), 200)
	for _, k := range []string{"movie_pick_rules_id", "show_pick_rules_id"} {
		putSettingRow(t, st, "what_to_pick", k, fmt.Sprint(p.ID))
	}
	putOutsideList(t, st, lf.ID, p.ID)
	path := fmt.Sprintf("%s/api/v1/pick-rules/%d", srv.URL, p.ID)
	var got pickRulesDetail
	_ = json.Unmarshal(do(t, "GET", path, "", 200), &got)
	u := got.InUse
	if u.Versions != 1 || u.LibraryFolders != 1 || u.OutsideLists != 1 || u.Wishlist != 2 || u.TitleDefaults != 2 {
		t.Fatalf("in_use %+v", u)
	}
	if v := ruleByName(t, srv.URL, "Mine"); v.InUse != 7 {
		t.Fatalf("list in_use %d", v.InUse)
	}
	var e struct {
		Error string `json:"error"`
		InUse struct {
			Versions int `json:"versions"`
			Wishlist int `json:"wishlist"`
		} `json:"in_use"`
	}
	_ = json.Unmarshal(do(t, "DELETE", path, "", 409), &e)
	if e.InUse.Versions != 1 || e.InUse.Wishlist != 2 || !strings.Contains(e.Error, "1 Version") || !strings.Contains(e.Error, "Request setting") {
		t.Fatalf("delete in use %+v", e)
	}
}

func TestInUseCannotDropAKindOrSwitchLadder(t *testing.T) {
	t.Parallel()
	srv, _, _ := libSetup(t)
	var p pickRulesDetail
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, "Mine", []string{"movie", "show"}, mediafiles.GoodEveryday()), 201), &p)
	do(t, "POST", srv.URL+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"Movies"}`, t.TempDir()), 201)
	do(t, "POST", srv.URL+"/api/v1/titles", fmt.Sprintf(`{"id":"tmdb:693134","pick_rules_id":%d}`, p.ID), 201)
	path := fmt.Sprintf("%s/api/v1/pick-rules/%d", srv.URL, p.ID)
	do(t, "PUT", path, ruleJSON(t, "Mine", []string{"show"}, mediafiles.GoodEveryday()), 409)
	do(t, "PUT", path, ruleJSON(t, "Mine", []string{"music"}, mediafiles.LosslessFirst()), 409)
	do(t, "PUT", path, ruleJSON(t, "Mine", []string{"movie"}, mediafiles.GoodEveryday()), 200)
}

func TestPickRulesEditNeedsSettings(t *testing.T) {
	t.Parallel()
	k := newAskRig(t)
	body := ruleJSON(t, "Mine", []string{"movie"}, mediafiles.GoodEveryday())
	k.ana.call("POST", "/api/v1/pick-rules", body, 403)
	k.owner.call("PUT", fmt.Sprintf("/api/v1/users/%d", k.bo.id), `{"perms":["ask","manage_library"]}`, 200)
	k.bo.call("POST", "/api/v1/pick-rules", body, 403)
	var p pickRulesDetail
	_ = json.Unmarshal(k.owner.call("POST", "/api/v1/pick-rules", body, 201), &p)
	path := fmt.Sprintf("/api/v1/pick-rules/%d", p.ID)
	k.bo.call("GET", path, "", 200)
	k.bo.call("PUT", path, body, 403)
	k.bo.call("DELETE", path, "", 403)
	k.ana.call("GET", path, "", 403)
}

func putSettingRow(t *testing.T, st *database.Store, grp, key, value string) {
	t.Helper()
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by) VALUES (?, ?, ?, 'x', '{}', 'x', '{}')
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value`, grp, key, value)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func putOutsideList(t *testing.T, st *database.Store, folderID, rulesID int64) {
	t.Helper()
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO outside_lists(created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id, pick_rules_id)
			VALUES ('x', '{}', 'x', '{}', 'L', 'tmdb_list', '1', ?, ?)`, folderID, rulesID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

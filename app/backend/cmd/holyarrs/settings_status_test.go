package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

var statusNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

// route: GET /api/v1/settings/status
func TestSettingsStatusRoute(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	b := string(r.owner.call("GET", "/api/v1/settings/status", "", 200))
	var a struct {
		Items      []health.StatusItem `json:"items"`
		Unset      map[string][]string `json:"unset"`
		Known      []string            `json:"known"`
		MetadataUp bool                `json:"metadata_up"`
	}
	if err := json.Unmarshal([]byte(b), &a); err != nil || a.Items == nil || a.Unset == nil || a.Known == nil || !a.MetadataUp {
		t.Fatalf("answer %s", b)
	}
	// No key is ever asked for or flagged.
	if slices.Contains(pagesOf(a.Items), "/settings/tmdb") || strings.Contains(strings.ToLower(b), "key") || !slices.Contains(a.Unset["copies"], "/settings/sources") {
		t.Fatalf("answer %s", b)
	}
	r.ana.call("GET", "/api/v1/settings/status", "", 403)
	r.api.s.MetadataUp = func(context.Context) bool { return false }
	r.api.status.Forget()
	b = string(r.owner.call("GET", "/api/v1/settings/status", "", 200))
	if !strings.Contains(b, "Metadata server is down") || !strings.Contains(b, `"metadata_up":false`) {
		t.Fatalf("server down: %s", b)
	}
}

// route: GET /api/v1/metadata/health
func TestMetadataHealthRoute(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	if b := string(r.ana.call("GET", "/api/v1/metadata/health", "", 200)); !strings.Contains(b, `"up":true`) {
		t.Fatal(b)
	}
	r.api.s.MetadataUp = func(context.Context) bool { return false }
	if b := string(r.ana.call("GET", "/api/v1/metadata/health", "", 200)); !strings.Contains(b, `"up":false`) || !strings.Contains(b, "searches and new artwork will return") {
		t.Fatal(b)
	}
}

func pagesOf(items []health.StatusItem) []string {
	var out []string
	for _, i := range items {
		out = append(out, i.Page)
	}
	return out
}

func TestStatusFlagsGoneDefaults(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	var folderID int64
	_ = r.st.Read().QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&folderID)
	var p pickRulesDetail
	_ = json.Unmarshal(r.owner.call("POST", "/api/v1/pick-rules", ruleJSON(t, "Mine", []string{"movie"}, mediafiles.GoodEveryday()), 201), &p)
	r.owner.call("PUT", "/api/v1/settings/title-defaults", fmt.Sprintf(`{"movie":{"library_folder_id":%d,"pick_rules_id":%d}}`, folderID, p.ID), 200)
	if b := string(r.owner.call("GET", "/api/v1/settings/status", "", 200)); strings.Contains(b, "/settings/title-defaults\",\"text") {
		t.Fatalf("fine defaults flagged: %s", b)
	}
	// A folder and Pick rules saved and later removed (written straight to
	// the rows here; the API refuses to remove Pick rules in use).
	putSettingRow(t, r.st, "what_to_pick", "movie_pick_rules_id", "0")
	putSettingRow(t, r.st, "what_to_pick", "movie_library_folder_id", "9999")
	putSettingRow(t, r.st, "what_to_pick", "show_pick_rules_id", "8888")
	r.api.status.Forget()
	var a struct {
		Items []health.StatusItem `json:"items"`
		Known []string            `json:"known"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/settings/status", "", 200), &a)
	var got []health.StatusItem
	for _, i := range a.Items {
		if i.Page == "/settings/title-defaults" {
			got = append(got, i)
		}
	}
	want := []health.StatusItem{
		{Group: "library", Page: "/settings/title-defaults", Text: "The Library folder saved for new movies is gone, so new movies go to the first movie folder", Severity: "warning"},
		{Group: "library", Page: "/settings/title-defaults", Text: "The Pick rules saved for new shows are gone, so new shows use their folder's own Pick rules", Severity: "warning"},
	}
	if !slices.Equal(got, want) || !slices.Contains(a.Known, "/settings/title-defaults") {
		t.Fatalf("got %+v", a)
	}
}

func TestSettingsStatusCached(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	at := statusNow
	r.api.status.Now = func() time.Time { return at }
	first := string(r.owner.call("GET", "/api/v1/settings/status", "", 200))
	if !strings.Contains(first, `"/settings/sources"`) {
		t.Fatalf("first %s", first)
	}
	putSettingRow(t, r.st, "what_to_pick", "movie_library_folder_id", "9999")
	at = at.Add(29 * time.Second)
	if b := string(r.owner.call("GET", "/api/v1/settings/status", "", 200)); b != first {
		t.Fatalf("not cached: %s", b)
	}
	at = at.Add(2 * time.Second)
	if b := string(r.owner.call("GET", "/api/v1/settings/status", "", 200)); b == first || !strings.Contains(b, "/settings/title-defaults\",\"text") {
		t.Fatalf("not refreshed after 30 s: %s", b)
	}
}

// TestSettingsStatusForgetsAfterWrites: a write that can change what the
// status looks at drops the kept answer, so Settings home is right at once.
func TestSettingsStatusForgetsAfterWrites(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM library_folders")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	unsetFolders := func() bool {
		var a health.StatusAnswer
		if err := json.Unmarshal(r.owner.call("GET", "/api/v1/settings/status", "", 200), &a); err != nil {
			t.Fatal(err)
		}
		return slices.Contains(a.Unset["library"], "/settings/library-folders")
	}
	if !unsetFolders() {
		t.Fatal("no folder, yet Library folders is not marked not set up")
	}
	r.owner.call("POST", "/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"movie","label":"Movies"}`, t.TempDir()), 201)
	if unsetFolders() {
		t.Fatal("a folder was added, yet the status still says Library folders is not set up")
	}
}

// TestStatusFlagsKidWithLocalSkip: a Kid while sign-in is skipped at home
// is an error on the sign-in page; either alone is not.

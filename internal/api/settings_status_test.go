package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

var statusNow = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func check(kind, subject, state, message string) domain.HealthCheck {
	return domain.HealthCheck{Kind: kind, Subject: subject, State: state, Message: message}
}

func intp(n int) *int        { return &n }
func boolp(b bool) *bool     { return &b }
func timep(s string) *string { return &s }

func pages(a statusAnswer) []string {
	var out []string
	for _, i := range a.Items {
		out = append(out, i.Page)
	}
	slices.Sort(out)
	return out
}

func inGroup(a statusAnswer, g string) int {
	n := 0
	for _, i := range a.Items {
		if i.Group == g {
			n++
		}
	}
	return n
}

// TestSettingsStatusRules carries each case of the web app's former
// settings-status rules (settings-status.test.ts) over unchanged in meaning.
func TestSettingsStatusRules(t *testing.T) {
	t.Run("all set when every known check is fine", func(t *testing.T) {
		a := settingsStatus(statusInput{Checks: []domain.HealthCheck{check("downloader", "qBit", "ok", "")},
			Sources: []namedState{{"A", "working"}}, Downloaders: []namedState{{"Q", "working"}}, Now: statusNow})
		if len(a.Items) != 0 || !slices.Contains(a.Known, "/settings/sources") || !slices.Contains(a.Known, "/settings/downloaders") || len(a.Unset) != 0 {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("a failing downloader counts once, from the check and the list", func(t *testing.T) {
		a := settingsStatus(statusInput{Checks: []domain.HealthCheck{check("downloader", "qBit", "error", "not answering")},
			Downloaders: []namedState{{"qBit", "not_answering"}}, Sources: []namedState{{"A", "working"}}, Now: statusNow})
		if len(a.Items) != 1 || a.Items[0].Page != "/settings/downloaders" || a.Items[0].Group != "copies" ||
			a.Items[0].Text != "qBit: not answering" || a.Items[0].Severity != "error" {
			t.Fatalf("%+v", a.Items)
		}
		if inGroup(a, "copies") != 1 || !slices.Contains(a.Known, "/settings/sources") {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("a missing TMDB key, an old backup and a failed media server", func(t *testing.T) {
		a := settingsStatus(statusInput{TMDBKey: boolp(false), NewestBackup: timep("2026-10-01T00:00:00Z"),
			States: []supervisor.Status{{Key: "media_server:1", Kind: "media_server", Name: "Jelly", State: supervisor.Failed}}, Now: statusNow})
		if got := pages(a); !slices.Equal(got, []string{"/settings/backups", "/settings/media-servers", "/settings/tmdb"}) {
			t.Fatalf("pages %v", got)
		}
		if inGroup(a, "maintenance") != 1 {
			t.Fatalf("%+v", a.Items)
		}
		want := map[string]statusItem{
			"/settings/tmdb":          {Group: "library", Page: "/settings/tmdb", Text: "No TMDB key yet, so names and posters cannot be found", Severity: "warning"},
			"/settings/backups":       {Group: "maintenance", Page: "/settings/backups", Text: "No backup in the last 7 days", Severity: "warning"},
			"/settings/media-servers": {Group: "linked", Page: "/settings/media-servers", Text: "The media server Jelly failed", Severity: "error"},
		}
		for _, i := range a.Items {
			if i != want[i.Page] {
				t.Errorf("got %+v want %+v", i, want[i.Page])
			}
		}
	})
	t.Run("a recent backup is fine and no backup at all is flagged", func(t *testing.T) {
		if a := settingsStatus(statusInput{NewestBackup: timep("2026-10-17T00:00:00Z"), Now: statusNow}); len(a.Items) != 0 {
			t.Fatalf("%+v", a.Items)
		}
		a := settingsStatus(statusInput{NewestBackup: timep(""), Now: statusNow})
		if len(a.Items) != 1 || a.Items[0].Text != "No backup yet" || a.Items[0].Severity != "warning" {
			t.Fatalf("%+v", a.Items)
		}
	})
	t.Run("not set up without folders, Sources or Downloaders", func(t *testing.T) {
		a := settingsStatus(statusInput{Folders: intp(0), Sources: []namedState{}, Downloaders: []namedState{}, Now: statusNow})
		if !slices.Equal(a.Unset["library"], []string{"/settings/library-folders"}) ||
			!slices.Equal(a.Unset["copies"], []string{"/settings/sources", "/settings/downloaders"}) {
			t.Fatalf("unset %+v", a.Unset)
		}
	})
	t.Run("nothing about pages it knows nothing of", func(t *testing.T) {
		a := settingsStatus(statusInput{Now: statusNow})
		if len(a.Items) != 0 || len(a.Known) != 0 || len(a.Unset) != 0 {
			t.Fatalf("%+v", a)
		}
	})
	t.Run("severities", func(t *testing.T) {
		a := settingsStatus(statusInput{
			Checks: []domain.HealthCheck{check("disk_space", "Movies", "warning", "low on space"), check("backups", "", "error", "")},
			States: []supervisor.Status{{Key: "stage:search", Kind: "stage", Name: "search", State: supervisor.Running, Stuck: true},
				{Key: "subtitle_site:2", Kind: "subtitle_site", Name: "Subs", State: supervisor.Failed}},
			Sources:     []namedState{{"Ind", "auth_failed"}},
			UpdateNewer: boolp(true), Now: statusNow})
		want := []statusItem{
			{Group: "library", Page: "/settings/library-folders", Text: "Movies: low on space", Severity: "warning"},
			{Group: "maintenance", Page: "/settings/backups", Text: "needs a look", Severity: "error"},
			{Group: "general", Page: "/settings/parts", Text: "The stage search is stuck", Severity: "warning"},
			{Group: "library", Page: "/settings/subtitles", Text: "The subtitle site Subs failed", Severity: "error"},
			{Group: "copies", Page: "/settings/sources", Text: "The Source Ind is not answering", Severity: "error"},
			{Group: "general", Page: "/settings/this-app", Text: "A newer HolyArrs is out", Severity: "info"},
		}
		if !slices.Equal(a.Items, want) {
			t.Fatalf("got  %+v\nwant %+v", a.Items, want)
		}
	})
}

// route: GET /api/v1/settings/status
func TestSettingsStatusRoute(t *testing.T) {
	r := newAskRig(t)
	const tok = "tmdb-secret-test-token-1234567890"
	b := string(r.owner.call("GET", "/api/v1/settings/status", "", 200))
	var a struct {
		Items      []statusItem        `json:"items"`
		Unset      map[string][]string `json:"unset"`
		Known      []string            `json:"known"`
		TMDBKeySet bool                `json:"tmdb_key_set"`
	}
	if err := json.Unmarshal([]byte(b), &a); err != nil || a.Items == nil || a.Unset == nil || a.Known == nil || a.TMDBKeySet {
		t.Fatalf("answer %s", b)
	}
	if !slices.Contains(pagesOf(a.Items), "/settings/tmdb") || !slices.Contains(a.Unset["copies"], "/settings/sources") {
		t.Fatalf("answer %s", b)
	}
	r.ana.call("GET", "/api/v1/settings/status", "", 403)
	if err := r.api.s.Secrets.SetSetting(context.Background(), tmdbTokenGroup, tmdbTokenKey, tok, domain.System); err != nil {
		t.Fatal(err)
	}
	r.api.status.forget()
	b = string(r.owner.call("GET", "/api/v1/settings/status", "", 200))
	if strings.Contains(b, tok) || strings.Contains(b, "1234567890") || !strings.Contains(b, `"tmdb_key_set":true`) || strings.Contains(b, "No TMDB key") {
		t.Fatalf("with a key: %s", b)
	}
}

func pagesOf(items []statusItem) []string {
	var out []string
	for _, i := range items {
		out = append(out, i.Page)
	}
	return out
}

func TestStatusFlagsGoneDefaults(t *testing.T) {
	r := newAskRig(t)
	var folderID int64
	_ = r.st.Read().QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&folderID)
	var p pickRulesDetail
	_ = json.Unmarshal(r.owner.call("POST", "/api/v1/pick-rules", ruleJSON(t, "Mine", []string{"movie"}, domain.GoodEveryday()), 201), &p)
	r.owner.call("PUT", "/api/v1/settings/title-defaults", fmt.Sprintf(`{"movie":{"library_folder_id":%d,"pick_rules_id":%d}}`, folderID, p.ID), 200)
	if b := string(r.owner.call("GET", "/api/v1/settings/status", "", 200)); strings.Contains(b, "/settings/title-defaults\",\"text") {
		t.Fatalf("fine defaults flagged: %s", b)
	}
	// A folder and Pick rules saved and later removed (written straight to
	// the rows here; the API refuses to remove Pick rules in use).
	putSettingRow(t, r.st, "what_to_pick", "movie_pick_rules_id", "0")
	putSettingRow(t, r.st, "what_to_pick", "movie_library_folder_id", "9999")
	putSettingRow(t, r.st, "what_to_pick", "show_pick_rules_id", "8888")
	r.api.status.forget()
	var a struct {
		Items []statusItem `json:"items"`
		Known []string     `json:"known"`
	}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/settings/status", "", 200), &a)
	var got []statusItem
	for _, i := range a.Items {
		if i.Page == "/settings/title-defaults" {
			got = append(got, i)
		}
	}
	want := []statusItem{
		{Group: "library", Page: "/settings/title-defaults", Text: "The Library folder saved for new movies is gone, so new movies go to the first movie folder", Severity: "warning"},
		{Group: "library", Page: "/settings/title-defaults", Text: "The Pick rules saved for new shows are gone, so new shows use their folder's own Pick rules", Severity: "warning"},
	}
	if !slices.Equal(got, want) || !slices.Contains(a.Known, "/settings/title-defaults") {
		t.Fatalf("got %+v", a)
	}
}

func TestSettingsStatusCached(t *testing.T) {
	r := newAskRig(t)
	at := statusNow
	r.api.status.now = func() time.Time { return at }
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
	r := newAskRig(t)
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM library_folders")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	unsetFolders := func() bool {
		var a statusAnswer
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

package importer

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

const (
	webDL   = "Dune.Part.Two.2024.1080p.AMZN.WEB-DL.DDP5.1.H264-R3MiX"
	bluRay  = "Dune.Part.Two.2024.1080p.BluRay.x264-YTS"
	web720  = "Dune.Part.Two.2024.720p.AMZN.WEB-DL.DDP5.1.H264-R3MiX"
	groupWB = `[["1080p_web","1080p_bluray"]]`
)

// importOver imports file over a copy in place named old, with the Version's
// Pick rules given equal steps (none when equal is "").
func importOver(t *testing.T, file, old, equal string) (mediafiles.Import, string) {
	t.Helper()
	w, relID, _ := upgradeWorld(t, file, file, old)
	ctx := context.Background()
	if equal != "" {
		err := w.s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(`UPDATE pick_rules SET body = json_set(body, '$.equal_steps', json(?), '$.replace_under_min', json('true')) WHERE name = ?`,
				equal, mediafiles.GoodEverydayName)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	imp, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var rel domain.Release
	_ = database.Get(ctx, w.s.Read(), "releases", relID, &rel)
	why, _ := rel.Verdict["turned_down_reason"].(string)
	return *imp, why
}

// The Pick rules rank 1080p WEB and BluRay as one step and chose a WEB
// to replace a small BluRay; the import must not call it lower.
func TestImportHonoursEqualSteps(t *testing.T) {
	if imp, why := importOver(t, webDL, bluRay, groupWB); imp.State != "done" || imp.Plan[0].ReplacesFileID == nil {
		t.Fatalf("import %s, turned down %q", imp.State, why)
	}
	if imp, why := importOver(t, webDL, bluRay, ""); imp.State != "rejected" || !strings.Contains(why, "lower than the copy in place") {
		t.Fatalf("without equal steps: import %s, turned down %q", imp.State, why)
	}
}

// The import check ranks steps as judging does, for rules made here and for
// an old app's rules alike: a step in the group is not lower than another in
// it, and a step outside the group still is.
func TestLieOfRanksEqualSteps(t *testing.T) {
	facts := map[string]any{"width": 1920, "height": 1080}
	small := map[string]any{"width": 1280, "height": 720}
	group := mediafiles.GoodEveryday()
	group.EqualSteps = [][]string{{"1080p_web", "1080p_bluray"}}
	oldApp := group
	oldApp.Synced = &mediafiles.SyncedProfile{From: "import", App: "radarr"}
	cases := []struct {
		name      string
		rules     mediafiles.PickRulesBody
		file, old string
		facts     map[string]any
		lower     bool
	}{
		{"no group, web over bluray", mediafiles.GoodEveryday(), webDL, bluRay, facts, true},
		{"group, web over bluray", group, webDL, bluRay, facts, false},
		{"group, bluray over web", group, bluRay, webDL, facts, false},
		{"group, 720p web over bluray", group, web720, bluRay, small, true},
		{"old app group, web over bluray", oldApp, webDL, bluRay, facts, false},
		{"old app group, 720p web over bluray", oldApp, web720, bluRay, small, true},
	}
	for _, c := range cases {
		why := lieOf(c.file, c.file, true, c.old, c.rules, c.facts)
		if got := strings.Contains(why, "lower than the copy in place"); got != c.lower {
			t.Errorf("%s: lower %v, want %v (%q)", c.name, got, c.lower, why)
		}
	}
}

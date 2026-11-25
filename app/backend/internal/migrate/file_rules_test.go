package migrate

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// TestUnmonitorDeletedIsRead: the "unmonitor deleted" switch is read from
// /config/mediamanagement or from a backup's Config table.
func TestUnmonitorDeletedIsRead(t *testing.T) {
	u, key := radarrAPIWith(t, map[string]any{"/api/v3/config/mediamanagement": map[string]any{"id": 1, "autoUnmonitorPreviouslyDownloadedMovies": true}})
	if s := read(t, api(Radarr, u, key)); s.Settings["unkeep_deleted"] != true {
		t.Fatalf("radarr api %+v", s.Settings)
	}
	u, key = sonarrAPIWith(t, false, map[string]any{"/api/v3/config/mediamanagement": map[string]any{"id": 1, "autoUnmonitorPreviouslyDownloadedEpisodes": false}})
	if s := read(t, api(Sonarr, u, key)); s.Settings["unkeep_deleted"] != false {
		t.Fatalf("sonarr api %+v", s.Settings)
	}
	p := filepath.Join(t.TempDir(), "sonarr.db")
	makeDB(t, p, append(sonarrDB(false), `CREATE TABLE Config (Id INTEGER PRIMARY KEY, Key TEXT, Value TEXT)`,
		`INSERT INTO Config VALUES (1, 'autounmonitorpreviouslydownloadedepisodes', 'True')`)...)
	if s := read(t, file(Sonarr, p)); s.Settings["unkeep_deleted"] != true {
		t.Fatalf("sonarr backup %+v", s.Settings)
	}
}

// TestUnmonitorDeletedPlan: a switch that is on becomes a Naming item for
// its kind; off matches this server's default and is only mapped.
func TestUnmonitorDeletedPlan(t *testing.T) {
	on := &Snapshot{Kind: Sonarr, Settings: map[string]any{"unkeep_deleted": true}}
	off := &Snapshot{Kind: Radarr, Settings: map[string]any{"unkeep_deleted": false}}
	p := BuildPlan([]Input{{1, on}, {2, off}}, Choices{})
	var rules []Item // the names choice items sit in this group too
	for _, it := range p.Group(GroupNaming).Items {
		if it.FileRules != nil {
			rules = append(rules, it)
		}
	}
	if len(rules) != 1 || *rules[0].FileRules != (FileRulesPlan{Kind: "show", UnkeepDeleted: true}) {
		t.Fatalf("items %+v", rules)
	}
	if !accounted(p, "radarr settings.unkeep_deleted") {
		t.Fatal("radarr's off switch is not accounted for")
	}
	for _, n := range p.NotMoved {
		if n.What == "unkeep_deleted" {
			t.Fatalf("listed as not moved: %+v", n)
		}
	}
}

// TestMoveSetsUnkeepDeleted: the move turns the switch on and undo puts it
// back; a switch changed by hand since stays.
func TestMoveSetsUnkeepDeleted(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Settings = map[string]any{"unkeep_deleted": true}
	})
	ctx := context.Background()
	mv := l.move(t, Choices{})
	if got := lineOf(t, mv.Report, "file_rules:unkeep_deleted:movie"); got.Result != ResChanged {
		t.Fatalf("line %+v", got)
	}
	r, _ := importer.ReadFileRules(ctx, l.st.Read())
	if !r.UnkeepDeletedMovies || r.UnkeepDeletedEpisodes {
		t.Fatalf("rules %+v", r)
	}
	if u := l.undo(t, mv.ID); lineOf(t, u.Report, "file_rules:unkeep_deleted:movie").Result != ResRemoved {
		t.Fatalf("undo %+v", u.Report)
	}
	if r, _ := importer.ReadFileRules(ctx, l.st.Read()); r.UnkeepDeletedMovies {
		t.Fatal("undo left the switch on")
	}
}

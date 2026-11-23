package migrate

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The owner's names choice, not the old app's rename setting, decides
// whether later imports rename: "keep" (the default) turns it off for every
// part an app brings, "rename" turns it on. Nothing on disk changes either way.
func TestNamesChoiceSetsRenameOnImport(t *testing.T) {
	sonarrOn := &Snapshot{Kind: Sonarr, Naming: map[string]any{"renameEpisodes": true}}
	radarrOff := &Snapshot{Kind: Radarr, Naming: map[string]any{"renameMovies": false}}
	radarrNone := &Snapshot{Kind: Radarr}
	lidarrOn := &Snapshot{Kind: Lidarr, Naming: map[string]any{"renameTracks": true}}
	cases := []struct {
		name  string
		snaps []*Snapshot
		names string
		want  map[string]string // names:<kind> -> value
	}{
		{"default keeps, even when Sonarr renamed", []*Snapshot{sonarrOn}, "", map[string]string{"names:sonarr": "false"}},
		{"keep", []*Snapshot{sonarrOn, lidarrOn}, NamesKeep, map[string]string{"names:sonarr": "false", "names:lidarr": "false"}},
		{"rename, even when Radarr did not", []*Snapshot{radarrOff}, NamesRename, map[string]string{"names:radarr": "true"}},
		{"keep with no naming sent", []*Snapshot{radarrNone}, "", map[string]string{"names:radarr": "false"}},
		{"rename with no naming sent", []*Snapshot{radarrNone}, NamesRename, map[string]string{"names:radarr": "true"}},
		{"not an app with files", []*Snapshot{{Kind: Prowlarr}}, NamesRename, map[string]string{}},
	}
	part := map[string]string{"names:sonarr": "show_", "names:radarr": "", "names:lidarr": "music_"}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var in []Input
			for i, s := range c.snaps {
				in = append(in, Input{ID: int64(i + 1), Snap: s})
			}
			p := BuildPlan(in, Choices{Names: c.names})
			got := 0
			for _, g := range p.Groups {
				for _, it := range g.Items {
					if it.Naming == nil {
						continue
					}
					for _, pre := range []string{"", "show_", "music_"} {
						if _, ok := it.Naming[pre+"rename_on_import"]; ok && c.want[it.Key] == "" {
							t.Fatalf("%s carries %srename_on_import: %v", it.Key, pre, it.Naming)
						}
					}
				}
			}
			for key, v := range c.want {
				it := item(t, p, GroupNaming, key)
				if it.Naming[part[key]+"rename_on_import"] != v || len(it.Naming) != 1 {
					t.Fatalf("%s = %v, want %s", key, it.Naming, v)
				}
				got++
			}
			if got != len(c.want) {
				t.Fatalf("got %d names items", got)
			}
		})
	}
}

// A bad names value is refused at the door.
func TestNamesChoiceValid(t *testing.T) {
	for v, ok := range map[string]bool{"": true, NamesKeep: true, NamesRename: true, "move": false, "KEEP": false} {
		if got := validNames(v); got != ok {
			t.Fatalf("validNames(%q) = %v", v, got)
		}
	}
}

// Leaving Naming out still applies the names choice: keep never leaves
// HolyArrs's own rename-on default in place for a moved app. A second move
// changes nothing and undo puts it back.
func TestNamesChoiceAppliesWhenNamingLeftOut(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, _, _ *Snapshot) {
		radarr.Naming = map[string]any{"renameMovies": true, "colonReplacementFormat": "dash"}
	})
	ctx := context.Background()
	mv := l.move(t, Choices{Skip: []string{GroupNaming}})
	if got := lineOf(t, mv.Report, "names:radarr"); got.Result != ResChanged {
		t.Fatalf("names line: %+v", got)
	}
	if got := lineOf(t, mv.Report, "naming:radarr"); got.Result != ResSkipped {
		t.Fatalf("naming line: %+v", got)
	}
	ns, _ := mediafiles.ReadNaming(ctx, l.st.Read())
	if ns.RenameOnImport || ns.Colon != mediafiles.NamingDefaults().Colon {
		t.Fatalf("after keep: %+v", ns)
	}
	again := l.move(t, Choices{Skip: []string{GroupNaming}})
	if got := lineOf(t, again.Report, "names:radarr"); got.Result != ResAlready {
		t.Fatalf("second move: %+v", got)
	}
	l.undo(t, mv.ID)
	if ns, _ := mediafiles.ReadNaming(ctx, l.st.Read()); ns != mediafiles.NamingDefaults() {
		t.Fatalf("undo: %+v", ns)
	}
}

// Undoing the names item puts back only rename on import: a file pattern
// changed after the move stays.
func TestNamesUndoKeepsOtherNamingChanges(t *testing.T) {
	l := newMoveLab(t)
	mv := l.move(t, Choices{})
	ctx := context.Background()
	ns, _ := mediafiles.ReadNaming(ctx, l.st.Read())
	ns.FilePattern = "{title} mine"
	if err := mediafiles.PutNamingSettings(ctx, l.st, ns, domain.System); err != nil {
		t.Fatal(err)
	}
	u := l.undo(t, mv.ID)
	if got := lineOf(t, u.Report, "names:radarr"); got.Result != ResRemoved {
		t.Fatalf("names undo: %+v", got)
	}
	got, _ := mediafiles.ReadNaming(ctx, l.st.Read())
	if got.FilePattern != "{title} mine" || got.RenameOnImport != mediafiles.NamingDefaults().RenameOnImport {
		t.Fatalf("after undo: %+v", got)
	}
}

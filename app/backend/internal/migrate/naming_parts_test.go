package migrate

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Each app's rename, colon and character rules go to its own part,
// so the order the apps are read in changes nothing.
func TestNamingGoesToEachAppsPart(t *testing.T) {
	sonarr := &Snapshot{Kind: Sonarr, Naming: map[string]any{"renameEpisodes": true, "colonReplacementFormat": 4.0, "replaceIllegalCharacters": true,
		"specialsFolderFormat": "Extras"}}
	radarr := &Snapshot{Kind: Radarr, Naming: map[string]any{"renameMovies": false, "colonReplacementFormat": "dash", "replaceIllegalCharacters": false}}
	lidarr := &Snapshot{Kind: Lidarr, Naming: map[string]any{"renameTracks": false, "colonReplacementFormat": 0.0,
		"standardTrackFormat":  "{Album Title} ({Release Year})/{Artist Name} - {track:00} - {Track Title}",
		"multiDiscTrackFormat": "{Album Title} ({Release Year})/Disc {medium:0}/{track:00} {Track Title}", "artistFolderFormat": "{Artist NameThe}"}}
	want := map[string]map[string]string{
		"naming:sonarr": {"show_colon": "smart", "show_illegal": "replace", "specials_folder": "Extras"},
		"naming:radarr": {"colon": "dash", "illegal": "remove"},
		"naming:lidarr": {"music_colon": "remove", "album_folder_pattern": "{album} ({year})",
			"track_pattern": "{artist} - {track} - {track_title}", "disc_folder_pattern": "Disc {disc}", "disc_folders": "true",
			"multi_disc_track_pattern": "{track} {track_title}", "artist_folder_pattern": "{artist_sort}"},
	}
	for _, order := range [][]*Snapshot{{sonarr, radarr, lidarr}, {lidarr, radarr, sonarr}, {radarr}, {sonarr}} {
		var in []Input
		for i, s := range order {
			in = append(in, Input{ID: int64(i + 1), Snap: s})
		}
		p := BuildPlan(in, Choices{})
		for _, s := range order {
			key := "naming:" + string(s.Kind)
			got := item(t, p, GroupNaming, key).Naming
			if len(got) != len(want[key]) {
				t.Fatalf("%s: %v", key, got)
			}
			for k, v := range want[key] {
				if got[k] != v {
					t.Fatalf("%s %s = %q, want %q (%v)", key, k, got[k], v, got)
				}
			}
		}
	}
	// A colon choice with no match (a custom text) is listed, not moved.
	p := BuildPlan([]Input{{ID: 1, Snap: &Snapshot{Kind: Sonarr, Naming: map[string]any{"colonReplacementFormat": 5.0, "renameEpisodes": true, "specialsFolderFormat": "Extras"}}}}, Choices{})
	if _, ok := item(t, p, GroupNaming, "naming:sonarr").Naming["show_colon"]; ok {
		t.Fatal("custom colon moved")
	}
	if _, ok := notMoved(p, "sonarr naming.colonReplacementFormat"); !ok {
		t.Fatalf("custom colon not listed: %+v", p.NotMoved)
	}
}

// Sonarr and Radarr disagree; after the move each part keeps its own app's
// rules, a second move changes nothing and undo puts every part back.
func TestMoveNamingFromTwoAppsThatDisagree(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, bazarr, _ *Snapshot) {
		radarr.Naming = map[string]any{"renameMovies": false, "colonReplacementFormat": "dash", "replaceIllegalCharacters": false}
		*bazarr = Snapshot{Kind: Sonarr, Naming: map[string]any{"renameEpisodes": true, "colonReplacementFormat": 2.0, "replaceIllegalCharacters": true}}
	})
	ctx := context.Background()
	rep := l.mv.Run(ctx, 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if lineOf(t, rep, "naming:sonarr").Result != ResChange || lineOf(t, rep, "naming:radarr").Result != ResChange {
		t.Fatal("dry run lines")
	}
	if ns, _ := mediafiles.ReadNaming(ctx, l.st.Read()); ns != mediafiles.NamingDefaults() {
		t.Fatalf("dry run changed naming: %+v", ns)
	}
	mv := l.move(t, Choices{})
	ns, _ := mediafiles.ReadNaming(ctx, l.st.Read())
	if ns.RenameOnImport || ns.Colon != mediafiles.ColonDash || ns.Illegal != mediafiles.IllegalRemove {
		t.Fatalf("movies: %+v", ns)
	}
	// Rename on import is off for each app moved: the names choice defaults to keep.
	if ns.ShowRenameOnImport || ns.ShowColon != mediafiles.ColonSpaceDash || ns.ShowIllegal != mediafiles.IllegalReplace {
		t.Fatalf("shows: %+v", ns)
	}
	if !ns.MusicRenameOnImport || ns.MusicColon != mediafiles.ColonSmart || ns.MusicIllegal != mediafiles.IllegalReplace {
		t.Fatalf("music changed: %+v", ns)
	}
	again := l.move(t, Choices{})
	for _, key := range []string{"naming:sonarr", "naming:radarr", "names:sonarr", "names:radarr"} {
		if got := lineOf(t, again.Report, key); got.Result != ResAlready {
			t.Fatalf("second move %s: %+v", key, got)
		}
	}
	l.undo(t, mv.ID)
	if ns, _ := mediafiles.ReadNaming(ctx, l.st.Read()); ns != mediafiles.NamingDefaults() {
		t.Fatalf("undo: %+v", ns)
	}
}

// A word from another part (a Lidarr {Quality Title}, a music word in a
// Radarr pattern) is listed as no match; the rest of that app's naming,
// its rename rule included, still moves and applies.
func TestNamingWordFromOtherPartListedNotFailed(t *testing.T) {
	l := newMoveLabWith(t, func(radarr, bazarr, _ *Snapshot) {
		radarr.Naming = map[string]any{"renameMovies": false, "standardMovieFormat": "{Movie Title} {Album Title}"}
		*bazarr = Snapshot{Kind: Lidarr, Naming: map[string]any{"renameTracks": false, "colonReplacementFormat": 1.0,
			"standardTrackFormat": "{Artist Name} - {track:00} - {Track Title} {Quality Title}", "artistFolderFormat": "{Artist Name} ({Release Year})"}}
	})
	p := l.plan(t)
	for _, k := range []string{"radarr naming.standardMovieFormat", "lidarr naming.standardTrackFormat"} {
		if _, ok := notMoved(p, k); !ok {
			t.Fatalf("%s not listed: %+v", k, p.NotMoved)
		}
	}
	mv := l.move(t, Choices{})
	for _, k := range []string{"names:radarr", "naming:lidarr"} {
		if got := lineOf(t, mv.Report, k); got.Result != ResChanged {
			t.Fatalf("%s: %+v", k, got)
		}
	}
	ns, _ := mediafiles.ReadNaming(context.Background(), l.st.Read())
	if ns.RenameOnImport || ns.MusicRenameOnImport || ns.MusicColon != mediafiles.ColonDash || !ns.ShowRenameOnImport ||
		ns.ArtistFolderPattern != "{artist} ({year})" || ns.TrackPattern != mediafiles.NamingDefaults().TrackPattern {
		t.Fatalf("naming: %+v", ns)
	}
}

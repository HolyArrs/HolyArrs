package importer

import (
	"errors"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/naming"
	"github.com/CoderLuii/holyarrs/internal/tags"
)

// 4.15.1: tags settle only what a name leaves open; a name that settles a
// track is never second-guessed by tags, and tags whose title names
// another track are not used.
func TestPlanTracksTagsOnlyWhenNamesAreAmbiguous(t *testing.T) {
	read := map[string]tags.Tags{
		"/dl/track.flac":     {Track: 2, Title: "Dawn"},           // no number or name in the file name
		"/dl/Intro (a).flac": {Disc: 2, Track: 1, Title: "Intro"}, // two tracks named Intro
		"/dl/03 - Noon.flac": {Track: 3},                          // its name settles it
		"/dl/misc.flac":      {Track: 3, Title: "Something else"}, // tag title is not the track's
		"/dl/untagged.flac":  {},
	}
	var asked []string
	at := AlbumTarget{LibraryPath: "/lib", ArtistFolder: "A", Album: "B", Tracks: []Track{
		{ItemID: 1, Disc: 1, Number: 1, Name: "Intro", SlotID: 11, Wanted: true},
		{ItemID: 2, Disc: 1, Number: 2, Name: "Dawn", SlotID: 12, Wanted: true},
		{ItemID: 3, Disc: 1, Number: 3, Name: "Noon", SlotID: 13, Wanted: true},
		{ItemID: 4, Disc: 2, Number: 1, Name: "Intro", SlotID: 21, Wanted: true},
	}, Tags: func(p string) (tags.Tags, error) {
		asked = append(asked, p)
		tg := read[p]
		if tg.Empty() {
			return tg, errors.New("no tags")
		}
		return tg, nil
	}}
	cands := []Candidate{{Path: "/dl/track.flac", Size: 5}, {Path: "/dl/Intro (a).flac", Size: 5}, {Path: "/dl/03 - Noon.flac", Size: 4},
		{Path: "/dl/misc.flac", Size: 3}, {Path: "/dl/untagged.flac", Size: 3}}
	steps := PlanTracks(cands, at)
	got := map[string][]int64{}
	for _, s := range steps {
		if s.Method == "hardlink" {
			got[s.SourceFile] = s.TargetSlotIDs
		}
	}
	if ids := got["/dl/track.flac"]; len(ids) != 1 || ids[0] != 12 {
		t.Fatalf("bare name: %+v", steps)
	}
	if ids := got["/dl/Intro (a).flac"]; len(ids) != 1 || ids[0] != 21 {
		t.Fatalf("same name on two discs: %+v", steps)
	}
	if _, ok := got["/dl/misc.flac"]; ok {
		t.Fatal("a tag title naming another track was used")
	}
	if _, ok := got["/dl/untagged.flac"]; ok {
		t.Fatal("an untagged file was placed")
	}
	// "03 - Noon" on an album of two discs is Noon by its name; its tags
	// (track 3) are never read.
	if ids := got["/dl/03 - Noon.flac"]; len(ids) != 1 || ids[0] != 13 {
		t.Fatalf("named file: %+v", steps)
	}
	for _, p := range asked {
		if p == "/dl/03 - Noon.flac" {
			t.Fatal("tags read for a file its name settled")
		}
	}
}

// With no tag reader the plan is the file-name plan of 4.14.
func TestPlanTracksNoTagReader(t *testing.T) {
	at := AlbumTarget{LibraryPath: "/lib", ArtistFolder: "A", Album: "B", Naming: func() *naming.Settings { s := naming.Defaults(); return &s }(),
		Tracks: []Track{{ItemID: 1, Disc: 1, Number: 1, Name: "Intro", SlotID: 11, Wanted: true}}}
	steps := PlanTracks([]Candidate{{Path: "/dl/track.flac", Size: 1}}, at)
	if steps[0].Method != "skip" {
		t.Fatalf("%+v", steps)
	}
}

// Rename on import off keeps each track's own file name inside the
// pattern's album folder.
func TestTrackFileKeepNames(t *testing.T) {
	at := AlbumTarget{ArtistFolder: "A", Album: "B", Year: 2001, KeepNames: true}
	if p := at.trackFile(Track{Disc: 1, Number: 2, Name: "Song"}, ".MP3", 1, "/dl/02_song_final.MP3"); p != "A/B (2001)/02_song_final.mp3" {
		t.Fatalf("%q", p)
	}
}

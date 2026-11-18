package tags

import (
	"bytes"
	"testing"
)

func TestMusicBrainzIDs(t *testing.T) {
	const art, rg, rel, rec = "a1b2c3d4-0000-4000-8000-000000000001", "a1b2c3d4-0000-4000-8000-000000000002",
		"a1b2c3d4-0000-4000-8000-000000000003", "a1b2c3d4-0000-4000-8000-000000000004"
	want := Tags{Title: "Tide", Artist: "Lab Lanterns", AlbumArtist: "Lab Lanterns", Album: "Small Hours", Track: 3,
		ArtistID: art + " a1b2c3d4-0000-4000-8000-00000000000f", ReleaseGroupID: rg, ReleaseID: rel, RecordingID: rec}
	got, err := Read(bytes.NewReader(flac("TITLE=Tide", "ARTIST=Lab Lanterns", "ALBUMARTIST=Lab Lanterns", "ALBUM=Small Hours", "TRACKNUMBER=3",
		"MUSICBRAINZ_ARTISTID="+art+"; a1b2c3d4-0000-4000-8000-00000000000f", "MUSICBRAINZ_RELEASEGROUPID="+rg, "MUSICBRAINZ_ALBUMID="+rel,
		"MUSICBRAINZ_TRACKID="+rec)))
	if err != nil || got != want {
		t.Fatalf("flac: %+v %v", got, err)
	}
	got, err = Read(bytes.NewReader(append(id3(3, [2]string{"TIT2", "Tide"}, [2]string{"TPE1", "Lab Lanterns"}, [2]string{"TPE2", "Lab Lanterns"},
		[2]string{"TALB", "Small Hours"}, [2]string{"TRCK", "3"}, [2]string{"TXXX", "MusicBrainz Artist Id\x00" + art},
		[2]string{"TXXX", "MusicBrainz Release Group Id\x00" + rg}, [2]string{"TXXX", "MusicBrainz Album Id\x00" + rel},
		[2]string{"TXXX", "MusicBrainz Release Track Id\x00ignored"}), 0xFF, 0xFB)))
	want.RecordingID, want.ArtistID = "", art
	if err != nil || got != want {
		t.Fatalf("id3: %+v %v", got, err)
	}
	// Anything not shaped like an id is dropped.
	got, _ = Read(bytes.NewReader(flac("TITLE=x", "MUSICBRAINZ_ALBUMID=not an id")))
	if got.ReleaseID != "" {
		t.Fatalf("bad id kept: %+v", got)
	}
}

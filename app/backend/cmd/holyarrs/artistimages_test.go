package main

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestArtistImagesSource: the direct artist profile source is off by
// default (the metadata server answers), Deezer's public API only for
// developers (HOLYARRS_DIRECT_SOURCES=1), a stand-in when
// HOLYARRS_DEEZER_URL names one, and off with "off" or when MusicBrainz is
// a stand-in (tests) and no Deezer stand-in is named, so tests never reach
// the real Deezer.
func TestArtistImagesSource(t *testing.T) {
	t.Setenv("HOLYARRS_DEEZER_URL", "")
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "")
	t.Setenv("HOLYARRS_DIRECT_SOURCES", "")
	if d := artistImages(); d != nil {
		t.Fatalf("on by default %+v", d)
	}
	t.Setenv("HOLYARRS_DIRECT_SOURCES", "1")
	d := artistImages()
	if d == nil || d.BaseURL != metadata.DeezerBase {
		t.Fatalf("developer stand-in %+v", d)
	}
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "http://127.0.0.1:18739")
	if d := artistImages(); d != nil {
		t.Fatalf("with a MusicBrainz stand-in %+v", d)
	}
	t.Setenv("HOLYARRS_DEEZER_URL", "http://127.0.0.1:18750/")
	if d = artistImages(); d == nil || d.BaseURL != "http://127.0.0.1:18750" {
		t.Fatalf("stand-in %+v", d)
	}
	t.Setenv("HOLYARRS_DEEZER_URL", "off")
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "")
	if d := artistImages(); d != nil {
		t.Fatalf("off %+v", d)
	}
}

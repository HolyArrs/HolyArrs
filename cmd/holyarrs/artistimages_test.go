package main

import (
	"slices"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// TestArtistImagesSource: the artist picture source is Deezer's public
// search by default, a stand-in when HOLYARRS_DEEZER_URL names one, and off
// with "off" or when MusicBrainz is a stand-in (tests) and no Deezer
// stand-in is named, so tests never reach the real Deezer.
func TestArtistImagesSource(t *testing.T) {
	t.Setenv("HOLYARRS_DEEZER_URL", "")
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "")
	d, allowed := artistImages()
	if d == nil || d.BaseURL != metadata.DeezerBase || !slices.Contains(allowed, "https://cdn-images.dzcdn.net/images/artist/") {
		t.Fatalf("default %+v %v", d, allowed)
	}
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "http://127.0.0.1:18739")
	if d, allowed := artistImages(); d != nil || allowed != nil {
		t.Fatalf("with a MusicBrainz stand-in %+v %v", d, allowed)
	}
	t.Setenv("HOLYARRS_DEEZER_URL", "http://127.0.0.1:18750/")
	d, allowed = artistImages()
	if d == nil || d.BaseURL != "http://127.0.0.1:18750" || !slices.Equal(allowed, []string{"http://127.0.0.1:18750/"}) {
		t.Fatalf("stand-in %+v %v", d, allowed)
	}
	t.Setenv("HOLYARRS_DEEZER_URL", "off")
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "")
	if d, _ := artistImages(); d != nil {
		t.Fatalf("off %+v", d)
	}
}

package main

import (
	"context"
	"slices"
	"strings"
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

// TestArtistPictureSources: TheAudioDB and the Cover Art Archive are on by
// default (TheAudioDB asked only with a key, from HOLYARRS_THEAUDIODB_KEY
// here), off like Deezer in tests, and stand-ins when named.
func TestArtistPictureSources(t *testing.T) {
	t.Setenv("HOLYARRS_DEEZER_URL", "")
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "")
	t.Setenv("HOLYARRS_THEAUDIODB_URL", "")
	t.Setenv("HOLYARRS_COVERART_URL", "")
	t.Setenv("HOLYARRS_THEAUDIODB_KEY", "")
	ctx := context.Background()
	adb, caa, hosts := artistPictureSources(nil, "test")
	if adb == nil || adb.BaseURL != metadata.AudioDBBase || adb.On(ctx) || caa != "https://coverartarchive.org" ||
		!slices.Contains(hosts, "https://*.archive.org/") || !strings.HasPrefix(adb.UserAgent, "HolyArrs/") {
		t.Fatalf("default %+v %q %v", adb, caa, hosts)
	}
	t.Setenv("HOLYARRS_THEAUDIODB_KEY", "lab-key")
	if !adb.On(ctx) {
		t.Fatal("key from the environment not used")
	}
	t.Setenv("HOLYARRS_MUSICBRAINZ_URL", "http://127.0.0.1:18739")
	if adb, caa, hosts := artistPictureSources(nil, "test"); adb != nil || caa != "" || hosts != nil {
		t.Fatalf("with a MusicBrainz stand-in %+v %q %v", adb, caa, hosts)
	}
	t.Setenv("HOLYARRS_THEAUDIODB_URL", "http://127.0.0.1:18751/")
	t.Setenv("HOLYARRS_COVERART_URL", "http://127.0.0.1:18752")
	adb, caa, hosts = artistPictureSources(nil, "test")
	if adb == nil || adb.BaseURL != "http://127.0.0.1:18751" || caa != "http://127.0.0.1:18752" ||
		!slices.Equal(hosts, []string{"http://127.0.0.1:18751/", "http://127.0.0.1:18752/"}) {
		t.Fatalf("stand-ins %+v %q %v", adb, caa, hosts)
	}
}

package library

import "testing"

// Review cases: extras naming styles, sample files, Plex extras
// folders and real movies whose names look like extras.
func TestAdvMovieExtra(t *testing.T) {
	for rel, want := range map[string]bool{
		// Plex/Jellyfin extras folders
		"Behind The Scenes/a.mkv": true, "Deleted Scenes/a.mkv": true, "Featurettes/a.mkv": true,
		"Interviews/a.mkv": true, "Scenes/a.mkv": true, "Shorts/a.mkv": true, "Trailers/a.mkv": true,
		"Other/a.mkv": true, "Extras/a.mkv": true, "extras/a.mkv": true, "EXTRAS/a.mkv": true,
		"Behind_The_Scenes/a.mkv": true, "Sample/a.mkv": true, "Specials/a.mkv": true,
		// Plex name suffixes, any case
		"Heat (1995)-Trailer.mkv": true, "Heat (1995)-deleted.mkv": true, "Heat (1995)-interview.mkv": true,
		"Heat (1995)-scene.mkv": true, "Heat (1995)-short.mkv": true, "Heat (1995)-other.mkv": true,
		"Heat (1995)-featurette.MP4": true,
		// other separators before the extras word
		"Heat (1995) - Trailer.mkv": true, "Heat.1995.trailer.mkv": true, "Heat_trailer.mkv": true,
		"sample.mkv": true, "heat.1995.1080p-sample.mkv": true, "Heat (1995).Featurette.mkv": true,
		"The.Big.Short.mkv": false, "The_Big_Short.mkv": false, "The.Other.Guys.mkv": false, "Heat.short.mkv": false,
		// real movies that must stay movies
		"Heat (1995).mkv": false, "Trailer Park Boys (2006).mkv": false, "The Other (1972).mkv": false,
		"Short Circuit (1986).mkv": false, "The Interview (2014).mkv": false, "Deleted (2020).mkv": false,
		"BDMV/STREAM/00000.m2ts": false, "VIDEO_TS/VTS_01_1.VOB": false, "CD1/heat.avi": false,
		"Feature/heat.mkv": false,
		// dash-joined titles stay movies
		"The-Other.mkv": false, "The-Interview.mkv": false, "Short-Circuit-1986.mkv": false,
		"Trailer-Park-Boys-The-Movie.mkv": false, "Deleted.mkv": false, "The.Interview.2014.mkv": false,
	} {
		if got := MovieExtra(rel); got != want {
			t.Errorf("MovieExtra(%q) = %v, want %v", rel, got, want)
		}
	}
}

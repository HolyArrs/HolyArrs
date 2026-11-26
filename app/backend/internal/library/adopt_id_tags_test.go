package library

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Movie folder id tags as Radarr names them give an exact match before
// any name search; an unknown id falls back to the name.
func TestMovieFolderIDTags(t *testing.T) {
	md := &metadata.Static{Movies: []metadata.Movie{
		{Name: "Heat", Year: 1995, IDs: map[string]string{"tmdb": "949", "imdb": "tt0113277"}},
		{Name: "Heat Wave", Year: 1995, IDs: map[string]string{"tmdb": "5555"}},
	}}
	for _, folder := range []string{"Heat (1995) {tmdb-949}", "Heat (1995) [tmdbid-949]", "Heat (1995) {imdb-tt0113277}", "Heat (1995) [imdbid-tt0113277]", "Heat (1995) [imdb-0113277]"} {
		_, _, ms, err := matchFolder(context.Background(), md, folder)
		if err != nil || len(ms) != 1 || ms[0].TMDB != "949" || ms[0].Score != 1 {
			t.Fatalf("%s: %+v %v", folder, ms, err)
		}
	}
	for _, folder := range []string{"Heat Wave (1995) {tmdb-123456}", "Heat Wave (1995) [imdb-tt9999999]", "Heat Wave (1995) [tmdb-tt0113277]"} {
		name, year, ms, err := matchFolder(context.Background(), md, folder)
		if err != nil || name != "Heat Wave" || year != 1995 || len(ms) == 0 || ms[0].TMDB != "5555" {
			t.Fatalf("%s fallback: %q %d %+v %v", folder, name, year, ms, err)
		}
	}
}

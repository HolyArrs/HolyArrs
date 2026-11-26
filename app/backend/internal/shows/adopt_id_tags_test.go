package shows

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// tagShows answers lookups by id and by name, and records the ids asked.
type tagShows struct {
	byID  map[string]metadata.Show
	asked []string
}

func (f *tagShows) SearchShows(_ context.Context, q string) ([]metadata.Show, error) {
	return []metadata.Show{{Name: "Breaking Bad", Year: 2008, Source: "tvdb", IDs: map[string]string{"tvdb": "81189"}}}, nil
}

func (f *tagShows) Show(_ context.Context, id string) (metadata.Show, error) {
	f.asked = append(f.asked, id)
	if sh, ok := f.byID[id]; ok {
		return sh, nil
	}
	return metadata.Show{}, metadata.ErrShowNotFound
}

// Folder id tags as Sonarr names them give an exact match before any
// name search; an unknown id falls back to the name.
func TestShowFolderIDTags(t *testing.T) {
	lost := metadata.Show{Name: "Lost", Year: 2004, Source: "tvdb", IDs: map[string]string{"tvdb": "73739", "tmdb": "4607", "imdb": "tt0411008"}}
	f := &tagShows{byID: map[string]metadata.Show{"tvdb:73739": lost, "tmdb:4607": lost, "imdb:tt0411008": lost}}
	for folder, want := range map[string]string{
		"Lost (2004) {tvdb-73739}":     "tvdb:73739",
		"Lost (2004) [tvdb-73739]":     "tvdb:73739",
		"Lost (2004) [tvdbid-73739]":   "tvdb:73739",
		"Lost (2004) {tmdb-4607}":      "tmdb:4607",
		"Lost (2004) [imdb-tt0411008]": "imdb:tt0411008",
		"Lost (2004) [imdbid-0411008]": "imdb:tt0411008",
	} {
		f.asked = nil
		_, _, ms, err := matchShowFolder(context.Background(), f, folder)
		if err != nil || len(ms) != 1 || ms[0].Name != "Lost" || ms[0].Score != 1 || len(f.asked) != 1 || f.asked[0] != want {
			t.Fatalf("%s: %+v %v asked %v", folder, ms, err, f.asked)
		}
	}
	for _, folder := range []string{"Breaking Bad (2008) {tvdb-999999}", "Breaking Bad (2008) [tmdb-tt123]", "Breaking Bad (2008) [imdb-tt0000001]"} {
		name, year, ms, err := matchShowFolder(context.Background(), f, folder)
		if err != nil || name != "Breaking Bad" || year != 2008 || len(ms) == 0 || ms[0].ID != "tvdb:81189" {
			t.Fatalf("%s fallback: %q %d %+v %v", folder, name, year, ms, err)
		}
	}
}

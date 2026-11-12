package shows

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type fakeFinder struct{ ids map[string]string }

func (f fakeFinder) ShowTMDBID(_ context.Context, id string) (string, error) {
	if v, ok := f.ids[id]; ok {
		return v, nil
	}
	return "", metadata.ErrShowNotFound
}

// TVMaze results get their tmdb id so search can offer Add and details.
func TestFillTMDBIDs(t *testing.T) {
	list := []metadata.Show{
		{Name: "A", IDs: map[string]string{"tvmaze": "1", "tvdb": "200"}},
		{Name: "B", IDs: map[string]string{"tvmaze": "2", "imdb": "tt9"}},
		{Name: "C", IDs: map[string]string{"tvmaze": "3"}},
		{Name: "D", IDs: map[string]string{"tvmaze": "4", "tmdb": "7", "tvdb": "999"}},
	}
	fillTMDBIDs(context.Background(), fakeFinder{ids: map[string]string{"tvdb:200": "100", "imdb:tt9": "101", "tvdb:999": "8"}}, list)
	want := []string{"100", "101", "", "7"}
	for i, w := range want {
		if got := list[i].IDs["tmdb"]; got != w {
			t.Errorf("%s: tmdb %q, want %q", list[i].Name, got, w)
		}
	}
}

func TestFillTMDBIDsNoFinder(t *testing.T) {
	list := []metadata.Show{{IDs: map[string]string{"tvdb": "1"}}}
	fillTMDBIDs(context.Background(), nil, list)
	if list[0].IDs["tmdb"] != "" {
		t.Fatal("no finder, no change")
	}
}

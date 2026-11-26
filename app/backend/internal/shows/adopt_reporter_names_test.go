package shows

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// queryShows answers a name search from a fixed list and records the query.
type queryShows struct {
	byQuery map[string][]metadata.Show
	asked   []string
}

func (f *queryShows) SearchShows(_ context.Context, q string) ([]metadata.Show, error) {
	f.asked = append(f.asked, q)
	return f.byQuery[q], nil
}

func (f *queryShows) Show(context.Context, string) (metadata.Show, error) {
	return metadata.Show{}, metadata.ErrShowNotFound
}

// Folder names from a user report read down to a name the search finds.
func TestShowFolderNamesFromReport(t *testing.T) {
	for in, want := range map[string]string{
		"Law and Order - SVU Season 03":                       "Law and Order SVU",
		"The.Office.US.S02.BRRip.x264-ION10":                  "The Office US",
		"Dragon Ball Super Complete Season 2 Golden Frieza":   "Dragon Ball Super",
		"Dragon Ball Super Complete Season 3 Universe 6 Saga": "Dragon Ball Super",
		"Dragon Ball Super Complete Series 1080p":             "Dragon Ball Super",
		"Complete": "Complete",
	} {
		if n, _ := showFolderName(in); n != want {
			t.Errorf("%q -> %q, want %q", in, n, want)
		}
	}
}

// A trailing country code is left out of the search, and the answer named
// with it still ranks first.
func TestShowFolderCountryCode(t *testing.T) {
	f := &queryShows{byQuery: map[string][]metadata.Show{"The Office": {
		{Name: "The Office", Year: 2001, Source: "tmdb", IDs: map[string]string{"tmdb": "2996"}},
		{Name: "The Office", Year: 2005, Source: "tmdb", IDs: map[string]string{"tmdb": "2316"}, OtherNames: []string{"The Office (US)"}},
	}}}
	_, _, ms, err := matchShowFolder(context.Background(), f, "The.Office.US.S02.BRRip.x264-ION10")
	if err != nil || len(ms) != 2 {
		t.Fatalf("asked %v: %v %v", f.asked, ms, err)
	}
	if ms[0].TMDB != "2316" || ms[0].Score < library.MatchUnsure {
		t.Fatalf("first %+v", ms[0])
	}
	if f.asked[0] != "The Office" {
		t.Fatalf("asked %v", f.asked)
	}
}

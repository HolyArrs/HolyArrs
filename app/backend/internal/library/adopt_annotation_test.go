package library

import (
	"context"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"testing"
)

func TestReleaseAnnotationPositions(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		y        int
	}{
		{"Shutter.Island.720p.2010", "Shutter Island", 2010},
		{"The.Texas.Chainsaw.Massacre.The.Beginning.UNRATED.2006.1080p.BluRay.x264-AMBASSADOR", "The Texas Chainsaw Massacre The Beginning", 2006},
		{"Batman The Dark Knight Rises (1080p x265 Joy)", "Batman The Dark Knight Rises", 0},
		{"Example.Feature.720p.2000", "Example Feature", 2000},
		{"Example.Feature.UNRATED.2000.1080p.BluRay.x264", "Example Feature", 2000},
		{"Underworld EXTENDED (2003) [1080p]", "Underworld", 2003},
		{"Example (A True Story)", "Example (A True Story)", 0},
		{"Example (Life in 4K)", "Example (Life in 4K)", 0},
		{"The Unrated 2000", "The Unrated", 2000},
		{"2001 A Space Odyssey 1968", "2001 A Space Odyssey", 1968},
		{"101.Dalmatians.1961.1080p", "101 Dalmatians", 1961},
	} {
		t.Run(tc.in, func(t *testing.T) {
			n, y := FolderName(tc.in)
			if n != tc.want || y != tc.y {
				t.Fatalf("%q/%d want %q/%d", n, y, tc.want, tc.y)
			}
			if folderRead(tc.in).Query.Get("query") != n {
				t.Fatal("prefetch differs")
			}
		})
	}
}
func TestReleasePrefixSuggestionIsBoundedAndReviewable(t *testing.T) {
	for _, prefix := range []string{"Batman", "Imaginary"} {
		name := prefix + " The Dark Knight Rises (1080p x265 Joy).m4v"
		md := &evidenceProvider{results: map[string][]metadata.Movie{"The Dark Knight Rises/0": {{Name: "The Dark Knight Rises", Year: 2012, IDs: map[string]string{"tmdb": "test"}}}}}
		budget := 1
		_, _, ms, err := matchCandidateWithBudget(context.Background(), md, name, &budget)
		if err != nil || len(ms) != 1 || ms[0].Score >= MatchSure || ms[0].Score < MatchUnsure || budget != 0 || md.searches != 2 || md.details != 0 {
			t.Fatalf("%+v %v budget %d calls %d", ms, err, budget, md.searches)
		}
		budget = 0
		md.searches = 0
		_, _, ms, err = matchCandidateWithBudget(context.Background(), md, name, &budget)
		if err != nil || len(ms) != 0 || md.searches != 1 {
			t.Fatal("budget exceeded")
		}
	}
}

func TestAnnotationQueriesMatchMetadata(t *testing.T) {
	for _, tc := range []struct {
		file, title string
		year        int
	}{
		{"Shutter.Island.720p.2010", "Shutter Island", 2010},
		{"The.Texas.Chainsaw.Massacre.The.Beginning.UNRATED.2006.1080p.BluRay.x264-AMBASSADOR", "The Texas Chainsaw Massacre: The Beginning", 2006},
		{"Invented.Feature.UNRATED.2000.1080p.BluRay", "Invented Feature", 2000},
	} {
		query, y := FolderName(matchingLookup(tc.file))
		md := &evidenceProvider{results: map[string][]metadata.Movie{fmt.Sprintf("%s/%d", query, y): {{Name: tc.title, Year: tc.year, IDs: map[string]string{"tmdb": "example"}}}}}
		budget := 40
		_, _, ms, err := matchCandidateWithBudget(context.Background(), md, tc.file, &budget)
		if err != nil || len(ms) != 1 || ms[0].Score != 1 || md.searches != 1 || md.details != 0 || budget != 40 {
			t.Fatalf("%s: %+v %v calls %d", tc.file, ms, err, md.searches)
		}
	}
}
func TestPrefixSuggestionDoesNotBroadenOrdinaryTitles(t *testing.T) {
	for _, file := range []string{"Example The Dark Knight Rises (A True Story).mkv", "Example The Dark Knight Rises.mkv", "Example Short Title (1080p).mkv"} {
		md := &evidenceProvider{}
		budget := 40
		_, _, _, err := matchCandidateWithBudget(context.Background(), md, file, &budget)
		if err != nil || md.searches != 1 || budget != 40 {
			t.Fatalf("broadened %s", file)
		}
	}
	md := &evidenceProvider{results: map[string][]metadata.Movie{"Example The Dark Knight Rises/0": {{Name: "Example The Dark Knight Rises", Year: 2000, IDs: map[string]string{"tmdb": "full"}}}}}
	budget := 40
	_, _, ms, _ := matchCandidateWithBudget(context.Background(), md, "Example The Dark Knight Rises (1080p).mkv", &budget)
	if len(ms) != 1 || ms[0].TMDB != "full" || md.searches != 1 || budget != 40 {
		t.Fatal("full match replaced")
	}
}

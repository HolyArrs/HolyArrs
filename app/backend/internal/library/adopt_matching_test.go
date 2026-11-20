package library

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestFolderNameMatchingNormalization(t *testing.T) {
	for _, tc := range []struct {
		input, title string
		year         int
	}{
		{"Judge Dredd 1995", "Judge Dredd", 1995},
		{"Judge.Dredd.1995", "Judge Dredd", 1995},
		{"Judge.Dredd", "Judge Dredd", 0},
		{"Judge.Dredd.[1080p]", "Judge Dredd", 0},
		{"Judge.Dredd.1995.[1080p].[BluRay]", "Judge Dredd", 1995},
		{"[1080p] Judge.Dredd [1995] [x264]", "Judge Dredd", 1995},
		{"Judge Dredd [release group] (1995)", "Judge Dredd", 1995},
		{"Judge.Dredd.1995.1080p.BluRay.x264", "Judge Dredd", 1995},
		{"  Judge..Dredd   1995  ", "Judge Dredd", 1995},
		{"2001.A.Space.Odyssey.1968", "2001 A Space Odyssey", 1968},
		{"1917", "1917", 0},
		{"[REC]", "[REC]", 0},
		{"[REC] (2007)", "[REC]", 2007},
		{"[REC] [1080p] (2007)", "[REC]", 2007},
		{"[REC].2.(2009).[1080p]", "[REC] 2", 2009},
		{"[1080p].[REC].(2007)", "[REC]", 2007},
		{"Judge.Dredd.[ 1995 ].[1080p]", "Judge Dredd", 1995},
	} {
		t.Run(tc.input, func(t *testing.T) {
			name, year := FolderName(tc.input)
			if name != tc.title || year != tc.year {
				t.Fatalf("got %q (%d), want %q (%d)", name, year, tc.title, tc.year)
			}
		})
	}
}

type matchingQueryRecorder struct {
	metadata.Provider
	query string
	year  int
}

func (m *matchingQueryRecorder) Search(_ context.Context, query string, year int) ([]metadata.Movie, error) {
	m.query, m.year = query, year
	return []metadata.Movie{{Name: "Judge Dredd", Year: 1995, IDs: map[string]string{"tmdb": "9482"}}}, nil
}

func TestMatchingAndPrefetchUseNormalizedTitle(t *testing.T) {
	for _, input := range []string{"Judge.Dredd.1995", "Judge.Dredd.[1995].[1080p]", "Judge Dredd 1995"} {
		t.Run(input, func(t *testing.T) {
			md := &matchingQueryRecorder{}
			_, _, matches, err := matchFolder(context.Background(), md, input)
			if err != nil || md.query != "Judge Dredd" || md.year != 1995 || len(matches) != 1 || matches[0].Score != 1 {
				t.Fatalf("query %q year %d matches %+v error %v", md.query, md.year, matches, err)
			}
			read := folderRead(input)
			if read.Query.Get("query") != md.query || read.Query.Get("year") != "1995" {
				t.Fatalf("prefetch %+v", read)
			}
		})
	}
	read := folderRead("Judge.Dredd.[1080p].[tmdb-9482]")
	if read.Path != "/movie/9482" {
		t.Fatalf("TMDB tag lost: %+v", read)
	}
}

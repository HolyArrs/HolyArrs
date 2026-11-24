package library

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestReleaseLookupStructures(t *testing.T) {
	for _, tc := range []struct {
		file, title string
		year        int
	}{
		{"2004.Saw.1920x1080.BDRip.x264.AC3.mkv", "Saw", 2004},
		{"2005.Saw.II.1920x1080.BDRip.x264.AC3.mkv", "Saw II", 2005},
		{"2006.Saw.III.1920x1080.BDRip.x264.AC3.mkv", "Saw III", 2006},
		{"2007.Saw.IV.1920x1080.BDRip.x264.AC3.mkv", "Saw IV", 2007},
		{"2008.Saw.V.1920x1080.BDRip.x264.AC3.mkv", "Saw V", 2008},
		{"2009.Saw.VI.1920x1080.BDRip.x264.AC3.mkv", "Saw VI", 2009},
		{"2010.Saw-.The.Final.Chapter.1920x1080.BDRip.x264.AC3.mkv", "Saw- The Final Chapter", 2010},
		{"2017.Jigsaw.1920x806.BDRip.x264.TrueHD-Atmos.mkv", "Jigsaw", 2017},
		{"2021.Spiral.1920x804.BDRip.x264.TrueHD-Atmos.mkv", "Spiral", 2021},
		{"Alien Collection/Alien 3 Special Edition - Sci-Fi 1992 Eng Subs 720p.mp4", "Alien 3", 1992},
		{"Alien Collection/Alien Directors Cut - Sci-Fi 1979 Eng Subs 720p.mp4", "Alien", 1979},
		{"Alien Collection/Aliens Special Edition - Sci-Fi 1986 Eng Subs 720p.mp4", "Aliens", 1986},
		{"Alien Collection/Alien Resurrection Special Edition - Sci-Fi 1997 Eng Subs 720p.mp4", "Alien Resurrection", 1997},
		{"Alien Collection/Alien vs Predator AVP Unrated - Sci-Fi 2004 Eng Subs 720p.mp4", "Alien vs Predator", 2004},
		{"Alien Collection/Alien vs Predator Requiem AVP2 Unrated - Sci-Fi 2007 Eng Subs 720p.mp4", "Alien vs Predator Requiem", 2007},
		{"Alien Collection/Prometheus - Sci-Fi 2012 Eng Subs 720p.mp4", "Prometheus", 2012},
		{"2001.A.Space.Odyssey.1968.1920x1080.BDRip.mkv", "2001 A Space Odyssey", 1968},
		{"1917 (2019).mkv", "1917", 2019},
		{"101.Dalmatians.1961.1080p.mkv", "101 Dalmatians", 1961},
		{"Generic Collection/2000.Imaginary.Feature.1920x1080.BDRip.x264.mkv", "Imaginary Feature", 2000},
		{"Imaginary Collection/Imaginary Feature IF Special Edition - Drama 2000 1080p.mkv", "Imaginary Feature", 2000},
	} {
		t.Run(tc.file, func(t *testing.T) {
			lookup := matchingLookup(tc.file)
			name, year := FolderName(lookup)
			if name != tc.title || year != tc.year {
				t.Fatalf("got %q/%d, want %q/%d", name, year, tc.title, tc.year)
			}
			if folderRead(lookup).Query.Get("query") != name {
				t.Fatal("prefetch diverged")
			}
		})
	}
}

type evidenceProvider struct {
	metadata.Provider
	results           map[string][]metadata.Movie
	detail            metadata.Movie
	searches, details int
	failDetail        bool
}

func (p *evidenceProvider) Search(_ context.Context, q string, y int) ([]metadata.Movie, error) {
	p.searches++
	return p.results[fmt.Sprintf("%s/%d", q, y)], nil
}
func (p *evidenceProvider) Movie(_ context.Context, id string) (metadata.Movie, error) {
	p.details++
	if p.failDetail {
		return metadata.Movie{}, errors.New("offline")
	}
	return p.detail, nil
}
func evidenceMovie(name string, year int) metadata.Movie {
	return metadata.Movie{Name: name, Year: year, IDs: map[string]string{"tmdb": "42"}}
}

func TestPackDirectorEvidence(t *testing.T) {
	for _, tc := range []struct {
		name                                               string
		initial, wrongDirector, wrongYear, duplicate, fail bool
		want                                               float64
		searches                                           int
	}{
		{"existing candidate", true, false, false, false, false, 1, 1},
		{"bounded fallback", false, false, false, false, false, 1, 3},
		{"wrong director", true, true, false, false, false, 0.5, 1},
		{"wrong detail year", true, false, true, false, false, 0.5, 1},
		{"ambiguous fallback", false, false, false, true, false, 0, 3},
		{"detail unavailable", true, false, false, false, true, 0.5, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := evidenceMovie("Example Film", 2007)
			detail := m
			detail.Directors = []string{"Pat Smith"}
			if tc.wrongDirector {
				detail.Directors = []string{"Someone Else"}
			}
			if tc.wrongYear {
				detail.Year = 2008
			}
			p := &evidenceProvider{results: map[string][]metadata.Movie{"Example Film/2007": {m}}, detail: detail, failDetail: tc.fail}
			if tc.initial {
				p.results["Example Film Pat Smith/2007"] = []metadata.Movie{m}
			}
			if tc.duplicate {
				other := m
				other.IDs = map[string]string{"tmdb": "43"}
				p.results["Example Film/2007"] = append(p.results["Example Film/2007"], other)
			}
			_, _, matches, err := matchCandidate(context.Background(), p, "Example Collection/01 Example Film Pat Smith - Horror 2007 720p.mkv")
			got := 0.0
			if len(matches) > 0 {
				got = matches[0].Score
			}
			if (err != nil) != tc.fail || got != tc.want || p.searches != tc.searches || p.details > 1 {
				t.Fatalf("matches %+v err %v calls %d/%d", matches, err, p.searches, p.details)
			}
		})
	}
}

func TestPackNumberEvidenceAndOverhead(t *testing.T) {
	for _, tc := range []struct {
		file, query, result string
		year                int
		want                float64
	}{
		{"Example Collection/01 Example Film 1 - Horror 2000 720p.mkv", "Example Film 1", "Example Film", 2000, .89},
		{"Example Collection/02 Example Film 2 - Horror 2000 720p.mkv", "Example Film 2", "Example Film 2", 2000, 1},
		{"Example Film 1 (2000).mkv", "Example Film 1", "Example Film", 2000, .67},
		{"Example Collection/Example Film 3 (2000).mkv", "Example Film 3", "Example Film III", 2000, 1},
		{"Example Collection/Example 3 (2000).mkv", "Example 3", "Example³", 2000, 1},
		{"X (2000).mkv", "X", "10", 2000, 0},
	} {
		t.Run(tc.file, func(t *testing.T) {
			p := &evidenceProvider{results: map[string][]metadata.Movie{fmt.Sprintf("%s/%d", tc.query, tc.year): {evidenceMovie(tc.result, tc.year)}}}
			_, _, m, e := matchCandidate(context.Background(), p, tc.file)
			wantDetails := 0
			if tc.want == .89 {
				wantDetails = 1
			}
			if e != nil || len(m) != 1 || m[0].Score != tc.want || p.searches != 1 || p.details != wantDetails {
				t.Fatalf("%+v %v calls %d/%d", m, e, p.searches, p.details)
			}
		})
	}
}

func TestPackMetadataEvidenceExamples(t *testing.T) {
	for _, tc := range []struct {
		file, query, title, director, alias string
		year                                int
	}{
		{"Halloween Collection/09 Halloween Rob Zombie - Horror 2007 720p.mkv", "Halloween Rob Zombie", "Halloween", "Rob Zombie", "", 2007},
		{"Halloween Collection/10 Halloween II Rob Zombie - Horror 2009 720p.mkv", "Halloween II Rob Zombie", "Halloween II", "Rob Zombie", "", 2009},
		{"Final Destination Collection/01 Final Destination 1 - Horror 2000 720p.mkv", "Final Destination 1", "Final Destination", "", "Final Destination 1", 2000},
		{"Final Destination Collection/04 The Final Destination 4 - Horror 2009 720p.mkv", "The Final Destination 4", "The Final Destination", "", "Final Destination 4", 2009},
		{"2010.Saw-.The.Final.Chapter.1920x1080.BDRip.x264.mkv", "Saw- The Final Chapter", "Saw 3D", "", "Saw: The Final Chapter", 2010},
		{"2021.Spiral.1920x804.BDRip.x264.mkv", "Spiral", "Spiral: From the Book of Saw", "", "Spiral", 2021},
		{"Example Collection/Example Story 7 (2000).mkv", "Example Story 7", "Example Story", "", "Example Story 7", 2000},
	} {
		t.Run(tc.file, func(t *testing.T) {
			m := evidenceMovie(tc.title, tc.year)
			detail := m
			detail.Directors = []string{tc.director}
			detail.OtherNames = []string{tc.alias}
			p := &evidenceProvider{results: map[string][]metadata.Movie{fmt.Sprintf("%s/%d", tc.query, tc.year): {m}}, detail: detail}
			_, _, ms, err := matchCandidate(context.Background(), p, tc.file)
			if err != nil || len(ms) != 1 || ms[0].Name != tc.title || ms[0].Score != 1 || p.searches != 1 || p.details != 1 {
				t.Fatalf("%+v %v requests %d/%d", ms, err, p.searches, p.details)
			}
		})
	}
}

func TestEvidenceRejectsWrongYearAndCancellation(t *testing.T) {
	m := evidenceMovie("Example Film", 2001)
	p := &evidenceProvider{results: map[string][]metadata.Movie{"Example Film 1/2000": {m}}, detail: m}
	_, _, ms, err := matchCandidate(context.Background(), p, "Example Collection/Example Film 1 (2000).mkv")
	if err != nil || ms[0].Score >= MatchSure || p.details != 0 {
		t.Fatalf("wrong year accepted: %+v %v", ms, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m = evidenceMovie("Example Film", 2000)
	p = &evidenceProvider{results: map[string][]metadata.Movie{"Example Film Pat Smith/2000": {m}}, failDetail: true}
	_, _, _, err = matchCandidate(ctx, p, "Example Collection/Example Film Pat Smith (2000).mkv")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
}

func BenchmarkCollectionPackLookup(b *testing.B) {
	for b.Loop() {
		matchingLookup("Example Collection/01 Example Film Special Edition - Horror 2000 Eng Subs 720p.mkv")
	}
}

func TestEvidenceScanBudget(t *testing.T) {
	m := evidenceMovie("Example Film", 2000)
	detail := m
	detail.Directors = []string{"Pat Smith"}
	p := &evidenceProvider{results: map[string][]metadata.Movie{"Example Film Pat Smith/2000": {m}}, detail: detail}
	budget := 1
	for i := 0; i < 2; i++ {
		_, _, ms, err := matchCandidateWithBudget(context.Background(), p, "Example Collection/Example Film Pat Smith (2000).mkv", &budget)
		want := 1.0
		if i == 1 {
			want = .5
		}
		if err != nil || len(ms) != 1 || ms[0].Score != want {
			t.Fatalf("%+v %v", ms, err)
		}
	}
	if budget != 0 || p.details != 1 || p.searches != 2 {
		t.Fatalf("unbounded requests: %d %d/%d", budget, p.searches, p.details)
	}
}

func TestEvidenceFallbackSearchFailureIsNotNoMatch(t *testing.T) {
	md := &failedEvidenceSearch{}
	_, _, _, err := matchCandidate(context.Background(), md, "Example Collection/Example Film Pat Smith (2000).mkv")
	if err == nil || err.Error() != "metadata unavailable" {
		t.Fatalf("lost provider error: %v", err)
	}
}

type failedEvidenceSearch struct {
	metadata.Provider
	calls int
}

func (p *failedEvidenceSearch) Search(context.Context, string, int) ([]metadata.Movie, error) {
	p.calls++
	if p.calls == 3 {
		return nil, errors.New("metadata unavailable")
	}
	return nil, nil
}

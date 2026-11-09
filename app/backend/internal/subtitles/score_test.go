package subtitles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
)

// The fixed set of scoring cases.
func TestScore_FixedSet(t *testing.T) {
	ep := Video{Kind: "episode", Title: "Quiet Harbour", Year: 2019, Season: 1, Episode: 2, ReleaseName: "Quiet.Harbour.S01E02.1080p.WEB.h264-GRP"}
	mv := Video{Kind: "movie", Title: "The Lantern", Year: 2010, ReleaseName: "The.Lantern.2010.1080p.BluRay.x264-GRP"}
	en := Want{Language: "en", HearingImpaired: HINormal}
	pass := DefaultSettings()
	cases := []struct {
		n       int
		v       Video
		c       sites.Candidate
		total   int
		refused string
	}{
		{1, ep, sites.Candidate{Title: "Quiet Harbour", Season: 1, Episode: 2, MatchedByID: true, ReleaseNames: []string{"Quiet.Harbour.S01E02.1080p.WEB.h264-GRP"}}, 98, ""},
		{2, ep, sites.Candidate{Title: "Quiet Harbour", Season: 1, Episode: 2, MatchedByID: true}, 90, ""},
		{3, ep, sites.Candidate{Title: "Quiet Harbour", Season: 1, Episode: 3, MatchedByID: true}, 0, "wrong episode"},
		{4, ep, sites.Candidate{Title: "Quiet Harbour", Season: 1, Episode: 2, MatchedByID: true, HearingImpaired: true}, 88, ""},
		{5, mv, sites.Candidate{ReleaseNames: []string{"The.Lantern.2010.1080p.BluRay.x264-GRP"}}, 97, ""},
		{6, mv, sites.Candidate{ReleaseNames: []string{"The.Lantern.2010.720p.WEB.x264-OTHER"}}, 57, ""},
		{7, mv, sites.Candidate{ReleaseNames: []string{"The.Lantern.2010.1080p.BluRay.x264-OTHER"}}, 72, ""},
		{8, mv, sites.Candidate{ReleaseNames: []string{"The.Lantern.2011.1080p.BluRay.x264-GRP"}}, 0, "wrong year"},
		{9, mv, sites.Candidate{ReleaseNames: []string{"Other.Movie.2010.1080p.BluRay.x264-GRP"}}, 0, "wrong title"},
		{10, mv, sites.Candidate{Forced: true, ReleaseNames: []string{"The.Lantern.2010.1080p.BluRay.x264-GRP"}}, 0, "forced does not match the wish"},
	}
	var ranked []Scored
	for _, c := range cases {
		s := ScoreCandidate(c.v, en, c.c)
		if s.Total != c.total || s.Refused != c.refused {
			t.Errorf("#%d: %d %q, want %d %q (%+v)", c.n, s.Total, s.Refused, c.total, c.refused, s.Parts)
		}
		if c.v.Kind == "movie" {
			ranked = append(ranked, Scored{Candidate: sites.Candidate{Key: string(rune('0' + c.n))}, Score: s})
		}
	}
	// Case 2 meets the episode pass mark exactly and case 4 falls short.
	if pass.PassMark("episode") != 90 || 88 >= pass.PassMark("episode") {
		t.Fatal("episode pass mark")
	}
	Rank(ranked)
	if ranked[0].Key != "5" || ranked[1].Key != "7" || ranked[2].Key != "6" {
		t.Fatalf("order %v %v %v", ranked[0].Key, ranked[1].Key, ranked[2].Key)
	}
	over := 0
	for _, r := range ranked {
		if r.Score.Refused == "" && r.Score.Total >= pass.PassMark("movie") {
			over++
		}
	}
	if over != 2 { // 5 and 7 pass, while 6, 8, 9 and 10 are skipped
		t.Fatalf("over the pass mark: %d", over)
	}
}

func TestScore_Hints(t *testing.T) {
	mv := Video{Kind: "movie", Title: "The Lantern", Aliases: []string{"La Linterna"}, Year: 2010, ReleaseName: "x", FPS: 23.976}
	base := sites.Candidate{Title: "La Linterna", Year: 2010}
	s := ScoreCandidate(mv, Want{Language: "en", HearingImpaired: HIOnly}, base)
	if s.Total != 50 {
		t.Errorf("alias, not hi: %d", s.Total)
	}
	c := base
	c.HearingImpaired, c.Trusted, c.FPS = true, true, 23.976
	if s := ScoreCandidate(mv, Want{Language: "en", HearingImpaired: HIOnly}, c); s.Total != 56 {
		t.Errorf("hi, trusted, fps: %d %+v", s.Total, s.Parts)
	}
	c.FPS, c.Corrected = 25, false
	if s := ScoreCandidate(mv, Want{Language: "en", HearingImpaired: HIEither}, c); s.Total != 55 {
		t.Errorf("fps off: %d", s.Total)
	}
	// Every part is listed with the points it would give.
	if len(s.Parts) != 11 {
		t.Errorf("movie parts: %d", len(s.Parts))
	}
}

func TestRank_Ties(t *testing.T) {
	cs := []Scored{
		{Candidate: sites.Candidate{Key: "a", Downloads: 5}, Score: Score{Total: 80}, Priority: 25, Order: 0},
		{Candidate: sites.Candidate{Key: "b", Trusted: true}, Score: Score{Total: 80}, Priority: 25, Order: 1},
		{Candidate: sites.Candidate{Key: "c", Downloads: 9}, Score: Score{Total: 80}, Priority: 25, Order: 2},
		{Candidate: sites.Candidate{Key: "d"}, Score: Score{Total: 80}, Priority: 1, Order: 3},
		{Candidate: sites.Candidate{Key: "e"}, Score: Score{Refused: "wrong title"}, Order: 4},
	}
	Rank(cs)
	got := ""
	for _, c := range cs {
		got += c.Key
	}
	if got != "bdcae" {
		t.Fatalf("%s", got)
	}
}

// Only the first MaxReleaseNames names of a candidate are scored, so an exact
// match beyond the cap goes unseen.
func TestScore_CapsReleaseNames(t *testing.T) {
	mv := Video{Kind: "movie", Title: "The Lantern", Year: 2010, ReleaseName: "The.Lantern.2010.1080p.BluRay.x264-GRP"}
	var names []string
	for range MaxReleaseNames {
		names = append(names, "The.Lantern.2010.720p.WEB.x264-OTHER")
	}
	far := ScoreCandidate(mv, Want{Language: "en"}, sites.Candidate{ReleaseNames: append(names, mv.ReleaseName)})
	near := ScoreCandidate(mv, Want{Language: "en"}, sites.Candidate{ReleaseNames: append([]string{mv.ReleaseName}, names...)})
	if far.Total >= near.Total {
		t.Fatalf("name past the cap was scored: far %d near %d", far.Total, near.Total)
	}
}

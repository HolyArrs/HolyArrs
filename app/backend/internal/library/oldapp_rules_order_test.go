package library

import (
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A profile that ranks WEB-DL 1080p above Blu-ray 1080p keeps that order.
func TestOldProfileRulesKeepsOwnOrder(t *testing.T) {
	body, notes := OldProfileRules(OldProfile{
		Qualities:       []string{"HDTV-720p", "Bluray-1080p", "WEBDL-1080p"},
		CutoffQualities: []string{"WEBDL-1080p"},
		UpgradeAllowed:  true,
	})
	var on []string
	for _, s := range body.Steps {
		if s.Allowed {
			on = append(on, s.Quality)
		}
	}
	if want := []string{"720p_tv", "1080p_bluray", "1080p_web"}; !slices.Equal(on, want) {
		t.Fatalf("allowed steps %v, want %v", on, want)
	}
	if body.GoodEnoughAt != "1080p_web" {
		t.Fatalf("good enough %q", body.GoodEnoughAt)
	}
	if err := mediafiles.CheckPickRules("x", []string{"movie"}, body); err != nil {
		t.Fatal(err)
	}
	// The web file now reaches good enough; the Blu-ray one does not.
	if !mediafiles.ReachesGoodEnough("Movie.2020.1080p.WEB-DL.DDP5.1.H.264-GRP", body) {
		t.Fatal("web should reach good enough")
	}
	if mediafiles.ReachesGoodEnough("Movie.2020.1080p.BluRay.x264-GRP", body) {
		t.Fatal("Blu-ray should rank below good enough")
	}
	for _, n := range notes {
		if n == "it ranks its qualities in an order of its own; here they keep the ladder's order" {
			t.Fatal("order note still given")
		}
	}
}

// A profile in the ladder's own order changes nothing.
func TestOldProfileRulesLadderOrder(t *testing.T) {
	body, _ := OldProfileRules(OldProfile{Qualities: []string{"WEBDL-1080p", "Bluray-1080p"}, CutoffQualities: []string{"Bluray-1080p"}, UpgradeAllowed: true})
	for i, s := range body.Steps {
		if s.Quality != mediafiles.Ladder[i] {
			t.Fatalf("step %d is %q, want %q", i, s.Quality, mediafiles.Ladder[i])
		}
	}
	if body.GoodEnoughAt != "1080p_bluray" {
		t.Fatalf("good enough %q", body.GoodEnoughAt)
	}
}

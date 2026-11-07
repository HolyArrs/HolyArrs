package library

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
)

func TestMergeAliases(t *testing.T) {
	got := mergeAliases([]string{"Kaze no Tsubasa"}, "Wings of the Wind",
		[]string{"kaze no tsubasa!", "The Wings of the Wind", "La Casa de Papel", "la casa de papel", "1923", "Movie", "Ice", "Haus des Geldes", "  "})
	want := []string{"Kaze no Tsubasa", "La Casa de Papel", "Haus des Geldes"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMergeAliasesCap(t *testing.T) {
	var more []string
	for i := 0; i < 50; i++ {
		more = append(more, fmt.Sprintf("Another Name %c%c", 'a'+i/26, 'a'+i%26))
	}
	if got := mergeAliases([]string{"Old Name"}, "Main", more); len(got) != maxAliases || got[0] != "Old Name" {
		t.Errorf("%d aliases, first %q", len(got), got[0])
	}
}

func TestApplyFillsAliasesForMovieAndShow(t *testing.T) {
	var mv domain.Title
	applyMovie(&mv, metadata.Movie{Name: "Amelie", Year: 2001, OtherNames: []string{"Le Fabuleux Destin d'Amélie Poulain", "Amélie", "2001"}}, time.Now())
	if !slices.Equal(mv.Aliases, []string{"Le Fabuleux Destin d'Amélie Poulain"}) {
		t.Errorf("movie aliases %q", mv.Aliases)
	}
	sh := domain.Title{Aliases: []string{"Kept Name"}}
	applyShow(&sh, metadata.Show{Name: "Money Heist", Status: "ended", OtherNames: []string{"La Casa de Papel", "Haus des Geldes"}})
	if !slices.Equal(sh.Aliases, []string{"Kept Name", "La Casa de Papel", "Haus des Geldes"}) {
		t.Errorf("show aliases %q", sh.Aliases)
	}
}

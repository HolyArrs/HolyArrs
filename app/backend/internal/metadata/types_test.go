package metadata

import (
	"context"
	"errors"
	"testing"
)

func TestStaticLabMovie(t *testing.T) {
	var p Provider = Lab()
	ctx := context.Background()
	got, err := p.Search(ctx, "dune part two", 2024)
	if err != nil || len(got) != 1 {
		t.Fatalf("search %v %v", got, err)
	}
	m := got[0]
	if m.Name != "Dune: Part Two" || m.Year != 2024 || m.IDs["tmdb"] != "693134" || m.IDs["imdb"] != "tt15239678" {
		t.Fatalf("movie %+v", m)
	}
	byID, err := p.Movie(ctx, "tmdb:693134")
	if err != nil || byID.Name != m.Name {
		t.Fatalf("by id %+v %v", byID, err)
	}
	if _, err := p.Movie(ctx, "tmdb:1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	if got, _ := p.Search(ctx, "dune part two", 2021); len(got) != 0 {
		t.Fatal("year should filter")
	}
}

func TestSwitch(t *testing.T) {
	on := false
	s := &Switch{Use: func(context.Context) bool { return on }, Then: &Static{}, Else: Lab()}
	if got, _ := s.Search(context.Background(), "dune", 0); len(got) != 1 {
		t.Fatal("else branch not used")
	}
	on = true
	if got, _ := s.Search(context.Background(), "dune", 0); len(got) != 0 {
		t.Fatal("then branch not used")
	}
}

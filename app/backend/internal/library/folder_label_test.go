package library

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestFreeLabel(t *testing.T) {
	all := []mediafiles.LibraryFolder{{ID: 1, Kind: "movie", Label: "Movies"}, {ID: 2, Kind: "movie", Label: "Movies 2"}, {ID: 3, Kind: "show", Label: "Shows"}}
	if l, ok := freeLabel(all, "movie", "movies", 0, true); !ok || l != "movies 3" {
		t.Fatalf("a taken name gets the next number: %q %v", l, ok)
	}
	if l, ok := freeLabel(all, "show", "Movies", 0, false); !ok || l != "Movies" {
		t.Fatalf("names are per kind: %q %v", l, ok)
	}
	if _, ok := freeLabel(all, "movie", "Movies 2", 0, false); ok {
		t.Fatal("renaming to a taken name is refused")
	}
	if l, ok := freeLabel(all, "movie", "Movies", 1, false); !ok || l != "Movies" {
		t.Fatalf("a folder keeps its own name: %q %v", l, ok)
	}
}

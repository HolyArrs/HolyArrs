package mbread

import (
	"context"
	"strings"
	"testing"
)

// Brief answers what a picture lookup needs (name, country, URL links)
// without building the full answer (no release groups).
func TestArtistBrief(t *testing.T) {
	r := open(t)
	b, ok := r.ArtistBrief(context.Background(), artistGID)
	if !ok || b.Name != "Test Band" || b.Country != "GB" || len(b.Links) != 2 {
		t.Fatalf("brief %+v %v", b, ok)
	}
	if _, ok := r.ArtistBrief(context.Background(), "00000000-0000-0000-0000-000000000000"); ok {
		t.Fatal("unknown artist answered")
	}
}

func TestHomonymIDs(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	if ids := r.HomonymIDs(ctx, "Test Band", artistGID); len(ids) != 0 {
		t.Fatalf("the artist itself counted: %v", ids)
	}
	if ids := r.HomonymIDs(ctx, "Test Band", "00000000-0000-0000-0000-000000000000"); len(ids) != 1 || ids[0] != artistGID {
		t.Fatalf("same-name artist not answered: %v", ids)
	}
}

// TitleArtists counts the artists credited on release groups of a title,
// case aside.
func TestTitleArtists(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	for title, want := range map[string]int{"First Album": 1, "first album": 1, "Live Single": 2, "Nothing": 0} {
		if n := r.TitleArtists(ctx, title); n != want {
			t.Errorf("%q: %d, want %d", title, n, want)
		}
	}
}

// ArtistBarcodes answers the barcodes of releases credited to the artist
// alone (a shared credit is left out), each once.
func TestArtistBarcodes(t *testing.T) {
	r := open(t)
	got := r.ArtistBarcodes(context.Background(), artistGID, 5)
	if len(got) != 2 || !((got[0] == "5012345678900" && got[1] == "0123456789012") || (got[1] == "5012345678900" && got[0] == "0123456789012")) {
		t.Fatalf("barcodes %v", got)
	}
	if got := r.ArtistBarcodes(context.Background(), artistGID, 1); len(got) != 1 {
		t.Fatalf("limit not kept: %v", got)
	}
	if got := r.ArtistBarcodes(context.Background(), guestGID, 5); len(got) != 0 {
		t.Fatalf("a shared credit's barcodes: %v", got)
	}
}

// ArtistByBarcode answers the one artist a barcode's release credits alone;
// a shared credit or an unknown barcode answers "".
func TestArtistByBarcode(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	if g := r.ArtistByBarcode(ctx, "5012345678900"); g != artistGID {
		t.Fatalf("barcode artist %q", g)
	}
	if g := r.ArtistByBarcode(ctx, "0000000000000"); g != "" {
		t.Fatalf("unknown barcode %q", g)
	}
}

// ArtistLinks answers every artist's links whose address holds a part.
func TestArtistLinks(t *testing.T) {
	r := open(t)
	got := r.ArtistLinks(context.Background(), []string{"wikidata.org"})
	if len(got) != 1 || got[0][0] != artistGID || got[0][1] != "https://www.wikidata.org/wiki/Q1" {
		t.Fatalf("links %v", got)
	}
}

// NameHolders counts the artists carrying a name, case aside.
func TestNameHolders(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	if n := r.NameHolders(ctx, "test band"); n != 1 {
		t.Fatalf("holders %d", n)
	}
	if n := r.NameHolders(ctx, "Nobody"); n != 0 {
		t.Fatalf("holders %d", n)
	}
}

// ArtistNames answers the name, sort name and aliases; an unknown artist
// answers none.
func TestArtistNames(t *testing.T) {
	r := open(t)
	ctx := context.Background()
	got := strings.Join(r.ArtistNames(ctx, artistGID), "|")
	for _, want := range []string{"Test Band", "Band, Test", "TB"} {
		if !strings.Contains(got, want) {
			t.Fatalf("names %q, want %q", got, want)
		}
	}
	if n := r.ArtistNames(ctx, "00000000-0000-4000-8000-000000000000"); len(n) != 0 {
		t.Fatalf("unknown artist %v", n)
	}
}

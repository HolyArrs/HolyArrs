package music

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestCoreAlbumsCompleteListOnly(t *testing.T) {
	t.Parallel()
	art := metadata.Artist{Albums: []metadata.Album{
		{MBID: "A0000000-0000-0000-0000-000000000001", Name: "Old", PrimaryType: "Album", ReleaseDate: "2015-01-01"},
		{MBID: "a0000000-0000-0000-0000-000000000002", Name: "New", PrimaryType: "Single", ReleaseDate: "2024-05-01"},
		{MBID: "a0000000-0000-0000-0000-000000000003", Name: "Clip", PrimaryType: "Other", SecondaryTypes: []string{"Video"}},
	}}
	got := coreAlbums(art)
	if len(got) != 2 || got[0].Name != "New" || got[1].MBID != "a0000000-0000-0000-0000-000000000001" {
		t.Fatalf("coreAlbums = %+v", got)
	}
	if got[1].Image != artPath("album", got[1].MBID) || got[1].Shelf == "" {
		t.Fatalf("cover or shelf missing: %+v", got[1])
	}
	art.Partial = true
	if coreAlbums(art) != nil {
		t.Fatal("a partial list must not be sent with the hero")
	}
}

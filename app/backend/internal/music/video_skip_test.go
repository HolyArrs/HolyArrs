package music

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestWithoutVideosSkipsPlayed(t *testing.T) {
	t.Parallel()
	vids := []metadata.ArtistVideo{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got := withoutVideos(vids, []string{"a", " c", ""})
	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("want only b, got %+v", got)
	}
	if left := withoutVideos(vids, []string{"a", "b", "c"}); len(left) != 0 {
		t.Fatalf("all played: want none left, got %+v", left)
	}
}

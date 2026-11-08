package music

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/music/artists/{mbid}?part=videos
func TestArtistVideoList(t *testing.T) {
	t.Parallel()
	u := strings.Replace(videoSetup(t, func(context.Context, string) ([]metadata.ArtistVideo, error) {
		return []metadata.ArtistVideo{{ID: "dQw4w9WgXcQ", Title: "First", Duration: 212}, {ID: "not-a-youtube-id"}, {ID: "9bZkp7q19f0", Title: "Second"}}, nil
	}), "part=video", "part=videos", 1)
	var got struct {
		Videos []ArtistVideoItem `json:"videos"`
	}
	if err := json.Unmarshal(do(t, "GET", u, "", 200), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Videos) != 2 || got.Videos[0].ID != "dQw4w9WgXcQ" || got.Videos[0].Title != "First" || got.Videos[0].Duration != 212 || got.Videos[1].ID != "9bZkp7q19f0" {
		t.Fatalf("the kept videos with a valid id, in order: %+v", got.Videos)
	}

	// No videos, or no video source at all: an empty list, never an error.
	for _, fn := range []func(context.Context, string) ([]metadata.ArtistVideo, error){func(context.Context, string) ([]metadata.ArtistVideo, error) { return nil, nil }, nil} {
		u := strings.Replace(videoSetup(t, fn), "part=video", "part=videos", 1)
		var empty struct {
			Videos []ArtistVideoItem `json:"videos"`
		}
		if err := json.Unmarshal(do(t, "GET", u, "", 200), &empty); err != nil || empty.Videos == nil || len(empty.Videos) != 0 {
			t.Fatalf("empty list expected: %+v %v", empty, err)
		}
	}
}

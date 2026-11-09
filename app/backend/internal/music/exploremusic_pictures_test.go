package music

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/explore/music (music_rows_v7 picture records)
// Music Explore answers carry the records of the pictures they show, like
// every other music answer, so the first draw already asks the versioned
// address and nothing is fetched twice.
func TestExploreMusicCarriesPictureRecords(t *testing.T) {
	t.Parallel()
	r, _, _ := planRig(t, "household_large.json")
	rec := &metadata.PictureRecord{V: "v1", W: 1200, H: 1200, Color: "#102030"}
	r.api.s.MusicPictures = func(_ context.Context, artists, albums []string) (metadata.MusicPictures, error) {
		out := metadata.MusicPictures{Artists: map[string]metadata.ArtistPictures{}, Albums: map[string]metadata.AlbumPictures{}}
		for _, id := range artists {
			out.Artists[id] = metadata.ArtistPictures{Photo: rec}
		}
		for _, id := range albums {
			out.Albums[id] = metadata.AlbumPictures{Cover: rec}
		}
		return out, nil
	}
	check := func(name string, b []byte) {
		t.Helper()
		if !bytes.HasPrefix(b, []byte(`{"pictures":{`)) {
			t.Fatalf("%s: no picture records first: %.200s", name, b)
		}
		var a struct {
			Pictures map[string]metadata.PictureRecord `json:"pictures"`
			Next     string                            `json:"next"`
		}
		if err := json.Unmarshal(b, &a); err != nil || len(a.Pictures) == 0 {
			t.Fatalf("%s: %v, %d records", name, err, len(a.Pictures))
		}
	}
	first := r.owner.call("GET", "/api/v1/explore/music?region=US", "", 200)
	check("page 1", first)
	var a planAnswer
	_ = json.Unmarshal(first, &a)
	if a.Next == "" {
		t.Fatal("no next cursor")
	}
	check("cursor page", r.owner.call("GET", "/api/v1/explore/music?region=US&cursor="+a.Next, "", 200))
}

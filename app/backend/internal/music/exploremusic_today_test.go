package music

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// route: GET /api/v1/explore/music/rising
func TestRisingTodayRow(t *testing.T) {
	t.Parallel()
	r, _ := orderRig(t)
	r.ana.call("GET", "/api/v1/explore/music/rising", "", 404) // no source: no row
	asks := 0
	r.api.s.MusicRising = func(ctx context.Context) (metadata.MusicRising, error) {
		asks++
		return metadata.MusicRising{AsOf: "2026-10-27", Items: []metadata.MusicRisingArtist{
			{MBID: "9fff2f8a-21e6-47de-a2b8-7f449929d43f", Name: "Drake", Listeners: 80e6, Gain: 1_250_000, Rank: 7},
			{MBID: "a74b1b7f-71a5-4011-9441-d0b5e4122711", Name: "Radiohead", Listeners: 9e6},
		}}, nil
	}
	r.api.rising = risingCache{}
	var row MusicRow
	if err := json.Unmarshal(r.ana.call("GET", "/api/v1/explore/music/rising", "", 200), &row); err != nil {
		t.Fatal(err)
	}
	if row.Title != "Rising today" || row.Shape != "round" || len(row.Items) != 2 {
		t.Fatalf("%+v", row)
	}
	if c := row.Items[0]; c.Kind != "artist" || c.Note != "+1.2M listeners today" || c.Image != artPath("artist", c.MBID) {
		t.Fatalf("card %+v", c)
	}
	if row.Items[1].Note != "" {
		t.Fatal("no gain, no note")
	}
	r.ana.call("GET", "/api/v1/explore/music/rising", "", 200)
	if asks != 1 {
		t.Fatalf("kept for an hour, asked %d", asks)
	}
}

func TestShortCount(t *testing.T) {
	t.Parallel()
	for n, want := range map[int64]string{999: "999", 1234: "1.2K", 2_000_000: "2M", 250_400_000: "250M", 1_500_000_000: "1.5B"} {
		if got := shortCount(n); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
}

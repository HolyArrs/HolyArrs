package music

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// risingRig is the v3 household rig with music_rows_v6 on; ranks adds a
// 7-day-old rank to every chart item (all new entries, so each one
// gains), and the chart artists get a first release in 2025.
func risingRig(t *testing.T, ranks bool) *askRig {
	r, _ := orderRig(t)
	charts := r.api.s.MusicCharts
	r.api.s.MusicCharts = func(ctx context.Context, region, kind, genre string) (metadata.MusicChart, error) {
		c, err := charts(ctx, region, kind, genre)
		if ranks {
			for i := range c.Items {
				ago := 0
				c.Items[i].RankAgo = &ago
			}
		}
		return c, err
	}
	data := r.api.s.MusicArtistData
	r.api.s.MusicArtistData = func(ctx context.Context, ids []string) ([]metadata.MusicArtistData, error) {
		out, err := data(ctx, ids)
		for i := range out {
			if strings.HasPrefix(out[i].MBID, "e") {
				out[i].Albums = append(out[i].Albums, metadata.MusicTopAlbum{MBID: "1" + out[i].MBID[1:], Title: "Debut", Type: "album", Date: "2025-02-01", Artists: []string{out[i].MBID}})
			}
		}
		return out, err
	}
	return r
}

// route: GET /api/v1/explore/music (music_rows_v6 Rising)
func TestExploreMusicRising(t *testing.T) {
	t.Parallel()
	read := func(r *askRig) musicRowsAnswer {
		var m musicRowsAnswer
		if err := json.Unmarshal(r.ana.call("GET", "/api/v1/explore/music?region=US", "", 200), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	// Fewer than 7 days of ranks: no Rising row.
	if m := read(risingRig(t, false)); m.row("rising") != nil {
		t.Fatal("Rising must stay hidden without 7 days of ranks")
	}
	// 7-day fixture: Rising shows, each card with its move.
	m := read(risingRig(t, true))
	row := m.row("rising")
	if row == nil {
		t.Fatalf("no Rising row with 7 days of ranks:\n%s", musicShape(m))
	}
	for _, c := range row.Items {
		if c.Note == "" || !strings.HasPrefix(c.ArtistMBID, "e") {
			t.Fatalf("rising card %+v", c)
		}
	}
}

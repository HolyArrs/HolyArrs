package library

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A show on a chosen episode order is renamed by that order's numbers.
func TestRenameFactsByEpisodeOrder(t *testing.T) {
	release := "Trapped.S03E03E04.1080p.WEB.H264-GRP"
	f := mediafiles.File{RelativePath: "Trapped/" + release + ".mkv", ReleaseName: &release}
	eps := []importer.Episode{
		{Ref: mediafiles.EpisodeRef{Season: 3, Episode: 3, AltSeason: 3, AltEpisode: 4}, Name: "Three"},
		{Ref: mediafiles.EpisodeRef{Season: 3, Episode: 4, AltSeason: 3, AltEpisode: 3}, Name: "Four"},
	}
	ti := mediafiles.Title{Name: "Trapped"}
	if fa := episodeFacts(ti, f, eps); fa.Episodes[0] != 3 || fa.Titles[0] != "Three" {
		t.Fatalf("TMDB numbers: %+v", fa)
	}
	g := "64a0f1c2b7d8e9001c2a3b4c"
	ti.EpisodeGroupID = &g
	fa := episodeFacts(ti, f, eps)
	if fa.Season != 3 || len(fa.Episodes) != 2 || fa.Episodes[0] != 3 || fa.Titles[0] != "Four" || fa.Episodes[1] != 4 {
		t.Fatalf("order numbers: %+v", fa)
	}
}

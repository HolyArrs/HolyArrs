package main

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestEpisodesCarryStillAndRuntime(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	var titleID int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		show := mediafiles.Title{Kind: "show", Name: "S", SortName: "s", SourceIDs: map[string]string{"tmdb": "9"}, MetadataSource: "tmdb", NewPartsPolicy: "all", WhyAdded: "by_hand", State: "active", ReleaseGate: "released", GateDriver: "none", Status: "running"}
		if _, err := database.Insert(tx, "titles", &show, domain.System); err != nil {
			return err
		}
		titleID = show.ID
		season, ep, rt := 1, 1, 44
		it := mediafiles.Item{TitleID: show.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: &season, Episode: &ep}, Runtime: &rt,
			Still: "https://img.test/w780/s1.jpg", SourceIDs: map[string]string{}}
		_, err := database.Insert(tx, "items", &it, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	b := string(do(t, "GET", srv.URL+"/api/v1/titles/"+itoa(titleID)+"/episodes", "", 200))
	if !strings.Contains(b, `"still":"https://img.test/w780/s1.jpg"`) || !strings.Contains(b, `"runtime_minutes":44`) || !strings.Contains(b, `"runtime":44`) {
		t.Fatalf("episodes %s", b)
	}
}

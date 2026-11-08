package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/api"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

func TestLibrarySummaryRecentNewestFirst(t *testing.T) {
	t.Parallel()
	srv, s, _ := lbSetup(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		for n := range 25 {
			title := mediafiles.Title{Kind: "movie", Name: fmt.Sprintf("Movie %02d", n), SortName: fmt.Sprintf("movie %02d", n), Artwork: []string{"poster:p.jpg"},
				MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
			if n == 24 {
				title.State = "removed"
			}
			if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
				return err
			}
			// Movie 03 was added last although its id is low.
			at := base.Add(time.Duration(n) * time.Hour)
			if n == 3 {
				at = base.Add(100 * time.Hour)
			}
			if _, err := tx.Exec("UPDATE titles SET created_at = ? WHERE id = ?", database.FormatTime(at), title.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Recent []api.TitleView `json:"recent"`
	}
	if err := json.Unmarshal(lbDo(t, "GET", srv.URL+"/api/v1/library-summary", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Recent) != 20 || got.Recent[0].Name != "Movie 03" || got.Recent[1].Name != "Movie 23" || got.Recent[19].Name != "Movie 05" {
		names := []string{}
		for _, r := range got.Recent {
			names = append(names, r.Name)
		}
		t.Fatalf("recent %v", names)
	}
	if len(got.Recent[0].Artwork) != 1 || got.Recent[0].Artwork[0] != "/api/v1/artwork/p.jpg" {
		t.Fatalf("artwork %v", got.Recent[0].Artwork)
	}
}

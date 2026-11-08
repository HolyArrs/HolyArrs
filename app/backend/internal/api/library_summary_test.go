package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// route: GET /api/v1/library-summary
func TestHomeLibrarySummaryIsBounded(t *testing.T) {
	t.Parallel()
	srv, s, _ := setup(t)
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		for n := range 12 {
			title := mediafiles.Title{Kind: "movie", Name: fmt.Sprintf("Movie %02d", n), SortName: fmt.Sprintf("movie %02d", n),
				MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
			if n == 11 {
				title.State = "removed"
			}
			if _, err := database.Insert(tx, "titles", &title, domain.System); err != nil {
				return err
			}
			item := mediafiles.Item{TitleID: title.ID, Kind: "movie", Name: &title.Name}
			if _, err := database.Insert(tx, "items", &item, domain.System); err != nil {
				return err
			}
			version := mediafiles.Version{TitleID: title.ID, Label: "Main", IsMain: true, LibraryFolderID: 1, PickRulesID: 1}
			if _, err := database.Insert(tx, "versions", &version, domain.System); err != nil {
				return err
			}
			slot := mediafiles.Slot{ItemID: item.ID, VersionID: version.ID, State: "wanted"}
			if n == 0 {
				slot.State = "in_library"
			}
			if n == 1 {
				slot.State = "searching"
			}
			if _, err := database.Insert(tx, "slots", &slot, domain.System); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before := s.WriteCount()
	var got struct {
		Total   int                  `json:"total"`
		Missing []mediafiles.LibraryMissingView `json:"missing"`
	}
	body := do(t, "GET", srv.URL+"/api/v1/library-summary", "", 200)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 11 || len(got.Missing) != 8 || got.Missing[0].Name != "Movie 01" || got.Missing[7].Name != "Movie 08" {
		t.Fatalf("summary: %s", body)
	}
	if got.Missing[0].StateLabel != StateLabels[mediafiles.SlotSearching] {
		t.Fatalf("missing title state: %+v", got.Missing[0])
	}
	if s.WriteCount() != before {
		t.Fatal("Home summary wrote to the database")
	}
}

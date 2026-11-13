package discover

import (
	"context"
	"database/sql"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A title whose wanted copy could not be sent for want of a Usenet Downloader
// says so, until a Usenet Downloader is added.
func TestTitleNoDownloader(t *testing.T) {
	s := homeStore(t)
	ctx := context.Background()
	var titleID int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		ins := func(table string, v any) error { _, err := database.Insert(tx, table, v, domain.System); return err }
		ti := mediafiles.Title{Kind: "movie", Name: "Wanted", SortName: "wanted", MetadataSource: "tmdb", WhyAdded: "by_hand", State: "active", NewPartsPolicy: "all", ReleaseGate: "released", GateDriver: "none", Status: "released"}
		if err := ins("titles", &ti); err != nil {
			return err
		}
		titleID = ti.ID
		item := mediafiles.Item{TitleID: ti.ID, Kind: "movie", Name: &ti.Name}
		if err := ins("items", &item); err != nil {
			return err
		}
		v := mediafiles.Version{TitleID: ti.ID, Label: "Main", IsMain: true, LibraryFolderID: 1, PickRulesID: 1, FolderName: ti.Name}
		if err := ins("versions", &v); err != nil {
			return err
		}
		why := "not sent: no Usenet Downloader is set up"
		return ins("slots", &mediafiles.Slot{ItemID: item.ID, VersionID: v.ID, State: "wanted", LastReason: &why})
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := titleNoDownloader(ctx, s.Read(), titleID); got != "usenet" {
		t.Fatalf("got %q", got)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := database.Insert(tx, "downloaders", &domain.Downloader{Name: "SAB", Adapter: "sabnzbd", Protocol: "usenet", URL: "http://sab", Category: "c",
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := titleNoDownloader(ctx, s.Read(), titleID); got != "" {
		t.Fatalf("still says %q with SABnzbd added", got)
	}
}

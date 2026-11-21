package setup

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Under /api/v1/files/ the {id} of a title-decided route is a file, so the
// part comes from the file's title, not from a title that shares its number.
func TestByTitlePartReadsFileRoutesAsFiles(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var show, file int64
	err = st.Write(context.Background(), func(tx *sql.Tx) error {
		title := func(kind, name string) (int64, error) {
			return database.Insert(tx, "titles", &mediafiles.Title{Kind: kind, GateDriver: "none", Name: name, SortName: name, MetadataSource: "local", NewPartsPolicy: "all",
				ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{},
				MusicTypes: []string{}, Artwork: []string{}}, domain.System)
		}
		var err error
		if show, err = title("show", "a show"); err != nil {
			return err
		}
		movie, err := title("movie", "a movie")
		if err != nil {
			return err
		}
		var pr int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&pr)
		lf := &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", lf, domain.System); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: movie, Label: "HD", PickRulesID: pr, LibraryFolderID: lf.ID, FolderName: "a movie", IsMain: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		file, err = database.Insert(tx, "files", &mediafiles.File{SlotIDs: []int64{}, VersionID: v.ID, RelativePath: "a movie/a.mkv",
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "copy", Role: "main", State: "present"}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if file != show {
		t.Fatalf("the file (%d) and the show (%d) should share a number", file, show)
	}
	n := strconv.FormatInt(file, 10)
	if p, ok := byTitlePart(context.Background(), st.Read(), "GET /api/v1/files/{id}/download", "/api/v1/files/"+n+"/download"); !ok || p != parts.Movies {
		t.Fatalf("file route: %q %v", p, ok)
	}
	if p, ok := byTitlePart(context.Background(), st.Read(), "GET /api/v1/titles/{id}", "/api/v1/titles/"+n); !ok || p != parts.Shows {
		t.Fatalf("title route: %q %v", p, ok)
	}
	if _, ok := byTitlePart(context.Background(), st.Read(), "GET /api/v1/files/{id}/download", "/api/v1/files/x/download"); ok {
		t.Fatal("a bad id found a part")
	}
}

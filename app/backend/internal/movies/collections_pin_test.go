package movies

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Pins today's collection reads (movie_collections JOIN titles, versions, files
// and the downloads/imports/jobs look-ups) before they move to movies/store.

func t10AddMovies(t *testing.T, s *database.Store, n int, tmdbBase int) []int64 {
	t.Helper()
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		m := metadata.Movie{Name: fmt.Sprintf("Pin Movie %03d", i), Year: 2000 + i%20, IDs: map[string]string{"tmdb": fmt.Sprint(tmdbBase + i)}}
		id, err := library.AddMovie(context.Background(), s, m, library.AddOptions{}, nil, now, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func t10Collection(t *testing.T, s *database.Store, source, name string, titles []int64) int64 {
	t.Helper()
	var cid int64
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO movie_collections(source_id, name) VALUES (?, ?)", source, name)
		if err != nil {
			return err
		}
		cid, _ = res.LastInsertId()
		for _, id := range titles {
			if _, err := tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES (?, ?)", cid, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return cid
}

func TestT10PinListCollectionsFiltersAndOrders(t *testing.T) {
	s := open(t)
	addFolder(t, s)
	ctx := context.Background()
	ids := t10AddMovies(t, s, 6, 5000)
	// Order is lower(name); a collection with only removed movies, or none, is hidden.
	zeta := t10Collection(t, s, "tmdb:2", "zeta Saga", ids[0:2])
	alpha := t10Collection(t, s, "tmdb:1", "Alpha Saga", ids[2:4])
	t10Collection(t, s, "tmdb:3", "Gone Saga", ids[4:5])
	t10Collection(t, s, "tmdb:4", "Empty Saga", nil)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET state='removed' WHERE id=?", ids[4]); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE titles SET state='removed' WHERE id=?", ids[3])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ListCollections(ctx, s.Read())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != alpha || got[1].ID != zeta {
		t.Fatalf("collections %+v", got)
	}
	if got[0].OwnedTitles != 1 || len(got[0].TitleIDs) != 1 || got[0].TitleIDs[0] != ids[2] {
		t.Fatalf("alpha %+v", got[0])
	}
	if got[1].OwnedTitles != 2 || len(got[1].TitleIDs) != 2 {
		t.Fatalf("zeta %+v", got[1])
	}
}

func TestT10PinListCollectionsIgnoresNonMovieTitles(t *testing.T) {
	s := open(t)
	addFolder(t, s)
	ctx := context.Background()
	ids := t10AddMovies(t, s, 2, 5100)
	cid := t10Collection(t, s, "tmdb:10", "Mixed", ids)
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET kind='show' WHERE id=?", ids[1])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ListCollections(ctx, s.Read())
	if err != nil || len(got) != 1 || got[0].ID != cid || got[0].OwnedTitles != 1 {
		t.Fatalf("got %+v err %v", got, err)
	}
	d, err := CollectionDetails(ctx, s.Read(), cid)
	if err != nil || len(d.Titles) != 1 || d.Titles[0].ID != ids[0] || d.OwnedTitles != 1 {
		t.Fatalf("detail %+v err %v", d, err)
	}
}

func TestT10PinCollectionsLargeLibrary(t *testing.T) {
	s := open(t)
	addFolder(t, s)
	ctx := context.Background()
	const n = 300
	ids := t10AddMovies(t, s, n, 6000)
	cid := t10Collection(t, s, "tmdb:big", "Big Saga", ids)
	got, err := ListCollections(ctx, s.Read())
	if err != nil || len(got) != 1 || got[0].OwnedTitles != n || len(got[0].TitleIDs) != n {
		t.Fatalf("list %+v err %v", got, err)
	}
	d, err := CollectionDetails(ctx, s.Read(), cid)
	if err != nil || len(d.Titles) != n || d.OwnedTitles != n {
		t.Fatalf("detail %d err %v", len(d.Titles), err)
	}
	// One main Version each, no Files yet, and empty (not null) lists.
	for _, ti := range d.Titles {
		if len(ti.VersionIDs) != 1 || ti.Files == nil || len(ti.Files) != 0 || ti.Protected {
			t.Fatalf("title %+v", ti)
		}
	}
	for i := 1; i < len(d.Titles); i++ {
		if d.Titles[i-1].Name > d.Titles[i].Name {
			t.Fatalf("not sorted at %d", i)
		}
	}
}

func TestT10PinCollectionDetailsFilesNullYearAndProtected(t *testing.T) {
	_, s, cid, ids := collectionWorld(t)
	ctx := context.Background()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET year = NULL WHERE id = ?", ids[1]); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE files SET protected = 1 WHERE origin = 'adopted'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	d, err := CollectionDetails(ctx, s.Read(), cid)
	if err != nil {
		t.Fatal(err)
	}
	if d.SourceID != "tmdb:726871" || d.Name != "Dune Collection" || len(d.Titles) != 2 {
		t.Fatalf("detail %+v", d)
	}
	var withFiles, noYear *CollectionTitle
	for i := range d.Titles {
		if d.Titles[i].ID == ids[0] {
			withFiles = &d.Titles[i]
		} else {
			noYear = &d.Titles[i]
		}
	}
	// The partner title has no versions' files and a NULL year: year reads as 0.
	if noYear.Year != 0 || len(noYear.Files) != 0 || noYear.Protected {
		t.Fatalf("partner without files %+v", noYear)
	}
	if len(withFiles.Files) != 3 || !withFiles.Protected || len(withFiles.VersionIDs) != 1 {
		t.Fatalf("title with files %+v", withFiles)
	}
	for _, f := range withFiles.Files {
		if f.Protected != (f.Origin == "adopted") || f.State != "present" {
			t.Fatalf("file %+v", f)
		}
	}
	if _, err := CollectionDetails(ctx, s.Read(), 99999); !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("missing collection: %v", err)
	}
}

func TestT10PinPendingCollectionDownloads(t *testing.T) {
	_, s, _, ids := collectionWorld(t)
	ctx := context.Background()
	sl := slotsOf(t, s, ids[0])
	other := slotsOf(t, s, ids[1])
	mk := func(tx *sql.Tx, dr int64, ext, state string, slot int64) int64 {
		d := domain.Download{DownloaderID: dr, ExternalID: ext, SlotIDs: []int64{slot}, State: state, FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
		id, err := database.Insert(tx, "downloads", &d, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	var live, doneImp, openImp int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		dr := domain.Downloader{Adapter: "sabnzbd", Name: "Lab", Protocol: "usenet", URL: "http://lab.invalid", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", &dr, domain.System); err != nil {
			return err
		}
		live = mk(tx, dr.ID, "a", "downloading", sl[0].ID)
		doneImp = mk(tx, dr.ID, "b", "finished", sl[0].ID)
		openImp = mk(tx, dr.ID, "c", "seeding", sl[0].ID)
		mk(tx, dr.ID, "d", "cleaned", sl[0].ID)
		mk(tx, dr.ID, "e", "downloading", other[0].ID)
		for _, im := range []struct {
			dl    int64
			state string
		}{{doneImp, "done"}, {openImp, "planned"}} {
			if _, err := tx.Exec(`INSERT INTO imports(created_at, created_by, updated_at, updated_by, download_id, from_path, kind, mode, state)
				VALUES ('2026-10-13T12:00:00Z', '{}', '2026-10-13T12:00:00Z', '{}', ?, '/x', 'movie', 'auto', ?)`, im.dl, im.state); err != nil {
				return err
			}
		}
		for _, subj := range []string{
			fmt.Sprintf(`{"type":"download","id":%d}`, live),
			fmt.Sprintf(`{"type":"title","id":%d}`, live),
		} {
			if _, err := tx.Exec(`INSERT INTO jobs(created_at, created_by, updated_at, updated_by, kind, subject, state, run_after, asked_by)
				VALUES ('2026-10-13T12:00:00Z', '{}', '2026-10-13T12:00:00Z', '{}', 'import_download', ?, 'queued', '2026-10-13T12:00:00Z', '{}')`, subj); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := pendingCollectionDownloads(ctx, s.Read(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != live || got[1].ID != openImp {
		t.Fatalf("pending %+v", got)
	}
	if len(got[0].Jobs) != 1 || got[0].Imports == nil || len(got[0].Imports) != 0 {
		t.Fatalf("live %+v", got[0])
	}
	if len(got[1].Imports) != 1 || got[1].Imports[0].State != "planned" || got[1].Jobs == nil {
		t.Fatalf("open import %+v", got[1])
	}
	none, err := pendingCollectionDownloads(ctx, s.Read(), 424242)
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("no downloads: %v %v", none, err)
	}
}

// JSON golden: field names and shapes of the collection records.
func TestT10PinCollectionJSONGolden(t *testing.T) {
	d := CollectionDetail{
		MovieCollection: MovieCollection{ID: 7, SourceID: "tmdb:726871", Name: "Dune Collection", OwnedTitles: 2, TitleIDs: []int64{1, 2}},
		Titles: []CollectionTitle{
			{ID: 1, Name: "Dune", Year: 2021, State: "wanted", VersionIDs: []int64{3}, Files: []CollectionFile{{ID: 9, VersionID: 3, Path: "Dune (2021)/Dune.mkv", Origin: "imported", State: "present", Protected: true}}, Protected: true},
			{ID: 2, Name: "Nameless", State: "wanted", VersionIDs: []int64{}, Files: []CollectionFile{}},
		},
	}
	rev := CollectionReview{
		Collection:  d.MovieCollection,
		Titles:      []CollectionReviewTitle{{CollectionTitle: d.Titles[0], Selected: true}, {CollectionTitle: d.Titles[1], Skip: "protected"}},
		ReviewToken: "tok",
		Results:     []CollectionRemoveResult{{TitleID: 1, Name: "Dune", Status: "failed", Error: "boom"}},
	}
	b, err := json.MarshalIndent(map[string]any{
		"list":   []MovieCollection{d.MovieCollection, {ID: 8, SourceID: "tmdb:1", Name: "No ids", OwnedTitles: 0}},
		"detail": d,
		"review": rev,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t10Golden(t, "collections_pin.golden", b)
}

// t10Golden compares against testdata/<name>; PIN_UPDATE=1 writes it.
func t10Golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if os.Getenv("PIN_UPDATE") == "1" {
		if err := os.WriteFile(p, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("golden %s missing: %v", p, err)
	}
	if string(want) != string(got)+"\n" {
		t.Fatalf("golden %s differs:\n%s", p, got)
	}
}

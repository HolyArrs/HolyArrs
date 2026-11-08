package movies

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func collectionWorld(t *testing.T) (*Service, *database.Store, int64, []int64) {
	t.Helper()
	svc, s, _, first, _ := deleteWorld(t)
	secondMovie := dune
	secondMovie.Name = "Dune"
	secondMovie.Year = 2021
	secondMovie.IDs = map[string]string{"tmdb": "438631"}
	second, err := library.AddMovie(context.Background(), s, secondMovie, library.AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var collectionID int64
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO movie_collections(source_id, name) VALUES ('tmdb:726871', 'Dune Collection')")
		if err != nil {
			return err
		}
		collectionID, _ = res.LastInsertId()
		for _, id := range []int64{first, second} {
			if _, err := tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES (?, ?)", collectionID, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, s, collectionID, []int64{first, second}
}

func TestCollectionPreviewIsStableAndKeepsFilesByDefault(t *testing.T) {
	svc, s, cid, ids := collectionWorld(t)
	before := s.WriteCount()
	a, err := svc.PreviewCollectionRemoval(context.Background(), cid, nil, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.PreviewCollectionRemoval(context.Background(), cid, []int64{ids[1], ids[0]}, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if a.ReviewToken != b.ReviewToken || len(a.Titles) != 2 || a.Choice.DeleteFiles || a.Choice.NeverAdd {
		t.Fatalf("reviews differ or defaults changed: %+v %+v", a, b)
	}
	if s.WriteCount() != before {
		t.Fatal("preview wrote to the database")
	}
}

func TestCollectionConfirmationRejectsChangedMembershipAndChoices(t *testing.T) {
	svc, s, cid, ids := collectionWorld(t)
	r, err := svc.PreviewCollectionRemoval(context.Background(), cid, ids, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveCollection(context.Background(), cid, ids, library.RemoveChoice{NeverAdd: true}, r.ReviewToken, domain.System); !errors.Is(err, ErrCollectionReviewChanged) {
		t.Fatalf("changed choices: %v", err)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM movie_collection_titles WHERE collection_id = ? AND title_id = ?", cid, ids[1])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveCollection(context.Background(), cid, ids, library.RemoveChoice{}, r.ReviewToken, domain.System); !errors.Is(err, ErrCollectionReviewChanged) {
		t.Fatalf("changed membership: %v", err)
	}
}

func TestCollectionConfirmationRejectsChangedFileProtection(t *testing.T) {
	svc, s, cid, ids := collectionWorld(t)
	r, err := svc.PreviewCollectionRemoval(context.Background(), cid, ids, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE files SET protected = 1 WHERE id = (SELECT f.id FROM files f JOIN versions v ON v.id = f.version_id WHERE v.title_id = ? ORDER BY f.id LIMIT 1)`, ids[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveCollection(context.Background(), cid, ids, library.RemoveChoice{}, r.ReviewToken, domain.System); !errors.Is(err, ErrCollectionReviewChanged) {
		t.Fatalf("changed protection: %v", err)
	}
}

func TestCollectionConfirmationRejectsChangedPendingDownloadIdentity(t *testing.T) {
	svc, s, cid, ids := collectionWorld(t)
	ctx := context.Background()
	var firstDownload, secondDownload int64
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		var slotID int64
		if err := tx.QueryRow(`SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? ORDER BY s.id LIMIT 1`, ids[0]).Scan(&slotID); err != nil {
			return err
		}
		dr := &domain.Downloader{Name: "Collection test", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		for i, state := range []string{"downloading", "finished"} {
			d := &domain.Download{DownloaderID: dr.ID, SlotIDs: []int64{slotID}, ExternalID: "collection-" + string(rune('a'+i)), State: state,
				FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
			if _, err := database.Insert(tx, "downloads", d, domain.System); err != nil {
				return err
			}
			if i == 0 {
				firstDownload = d.ID
			} else {
				secondDownload = d.ID
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	review, err := svc.PreviewCollectionRemoval(ctx, cid, ids, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE downloads SET state = 'finished' WHERE id = ?", firstDownload); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE downloads SET state = 'downloading' WHERE id = ?", secondDownload)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveCollection(ctx, cid, ids, library.RemoveChoice{}, review.ReviewToken, domain.System); !errors.Is(err, ErrCollectionReviewChanged) {
		t.Fatalf("changed pending download identity: %v", err)
	}
}

func TestCollectionConfirmationRejectsChangedFinishedDownloadNeedsDecisionIdentity(t *testing.T) {
	svc, s, cid, ids := collectionWorld(t)
	ctx := context.Background()
	var firstImport, secondImport int64
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		var slotID int64
		if err := tx.QueryRow(`SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? ORDER BY s.id LIMIT 1`, ids[0]).Scan(&slotID); err != nil {
			return err
		}
		dr := &domain.Downloader{Name: "Collection finished test", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:1", Category: "holyarrs", Priority: 1,
			PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", dr, domain.System); err != nil {
			return err
		}
		for i, importState := range []string{"waiting_for_you", "rejected"} {
			d := &domain.Download{DownloaderID: dr.ID, SlotIDs: []int64{slotID}, ExternalID: "collection-finished-" + string(rune('a'+i)), State: "finished",
				FilesSeen: []domain.SeenFile{}, StartedBy: domain.System}
			if _, err := database.Insert(tx, "downloads", d, domain.System); err != nil {
				return err
			}
			im := &mediafiles.Import{DownloadID: &d.ID, FromPath: "download", Kind: "movie", Plan: []mediafiles.PlanStep{}, Mode: "auto", Result: []mediafiles.PlaceResult{}, State: importState}
			if _, err := database.Insert(tx, "imports", im, domain.System); err != nil {
				return err
			}
			if i == 0 {
				firstImport = im.ID
			} else {
				secondImport = im.ID
			}
		}
		_, err := tx.Exec("UPDATE slots SET state = 'needs_a_decision' WHERE id = ?", slotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	review, err := svc.PreviewCollectionRemoval(ctx, cid, ids, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE imports SET state = 'rejected' WHERE id = ?", firstImport); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE imports SET state = 'waiting_for_you' WHERE id = ?", secondImport)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveCollection(ctx, cid, ids, library.RemoveChoice{}, review.ReviewToken, domain.System); !errors.Is(err, ErrCollectionReviewChanged) {
		t.Fatalf("changed finished download needing a decision: %v", err)
	}
}

func TestCollectionProtectedTitleSkippedAsAWhole(t *testing.T) {
	svc, s, cid, ids := collectionWorld(t)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE files SET protected = 1 WHERE version_id IN (SELECT id FROM versions WHERE title_id = ?)`, ids[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.PreviewCollectionRemoval(context.Background(), cid, ids, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	var protectedReview *CollectionReviewTitle
	for i := range r.Titles {
		if r.Titles[i].ID == ids[0] {
			protectedReview = &r.Titles[i]
		}
	}
	if protectedReview == nil || !protectedReview.Protected || protectedReview.Skip == "" {
		t.Fatalf("protected title was not explained: %+v", r.Titles)
	}
	done, err := svc.RemoveCollection(context.Background(), cid, ids, library.RemoveChoice{}, r.ReviewToken, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[int64]string{}
	for _, result := range done.Results {
		statuses[result.TitleID] = result.Status
	}
	if len(done.Results) != 2 || statuses[ids[0]] != "skipped" || statuses[ids[1]] != "removed" {
		t.Fatalf("results %+v", done.Results)
	}
	var state string
	if err := s.Read().QueryRow("SELECT state FROM titles WHERE id = ?", ids[0]).Scan(&state); err != nil || state == "removed" {
		t.Fatalf("protected title state %q, err %v", state, err)
	}
}

func TestCollectionRejectsOutsideOrEmptySelection(t *testing.T) {
	svc, _, cid, _ := collectionWorld(t)
	for _, selected := range [][]int64{{}, {999999}} {
		if _, err := svc.PreviewCollectionRemoval(context.Background(), cid, selected, library.RemoveChoice{}); !errors.Is(err, ErrCollectionSelection) {
			t.Fatalf("selection %v: %v", selected, err)
		}
	}
}

func TestLargeCollectionSelectionIsDeterministicAndBounded(t *testing.T) {
	d := CollectionDetail{MovieCollection: MovieCollection{ID: 9, SourceID: "tmdb:large", Name: "Large"}}
	for id := int64(1); id <= 2000; id++ {
		d.Titles = append(d.Titles, CollectionTitle{ID: id, Name: "Movie", State: "active", VersionIDs: []int64{id}, Files: []CollectionFile{}})
	}
	selected, err := selectedCollectionTitles(d, nil)
	if err != nil || len(selected) != 2000 {
		t.Fatalf("selected %d, err %v", len(selected), err)
	}
	if a, b := collectionReviewToken(d, selected, library.RemoveChoice{}, nil), collectionReviewToken(d, append([]int64(nil), selected...), library.RemoveChoice{}, nil); a == "" || a != b {
		t.Fatalf("tokens %q %q", a, b)
	}
	tooMany := make([]int64, 5001)
	if _, err := selectedCollectionTitles(d, tooMany); !errors.Is(err, ErrCollectionSelection) {
		t.Fatalf("large selection: %v", err)
	}
}

func TestCollectionPartialFailureCanBeReviewedAndRetried(t *testing.T) {
	svc, s, lf, cid, ids := func() (*Service, *database.Store, mediafiles.LibraryFolder, int64, []int64) {
		svc, s, lf, first, files := deleteWorld(t)
		secondMovie := dune
		secondMovie.Name, secondMovie.Year, secondMovie.IDs = "Dune", 2021, map[string]string{"tmdb": "438631"}
		second, err := library.AddMovie(context.Background(), s, secondMovie, library.AddOptions{}, nil, now, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		var cid int64
		if err := s.Write(context.Background(), func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO movie_collections(source_id, name) VALUES ('tmdb:726871', 'Dune Collection')")
			if err != nil {
				return err
			}
			cid, _ = res.LastInsertId()
			for _, id := range []int64{first, second} {
				if _, err := tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES (?, ?)", cid, id); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(lf.Path, filepath.FromSlash(files[0].RelativePath))); err != nil {
			t.Fatal(err)
		}
		return svc, s, lf, cid, []int64{first, second}
	}()
	_ = lf
	r, err := svc.PreviewCollectionRemoval(context.Background(), cid, ids, library.RemoveChoice{DeleteFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	done, err := svc.RemoveCollection(context.Background(), cid, ids, library.RemoveChoice{DeleteFiles: true}, r.ReviewToken, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[int64]string{}
	for _, result := range done.Results {
		statuses[result.TitleID] = result.Status
	}
	if statuses[ids[0]] != "failed" || statuses[ids[1]] != "removed" {
		t.Fatalf("partial results %+v", done.Results)
	}
	retry, err := svc.PreviewCollectionRemoval(context.Background(), cid, []int64{ids[0]}, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveCollection(context.Background(), cid, []int64{ids[0]}, library.RemoveChoice{}, retry.ReviewToken, domain.System); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := s.Read().QueryRow("SELECT count(*) FROM titles WHERE id IN (?, ?) AND state != 'removed'", ids[0], ids[1]).Scan(&active); err != nil || active != 0 {
		t.Fatalf("active %d err %v", active, err)
	}
}

func TestCollectionRemovalPreservesSharedMembership(t *testing.T) {
	svc, s, cid, ids := collectionWorld(t)
	var other int64
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO movie_collections(source_id, name) VALUES ('tmdb:other', 'Other Collection')")
		if err != nil {
			return err
		}
		other, _ = res.LastInsertId()
		_, err = tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES (?, ?)", other, ids[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r, err := svc.PreviewCollectionRemoval(context.Background(), cid, []int64{ids[0]}, library.RemoveChoice{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveCollection(context.Background(), cid, []int64{ids[0]}, library.RemoveChoice{}, r.ReviewToken, domain.System); err != nil {
		t.Fatal(err)
	}
	var memberships int
	if err := s.Read().QueryRow("SELECT count(*) FROM movie_collection_titles WHERE title_id = ?", ids[0]).Scan(&memberships); err != nil || memberships != 2 {
		t.Fatalf("memberships %d err %v", memberships, err)
	}
}

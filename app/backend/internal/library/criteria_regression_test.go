package library

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func currentCriteriaCopy(t *testing.T, s *database.Store, titleID int64, facts map[string]any, protected bool) (mediafiles.Version, mediafiles.Slot, mediafiles.File) {
	t.Helper()
	ctx := context.Background()
	v := versionsOf(t, s, titleID)[0]
	slot := slotsOf(t, s, titleID)[0]
	release := "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.H.264-GRP"
	f := mediafiles.File{Protected: protected, SlotIDs: []int64{slot.ID}, VersionID: v.ID, RelativePath: filepath.ToSlash(filepath.Join(v.FolderName, release+".mkv")),
		AddedAt: now, Languages: []string{"en"}, MediaInfo: facts, ReleaseName: &release, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "files", &f, domain.System); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library', good_enough = 1 WHERE id = ?", f.ID, slot.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return v, slot, f
}

func TestEditVersionReevaluatesActualCopyCriteria(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change VersionChange
	}{
		{name: "edition", change: VersionChange{Edition: ptr("Director's Cut")}},
		{name: "audio", change: VersionChange{AudioLanguages: &[]string{"ja"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := open(t)
			addFolder(t, s)
			id, err := AddMovie(context.Background(), s, dune, AddOptions{}, nil, now, domain.System)
			if err != nil {
				t.Fatal(err)
			}
			_, slot, file := currentCriteriaCopy(t, s, id, map[string]any{
				"inspection_status": "complete", "edition": "Theatrical", "audio_languages": []string{"en"},
			}, true)
			if _, err := EditVersion(context.Background(), s, id, file.VersionID, tc.change, domain.System); err != nil {
				t.Fatal(err)
			}
			if err := database.Get(context.Background(), s.Read(), "slots", slot.ID, &slot); err != nil {
				t.Fatal(err)
			}
			if slot.State != mediafiles.SlotBetterWanted || slot.GoodEnough {
				t.Fatalf("mismatching protected copy stayed good enough: %+v", slot)
			}
			if err := database.Get(context.Background(), s.Read(), "files", file.ID, &file); err != nil || !file.Protected {
				t.Fatalf("protection changed: %+v %v", file, err)
			}
		})
	}
}

func TestEditVersionPreservesUnconstrainedQualityBehavior(t *testing.T) {
	s := open(t)
	addFolder(t, s)
	id, err := AddMovie(context.Background(), s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	v, slot, _ := currentCriteriaCopy(t, s, id, map[string]any{
		"inspection_status": "unsupported", "edition": "Theatrical", "audio_languages": []string{},
	}, false)
	keep := true
	if _, err := EditVersion(context.Background(), s, id, v.ID, VersionChange{KeepUpToDate: &keep}, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(context.Background(), s.Read(), "slots", slot.ID, &slot); err != nil {
		t.Fatal(err)
	}
	if slot.State != mediafiles.SlotInLibrary || !slot.GoodEnough {
		t.Fatalf("unconstrained copy no longer follows quality rules: %+v", slot)
	}
}

func TestVersionMutationsRejectRemovedTitle(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	id, err := AddMovie(context.Background(), s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	v := versionsOf(t, s, id)[0]
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddVersion(context.Background(), s, id, VersionInput{Label: "Archive", LibraryFolderID: lf.ID}, nil, now, domain.System); !errors.Is(err, ErrAlreadyRemoved) {
		t.Fatalf("AddVersion removed title: %v", err)
	}
	edition := "Theatrical"
	if _, err := EditVersion(context.Background(), s, id, v.ID, VersionChange{Edition: &edition}, domain.System); !errors.Is(err, ErrAlreadyRemoved) {
		t.Fatalf("EditVersion removed title: %v", err)
	}
}

func TestRescanPersistsActualLanguagesAndAuditActor(t *testing.T) {
	s := open(t)
	lf := addFolder(t, s)
	id, err := AddMovie(context.Background(), s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	v := versionsOf(t, s, id)[0]
	rel := filepath.ToSlash(filepath.Join(v.FolderName, "Dune.Part.Two.2024.English.1080p.WEB-DL.mkv"))
	writeInspectionMP4(t, filepath.Join(lf.Path, filepath.FromSlash(rel)))
	_, _, f := currentCriteriaCopy(t, s, id, map[string]any{"inspection_size": 0, "inspection_modified": "0"}, false)
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET relative_path = ?, languages = json('[\"en\"]'), updated_at = ?, updated_by = json(?) WHERE id = ?",
			rel, database.FormatTime(now.Add(-time.Hour)), ActorText(domain.System), f.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	actor := domain.Actor{Kind: "user", ID: 42}
	svc := &Service{Store: s, Now: func() time.Time { return now }}
	if _, err := svc.Rescan(context.Background(), actor); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(context.Background(), s.Read(), "files", f.ID, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Languages) != 1 || f.Languages[0] != "fr" {
		t.Fatalf("languages = %v, want inspected [fr]", f.Languages)
	}
	if !f.UpdatedAt.Equal(now) || f.UpdatedBy != actor {
		t.Fatalf("audit = %s %+v, want %s %+v", f.UpdatedAt, f.UpdatedBy, now, actor)
	}
}

func ptr(s string) *string { return &s }

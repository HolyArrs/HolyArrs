package library

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func fourKRules(t *testing.T, s *database.Store) int64 {
	t.Helper()
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.FourKName).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func versionsOf(t *testing.T, s *database.Store, titleID int64) []mediafiles.Version {
	t.Helper()
	vs, err := database.Select[mediafiles.Version](context.Background(), s.Read(), "versions", "title_id = ? ORDER BY id", titleID)
	if err != nil {
		t.Fatal(err)
	}
	return vs
}

func TestAddMovieWithHDAnd4KVersions(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	hd, uhd := addFolder(t, s), addFolder(t, s)
	pr4k := fourKRules(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{LibraryFolderID: hd.ID,
		MoreVersions: []VersionInput{{Label: "4K", PickRulesID: pr4k, LibraryFolderID: uhd.ID}}}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	vs, sl := versionsOf(t, s, id), slotsOf(t, s, id)
	if len(vs) != 2 || len(sl) != 2 {
		t.Fatalf("versions %+v slots %+v", vs, sl)
	}
	if !vs[0].IsMain || vs[1].IsMain || vs[1].Label != "4K" || vs[1].PickRulesID != pr4k || vs[1].LibraryFolderID != uhd.ID ||
		vs[1].FolderName != vs[0].FolderName {
		t.Fatalf("versions %+v", vs)
	}
	if sl[0].ItemID != sl[1].ItemID || sl[1].VersionID != vs[1].ID || sl[1].State != mediafiles.SlotWanted {
		t.Fatalf("slots %+v", sl)
	}
}

func TestAddVersionRules(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	pr4k := fourKRules(t, s)
	id, err := AddMovie(ctx, s, future, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// Same Library folder: the 4K copy gets its own folder name.
	v, err := AddVersion(ctx, s, id, VersionInput{Label: "4K", PickRulesID: pr4k, LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if v.FolderName != "The Lantern Keepers (2027) [4K]" || v.IsMain {
		t.Fatalf("version %+v", v)
	}
	sl := slotsOf(t, s, id)
	if len(sl) != 2 || sl[1].State != mediafiles.SlotNotOutYet {
		t.Fatalf("slots %+v", sl) // the gate is the Title's, so the new Slot waits too
	}
	if _, err := AddVersion(ctx, s, id, VersionInput{Label: "4k", LibraryFolderID: lf.ID}, nil, now, domain.System); !errors.Is(err, ErrVersionLabel) {
		t.Fatalf("same label: %v", err)
	}
	if _, err := AddVersion(ctx, s, id, VersionInput{Label: " "}, nil, now, domain.System); !errors.Is(err, ErrVersionLabel) {
		t.Fatalf("empty label: %v", err)
	}
	if _, err := AddVersion(ctx, s, id, VersionInput{Label: "x", PickRulesID: 999}, nil, now, domain.System); !errors.Is(err, ErrNoPickRules) {
		t.Fatalf("bad rules: %v", err)
	}
	if _, err := AddVersion(ctx, s, 999, VersionInput{Label: "x"}, nil, now, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("no title: %v", err)
	}

	// Editing one Version leaves the other alone; the title-wide edit changes only the main one.
	ge := versionsOf(t, s, id)[0].PickRulesID
	keep := false
	if _, err := EditVersion(ctx, s, id, v.ID, VersionChange{PickRulesID: &ge, KeepUpToDate: &keep}, domain.System); err != nil {
		t.Fatal(err)
	}
	vs := versionsOf(t, s, id)
	if vs[1].PickRulesID != ge || vs[1].KeepUpToDate || !vs[0].KeepUpToDate {
		t.Fatalf("after edit %+v", vs)
	}
	svc := &Service{Store: s, Now: func() time.Time { return now }}
	if _, err := svc.Edit(ctx, id, TitleChange{PickRulesID: &pr4k}, false, domain.System); err != nil {
		t.Fatal(err)
	}
	vs = versionsOf(t, s, id)
	if vs[0].PickRulesID != pr4k || vs[1].PickRulesID != ge {
		t.Fatalf("title edit should change only the main Version: %+v", vs)
	}
}

func TestRemoveVersion(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	id, _ := AddMovie(ctx, s, dune, AddOptions{MoreVersions: []VersionInput{{Label: "4K", LibraryFolderID: lf.ID}}}, nil, now, domain.System)
	vs := versionsOf(t, s, id)
	if err := RemoveVersion(ctx, s, id, vs[0].ID, domain.System); !errors.Is(err, ErrMainVersion) {
		t.Fatalf("main: %v", err)
	}
	if err := RemoveVersion(ctx, s, 999, vs[1].ID, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("wrong title: %v", err)
	}
	if err := RemoveVersion(ctx, s, id, vs[1].ID, domain.System); err != nil {
		t.Fatal(err)
	}
	if len(versionsOf(t, s, id)) != 1 || len(slotsOf(t, s, id)) != 1 {
		t.Fatal("4K Version or its Slot is still there")
	}
}

func TestRemoveVersionRefusesWhileBusy(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addFolder(t, s)
	id, _ := AddMovie(ctx, s, dune, AddOptions{MoreVersions: []VersionInput{{Label: "4K", LibraryFolderID: lf.ID}}}, nil, now, domain.System)
	v := versionsOf(t, s, id)[1]
	exec := func(q string, args ...any) {
		t.Helper()
		if err := s.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
			t.Fatal(err)
		}
	}
	// A Slot downloading: refused.
	exec("UPDATE slots SET state = 'downloading' WHERE version_id = ?", v.ID)
	if err := RemoveVersion(ctx, s, id, v.ID, domain.System); !errors.Is(err, ErrVersionBusy) {
		t.Fatalf("downloading: %v", err)
	}
	exec("UPDATE slots SET state = 'wanted' WHERE version_id = ?", v.ID)
	// A file that is missing (it may come back): refused.
	exec(`INSERT INTO files (created_at, created_by, updated_at, updated_by, version_id, relative_path, added_at, origin, placed_by, state)
		VALUES ('2026-01-01T00:00:00Z', 'system', '2026-01-01T00:00:00Z', 'system', ?, 'a.mkv', '2026-01-01T00:00:00Z', 'imported', 'move', 'missing')`, v.ID)
	if err := RemoveVersion(ctx, s, id, v.ID, domain.System); !errors.Is(err, ErrVersionHasFiles) {
		t.Fatalf("missing file: %v", err)
	}
	// Only gone or replaced files: removed.
	exec("UPDATE files SET state = 'gone' WHERE version_id = ?", v.ID)
	if err := RemoveVersion(ctx, s, id, v.ID, domain.System); err != nil {
		t.Fatal(err)
	}
}

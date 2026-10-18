package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

func presetID(t *testing.T, s *store.Store, name string) int64 {
	t.Helper()
	var id int64
	if err := s.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPickRulesCreateUpdateDelete(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	in := PickRulesInput{Name: "Small files", Kinds: []string{"movie"}, Body: domain.GoodEveryday()}
	p, err := CreatePickRules(ctx, s, in, domain.System)
	if err != nil || p.ID == 0 || p.Revision != 1 || p.Preset {
		t.Fatalf("create: %+v %v", p, err)
	}
	if _, err := CreatePickRules(ctx, s, PickRulesInput{Name: "small FILES", Kinds: []string{"movie"}, Body: domain.GoodEveryday()}, domain.System); !errors.Is(err, ErrPickRulesName) {
		t.Fatalf("taken name: %v", err)
	}
	bad := in
	bad.Body.Order = "random"
	var fe *domain.FieldError
	if _, err := CreatePickRules(ctx, s, bad, domain.System); !errors.As(err, &fe) || fe.Field != "body.order" {
		t.Fatalf("bad body: %v", err)
	}
	in.Name = "Smaller files"
	in.Body.MinPoints = 5
	p, err = UpdatePickRules(ctx, s, p.ID, in, domain.System)
	if err != nil || p.Name != "Smaller files" || p.Revision != 2 || p.Body.MinPoints != 5 {
		t.Fatalf("update: %+v %v", p, err)
	}
	if _, err := UpdatePickRules(ctx, s, 9999, in, domain.System); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if err := DeletePickRules(ctx, s, p.ID, domain.System); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := DeletePickRules(ctx, s, p.ID, domain.System); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("delete again: %v", err)
	}
}

func TestPickRulesPresetLock(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id := presetID(t, s, domain.GoodEverydayName)
	body := domain.GoodEveryday()
	body.MinPoints = 7
	if _, err := UpdatePickRules(ctx, s, id, PickRulesInput{Name: "Renamed", Kinds: []string{"movie", "show"}, Body: body}, domain.System); !errors.Is(err, ErrPresetLocked) {
		t.Fatalf("rename preset: %v", err)
	}
	if _, err := UpdatePickRules(ctx, s, id, PickRulesInput{Name: domain.GoodEverydayName, Kinds: []string{"movie"}, Body: body}, domain.System); !errors.Is(err, ErrPresetLocked) {
		t.Fatalf("narrow preset kinds: %v", err)
	}
	var p domain.PickRules
	_ = store.Get(ctx, s.Read(), "pick_rules", id, &p)
	p2, err := UpdatePickRules(ctx, s, id, PickRulesInput{Name: p.Name, Kinds: p.Kinds, Body: body}, domain.System)
	if err != nil || p2.Body.MinPoints != 7 || !p2.Preset {
		t.Fatalf("preset body change: %+v %v", p2, err)
	}
	if err := DeletePickRules(ctx, s, id, domain.System); !errors.Is(err, ErrPresetLocked) {
		t.Fatalf("delete preset: %v", err)
	}
}

func TestPickRulesUseCountsEveryReference(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	p, err := CreatePickRules(ctx, s, PickRulesInput{Name: "Mine", Kinds: []string{"movie", "show"}, Body: domain.GoodEveryday()}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if u, err := PickRulesUse(ctx, s, p.ID); err != nil || u.Total() != 0 {
		t.Fatalf("unused: %+v %v", u, err)
	}
	lf := addFolder(t, s)
	err = s.Write(ctx, func(tx *sql.Tx) error {
		stmts := []string{
			fmt.Sprintf("UPDATE library_folders SET default_pick_rules_id = %d WHERE id = %d", p.ID, lf.ID),
			fmt.Sprintf(`INSERT INTO outside_lists(created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id, pick_rules_id)
				VALUES ('x', '{}', 'x', '{}', 'L', 'tmdb_list', '1', %d, %d)`, lf.ID, p.ID),
		}
		for _, k := range []string{"ask_pick_rules_id", "ask_show_pick_rules_id"} {
			stmts = append(stmts, fmt.Sprintf(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', '%s', '%d', 'x', '{}', 'x', '{}')`, k, p.ID))
		}
		for _, k := range []string{"movie_pick_rules_id", "show_pick_rules_id"} {
			stmts = append(stmts, fmt.Sprintf(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', '%s', '%d', 'x', '{}', 'x', '{}')`, k, p.ID))
		}
		for _, q := range stmts {
			if _, err := tx.Exec(q); err != nil {
				return fmt.Errorf("%s: %w", q, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddMovie(ctx, s, dune, AddOptions{PickRulesID: p.ID}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	u, err := PickRulesUse(ctx, s, p.ID)
	want := Use{Versions: 1, LibraryFolders: 1, OutsideLists: 1, Wishlist: 2, TitleDefaults: 2}
	if err != nil || u != want || u.Total() != 7 {
		t.Fatalf("use %+v want %+v (%v)", u, want, err)
	}
	if err := DeletePickRules(ctx, s, p.ID, domain.System); !errors.Is(err, ErrPickRulesInUse) {
		t.Fatalf("delete in use: %v", err)
	}
	// Dropping show, which the show ask setting and show default use, is refused.
	if _, err := UpdatePickRules(ctx, s, p.ID, PickRulesInput{Name: "Mine", Kinds: []string{"movie"}, Body: domain.GoodEveryday()}, domain.System); !errors.Is(err, ErrPickRulesInUse) {
		t.Fatalf("drop used kind: %v", err)
	}
	// Switching to the music ladder is refused.
	if _, err := UpdatePickRules(ctx, s, p.ID, PickRulesInput{Name: "Mine", Kinds: []string{"music"}, Body: domain.LosslessFirst()}, domain.System); !errors.Is(err, ErrPickRulesInUse) {
		t.Fatalf("switch ladder: %v", err)
	}
}

func TestPickRulesUnusedKindMayBeDropped(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	p, _ := CreatePickRules(ctx, s, PickRulesInput{Name: "Mine", Kinds: []string{"movie", "show"}, Body: domain.GoodEveryday()}, domain.System)
	addFolder(t, s)
	if _, err := AddMovie(ctx, s, dune, AddOptions{PickRulesID: p.ID}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdatePickRules(ctx, s, p.ID, PickRulesInput{Name: "Mine", Kinds: []string{"movie"}, Body: domain.GoodEveryday()}, domain.System); err != nil {
		t.Fatalf("drop unused show: %v", err)
	}
}

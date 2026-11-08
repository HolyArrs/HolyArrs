package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func presetID(t *testing.T, s *database.Store, name string) int64 {
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
	in := PickRulesInput{Name: "Small files", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}
	p, err := CreatePickRules(ctx, s, in, domain.System)
	if err != nil || p.ID == 0 || p.Revision != 1 || p.Preset {
		t.Fatalf("create: %+v %v", p, err)
	}
	if _, err := CreatePickRules(ctx, s, PickRulesInput{Name: "small FILES", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}, domain.System); !errors.Is(err, ErrPickRulesName) {
		t.Fatalf("taken name: %v", err)
	}
	bad := in
	bad.Body.Order = "random"
	var fe *mediafiles.FieldError
	if _, err := CreatePickRules(ctx, s, bad, domain.System); !errors.As(err, &fe) || fe.Field != "body.order" {
		t.Fatalf("bad body: %v", err)
	}
	in.Name = "Smaller files"
	in.Body.MinPoints = 5
	p, err = UpdatePickRules(ctx, s, p.ID, in, domain.System)
	if err != nil || p.Name != "Smaller files" || p.Revision != 2 || p.Body.MinPoints != 5 {
		t.Fatalf("update: %+v %v", p, err)
	}
	if _, err := UpdatePickRules(ctx, s, 9999, in, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if err := DeletePickRules(ctx, s, p.ID, domain.System); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := DeletePickRules(ctx, s, p.ID, domain.System); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("delete again: %v", err)
	}
}

func TestPickRulesPresetLock(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	id := presetID(t, s, mediafiles.GoodEverydayName)
	body := mediafiles.GoodEveryday()
	body.MinPoints = 7
	if _, err := UpdatePickRules(ctx, s, id, PickRulesInput{Name: "Renamed", Kinds: []string{"movie", "show"}, Body: body}, domain.System); !errors.Is(err, ErrPresetLocked) {
		t.Fatalf("rename preset: %v", err)
	}
	if _, err := UpdatePickRules(ctx, s, id, PickRulesInput{Name: mediafiles.GoodEverydayName, Kinds: []string{"movie"}, Body: body}, domain.System); !errors.Is(err, ErrPresetLocked) {
		t.Fatalf("narrow preset kinds: %v", err)
	}
	var p mediafiles.PickRules
	_ = database.Get(ctx, s.Read(), "pick_rules", id, &p)
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
	p, err := CreatePickRules(ctx, s, PickRulesInput{Name: "Mine", Kinds: []string{"movie", "show"}, Body: mediafiles.GoodEveryday()}, domain.System)
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
	if _, err := UpdatePickRules(ctx, s, p.ID, PickRulesInput{Name: "Mine", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}, domain.System); !errors.Is(err, ErrPickRulesInUse) {
		t.Fatalf("drop used kind: %v", err)
	}
	// Switching to the music ladder is refused.
	if _, err := UpdatePickRules(ctx, s, p.ID, PickRulesInput{Name: "Mine", Kinds: []string{"music"}, Body: mediafiles.LosslessFirst()}, domain.System); !errors.Is(err, ErrPickRulesInUse) {
		t.Fatalf("switch ladder: %v", err)
	}
}

func TestPickRulesUnusedKindMayBeDropped(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	p, _ := CreatePickRules(ctx, s, PickRulesInput{Name: "Mine", Kinds: []string{"movie", "show"}, Body: mediafiles.GoodEveryday()}, domain.System)
	addFolder(t, s)
	if _, err := AddMovie(ctx, s, dune, AddOptions{PickRulesID: p.ID}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdatePickRules(ctx, s, p.ID, PickRulesInput{Name: "Mine", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}, domain.System); err != nil {
		t.Fatalf("drop unused show: %v", err)
	}
}

// TestPickRulesUsesMatchesOneByOne: the counts for every rule at once equal
// each rule's own count.
func TestPickRulesUsesMatchesOneByOne(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	mk := func(name string) int64 {
		p, err := CreatePickRules(ctx, s, PickRulesInput{Name: name, Kinds: []string{"movie", "show"}, Body: mediafiles.GoodEveryday()}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		return p.ID
	}
	a, b, c := mk("A"), mk("B"), mk("C")
	lf1, lf2 := addFolder(t, s), addFolder(t, s)
	err := s.Write(ctx, func(tx *sql.Tx) error {
		stmts := []string{
			fmt.Sprintf("UPDATE library_folders SET default_pick_rules_id = %d WHERE id = %d", a, lf1.ID),
			fmt.Sprintf("UPDATE library_folders SET default_pick_rules_id = %d WHERE id = %d", b, lf2.ID),
			fmt.Sprintf(`INSERT INTO outside_lists(created_at, created_by, updated_at, updated_by, name, kind, ref, library_folder_id, pick_rules_id)
				VALUES ('x', '{}', 'x', '{}', 'L1', 'tmdb_list', '1', %d, %d), ('x', '{}', 'x', '{}', 'L2', 'tmdb_list', '2', %d, %d)`, lf1.ID, b, lf1.ID, b),
			fmt.Sprintf(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
				('this_app', 'ask_pick_rules_id', '%d', 'x', '{}', 'x', '{}'), ('this_app', 'ask_show_pick_rules_id', '%d', 'x', '{}', 'x', '{}'),
				('what_to_pick', 'show_pick_rules_id', '%d', 'x', '{}', 'x', '{}'), ('what_to_pick', 'movie_pick_rules_id', '%d', 'x', '{}', 'x', '{}'),
				('this_app', 'movie_pick_rules_id', '%d', 'x', '{}', 'x', '{}')`, a, b, b, c, a),
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
	if _, err := AddMovie(ctx, s, dune, AddOptions{PickRulesID: a}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	other := metadata.Movie{Name: "Quiet Orchard", Year: 2022, IDs: map[string]string{"tmdb": "900003"}}
	if _, err := AddMovie(ctx, s, other, AddOptions{PickRulesID: a}, nil, now, domain.System); err != nil {
		t.Fatal(err)
	}
	all, err := PickRulesUses(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	ps, err := database.Select[mediafiles.PickRules](ctx, s.Read(), "pick_rules", "1=1 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	used := 0
	for _, p := range ps {
		one, err := PickRulesUse(ctx, s, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if all[p.ID] != one {
			t.Fatalf("rule %d %s: all at once %+v, one by one %+v", p.ID, p.Name, all[p.ID], one)
		}
		if one.Total() > 0 {
			used++
		}
	}
	if used < 3 {
		t.Fatalf("only %d rules in use; the test needs several", used)
	}
}

// TestPickRulesUsedByShowList: an Outside list counts with the kind it
// adds. Show-only rules used by a show list stay editable, and rules for
// movies and shows used by a show list cannot drop show.
func TestPickRulesUsedByShowList(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf := addShowFolder(t, s)
	showOnly, _ := CreatePickRules(ctx, s, PickRulesInput{Name: "Shows only", Kinds: []string{"show"}, Body: mediafiles.GoodEveryday()}, domain.System)
	both, _ := CreatePickRules(ctx, s, PickRulesInput{Name: "Both", Kinds: []string{"movie", "show"}, Body: mediafiles.GoodEveryday()}, domain.System)
	err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(fmt.Sprintf(`INSERT INTO outside_lists(created_at, created_by, updated_at, updated_by, name, kind, title_kind, ref, library_folder_id, pick_rules_id)
			VALUES ('x', '{}', 'x', '{}', 'S1', 'tmdb_list', 'show', '1', %d, %d), ('x', '{}', 'x', '{}', 'S2', 'tmdb_list', 'show', '2', %d, %d)`, lf.ID, showOnly.ID, lf.ID, both.ID))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if u, err := PickRulesUse(ctx, s, showOnly.ID); err != nil || u.OutsideLists != 1 {
		t.Fatalf("use %+v %v", u, err)
	}
	if _, err := UpdatePickRules(ctx, s, showOnly.ID, PickRulesInput{Name: "Shows only, renamed", Kinds: []string{"show"}, Body: mediafiles.GoodEveryday()}, domain.System); err != nil {
		t.Fatalf("edit show-only rules used by a show list: %v", err)
	}
	if _, err := UpdatePickRules(ctx, s, both.ID, PickRulesInput{Name: "Both", Kinds: []string{"movie"}, Body: mediafiles.GoodEveryday()}, domain.System); !errors.Is(err, ErrPickRulesInUse) {
		t.Fatalf("dropped show while a show list uses it: %v", err)
	}
	if _, err := UpdatePickRules(ctx, s, both.ID, PickRulesInput{Name: "Both", Kinds: []string{"show"}, Body: mediafiles.GoodEveryday()}, domain.System); err != nil {
		t.Fatalf("drop unused movie: %v", err)
	}
}

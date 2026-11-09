package accounts

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func twoMembers(t *testing.T, f *fixture) (int64, int64) {
	t.Helper()
	var a, b int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		var err error
		if a, err = insertUserTx(tx, &domain.User{Name: "ann", Role: "member", State: "active", Perms: "[]"}, domain.System); err != nil {
			return err
		}
		b, err = insertUserTx(tx, &domain.User{Name: "bob", Role: "member", State: "active", Perms: "[]"}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

func TestPruneViewCountsDropsOldDays(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	m, _ := twoMembers(t, f)
	const old, fresh, cut = "2026-01-01", "2026-11-01", "2026-06-01"
	err := f.s.Write(ctx, func(tx *sql.Tx) error {
		for _, d := range []string{old, fresh} {
			if err := AddRowCounts(ctx, tx, m, d, "shown", []string{"trending"}); err != nil {
				return err
			}
			if err := AddLayerCounts(ctx, tx, m, d, "opens", []string{"genre"}); err != nil {
				return err
			}
			if err := AddCategoryCount(ctx, tx, m, "action", d, "shown"); err != nil {
				return err
			}
		}
		return PruneViewCounts(ctx, tx, cut)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"explore_counts", "explore_title_counts", "explore_category_counts"} {
		if n := countRows(t, f.s, "SELECT count(*) FROM "+tbl+" WHERE day < '"+cut+"'"); n != 0 {
			t.Fatalf("%s: %d old rows left", tbl, n)
		}
		if n := countRows(t, f.s, "SELECT count(*) FROM "+tbl+" WHERE day >= '"+cut+"'"); n != 1 {
			t.Fatalf("%s: %d recent rows", tbl, n)
		}
	}
}

func TestCategoryCountsResetIsPerMember(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	m1, m2 := twoMembers(t, f)
	const day = "2026-11-01"
	err := f.s.Write(ctx, func(tx *sql.Tx) error {
		if err := AddCategoryCount(ctx, tx, m1, "action", day, "opened"); err != nil {
			return err
		}
		if err := AddCategoryCount(ctx, tx, m2, "action", day, "opened"); err != nil {
			return err
		}
		// A second bump adds to the same row.
		return BumpCategoryCount(ctx, tx, m1, "action", day, "opened")
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := CategoryKeysOn(ctx, f.s.Read(), m1, day); err != nil || n != 1 {
		t.Fatalf("keys on the day: %d %v", n, err)
	}
	got, err := CategoryCountsSince(ctx, f.s.Read(), m1, "2026-01-01")
	if err != nil || len(got) != 1 || got[0].Key != "action" || got[0].Opened != 2 {
		t.Fatalf("member 1 counts: %+v %v", got, err)
	}
	// Bumping a key with no row makes none.
	if err := f.s.Write(ctx, func(tx *sql.Tx) error { return BumpCategoryCount(ctx, tx, m1, "drama", day, "opened") }); err != nil {
		t.Fatal(err)
	}
	if n, _ := CategoryKeysOn(ctx, f.s.Read(), m1, day); n != 1 {
		t.Fatalf("bump made a row: %d", n)
	}
	if err := f.s.Write(ctx, func(tx *sql.Tx) error { return ResetCategoryCounts(ctx, tx, m1) }); err != nil {
		t.Fatal(err)
	}
	if got, _ := CategoryCountsSince(ctx, f.s.Read(), m1, "2026-01-01"); len(got) != 0 {
		t.Fatalf("member 1 not cleared: %+v", got)
	}
	got, _ = CategoryCountsSince(ctx, f.s.Read(), m2, "2026-01-01")
	if len(got) != 1 || got[0].Opened != 1 {
		t.Fatalf("member 2 touched: %+v", got)
	}
}

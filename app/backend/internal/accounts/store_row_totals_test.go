package accounts

import (
	"context"
	"database/sql"
	"testing"
)

// Row totals add every person's counts per row type, only from the given
// day on, busiest row first, and say nothing about who did what.
func TestRowTotals(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	a, b := twoMembers(t, f)
	type add struct {
		member     int64
		day, field string
		row        string
	}
	adds := []add{
		{a, "2026-11-01", "shown", "trending"},
		{b, "2026-11-01", "shown", "trending"},
		{a, "2026-11-01", "opens", "trending"},
		{b, "2026-11-02", "opens", "hidden"},
		{b, "2026-11-02", "opens", "hidden"},
		{a, "2026-11-02", "asks", "hidden"},
		{a, "2026-10-01", "opens", "classics"}, // before the span
		{a, "2026-11-03", "shown", "mood"},
	}
	err := f.s.Write(ctx, func(tx *sql.Tx) error {
		for _, x := range adds {
			if err := AddRowCounts(ctx, tx, x.member, x.day, x.field, []string{x.row}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := RowTotals(ctx, f.s.Read(), "2026-10-15")
	if err != nil {
		t.Fatal(err)
	}
	want := []RowTotal{
		{Row: "hidden", Opens: 2, Asks: 1},
		{Row: "trending", Shown: 2, Opens: 1},
		{Row: "mood", Shown: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	empty, err := RowTotals(ctx, f.s.Read(), "2027-01-01")
	if err != nil || len(empty) != 0 {
		t.Fatalf("future span: %+v %v", empty, err)
	}
}

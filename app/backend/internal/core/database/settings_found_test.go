package database

import (
	"context"
	"database/sql"
	"sort"
	"testing"
	"time"
)

func TestSettingFoundTellsEmptyFromMissing(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	now := time.Now()
	if err := s.Write(ctx, func(tx *sql.Tx) error { return PutSetting(tx, "links", "empty", "", System, now) }); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := SettingFound(ctx, s.Read(), "links", "empty"); v != "" || !ok || err != nil {
		t.Fatalf("empty row: %q %v %v", v, ok, err)
	}
	if v, ok, err := SettingFound(ctx, s.Read(), "links", "nope"); v != "" || ok || err != nil {
		t.Fatalf("missing row: %q %v %v", v, ok, err)
	}
	_ = s.Close()
	if _, ok, err := SettingFound(ctx, s.Read(), "links", "empty"); ok || err == nil {
		t.Fatalf("closed db: %v %v", ok, err)
	}
}

func TestSettingsListsEveryRow(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	now := time.Now()
	var rows []SettingRow
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := PutSetting(tx, "links", "k1", "v1", System, now); err != nil {
			return err
		}
		if err := PutSetting(tx, "alerts", "k2", "v2", System, now); err != nil {
			return err
		}
		var err error
		rows, err = Settings(tx)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Grp+rows[i].Key < rows[j].Grp+rows[j].Key })
	want := map[SettingRow]bool{{"links", "k1", "v1"}: false, {"alerts", "k2", "v2"}: false}
	for _, r := range rows {
		if _, ok := want[r]; ok {
			want[r] = true
		}
	}
	for r, seen := range want {
		if !seen {
			t.Errorf("missing %+v in %+v", r, rows)
		}
	}
}

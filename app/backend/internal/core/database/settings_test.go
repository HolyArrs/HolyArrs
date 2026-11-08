package database

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestSettingHelpers(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if v, err := Setting(ctx, s.Read(), "this_app", "x"); v != "" || err != nil {
		t.Fatalf("missing: %q %v", v, err)
	}
	if v := SettingOr(ctx, s.Read(), "this_app", "x", "d"); v != "d" {
		t.Fatalf("default: %q", v)
	}
	now := time.Now()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := PutSettingIfMissing(tx, "this_app", "x", "1", System, now); err != nil {
			return err
		}
		if err := PutSettingIfMissing(tx, "this_app", "x", "2", System, now); err != nil {
			return err
		}
		if err := PutSetting(tx, "this_app", "y", "a", Actor{Kind: "user", ID: 3}, now); err != nil {
			return err
		}
		if err := PutSetting(tx, "this_app", "y", "b", System, now); err != nil {
			return err
		}
		return ReplaceSettingValue(tx, "this_app", "x", "3", now)
	}); err != nil {
		t.Fatal(err)
	}
	if v, _ := Setting(ctx, s.Read(), "this_app", "x"); v != "3" {
		t.Fatalf("x = %q", v)
	}
	if v := SettingOr(ctx, s.Read(), "this_app", "y", "d"); v != "b" {
		t.Fatalf("y = %q", v)
	}
	var by string
	_ = s.Read().QueryRow("SELECT created_by FROM settings WHERE key = 'x'").Scan(&by)
	if by != `{"kind":"system"}` {
		t.Fatalf("created_by %s", by)
	}
}

func TestEachSettingReadsOneGroup(t *testing.T) {
	s, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now()
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := PutSetting(tx, "links", "pin_a", "1", System, now); err != nil {
			return err
		}
		if err := PutSetting(tx, "links", "pin_b", "2", System, now); err != nil {
			return err
		}
		return PutSetting(tx, "alerts", "pin_a", "9", System, now)
	}); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	if err := EachSetting(ctx, s.Read(), "links", func(k, v string) { got[k] = v }); err != nil {
		t.Fatal(err)
	}
	if got["pin_a"] != "1" || got["pin_b"] != "2" {
		t.Fatalf("got %v", got)
	}
}

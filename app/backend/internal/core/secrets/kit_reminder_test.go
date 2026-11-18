package secrets

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The check value alone is not something worth protecting; the first real
// secret is, and its time is when it was saved.
func TestFirstSavedSkipsCheckValue(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir)
	key, _ := LoadKey(dir, "")
	v, err := Open(context.Background(), s, key)
	if err != nil {
		t.Fatal(err)
	}
	if at, err := FirstSaved(context.Background(), s); err != nil || !at.IsZero() {
		t.Fatalf("only the check value: %v %v, want zero", at, err)
	}
	// A setting HolyArrs saves for itself (its metadata server sign-up).
	if err := v.SetSetting(context.Background(), "links", "lab_install_token", "lab-token", database.System); err != nil {
		t.Fatal(err)
	}
	if at, err := FirstSaved(context.Background(), s); err != nil || !at.IsZero() {
		t.Fatalf("only HolyArrs' own setting: %v %v, want zero", at, err)
	}
	before := time.Now().Add(-time.Second)
	if _, err := v.Put(context.Background(), "source_api_key", "lab-value"); err != nil {
		t.Fatal(err)
	}
	at, err := FirstSaved(context.Background(), s)
	if err != nil || at.Before(before) || at.After(time.Now().Add(time.Second)) {
		t.Fatalf("after one secret: %v %v", at, err)
	}
}

func TestKitReminderLevels(t *testing.T) {
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		name           string
		secret, backup time.Time
		byHand         bool
		want           string
	}{
		{"fresh install", time.Time{}, time.Time{}, false, ""},
		{"only the first scheduled backup", time.Time{}, now.Add(-30 * day), false, ""},
		{"first secret", now.Add(-30 * day), time.Time{}, false, KitRecommend},
		{"first backup by hand", time.Time{}, now.Add(-30 * day), true, KitRecommend},
		{"both, recent", now.Add(-2 * day), now.Add(-day), false, KitRecommend},
		{"both, backup recent", now.Add(-30 * day), now.Add(-6 * day), false, KitRecommend},
		{"both, past the wait", now.Add(-8 * day), now.Add(-KitWarnAfter), false, KitWarn},
	}
	for _, c := range cases {
		if got := KitReminder(c.secret, c.backup, c.byHand, now); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

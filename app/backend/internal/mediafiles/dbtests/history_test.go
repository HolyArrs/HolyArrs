package dbtests

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestEventsFilterAndPaging(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	one, two := int64(1), int64(2)
	for i, e := range []domain.Event{
		{Kind: "title_added", TitleID: &one, At: day},
		{Kind: "import_done", TitleID: &one, At: day.Add(24 * time.Hour)},
		{Kind: "import_failed", TitleID: &two, At: day.Add(48 * time.Hour)},
		{Kind: "renamed", TitleID: &two, At: day.Add(72 * time.Hour)},
		{Kind: "import_x_y", At: day.Add(96 * time.Hour)},
	} {
		e.Summary = e.Kind
		if err := s.AddEvent(ctx, &e); err != nil {
			t.Fatal(i, err)
		}
	}
	kinds := func(es []domain.Event) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.Kind)
		}
		return out
	}
	cases := []struct {
		name  string
		f     database.EventFilter
		want  []string
		total int
	}{
		{"all newest first", database.EventFilter{}, []string{"import_x_y", "renamed", "import_failed", "import_done", "title_added"}, 5},
		{"by movie", database.EventFilter{TitleID: 1}, []string{"import_done", "title_added"}, 2},
		{"by kind", database.EventFilter{Kinds: []string{"renamed", "title_added"}}, []string{"renamed", "title_added"}, 2},
		{"by prefix", database.EventFilter{Kinds: []string{"import_*"}}, []string{"import_x_y", "import_failed", "import_done"}, 3},
		// _ in a prefix is a plain character, not a LIKE wildcard.
		{"prefix underscore is literal", database.EventFilter{Kinds: []string{"import_x_*"}}, []string{"import_x_y"}, 1},
		{"date range", database.EventFilter{From: day.Add(24 * time.Hour), To: day.Add(72 * time.Hour)}, []string{"import_failed", "import_done"}, 2},
		{"paging", database.EventFilter{Limit: 2, Offset: 2}, []string{"import_failed", "import_done"}, 5},
		{"movie and kind", database.EventFilter{TitleID: 2, Kinds: []string{"import_*"}}, []string{"import_failed"}, 1},
	}
	for _, c := range cases {
		es, total, err := s.Events(ctx, c.f)
		if err != nil {
			t.Fatal(c.name, err)
		}
		if got := kinds(es); total != c.total || len(got) != len(c.want) {
			t.Errorf("%s: got %v (total %d), want %v (total %d)", c.name, got, total, c.want, c.total)
		} else {
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("%s: got %v, want %v", c.name, got, c.want)
					break
				}
			}
		}
	}
}

func TestOnEventHearsEveryStoredEvent(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	var mu sync.Mutex
	var heard []string
	s.OnEvent(func(e domain.Event) {
		mu.Lock()
		heard = append(heard, e.Kind)
		mu.Unlock()
	})
	if err := s.AddEvent(context.Background(), &domain.Event{Kind: "renamed", Summary: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(heard) != 1 || heard[0] != "renamed" {
		t.Fatalf("heard %v", heard)
	}
}

func TestAfterCommitRunsOnlyAfterACommit(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	ran := 0
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		database.AfterCommit(tx, func() { ran++ })
		if ran != 0 {
			t.Error("ran before the commit")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if ran != 1 {
		t.Fatalf("ran %d times after a commit", ran)
	}
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		database.AfterCommit(tx, func() { ran++ })
		return errors.New("roll back")
	})
	if ran != 1 {
		t.Fatal("ran after a rollback")
	}
}

// A Slot that moves to needs_a_decision leaves a needs_decision Event with
// its movie and reason, once the move has committed.
func TestNeedsDecisionEvent(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	ctx := context.Background()
	var slotID, titleID int64
	err := s.Write(ctx, func(tx *sql.Tx) error {
		var prID int64
		_ = tx.QueryRow("SELECT id FROM pick_rules LIMIT 1").Scan(&prID)
		lf := &mediafiles.LibraryFolder{Path: "/m", Kind: "movie", Label: "M", Health: "ok", SameDiskAs: []int64{}}
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Arrival", SortName: "arrival", MetadataSource: "local", NewPartsPolicy: "all",
			ReleaseGate: "released", WhyAdded: "by_hand", Status: "released", State: "active"}
		if _, err := database.Insert(tx, "library_folders", lf, owner); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "titles", ti, owner); err != nil {
			return err
		}
		titleID = ti.ID
		it := &mediafiles.Item{TitleID: ti.ID, Kind: "movie"}
		if _, err := database.Insert(tx, "items", it, owner); err != nil {
			return err
		}
		ve := &mediafiles.Version{TitleID: ti.ID, Label: "HD", PickRulesID: prID, LibraryFolderID: lf.ID, FolderName: "A"}
		if _, err := database.Insert(tx, "versions", ve, owner); err != nil {
			return err
		}
		var err error
		slotID, err = database.Insert(tx, "slots", &mediafiles.Slot{ItemID: it.ID, VersionID: ve.ID, State: mediafiles.SlotImporting}, owner)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mediafiles.MoveSlot(ctx, s, slotID, mediafiles.SlotNeedsADecision, "nothing could be placed", owner); err != nil {
		t.Fatal(err)
	}
	es, _, err := s.Events(ctx, database.EventFilter{Kinds: []string{domain.EventNeedsDecision}})
	if err != nil || len(es) != 1 {
		t.Fatalf("events %v %v", es, err)
	}
	e := es[0]
	if e.TitleID == nil || *e.TitleID != titleID || e.SlotID == nil || *e.SlotID != slotID || e.Data["reason"] != "nothing could be placed" || e.Summary != "Needs a decision: Arrival" {
		t.Fatalf("event %+v", e)
	}
}

// Migration 0019 rebuilds the alerts table for the webhook kind: every
// Alert and its settings stay.
func TestAlertsKeptByWebhookMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 18)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO alerts(id, created_at, created_by, updated_at, updated_by, name, kind, moments, url_secret_id, smtp_host, staff_to, allow_local_network, last_error)
			VALUES (7, '2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', 'Family', 'email', '["ask_ready"]', 3, 'mail.test', 'a@b.test', 1, 'boom')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var a domain.Alert
	if err := database.Get(ctx, s.Read(), "alerts", 7, &a); err != nil {
		t.Fatal(err)
	}
	if a.Name != "Family" || a.Kind != "email" || len(a.Moments) != 1 || a.URLSecretID == nil || *a.URLSecretID != 3 || a.SMTPHost != "mail.test" ||
		a.StaffTo != "a@b.test" || !a.AllowLocal || a.LastError != "boom" || a.WebhookAuth != "sign" {
		t.Fatalf("alert after the migration %+v", a)
	}
}

// TestAlertsKeptByTelegramMigration: 0082 rebuilds alerts to add the
// telegram kind and chat_id; every Alert keeps all it had.
func TestAlertsKeptByTelegramMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 81)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO alerts(id, created_at, created_by, updated_at, updated_by, name, kind, enabled, moments, url_secret_id, password_secret_id,
			allow_local_network, last_sent_at, last_error, webhook_auth)
			VALUES (9, '2026-10-01T00:00:00Z', '{}', '2026-10-02T00:00:00Z', '{}', 'Home hook', 'webhook', 0, '["grab"]', 4, 5, 1, '2026-10-03T00:00:00Z', 'boom', 'bearer')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	// All migrations: domain.Alert reads columns later ones add.
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var a domain.Alert
	if err := database.Get(ctx, s.Read(), "alerts", 9, &a); err != nil {
		t.Fatal(err)
	}
	if a.Name != "Home hook" || a.Kind != "webhook" || a.Enabled || len(a.Moments) != 1 || a.URLSecretID == nil || *a.URLSecretID != 4 ||
		a.PasswordSecretID == nil || *a.PasswordSecretID != 5 || !a.AllowLocal || a.LastSentAt == nil || a.LastError != "boom" || a.WebhookAuth != "bearer" || a.ChatID != "" {
		t.Fatalf("alert after the migration %+v", a)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO alerts(created_at, created_by, updated_at, updated_by, name, kind, chat_id) VALUES ('c', '{}', 'u', '{}', 'TG', 'telegram', '-42')`)
		return err
	})
	if err != nil {
		t.Fatalf("a telegram Alert: %v", err)
	}
}

// TestAlertsKeptByMoreAppsMigration: 0084 rebuilds alerts to add the ntfy,
// gotify and pushover kinds and their columns; every Alert keeps all it had,
// a Telegram chat included.
func TestAlertsKeptByMoreAppsMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 83)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO alerts(id, created_at, created_by, updated_at, updated_by, name, kind, enabled, moments, url_secret_id, password_secret_id,
			allow_local_network, last_sent_at, last_error, webhook_auth, chat_id)
			VALUES (9, '2026-10-01T00:00:00Z', '{}', '2026-10-02T00:00:00Z', '{}', 'Family TG', 'telegram', 0, '["grab"]', 4, 5, 1, '2026-10-03T00:00:00Z', 'boom', 'bearer', '-42'),
			(10, '2026-10-01T00:00:00Z', '{}', '2026-10-02T00:00:00Z', '{}', 'Mail', 'email', 1, '["ask_ready"]', NULL, NULL, 0, NULL, '', 'sign', '')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var a domain.Alert
	if err := database.Get(ctx, s.Read(), "alerts", 9, &a); err != nil {
		t.Fatal(err)
	}
	if a.Name != "Family TG" || a.Kind != "telegram" || a.Enabled || len(a.Moments) != 1 || a.URLSecretID == nil || *a.URLSecretID != 4 ||
		a.PasswordSecretID == nil || *a.PasswordSecretID != 5 || !a.AllowLocal || a.LastSentAt == nil || a.LastError != "boom" || a.WebhookAuth != "bearer" ||
		a.ChatID != "-42" || a.ServerURL != "" || a.Topic != "" || a.UserKey != "" {
		t.Fatalf("alert after the migration %+v", a)
	}
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM alerts").Scan(&n); err != nil || n != 2 {
		t.Fatalf("alerts %d %v", n, err)
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO alerts(created_at, created_by, updated_at, updated_by, name, kind, server_url, topic, user_key) VALUES
			('c', '{}', 'u', '{}', 'Phone', 'ntfy', 'https://ntfy.sh', 'holyarrs', ''),
			('c', '{}', 'u', '{}', 'Home', 'gotify', 'http://gotify.lan', '', ''),
			('c', '{}', 'u', '{}', 'Push', 'pushover', '', '', 'uQiRzpo4DXghDmr9QzzfQu27cmVRsG')`)
		return err
	})
	if err != nil {
		t.Fatalf("the new kinds: %v", err)
	}
}

// TestSettingsKeptByAlertsGroupMigration: 0023 rebuilds settings to add the
// alerts group; every row stays and pack_alerts moves from links to alerts.
func TestSettingsKeptByAlertsGroupMigration(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 22)})
	if err != nil {
		t.Fatal(err)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings(grp, key, value, created_at, created_by, updated_at, updated_by) VALUES
			('links', 'pack_alerts', 'each', 'c', '{}', 'u', '{}'), ('this_app', 'odd_key', 'v', 'c', '{}', 'u', '{}')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var before int
	_ = old.Read().QueryRow("SELECT COUNT(*) FROM settings").Scan(&before)
	_ = old.Close()
	// Up to 0023 only: later migrations seed settings of their own.
	s, err := database.Open(dir, database.Options{MainMigrations: upTo(t, 23)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var after int
	_ = s.Read().QueryRow("SELECT COUNT(*) FROM settings").Scan(&after)
	var grp, v string
	_ = s.Read().QueryRow("SELECT grp, value FROM settings WHERE key = 'pack_alerts'").Scan(&grp, &v)
	var odd string
	_ = s.Read().QueryRow("SELECT value FROM settings WHERE grp = 'this_app' AND key = 'odd_key'").Scan(&odd)
	if after != before || grp != "alerts" || v != "each" || odd != "v" {
		t.Fatalf("rows %d -> %d, pack_alerts in %q = %q, odd %q", before, after, grp, v, odd)
	}
	if _, err := s.Read().Exec("SELECT 1"); err != nil {
		t.Fatal(err)
	}
}

// A hook that crashes is caught: the Event is stored and the next hook hears it.
func TestOnEventHookCrashIsContained(t *testing.T) {
	s, _ := openTest(t, database.Options{})
	heard := 0
	s.OnEvent(func(domain.Event) { panic("hook crash") })
	s.OnEvent(func(domain.Event) { heard++ })
	if err := s.AddEvent(context.Background(), &domain.Event{Kind: "renamed", Summary: "x"}); err != nil {
		t.Fatal(err)
	}
	if heard != 1 {
		t.Fatalf("second hook heard %d", heard)
	}
}

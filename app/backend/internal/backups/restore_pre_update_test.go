package backups

import (
	"context"
	"fmt"
	"testing"
	"testing/fstest"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// events returns events migrations 1..n.
func events(n int) fstest.MapFS {
	fs := fstest.MapFS{}
	for i := 1; i <= n; i++ {
		fs[fmt.Sprintf("%04d_t.sql", i)] = &fstest.MapFile{Data: []byte(fmt.Sprintf("CREATE TABLE t%d(x);", i))}
	}
	return fs
}

// startAt opens dir as build version v with n events migrations and runs
// what a good start runs.
func startAt(t *testing.T, dir, v string, n int) *Manager {
	t.Helper()
	s, err := database.Open(dir, database.Options{EventsMigrations: events(n)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m := &Manager{Store: s, Dir: dir, Version: v}
	if _, err := m.AdoptMigrationCopies(); err != nil {
		t.Fatal(err)
	}
	if err := m.AfterStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	return m
}

// Scenario 3: after the first good start following an update the pre-update
// backup is still there, listed and named for the update.
func TestPreUpdateBackupSurvivesTheFirstGoodStart(t *testing.T) {
	dir := t.TempDir()
	startAt(t, dir, "1.0.0", 1)
	m := startAt(t, dir, "1.1.0", 2)
	all, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	var pre []Info
	for _, b := range all {
		if b.Kind == KindPreUpdate {
			pre = append(pre, b)
		}
	}
	if len(pre) != 1 || pre[0].Detail != "before updating from 1.0.0 to 1.1.0" {
		t.Fatalf("pre-update backups %+v (all %+v)", pre, all)
	}
	// A later start of the same build keeps it too.
	m = startAt(t, dir, "1.1.0", 2)
	if kept(t, m, KindPreUpdate) != 1 {
		t.Fatal("pre-update backup dropped on a plain restart")
	}
}

// Scenario 4: several updates in a row keep the newest three.
func TestThreeNewestPreUpdateBackupsKept(t *testing.T) {
	dir := t.TempDir()
	startAt(t, dir, "1.0.0", 1)
	var m *Manager
	for i := 2; i <= 5; i++ {
		m = startAt(t, dir, fmt.Sprintf("1.%d.0", i-1), i)
	}
	all, _ := m.List()
	var got []string
	for _, b := range all {
		if b.Kind == KindPreUpdate {
			got = append(got, b.Detail)
		}
	}
	want := []string{"before updating from 1.3.0 to 1.4.0", "before updating from 1.2.0 to 1.3.0", "before updating from 1.1.0 to 1.2.0"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("kept %q, want %q", got, want)
	}
	if err := m.Prune(Balanced); err != nil || kept(t, m, KindPreUpdate) != 3 {
		t.Fatalf("prune touched pre-update backups: %v", err)
	}
}

// Left-over update backups (made before older ones were dropped) go on the
// next good start even when no update ran, so only the newest stays.
func TestOldUpdateBackupsDroppedWithoutAnUpdate(t *testing.T) {
	dir := t.TempDir()
	startAt(t, dir, "1.0.0", 1)
	m := &Manager{Dir: dir}
	now := time.Now().UTC()
	makeAt(t, m, KindUpdate, now.Add(-3*time.Hour), now.Add(-2*time.Hour), now.Add(-time.Hour))
	m = startAt(t, dir, "1.0.0", 1)
	if n := kept(t, m, KindUpdate); n != 1 {
		t.Fatalf("%d update backups kept, want 1", n)
	}
	all, _ := m.List()
	for _, b := range all {
		if b.Kind == KindUpdate && !b.CreatedAt.Equal(now.Add(-time.Hour).Truncate(time.Millisecond)) {
			t.Fatalf("kept %s, want the newest", b.Name)
		}
	}
}

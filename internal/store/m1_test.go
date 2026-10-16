package store

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

var (
	sqlNonCodePattern      = regexp.MustCompile(`(?s)'(?:''|[^'])*'|"(?:""|[^"])*"|\x60(?:\x60\x60|[^\x60])*\x60|\[[^\]]*\]|--[^\r\n]*|/\*.*?\*/`)
	ftsTablePattern        = regexp.MustCompile(`(?i)\bCREATE\s+VIRTUAL\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+([A-Z_][A-Z0-9_]*)\s+USING\s+FTS5\s*\(`)
	triggerBodyPattern     = regexp.MustCompile(`(?is)\bCREATE\s+TRIGGER\b.*?\bBEGIN\b(.*?)\bEND\s*;`)
	deleteFromPattern      = regexp.MustCompile(`(?i)\bDELETE\s+FROM\b`)
	unquotedSQLNameAtStart = regexp.MustCompile(`(?i)^\s*([A-Z_][A-Z0-9_]*)\s*(?:WHERE\b|;|$)`)
)

// stage4 is the main migration set as it was after Stage 4 (up to 0006).
func stage4(t *testing.T) fs.FS {
	t.Helper()
	src, _ := fs.Sub(embedded, "migrations/main")
	ms, err := loadMigrations(src)
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	for _, m := range ms {
		if m.version <= 6 {
			out[m.name] = &fstest.MapFile{Data: []byte(m.sql)}
		}
	}
	return out
}

// TestM1RollForwardFromStage4 opens a database made by the Stage 4 schema,
// writes old-style rows, then opens it with this build: the old rows keep
// working, the new tables and columns are there, and a backup was taken.
func TestM1RollForwardFromStage4(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	old, err := Open(dir, Options{MainMigrations: stage4(t)})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := old.Version(); v != 6 {
		t.Fatalf("stage 4 version %d", v)
	}
	err = old.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, metadata_source, why_added)
			VALUES ('2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', '2026-10-01T00:00:00.000000000Z', '{"kind":"system"}', 'movie', 'Old Movie', 'old movie', 'tmdb', 'by_hand')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = old.Close()

	s, err := Open(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	mainV, _ := SchemaVersions()
	if v, _ := s.Version(); v != mainV || v < 7 {
		t.Fatalf("version %d, want %d (>= 7)", v, mainV)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "backups", "main-before-0007-*.db"))
	if len(backups) != 1 {
		t.Fatalf("backups before 0007: %v", backups)
	}
	ts, err := Select[domain.Title](ctx, s.Read(), "titles", "")
	if err != nil || len(ts) != 1 {
		t.Fatalf("titles %v %v", ts, err)
	}
	ti := ts[0]
	if ti.Name != "Old Movie" || ti.CinemaDate != nil || ti.DigitalDate != nil || ti.PhysicalDate != nil ||
		ti.GateDriver != "none" || ti.GateOpensAt != nil || ti.DroppedAt != nil {
		t.Fatalf("old title after roll forward %+v", ti)
	}

	// The new columns and tables take writes and read back.
	cinema := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	ti.CinemaDate, ti.GateDriver = &cinema, "cinema_plus_90"
	opens := cinema.AddDate(0, 0, 90)
	ti.GateOpensAt = &opens
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if err := Update(tx, "titles", &ti, domain.System); err != nil {
			return err
		}
		_, err := Insert(tx, "never_add", &domain.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": "42"}, Name: "Skip Me", Reason: "test"}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var back domain.Title
	if err := Get(ctx, s.Read(), "titles", ti.ID, &back); err != nil || back.CinemaDate == nil || !back.CinemaDate.Equal(cinema) || !back.GateOpensAt.Equal(opens) {
		t.Fatalf("title back %+v %v", back, err)
	}
	na, err := Select[domain.NeverAdd](ctx, s.Read(), "never_add", "json_extract(source_ids, '$.tmdb') = ?", "42")
	if err != nil || len(na) != 1 || na[0].Name != "Skip Me" {
		t.Fatalf("never_add %v %v", na, err)
	}
	// A bad gate driver is refused by the database.
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET gate_driver = 'whenever' WHERE id = ?", ti.ID)
		return err
	})
	if err == nil {
		t.Fatal("bad gate_driver accepted")
	}
	// Library folders keep a reason for their health.
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := Insert(tx, "library_folders", &domain.LibraryFolder{Path: dir, Kind: "movie", Label: "M", Health: "missing", HealthReason: "gone", SameDiskAs: []int64{}}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestM1Seeds(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	want := map[string]string{"release_gate": "released", "gate_delay_days": "0", "bulk_search_respects_gate": "true"}
	for k, v := range want {
		var got string
		if err := s.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'what_to_pick' AND key = ?", k).Scan(&got); err != nil || got != v {
			t.Errorf("setting %s = %q (%v), want %q", k, got, err, v)
		}
	}
	scheds, err := Select[domain.Schedule](ctx, s.Read(), "schedules", "1=1 ORDER BY kind")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, sc := range scheds {
		got[sc.Kind] = sc.EverySeconds
		if !sc.Enabled {
			t.Errorf("schedule %s off", sc.Kind)
		}
	}
	if got["refresh_titles"] != 86400 || got["check_library_folders"] != 21600 {
		t.Fatalf("schedules %v", got)
	}
}

func TestM1AddOnlyDeleteGuard(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		bad  bool
	}{
		{
			name: "virtual table maintenance inside a trigger",
			sql: `CREATE VIRTUAL TABLE title_search USING fts5(name);
CREATE TRIGGER title_search_delete AFTER DELETE ON titles BEGIN
    DELETE
      FROM title_search WHERE rowid = old.id;
END;`,
		},
		{
			name: "non-FTS virtual table maintenance inside a trigger",
			sql: `CREATE VIRTUAL TABLE paths USING rtree(id, min_x, max_x);
CREATE TRIGGER paths_delete AFTER DELETE ON titles BEGIN
    DELETE FROM paths WHERE id = old.id;
END;`,
			bad: true,
		},
		{
			name: "top-level delete from a virtual table",
			sql: `CREATE VIRTUAL TABLE title_search USING fts5(name);
DELETE FROM title_search;`,
			bad: true,
		},
		{
			name: "trigger delete from a stored table",
			sql: `CREATE TABLE old_titles (id INTEGER PRIMARY KEY);
CREATE TRIGGER remove_old_titles AFTER INSERT ON titles BEGIN
    DELETE FROM old_titles WHERE id = new.id;
END;`,
			bad: true,
		},
		{
			name: "top-level delete from a stored table",
			sql:  `DELETE FROM titles WHERE kind = 'movie';`,
			bad:  true,
		},
		{
			name: "quoted top-level delete from a stored table",
			sql:  `DELETE FROM "titles" WHERE kind = 'movie';`,
			bad:  true,
		},
		{
			name: "qualified top-level delete from a stored table",
			sql:  `DELETE FROM main.titles WHERE kind = 'movie';`,
			bad:  true,
		},
		{
			name: "qualified trigger target with an FTS prefix",
			sql: `CREATE VIRTUAL TABLE title_search USING fts5(name);
CREATE TRIGGER remove_old_titles AFTER INSERT ON titles BEGIN
    DELETE FROM title_search.old_titles WHERE id = new.id;
END;`,
			bad: true,
		},
		{
			name: "fake FTS declaration in a line comment",
			sql: `-- CREATE VIRTUAL TABLE titles USING fts5(name);
CREATE TRIGGER remove_titles AFTER INSERT ON items BEGIN
    DELETE FROM titles WHERE id = new.title_id;
END;`,
			bad: true,
		},
		{
			name: "fake FTS declaration in a block comment",
			sql: `/* CREATE VIRTUAL TABLE titles USING fts5(name); */
CREATE TRIGGER remove_titles AFTER INSERT ON items BEGIN
    DELETE FROM titles WHERE id = new.title_id;
END;`,
			bad: true,
		},
		{
			name: "fake FTS declaration in a string",
			sql: `SELECT 'CREATE VIRTUAL TABLE titles USING fts5(name);';
CREATE TRIGGER remove_titles AFTER INSERT ON items BEGIN
    DELETE FROM titles WHERE id = new.title_id;
END;`,
			bad: true,
		},
		{
			name: "fake FTS declaration in a double-quoted string",
			sql: `SELECT "CREATE VIRTUAL TABLE titles USING fts5(name);";
CREATE TRIGGER remove_titles AFTER INSERT ON items BEGIN
    DELETE FROM titles WHERE id = new.title_id;
END;`,
			bad: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := migrationHasDestructiveDelete(tt.sql); got != tt.bad {
				t.Fatalf("migrationHasDestructiveDelete() = %v, want %v", got, tt.bad)
			}
		})
	}
}

func migrationHasDestructiveDelete(sqlText string) bool {
	// SQL-looking words in comments or values do not declare tables or triggers.
	sqlText = sqlNonCodePattern.ReplaceAllString(sqlText, " ")
	ftsTables := map[string]bool{}
	for _, match := range ftsTablePattern.FindAllStringSubmatch(sqlText, -1) {
		ftsTables[strings.ToUpper(match[1])] = true
	}
	triggerBodies := triggerBodyPattern.FindAllStringSubmatchIndex(sqlText, -1)
	for _, match := range deleteFromPattern.FindAllStringIndex(sqlText, -1) {
		target := unquotedSQLNameAtStart.FindStringSubmatch(sqlText[match[1]:])
		if target == nil {
			return true
		}
		table := strings.ToUpper(target[1])
		inTriggerBody := false
		for _, body := range triggerBodies {
			if match[0] >= body[2] && match[0] < body[3] {
				inTriggerBody = true
				break
			}
		}
		if !inTriggerBody || !ftsTables[table] {
			return true
		}
	}
	return false
}

// TestM1MigrationsAreAddOnly guards old rows: M1 migrations only add.
func TestM1MigrationsAreAddOnly(t *testing.T) {
	src, _ := fs.Sub(embedded, "migrations/main")
	ms, _ := loadMigrations(src)
	for _, m := range ms {
		// 0008 rebuilds users, sessions and API keys to widen the role
		// check; TestAccountsKeepTheOwner proves no row is lost.
		// 0019 rebuilds alerts to add the webhook kind; TestAlertsKeptByWebhookMigration
		// proves no Alert is lost.
		// 0021 rebuilds asks (with ask_people and messages) to add the show
		// kind; TestShowAsksMigrationKeepsAsks proves no row is lost.
		// 0023 rebuilds settings to add the alerts group;
		// TestSettingsKeptByAlertsGroupMigration proves no row is lost.
		// 0025 rebuilds asks again to add the artist kind;
		// TestMusicAsksMigrationKeepsAsks proves no row is lost.
		// 0042 rebuilds downloaders to widen its adapter check;
		// TestOptionalDownloadersMigrationKeepsData proves no row is lost.
		if m.version < 7 || m.version == 8 || m.version == 19 || m.version == 21 || m.version == 23 || m.version == 25 || m.version == 42 {
			continue
		}
		up := strings.ToUpper(m.sql)
		for _, bad := range []string{"DROP TABLE", "DROP COLUMN"} {
			if strings.Contains(up, bad) {
				t.Errorf("%s contains %s", m.name, bad)
			}
		}
		// Trigger maintenance may remove stale rows from a derived virtual
		// table. Deletes outside such a trigger, or from stored tables, remain
		// destructive migration work and fail this guard.
		if migrationHasDestructiveDelete(m.sql) {
			t.Errorf("%s contains a destructive DELETE FROM", m.name)
		}
	}
}

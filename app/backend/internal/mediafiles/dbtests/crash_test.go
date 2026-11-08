package dbtests_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// The crash-loop test is the closest local stand-in for pulling the power:
// a child process runs an update (pre-update backup, then migrations) and
// is killed at a random moment; the next start must leave sound data that
// equals a clean run.

const (
	crashHelperEnv = "HOLYARRS_CRASH_HELPER" // mode:dir for the child
	crashRunsEnv   = "HOLYARRS_CRASH_RUNS"
	crashSeedEnv   = "HOLYARRS_CRASH_SEED"
)

// crashBase seeds 50k child rows over 500 parents, plus the settings table
// the backup manager reads after a good start.
var crashBase = fstest.MapFS{
	"0001_base.sql": {Data: []byte(`
CREATE TABLE settings(grp TEXT NOT NULL, key TEXT NOT NULL, value TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT '', created_by TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT '', updated_by TEXT NOT NULL DEFAULT '', PRIMARY KEY(grp, key));
CREATE TABLE parent(id INTEGER PRIMARY KEY, name TEXT NOT NULL);
CREATE TABLE child(id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parent(id), body TEXT NOT NULL, n INTEGER NOT NULL);
WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM c WHERE i < 500)
INSERT INTO parent(id, name) SELECT i, 'parent-' || i FROM c;
WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM c WHERE i < 50000)
INSERT INTO child(id, parent_id, body, n) SELECT i, (i % 500) + 1, 'body-' || i || '-' || hex(zeroblob(40)), i * 7 FROM c;
CREATE INDEX child_parent ON child(parent_id);
`)},
}

// crashSteps are the synthetic update: a table rebuild, a batched-style
// rewrite and an index rebuild.
var crashSteps = map[string]string{
	"0002_rebuild.sql": `
CREATE TABLE child_new(id INTEGER PRIMARY KEY, parent_id INTEGER NOT NULL REFERENCES parent(id),
	body TEXT NOT NULL, n INTEGER NOT NULL, kind TEXT NOT NULL DEFAULT 'plain');
INSERT INTO child_new(id, parent_id, body, n, kind) SELECT id, parent_id, body, n, CASE WHEN n % 3 = 0 THEN 'three' ELSE 'plain' END FROM child;
DROP TABLE child;
ALTER TABLE child_new RENAME TO child;
CREATE INDEX child_parent ON child(parent_id);
`,
	"0003_rewrite.sql": `
UPDATE child SET body = upper(body) || ':' || n WHERE id BETWEEN 1 AND 10000;
UPDATE child SET body = upper(body) || ':' || n WHERE id BETWEEN 10001 AND 20000;
UPDATE child SET body = upper(body) || ':' || n WHERE id BETWEEN 20001 AND 30000;
UPDATE child SET body = upper(body) || ':' || n WHERE id BETWEEN 30001 AND 40000;
UPDATE child SET body = upper(body) || ':' || n WHERE id > 40000;
UPDATE parent SET name = name || '-v3';
`,
	"0004_index.sql": `
DROP INDEX child_parent;
CREATE INDEX child_parent_kind ON child(parent_id, kind, n);
CREATE INDEX child_body ON child(body);
REINDEX;
`,
}

// crashFS is the base plus the first upTo-1 update steps (upTo = 4: all).
func crashFS(upTo int) fstest.MapFS {
	m := fstest.MapFS{}
	for k, v := range crashBase {
		m[k] = v
	}
	for name, sql := range crashSteps {
		if v, _ := strconv.Atoi(name[:4]); v <= upTo {
			m[name] = &fstest.MapFile{Data: []byte(sql)}
		}
	}
	return m
}

// TestCrashHelper is the child process; it does nothing in a normal run.
func TestCrashHelper(t *testing.T) {
	spec := os.Getenv(crashHelperEnv)
	if spec == "" {
		t.Skip("crash helper: only runs as a child process")
	}
	mode, dir, _ := strings.Cut(spec, ":")
	switch mode {
	case "update":
		s, err := database.Open(dir, database.Options{MainMigrations: crashFS(4)})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		_ = s.Close()
	case "apply":
		if _, err := backups.ApplyStaged(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
	}
	os.Exit(0)
}

// startChild re-runs this test binary as the helper.
func startChild(t *testing.T, mode, dir string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashHelper$")
	cmd.Env = append(os.Environ(), crashHelperEnv+"="+mode+":"+dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

// state is what must match a clean run.
type state struct {
	version int
	counts  string
	hash    string
}

func readState(t *testing.T, path string) state {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var st state
	if err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&st.version); err != nil {
		t.Fatal(err)
	}
	var np, nc int
	_ = db.QueryRow("SELECT COUNT(*) FROM parent").Scan(&np)
	_ = db.QueryRow("SELECT COUNT(*) FROM child").Scan(&nc)
	st.counts = fmt.Sprintf("parent=%d child=%d", np, nc)
	h := sha256.New()
	for _, q := range []string{
		"SELECT type || ' ' || name || ' ' || COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name",
		"SELECT version || ' ' || name FROM schema_migrations ORDER BY version",
		"SELECT id || '|' || name FROM parent ORDER BY id",
		"SELECT id || '|' || parent_id || '|' || body || '|' || n FROM child ORDER BY id",
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			h.Write([]byte(s + "\n"))
		}
		rows.Close()
	}
	if st.version >= 2 { // the kind column exists from step 2
		rows, err := db.Query("SELECT kind FROM child ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			h.Write([]byte(s))
		}
		rows.Close()
	}
	st.hash = hex.EncodeToString(h.Sum(nil))
	return st
}

func checkSound(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var res string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&res); err != nil || res != "ok" {
		t.Fatalf("integrity_check = %q (%v)", res, err)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check is not empty")
	}
}

// openBase makes a config folder at schema 1 (no backup: new data).
func openBase(t *testing.T, dir string) {
	t.Helper()
	s, err := database.Open(dir, database.Options{MainMigrations: crashFS(1)})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}

// cleanRefs returns the clean state at each schema 1..4.
func cleanRefs(t *testing.T) map[int]state {
	t.Helper()
	refs := map[int]state{}
	for v := 1; v <= 4; v++ {
		dir := t.TempDir()
		s, err := database.Open(dir, database.Options{MainMigrations: crashFS(v)})
		if err != nil {
			t.Fatal(err)
		}
		_ = s.Close()
		refs[v] = readState(t, filepath.Join(dir, database.MainFile))
	}
	return refs
}

// fromRx reads the schema a pre-update backup was taken at from its detail.
var fromRx = regexp.MustCompile(`main (\d+) to \d+`)

func crashRuns(t *testing.T) (int, *rand.Rand, int64) {
	t.Helper()
	if testing.Short() {
		t.Skip("crash loop: skipped under -short")
	}
	if os.Getenv(crashHelperEnv) != "" {
		t.Skip("inside the helper")
	}
	runs := 30
	if v, err := strconv.Atoi(os.Getenv(crashRunsEnv)); err == nil && v > 0 {
		runs = v
	}
	seed := time.Now().UnixNano()
	if v, err := strconv.ParseInt(os.Getenv(crashSeedEnv), 10, 64); err == nil {
		seed = v
	}
	t.Logf("seed %d (rerun with %s=%d), %d runs", seed, crashSeedEnv, seed, runs)
	return runs, rand.New(rand.NewSource(seed)), seed
}

// goodStart is the restart after a kill: the store finishes the update,
// then the backup manager adopts the raw copies and prunes, as the app does.
func goodStart(t *testing.T, dir string) {
	t.Helper()
	s, err := database.Open(dir, database.Options{MainMigrations: crashFS(4)})
	if err != nil {
		t.Fatalf("restart after kill: %v", err)
	}
	defer s.Close()
	bm := &backups.Manager{Store: s, Dir: dir, Version: "test"}
	if _, err := bm.AdoptMigrationCopies(); err != nil {
		t.Fatal(err)
	}
	if err := bm.AfterStart(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCrashLoopDuringUpdate(t *testing.T) {
	runs, rng, _ := crashRuns(t)
	refs := cleanRefs(t)

	// Calibrate: how long a whole child update takes on this machine.
	cal := t.TempDir()
	openBase(t, cal)
	t0 := time.Now()
	if err := startChild(t, "update", cal).Wait(); err != nil {
		t.Fatalf("clean child update: %v", err)
	}
	window := time.Since(t0)
	if got := readState(t, filepath.Join(cal, database.MainFile)); got != refs[4] {
		t.Fatalf("child update differs from a clean run: %+v vs %+v", got, refs[4])
	}
	t.Logf("clean child update takes %v; kills land in 0..%v", window, window*6/5)

	start := time.Now()
	phases := map[string]int{}
	for i := 0; i < runs; i++ {
		dir := t.TempDir()
		openBase(t, dir)
		before := readState(t, filepath.Join(dir, database.MainFile))
		delay := time.Duration(rng.Int63n(int64(window * 6 / 5)))
		cmd := startChild(t, "update", dir)
		time.Sleep(delay)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		// Where did the kill land?
		parts, _ := filepath.Glob(filepath.Join(dir, "backups", "*.partial"))
		mid := readState(t, filepath.Join(dir, database.MainFile))
		phase := fmt.Sprintf("v%d", mid.version)
		if len(parts) > 0 {
			phase = "backup"
		}
		phases[phase]++

		goodStart(t, dir)
		main := filepath.Join(dir, database.MainFile)
		checkSound(t, main)
		if got := readState(t, main); got != refs[4] {
			t.Fatalf("run %d (kill at %v, %s): after restart %+v, clean %+v", i, delay, phase, got, refs[4])
		}
		if parts, _ := filepath.Glob(filepath.Join(dir, "backups", "*.partial")); len(parts) > 0 {
			t.Fatalf("run %d: stale %v after a good start", i, parts)
		}
		bm := &backups.Manager{Dir: dir, Version: "test"}
		all, err := bm.List()
		if err != nil {
			t.Fatal(err)
		}
		pre := 0
		for _, b := range all {
			if b.Kind == backups.KindPreUpdate {
				pre++
			}
		}
		if pre < 1 || pre > backups.KeepPreUpdate {
			t.Fatalf("run %d: %d pre-update backups", i, pre)
		}

		// The newest pre-update backup goes back to the original data from
		// before the update, even when steps had committed before the kill:
		// the resumed update keeps the first backup.
		newest, err := bm.NewestPreUpdate()
		if err != nil {
			t.Fatal(err)
		}
		if err := bm.StageNamed(context.Background(), newest.Name); err != nil {
			t.Fatalf("run %d: staging %s: %v", i, newest.Name, err)
		}
		if _, err := backups.ApplyStaged(dir); err != nil {
			t.Fatal(err)
		}
		checkSound(t, main)
		got := readState(t, main)
		from := 0
		if m := fromRx.FindStringSubmatch(newest.Detail); m != nil {
			from, _ = strconv.Atoi(m[1])
		}
		if from != 1 || got != before {
			t.Fatalf("run %d (kill at %s): restored %+v (backup %q), want the original pre-update data %+v", i, phase, got, newest.Detail, before)
		}
	}
	t.Logf("%d/%d runs passed in %v; kills landed: %v", runs, runs, time.Since(start).Round(time.Millisecond), phases)
}

func TestCrashLoopDuringRestore(t *testing.T) {
	runs, rng, _ := crashRuns(t)
	refs := cleanRefs(t)
	oldS, newS := refs[4], refs[1] // live data, staged backup data

	// Calibrate one child ApplyStaged.
	prep := func() string {
		dir := t.TempDir()
		openBase(t, dir)
		goodStart(t, dir)
		bm := &backups.Manager{Dir: dir, Version: "test"}
		b, err := bm.NewestPreUpdate()
		if err != nil {
			t.Fatal(err)
		}
		if err := bm.StageNamed(context.Background(), b.Name); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	cal := prep()
	t0 := time.Now()
	if err := startChild(t, "apply", cal).Wait(); err != nil {
		t.Fatalf("clean child restore: %v", err)
	}
	window := time.Since(t0)
	t.Logf("clean child restore takes %v", window)

	start := time.Now()
	got := map[string]int{}
	for i := 0; i < runs; i++ {
		dir := prep()
		delay := time.Duration(rng.Int63n(int64(window * 6 / 5)))
		cmd := startChild(t, "apply", dir)
		time.Sleep(delay)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if _, err := os.Stat(filepath.Join(dir, backups.JournalFile)); err == nil {
			got["journal-left"]++
		}
		// The next start finishes or undoes it before the store opens.
		if _, err := backups.ApplyStaged(dir); err != nil {
			t.Fatalf("run %d: recovery: %v", i, err)
		}
		main := filepath.Join(dir, database.MainFile)
		checkSound(t, main)
		switch s := readState(t, main); s {
		case oldS:
			got["old"]++
		case newS:
			got["new"]++
		default:
			t.Fatalf("run %d (kill at %v): data is neither old nor new: %+v", i, delay, s)
		}
		if _, err := os.Stat(filepath.Join(dir, backups.JournalFile)); err == nil {
			t.Fatalf("run %d: journal still there after recovery", i)
		}
	}
	t.Logf("%d/%d runs passed in %v; outcomes: %v", runs, runs, time.Since(start).Round(time.Millisecond), got)
}

package secretsfile

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// A Source or Downloader the import makes is paused in the same
// transaction that makes it: a failure leaves nothing behind.
func TestCreatedPausedInOneStep(t *testing.T) {
	d := deps(t)
	ctx := context.Background()
	f := parse(t, fakeFile)
	testFailCreate = func(table string) error { return errors.New("made-up failure in " + table) }
	ls := Apply(ctx, d, f, false)
	testFailCreate = nil
	r := results(ls)
	for _, k := range []string{"indexers[1]", "indexers[2]", "qbittorrent", "sabnzbd"} {
		if !strings.HasPrefix(r[k], "failed: ") {
			t.Errorf("%s: %q", k, r[k])
		}
	}
	if n := count(t, d, "SELECT count(*) FROM sources") + count(t, d, "SELECT count(*) FROM downloaders"); n != 0 {
		t.Fatalf("a failed create left %d items", n)
	}
	if n := count(t, d, "SELECT count(*) FROM import_created WHERE live = 1"); n != 0 {
		t.Fatalf("marks left: %d", n)
	}

	// Made for real: paused and marked as the import's.
	if ls := Apply(ctx, d, f, false); !AllOK(ls) {
		t.Fatal(ls)
	}
	if n := count(t, d, "SELECT count(*) FROM import_created WHERE live = 1"); n != 4 {
		t.Errorf("marks %d", n)
	}

	// An item the import made that is somehow active (an earlier run cut
	// off between the two steps) is paused again by a re-run.
	mustExec(t, d, "UPDATE sources SET state = 'enabled' WHERE name = 'Lab feed'")
	mustExec(t, d, "UPDATE downloaders SET state = 'ok' WHERE adapter = 'qbittorrent'")
	mustExec(t, d, "INSERT OR REPLACE INTO import_created (tbl, item_id, live) SELECT 'sources', id, 1 FROM sources WHERE name = 'Lab feed'")
	mustExec(t, d, "INSERT OR REPLACE INTO import_created (tbl, item_id, live) SELECT 'downloaders', id, 1 FROM downloaders WHERE adapter = 'qbittorrent'")
	r = results(Apply(ctx, d, f, false))
	for _, k := range []string{"indexers[1]", "qbittorrent"} {
		if !strings.HasPrefix(r[k], Set) || !strings.Contains(r[k], "paused") {
			t.Errorf("repair %s: %q", k, r[k])
		}
	}
	if r["indexers[2]"] != Unchanged || r["sabnzbd"] != Unchanged {
		t.Errorf("others %v", r)
	}
	if n := count(t, d, "SELECT count(*) FROM sources WHERE state <> 'disabled'") + count(t, d, "SELECT count(*) FROM downloaders WHERE state <> 'disabled'"); n != 0 {
		t.Errorf("still active: %d", n)
	}

	// The owner resumes one: the mark goes, and a re-run keeps it running.
	mustExec(t, d, "UPDATE sources SET state = 'enabled' WHERE name = 'Lab news'")
	mustExec(t, d, "UPDATE downloaders SET state = 'ok' WHERE adapter = 'sabnzbd'")
	r = results(Apply(ctx, d, f, false))
	if r["indexers[2]"] != Unchanged || r["sabnzbd"] != Unchanged {
		t.Errorf("owner's resume undone: %v", r)
	}
}

func mustExec(t *testing.T, d Deps, q string) {
	t.Helper()
	if err := d.Store.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q); return err }); err != nil {
		t.Fatal(err)
	}
}

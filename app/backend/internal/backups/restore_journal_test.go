package backups

import (
	"os"
	"path/filepath"
	"testing"
)

// A killed restore can leave restore.journal.tmp behind; the next journal
// write must replace it, not fail.
func TestWriteJournalStaleTmp(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, JournalFile+".tmp")
	if err := os.WriteFile(stale, []byte("half a journal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeJournal(dir, journal{Keep: "k"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale tmp still there: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, JournalFile))
	if err != nil || string(b) != `{"keep":"k","moves":null}` {
		t.Fatalf("journal %q (%v)", b, err)
	}
}

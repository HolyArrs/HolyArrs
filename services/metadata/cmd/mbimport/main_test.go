package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// pruneCache keeps the given date's archives (and its .part) and other
// files, and removes the archives of other dates.
func TestPruneCache(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"20261024-001500-mbdump.tar.bz2", "20261024-001500-mbdump-derived.tar.bz2.part",
		"20261028-002228-mbdump.tar.bz2", "20261028-002228-mbdump-derived.tar.bz2.part", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644)
	}
	pruneCache(dir, "20261028-002228")
	ents, _ := os.ReadDir(dir)
	var got []string
	for _, e := range ents {
		got = append(got, e.Name())
	}
	sort.Strings(got)
	want := "20261028-002228-mbdump-derived.tar.bz2.part,20261028-002228-mbdump.tar.bz2,notes.txt"
	if strings.Join(got, ",") != want {
		t.Fatalf("left %v", got)
	}
}

package main

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// A movie folder that is not a full path (Git Bash's "C:/Program Files/Git/..."
// inside a Linux container) is never added: it could never be reached and
// would show as a broken Library folder for good.
func TestEnsureSkipsAPathThatIsNotFull(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("C:/... is a full path on Windows")
	}
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	good := filepath.Join(t.TempDir(), "movies")
	for _, p := range []string{"C:/Program Files/Git/data/media/movies", good} {
		if err := ensure(ctx, s, config{Movies: p}); err != nil {
			t.Fatal(err)
		}
	}
	var paths []string
	rows, err := s.Read().Query("SELECT path FROM library_folders ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		_ = rows.Scan(&p)
		paths = append(paths, p)
	}
	if len(paths) != 1 || paths[0] != good {
		t.Fatalf("library folders %v, want only %s", paths, good)
	}
}

package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCheckPlainFolder(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	base := t.TempDir()
	lib := filepath.Join(base, "media", "movies")
	sibling := filepath.Join(base, "media", "youtube")
	child := filepath.Join(lib, "inside")
	for _, d := range []string{lib, sibling, child} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AddFolder(ctx, s, FolderInput{Path: lib, Kind: "movie"}, nil, domain.System); err != nil {
		t.Fatal(err)
	}
	dataChild := filepath.Join(s.Dir(), "inside")
	if err := os.MkdirAll(dataChild, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path string
		want error
	}{
		{"relative/path", ErrFolderPath},
		{"", ErrFolderPath},
		{"/", ErrFolderPath},
		{s.Dir(), ErrFolderPath},
		{dataChild, ErrFolderPath},
		{filepath.Join(base, "missing"), ErrFolderAbsent},
		{lib, ErrFolderOverlap},
		{filepath.Dir(lib), ErrFolderOverlap},
		{child, ErrFolderOverlap},
		{sibling, nil},
		{sibling + "/", nil},
	}
	for _, c := range cases {
		if err := CheckPlainFolder(ctx, s, c.path); !errors.Is(err, c.want) {
			t.Errorf("%q: got %v, want %v", c.path, err, c.want)
		}
	}

	file := filepath.Join(base, "a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckPlainFolder(ctx, s, file); !errors.Is(err, ErrFolderAbsent) {
		t.Errorf("a file: got %v, want ErrFolderAbsent", err)
	}

	if os.Geteuid() == 0 {
		t.Log("running as root: the read-only case cannot be tested")
		return
	}
	ro := filepath.Join(base, "read-only")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	if err := CheckPlainFolder(ctx, s, ro); !errors.Is(err, ErrFolderReadOnly) {
		t.Errorf("read-only: got %v, want ErrFolderReadOnly", err)
	}
}

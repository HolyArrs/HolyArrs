package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestAddFolderRefusesRootAndDataDir(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	data := s.Dir()
	bad := []string{"/", data, filepath.Dir(data)}
	if runtime.GOOS == "windows" {
		bad = append(bad, `C:\`)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(data, link); err == nil {
		bad = append(bad, link)
	} else {
		t.Logf("no symlink case: %v", err)
	}
	for _, p := range bad {
		if _, err := AddFolder(ctx, s, FolderInput{Path: p, Kind: "movie"}, nil, domain.System); !errors.Is(err, ErrFolderPath) {
			t.Errorf("%s: got %v, want ErrFolderPath", p, err)
		}
	}
	ok := filepath.Join(t.TempDir(), "media", "movies")
	if err := os.MkdirAll(ok, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFolder(ctx, s, FolderInput{Path: ok, Kind: "movie"}, nil, domain.System); err != nil {
		t.Fatalf("%s: %v", ok, err)
	}
}

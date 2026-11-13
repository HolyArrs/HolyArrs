package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
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

func TestFolderQualityFitsKindAndClears(t *testing.T) {
	t.Parallel()
	s := open(t)
	ctx := context.Background()
	movies := addFolder(t, s)
	ge := presetID(t, s, mediafiles.GoodEverydayName)
	lossless := presetID(t, s, mediafiles.LosslessFirstName)
	if _, err := UpdateFolder(ctx, s, movies.ID, FolderChange{DefaultPickRules: &lossless}, domain.System); !errors.Is(err, ErrNotVideoRules) {
		t.Fatalf("music rules on a movie folder: %v", err)
	}
	got, err := UpdateFolder(ctx, s, movies.ID, FolderChange{DefaultPickRules: &ge}, domain.System)
	if err != nil || got.DefaultPickRules == nil || *got.DefaultPickRules != ge {
		t.Fatalf("set %+v %v", got, err)
	}
	zero := int64(0)
	got, err = UpdateFolder(ctx, s, movies.ID, FolderChange{DefaultPickRules: &zero}, domain.System)
	if err != nil || got.DefaultPickRules != nil {
		t.Fatalf("clear %+v %v", got, err)
	}
	if _, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "music", DefaultPickRules: &ge}, nil, domain.System); !errors.Is(err, ErrNotMusicRules) {
		t.Fatalf("video rules on a new music folder: %v", err)
	}
}

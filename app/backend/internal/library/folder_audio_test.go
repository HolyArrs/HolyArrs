package library

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func mainAudio(t *testing.T, s *database.Store, titleID int64) []string {
	t.Helper()
	vs, err := database.Select[mediafiles.Version](context.Background(), s.Read(), "versions", "title_id = ? AND is_main = 1", titleID)
	if err != nil || len(vs) != 1 {
		t.Fatalf("main Version of %d: %v %v", titleID, vs, err)
	}
	return vs[0].AudioLanguages
}

// A Library folder's audio languages go onto the main copy of each new
// movie added into it; a change does not touch titles already there, and
// bad codes or a music folder are refused.
func TestFolderAudioForNewTitles(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Italiano", NewTitleAudio: []string{" IT ", "it"}}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lf.NewTitleAudio, []string{"it"}) {
		t.Fatalf("folder audio %v, want [it]", lf.NewTitleAudio)
	}
	id, err := AddMovie(ctx, s, dune, AddOptions{LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := mainAudio(t, s, id); !reflect.DeepEqual(got, []string{"it"}) {
		t.Fatalf("new title's main copy needs %v, want [it]", got)
	}
	none := []string{}
	if got, err := UpdateFolder(ctx, s, lf.ID, FolderChange{NewTitleAudio: &none}, domain.System); err != nil || len(got.NewTitleAudio) != 0 {
		t.Fatalf("clear %+v %v", got, err)
	}
	if got := mainAudio(t, s, id); !reflect.DeepEqual(got, []string{"it"}) {
		t.Fatalf("a title already added changed: %v", got)
	}
	other, err := AddMovie(ctx, s, future, AddOptions{LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := mainAudio(t, s, other); len(got) != 0 {
		t.Fatalf("cleared folder still gave %v", got)
	}
	bad := []string{"italian!"}
	if _, err := UpdateFolder(ctx, s, lf.ID, FolderChange{NewTitleAudio: &bad}, domain.System); !errors.Is(err, ErrCopyChoice) {
		t.Fatalf("bad code: %v", err)
	}
	if _, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "music", Label: "Music", NewTitleAudio: []string{"it"}}, nil, domain.System); !errors.Is(err, ErrCopyChoice) {
		t.Fatalf("music folder took audio: %v", err)
	}
	// An edit that sends no audio keeps it.
	set := []string{"it", "en"}
	if _, err := UpdateFolder(ctx, s, lf.ID, FolderChange{NewTitleAudio: &set}, domain.System); err != nil {
		t.Fatal(err)
	}
	label := "Films"
	if got, err := UpdateFolder(ctx, s, lf.ID, FolderChange{Label: &label}, domain.System); err != nil || !reflect.DeepEqual(got.NewTitleAudio, set) {
		t.Fatalf("rename dropped audio: %+v %v", got, err)
	}
}

// A title adopted from files already in the folder keeps any audio: its
// copy is what is on disk.
func TestFolderAudioSkipsAdopted(t *testing.T) {
	lf := &mediafiles.LibraryFolder{NewTitleAudio: []string{"it"}}
	if got := NewTitleAudio(lf, true); got != nil {
		t.Fatalf("adopted got %v", got)
	}
	if got := NewTitleAudio(lf, false); !reflect.DeepEqual(got, []string{"it"}) {
		t.Fatalf("new got %v", got)
	}
	if got := NewTitleAudio(nil, false); got != nil {
		t.Fatalf("nil folder got %v", got)
	}
}

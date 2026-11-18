package library

import (
	"context"
	"reflect"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func tagsOfTitle(t *testing.T, s *database.Store, id int64) []string {
	t.Helper()
	var title mediafiles.Title
	if err := database.Get(context.Background(), s.Read(), "titles", id, &title); err != nil {
		t.Fatal(err)
	}
	return title.Tags
}

// A Library folder's tags for new titles go onto each title added into it,
// are cleaned like every other tag, and can be changed and cleared.
func TestFolderTagsForNewTitles(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Anime", NewTitleTags: []string{" Anime ", "anime", "Long Hair"}}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"anime", "long-hair"}; !reflect.DeepEqual(lf.NewTitleTags, want) {
		t.Fatalf("folder tags %v, want %v", lf.NewTitleTags, want)
	}
	id, err := AddMovie(ctx, s, dune, AddOptions{LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagsOfTitle(t, s, id); !reflect.DeepEqual(got, lf.NewTitleTags) {
		t.Fatalf("a title added into the folder has tags %v", got)
	}

	// A folder with none leaves a new title untagged, and a change in the
	// folder does not touch titles already added.
	plain, _ := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Movies"}, nil, domain.System)
	other, err := AddMovie(ctx, s, future, AddOptions{LibraryFolderID: plain.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagsOfTitle(t, s, other); len(got) != 0 {
		t.Fatalf("untagged folder gave %v", got)
	}
	none := []string{}
	got, err := UpdateFolder(ctx, s, lf.ID, FolderChange{NewTitleTags: &none}, domain.System)
	if err != nil || len(got.NewTitleTags) != 0 {
		t.Fatalf("clear %+v %v", got, err)
	}
	if got := tagsOfTitle(t, s, id); len(got) != 2 {
		t.Fatalf("an existing title lost its tags: %v", got)
	}
	set := []string{"Kids"}
	got, err = UpdateFolder(ctx, s, plain.ID, FolderChange{NewTitleTags: &set}, domain.System)
	if err != nil || !reflect.DeepEqual(got.NewTitleTags, []string{"kids"}) {
		t.Fatalf("set %+v %v", got, err)
	}
	// An edit that sends no tags keeps them.
	label := "Films"
	if got, err = UpdateFolder(ctx, s, plain.ID, FolderChange{Label: &label}, domain.System); err != nil || len(got.NewTitleTags) != 1 {
		t.Fatalf("rename dropped tags: %+v %v", got, err)
	}
}

func TestFolderTagsForNewArtists(t *testing.T) {
	svc, s, _, _ := musicService(t)
	ctx := context.Background()
	lf, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "music", Label: "Soundtracks", NewTitleTags: []string{"ost"}}, nil, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := svc.AddArtist(ctx, labArtist, ArtistAddOptions{LibraryFolderID: lf.ID}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagsOfTitle(t, s, id); !reflect.DeepEqual(got, []string{"ost"}) {
		t.Fatalf("artist tags %v", got)
	}
}

// A folder's switch to ask only tagged Sources is saved and can be turned
// off again; it is read from the folder, never copied onto a title.
func TestFolderOnlyTaggedSources(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lf, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Anime", NewTitleTags: []string{"anime"}, OnlyTaggedSources: true}, nil, domain.System)
	if err != nil || !lf.OnlyTaggedSources {
		t.Fatalf("add %+v %v", lf, err)
	}
	id, err := AddMovie(ctx, s, dune, AddOptions{LibraryFolderID: lf.ID}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// The tags of the title's one copy (no copy is listed while no folder has the switch on).
	copyTags := func() []string {
		t.Helper()
		got, err := mediafiles.FolderSourceTags(ctx, s.Read(), id)
		if err != nil || len(got[id]) > 1 {
			t.Fatalf("copies %v %v", got, err)
		}
		for _, tags := range got[id] {
			return tags
		}
		return nil
	}
	if got := copyTags(); !reflect.DeepEqual(got, []string{"anime"}) {
		t.Fatalf("switch on: %v", got)
	}
	off := false
	if lf, err = UpdateFolder(ctx, s, lf.ID, FolderChange{OnlyTaggedSources: &off}, domain.System); err != nil || lf.OnlyTaggedSources {
		t.Fatalf("off %+v %v", lf, err)
	}
	if got := copyTags(); len(got) != 0 {
		t.Fatalf("switch off: %v", got)
	}
	// Taking the last tag off turns the switch off, so a tag typed later does not bring it back unasked.
	on, none, again := true, []string{}, []string{"kids"}
	if lf, err = UpdateFolder(ctx, s, lf.ID, FolderChange{OnlyTaggedSources: &on}, domain.System); err != nil || !lf.OnlyTaggedSources {
		t.Fatalf("on %+v %v", lf, err)
	}
	if lf, err = UpdateFolder(ctx, s, lf.ID, FolderChange{NewTitleTags: &none}, domain.System); err != nil || lf.OnlyTaggedSources {
		t.Fatalf("no tags left, switch still on: %+v %v", lf, err)
	}
	if lf, err = UpdateFolder(ctx, s, lf.ID, FolderChange{NewTitleTags: &again}, domain.System); err != nil || lf.OnlyTaggedSources {
		t.Fatalf("a new tag brought the switch back: %+v %v", lf, err)
	}
	if lf, err = AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie", Label: "Bare", OnlyTaggedSources: true}, nil, domain.System); err != nil || lf.OnlyTaggedSources {
		t.Fatalf("a new folder without tags has the switch on: %+v %v", lf, err)
	}
}

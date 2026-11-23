package library

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A folder another app made, with a colon or a question mark in its name,
// can be kept as a Version's folder when it is already on disk, as a
// folder adoption keeps it; a folder typed for a new copy still may not
// use those characters, and neither may leave the Library folder or hide.
func TestExistingFolderKeptAsVersionFolder(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddVersion(ctx, s, id, VersionInput{Label: "Typed", FolderName: "Dune: Part Two (2024) [Typed]"}, nil, now, domain.System); err == nil {
		t.Fatal("a typed folder with a colon was taken")
	}
	for _, bad := range []string{"../elsewhere", ".hidden"} {
		if _, err := AddVersion(ctx, s, id, VersionInput{Label: "Bad", FolderName: bad, ExistingFolder: true}, nil, now, domain.System); err == nil {
			t.Fatalf("existing folder %q was taken", bad)
		}
	}
	v, err := AddVersion(ctx, s, id, VersionInput{Label: "4K", FourK: true, FolderName: "Dune: Part Two (2024) [4K]", ExistingFolder: true}, nil, now, domain.System)
	if err != nil || v.FolderName != "Dune: Part Two (2024) [4K]" {
		t.Fatalf("existing folder: %+v %v", v, err)
	}
}

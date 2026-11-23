package library

import (
	"context"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A folder another app made, named as a typed folder may not be, can be
// kept as a Version's folder when it is already on disk, as a folder
// adoption keeps it; a folder typed for a new copy still may not be named
// so, and neither may leave the Library folder or hide. The name is one
// every system allows (over the 200 characters a typed folder may have),
// so the test holds on Windows too, where a colon is never in a name.
func TestExistingFolderKeptAsVersionFolder(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	addFolder(t, s)
	id, err := AddMovie(ctx, s, dune, AddOptions{}, nil, now, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	long := "Dune Part Two (2024) " + strings.Repeat("[Extended] ", 17) + "[4K]"
	if _, err := AddVersion(ctx, s, id, VersionInput{Label: "Typed", FolderName: long}, nil, now, domain.System); err == nil {
		t.Fatal("a typed folder over 200 characters was taken")
	}
	if _, err := AddVersion(ctx, s, id, VersionInput{Label: "Typed", FolderName: "Dune: Part Two (2024) [Typed]"}, nil, now, domain.System); err == nil {
		t.Fatal("a typed folder with a colon was taken")
	}
	for _, bad := range []string{"../elsewhere", ".hidden"} {
		if _, err := AddVersion(ctx, s, id, VersionInput{Label: "Bad", FolderName: bad, ExistingFolder: true}, nil, now, domain.System); err == nil {
			t.Fatalf("existing folder %q was taken", bad)
		}
	}
	v, err := AddVersion(ctx, s, id, VersionInput{Label: "4K", FourK: true, FolderName: long, ExistingFolder: true}, nil, now, domain.System)
	if err != nil || v.FolderName != long {
		t.Fatalf("existing folder: %+v %v", v, err)
	}
}

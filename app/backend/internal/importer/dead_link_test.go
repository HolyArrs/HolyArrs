package importer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A downloaded link whose target is not there (a streaming drive that is
// not mounted) is refused with a reason that says so, in every place a file
// is read or placed, and the download can be imported again once it is.
func TestDeadLinkSaysTheDriveIsNotThere(t *testing.T) {
	mount, dl, lib := t.TempDir(), t.TempDir(), t.TempDir()
	src := filepath.Join(dl, "Movie.2020.mkv")
	if err := os.Symlink(filepath.Join(mount, "gone.mkv"), src); err != nil {
		t.Skip("symlinks not available:", err)
	}
	old := FollowLinks
	defer func() { FollowLinks = old }()

	FollowLinks = false
	if DeadLink(src) {
		t.Fatal("a link is only judged while links are followed")
	}
	FollowLinks = true
	if !DeadLink(src) || DeadLink(dl) || DeadLink(filepath.Join(dl, "none.mkv")) {
		t.Fatal("DeadLink: only the link to nothing is dead")
	}

	step := mediafiles.PlanStep{SourceFile: src, TargetPath: filepath.Join(lib, "Movie (2020)", "Movie (2020).mkv"), Method: "hardlink"}
	if r := place(step, Ops{}, time.Time{}); r.Outcome != "refused" || r.Error != deadLinkWhy {
		t.Fatalf("place: %+v", r)
	}
	step.Symlink = true
	if r := placeSymlink(step); r.Outcome != "refused" || r.Error != deadLinkWhy {
		t.Fatalf("placeSymlink: %+v", r)
	}
	if _, err := os.Lstat(step.TargetPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("something was placed for a dead link")
	}

	_, err := os.Open(src)
	if why := unreadable(src, err); why != deadLinkWhy {
		t.Fatalf("unreadable: %q", why)
	}
	if why := unreadable(filepath.Join(dl, "none.mkv"), err); why != notInFolderWhy {
		t.Fatalf("a missing plain file: %q", why)
	}
	plan := []mediafiles.PlanStep{{SourceFile: src, Method: "refuse", Why: []string{deadLinkWhy}}}
	if why := notPlacedWhy(dl, plan, []mediafiles.PlaceResult{{Outcome: "refused"}}); why != deadLinkWhy {
		t.Fatalf("notPlacedWhy: %q", why)
	}
	if couldNotPlace(deadLinkWhy) != deadLinkWhy || !CanImportAgain(deadLinkWhy) {
		t.Fatal("the reason is shown as it is and the download can be imported again")
	}
}

// Import by hand checks a followed link where it lies, not where it points:
// the target of a streaming drive's link is outside every allowed place by
// design.
func TestByHandHoldsAFollowedLinkByItsFolder(t *testing.T) {
	mount, dl := t.TempDir(), t.TempDir()
	target := filepath.Join(mount, "real.mkv")
	if err := os.WriteFile(target, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dl, "Movie (2020)", "Movie.2020.mkv")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, src); err != nil {
		t.Skip("symlinks not available:", err)
	}
	places := ByHandPlaces{Extra: []string{dl}}
	old := FollowLinks
	defer func() { FollowLinks = old }()

	FollowLinks = false
	if places.Holds(src) {
		t.Fatal("not followed: a link out of the places is refused")
	}
	FollowLinks = true
	if !places.Holds(src) {
		t.Fatal("followed: the link's own folder is inside the places")
	}
	if got, err := CleanByHandPath(src, places); err != nil || got != src {
		t.Fatalf("CleanByHandPath: %q %v", got, err)
	}
	if places.Holds(target) {
		t.Fatal("the target itself is still outside the places")
	}
}

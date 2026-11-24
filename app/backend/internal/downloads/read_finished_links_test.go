package downloads

import (
	"os"
	"path/filepath"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// A streaming drive's finished folder holds only links into the mount, as
// SABnzbd reports it with no file list. With links followed, the links to
// plain files are the job's files; links to folders are still never walked.
func TestReadFinishedListsLinksWhenFollowed(t *testing.T) {
	old := importer.FollowLinks
	defer func() { importer.FollowLinks = old }()

	mount := t.TempDir()
	real := filepath.Join(mount, "Movie.2014.mkv")
	if err := os.WriteFile(real, make([]byte, 12), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mount, "elsewhere"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(mount, "elsewhere", "secret.mkv"), make([]byte, 5), 0o644)

	dir := t.TempDir()
	job := filepath.Join(dir, "Movie.2014.1080p-GRP")
	if err := os.MkdirAll(job, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(job, "Movie.2014.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(mount, "elsewhere"), filepath.Join(job, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(mount, "gone.mkv"), filepath.Join(job, "broken.mkv")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(mount, "gone.nfo"), filepath.Join(job, "broken.nfo")); err != nil {
		t.Fatal(err)
	}
	// A season pack keeps its links in folders of their own.
	if err := os.MkdirAll(filepath.Join(job, "Season 1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(job, "Season 1", "Movie.2014.part2.mkv")); err != nil {
		t.Fatal(err)
	}
	single := filepath.Join(dir, "Single.mkv")
	if err := os.Symlink(real, single); err != nil {
		t.Fatal(err)
	}
	pm := []domain.PathMap{{Theirs: "/dl", Ours: dir, Link: true}}

	importer.FollowLinks = false
	if _, files := readFinished(Job{ContentPath: "/dl/Movie.2014.1080p-GRP"}, pm, ""); len(files) != 0 {
		t.Fatalf("links listed while not followed: %+v", files)
	}

	importer.FollowLinks = true
	// The live link counts as its target; the dead link to a video is listed
	// with size 0 so the import can say the drive is not mounted. The dead
	// link to an .nfo and the link to a folder stay out.
	save, files := readFinished(Job{ContentPath: "/dl/Movie.2014.1080p-GRP"}, pm, "")
	got := map[string]int64{}
	for _, f := range files {
		got[f.Name] = f.Size
	}
	if save != "/dl" || len(files) != 3 || got["Movie.2014.1080p-GRP/Movie.2014.mkv"] != 12 || got["Movie.2014.1080p-GRP/Season 1/Movie.2014.part2.mkv"] != 12 {
		t.Fatalf("folder of links: %s %+v", save, files)
	}
	if size, ok := got["Movie.2014.1080p-GRP/broken.mkv"]; !ok || size != 0 {
		t.Fatalf("dead link: %+v", files)
	}
	save, files = readFinished(Job{ContentPath: "/dl/Single.mkv"}, pm, "")
	if save != "/dl" || len(files) != 1 || files[0].Name != "Single.mkv" || files[0].Size != 12 {
		t.Fatalf("job that is a link: %s %+v", save, files)
	}
}

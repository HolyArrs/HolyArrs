package importer

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCandidatesRefuseNamesOutsideTheDownload(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, relName+".mkv"), 10)
	files := []domain.SeenFile{{Name: relName + ".mkv", Size: 10}, {Name: "../../x.mkv"}, {Name: "/etc/x.mkv"}, {Name: "a/../../x.mkv"}, {Name: `..\x.mkv`}}
	cands, bad := candidates(root, files)
	if len(cands) != 1 || filepath.Base(cands[0].Path) != relName+".mkv" {
		t.Fatalf("cands %+v", cands)
	}
	if len(bad) != 4 {
		t.Fatalf("bad %+v", bad)
	}
	for _, s := range bad {
		if s.Method != "refuse" || s.TargetPath != "" {
			t.Errorf("step %+v", s)
		}
	}
}

func TestCandidatesRefuseSymlinkedFolderOutsideTheDownload(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	write(t, filepath.Join(outside, "x.mkv"), 10)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privileges here")
		}
		t.Fatal(err)
	}
	cands, bad := candidates(root, []domain.SeenFile{{Name: "link/x.mkv", Size: 10}})
	if len(cands) != 0 || len(bad) != 1 {
		t.Fatalf("cands %+v bad %+v", cands, bad)
	}
}

func TestPlaceRefusesSymlinkSource(t *testing.T) {
	dl, lib := t.TempDir(), t.TempDir()
	real := filepath.Join(t.TempDir(), "secret.mkv")
	write(t, real, 10)
	src := filepath.Join(dl, relName+".mkv")
	if err := os.Symlink(real, src); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privileges here")
		}
		t.Fatal(err)
	}
	steps := Plan([]Candidate{{Path: src, Size: 10}}, target(lib))
	if steps[0].Method != "hardlink" {
		t.Fatalf("step %+v", steps[0])
	}
	res := Execute(steps, Ops{})
	if res[0].Outcome != "refused" {
		t.Fatalf("result %+v", res)
	}
	if _, err := os.Lstat(steps[0].TargetPath); err == nil {
		t.Fatal("symlink source was placed")
	}
}

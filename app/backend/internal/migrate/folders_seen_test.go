package migrate

import (
	"path/filepath"
	"slices"
	"testing"
)

// The folders HolyArrs can see are the mounts it was given, without the
// system ones and without its own config folder.
func TestSeenFolders(t *testing.T) {
	info := `22 1 0:21 / / rw - overlay overlay rw
23 22 0:22 / /proc rw - proc proc rw
24 22 0:23 / /dev rw - tmpfs tmpfs rw
25 24 0:24 / /dev/shm rw - tmpfs shm rw
26 22 8:1 /home/mark/config /config rw - ext4 /dev/sda1 rw
27 22 8:1 /home/mark/Holyarrs/Movies /data/movies rw - ext4 /dev/sda1 rw
28 22 8:1 /home/mark/Holyarrs/TV\040Shows /data/tv\040shows rw - ext4 /dev/sda1 rw
29 22 8:1 /etc/hosts /etc/hosts rw - ext4 /dev/sda1 rw
30 22 8:1 /home/mark/file.txt /data/file.txt rw - ext4 /dev/sda1 rw
`
	dirs := map[string]bool{"/proc": true, "/dev": true, "/dev/shm": true, "/config": true, "/data/movies": true, "/data/tv shows": true}
	got := seenFolders(info, func(p string) bool { return dirs[p] })
	if want := []string{"/data/movies", "/data/tv shows"}; !slices.Equal(got, want) {
		t.Fatalf("seen %v, want %v", got, want)
	}
}

// A path rule that points at a folder HolyArrs cannot see is listed for its
// old folder, so the folder step can say so right away.
func TestMissingTargets(t *testing.T) {
	here := filepath.ToSlash(t.TempDir())
	snap := &Snapshot{Kind: Radarr, Folders: []Folder{{Path: "/movies"}, {Path: "/4k"}, {Path: "/kept"}}}
	p := BuildPlan([]Input{{1, snap}}, Choices{PathRules: []PathRule{{From: "/movies", To: "/home/mark/Holyarrs/Movies"}, {From: "/4k", To: here}}})
	got := missingTargets(p)
	if len(got) != 1 || got["/movies"] != "/home/mark/Holyarrs/Movies" {
		t.Fatalf("missing %v", got)
	}
}

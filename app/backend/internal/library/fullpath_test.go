package library

import (
	"runtime"
	"testing"
)

func TestFullPathKnowsContainerPathsAndGitBashMangling(t *testing.T) {
	for p, want := range map[string]bool{
		"/data/media/movies":                     true,
		"data/media/movies":                      false,
		"":                                       false,
		"C:/Program Files/Git/data/media/movies": false,
		`C:\Program Files\Git\data\media\movies`: false,
		"c:/program files/git/data/media/movies": false,
	} {
		if got := FullPath(p); got != want {
			t.Errorf("FullPath(%q) = %v, want %v", p, got, want)
		}
	}
	// A Windows folder is a full path on Windows only.
	if got := FullPath(`D:\media\movies`); got != (runtime.GOOS == "windows") {
		t.Errorf("windows folder: %v", got)
	}
}

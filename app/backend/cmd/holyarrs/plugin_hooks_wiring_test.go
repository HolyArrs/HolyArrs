package main

import (
	"path/filepath"
	"testing"
)

func TestInsideAnyTable(t *testing.T) {
	root := filepath.FromSlash("/media/movies")
	for _, c := range []struct {
		p  string
		ok bool
	}{
		{"/media/movies/Dune (2021)/dune.mkv", true},
		{"/media/movies", true},
		{"/media/movies-old/x.mkv", false},
		{"/media/movies/../tv/x.mkv", false},
		{"/config/holyarrs.db", false},
	} {
		if got := insideAny(filepath.FromSlash(c.p), []string{root}); got != c.ok {
			t.Errorf("%s: %v", c.p, got)
		}
	}
	if insideAny(filepath.FromSlash("/media/movies/x"), nil) {
		t.Error("no folders granted, still inside")
	}
}

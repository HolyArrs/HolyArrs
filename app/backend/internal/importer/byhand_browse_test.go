package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestBrowseByHandPlaces(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	outside := t.TempDir()
	for _, d := range []string{"b Show", "A Movie", filepath.Join("A Movie", "extras")} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(root, "loose.mkv"), 10)
	write(t, filepath.Join(root, "song.flac"), 10)
	write(t, filepath.Join(root, "notes.txt"), 10)
	write(t, filepath.Join(outside, "secret.mkv"), 10)
	// Links out of and inside the place: the picker never follows either.
	linked := os.Symlink(outside, filepath.Join(root, "out")) == nil &&
		os.Symlink(filepath.Join(root, "b Show"), filepath.Join(root, "in")) == nil
	places := ByHandPlaces{Extra: []string{root, filepath.Join(t.TempDir(), "gone")}}

	cases := []struct {
		name    string
		path    string
		folders []string
		parent  string
		files   int
		refused bool
		links   bool
	}{
		{name: "places", path: "", folders: []string{root}},
		{name: "place", path: root, folders: []string{"A Movie", "b Show"}, files: 2},
		{name: "inside", path: filepath.Join(root, "A Movie"), folders: []string{"extras"}, parent: root},
		{name: "spaces", path: "  " + root + "  ", folders: []string{"A Movie", "b Show"}, files: 2},
		{name: "outside", path: outside, refused: true},
		{name: "dot dot", path: root + "/A Movie/../..", refused: true},
		{name: "relative", path: "A Movie", refused: true},
		{name: "a file", path: filepath.Join(root, "loose.mkv"), refused: true},
		{name: "missing", path: filepath.Join(root, "missing"), refused: true},
		{name: "link out", path: filepath.Join(root, "out"), refused: true, links: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.links && !linked {
				t.Skip("no symlinks here")
			}
			got, err := browsePlaces(ctx, places, c.path)
			if c.refused {
				if err == nil {
					t.Fatalf("listed %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, f := range got.Folders {
				names = append(names, f.Name)
				if c.path != "" && filepath.Dir(f.Path) != filepath.Clean(got.Path) {
					t.Errorf("path %q not inside %q", f.Path, got.Path)
				}
			}
			if len(names) != len(c.folders) {
				t.Fatalf("folders %v, want %v", names, c.folders)
			}
			for i := range names {
				if names[i] != c.folders[i] {
					t.Fatalf("folders %v, want %v", names, c.folders)
				}
			}
			if got.Parent != c.parent || got.Files != c.files || got.More {
				t.Fatalf("result %+v", got)
			}
		})
	}
}

func TestBrowseByHandNoPlaces(t *testing.T) {
	var pe *PlaceError
	if _, err := browsePlaces(context.Background(), ByHandPlaces{}, ""); !errors.As(err, &pe) {
		t.Fatalf("no places: %v", err)
	}
}

func TestBrowseByHandCaps(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < MaxBrowseFolders+3; i++ {
		if err := os.Mkdir(filepath.Join(root, "d"+strconv.Itoa(i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := browsePlaces(context.Background(), ByHandPlaces{Extra: []string{root}}, root)
	if err != nil || len(got.Folders) != MaxBrowseFolders || !got.More {
		t.Fatalf("%d folders, more %v, %v", len(got.Folders), got.More, err)
	}
}

// The folder is read through an os.Root on the place: a link swapped in
// after the checks cannot lead the listing out of it.
func TestReadInsideStaysInPlace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(outside, "secret.mkv"), 10)
	linked := os.Symlink(outside, filepath.Join(root, "swapped")) == nil
	cases := []struct {
		name  string
		dir   string
		ok    bool
		links bool
	}{
		{"place", root, true, false},
		{"inside", filepath.Join(root, "real"), true, false},
		{"outside", outside, false, false},
		{"swapped for a link", filepath.Join(root, "swapped"), false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.links && !linked {
				t.Skip("no symlinks here")
			}
			_, err := readInside(root, c.dir)
			if (err == nil) != c.ok {
				t.Fatalf("err %v, want ok %v", err, c.ok)
			}
		})
	}
}

// A finished download's own folder may be scanned, but the picker offers only
// the listed places.
func TestBrowseByHandOnlyListedPlaces(t *testing.T) {
	dl := t.TempDir()
	places := ByHandPlaces{Downloads: []string{dl}}
	cases := []struct {
		name   string
		places ByHandPlaces
		ok     bool
	}{
		{"download folder only", places, false},
		{"also listed", ByHandPlaces{Downloads: []string{dl}, Extra: []string{dl}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := browsePlaces(context.Background(), c.places, dl)
			if (err == nil) != c.ok {
				t.Fatalf("err %v, want ok %v", err, c.ok)
			}
		})
	}
}

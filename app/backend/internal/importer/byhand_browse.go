package importer

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MaxBrowseFolders caps how many folders one folder listing returns.
const MaxBrowseFolders = 1000

// BrowseFolder is one folder a by-hand import may open.
type BrowseFolder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// BrowseResult is one step of the folder picker: the folders inside Path,
// or the allowed places themselves when Path is empty.
type BrowseResult struct {
	Path string `json:"path"`
	// Parent is the folder one level up, or "" when Path is an allowed
	// place itself and going up returns to the list of places.
	Parent  string         `json:"parent"`
	Folders []BrowseFolder `json:"folders"`
	// Files counts the video and music files directly in Path.
	Files int  `json:"files"`
	More  bool `json:"more"`
}

// BrowseByHand lists the folders inside p for the folder picker. An empty p
// lists the allowed places. Nothing outside them is listed: a folder that
// is a symlink, or that leads out of the places, is left out.
func (im *Importer) BrowseByHand(ctx context.Context, p string) (BrowseResult, error) {
	places, err := im.ByHandPlaces(ctx)
	if err != nil {
		return BrowseResult{}, err
	}
	return browsePlaces(ctx, places, p)
}

func browsePlaces(ctx context.Context, places ByHandPlaces, p string) (BrowseResult, error) {
	out := BrowseResult{Folders: []BrowseFolder{}}
	if strings.TrimSpace(p) == "" {
		for _, root := range places.all() {
			if fi, err := os.Stat(root); err == nil && fi.IsDir() && places.Holds(filepath.Clean(root)) {
				out.Folders = append(out.Folders, BrowseFolder{Name: root, Path: filepath.Clean(root)})
			}
		}
		if len(out.Folders) == 0 && len(places.all()) == 0 {
			return out, places.Refusal()
		}
		return out, nil
	}
	dir, err := CleanByHandPath(p, places)
	if err != nil {
		return out, err
	}
	// Only the listed places are offered, never a download's own folder.
	root := listedRoot(places, dir)
	if root == "" {
		return out, places.Refusal()
	}
	out.Path = dir
	if up := filepath.Dir(dir); up != dir && listedRoot(places, up) != "" && places.Holds(up) {
		out.Parent = up
	}
	entries, err := readInside(root, dir)
	if err != nil {
		return out, ErrByHandPath
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		child := filepath.Join(dir, e.Name())
		switch {
		case e.Type()&os.ModeSymlink != 0:
			// A link may lead anywhere; the picker never follows one.
		case e.IsDir():
			if len(out.Folders) >= MaxBrowseFolders {
				out.More = true
			} else {
				out.Folders = append(out.Folders, BrowseFolder{Name: e.Name(), Path: child})
			}
		case e.Type().IsRegular():
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if videoExt[ext] || audioExt[ext] {
				out.Files++
			}
		}
	}
	sort.SliceStable(out.Folders, func(i, j int) bool {
		return strings.ToLower(out.Folders[i].Name) < strings.ToLower(out.Folders[j].Name)
	})
	return out, nil
}

// listedRoot returns the listed place that holds p, or "".
func listedRoot(places ByHandPlaces, p string) string {
	return ByHandPlaces{Library: places.Library, Downloaders: places.Downloaders, Extra: places.Extra, Watch: places.Watch}.within(p)
}

// readInside lists dir through an os.Root opened on root, so a folder
// swapped for a link after the checks still cannot lead out of the place.
func readInside(root, dir string) ([]os.DirEntry, error) {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrByHandPath
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	f, err := r.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.IsDir() {
		return nil, ErrByHandPath
	}
	return f.ReadDir(-1)
}

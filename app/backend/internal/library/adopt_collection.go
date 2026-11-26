package library

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func checkAdoptedOwner(ctx context.Context, db Reader, owner mediafiles.AdoptedFileOwner, tmdb string) error {
	if !owner.Assigned {
		return errors.New("this file is already recorded as an unassigned file; review it before importing")
	}
	id, err := TitleByTMDB(ctx, db, tmdb)
	if err != nil {
		return err
	}
	if id != owner.TitleID {
		return errors.New("this file already belongs to another movie")
	}
	return nil
}

// collectionFiles recognizes distinct movies stored together, rather than
// alternate encodes or discs of one movie. Unnamed videos stay folder-based.
func collectionFiles(dir string) []string {
	return collectionFilesIn(dir, false)
}

func collectionFilesIn(dir string, shared bool) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return collectionEntries(ents, shared)
}

func collectionEntries(ents []os.DirEntry, shared bool) []string {
	var files []string
	titles := map[string]bool{}
	for _, e := range ents {
		if !e.Type().IsRegular() || !videoExt[strings.ToLower(filepath.Ext(e.Name()))] || sampleVideo(e.Name()) {
			continue
		}
		files = append(files, e.Name())
		name, year := FolderName(candidateLookup(e.Name()))
		if year == 0 {
			name, year = collectionReleaseFacts(strings.TrimSuffix(e.Name(), path.Ext(e.Name())))
		}
		if !shared && (year == 0 || name == "") {
			continue
		}
		// The year distinguishes remakes with the same title.
		titles[strings.ToLower(name)+"/"+strconv.Itoa(year)] = true
	}
	if !shared && len(titles) < 2 {
		return nil
	}
	return files
}

func sharedAncestors(shared map[string]bool) map[string]bool {
	out := map[string]bool{}
	for dir := range shared {
		for !out[dir] {
			out[dir] = true
			parent := path.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return out
}

// candidateLookup removes a video's extension before title parsing. Paths
// themselves remain intact for paging and prefetch identity.
func candidateLookup(rel string) string {
	n := path.Base(rel)
	if videoExt[strings.ToLower(path.Ext(n))] {
		n = strings.TrimSuffix(n, path.Ext(n))
	}
	return n
}

func sampleVideo(name string) bool {
	for _, w := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return r < 'a' || r > 'z' }) {
		if w == "sample" {
			return true
		}
	}
	return false
}

// candidateVideos limits a file candidate to that exact file. Folder
// candidates retain the existing disc/extras traversal.
func candidateVideos(root, rel string) []AdoptVideo {
	fi, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if err == nil && fi.Mode().IsRegular() && videoExt[strings.ToLower(path.Ext(rel))] {
		return []AdoptVideo{{Path: rel, Size: fi.Size()}}
	}
	return listVideos(root, rel)
}

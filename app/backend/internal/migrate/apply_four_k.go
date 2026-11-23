package migrate

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// fourKDryWords says what the real run will do with a movie's 4K copy,
// by the same checks addFourK makes.
func (r *run) fourKDryWords(fk *FourKPlan) string {
	if fk.Unresolved != "" {
		return "; its 4K copy is not placed: " + fk.Unresolved
	}
	lib := filepath.Clean(filepath.FromSlash(fk.Library))
	switch {
	case r.left[lib]:
		return "; its 4K copy is left out: " + filepath.ToSlash(lib) + " is set to \"Don't bring them\""
	case r.folderAt(lib) == 0 && !slices.Contains(r.coming["movie"], lib):
		return "; its 4K copy will not be brought: its Library folder " + fk.Library + " is not on this server"
	case !isDir(filepath.Join(lib, fk.Folder)):
		return "; its 4K copy becomes a 4K Version with no file yet; there is no folder at " + path.Join(fk.Library, fk.Folder)
	}
	return "; its 4K copy becomes a 4K Version using the folder " + path.Join(fk.Library, fk.Folder) + " where it is"
}

// recordQualities puts the quality each old app recorded on the copy it
// came from only: the main Version takes the main app's, the 4K Version
// the 4K app's. A 1080p and a 4K copy often share a file name, so a
// quality must never cross to the other copy. It says how many changed.
func (r *run) recordQualities(id int64, t *TitlePlan) int {
	n := 0
	if main := r.versionOf(id, false); main != 0 && len(t.Qualities) > 0 {
		m, _ := r.m.Library.SetVersionRecordedQualities(r.ctx, id, main, t.Qualities, r.by)
		n += m
	}
	if t.FourK != nil && len(t.FourK.Qualities) > 0 {
		if v := r.versionOf(id, true); v != 0 {
			m, _ := r.m.Library.SetVersionRecordedQualities(r.ctx, id, v, t.FourK.Qualities, r.by)
			n += m
		}
	}
	return n
}

// addFourK brings a merged movie's 4K copy over as a 4K Version of the
// title this run just made, adopting its video where it is. It returns the
// note for the title's line ("" is never returned: the copy either came
// over or the line says why not). No file is moved, renamed or written.
func (r *run) addFourK(titleID int64, fk *FourKPlan) string {
	if fk.Unresolved != "" {
		return "; its 4K copy was not brought: " + fk.Unresolved
	}
	lib := filepath.Clean(filepath.FromSlash(fk.Library))
	if r.left[lib] {
		return "; its 4K copy was left out: " + filepath.ToSlash(lib) + " is set to \"Don't bring them\""
	}
	lfID := r.folderAt(lib)
	if lfID == 0 {
		return "; its 4K copy was not brought: its Library folder " + fk.Library + " is not on this server"
	}
	// A 4K copy not downloaded yet has no folder, as Radarr keeps it; it
	// comes over wanted, as the main copy does.
	dir := filepath.Join(lib, fk.Folder)
	onDisk := isDir(dir)
	if r.versionOf(titleID, true) != 0 {
		return "; its 4K copy was not brought: the title already has a 4K Version"
	}
	keep := fk.KeepUpToDate
	// Pick rules made from the 4K app's profile, or the 4K ones when that
	// group was left out, never the main copy's.
	pr := r.presetID(fk.Preset, "movie")
	if pr == 0 {
		pr = r.presetID("4K", "movie")
	}
	paths, _ := library.DownloadPaths(r.ctx, r.m.Store.Read(), r.m.Library.SavePath)
	v, err := library.AddVersion(r.ctx, r.m.Store, titleID, library.VersionInput{Label: "4K", FourK: true,
		LibraryFolderID: lfID, FolderName: fk.Folder, ExistingFolder: true, PickRulesID: pr, KeepUpToDate: &keep}, paths, r.m.now(), r.by)
	if err != nil {
		return "; its 4K copy was not brought: " + err.Error()
	}
	if !onDisk {
		return "; its 4K copy came over as a 4K Version with no file yet; there is no folder at " + filepath.ToSlash(dir)
	}
	note := "; its 4K copy came over as a 4K Version using the folder " + filepath.ToSlash(dir) + " where it is"
	if fourKVideo(lib, fk.Folder, fk.Qualities) == "" {
		note += "; no file was linked: the 4K app's file is not there and the folder does not hold exactly one movie file"
	}
	if video := fourKVideo(lib, fk.Folder, fk.Qualities); video != "" {
		if _, err := r.m.Library.AdoptMovieCopy(r.ctx, titleID, v.ID, library.CopyAdoption{RelativePath: video}, r.by); err != nil {
			why := err.Error()
			// Leave no empty Version behind when the file could not be
			// adopted, so the line's "not brought" stays true.
			if rmErr := library.RemoveVersion(r.ctx, r.m.Store, titleID, v.ID, r.by); rmErr != nil {
				return "; its 4K Version was added but its file was not adopted (" + why + "); the empty Version stays: " + rmErr.Error()
			}
			return "; its 4K copy was not brought: " + why
		}
		note += ", with " + video
	}
	if len(fk.Qualities) > 0 {
		// The quality the 4K app recorded, on the 4K copy only.
		_, _ = r.m.Library.SetVersionRecordedQualities(r.ctx, titleID, v.ID, fk.Qualities, r.by)
	}
	return note
}

// fourKVideo picks the 4K copy's file in its folder: a file the 4K app
// recorded (its names are the keys of qualities), or else the folder's
// only movie video. Two candidate cuts and no recorded name link none, so
// an alternate cut is never taken for the 4K copy.
func fourKVideo(lib, folder string, qualities map[string]string) string {
	for name := range qualities {
		rel := path.Join(folder, name)
		if fi, err := os.Lstat(filepath.Join(lib, filepath.FromSlash(rel))); err == nil && fi.Mode().IsRegular() && path.Base(rel) == name {
			return rel
		}
	}
	var only []string
	for _, v := range library.ListVideosN(lib, folder, 5000) {
		rel := strings.TrimPrefix(v.Path, folder+"/")
		if !strings.Contains(rel, "/") && !extraName(rel) {
			only = append(only, v.Path)
		}
	}
	if len(only) == 1 {
		return only[0]
	}
	return ""
}

// finishMovedTitle brings a title an earlier run moved up to date: the
// quality each app recorded, and a merged 4K copy that a run cut short
// before it came over. It returns what to add to the line.
func (r *run) finishMovedTitle(id int64, t *TitlePlan) string {
	d := ""
	if n := r.recordQualities(id, t); n > 0 {
		d = "; the quality of " + strconv.Itoa(n) + " files was updated"
	}
	if t.FourK != nil && r.versionOf(id, true) == 0 {
		d += r.addFourK(id, t.FourK)
	}
	return d
}

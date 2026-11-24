package migrate

import (
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// fourKDryWords says what the real run will do with a movie's 4K copy,
// by the same checks addFourK makes, and claims its folder when it comes.
func (r *run) fourKDryWords(fk *FourKPlan) string {
	if fk.Unresolved != "" {
		return "; its 4K copy is not placed: " + fk.Unresolved
	}
	lib := filepath.Clean(filepath.FromSlash(fk.Library))
	lfID := r.folderAt(lib)
	if r.left[lib] {
		return "; its 4K copy is left out: " + filepath.ToSlash(lib) + " is set to \"Don't bring them\""
	}
	if lfID == 0 && !slices.Contains(r.coming["movie"], lib) {
		return "; its 4K copy will not be brought: its Library folder " + fk.Library + " is not on this server"
	}
	// The Library folder must be usable, as adding the Version checks; one
	// this run adds is checked as it will be once added.
	paths, _ := library.DownloadPaths(r.ctx, r.m.Store.Read(), r.m.Library.SavePath)
	if lfID != 0 {
		if _, err := library.CheckVersionFolder(r.ctx, r.m.Store.Read(), lfID, "movie", paths); err != nil {
			return "; its 4K copy will not be brought: " + err.Error()
		}
	} else if h := library.CheckUse(mediafiles.LibraryFolder{Path: lib, Kind: "movie"}, paths); !h.Usable() {
		return "; its 4K copy will not be brought: " + (&library.FolderError{Health: h}).Error()
	} else if library.UnsafeFolder(lib, r.m.Store.Dir()) {
		return "; its 4K copy will not be brought: its Library folder " + fk.Library + " cannot be added: " + library.ErrFolderPath.Error()
	} else if r.folderRefused(lib) {
		return "; its 4K copy will not be brought: its Library folder " + fk.Library + " overlaps another Library folder, so it cannot be added"
	}
	switch {
	case r.folderHeld(lfID, fk.Folder):
		return "; its 4K copy will not be brought: the folder " + path.Join(fk.Library, fk.Folder) + " already belongs to a title in the Library"
	case r.claimed[claimKey(lib, fk.Folder)] != "":
		return "; its 4K copy will not be brought: the folder " + path.Join(fk.Library, fk.Folder) + " is taken by another title in this import"
	}
	// Its file must lie inside the Version's folder and the Library folder,
	// as adopting it checks; a folder that leads elsewhere takes it back.
	if video := fourKVideo(lib, fk.Folder, recordedNames(fk.File, fk.Qualities)); video != "" {
		abs := filepath.Join(lib, filepath.FromSlash(video))
		if !importer.InsideRealFile(filepath.Join(lib, fk.Folder), abs) || !importer.InsideRealFile(lib, abs) {
			return "; its 4K copy will not be brought: its file " + video + " is not inside the Library folder " + fk.Library
		}
	}
	// A folder not made yet must take a name every system can create, as
	// adding the Version checks; only one already on disk keeps its name.
	onDisk := isDir(filepath.Join(lib, fk.Folder))
	if !onDisk && !library.NewVersionFolderOK(fk.Folder) {
		return "; its 4K copy will not be brought: " + library.ErrVersionFolder.Error()
	}
	r.claim(lib, fk.Folder, claimFourK)
	if !onDisk {
		return "; its 4K copy becomes a 4K Version with no file yet; there is no folder at " + path.Join(fk.Library, fk.Folder)
	}
	return "; its 4K copy becomes a 4K Version using the folder " + path.Join(fk.Library, fk.Folder) + " where it is"
}

// folderRefused says adding the Library folder at lib in this run will be
// refused, as adding a Library folder checks: it is a whole drive or
// around HolyArrs's own folder (library.UnsafeFolder), or it overlaps one
// already kept or one of any kind this run adds before it. The run's
// folders are added in order, and one refused takes no path from those
// after it.
func (r *run) folderRefused(lib string) bool {
	var added []string
	takes := func(c string) bool {
		if library.UnsafeFolder(c, r.m.Store.Dir()) || slices.ContainsFunc(added, func(a string) bool { return library.FoldersOverlap(a, c) }) {
			return false
		}
		kept, _ := library.FolderOverlapsKept(r.ctx, r.m.Store.Read(), c)
		return !kept
	}
	for _, c := range r.comingAll[:max(slices.Index(r.comingAll, lib), 0)] {
		if takes(c) {
			added = append(added, c)
		}
	}
	return !takes(lib)
}

// What took a folder in the dry run's claimed record.
const (
	claimMain  = "main"
	claimFourK = "4k"
)

// claimKey names a folder in a Library folder here, with letters A to Z
// in either case (foldName), as adding a Version compares them.
func claimKey(lib, folder string) string {
	return filepath.Clean(lib) + "\x00" + foldName(folder)
}

// claim records that a title of this import takes folder in lib here.
func (r *run) claim(lib, folder, by string) {
	if r.claimed == nil {
		r.claimed = map[string]string{}
	}
	if k := claimKey(lib, folder); r.claimed[k] == "" {
		r.claimed[k] = by
	}
}

// mainHere is the Library folder here a title's own copy goes to: lib, or
// fallback (a Library folder here used in place of one this server cannot
// see) when set.
func mainHere(lib, fallback string) string {
	if fallback != "" {
		return filepath.FromSlash(fallback)
	}
	return lib
}

// folderHeld says a Version of a title still in the Library already uses
// the folder in Library folder lfID, the check adding a Version makes.
func (r *run) folderHeld(lfID int64, folder string) bool {
	if lfID <= 0 {
		return false
	}
	held, _ := mediafiles.VersionFolderTaken(r.ctx, r.m.Store.Read(), lfID, folder)
	return held
}

// fourKAt is a merged movie's 4K copy as this run brings it, once the main
// copy's place here is known: lib, or fallback (a Library folder here used
// in place of one this server cannot see) when set. A main copy that lands
// in the 4K copy's own folder (by name, or on disk: another path to the
// same folder) leaves it no folder of its own, so the copy is not brought.
// nil when the title has no 4K copy.
func fourKAt(t *TitlePlan, lib, fallback string) *FourKPlan {
	fk := t.FourK
	if fk == nil || fk.Unresolved != "" {
		return fk
	}
	mainDir := filepath.Join(mainHere(lib, fallback), t.Folder)
	if claimKey(mainHere(lib, fallback), t.Folder) != claimKey(filepath.FromSlash(fk.Library), fk.Folder) &&
		!sameFolder(mainDir, filepath.Join(filepath.FromSlash(fk.Library), fk.Folder)) {
		return fk
	}
	at := *fk
	at.Unresolved = "the main copy comes over in its folder " + path.Join(fk.Library, fk.Folder) + ", and a 4K Version needs a folder of its own"
	return &at
}

// mainCopyFile names the file a merged movie's main copy adopts in its
// folder: the one its own app recorded, or, when that file is not there,
// the biggest movie video the 4K app did not record. "" leaves the pick to
// adopting: then a folder both apps share holds only the 4K app's file, and
// the main copy takes it, as a rescan links a movie folder's one video.
func mainCopyFile(lib string, t *TitlePlan) string {
	if rel := recordedFile(lib, t.Folder, recordedNames(t.File, t.Qualities)); rel != "" {
		return path.Base(rel)
	}
	if t.FourK == nil {
		return ""
	}
	fourK := recordedNames(t.FourK.File, t.FourK.Qualities)
	// Only a folder both apps share holds the 4K app's file; in a folder of
	// its own the main copy is picked as adopting always picks.
	if len(fourK) == 0 || !sameFolder(filepath.Join(lib, t.Folder), filepath.Join(filepath.FromSlash(t.FourK.Library), t.FourK.Folder)) {
		return ""
	}
	// A file system that ignores case finds the 4K app's file under any
	// case, so the names are compared the same way.
	fold := library.FoldsCase(lib)
	isFourK := func(rel string) bool {
		return slices.ContainsFunc(fourK, func(n string) bool { return sameFileName(fold, n, rel) })
	}
	// The listing is biggest first.
	for _, v := range library.ListVideosN(lib, t.Folder, 5000) {
		rel := strings.TrimPrefix(v.Path, t.Folder+"/")
		if isFourK(rel) || strings.Contains(rel, "/") || extraName(rel) {
			continue
		}
		return rel
	}
	return ""
}

// recordQualities puts the quality the old app recorded on the title's
// files, as SetRecordedQualities always has. For a merged movie each app's
// goes on the copy it came from only: the main Version takes the main
// app's, the 4K Version the 4K app's. A 1080p and a 4K copy often share a
// file name, so a quality must never cross to the other copy. It says how
// many changed.
func (r *run) recordQualities(id int64, t *TitlePlan) int {
	if t.FourK == nil {
		n, _ := r.m.Library.SetRecordedQualities(r.ctx, id, t.Qualities, r.by)
		return n
	}
	n := 0
	if main := r.versionOf(id, false); main != 0 && len(t.Qualities) > 0 {
		m, _ := r.m.Library.SetVersionRecordedQualities(r.ctx, id, main, t.Qualities, r.by)
		n += m
	}
	if len(t.FourK.Qualities) > 0 {
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
		LibraryFolderID: lfID, FolderName: fk.Folder, ExistingFolder: onDisk, PickRulesID: pr, KeepUpToDate: &keep}, paths, r.m.now(), r.by)
	if err != nil {
		return "; its 4K copy was not brought: " + err.Error()
	}
	if !onDisk {
		return "; its 4K copy came over as a 4K Version with no file yet; there is no folder at " + filepath.ToSlash(dir)
	}
	note := "; its 4K copy came over as a 4K Version using the folder " + filepath.ToSlash(dir) + " where it is"
	video := fourKVideo(lib, fk.Folder, recordedNames(fk.File, fk.Qualities))
	if video == "" {
		note += "; no file was linked: the 4K app's file is not there and the folder does not hold exactly one movie file"
	}
	if video != "" {
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
// recorded (names, from recordedNames), or else the folder's only movie
// video. Two candidate cuts and no recorded name link none, so an
// alternate cut is never taken for the 4K copy.
func fourKVideo(lib, folder string, names []string) string {
	if rel := recordedFile(lib, folder, names); rel != "" {
		return rel
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

// sameFolder says two folders on disk are one: the same path, or another
// path to it (a link, or a name in another case where the file system
// ignores case). A folder not on disk is never the same as another.
func sameFolder(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	return err == nil && os.SameFile(ai, bi)
}

// sameFileName says two file names in one folder name the same file: the
// same text, or, where the file system ignores case (fold), the same text
// in any case.
func sameFileName(fold bool, a, b string) bool {
	return a == b || fold && strings.EqualFold(a, b)
}

// recordedNames are the files an old app recorded for a movie, in the
// order they are tried: its recorded file (with or without a quality),
// then the names it recorded a quality for, sorted so the pick is the same
// on every run.
func recordedNames(file string, qualities map[string]string) []string {
	names := slices.Sorted(maps.Keys(qualities))
	if file != "" {
		names = append([]string{file}, names...)
	}
	return names
}

// recordedFile is the first of names that is a movie video directly in the
// folder, by the check adopting makes (library.NamedMovieVideo: a plain
// file, or a followed link to one, and never a name with a separator), as
// a path in the Library folder; "" when none is there.
func recordedFile(lib, folder string, names []string) string {
	for _, name := range names {
		if v, ok := library.NamedMovieVideo(lib, folder, name); ok {
			return v.Path
		}
	}
	return ""
}

// finishMovedTitle brings a title an earlier run moved up to date: the
// quality each app recorded. A merged 4K copy that the earlier run did not
// bring is added on the title page, as for a title that was already here,
// so each run's undo takes back only what that run made; the dry run says
// so too. When this same move made the title and stopped before the 4K
// copy, the copy comes over now. It returns what to add to the line.
func (r *run) finishMovedTitle(id int64, key string, t *TitlePlan) string {
	d := ""
	if !r.dry {
		if n := r.recordQualities(id, t); n > 0 {
			d = "; the quality of " + strconv.Itoa(n) + " files was updated"
		}
	}
	if t.FourK == nil || r.versionOf(id, true) != 0 {
		return d
	}
	// This same move made the title and stopped before its 4K copy (a
	// restart): the copy is added now, as the title's own step adds it.
	if _, mine := r.madeThisMove(key); mine && !r.dry {
		return d + r.addFourK(id, t.FourK) + r.keepForFourK(id, t, t.FourK, "")
	}
	return d + "; add its 4K copy on the title page"
}

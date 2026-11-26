package library

import (
	"context"
	"database/sql"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// replacedInPlace says the file a rescan just read is not the one its
// stored facts were read from: both reads stamped the file, and its size or
// modified time changed. A file with no stamp yet (facts from before the
// stamps) is not taken for replaced.
func replacedInPlace(old, fresh map[string]any) bool {
	oldSize, ok := inspectionSize(old["inspection_size"])
	newSize, ok2 := inspectionSize(fresh["inspection_size"])
	if !ok || !ok2 || old["inspection_modified"] == nil {
		return false
	}
	return oldSize != newSize || fmt.Sprint(old["inspection_modified"]) != fmt.Sprint(fresh["inspection_modified"])
}

// updateReplacedFile stores what a rescan read from a file replaced in
// place (another app or the person put a new file at the same path) and
// judges it as the new file it is: its size, its quality and its Slots'
// good enough state follow the new header. What the File's name facts say
// about the old file is dropped first (dropStaleRelease).
func (s *Service) updateReplacedFile(ctx context.Context, fileID int64, info map[string]any, by domain.Actor) error {
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		var f mediafiles.File
		if err := database.Get(ctx, tx, "files", fileID, &f); err != nil {
			return err
		}
		var v mediafiles.Version
		if err := database.Get(ctx, tx, "versions", f.VersionID, &v); err != nil {
			return err
		}
		var t mediafiles.Title
		if err := database.Get(ctx, tx, "titles", v.TitleID, &t); err != nil {
			return err
		}
		dropStaleRelease(&f, info)
		f.MediaInfo = info
		if f.Languages = InspectedLanguages(info); f.Languages == nil {
			f.Languages = []string{}
		}
		if n, ok := inspectionSize(info["inspection_size"]); ok {
			f.Size = n
		}
		name := ownName(f.RelativePath)
		if f.ReleaseName != nil && *f.ReleaseName != "" {
			name = *f.ReleaseName
		}
		p := releasename.Parse(mediafiles.JudgeName(name, info), releasename.Hints{Size: f.Size, Show: t.Kind == "show"})
		f.Quality = mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value, Audio: f.Quality.Audio}
		info["step"] = mediafiles.StepFor(p)
		if err := mediafiles.SetFileFactsTx(ctx, tx, f, s.now(), by); err != nil {
			return err
		}
		_, err := goodEnoughTx(ctx, tx, v, t.KeepUpToDate && v.KeepUpToDate, by)
		return err
	})
}

// dropStaleRelease forgets, for a file replaced in place, the name facts
// that belonged to the file before it: the quality the old app recorded
// for it, and a release name other than the file's own name (the release
// it was downloaded as, or the old app's scene name). Each is dropped only
// when it names a resolution and the new header's differs (a 1080p release
// name on a 2160p file); a file retagged or remuxed in place keeps them, as
// the header cannot tell a web copy from a Blu-ray. When either is dropped
// the release group is again the one the file's own name names, if any.
func dropStaleRelease(f *mediafiles.File, info map[string]any) {
	r, _ := info["resolution"].(string)
	header, err := strconv.Atoi(strings.TrimSuffix(r, "p"))
	if err != nil || header <= 0 {
		return
	}
	stale := func(name string) bool {
		named := releasename.Parse(name, releasename.Hints{}).Resolution.Value
		return named > 0 && resolutionClass(named) != resolutionClass(header)
	}
	dropped := false
	if rec, _ := info[mediafiles.RecordedQualityKey].(string); rec != "" && stale(rec) {
		delete(info, mediafiles.RecordedQualityKey)
		dropped = true
	}
	own := ownName(f.RelativePath)
	if f.ReleaseName != nil && *f.ReleaseName != own && stale(*f.ReleaseName) {
		f.ReleaseName = &own
		dropped = true
	}
	if dropped {
		f.Group = nil
		if g := releasename.Parse(own, releasename.Hints{}).Group.Value; g != "" {
			f.Group = &g
		}
	}
}

// resolutionClass puts the SD sizes (480p, 576p) together: an old app's
// "DVD" and a PAL header are the same copy.
func resolutionClass(r int) int {
	return max(r, 576)
}

// ownName is a file's name without its extension, the release name
// adoption gives a file.
func ownName(rel string) string {
	return strings.TrimSuffix(path.Base(rel), path.Ext(rel))
}

package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// ErrNoReviewFile is returned when a download holds no single clear video to
// import from Review.
var ErrNoReviewFile = errors.New("the download has no single video to import; use Import files by hand")

// ReviewFile returns the one video a finished movie download holds, for
// Review's Import this file: samples, programs and other files are left out,
// and a download with no video or more than one gives "".
func (im *Importer) ReviewFile(ctx context.Context, downloadID int64) string {
	db := im.Store.Read()
	var d domain.Download
	if database.Get(ctx, db, "downloads", downloadID, &d) != nil || (d.State != "finished" && d.State != "seeding") {
		return ""
	}
	var dr domain.Downloader
	if database.Get(ctx, db, "downloaders", d.DownloaderID, &dr) != nil {
		return ""
	}
	root, ok := domain.MapPath(d.SavePath, dr.PathMap)
	if !ok {
		return ""
	}
	cands, _ := candidates(root, d.FilesSeen)
	found := ""
	for _, c := range cands {
		ext := strings.ToLower(filepath.Ext(c.Path))
		p := releasename.Parse(filepath.Base(c.Path), releasename.Hints{Size: c.Size})
		if !videoExt[ext] || programExt[ext] || p.Has("double_extension") || isSample(c.Path, p) {
			continue
		}
		if found != "" {
			return ""
		}
		found = c.Path
	}
	if found == "" {
		return ""
	}
	if fi, err := os.Lstat(found); err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	return found
}

// ReviewFolder returns a finished download's own folder as HolyArrs sees it
// (its one top folder, or its one file when it holds a single loose file),
// for Review's Match files by hand; "" when it can't be read.
func (im *Importer) ReviewFolder(ctx context.Context, downloadID int64) string {
	db := im.Store.Read()
	var d domain.Download
	if database.Get(ctx, db, "downloads", downloadID, &d) != nil || (d.State != "finished" && d.State != "seeding") {
		return ""
	}
	var dr domain.Downloader
	if database.Get(ctx, db, "downloaders", d.DownloaderID, &dr) != nil || dr.Removed {
		return ""
	}
	return im.downloadContent(d, dr)
}

// downloadContent is the own folder (or one loose file) of a download
// HolyArrs sent, after path pairs; "" for a download it did not send, files
// in more than one place, a missing path, a whole disk or the data folder.
func (im *Importer) downloadContent(d domain.Download, dr domain.Downloader) string {
	root, ok := domain.MapPath(d.SavePath, dr.PathMap)
	if d.ReleaseID == nil || !ok || !filepath.IsAbs(root) || len(d.FilesSeen) == 0 {
		return ""
	}
	top := ""
	for _, f := range d.FilesSeen {
		head, _, inDir := strings.Cut(strings.ReplaceAll(f.Name, `\`, "/"), "/")
		if !inDir && len(d.FilesSeen) > 1 || head == "" || head == "." || head == ".." || (top != "" && head != top) {
			return ""
		}
		top = head
	}
	p := filepath.Join(root, top)
	if _, err := os.Stat(p); err != nil || wholeDisk(p) || im.Store != nil && UnsafeFolder(p, im.Store.Dir()) {
		return ""
	}
	return p
}

// ImportReviewFile imports the one video of a download into the Slot it was
// grabbed for, the way a by-hand import does. With replace, the Slot's file
// goes to the holding folder (Recently deleted) first.
func (im *Importer) ImportReviewFile(ctx context.Context, slotID, downloadID int64, replace bool, by domain.Actor) (ByHandResult, error) {
	var d domain.Download
	if err := database.Get(ctx, im.Store.Read(), "downloads", downloadID, &d); err != nil {
		return ByHandResult{}, err
	}
	if !slices.Contains(d.SlotIDs, slotID) {
		return ByHandResult{Outcome: "refused", Error: "the download was not grabbed for this movie"}, nil
	}
	src := im.ReviewFile(ctx, downloadID)
	if src == "" {
		return ByHandResult{Outcome: "refused", Error: ErrNoReviewFile.Error()}, nil
	}
	unlock := im.Store.LockFiles()
	defer unlock()
	db := im.Store.Read()
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return ByHandResult{}, err
	}
	folders, err := database.Select[mediafiles.LibraryFolder](ctx, db, "library_folders", "1 = 1")
	if err != nil {
		return ByHandResult{}, err
	}
	it := ByHandItem{Path: src, SlotID: slotID, Replace: replace, fromDownload: downloadID}
	r, err := im.importOneByHand(ctx, db, ns, folders, ByHandPlaces{}, it, by)
	if err != nil {
		r.Outcome, r.Error = "failed", err.Error()
	}
	if r.Outcome == "placed" && im.OnImported != nil {
		im.OnImported(ctx, downloadID)
	}
	return r, nil
}

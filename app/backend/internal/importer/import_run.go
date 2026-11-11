package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Import places one download into the library, or only plans it when dryRun is set.
func (im *Importer) Import(ctx context.Context, downloadID int64, dryRun bool, by domain.Actor) (*mediafiles.Import, error) {
	unlock := im.Store.LockFiles()
	defer unlock()
	db := im.Store.Read()
	var d domain.Download
	if err := database.Get(ctx, db, "downloads", downloadID, &d); err != nil {
		return nil, fmt.Errorf("download %d: %w", downloadID, err)
	}
	if d.State != "finished" && d.State != "seeding" {
		return nil, fmt.Errorf("download %d is %s, not finished", downloadID, d.State)
	}
	if len(d.SlotIDs) == 0 {
		return nil, fmt.Errorf("download %d has no Slot", downloadID)
	}
	var dr domain.Downloader
	if err := database.Get(ctx, db, "downloaders", d.DownloaderID, &dr); err != nil {
		return nil, err
	}
	var sl mediafiles.Slot
	var ver mediafiles.Version
	var lf mediafiles.LibraryFolder
	var it mediafiles.Item
	var ti mediafiles.Title
	if err := database.Get(ctx, db, "slots", d.SlotIDs[0], &sl); err != nil {
		return nil, err
	}
	var kind, titleState string
	if err := db.QueryRowContext(ctx, "SELECT t.kind, t.state FROM items i JOIN titles t ON t.id = i.title_id WHERE i.id = ?", sl.ItemID).Scan(&kind, &titleState); err != nil {
		return nil, err
	}
	if titleState == "removed" || sl.State == mediafiles.SlotRemoved {
		return nil, errors.New("the title or item was removed; import stopped")
	}
	if kind == "artist" {
		if !canImportAlbums {
			return nil, ErrAlbumImport
		}
		return im.importShow(ctx, d, dr, dryRun, by)
	}
	if kind == "show" {
		if !canImportEpisodes {
			return nil, ErrEpisodeImport
		}
		return im.importShow(ctx, d, dr, dryRun, by)
	}
	if !dryRun && sl.State != mediafiles.SlotDownloading && sl.State != mediafiles.SlotImporting {
		return nil, fmt.Errorf("slot %d is %s; nothing to import", sl.ID, sl.State)
	}
	for _, g := range []struct {
		table string
		id    int64
		dst   any
	}{{"versions", sl.VersionID, &ver}, {"items", sl.ItemID, &it}} {
		if err := database.Get(ctx, db, g.table, g.id, g.dst); err != nil {
			return nil, err
		}
	}
	if err := database.Get(ctx, db, "library_folders", ver.LibraryFolderID, &lf); err != nil {
		return nil, err
	}
	if err := database.Get(ctx, db, "titles", it.TitleID, &ti); err != nil {
		return nil, err
	}
	year := 0
	if ti.Year != nil {
		year = *ti.Year
	}
	root, rootOK := domain.MapPath(d.SavePath, dr.PathMap)
	cands, bad := candidates(root, d.FilesSeen)
	if !rootOK {
		cands, bad = nil, []mediafiles.PlanStep{unpackStep(d.SavePath, "the Downloader reported a folder with \"..\" in it")}
	}
	folder := safeName(ver.FolderName)
	if folder == "" || folder == "." || folder == ".." {
		folder = FolderName(ti.Name, year)
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return nil, err
	}
	rules, err := ReadFileRules(ctx, db)
	if err != nil {
		return nil, err
	}
	prior, err := database.Select[mediafiles.Import](ctx, db, "imports", "download_id = ? AND state = 'placing' ORDER BY id DESC LIMIT 1", d.ID)
	if err != nil {
		return nil, err
	}
	if len(prior) == 1 && !dryRun {
		restoreMoves(&prior[0])
	}
	unpacked, cleanup, unpackWhy := unpackRars(ctx, d.ID, root, cands, rules, dryRun)
	defer cleanup()
	cands = append(cands, unpacked...)
	t := Target{LibraryPath: lf.Path, FolderName: folder, TitleID: ti.ID, TitleName: ti.Name, Year: year, SlotIDs: d.SlotIDs, IDs: ti.SourceIDs, Naming: &ns, Edition: ver.Edition}
	plan := append(Plan(cands, t), bad...)
	if unpackWhy != "" {
		plan = append(plan, unpackStep(root, unpackWhy))
	}
	lowSpace, release := im.guard(plan, lf.Path, cands, rules)
	defer release()
	minutes := 0
	if it.Runtime != nil {
		minutes = *it.Runtime
	}
	up, err := checkUpgrade(ctx, db, sl, ver, d, plan, im.refuseFakes(ctx, db), minutes)
	if err != nil {
		return nil, err
	}
	im.markMoves(ctx, db, plan, dr, unpacked)
	imp := &mediafiles.Import{DownloadID: &d.ID, FromPath: root, Kind: "movie", Plan: plan, Result: []mediafiles.PlaceResult{}, Mode: "dry_run", State: "planned"}
	if dryRun {
		return imp, nil
	}
	imp.Mode = "auto"
	if lowSpace != nil {
		im.lowSpace(ctx, lowSpace, sl.ID, ti.ID, d.ID, by)
	}
	if up.lie != "" && !hasMain(plan) {
		slotTo, err := im.reject(ctx, imp, sl, d, up.lie, by, up.old != nil, ver.KeepUpToDate && it.KeepUpToDate)
		if err != nil {
			return imp, err
		}
		slotID, dlID, titleID := sl.ID, d.ID, ti.ID
		e := &domain.Event{Actor: by, Kind: "import_rejected", TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID, Summary: "Import rejected: " + up.lie,
			Data: map[string]any{"import_id": imp.ID, "release_id": d.ReleaseID, "reason": up.lie}, GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
		if im.Store.AddEvent(ctx, e) == nil {
			im.publish("event", e)
		}
		im.publish("slot", map[string]any{"id": sl.ID, "state": slotTo, "reason": up.lie})
		im.publish("import", imp)
		if im.OnRejected != nil {
			im.OnRejected(ctx, sl.ID)
		}
		return imp, nil
	}
	resumable := map[string]time.Time{}
	recorded := ""
	if len(prior) == 1 {
		imp.Meta = prior[0].Meta
		for _, r := range prior[0].Result {
			if r.Outcome == "placing" || r.Outcome == "placed" {
				resumable[r.SourceFile] = prior[0].CreatedAt
			}
			if r.HeldAside != "" {
				recorded = r.HeldAside
			}
		}
	}
	for _, st := range plan {
		r := mediafiles.PlaceResult{SourceFile: st.SourceFile, Outcome: "pending", PlacedBy: "none"}
		if st.Method == "hardlink" {
			r.Outcome = "placing"
		}
		imp.Result = append(imp.Result, r)
	}
	var before []mediafiles.PlaceResult
	if len(prior) == 1 {
		before = prior[0].Result
	}
	moveProof(plan, imp.Result, before)
	imp.State = "placing"
	moved := false
	if err := im.Store.Write(ctx, func(tx *sql.Tx) error {
		if sl.State == mediafiles.SlotDownloading {
			if err := mediafiles.MoveSlotTx(tx, sl.ID, mediafiles.SlotImporting, "importing", by); err != nil {
				return err
			}
			moved = true
		}
		if imp.ID > 0 {
			return database.Update(tx, "imports", imp, by)
		}
		_, err := database.Insert(tx, "imports", imp, by)
		return err
	}); err != nil {
		return nil, err
	}
	if moved {
		im.publish("slot", map[string]any{"id": sl.ID, "state": mediafiles.SlotImporting})
	}

	swept := map[string]bool{}
	for _, st := range plan {
		if st.Method == "hardlink" && !swept[filepath.Dir(st.TargetPath)] {
			swept[filepath.Dir(st.TargetPath)] = true
			sweepStaleParts(filepath.Dir(st.TargetPath))
		}
	}
	heldEarly, sameIdx, oldAbs := "", -1, ""
	if up.old != nil {
		oldAbs = filepath.Join(lf.Path, filepath.FromSlash(up.old.RelativePath))
		for i, st := range plan {
			if st.Method == "hardlink" && filepath.Clean(st.TargetPath) == filepath.Clean(oldAbs) {
				sameIdx = i
			}
		}
	}
	if sameIdx >= 0 {
		if _, err := os.Lstat(recorded); recorded != "" && err == nil {
			heldEarly = recorded
		} else if _, err := os.Lstat(oldAbs); err == nil {
			to := recorded
			if to == "" {
				if to, err = holdPath(lf.Path, up.old.RelativePath, time.Now()); err != nil {
					return imp, fmt.Errorf("moving the old copy aside: %w", err)
				}
			}
			imp.Result[sameIdx].HeldAside = to
			if err := im.Store.Write(ctx, func(tx *sql.Tx) error { return database.Update(tx, "imports", imp, by) }); err != nil {
				return imp, err
			}
			if err := moveFile(oldAbs, to); err != nil {
				return imp, fmt.Errorf("moving the old copy aside: %w", err)
			}
			heldEarly = to
		}
		if crashAt != nil {
			if err := crashAt("held"); err != nil {
				return imp, err
			}
		}
	}
	proof := imp.Result
	imp.Result = execute(plan, im.Ops, resumable)
	keepProof(imp.Result, proof)
	if heldEarly != "" {
		imp.Result[sameIdx].HeldAside = heldEarly
	}
	if crashAt != nil {
		if err := crashAt("placed"); err != nil {
			return imp, err
		}
	}
	replacedTo := heldEarly
	if up.old != nil {
		placedAny := placedSome(imp.Result)
		switch {
		case !placedAny && heldEarly != "":
			if err := moveFile(heldEarly, oldAbs); err != nil {
				log.Printf("importer: putting the old copy back from %s: %v", heldEarly, err)
			} else {
				replacedTo = ""
				imp.Result[sameIdx].HeldAside = ""
			}
		case placedAny && heldEarly == "" && sameIdx < 0:
			to, err := recorded, error(nil)
			if _, lerr := os.Lstat(oldAbs); lerr != nil {
				if _, rerr := os.Lstat(recorded); recorded == "" || rerr != nil {
					to = ""
				}
			} else {
				if to == "" {
					to, err = holdPath(lf.Path, up.old.RelativePath, time.Now())
				}
				if err == nil {
					for i := range imp.Result {
						if imp.Result[i].Outcome == "placed" {
							imp.Result[i].HeldAside = to
							break
						}
					}
					err = im.Store.Write(ctx, func(tx *sql.Tx) error { return database.Update(tx, "imports", imp, by) })
				}
				if err == nil {
					err = moveFile(oldAbs, to)
				}
				if err == nil && crashAt != nil {
					if cerr := crashAt("held_late"); cerr != nil {
						return imp, cerr
					}
				}
			}
			if err != nil {
				to = ""
				log.Printf("importer: moving the replaced copy aside: %v", err)
				e := &domain.Event{Actor: by, Kind: "file_hold_failed", SlotID: &sl.ID, Summary: "Could not move the replaced copy aside: " + err.Error(),
					Data: map[string]any{"file_id": up.old.ID, "path": up.old.RelativePath}, GroupKey: "slot:" + strconv.FormatInt(sl.ID, 10)}
				if im.Store.AddEvent(ctx, e) == nil {
					im.publish("event", e)
				}
			}
			replacedTo = to
		}
	}
	sizes := make([]int64, len(plan))
	for i, r := range imp.Result {
		if r.Outcome == "placed" {
			if fi, err := os.Stat(plan[i].TargetPath); err == nil {
				sizes[i] = fi.Size()
			}
		}
	}
	how := ""
	for _, r := range imp.Result {
		if r.Outcome == "placed" {
			how = r.PlacedBy
			if r.FellBack {
				how += " (hardlink not possible)"
			}
		}
	}
	slotTo, reason := mediafiles.SlotNeedsADecision, "nothing could be placed; look at the import plan"
	if unpackWhy != "" {
		reason = unpackWhy
	}
	if up.copyWhy != "" {
		reason = up.copyWhy
	}
	for _, r := range imp.Result {
		if strings.HasPrefix(r.Error, "not enough free space") {
			reason = "nothing could be placed: not enough free space in the Library folder"
		}
	}
	placed, failed := 0, 0
	var fileID int64
	newName := ""
	now := time.Now().UTC()
	err = im.Store.Write(ctx, func(tx *sql.Tx) error {
		if up.old != nil && placedSome(imp.Result) {
			relPath := up.old.RelativePath
			if replacedTo != "" {
				if rp, err := filepath.Rel(lf.Path, replacedTo); err == nil {
					relPath = filepath.ToSlash(rp)
				}
			} else if sameIdx >= 0 {
				relPath += "#replaced-" + strconv.FormatInt(up.old.ID, 10)
			}
			if _, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				relPath, database.FormatTime(now), actorText(by), up.old.ID); err != nil {
				return err
			}
		}
		for i, r := range imp.Result {
			switch r.Outcome {
			case "placed":
				placed++
			case "failed":
				failed++
				continue
			default:
				continue
			}
			step := plan[i]
			rel, err := filepath.Rel(lf.Path, step.TargetPath)
			if err != nil {
				return err
			}
			p := releasename.Parse(filepath.Base(step.SourceFile), releasename.Hints{})
			size := sizes[i]
			name := strings.TrimSuffix(filepath.Base(step.SourceFile), filepath.Ext(step.SourceFile))
			newName = name
			f := &mediafiles.File{SlotIDs: step.TargetSlotIDs, VersionID: ver.ID, RelativePath: filepath.ToSlash(rel), Size: size, AddedAt: now,
				Quality:   mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value},
				Languages: InspectedLanguages(up.facts[step.SourceFile]), MediaInfo: map[string]any{"step": mediafiles.StepFor(p)}, ReleaseName: &name, Origin: "imported", PlacedBy: r.PlacedBy,
				FellBack: r.FellBack, Role: "main", State: "present"}
			if f.Languages == nil {
				f.Languages = []string{}
			}
			for k, value := range up.facts[step.SourceFile] {
				f.MediaInfo[k] = value
			}
			if g := p.Group.Value; g != "" {
				f.Group = &g
			}
			id, err := database.Insert(tx, "files", f, by)
			if err != nil {
				return err
			}
			fileID = id
		}
		switch {
		case placed > 0 && failed == 0:
			imp.State = "done"
		case placed > 0:
			imp.State = "partly_done"
		default:
			imp.State = "failed"
		}
		if err := database.Update(tx, "imports", imp, by); err != nil {
			return err
		}
		ge := false
		if fileID > 0 {
			ge = mediafiles.ReachesGoodEnough(newName, up.rules)
			if _, err := tx.Exec("UPDATE slots SET file_id = ?, active_download_id = NULL, good_enough = ? WHERE id = ?", fileID, ge, sl.ID); err != nil {
				return err
			}
			slotTo, reason = mediafiles.SlotInLibrary, "placed by "+how
			if up.old != nil {
				reason += "; replaced " + up.oldName
			}
		}
		if err := mediafiles.MoveSlotTx(tx, sl.ID, slotTo, reason, by); err != nil {
			return err
		}
		if slotTo == mediafiles.SlotInLibrary && !ge && ver.KeepUpToDate && it.KeepUpToDate {
			slotTo = mediafiles.SlotBetterWanted
			return mediafiles.MoveSlotTx(tx, sl.ID, slotTo, reason+"; below good enough, so a better copy is sought", by)
		}
		return nil
	})
	if err != nil {
		return imp, err
	}
	im.publish("slot", map[string]any{"id": sl.ID, "state": slotTo, "reason": reason})
	summary := fmt.Sprintf("Import %s: %d placed", imp.State, placed)
	if how != "" {
		summary += " by " + how
	}
	if placed == 0 && imp.State == "failed" {
		summary = "Import failed: " + reason
	}
	slotID, dlID, titleID := sl.ID, d.ID, ti.ID
	e := &domain.Event{Actor: by, Kind: "import_" + imp.State, TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID, Summary: summary,
		Data: map[string]any{"import_id": imp.ID, "result": imp.Result, "from": imp.FromPath, "paths": planPaths(imp.Plan)}, GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
	if im.Store.AddEvent(ctx, e) == nil {
		im.publish("event", e)
	}
	if up.old != nil && fileID > 0 {
		e := &domain.Event{Actor: by, Kind: "file_replaced", TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID,
			Summary: "Replaced " + up.oldName + " with a better copy", Data: map[string]any{"old_file_id": up.old.ID, "new_file_id": fileID, "moved_to": replacedTo},
			GroupKey: "slot:" + strconv.FormatInt(slotID, 10)}
		im.upgradeFacts(ctx, e.Data, up.old.ID, fileID)
		if im.Store.AddEvent(ctx, e) == nil {
			im.publish("event", e)
		}
	}
	im.publish("import", imp)
	if imp.State == "done" && !dryRun && im.OnImported != nil {
		im.OnImported(ctx, d.ID)
	}
	return imp, nil
}

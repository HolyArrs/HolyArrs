package importer

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func (im *Importer) importShow(ctx context.Context, d domain.Download, dr domain.Downloader, dryRun bool, by domain.Actor) (*mediafiles.Import, error) {
	if !dryRun {
		mu, _ := im.runs.LoadOrStore(d.ID, &sync.Mutex{})
		mu.(*sync.Mutex).Lock()
		defer mu.(*sync.Mutex).Unlock()
		if showImportEntered != nil {
			showImportEntered(d.ID)
		}
		if err := database.Get(ctx, im.Store.Read(), "downloads", d.ID, &d); err != nil {
			return nil, err
		}
	}
	db := im.Store.Read()
	slots := map[int64]*showSlot{}
	var ver mediafiles.Version
	var ti mediafiles.Title
	for i, id := range d.SlotIDs {
		ss := &showSlot{step: -1}
		if err := database.Get(ctx, db, "slots", id, &ss.sl); err != nil {
			return nil, err
		}
		if err := database.Get(ctx, db, "items", ss.sl.ItemID, &ss.it); err != nil {
			return nil, err
		}
		if i == 0 {
			if err := database.Get(ctx, db, "versions", ss.sl.VersionID, &ver); err != nil {
				return nil, err
			}
			if err := database.Get(ctx, db, "titles", ss.it.TitleID, &ti); err != nil {
				return nil, err
			}
		}
		if ss.sl.VersionID != ver.ID || ss.it.TitleID != ti.ID {
			return nil, fmt.Errorf("download %d fills Slots of more than one show or Version", d.ID)
		}
		// A Slot another download now fills (a replacement grab) does not wait for this one.
		ss.live = (ss.sl.State == mediafiles.SlotDownloading || ss.sl.State == mediafiles.SlotImporting) &&
			(ss.sl.ActiveDownloadID == nil || *ss.sl.ActiveDownloadID == d.ID)
		slots[id] = ss
	}
	files, waiting := packReady(d)
	for id := range waiting {
		if ss := slots[id]; ss != nil && ss.live && !dryRun {
			ss.live = false // its file is switched on but not finished: it stays downloading
		} else {
			delete(waiting, id)
		}
	}
	liveAny := false
	for _, ss := range slots {
		liveAny = liveAny || ss.live
	}
	if !dryRun && !liveAny && len(waiting) > 0 {
		if err := reopenPack(ctx, im.Store, d.ID); err != nil {
			return nil, err
		}
		return nil, ErrPackWaiting
	}
	if !dryRun && !liveAny {
		return nil, fmt.Errorf("no Slot of download %d waits for it; nothing to import", d.ID)
	}
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", ver.LibraryFolderID, &lf); err != nil {
		return nil, err
	}
	var pr mediafiles.PickRules
	if err := database.Get(ctx, db, "pick_rules", ver.PickRulesID, &pr); err != nil {
		return nil, err
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return nil, err
	}
	rules, err := ReadFileRules(ctx, db)
	if err != nil {
		return nil, err
	}
	music := ti.Kind == "artist"
	itemWhere := "title_id = ? AND kind = 'episode' AND dropped_at IS NULL"
	args := []any{ti.ID}
	if music {
		part := slots[d.SlotIDs[0]].it.PartID
		if part == nil {
			return nil, fmt.Errorf("download %d: its first track belongs to no album; an album import needs one", d.ID)
		}
		itemWhere = "title_id = ? AND kind = 'track' AND dropped_at IS NULL AND part_id = ?"
		args = append(args, *part)
	}
	items, err := database.Select[mediafiles.Item](ctx, db, "items", itemWhere, args...)
	if err != nil {
		return nil, err
	}
	verSlots, err := database.Select[mediafiles.Slot](ctx, db, "slots", "version_id = ?", ver.ID)
	if err != nil {
		return nil, err
	}
	slotOf := map[int64]int64{}
	for _, s := range verSlots {
		slotOf[s.ItemID] = s.ID
	}
	year := 0
	if ti.Year != nil {
		year = *ti.Year
	}
	relName := ""
	if d.ReleaseID != nil {
		_ = db.QueryRowContext(ctx, "SELECT name FROM releases WHERE id = ?", *d.ReleaseID).Scan(&relName)
	}
	t := ShowTarget{LibraryPath: lf.Path, Show: mediafiles.TitleRef{ID: ti.ID, Name: ti.Name, Aliases: OtherNames(ti), Year: year, Kind: "show"},
		IDs: ti.SourceIDs, Order: mediafiles.OrderAired, ReleaseName: relName, Naming: &ns, ByOrder: ti.EpisodeGroupID != nil && *ti.EpisodeGroupID != ""}
	if ti.EpisodeOrder != nil && *ti.EpisodeOrder != "" {
		t.Order = *ti.EpisodeOrder
	}
	t.FolderName = safeName(ver.FolderName)
	if t.FolderName == "" || t.FolderName == "." || t.FolderName == ".." {
		t.FolderName = mediafiles.ShowFolder(ns, mediafiles.NameFacts{Title: ti.Name, Year: year})
	}
	for _, it := range items {
		e := Episode{Ref: mediafiles.EpisodeRef{ItemID: it.ID}, SlotID: slotOf[it.ID]}
		if it.Numbers.Season != nil {
			e.Ref.Season = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			e.Ref.Episode = *it.Numbers.Episode
		}
		if it.Numbers.Absolute != nil {
			e.Ref.Absolute = *it.Numbers.Absolute
		}
		if it.Numbers.AltSeason != nil && it.Numbers.AltEpisode != nil {
			e.Ref.AltSeason, e.Ref.AltEpisode = *it.Numbers.AltSeason, *it.Numbers.AltEpisode
		}
		if it.AirDay != nil {
			e.Ref.AirDay = *it.AirDay
		}
		if it.Name != nil {
			e.Name = *it.Name
		}
		if ss := slots[e.SlotID]; ss != nil && (ss.live || dryRun) {
			e.Wanted = true
		}
		t.Episodes = append(t.Episodes, e)
	}
	root, rootOK := domain.MapPath(d.SavePath, dr.PathMap)
	cands, bad := candidates(root, files)
	if !rootOK {
		cands, bad = nil, []mediafiles.PlanStep{unpackStep(d.SavePath, "the Downloader reported a folder with \"..\" in it")}
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
	if unpackWhy != "" {
		bad = append(bad, unpackStep(root, unpackWhy))
	}
	var plan []mediafiles.PlanStep
	if music {
		at, err := albumTarget(ctx, db, lf, ver, ti, items, slotOf, slots, dryRun, ns)
		if err != nil {
			return nil, err
		}
		plan = PlanTracks(cands, at)
	} else {
		plan = PlanEpisodes(cands, t)
	}
	plan = append(plan, bad...)
	gone := sourcesGone(root, plan, dr.Protocol == "usenet")
	lowSpace, release := im.guard(plan, lf.Path, cands, rules)
	defer release()
	if music {
		wholeAlbumSpace(plan, lowSpace)
	}

	filled := map[int64]bool{}
	for _, st := range plan {
		if st.Method == "hardlink" {
			for _, id := range st.TargetSlotIDs {
				filled[id] = true
			}
		}
	}
	olds := map[int][]mediafiles.File{}
	inspected := map[string]map[string]any{}
	fakes := im.refuseFakes(ctx, db)
	var claimed int64
	if d.ReleaseID != nil {
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(size, 0) FROM releases WHERE id = ?", *d.ReleaseID).Scan(&claimed)
	}
	mismatch := fakes.claimMismatch(claimed, claimSet(d, cands))
	for i := range plan {
		if plan[i].Method != "hardlink" {
			continue
		}
		seen := map[int64]bool{}
		for _, id := range plan[i].TargetSlotIDs {
			ss := slots[id]
			if ss == nil || ss.sl.FileID == nil || seen[*ss.sl.FileID] {
				continue
			}
			seen[*ss.sl.FileID] = true
			var f mediafiles.File
			if err := database.Get(ctx, db, "files", *ss.sl.FileID, &f); err == nil && f.State == "present" {
				olds[i] = append(olds[i], f)
			}
		}
		copyWhy := ""
		for _, old := range olds[i] {
			if old.Protected {
				copyWhy = "the copy in place is protected; choose another Version for an upgrade"
			}
		}
		if !music && copyWhy == "" {
			facts := InspectFacts(plan[i].SourceFile)
			inspected[plan[i].SourceFile] = facts
			copyWhy = copyRefusal(ver, filepath.Base(plan[i].SourceFile), relName, "", facts)
		}
		if copyWhy != "" {
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, copyWhy)
			delete(olds, i)
			continue
		}
		minutes := 0
		for _, id := range plan[i].TargetSlotIDs {
			if ss := slots[id]; ss != nil && ss.it.Runtime != nil && minutes >= 0 {
				minutes += *ss.it.Runtime
			} else {
				minutes = -1
			}
		}
		if !music && sampleByLength(inspected[plan[i].SourceFile], fakes.lengthMinutes(max(minutes, 0))) {
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, "a sample, not the full video (it plays "+lengthWords(factInt(inspected[plan[i].SourceFile], "runtime_seconds"))+")")
			delete(olds, i)
			continue
		}
		why, err := fakes.fakeFileOf(plan[i].SourceFile, music, max(minutes, 0), relName)
		if err != nil {
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, unreadable(err))
			continue
		}
		if why == "" {
			why = mismatch
		}
		if why != "" {
			for _, id := range plan[i].TargetSlotIDs {
				slots[id].lie = why
			}
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, why)
			continue
		}
		name := strings.TrimSuffix(filepath.Base(plan[i].SourceFile), filepath.Ext(plan[i].SourceFile))
		oldName := ""
		if len(olds[i]) > 0 {
			oldName = fileName(olds[i][0])
		}
		why = lieOf(name, relName, len(olds[i]) > 0, oldName, pr.Body, inspected[plan[i].SourceFile])
		if why == "" && music && len(olds[i]) > 0 {
			why = lossyOverLossless(plan[i].SourceFile, relName, olds[i][0], filepath.Join(lf.Path, filepath.FromSlash(olds[i][0].RelativePath)), pr.Body)
		}
		if why != "" {
			for _, id := range plan[i].TargetSlotIDs {
				slots[id].lie = why
			}
			plan[i].Method, plan[i].TargetPath = "refuse", ""
			plan[i].Why = append(plan[i].Why, why)
			delete(olds, i)
			continue
		}
		if len(olds[i]) > 0 {
			id := olds[i][0].ID
			plan[i].ReplacesFileID = &id
		}
	}
	filled = map[int64]bool{}
	for _, st := range plan {
		if st.Method == "hardlink" {
			for _, id := range st.TargetSlotIDs {
				filled[id] = true
			}
		}
	}
	for i, st := range plan {
		for _, id := range st.TargetSlotIDs {
			if ss := slots[id]; ss != nil && st.Method == "hardlink" {
				ss.step = i
			}
		}
	}
	im.markMoves(ctx, db, plan, dr, unpacked)
	imp := &mediafiles.Import{DownloadID: &d.ID, FromPath: root, Kind: map[bool]string{false: "episodes", true: "tracks"}[music], Plan: plan, Result: []mediafiles.PlaceResult{}, Mode: "dry_run", State: "planned"}
	if dryRun {
		return imp, nil
	}
	imp.Mode = "auto"
	firstSlot := d.SlotIDs[0]
	if lowSpace != nil {
		im.lowSpace(ctx, lowSpace, firstSlot, ti.ID, d.ID, by)
	}

	resumable := map[string]time.Time{}
	recorded := []string{}
	if len(prior) == 1 {
		imp.Meta = prior[0].Meta
		for _, r := range prior[0].Result {
			if r.Outcome == "placing" || r.Outcome == "placed" {
				resumable[r.SourceFile] = prior[0].CreatedAt
			}
			if r.HeldAside != "" {
				recorded = append(recorded, r.HeldAside)
			}
			recorded = append(recorded, r.HeldAsideMore...)
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
	var moved []int64
	if err := im.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range d.SlotIDs {
			if ss := slots[id]; ss.live && ss.sl.State == mediafiles.SlotDownloading {
				if err := mediafiles.MoveSlotTx(tx, id, mediafiles.SlotImporting, "importing", by); err != nil {
					return err
				}
				moved = append(moved, id)
			}
		}
		if imp.ID > 0 {
			return database.Update(tx, "imports", imp, by)
		}
		_, err := database.Insert(tx, "imports", imp, by)
		return err
	}); err != nil {
		return nil, err
	}
	for _, id := range moved {
		im.publish("slot", map[string]any{"id": id, "state": mediafiles.SlotImporting})
	}
	swept := map[string]bool{}
	for _, st := range plan {
		if st.Method == "hardlink" && !swept[filepath.Dir(st.TargetPath)] {
			swept[filepath.Dir(st.TargetPath)] = true
			sweepStaleParts(filepath.Dir(st.TargetPath))
		}
	}
	whole := func(f mediafiles.File) bool {
		for _, id := range f.SlotIDs {
			if !filled[id] {
				return false
			}
		}
		return true
	}
	abs := func(f mediafiles.File) string { return filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath)) }
	save := func() error {
		return im.Store.Write(ctx, func(tx *sql.Tx) error { return database.Update(tx, "imports", imp, by) })
	}
	heldFirst := map[int]string{}
	for i, fs := range olds {
		for _, f := range fs {
			if !whole(f) || filepath.Clean(abs(f)) != filepath.Clean(plan[i].TargetPath) {
				continue
			}
			if r := heldFrom(recorded, f.RelativePath); r != "" {
				heldFirst[i] = r
				continue
			}
			if _, err := os.Lstat(abs(f)); err != nil {
				continue
			}
			to, err := holdPath(lf.Path, f.RelativePath, time.Now())
			if err != nil {
				return imp, fmt.Errorf("moving the old copy aside: %w", err)
			}
			imp.Result[i].HeldAside = to
			if err := save(); err != nil {
				return imp, err
			}
			if err := moveFile(abs(f), to); err != nil {
				return imp, fmt.Errorf("moving the old copy aside: %w", err)
			}
			heldFirst[i] = to
		}
	}
	if crashAt != nil && len(heldFirst) > 0 {
		if err := crashAt("held"); err != nil {
			return imp, err
		}
	}
	proof := imp.Result
	imp.Result = execute(plan, im.Ops, resumable)
	keepProof(imp.Result, proof)
	im.placeSideFiles(ctx, imp.FromPath, plan, imp.Result, true)
	for i, to := range heldFirst {
		imp.Result[i].HeldAside = to
	}
	if crashAt != nil {
		if err := crashAt("placed"); err != nil {
			return imp, err
		}
	}
	replacedTo := map[int64]string{}
	for i, fs := range olds {
		placed := imp.Result[i].Outcome == "placed"
		for _, f := range fs {
			if !whole(f) {
				continue
			}
			sameSpot := filepath.Clean(abs(f)) == filepath.Clean(plan[i].TargetPath)
			switch {
			case sameSpot && !placed:
				if to := heldFirst[i]; to != "" {
					if err := moveFile(to, abs(f)); err != nil {
						log.Printf("importer: putting the old copy back from %s: %v", to, err)
					} else {
						imp.Result[i].HeldAside = ""
					}
				}
			case sameSpot:
				replacedTo[f.ID] = heldFirst[i]
			case placed:
				to := heldFrom(recorded, f.RelativePath)
				if _, err := os.Lstat(abs(f)); err == nil && to == "" {
					var herr error
					if to, herr = holdPath(lf.Path, f.RelativePath, time.Now()); herr == nil {
						imp.Result[i].HeldAsideMore = append(imp.Result[i].HeldAsideMore, to)
						if herr = save(); herr == nil {
							herr = moveFile(abs(f), to)
						}
					}
					if herr != nil {
						to = ""
						log.Printf("importer: moving the replaced copy aside: %v", herr)
						e := &domain.Event{Actor: by, Kind: "file_hold_failed", SlotID: &firstSlot, Summary: "Could not move the replaced copy aside: " + herr.Error(),
							Data: map[string]any{"file_id": f.ID, "path": f.RelativePath}, GroupKey: "download:" + strconv.FormatInt(d.ID, 10)}
						if im.Store.AddEvent(ctx, e) == nil {
							im.publish("event", e)
						}
					}
				}
				replacedTo[f.ID] = to
			}
		}
	}
	if crashAt != nil && len(replacedTo) > 0 {
		if err := crashAt("held_late"); err != nil {
			return imp, err
		}
	}

	matched := false
	for _, st := range plan {
		matched = matched || len(st.TargetSlotIDs) > 0
	}
	for _, ss := range slots {
		if !ss.live {
			continue
		}
		switch {
		case ss.step >= 0 && imp.Result[ss.step].Outcome == "placed":
			ss.outcome = "placed"
		case gone:
			// Another app imported the files and removed them: let the
			// download go, as Sonarr does, and keep the episode wanted.
			ss.outcome, ss.reason = "gone", FilesGoneReason
		case ss.lie != "":
			ss.outcome, ss.reason = "lied", ss.lie
		case ss.step >= 0:
			ss.outcome = "stuck"
			ss.reason = "the file could not be placed: " + imp.Result[ss.step].Error
		case matched:
			ss.outcome, ss.reason = "missing", "the download did not hold "+refLabel(ss.it)
			for i, st := range plan {
				if slices.Contains(st.TargetSlotIDs, ss.sl.ID) {
					ss.outcome, ss.reason = "stuck", "the file for "+refLabel(ss.it)+" could not be placed: "+strings.Join(plan[i].Why, "; ")
				}
			}
		default:
			ss.outcome, ss.reason = "stuck", im.nothingMatched(dr, files, music)
			if unpackWhy != "" {
				ss.reason = unpackWhy
			}
			for _, r := range imp.Result {
				if strings.HasPrefix(r.Error, "not enough free space") {
					ss.reason = "nothing could be placed: not enough free space in the Library folder"
				}
			}
		}
	}
	if gone {
		markGone(imp.Result)
	}
	kept := func(ss *showSlot) bool { return ver.KeepUpToDate && ss.it.KeepUpToDate && ti.KeepUpToDate }
	placed, failed := 0, 0
	for _, r := range imp.Result {
		switch r.Outcome {
		case "placed":
			placed++
		case "failed":
			failed++
		}
	}
	var lookAgain []int64
	newFiles := map[int64]int64{}
	now := time.Now().UTC()
	err = im.Store.Write(ctx, func(tx *sql.Tx) error {
		for i, fs := range olds {
			if imp.Result[i].Outcome != "placed" {
				continue
			}
			for _, f := range fs {
				if !whole(f) {
					var rest []int64
					for _, id := range f.SlotIDs {
						if !filled[id] {
							rest = append(rest, id)
						}
					}
					if _, err := tx.Exec("UPDATE files SET slot_ids = ?, updated_at = ?, updated_by = ? WHERE id = ?",
						jsonIDs(rest), database.FormatTime(now), actorText(by), f.ID); err != nil {
						return err
					}
					continue
				}
				rel := f.RelativePath
				if to := replacedTo[f.ID]; to != "" {
					if rp, err := filepath.Rel(lf.Path, to); err == nil {
						rel = filepath.ToSlash(rp)
					}
				} else {
					rel += "#replaced-" + strconv.FormatInt(f.ID, 10)
				}
				if _, err := tx.Exec("UPDATE files SET state = 'replaced', relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?",
					rel, database.FormatTime(now), actorText(by), f.ID); err != nil {
					return err
				}
			}
		}
		for i, r := range imp.Result {
			if r.Outcome != "placed" {
				continue
			}
			step := plan[i]
			rel, err := filepath.Rel(lf.Path, step.TargetPath)
			if err != nil {
				return err
			}
			var size int64
			if fi, err := os.Stat(step.TargetPath); err == nil {
				size = fi.Size()
			}
			p := releasename.Parse(filepath.Base(step.SourceFile), releasename.Hints{Show: !music})
			name := strings.TrimSuffix(filepath.Base(step.SourceFile), filepath.Ext(step.SourceFile))
			if music && relName != "" {
				name = relName
			}
			f := &mediafiles.File{SlotIDs: step.TargetSlotIDs, VersionID: ver.ID, RelativePath: filepath.ToSlash(rel), Size: size, AddedAt: now,
				Quality:   mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value},
				Languages: p.Languages, MediaInfo: map[string]any{"step": mediafiles.StepFor(p)}, ReleaseName: &name, Origin: "imported", PlacedBy: r.PlacedBy,
				FellBack: r.FellBack, Role: "main", State: "present"}
			for k, value := range inspected[step.SourceFile] {
				f.MediaInfo[k] = value
			}
			if !music {
				f.Languages = InspectedLanguages(inspected[step.SourceFile])
			}
			if music {
				f.Quality = mediafiles.Quality{Source: strings.TrimPrefix(strings.ToLower(filepath.Ext(step.SourceFile)), ".")}
			}
			if f.Languages == nil {
				f.Languages = []string{}
			}
			if g := p.Group.Value; g != "" {
				f.Group = &g
			}
			if f.Protected, err = mediafiles.TakeNotesAtTx(ctx, tx, ver.ID, f.RelativePath); err != nil {
				return err
			}
			id, err := database.Insert(tx, "files", f, by)
			if err != nil {
				return err
			}
			for _, o := range olds[i] {
				newFiles[o.ID] = id
			}
			how := r.PlacedBy
			if r.FellBack {
				how += " (hardlink not possible)"
			}
			ge := mediafiles.ReachesGoodEnough(name, pr.Body)
			for _, sid := range step.TargetSlotIDs {
				ss := slots[sid]
				if _, err := tx.Exec("UPDATE slots SET file_id = ?, active_download_id = NULL, good_enough = ? WHERE id = ?", id, ge, sid); err != nil {
					return err
				}
				ss.to, ss.reason = mediafiles.SlotInLibrary, "placed by "+how
				if len(olds[i]) > 0 {
					ss.reason += "; replaced " + fileName(olds[i][0])
				}
				if err := mediafiles.MoveSlotTx(tx, sid, ss.to, ss.reason, by); err != nil {
					return err
				}
				if !ge && kept(ss) {
					ss.to = mediafiles.SlotBetterWanted
					if err := mediafiles.MoveSlotTx(tx, sid, ss.to, ss.reason+"; below good enough, so a better copy is sought", by); err != nil {
						return err
					}
				}
			}
		}
		var downFor []int64
		for _, id := range d.SlotIDs {
			ss := slots[id]
			if !ss.live || ss.outcome == "placed" {
				continue
			}
			hasFile := ss.sl.FileID != nil
			switch ss.outcome {
			case "lied", "missing":
				downFor = append(downFor, id)
				if _, err := tx.Exec("UPDATE slots SET active_download_id = NULL WHERE id = ?", id); err != nil {
					return err
				}
				switch {
				case ss.outcome == "lied" && !hasFile:
					ss.to = mediafiles.SlotRejected
				case ss.outcome == "lied":
					ss.to = mediafiles.SlotInLibrary
				case hasFile:
					ss.to = mediafiles.SlotBetterWanted
				default:
					ss.to = mediafiles.SlotWanted
				}
				if err := mediafiles.MoveSlotTx(tx, id, ss.to, ss.reason, by); err != nil {
					return err
				}
				if ss.to == mediafiles.SlotInLibrary && kept(ss) && !ss.sl.GoodEnough {
					ss.to = mediafiles.SlotBetterWanted
					if err := mediafiles.MoveSlotTx(tx, id, ss.to, ss.reason+"; the copy in place stays and a better one is sought", by); err != nil {
						return err
					}
				}
				if ss.to != mediafiles.SlotInLibrary {
					lookAgain = append(lookAgain, id)
				}
			case "gone":
				if err := mediafiles.ClearSlotDownloadTx(tx, id); err != nil {
					return err
				}
				ss.to = mediafiles.SlotWanted
				if hasFile {
					ss.to = mediafiles.SlotBetterWanted
				}
				if err := mediafiles.MoveSlotTx(tx, id, ss.to, ss.reason, by); err != nil {
					return err
				}
			default:
				ss.to = mediafiles.SlotNeedsADecision
				if err := mediafiles.MoveSlotTx(tx, id, ss.to, ss.reason, by); err != nil {
					return err
				}
			}
		}
		if len(downFor) > 0 && d.ReleaseID != nil {
			if err := turnDownForTx(tx, *d.ReleaseID, downFor, by); err != nil {
				return err
			}
		}
		switch {
		case placed > 0 && failed == 0 && len(downFor) == 0 && !anyStuck(slots):
			imp.State = "done"
		case placed > 0:
			imp.State = "partly_done"
		case len(downFor) > 0 && !anyStuck(slots):
			imp.State = "rejected"
		default:
			imp.State = "failed"
		}
		return database.Update(tx, "imports", imp, by)
	})
	if err != nil {
		return imp, err
	}
	titleID, dlID := ti.ID, d.ID
	group := "download:" + strconv.FormatInt(d.ID, 10)
	for _, id := range d.SlotIDs {
		ss := slots[id]
		if ss.live && ss.to != "" {
			im.publish("slot", map[string]any{"id": id, "state": ss.to, "reason": ss.reason})
		}
		if ss.outcome == "lied" || ss.outcome == "missing" {
			slotID := id
			e := &domain.Event{Actor: by, Kind: "import_rejected", TitleID: &titleID, SlotID: &slotID, DownloadID: &dlID,
				Summary: "Import rejected for " + refLabel(ss.it) + ": " + ss.reason,
				Data:    map[string]any{"import_id": imp.ID, "release_id": d.ReleaseID, "reason": ss.reason}, GroupKey: "slot:" + strconv.FormatInt(id, 10)}
			if im.Store.AddEvent(ctx, e) == nil {
				im.publish("event", e)
			}
		}
	}
	var placedLabels []string
	placedIDs := []int64{}
	for _, id := range d.SlotIDs {
		if slots[id].outcome == "placed" {
			placedLabels = append(placedLabels, refLabel(slots[id].it))
			placedIDs = append(placedIDs, id)
		}
	}
	summary := fmt.Sprintf("Import %s: %d placed", imp.State, placed)
	if len(placedLabels) > 0 {
		summary += " (" + strings.Join(placedLabels, ", ") + ")"
	}
	if placed == 0 && imp.State == "failed" {
		why := unpackWhy
		for _, id := range d.SlotIDs {
			if why == "" && slots[id].reason != "" {
				why = slots[id].reason
			}
		}
		if why != "" {
			summary = "Import failed: " + why
		}
	}
	e := &domain.Event{Actor: by, Kind: "import_" + imp.State, TitleID: &titleID, SlotID: &firstSlot, DownloadID: &dlID, Summary: summary,
		Data: map[string]any{"import_id": imp.ID, "result": imp.Result, "from": imp.FromPath, "paths": planPaths(imp.Plan), "slot_ids": d.SlotIDs,
			"placed_slot_ids": placedIDs}, GroupKey: group}
	if im.Store.AddEvent(ctx, e) == nil {
		im.publish("event", e)
	}
	for oldID, newID := range newFiles {
		var f mediafiles.File
		if database.Get(ctx, im.Store.Read(), "files", oldID, &f) != nil || f.State != "replaced" {
			continue
		}
		e := &domain.Event{Actor: by, Kind: "file_replaced", TitleID: &titleID, SlotID: &firstSlot, DownloadID: &dlID,
			Summary: "Replaced " + fileName(f) + " with a better copy", Data: map[string]any{"old_file_id": oldID, "new_file_id": newID, "moved_to": replacedTo[oldID]},
			GroupKey: group}
		im.upgradeFacts(ctx, e.Data, oldID, newID)
		if im.Store.AddEvent(ctx, e) == nil {
			im.publish("event", e)
		}
	}
	im.publish("import", imp)
	if len(waiting) > 0 {
		if err := reopenPack(ctx, im.Store, d.ID); err != nil {
			log.Printf("importer: download %d back to downloading: %v", d.ID, err)
		}
	}
	if imp.State == "done" && !dryRun && im.OnImported != nil {
		im.OnImported(ctx, d.ID)
	}
	if im.OnRejected != nil {
		for _, id := range lookAgain {
			im.OnRejected(ctx, id)
		}
	}
	if gone && im.OnFilesGone != nil {
		im.OnFilesGone(ctx, ti.ID)
	}
	return imp, nil
}

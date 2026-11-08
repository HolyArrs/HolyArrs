package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// applyRenameLocked requires the Store file lock. Keeping the proof, journal,
// disk move and File update under one lock excludes imports and removals.
func (s *Service) applyRenameLocked(ctx context.Context, plan []RenameStep, jobID int64, by domain.Actor) (int, error) {
	type row struct {
		id        int64
		st        RenameStep
		proof     preparedFileProof
		subProofs []preparedFileProof
		subs      []int64 // the step ids of its subtitles, in order
		prepErr   error   // this File failed before its journal rows were written
	}
	var rows []row
	// Hashing a large video can take time. Prepare every durable recovery
	// proof before opening the sole write transaction, then make only cheap
	// identity/metadata checks while inserting the journal rows.
	for _, st := range plan {
		if st.Skip != "" || st.To == "" {
			continue
		}
		from := filepath.Join(st.root, filepath.FromSlash(st.From))
		proof, err := prepareFileProof(from)
		if err != nil {
			rows = append(rows, row{st: st, prepErr: err})
			continue
		}
		rw := row{st: st, proof: proof}
		for _, m := range st.Subtitles {
			if m.Skip != "" || m.To == "" {
				continue
			}
			proof, err := prepareFileProof(filepath.Join(st.root, filepath.FromSlash(m.From)))
			if err != nil {
				rw.prepErr = err
				break
			}
			rw.subProofs = append(rw.subProofs, proof)
		}
		rows = append(rows, rw)
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for i := range rows {
			rw := &rows[i]
			st := rw.st
			if rw.prepErr != nil {
				continue
			}
			from := filepath.Join(st.root, filepath.FromSlash(st.From))
			if !proofSourceUnchanged(from, rw.proof) {
				rw.prepErr = fmt.Errorf("%s changed before its rename was recorded", from)
				continue
			}
			proofIndex := 0
			for _, m := range st.Subtitles {
				if m.Skip != "" || m.To == "" {
					continue
				}
				subFrom := filepath.Join(st.root, filepath.FromSlash(m.From))
				if !proofSourceUnchanged(subFrom, rw.subProofs[proofIndex]) {
					rw.prepErr = fmt.Errorf("%s changed before its rename was recorded", subFrom)
					break
				}
				proofIndex++
			}
			if rw.prepErr != nil {
				continue
			}
			res, err := tx.Exec(`INSERT INTO rename_steps (created_at, job_id, file_id, from_path, to_path, to_rel, state, error) VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`,
				database.FormatTime(s.now()), nullID(jobID), st.FileID, from,
				filepath.Join(st.root, filepath.FromSlash(st.To)), st.To, rw.proof.value)
			if err != nil {
				return err
			}
			rw.id, _ = res.LastInsertId()
			proofIndex = 0
			for _, m := range st.Subtitles {
				if m.Skip != "" || m.To == "" {
					continue
				}
				subFrom := filepath.Join(st.root, filepath.FromSlash(m.From))
				proof := rw.subProofs[proofIndex]
				proofIndex++
				res, err := tx.Exec(`INSERT INTO rename_steps (created_at, job_id, file_id, from_path, to_path, to_rel, state, part, error) VALUES (?, ?, ?, ?, ?, ?, 'pending', 'subtitle', ?)`,
					database.FormatTime(s.now()), nullID(jobID), st.FileID, subFrom,
					filepath.Join(st.root, filepath.FromSlash(m.To)), m.To, proof.value)
				if err != nil {
					return err
				}
				sid, _ := res.LastInsertId()
				rw.subs = append(rw.subs, sid)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	idx := map[int64]int{}
	for i, st := range plan {
		if st.Skip == "" && st.To != "" {
			idx[st.FileID] = i
		}
	}
	done, failed := 0, 0
	var firstErr error
	emptied := map[[2]string]bool{} // root and old path of each moved file
	for _, r := range rows {
		if r.prepErr != nil {
			plan[idx[r.st.FileID]].Result = "failed: " + r.prepErr.Error()
			failed++
			if firstErr == nil {
				firstErr = r.prepErr
			}
			continue
		}
		err := s.finishRename(ctx, r.id, r.st.FileID, filepath.Join(r.st.root, filepath.FromSlash(r.st.From)),
			filepath.Join(r.st.root, filepath.FromSlash(r.st.To)), r.st.To, by)
		if err != nil {
			plan[idx[r.st.FileID]].Result = "failed: " + err.Error()
			failed++
			if firstErr == nil {
				firstErr = err
			}
			// Its subtitles stay with it under the old name.
			s.dropSubtitleSteps(ctx, r.id, r.subs)
			continue
		}
		plan[idx[r.st.FileID]].Result = "renamed"
		done++
		emptied[[2]string{r.st.root, r.st.From}] = true
		step := &plan[idx[r.st.FileID]]
		k := 0
		for i, m := range step.Subtitles {
			if m.Skip != "" || m.To == "" {
				continue
			}
			sid := r.subs[k]
			k++
			if m.pair && step.Subtitles[i-1].Result != "renamed" {
				// The .idx of this VobSub pair did not move: the .sub stays.
				s.failStep(ctx, sid, "its other half did not move")
				step.Subtitles[i].Result = "failed: its other half did not move"
				continue
			}
			err := s.finishSubtitleRename(ctx, sid, r.st.FileID, filepath.Join(r.st.root, filepath.FromSlash(m.From)),
				filepath.Join(r.st.root, filepath.FromSlash(m.To)), m.To)
			if err != nil && m.pair {
				// Put the .idx back so the pair stays whole under the old name.
				prev := step.Subtitles[i-1]
				if moveFile(filepath.Join(r.st.root, filepath.FromSlash(prev.To)), filepath.Join(r.st.root, filepath.FromSlash(prev.From))) == nil {
					s.failStep(ctx, r.subs[k-2], "its other half did not move")
					step.Subtitles[i-1].Result = "failed: its other half did not move"
				}
			}
			if err != nil {
				step.Subtitles[i].Result = "failed: " + err.Error()
				failed++
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			step.Subtitles[i].Result = "renamed"
		}
	}
	for k := range emptied {
		removeEmptiedFolders(k[0], k[1])
	}
	if failed > 0 {
		return done, fmt.Errorf("%d of %d renames failed; the first: %w", failed, len(rows), firstErr)
	}
	return done, nil
}

// removeEmptiedFolders removes the folders a renamed file left empty, from
// its old folder up to (not including) its title's folder. A folder that
// still holds anything, or is a link, stays.
func removeEmptiedFolders(root, rel string) {
	parts := strings.Split(path.Dir(rel), "/")
	for i := len(parts); i > 1; i-- {
		d := filepath.Join(root, filepath.FromSlash(strings.Join(parts[:i], "/")))
		if fi, err := os.Lstat(d); err != nil || !fi.IsDir() || !importer.InsideReal(root, d) {
			return
		}
		if os.Remove(d) != nil {
			return
		}
	}
}

// dropSubtitleSteps marks the subtitle steps of a video that did not move
// as failed, so they are never carried out on their own. A video step
// still pending (cut off midway) keeps its subtitles pending too: the next
// run moves the video, then them.
func (s *Service) dropSubtitleSteps(ctx context.Context, videoStep int64, ids []int64) {
	var state string
	if s.Store.Read().QueryRowContext(ctx, "SELECT state FROM rename_steps WHERE id = ?", videoStep).Scan(&state) != nil || state != "failed" {
		return
	}
	for _, id := range ids {
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE rename_steps SET state = 'failed', error = 'the video did not move' WHERE id = ? AND state = 'pending'", id)
			return err
		})
	}
}

// failStep marks a rename step failed with why.
func (s *Service) failStep(ctx context.Context, id int64, why string) {
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE rename_steps SET state = 'failed', error = ? WHERE id = ?", why, id)
		return err
	})
}

// finishSubtitleRename moves one subtitle file after its video and moves
// its subtitle rows with it. Like finishRename it checks the Library
// folder again just before the move and never replaces a file.
func (s *Service) finishSubtitleRename(ctx context.Context, stepID, fileID int64, from, to, toRel string) error {
	var root string
	var proof sql.NullString
	if err := s.Store.Read().QueryRowContext(ctx, "SELECT error FROM rename_steps WHERE id = ?", stepID).Scan(&proof); err != nil {
		return err
	}
	err := s.Store.Read().QueryRowContext(ctx, `SELECT lf.path FROM files f JOIN versions v ON v.id = f.version_id
		JOIN library_folders lf ON lf.id = v.library_folder_id WHERE f.id = ?`, fileID).Scan(&root)
	switch {
	case err != nil:
		err = fmt.Errorf("the file's Library folder cannot be read: %w", err)
	case !importer.InsideReal(root, filepath.Dir(from)) || !importer.InsideReal(root, filepath.Dir(to)):
		err = errors.New("a link leads the subtitle or its new name outside the Library folder")
	default:
		err = moveRecordedFile(from, to, proof.String)
	}
	if err != nil {
		if _, terr := os.Lstat(from + caseTempSuffix); terr == nil {
			return fmt.Errorf("renaming %s: %w", filepath.Base(from), err)
		}
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err2 := tx.Exec("UPDATE rename_steps SET state = 'failed', error = ? WHERE id = ?", err.Error(), stepID)
			return err2
		})
		return fmt.Errorf("renaming %s: %w", filepath.Base(from), err)
	}
	fromRel := ""
	if r, err := filepath.Rel(root, from); err == nil {
		fromRel = filepath.ToSlash(r)
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE subtitles SET relative_path = ?, updated_at = ? WHERE file_id = ? AND relative_path = ?",
			toRel, database.FormatTime(s.now()), fileID, fromRel); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE rename_steps SET state = 'done', error = NULL WHERE id = ?", stepID)
		return err
	})
}

// finishRename moves one file and records its new name. moveFile never
// replaces a file and takes a finished earlier move as done.
func (s *Service) finishRename(ctx context.Context, stepID, fileID int64, from, to, toRel string, by domain.Actor) error {
	if renameCrash != nil {
		if err := renameCrash("before_move"); err != nil {
			return err
		}
	}
	// Checked again just before the move: a folder swapped for a link
	// since the plan must not lead the move out of the Library folder.
	var root string
	var proof sql.NullString
	if err := s.Store.Read().QueryRowContext(ctx, "SELECT error FROM rename_steps WHERE id = ?", stepID).Scan(&proof); err != nil {
		return err
	}
	err := s.Store.Read().QueryRowContext(ctx, `SELECT lf.path FROM files f JOIN versions v ON v.id = f.version_id
		JOIN library_folders lf ON lf.id = v.library_folder_id WHERE f.id = ?`, fileID).Scan(&root)
	switch {
	case err != nil:
		err = fmt.Errorf("the file's Library folder cannot be read: %w", err)
	case !importer.InsideReal(root, filepath.Dir(from)) || !importer.InsideReal(root, filepath.Dir(to)):
		err = errors.New("a link leads the file or its new name outside the Library folder")
	default:
		err = moveRecordedFile(from, to, proof.String)
	}
	if err != nil {
		if _, terr := os.Lstat(from + caseTempSuffix); terr == nil {
			// Cut off between the two renames of a case-only rename: the
			// step stays pending and the next run finishes it.
			return fmt.Errorf("renaming %s: %w", filepath.Base(from), err)
		}
		_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err2 := tx.Exec("UPDATE rename_steps SET state = 'failed', error = ? WHERE id = ?", err.Error(), stepID)
			return err2
		})
		return fmt.Errorf("renaming %s: %w", filepath.Base(from), err)
	}
	if renameCrash != nil {
		if err := renameCrash("after_move"); err != nil {
			return err
		}
	}
	var f mediafiles.File
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "files", fileID, &f); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE files SET relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?",
			toRel, database.FormatTime(s.now()), ActorText(by), fileID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE rename_steps SET state = 'done', error = NULL WHERE id = ?", stepID)
		return err
	})
	if err != nil {
		return err
	}
	var titleID int64
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT title_id FROM versions WHERE id = ?", f.VersionID).Scan(&titleID)
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventRenamed, TitleID: &titleID,
		Summary: "Renamed " + path.Base(f.RelativePath) + " to " + path.Base(toRel),
		Data:    map[string]any{"file_id": fileID, "from": f.RelativePath, "to": toRel, "slot_ids": f.SlotIDs}, GroupKey: GroupKey(titleID)})
	return nil
}

// RecoverRenames finishes the steps a cut-off rename left pending. It runs
// before every rename and at start. It returns how many it finished.
func (s *Service) RecoverRenames(ctx context.Context, by domain.Actor) (int, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	return s.recoverRenamesLocked(ctx, by)
}

// recoverRenamesLocked requires the Store file lock.
func (s *Service) recoverRenamesLocked(ctx context.Context, by domain.Actor) (int, error) {
	rows, err := s.Store.Read().QueryContext(ctx, "SELECT id, file_id, from_path, to_path, to_rel, part FROM rename_steps WHERE state = 'pending' ORDER BY id")
	if err != nil {
		return 0, err
	}
	type pending struct {
		id, fileID          int64
		from, to, rel, part string
	}
	var ps []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.fileID, &p.from, &p.to, &p.rel, &p.part); err != nil {
			rows.Close()
			return 0, err
		}
		ps = append(ps, p)
	}
	rows.Close()
	n := 0
	var firstErr error
	for _, p := range ps {
		if p.part == "subtitle" {
			// A subtitle follows only a video that moved. Its video step is
			// the File's last video step written before it; its state is
			// read from the database (a crash may have marked it failed
			// without reaching the subtitle steps). Pending video steps come
			// first in id order, so they have already run by now.
			var videoStep int64
			var state string
			var pairID int64
			var pairFrom, pairTo string
			err := s.Store.Read().QueryRowContext(ctx, `SELECT id, state FROM rename_steps
				WHERE file_id = ? AND part = 'video' AND id < ? ORDER BY id DESC LIMIT 1`, p.fileID, p.id).Scan(&videoStep, &state)
			if err == nil && state == "done" && strings.EqualFold(filepath.Ext(p.from), ".sub") {
				// The .sub of a VobSub pair moves only after its .idx did.
				var idxState string
				if s.Store.Read().QueryRowContext(ctx, `SELECT id, from_path, to_path, state FROM rename_steps WHERE file_id = ? AND part = 'subtitle' AND id < ?
					AND lower(from_path) = lower(?) ORDER BY id DESC LIMIT 1`, p.fileID, p.id,
					strings.TrimSuffix(p.from, filepath.Ext(p.from))+".idx").Scan(&pairID, &pairFrom, &pairTo, &idxState) == nil {
					state = idxState
				}
			}
			if err != nil || state != "done" {
				if err == nil && state == "pending" {
					continue // cut off midway: the next run moves the video, then this
				}
				_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
					_, err := tx.Exec("UPDATE rename_steps SET state = 'failed', error = 'the video did not move' WHERE id = ? AND state = 'pending'", p.id)
					return err
				})
				continue
			}
			if err := s.finishSubtitleRename(ctx, p.id, p.fileID, p.from, p.to, p.rel); err != nil {
				if pairID != 0 && state == "done" {
					// Recovery can be cut off after the .idx was marked done. If
					// its .sub cannot move, put the .idx back just as the immediate
					// rename path does, so the usable pair keeps one basename.
					if rerr := moveFile(pairTo, pairFrom); rerr == nil {
						s.failStep(ctx, pairID, "its other half did not move")
					} else {
						err = errors.Join(err, fmt.Errorf("putting the VobSub index back: %w", rerr))
					}
				}
				if firstErr == nil {
					firstErr = err
				}
			}
			continue
		}
		if err := s.finishRename(ctx, p.id, p.fileID, p.from, p.to, p.rel, by); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		n++
	}
	return n, firstErr
}

// Rename queues a rename Job for the given Titles (all when empty),
// limited to fileIDs when given.
func (s *Service) Rename(ctx context.Context, titleIDs, fileIDs []int64, planHash string, by domain.Actor) (int64, error) {
	ts := make([]any, 0, len(titleIDs))
	for _, id := range titleIDs {
		ts = append(ts, id)
	}
	fs := make([]any, 0, len(fileIDs))
	for _, id := range fileIDs {
		fs = append(fs, id)
	}
	key := "rename_files"
	if len(titleIDs) == 1 {
		key += ":" + strconv.FormatInt(titleIDs[0], 10)
	}
	id, _, err := s.Queue.Enqueue(ctx, KindRename, nil, map[string]any{"title_ids": ts, "file_ids": fs, "plan_hash": planHash}, key, by)
	return id, err
}

func idsParam(v any) []int64 {
	raw, _ := v.([]any)
	out := []int64{}
	for _, x := range raw {
		if f, ok := x.(float64); ok {
			out = append(out, int64(f))
		}
	}
	return out
}

// handleRename finishes any cut-off rename, then plans again with the same
// code the preview used and carries the plan out.
func (s *Service) handleRename(ctx context.Context, r *jobs.Run) error {
	by := JobActor(r)
	unlock := s.Store.LockFiles()
	defer unlock()
	// A recovery that fails marks its steps failed; it never blocks this run.
	_, recoverErr := s.recoverRenamesLocked(ctx, by)
	plan, hash, err := PlanRenameHashed(ctx, s.Store.Read(), idsParam(r.Job.Params["title_ids"]))
	if err != nil {
		return err
	}
	if want, _ := r.Job.Params["plan_hash"].(string); want != hash {
		return jobError{code: "plan_changed", msg: "The plan changed since the preview: preview again.", final: true}
	}
	if only := idsParam(r.Job.Params["file_ids"]); len(only) > 0 {
		keep := map[int64]bool{}
		for _, id := range only {
			keep[id] = true
		}
		var sub []RenameStep
		for _, st := range plan {
			if keep[st.FileID] {
				sub = append(sub, st)
			}
		}
		plan = sub
	}
	n, err := s.applyRenameLocked(ctx, plan, r.Job.ID, by)
	res := map[string]any{"renamed": n, "steps": plan}
	if recoverErr != nil {
		res["recovery_error"] = recoverErr.Error()
	}
	r.SetResult(res)
	return err
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

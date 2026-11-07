package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/judge"
	"github.com/HolyArrs/holyarrs/app/backend/internal/naming"
	"github.com/HolyArrs/holyarrs/app/backend/internal/parse"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// KindRename is the Job that renames Files to the naming pattern.
const KindRename = "rename_files"

// reader reads rows and single rows (the store's read pool fits).
type reader interface {
	store.Lister
	store.Querier
}

// RenameStep is one File in a rename. From and To are relative to the
// Library folder. A step with Skip set is shown but never carried out.
type RenameStep struct {
	FileID          int64  `json:"file_id"`
	TitleID         int64  `json:"title_id"`
	TitleName       string `json:"title_name"`
	VersionID       int64  `json:"version_id"`
	LibraryFolderID int64  `json:"library_folder_id"`
	Origin          string `json:"origin"`
	From            string `json:"from"`
	To              string `json:"to,omitempty"`
	Skip            string `json:"skip,omitempty"`
	// AlbumID is the album of a track file, so an album page can show its
	// own part of the artist's rename.
	AlbumID int64 `json:"album_id,omitempty"`
	// Result is what the rename did with this step: renamed, or failed
	// and why.
	Result string `json:"result,omitempty"`
	// Subtitles are the subtitle files beside the video
	// (name.lang[.hi|.forced].ext); they follow it to its new name.
	Subtitles []SubtitleMove `json:"subtitles,omitempty"`
	root      string
}

// SubtitleMove is one subtitle file that follows its video in a rename.
// From and To are relative to the Library folder; a move with Skip set
// is shown but never carried out (the video still moves).
type SubtitleMove struct {
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
	Skip   string `json:"skip,omitempty"`
	Result string `json:"result,omitempty"`
	// pair marks the .sub of a VobSub pair: it goes with the .idx just
	// before it, or not at all.
	pair bool
}

// besideSubtitles lists the subtitle files next to a video (relative to
// the Library folder), in name order, as groups that move together. Only
// plain files count. A picture (VobSub) .sub is a pair with the .idx of
// the same name, listed .idx first; a .sub with no .idx goes alone only
// when it is clearly text, else it is left where it is.
func besideSubtitles(root, rel string) [][]string {
	dir := filepath.Join(root, filepath.FromSlash(path.Dir(rel)))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	base := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	idx := map[string]string{} // lower-case stem of each .idx: its name
	for _, e := range ents {
		if n := e.Name(); e.Type().IsRegular() && strings.EqualFold(path.Ext(n), ".idx") {
			idx[strings.ToLower(strings.TrimSuffix(n, path.Ext(n)))] = n
		}
	}
	var out [][]string
	for _, e := range ents {
		n := e.Name()
		if !e.Type().IsRegular() || !subtitles.IsBeside(base, n) {
			continue
		}
		one := path.Join(path.Dir(rel), n)
		if strings.EqualFold(path.Ext(n), ".sub") {
			if partner, ok := idx[strings.ToLower(strings.TrimSuffix(n, path.Ext(n)))]; ok {
				out = append(out, []string{path.Join(path.Dir(rel), partner), one})
				continue
			}
			if !textSub(filepath.Join(dir, n)) {
				continue
			}
		}
		out = append(out, []string{one})
	}
	return out
}

// textSub reads the head of a .sub file and says whether it is clearly text.
func textSub(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := io.ReadFull(f, head)
	return subtext.TextSub(head[:n])
}

// planSubtitles works out where each subtitle beside a renamed video goes:
// the video's new name, then the same language and tags. The files of a
// pair are skipped together when either cannot move.
func planSubtitles(root, from, to string, taken map[string]bool) []SubtitleMove {
	oldBase := strings.TrimSuffix(path.Base(from), path.Ext(from))
	newBase := strings.TrimSuffix(path.Base(to), path.Ext(to))
	var out []SubtitleMove
	for _, group := range besideSubtitles(root, from) {
		ms := make([]SubtitleMove, len(group))
		skip := ""
		for i, sub := range group {
			ms[i] = SubtitleMove{From: sub, pair: i > 0}
			nt := path.Join(path.Dir(to), newBase+path.Base(sub)[len(oldBase):])
			fromAbs := filepath.Join(root, filepath.FromSlash(sub))
			toAbs := filepath.Join(root, filepath.FromSlash(nt))
			switch {
			case !filepath.IsLocal(filepath.FromSlash(nt)):
				ms[i].Skip = "the name would leave the Library folder"
			case taken[strings.ToLower(nt)]:
				ms[i].Skip = "another file in this rename gets the same name"
			case importer.PathTooLong(toAbs) != "":
				ms[i].Skip = importer.PathTooLong(toAbs)
			default:
				if other, err := os.Stat(toAbs); err == nil {
					if self, _ := os.Stat(fromAbs); !os.SameFile(self, other) {
						ms[i].Skip = "a file named " + path.Base(nt) + " is already there"
					}
				}
			}
			ms[i].To = nt
			if skip == "" {
				skip = ms[i].Skip
			}
		}
		for i := range ms {
			if skip != "" {
				if ms[i].Skip == "" {
					ms[i].Skip = "its other half cannot move: " + skip
				}
				ms[i].To = ""
				continue
			}
			taken[strings.ToLower(ms[i].To)] = true
		}
		out = append(out, ms...)
	}
	return out
}

// renameCrash lets tests stop a rename at a stage ("before_move",
// "after_move") as if the process died there.
var renameCrash func(stage string) error

// fileFacts are the naming facts for a File already in the Library: the
// same ones its import used (the release name and its quality step).
func fileFacts(t domain.Title, f domain.File) naming.Facts {
	release := ""
	if f.ReleaseName != nil {
		release = *f.ReleaseName
	}
	year := 0
	if t.Year != nil {
		year = *t.Year
	}
	ext := path.Ext(f.RelativePath)
	fa := naming.FactsFrom(t.Name, year, t.SourceIDs, parse.Parse(release, parse.Hints{}), release, ext)
	if edition, ok := f.MediaInfo["edition"].(string); ok {
		fa.Edition = edition
	}
	if step, ok := f.MediaInfo["step"].(string); ok && step != "" {
		fa.Step = step
	}
	return fa
}

// fileEpisodes lists the episodes a File holds, in episode order.
func fileEpisodes(ctx context.Context, db reader, f domain.File) ([]importer.Episode, error) {
	var out []importer.Episode
	for _, sid := range f.SlotIDs {
		var it domain.Item
		var itemID int64
		if err := db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", sid).Scan(&itemID); err != nil {
			continue
		}
		if err := store.Get(ctx, db, "items", itemID, &it); err != nil {
			return nil, err
		}
		e := importer.Episode{Ref: judge.EpisodeRef{ItemID: it.ID}, SlotID: sid}
		if it.Numbers.Season != nil {
			e.Ref.Season = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			e.Ref.Episode = *it.Numbers.Episode
		}
		if it.Numbers.Absolute != nil {
			e.Ref.Absolute = *it.Numbers.Absolute
		}
		if it.AirDay != nil {
			e.Ref.AirDay = *it.AirDay
		}
		if it.Name != nil {
			e.Name = *it.Name
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref.Season != out[j].Ref.Season {
			return out[i].Ref.Season < out[j].Ref.Season
		}
		return out[i].Ref.Episode < out[j].Ref.Episode
	})
	return out, nil
}

// episodeFacts are the naming facts of an episode File in the Library.
func episodeFacts(t domain.Title, f domain.File, eps []importer.Episode) naming.EpisodeFacts {
	release := ""
	if f.ReleaseName != nil {
		release = *f.ReleaseName
	}
	year := 0
	if t.Year != nil {
		year = *t.Year
	}
	order := naming.OrderAired
	if t.EpisodeOrder != nil && *t.EpisodeOrder != "" {
		order = *t.EpisodeOrder
	}
	fa := importer.EpisodeFactsFor(t.Name, year, t.SourceIDs, order, parse.Parse(release, parse.Hints{Show: true}), release, path.Ext(f.RelativePath), eps)
	if step, ok := f.MediaInfo["step"].(string); ok && step != "" {
		fa.Step = step
	}
	return fa
}

// albumFacts are an album's naming facts and its disc count.
type albumFacts struct {
	f     naming.TrackFacts
	discs int
}

// trackFacts are the naming facts of a track File in the Library: its
// track (the first Slot's), album (and its id) and artist. ok is false for
// a File that holds no track.
func trackFacts(ctx context.Context, db reader, t domain.Title, f domain.File, cache map[int64]albumFacts) (naming.TrackFacts, int64, bool, error) {
	if len(f.SlotIDs) == 0 {
		return naming.TrackFacts{}, 0, false, nil
	}
	var it domain.Item
	var itemID int64
	if err := db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", f.SlotIDs[0]).Scan(&itemID); err != nil {
		return naming.TrackFacts{}, 0, false, nil
	}
	if err := store.Get(ctx, db, "items", itemID, &it); err != nil {
		return naming.TrackFacts{}, 0, false, err
	}
	if it.Kind != "track" || it.PartID == nil {
		return naming.TrackFacts{}, 0, false, nil
	}
	af, ok := cache[*it.PartID]
	if !ok {
		var err error
		af.f, af.discs, err = importer.AlbumFacts(ctx, db, t.Name, *it.PartID)
		if err != nil {
			return naming.TrackFacts{}, 0, false, err
		}
		cache[*it.PartID] = af
	}
	tf := af.f
	tf.Discs, tf.Disc = af.discs, 1
	if it.Numbers.Disc != nil && *it.Numbers.Disc > 0 {
		tf.Disc = *it.Numbers.Disc
	}
	if it.Numbers.Track != nil {
		tf.Track = *it.Numbers.Track
	}
	if it.Name != nil {
		tf.Title = *it.Name
	}
	tf.Ext = path.Ext(f.RelativePath)
	return tf, *it.PartID, true, nil
}

// PlanRename works out the new name of every main File of the given Titles
// (every Title when titleIDs is empty) under the naming settings. Files that
// would keep their name are left out. Nothing on disk changes.
func PlanRename(ctx context.Context, db reader, titleIDs []int64) ([]RenameStep, error) {
	ns, err := naming.Read(ctx, db)
	if err != nil {
		return nil, err
	}
	return planRename(ctx, db, ns, titleIDs)
}

// PlanRenameHashed is PlanRename with the plan hash a rename must be
// asked with, so it does only what this preview showed.
func PlanRenameHashed(ctx context.Context, db reader, titleIDs []int64) ([]RenameStep, string, error) {
	ns, err := naming.Read(ctx, db)
	if err != nil {
		return nil, "", err
	}
	plan, err := planRename(ctx, db, ns, titleIDs)
	if err != nil {
		return nil, "", err
	}
	return plan, planHash(plan, ns), nil
}

// planHash is the SHA-256 of every step's file, old and new name and skip,
// and of the naming settings.
func planHash(plan []RenameStep, ns naming.Settings) string {
	h := sha256.New()
	ns.AdoptScanLimit, ns.MusicAdoptScanLimit = 0, 0 // not naming choices
	set, _ := json.Marshal(ns)
	h.Write(set)
	for _, st := range plan {
		fmt.Fprintf(h, "\n%d\x00%s\x00%s\x00%s", st.FileID, st.From, st.To, st.Skip)
		for _, m := range st.Subtitles {
			fmt.Fprintf(h, "\n\x01%s\x00%s\x00%s", m.From, m.To, m.Skip)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func planRename(ctx context.Context, db reader, ns naming.Settings, titleIDs []int64) ([]RenameStep, error) {
	where, args := "state != 'removed' ORDER BY sort_name, id", []any{}
	if len(titleIDs) > 0 {
		where = "state != 'removed' AND id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(titleIDs)), ",") + ") ORDER BY sort_name, id"
		for _, id := range titleIDs {
			args = append(args, id)
		}
	}
	titles, err := store.Select[domain.Title](ctx, db, "titles", where, args...)
	if err != nil {
		return nil, err
	}
	out := []RenameStep{}
	for _, t := range titles {
		files, err := store.Select[domain.File](ctx, db, "files",
			"state = 'present' AND role = 'main' AND version_id IN (SELECT id FROM versions WHERE title_id = ?) ORDER BY id", t.ID)
		if err != nil {
			return nil, err
		}
		taken := map[string]bool{}
		albums := map[int64]albumFacts{}
		for _, f := range files {
			var v domain.Version
			var lf domain.LibraryFolder
			if err := store.Get(ctx, db, "versions", f.VersionID, &v); err != nil {
				return nil, err
			}
			if err := store.Get(ctx, db, "library_folders", v.LibraryFolderID, &lf); err != nil {
				return nil, err
			}
			st := RenameStep{FileID: f.ID, TitleID: t.ID, TitleName: t.Name, VersionID: v.ID, LibraryFolderID: lf.ID, Origin: f.Origin,
				From: f.RelativePath, root: lf.Path}
			facts := fileFacts(t, f)
			to := path.Join(path.Dir(f.RelativePath), importer.EditionFileName(naming.File(ns, facts), facts.Edition))
			if path.Dir(f.RelativePath) == "." {
				to = path.Join(v.FolderName, importer.EditionFileName(naming.File(ns, facts), facts.Edition))
			}
			if t.Kind == "show" {
				// An episode file: show folder, season folder, episode name,
				// the same function the import uses.
				eps, err := fileEpisodes(ctx, db, f)
				if err != nil {
					return nil, err
				}
				if len(eps) == 0 {
					continue
				}
				to = naming.EpisodePath(ns, v.FolderName, episodeFacts(t, f, eps))
			}
			if t.Kind == "artist" {
				// A track file: artist folder, album folder, disc folder,
				// track name, the same function the album import uses.
				tf, partID, ok, err := trackFacts(ctx, db, t, f, albums)
				if err != nil {
					return nil, err
				}
				if !ok {
					continue
				}
				to = naming.TrackPath(ns, v.FolderName, tf)
				st.AlbumID = partID
			}
			fromAbs := filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath))
			toAbs := filepath.Join(lf.Path, filepath.FromSlash(to))
			switch {
			case to == f.RelativePath:
				continue
			case f.Origin != "imported" && f.Origin != "adopted":
				st.Skip = "found on disk, not imported or adopted; HolyArrs never renames it"
			case !filepath.IsLocal(filepath.FromSlash(to)) || !filepath.IsLocal(filepath.FromSlash(f.RelativePath)):
				st.Skip = "the name would leave the Library folder"
			case taken[strings.ToLower(to)]:
				st.Skip = "another file in this rename gets the same name"
			case importer.PathTooLong(toAbs) != "":
				st.Skip = importer.PathTooLong(toAbs)
			case !importer.InsideReal(lf.Path, filepath.Dir(fromAbs)) || !importer.InsideReal(lf.Path, filepath.Dir(toAbs)):
				st.Skip = "a link leads the file or its new name outside the Library folder"
			default:
				if _, err := os.Stat(lf.Path); err != nil {
					st.Skip = "the Library folder cannot be reached"
				} else if _, err := os.Stat(fromAbs); err != nil {
					st.Skip = "the file is not on disk"
				} else if other, err := os.Stat(toAbs); err == nil {
					if self, _ := os.Stat(fromAbs); !os.SameFile(self, other) {
						st.Skip = "a file named " + path.Base(to) + " is already there"
					}
				}
			}
			if st.Skip == "" {
				st.To = to
				taken[strings.ToLower(to)] = true
				if t.Kind != "artist" { // music has no subtitles
					st.Subtitles = planSubtitles(lf.Path, f.RelativePath, to, taken)
				}
			}
			out = append(out, st)
		}
	}
	return out, nil
}

// ApplyRename carries out the steps of a plan that are not skipped. Each
// step is written to rename_steps before the disk changes, so a run cut off
// halfway is finished by RecoverRenames. It returns how many Files moved.
func (s *Service) ApplyRename(ctx context.Context, plan []RenameStep, jobID int64, by domain.Actor) (int, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	return s.applyRenameLocked(ctx, plan, jobID, by)
}

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
				store.FormatTime(s.now()), nullID(jobID), st.FileID, from,
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
					store.FormatTime(s.now()), nullID(jobID), st.FileID, subFrom,
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
			toRel, store.FormatTime(s.now()), fileID, fromRel); err != nil {
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
	var f domain.File
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := store.Get(ctx, tx, "files", fileID, &f); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE files SET relative_path = ?, updated_at = ?, updated_by = ? WHERE id = ?",
			toRel, store.FormatTime(s.now()), actorText(by), fileID); err != nil {
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
	s.event(ctx, &domain.Event{Actor: by, Kind: domain.EventRenamed, TitleID: &titleID,
		Summary: "Renamed " + path.Base(f.RelativePath) + " to " + path.Base(toRel),
		Data:    map[string]any{"file_id": fileID, "from": f.RelativePath, "to": toRel, "slot_ids": f.SlotIDs}, GroupKey: groupKey(titleID)})
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
	by := jobActor(r)
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

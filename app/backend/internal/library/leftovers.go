package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// KindRescanTitle refreshes one title's episode list, then rescans only its
// folder.
const KindRescanTitle = "rescan_title"

// LeftoverView is one leftover file of a title: a video in its folder that
// fills no listed episode. Reason is a short key the app translates.
type LeftoverView struct {
	FileID        int64          `json:"file_id"`
	RelativePath  string         `json:"relative_path"`
	Size          int64          `json:"size"`
	Resolution    int            `json:"resolution,omitempty"`
	Reason        string         `json:"reason"`
	DroppedSeason *int           `json:"dropped_season,omitempty"`
	Guess         *LeftoverGuess `json:"guess,omitempty"`
	Dismissed     bool           `json:"dismissed"`
}

// LeftoverGuess is the season and episodes the file name gives: only a
// hint for matching by hand.
type LeftoverGuess struct {
	Season   int   `json:"season"`
	Episodes []int `json:"episodes"`
}

// Leftovers lists a title's leftover files.
func (s *Service) Leftovers(ctx context.Context, titleID int64) ([]LeftoverView, error) {
	ls, err := mediafiles.TitleLeftovers(ctx, s.Store.Read(), titleID)
	if err != nil {
		return nil, err
	}
	out := make([]LeftoverView, 0, len(ls))
	for _, l := range ls {
		f := l.File
		v := LeftoverView{FileID: f.ID, RelativePath: f.RelativePath, Size: f.Size, Resolution: f.Quality.Resolution, Dismissed: f.LeftoverDismissed, Reason: "episode_not_listed"}
		if f.LeftoverReason != nil && *f.LeftoverReason != "" {
			v.Reason = *f.LeftoverReason
		}
		if f.Origin != "found" {
			v.Reason = "episode_not_listed"
			if l.DroppedSeason >= 0 {
				n := l.DroppedSeason
				v.DroppedSeason = &n
			}
		}
		p := releasename.Parse(strings.TrimSuffix(path.Base(f.RelativePath), path.Ext(f.RelativePath)), releasename.Hints{Size: f.Size, Show: true})
		if v.Resolution == 0 {
			v.Resolution = p.Resolution.Value
		}
		if p.Show != nil && len(p.Show.Seasons) == 1 && len(p.Show.Episodes) > 0 {
			v.Guess = &LeftoverGuess{Season: p.Show.Seasons[0], Episodes: append([]int{}, p.Show.Episodes...)}
		}
		out = append(out, v)
	}
	return out, nil
}

// LeftoverMatch links one leftover to episodes (none for a movie).
type LeftoverMatch struct {
	FileID     int64   `json:"file_id"`
	EpisodeIDs []int64 `json:"episode_ids"`
}

// MatchOutcome says what happened to one LeftoverMatch.
type MatchOutcome struct {
	FileID  int64  `json:"file_id"`
	Linked  bool   `json:"linked"`
	Refused string `json:"refused,omitempty"`
}

// errRefused stops one item's links; Refused carries the reason key.
type errRefused struct{ key string }

func (e errRefused) Error() string { return e.key }

// MatchLeftovers links leftovers to the episodes the owner picked, where the
// files lie: nothing moves on disk. An episode another file fills is refused
// unless replace is set; then that file becomes a found note.
func (s *Service) MatchLeftovers(ctx context.Context, titleID int64, items []LeftoverMatch, replace bool, by domain.Actor) ([]MatchOutcome, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	var t mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil {
		return nil, err
	}
	ls, err := mediafiles.TitleLeftovers(ctx, s.Store.Read(), titleID)
	if err != nil {
		return nil, err
	}
	left := map[int64]mediafiles.File{}
	for _, l := range ls {
		left[l.File.ID] = l.File
	}
	out := make([]MatchOutcome, len(items))
	for i, it := range items {
		out[i].FileID = it.FileID
		if _, ok := left[it.FileID]; !ok {
			out[i].Refused = "not_a_leftover"
		}
	}
	if t.Kind == "movie" {
		for i, it := range items {
			if out[i].Refused == "" {
				out[i].Refused = s.matchMovie(ctx, t, left[it.FileID], replace, by)
				out[i].Linked = out[i].Refused == ""
			}
		}
	} else {
		err = s.Store.Write(ctx, func(tx *sql.Tx) error {
			for i, it := range items {
				if out[i].Refused != "" {
					continue
				}
				err := s.matchEpisodesTx(ctx, tx, left[it.FileID], it.EpisodeIDs, replace, by)
				var r errRefused
				if errors.As(err, &r) {
					out[i].Refused = r.key
					continue
				}
				if err != nil {
					return err
				}
				out[i].Linked = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	n := 0
	for _, o := range out {
		if o.Linked {
			n++
		}
	}
	if n > 0 {
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAdopted, TitleID: &titleID, Summary: fmt.Sprintf("%s: %d files matched by hand", t.Name, n),
			Data: map[string]any{"matched": n, "by_hand": true}, GroupKey: GroupKey(titleID)})
	}
	return out, nil
}

func (s *Service) matchEpisodesTx(ctx context.Context, tx *sql.Tx, f mediafiles.File, episodeIDs []int64, replace bool, by domain.Actor) error {
	if len(episodeIDs) == 0 || len(episodeIDs) > 20 {
		return errRefused{"no_episode"}
	}
	var v mediafiles.Version
	if err := database.Get(ctx, tx, "versions", f.VersionID, &v); err != nil {
		return err
	}
	var slotIDs []int64
	for _, eid := range episodeIDs {
		es, err := mediafiles.EpisodeSlotTx(ctx, tx, v.ID, eid)
		if errors.Is(err, mediafiles.ErrNotThisVersion) {
			return errRefused{"wrong_title"}
		}
		if err != nil {
			return err
		}
		sl := es.Slot
		if es.Dropped {
			return errRefused{"episode_dropped"}
		}
		switch sl.State {
		case mediafiles.SlotRemoved, mediafiles.SlotChosen, mediafiles.SlotDownloading, mediafiles.SlotImporting:
			return errRefused{"episode_busy"}
		}
		if sl.FileID == nil && sl.ActiveDownloadID != nil && s.DownloadRunning != nil {
			running, err := s.DownloadRunning(tx, *sl.ActiveDownloadID)
			if err != nil {
				return err
			}
			if running {
				return errRefused{"episode_busy"}
			}
			if err := mediafiles.ClearSlotDownloadTx(tx, sl.ID); err != nil {
				return err
			}
		}
		if sl.FileID != nil && *sl.FileID != f.ID {
			if !replace {
				return errRefused{"episode_has_file"}
			}
			if err := mediafiles.UnlinkFileTx(ctx, tx, *sl.FileID, "replaced_by_hand", s.now(), by); err != nil {
				return err
			}
		}
		slotIDs = append(slotIDs, sl.ID)
	}
	var pr mediafiles.PickRules
	if err := database.Get(ctx, tx, "pick_rules", v.PickRulesID, &pr); err != nil {
		return err
	}
	release := strings.TrimSuffix(path.Base(f.RelativePath), path.Ext(f.RelativePath))
	if f.ReleaseName != nil && *f.ReleaseName != "" {
		release = *f.ReleaseName
	}
	ge := mediafiles.OwnedReachesGoodEnough(release, f.MediaInfo, pr.Body)
	state := mediafiles.SlotInLibrary
	if !ge && v.KeepUpToDate {
		state = mediafiles.SlotBetterWanted
	}
	return mediafiles.LinkFileInPlaceTx(ctx, tx, f.ID, slotIDs, ge, state, "matched by hand", HoldingSlots, s.now(), by)
}

// matchMovie links a movie's found file; with replace, the copy the
// Version has becomes a found note first.
func (s *Service) matchMovie(ctx context.Context, t mediafiles.Title, f mediafiles.File, replace bool, by domain.Actor) string {
	if replace {
		if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			return mediafiles.UnlinkVersionFilesTx(ctx, tx, f.VersionID, f.ID, "replaced_by_hand", s.now(), by)
		}); err != nil {
			return "failed"
		}
	}
	_, err := s.adoptMovieCopy(ctx, t.ID, f.VersionID, CopyAdoption{RelativePath: f.RelativePath}, true, by)
	switch {
	case err == nil:
		return ""
	case strings.Contains(err.Error(), "already has a copy"):
		return "episode_has_file"
	case strings.Contains(err.Error(), "being downloaded"):
		return "episode_busy"
	case errors.Is(err, ErrCopyChoice):
		return "movie_copy_choice"
	}
	return "failed"
}

// UnmatchLeftovers turns files of the title back into found notes: the
// undo of a wrong match by hand. Nothing on disk changes.
func (s *Service) UnmatchLeftovers(ctx context.Context, titleID int64, fileIDs []int64, by domain.Actor) (int, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	n := 0
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range fileIDs {
			f, _, err := ownedFile(ctx, tx, titleID, id)
			if err != nil || f.State != "present" || f.Role != "main" || f.Origin == "found" {
				continue
			}
			if err := mediafiles.UnlinkFileTx(ctx, tx, id, "unmatched", s.now(), by); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// DismissLeftover marks a leftover as left alone, or brings it back.
func (s *Service) DismissLeftover(ctx context.Context, titleID, fileID int64, dismissed bool) error {
	if _, _, err := ownedFile(ctx, s.Store.Read(), titleID, fileID); err != nil {
		return err
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error { return mediafiles.SetLeftoverDismissed(ctx, tx, fileID, dismissed) })
}

// RescanTitle rescans only one title's files in each of its Library
// folders, with the same guards as a whole rescan.
func (s *Service) RescanTitle(ctx context.Context, titleID int64, by domain.Actor) (RescanResult, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	var res RescanResult
	vs, err := database.Select[mediafiles.Version](ctx, s.Store.Read(), "versions", "title_id = ? ORDER BY id", titleID)
	if err != nil {
		return res, err
	}
	backfill := backfillPerRescan
	seen := map[int64]bool{}
	for _, v := range vs {
		if seen[v.LibraryFolderID] {
			continue
		}
		seen[v.LibraryFolderID] = true
		var lf mediafiles.LibraryFolder
		if err := database.Get(ctx, s.Store.Read(), "library_folders", v.LibraryFolderID, &lf); err != nil {
			return res, err
		}
		res.Folders++
		if err := s.rescanFolder(ctx, lf, titleID, &res, &backfill, by); err != nil {
			return res, err
		}
	}
	return res, nil
}

// handleRescanTitle refreshes a show's episode list first, so a renumbered
// season shows up as leftovers, then rescans the title's folder.
func (s *Service) handleRescanTitle(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "title" {
		return errors.New("rescan_title needs a title subject")
	}
	id := r.Job.Subject.ID
	var t mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", id, &t); err != nil {
		return err
	}
	if t.Kind == "show" {
		if _, err := s.RefreshTitle(ctx, id, JobActor(r), &r.Job.ID); err != nil {
			return fmt.Errorf("refresh: %w", err)
		}
	}
	res, err := s.RescanTitle(ctx, id, JobActor(r))
	left := 0
	if ls, lerr := s.Leftovers(ctx, id); lerr == nil {
		for _, l := range ls {
			if !l.Dismissed {
				left++
			}
		}
	}
	r.SetResult(map[string]any{"missing": res.Missing, "back": res.Back, "found": res.Found, "unreachable": res.Unreachable, "leftovers": left})
	return err
}

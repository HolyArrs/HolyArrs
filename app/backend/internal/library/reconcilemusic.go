package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// reconcileFoundTracks links the found audio files in an artist's
// folder to the empty tracks they hold, the way adopting the folder would
// have: by the files' MusicBrainz ids, then album tag with disc and track
// number, then the album folder and file name. Files never move. Only this
// artist is read; a file already linked, protected or unlinked by hand is
// left alone, so a second run changes nothing.
//
// A file that fills no track keeps the reason (a "music_" key), so the
// artist page lists it and the next scan tries it again. A file whose tags
// belong to another artist of the Library moves to that artist when both
// sit in this Library folder (cross is false for that artist's own run, so
// a file is passed on once); nothing moves on disk.
func (s *Service) reconcileFoundTracks(ctx context.Context, lf mediafiles.LibraryFolder, v mediafiles.Version, by domain.Actor) (int, error) {
	return s.reconcileFoundTracksOf(ctx, lf, v, true, by)
}

func (s *Service) reconcileFoundTracksOf(ctx context.Context, lf mediafiles.LibraryFolder, v mediafiles.Version, cross bool, by domain.Actor) (int, error) {
	db := s.Store.Read()
	found, err := database.Select[mediafiles.File](ctx, db, "files",
		`version_id = ? AND origin = 'found' AND state = 'present' AND role = 'main' AND protected = 0 AND (leftover_reason IS NULL OR substr(leftover_reason, 1, 6) = 'music_')
		AND leftover_dismissed = 0 AND slot_ids = '[]'`, v.ID)
	if err != nil || len(found) == 0 {
		return 0, err
	}
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", v.TitleID, &t); err != nil {
		return 0, err
	}
	if t.Kind != "artist" || t.State == "removed" {
		return 0, nil
	}
	files, byPath, err := foundMusicFiles(ctx, db, lf, v, t, found)
	if err != nil {
		return 0, err
	}
	// Biggest first, as adoption does, so of two copies the fuller one wins.
	sort.SliceStable(files, func(i, j int) bool { return files[i].Size > files[j].Size })
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return 0, err
	}
	if err := importer.MatchFoundMusic(ctx, db, t, v.ID, ns.MusicReadTags, files); err != nil {
		return 0, err
	}
	var pr mediafiles.PickRules
	if err := database.Get(ctx, db, "pick_rules", v.PickRulesID, &pr); err != nil {
		return 0, err
	}
	// Files whose tags name another artist of the Library, in this folder or not.
	passOn := map[string]int64{}
	if cross {
		var owners *importer.MusicOwners
		for _, ff := range files {
			if ff.Reason != importer.FoundNoAlbum || ff.Tags.Empty() {
				continue
			}
			if owners == nil {
				if owners, err = importer.NewMusicOwners(ctx, db); err != nil {
					return 0, err
				}
			}
			to, err := owners.Owner(ctx, t.ID, ff.Tags, ff.AlbumFolder, ff.AlbumYear)
			if err != nil {
				return 0, err
			}
			if to != 0 {
				passOn[ff.Path] = to
			}
		}
	}
	linked, unmatched, newly := 0, 0, 0
	targets := map[int64]mediafiles.Version{}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, ff := range files {
			if ff.SlotID == 0 {
				rel, _ := filepath.Rel(lf.Path, ff.Path)
				f := byPath[filepath.ToSlash(rel)]
				if f.ID == 0 {
					continue
				}
				reason := ff.Reason
				if to := passOn[ff.Path]; to != 0 {
					var tv mediafiles.Version
					err := tx.QueryRowContext(ctx, `SELECT id, folder_name FROM versions WHERE title_id = ? AND library_folder_id = ? ORDER BY id LIMIT 1`, to, lf.ID).Scan(&tv.ID, &tv.FolderName)
					switch {
					case err == nil:
						tv.TitleID = to
						if _, err := tx.ExecContext(ctx, `UPDATE files SET version_id = ?, leftover_reason = NULL, updated_at = ?, updated_by = ? WHERE id = ?`,
							tv.ID, database.FormatTime(s.now()), ActorText(by), f.ID); err != nil {
							return err
						}
						targets[tv.ID] = tv
						continue
					case errors.Is(err, sql.ErrNoRows):
						reason = importer.FoundOtherArtist
					default:
						return err
					}
				}
				if reason == "" {
					continue
				}
				unmatched++
				if f.LeftoverReason == nil {
					newly++
				}
				if err := mediafiles.SetLeftoverReason(ctx, tx, f.ID, reason); err != nil {
					return err
				}
				continue
			}
			rel, _ := filepath.Rel(lf.Path, ff.Path)
			f := byPath[filepath.ToSlash(rel)]
			// The track may have been filled since it was read.
			free, err := mediafiles.SlotStillEmptyTx(ctx, tx, ff.SlotID, v.ID)
			if err != nil {
				return err
			}
			if !free || f.ID == 0 {
				continue
			}
			dir := path.Base(path.Dir(f.RelativePath))
			release := dir + " [" + adoptedFormat(ff.Path, strings.ToLower(path.Ext(f.RelativePath))) + "]"
			if err := mediafiles.SetReleaseNameTx(ctx, tx, f.ID, release); err != nil {
				return err
			}
			ge := mediafiles.ReachesGoodEnough(release, pr.Body)
			state, why := mediafiles.SlotInLibrary, "found in "+v.FolderName+" by a library scan"
			if !ge && v.KeepUpToDate && t.KeepUpToDate {
				state, why = mediafiles.SlotBetterWanted, "found below good enough, so a better copy is sought"
			}
			if err := mediafiles.LinkFileInPlaceTx(ctx, tx, f.ID, []int64{ff.SlotID}, ge, state, why, "file_id IS NOT NULL", s.now(), by); err != nil {
				return err
			}
			linked++
		}
		return nil
	})
	if err != nil {
		return linked, err
	}
	id := t.ID
	if linked > 0 {
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAdopted, TitleID: &id,
			Summary:  fmt.Sprintf("Matched %d found files of %s to their tracks; no file was moved", linked, t.Name),
			Data:     map[string]any{"library_folder_id": lf.ID, "folder": v.FolderName, "files": linked},
			GroupKey: GroupKey(id)})
	}
	// Told once per file: a later scan that finds the same ones says nothing.
	if newly > 0 {
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventFolderScanned, TitleID: &id,
			Summary:  fmt.Sprintf("%s: %d found files fit no track; they are listed under Unmatched files", t.Name, unmatched),
			Data:     map[string]any{"library_folder_id": lf.ID, "folder": v.FolderName, "unmatched": unmatched, "new": newly},
			GroupKey: GroupKey(id)})
	}
	ids := make([]int64, 0, len(targets))
	for vid := range targets {
		ids = append(ids, vid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, vid := range ids {
		var tv mediafiles.Version
		if err := database.Get(ctx, s.Store.Read(), "versions", vid, &tv); err != nil {
			return linked, err
		}
		if _, err := s.reconcileFoundTracksOf(ctx, lf, tv, false, by); err != nil {
			return linked, err
		}
	}
	return linked, nil
}

// foundMusicFiles readies an artist's found files for MatchFoundMusic: each
// with its album folder's name and the album that folder was matched to.
func foundMusicFiles(ctx context.Context, db *sql.DB, lf mediafiles.LibraryFolder, v mediafiles.Version, t mediafiles.Title, found []mediafiles.File) ([]importer.FoundMusicFile, map[string]mediafiles.File, error) {
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL ORDER BY id", t.ID)
	if err != nil {
		return nil, nil, err
	}
	refs := make([]albumRef, len(parts))
	for i, p := range parts {
		refs[i] = albumRef{ID: p.SourceIDs["musicbrainz"], Name: p.Name, PartID: p.ID, Included: p.Included}
		if p.ReleaseDate != nil {
			refs[i].Year = p.ReleaseDate.Year()
		}
	}
	// The album folders of the found files, matched to albums by name.
	vids := make([]AdoptVideo, 0, len(found))
	for _, f := range found {
		if strings.HasPrefix(f.RelativePath, v.FolderName+"/") {
			vids = append(vids, AdoptVideo{Path: f.RelativePath, Size: f.Size})
		}
	}
	dirs, _ := albumDirs(v.FolderName, vids)
	got, _, _ := matchAlbums(dirs, t.Name, refs)
	partOf := map[string]int64{}
	for i, d := range dirs {
		if got[i] != nil {
			partOf[d.name] = got[i].PartID
		}
	}
	byPath := map[string]mediafiles.File{}
	files := make([]importer.FoundMusicFile, 0, len(found))
	for _, f := range found {
		byPath[f.RelativePath] = f
		ff := importer.FoundMusicFile{Path: filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath)), Size: f.Size}
		if rel, ok := strings.CutPrefix(f.RelativePath, v.FolderName+"/"); ok {
			if top, _, nested := strings.Cut(rel, "/"); nested {
				ff.AlbumFolder, ff.AlbumYear = albumFolderName(top, t.Name)
				ff.FolderPartID = partOf[top]
			}
		} else if dir := path.Dir(f.RelativePath); dir != "." {
			// Passed on from another artist's folder: only its own album folder's name counts.
			ff.AlbumFolder, ff.AlbumYear = albumFolderName(path.Base(dir), t.Name)
		}
		files = append(files, ff)
	}
	return files, byPath, nil
}

// rematchArtist runs reconcileFoundTracks for one artist's own folders,
// after its tracks changed (a refresh, a Pressing picked by hand), so found
// files that now have a track are linked without a Library rescan.
// Only files already recorded are read; nothing moves on disk.
func (s *Service) rematchArtist(ctx context.Context, titleID int64, by domain.Actor) {
	vs, err := database.Select[mediafiles.Version](ctx, s.Store.Read(), "versions", "title_id = ? ORDER BY id", titleID)
	if err != nil {
		log.Printf("library: rematch artist %d: %v", titleID, err)
		return
	}
	for _, v := range vs {
		if v.SharedFolder || v.FolderName == "" || !filepath.IsLocal(v.FolderName) {
			continue
		}
		var lf mediafiles.LibraryFolder
		if err := database.Get(ctx, s.Store.Read(), "library_folders", v.LibraryFolderID, &lf); err != nil || lf.Kind != "music" {
			continue
		}
		if _, err := s.reconcileFoundTracks(ctx, lf, v, by); err != nil {
			log.Printf("library: rematch artist %d: %v", titleID, err)
		}
	}
}

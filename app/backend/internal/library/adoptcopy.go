package library

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

type CopyAdoption struct {
	RelativePath     string `json:"relative_path"`
	ConfirmedEdition string `json:"confirmed_edition"`
}

// AdoptMovieCopy records a reviewed existing file in an empty movie Version.
// The file stays exactly where it is; adoption never replaces another copy.
func (s *Service) AdoptMovieCopy(ctx context.Context, titleID, versionID int64, in CopyAdoption, by domain.Actor) (mediafiles.File, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	return s.adoptMovieCopy(ctx, titleID, versionID, in, false, by)
}

// adoptMovieCopy is AdoptMovieCopy under the files lock. With fromRescan, a
// slot whose download is no longer running is untied from it first, and a
// slot being grabbed or imported is left alone.
func (s *Service) adoptMovieCopy(ctx context.Context, titleID, versionID int64, in CopyAdoption, fromRescan bool, by domain.Actor) (mediafiles.File, error) {
	var f mediafiles.File
	v, err := versionOf(ctx, s.Store.Read(), titleID, versionID)
	if err != nil {
		return f, err
	}
	var t mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil {
		return f, err
	}
	if t.Kind != "movie" || t.State == "removed" {
		return f, fmt.Errorf("%w: choose a current movie", ErrCopyChoice)
	}
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, s.Store.Read(), "library_folders", v.LibraryFolderID, &lf); err != nil {
		return f, err
	}
	rel := filepath.FromSlash(in.RelativePath)
	root := filepath.Join(lf.Path, v.FolderName)
	abs := filepath.Join(lf.Path, rel)
	within, err := filepath.Rel(root, abs)
	if !filepath.IsLocal(rel) || err != nil || !filepath.IsLocal(within) || !importer.InsideRealFile(root, abs) || !importer.InsideRealFile(lf.Path, abs) {
		return f, fmt.Errorf("%w: choose a file inside this Version's folder", ErrCopyChoice)
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.Mode().IsRegular() || !videoExt[strings.ToLower(filepath.Ext(abs))] {
		return f, fmt.Errorf("%w: choose an existing video file", ErrCopyChoice)
	}
	facts := importer.InspectFacts(abs)
	edition := importer.ParsedEdition(filepath.Base(abs))
	source := "name"
	if edition == "" {
		source = "unknown"
	}
	if in.ConfirmedEdition != "" {
		edition, _, err = copyChoices("movie", in.ConfirmedEdition, nil)
		if err != nil {
			return f, fmt.Errorf("%w: %v", ErrCopyChoice, err)
		}
		source = "confirmed"
	}
	if v.Edition != "" && (edition == "" || importer.EditionKey(v.Edition) != importer.EditionKey(edition)) {
		return f, fmt.Errorf("%w: this file's edition needs an explicit matching choice", ErrCopyChoice)
	}
	if !importer.AudioMeets(facts, v.AudioLanguages) {
		return f, fmt.Errorf("%w: inspected audio does not contain every requested language", ErrCopyChoice)
	}
	facts["edition"], facts["edition_source"] = edition, source
	name := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	p := releasename.Parse(name, releasename.Hints{})
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		var slot mediafiles.Slot
		slots, err := database.Select[mediafiles.Slot](ctx, tx, "slots", "version_id = ?", v.ID)
		if err != nil {
			return err
		}
		if len(slots) != 1 {
			return fmt.Errorf("%w: choose one movie Version", ErrCopyChoice)
		}
		slot = slots[0]
		if fromRescan {
			switch slot.State {
			case mediafiles.SlotChosen, mediafiles.SlotDownloading, mediafiles.SlotImporting:
				return fmt.Errorf("%w: this Version is being downloaded", ErrCopyChoice)
			}
			if slot.FileID == nil && slot.ActiveDownloadID != nil && s.DownloadRunning != nil {
				running, err := s.DownloadRunning(tx, *slot.ActiveDownloadID)
				if err != nil {
					return err
				}
				if !running {
					if err := mediafiles.ClearSlotDownloadTx(tx, slot.ID); err != nil {
						return err
					}
					slot.ActiveDownloadID = nil
				}
			}
		}
		if slot.FileID != nil || slot.ActiveDownloadID != nil || slot.State == mediafiles.SlotRemoved {
			return fmt.Errorf("%w: this Version already has a copy or download", ErrCopyChoice)
		}
		existing, err := database.Select[mediafiles.File](ctx, tx, "files", `version_id IN (SELECT id FROM versions WHERE library_folder_id = ?) AND lower(relative_path) = lower(?) AND state IN ('present','missing')`, lf.ID, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		var foundID int64
		if len(existing) > 0 {
			if len(existing) != 1 || existing[0].VersionID != v.ID || existing[0].Origin != "found" || len(existing[0].SlotIDs) != 0 || existing[0].Protected {
				return fmt.Errorf("%w: this file already belongs to a copy", ErrCopyChoice)
			}
			foundID = existing[0].ID
		}
		f = mediafiles.File{VersionID: v.ID, SlotIDs: []int64{slot.ID}, RelativePath: filepath.ToSlash(rel), Size: fi.Size(), AddedAt: s.now(), Languages: InspectedLanguages(facts),
			MediaInfo: facts, Quality: mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value}, ReleaseName: &name, Origin: "adopted", PlacedBy: "none", Role: "main", State: "present"}
		if foundID == 0 {
			if _, err := database.Insert(tx, "files", &f, by); err != nil {
				return err
			}
		} else {
			f.ID = foundID
			if err := database.Update(tx, "files", &f, by); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`UPDATE slots SET file_id = ?, state = 'in_library', last_reason = 'adopted an existing copy', updated_at = ?, updated_by = ? WHERE id = ?`, f.ID, database.FormatTime(s.now()), ActorText(by), slot.ID)
		return err
	})
	if err == nil {
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAdopted, TitleID: &titleID, Summary: "Adopted an existing " + v.Label + " copy", Data: map[string]any{"file_id": f.ID, "version_id": v.ID, "edition": edition, "edition_source": source}, GroupKey: GroupKey(titleID)})
	}
	return f, err
}

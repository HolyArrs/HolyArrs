package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

var ErrCopyChoice = errors.New("invalid copy choice")

type FileChoice struct {
	Protected             *bool   `json:"protected"`
	Edition               *string `json:"edition"`
	AcknowledgeProtection bool    `json:"acknowledge_protection"`
}

func ownedFile(ctx context.Context, q store.Querier, titleID, fileID int64) (domain.File, domain.Version, error) {
	var f domain.File
	if err := store.Get(ctx, q, "files", fileID, &f); err != nil {
		return f, domain.Version{}, err
	}
	v, err := versionOf(ctx, q, titleID, f.VersionID)
	if err != nil {
		return f, v, err
	}
	if f.Role != "main" || f.State != "present" && f.State != "missing" {
		return f, v, fmt.Errorf("%w: choose a current main copy", ErrCopyChoice)
	}
	var t domain.Title
	if err := store.Get(ctx, q, "titles", titleID, &t); err != nil {
		return f, v, err
	}
	if t.State == "removed" {
		return f, v, ErrAlreadyRemoved
	}
	return f, v, nil
}

// SetFileChoice changes explicit choices while placement/removal is excluded.
// Edition correction records a human decision; it does not move a file.
func (s *Service) SetFileChoice(ctx context.Context, titleID, fileID int64, choice FileChoice, by domain.Actor) (domain.File, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	var f domain.File
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var err error
		if f, _, err = ownedFile(ctx, tx, titleID, fileID); err != nil {
			return err
		}
		if choice.Protected != nil {
			if f.Protected && !*choice.Protected && !choice.AcknowledgeProtection {
				return fmt.Errorf("%w: acknowledge removal of protection separately", ErrCopyChoice)
			}
			f.Protected = *choice.Protected
		}
		if choice.Edition != nil {
			var kind string
			if err := tx.QueryRow("SELECT kind FROM titles WHERE id = ?", titleID).Scan(&kind); err != nil {
				return err
			}
			edition, _, err := copyChoices(kind, *choice.Edition, nil)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrCopyChoice, err)
			}
			if f.MediaInfo == nil {
				f.MediaInfo = map[string]any{}
			}
			f.MediaInfo["edition"], f.MediaInfo["edition_source"] = edition, "confirmed"
		}
		return store.Update(tx, "files", &f, by)
	})
	if err == nil {
		s.event(ctx, &domain.Event{Actor: by, Kind: "file_choice_changed", TitleID: &titleID, Summary: "Changed copy choices for " + f.RelativePath,
			Data: map[string]any{"file_id": f.ID, "protected": f.Protected, "edition": f.MediaInfo["edition"], "protection_acknowledged": choice.AcknowledgeProtection}, GroupKey: groupKey(titleID)})
	}
	return f, err
}

// KeepAndAddVersion protects the owned copy and adds an explicitly chosen
// separate destination atomically. It never searches or starts a download.
func (s *Service) KeepAndAddVersion(ctx context.Context, titleID, fileID int64, in VersionInput, by domain.Actor) (domain.Version, error) {
	if in.LibraryFolderID == 0 || in.PickRulesID == 0 {
		return domain.Version{}, fmt.Errorf("%w: choose rules and a Library folder for the other copy", ErrCopyChoice)
	}
	unlock := s.Store.LockFiles()
	defer unlock()
	var title domain.Title
	if err := store.Get(ctx, s.Store.Read(), "titles", titleID, &title); err != nil {
		return domain.Version{}, err
	}
	if _, err := checkVersionFolder(ctx, s.Store.Read(), in.LibraryFolderID, title.Kind, s.downloadPaths(ctx)); err != nil {
		return domain.Version{}, err
	}
	var v domain.Version
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		f, _, err := ownedFile(ctx, tx, titleID, fileID)
		if err != nil {
			return err
		}
		if v, err = addVersionTx(ctx, tx, title, in, s.now(), by); err != nil {
			return err
		}
		f.Protected = true
		return store.Update(tx, "files", &f, by)
	})
	if err == nil {
		s.VersionEvent(ctx, title, "Protected the existing copy and added the "+v.Label+" Version", v, by)
	}
	return v, err
}

// ConfirmDownloadEdition records an explicit correction for one movie
// download. A repeat import still checks protection and inspected audio.
func (s *Service) ConfirmDownloadEdition(ctx context.Context, downloadID int64, edition string, by domain.Actor) error {
	edition, _, err := copyChoices("movie", edition, nil)
	if err != nil || edition == "" {
		return fmt.Errorf("%w: choose a nonempty movie edition", ErrCopyChoice)
	}
	unlock := s.Store.LockFiles()
	defer unlock()
	var titleID int64
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		var d domain.Download
		if err := store.Get(ctx, tx, "downloads", downloadID, &d); err != nil {
			return err
		}
		if d.State != "finished" && d.State != "seeding" {
			return fmt.Errorf("%w: wait for the download to finish", ErrCopyChoice)
		}
		if len(d.SlotIDs) != 1 {
			return fmt.Errorf("%w: choose one movie download", ErrCopyChoice)
		}
		var kind, state string
		if err := tx.QueryRow(`SELECT t.id, t.kind, t.state FROM slots sl JOIN items i ON i.id = sl.item_id JOIN titles t ON t.id = i.title_id WHERE sl.id = ?`, d.SlotIDs[0]).Scan(&titleID, &kind, &state); err != nil {
			return err
		}
		if kind != "movie" || state == "removed" {
			return fmt.Errorf("%w: choose a current movie download", ErrCopyChoice)
		}
		d.ConfirmedEdition = edition
		return store.Update(tx, "downloads", &d, by)
	})
	if err == nil {
		s.event(ctx, &domain.Event{Actor: by, Kind: "download_edition_confirmed", TitleID: &titleID, DownloadID: &downloadID,
			Summary: "Confirmed movie edition: " + edition, Data: map[string]any{"edition": edition}, GroupKey: groupKey(titleID)})
	}
	return err
}

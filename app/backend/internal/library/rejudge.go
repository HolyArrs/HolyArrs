package library

import (
	"context"
	"database/sql"
	"log"
	"path"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

const rejudgedSetting = "adopted_quality_rejudged"

// RejudgeAdopted runs once per install. Files moved over from the old apps
// were judged by a name reader that took "WEBDL-1080p" for a release group
// and ignored the resolution the file inspection read, so many good copies
// were marked "better copy wanted". It reads each adopted file's quality
// again and lets each Version's Slots swap to match. It returns how many
// Slots changed.
func (s *Service) RejudgeAdopted(ctx context.Context) (int, error) {
	if database.SettingOr(ctx, s.Store.Read(), "this_app", rejudgedSetting, "") != "" {
		return 0, nil
	}
	versionIDs, err := mediafiles.AdoptedVersionIDs(ctx, s.Store.Read())
	if err != nil {
		return 0, err
	}
	by := domain.System
	changed := 0
	// Small writes, so the app stays responsive on a large Library.
	for start := 0; start < len(versionIDs); start += 50 {
		batch := versionIDs[start:min(start+50, len(versionIDs))]
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			for _, id := range batch {
				n, err := rejudgeVersionTx(ctx, tx, id, by)
				if err != nil {
					return err
				}
				changed += n
			}
			return nil
		})
		if err != nil {
			return changed, err
		}
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, "this_app", rejudgedSetting, time.Now().UTC().Format(time.RFC3339), by, time.Now())
	})
	if changed > 0 {
		log.Printf("library: read the quality of moved-over files again; %d slots changed", changed)
	}
	return changed, err
}

// rejudgeVersionTx stores the quality read again from each adopted file of
// a Version, then swaps its Slots' states to match.
func rejudgeVersionTx(ctx context.Context, tx *sql.Tx, versionID int64, by domain.Actor) (int, error) {
	var v mediafiles.Version
	if err := database.Get(ctx, tx, "versions", versionID, &v); err != nil {
		return 0, nil
	}
	var t mediafiles.Title
	if err := database.Get(ctx, tx, "titles", v.TitleID, &t); err != nil {
		return 0, nil
	}
	files, err := database.Select[mediafiles.File](ctx, tx, "files", "version_id = ?", versionID)
	if err != nil {
		return 0, err
	}
	for _, f := range files {
		if f.Origin != "adopted" || f.State != "present" || f.ReleaseName == nil || *f.ReleaseName == "" {
			continue
		}
		p := releasename.Parse(mediafiles.JudgeName(*f.ReleaseName, f.MediaInfo), releasename.Hints{Size: f.Size, Show: t.Kind == "show"})
		q := mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value, Audio: f.Quality.Audio}
		if q.Resolution == f.Quality.Resolution && q.Source == f.Quality.Source && q.Codec == f.Quality.Codec {
			continue
		}
		f.Quality = q
		if f.MediaInfo == nil {
			f.MediaInfo = map[string]any{}
		}
		f.MediaInfo["step"] = mediafiles.StepFor(p)
		if err := database.Update(tx, "files", &f, by); err != nil {
			return 0, err
		}
	}
	ch, err := goodEnoughTx(ctx, tx, v, t.KeepUpToDate && v.KeepUpToDate, by)
	return len(ch), err
}

// SetRecordedQualities stores the quality the old app recorded for each of
// a title's present files (by file base name), reads each file's quality
// again and lets the title's Slots swap to match. It returns how many
// files it updated. A move calls it for titles it adopts and for titles
// already here, so moving again after reading the old app updates them.
func (s *Service) SetRecordedQualities(ctx context.Context, titleID int64, qualities map[string]string, by domain.Actor) (int, error) {
	if len(qualities) == 0 {
		return 0, nil
	}
	updated := 0
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		versions, err := database.Select[mediafiles.Version](ctx, tx, "versions", "title_id = ?", titleID)
		if err != nil {
			return err
		}
		for _, v := range versions {
			files, err := database.Select[mediafiles.File](ctx, tx, "files", "version_id = ?", v.ID)
			if err != nil {
				return err
			}
			for _, f := range files {
				q := qualities[path.Base(f.RelativePath)]
				if f.State != "present" || q == "" {
					continue
				}
				if old, _ := f.MediaInfo[mediafiles.RecordedQualityKey].(string); old == q {
					continue
				}
				if f.MediaInfo == nil {
					f.MediaInfo = map[string]any{}
				}
				f.MediaInfo[mediafiles.RecordedQualityKey] = q
				if err := database.Update(tx, "files", &f, by); err != nil {
					return err
				}
				updated++
			}
			if _, err := rejudgeVersionTx(ctx, tx, v.ID, by); err != nil {
				return err
			}
		}
		return nil
	})
	return updated, err
}

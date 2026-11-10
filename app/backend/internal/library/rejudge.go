package library

import (
	"context"
	"database/sql"
	"log"
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
	var done string
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", rejudgedSetting).Scan(&done)
	if done != "" {
		return 0, nil
	}
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT DISTINCT version_id FROM files WHERE origin = 'adopted' AND state = 'present'`)
	if err != nil {
		return 0, err
	}
	var versionIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		versionIDs = append(versionIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
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
	now := database.FormatTime(time.Now())
	who := ActorText(by)
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			rejudgedSetting, time.Now().UTC().Format(time.RFC3339), now, who, now, who)
		return err
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
	files, err := database.Select[mediafiles.File](ctx, tx, "files", "version_id = ? AND origin = 'adopted' AND state = 'present'", versionID)
	if err != nil {
		return 0, err
	}
	for _, f := range files {
		if f.ReleaseName == nil || *f.ReleaseName == "" {
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

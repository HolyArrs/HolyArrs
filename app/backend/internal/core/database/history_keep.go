package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// KindCleanHistory is the daily Job that removes old History Events.
const KindCleanHistory = "clean_history"

// AuditKeepDays is how long audit Events are kept (fixed).
const AuditKeepDays = 365

// MaxHistoryKeepDays bounds both values (about ten years).
const MaxHistoryKeepDays = 3650

const (
	settingKeepRoutine = "history_keep_routine_days"
	settingKeepOther   = "history_keep_other_days"
)

// pruneBatch is how many Events one write removes, so the events database
// is never locked for long.
const pruneBatch = 2000

// RoutineEventKinds are the kinds the History page files under Routine
// (app/frontend/src/lib/downloads/history-kinds.ts, the routine tier): searches, feed
// checks, refreshed details and folder scans. Keep the two lists alike.
var RoutineEventKinds = []string{
	"search_done", "feed_checked",
	"title_refreshed", "albums_changed", "album_changed", "episodes_changed", "details_source_changed", "season_changed", "title_edited",
	"library_scanned", "library_rescanned", "folder_scanned", "file_back", "media_server_told",
}

// AuditEventKinds are the audit trail: sign-ins, sign-outs, API keys,
// settings changes and owner controls. The keep times never apply to them;
// they are kept AuditKeepDays, so failed sign-ins from outside cannot grow
// without end. setting_changed is the older kind that held the values sent;
// rows of it already saved stay hidden from History readers too.
// secrets_imported is the owner's import of a secrets file (credentials).
var AuditEventKinds = []string{"signed_in", "sign_in_failed", "signed_out", "api_key_created", "api_key_deleted", "settings_changed", "setting_changed", "secrets_imported", "account_changed", "backup_used", "control_used"}

// DefaultHistoryKeep: routine work for 30 days, everything else forever.
var DefaultHistoryKeep = HistoryKeep{RoutineDays: 30}

// ErrHistoryKeep is a keep time out of range.
var ErrHistoryKeep = errors.New("keep times are whole days from 0 (forever) to 3650")

// HistoryKeep is how long History keeps Events, in days. 0 keeps them
// forever.
type HistoryKeep struct {
	RoutineDays int `json:"routine_days"`
	OtherDays   int `json:"other_days"`
}

// ReadHistoryKeep reads the keep times from the this_app settings group;
// a missing or unreadable value is its default.
func ReadHistoryKeep(ctx context.Context, db Querier) HistoryKeep {
	k := DefaultHistoryKeep
	for key, dst := range map[string]*int{settingKeepRoutine: &k.RoutineDays, settingKeepOther: &k.OtherDays} {
		var v string
		if db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v) != nil {
			continue
		}
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= MaxHistoryKeepDays {
			*dst = n
		}
	}
	return k
}

// SaveHistoryKeep stores both keep times.
func (s *Store) SaveHistoryKeep(ctx context.Context, k HistoryKeep, by Actor) error {
	if k.RoutineDays < 0 || k.OtherDays < 0 || k.RoutineDays > MaxHistoryKeepDays || k.OtherDays > MaxHistoryKeepDays {
		return ErrHistoryKeep
	}
	now := FormatTime(time.Now())
	who, _ := json.Marshal(by)
	return s.Write(ctx, func(tx *sql.Tx) error {
		for key, v := range map[string]int{settingKeepRoutine: k.RoutineDays, settingKeepOther: k.OtherDays} {
			if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				key, strconv.Itoa(v), now, string(who), now, string(who)); err != nil {
				return err
			}
		}
		return nil
	})
}

// PruneEvents removes Events older than their keep time, a batch per
// write, and returns how many went. Nothing in the app reads old Events to
// decide anything (only to show them); AuditEventKinds stay AuditKeepDays.
func (s *Store) PruneEvents(ctx context.Context, k HistoryKeep, now time.Time) (int, error) {
	in := "'" + strings.Join(RoutineEventKinds, "','") + "'"
	audit := "'" + strings.Join(AuditEventKinds, "','") + "'"
	total := 0
	for _, step := range []struct {
		days int
		cond string
	}{{k.RoutineDays, "kind IN (" + in + ")"}, {k.OtherDays, "kind NOT IN (" + in + ") AND kind NOT IN (" + audit + ")"}, {AuditKeepDays, "kind IN (" + audit + ")"}} {
		if step.days <= 0 {
			continue
		}
		cutoff := FormatTime(now.Add(-time.Duration(step.days) * 24 * time.Hour))
		for {
			var n int64
			err := s.WriteEvents(ctx, func(tx *sql.Tx) error {
				res, err := tx.Exec("DELETE FROM events WHERE id IN (SELECT id FROM events WHERE "+step.cond+" AND at < ? LIMIT ?)", cutoff, pruneBatch)
				if err == nil {
					n, err = res.RowsAffected()
				}
				return err
			})
			total += int(n)
			if err != nil {
				return total, err
			}
			if n < pruneBatch {
				break
			}
			if err := ctx.Err(); err != nil {
				return total, err
			}
		}
	}
	return total, nil
}

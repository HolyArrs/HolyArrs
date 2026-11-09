package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ScheduleEvery reads how often a schedule runs, in seconds, and whether it
// is on. A kind with no row reads as 0 and off, without an error.
func ScheduleEvery(ctx context.Context, q database.Querier, kind string) (every int, enabled bool, err error) {
	err = q.QueryRowContext(ctx, "SELECT every_seconds, enabled FROM schedules WHERE kind = ?", kind).Scan(&every, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return every, enabled, err
}

// ScheduleEnabled says whether a schedule is on; a kind with no row is off.
func ScheduleEnabled(ctx context.Context, q database.Querier, kind string) bool {
	var on bool
	_ = q.QueryRowContext(ctx, "SELECT enabled FROM schedules WHERE kind = ?", kind).Scan(&on)
	return on
}

// SetScheduleEveryTx sets how often a schedule runs and whether it is on, in
// the caller's transaction. Unlike UpdateSchedule it leaves the next run and
// the queued Jobs alone.
func SetScheduleEveryTx(tx *sql.Tx, kind string, everySeconds int, enabled bool, by database.Actor, now time.Time) error {
	who, _ := json.Marshal(by)
	_, err := tx.Exec("UPDATE schedules SET every_seconds = ?, enabled = ?, updated_at = ?, updated_by = ? WHERE kind = ?",
		everySeconds, enabled, database.FormatTime(now), string(who), kind)
	return err
}

// SetScheduleEnabledTx turns a schedule on or off in the caller's
// transaction, leaving the next run and the queued Jobs alone.
func SetScheduleEnabledTx(tx *sql.Tx, kind string, enabled bool, by database.Actor, now time.Time) error {
	who, _ := json.Marshal(by)
	_, err := tx.Exec("UPDATE schedules SET enabled = ?, updated_at = ?, updated_by = ? WHERE kind = ?",
		enabled, database.FormatTime(now), string(who), kind)
	return err
}

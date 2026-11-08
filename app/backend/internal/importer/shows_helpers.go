package importer

import (
	"database/sql"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func anyStuck(slots map[int64]*showSlot) bool {
	for _, ss := range slots {
		if ss.live && ss.outcome == "stuck" {
			return true
		}
	}
	return false
}

func refLabel(it mediafiles.Item) string {
	if it.Kind == "track" {
		d, n := 1, 0
		if it.Numbers.Disc != nil {
			d = *it.Numbers.Disc
		}
		if it.Numbers.Track != nil {
			n = *it.Numbers.Track
		}
		return trackLabel(d, n, map[bool]int{false: 1, true: 2}[d > 1])
	}
	s, e := 0, 0
	if it.Numbers.Season != nil {
		s = *it.Numbers.Season
	}
	if it.Numbers.Episode != nil {
		e = *it.Numbers.Episode
	}
	return fmt.Sprintf("S%02dE%02d", s, e)
}

func fileName(f mediafiles.File) string {
	if f.ReleaseName != nil && *f.ReleaseName != "" {
		return *f.ReleaseName
	}
	return strings.TrimSuffix(path.Base(f.RelativePath), path.Ext(f.RelativePath))
}

func jsonIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func turnDownForTx(tx *sql.Tx, releaseID int64, slotIDs []int64, by domain.Actor) error {
	var cur sql.NullString
	if err := tx.QueryRow("SELECT json_extract(verdict, '$.turned_down_slot_ids') FROM releases WHERE id = ?", releaseID).Scan(&cur); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	ids := []int64{}
	if cur.Valid {
		for _, s := range strings.Split(strings.Trim(cur.String, "[]"), ",") {
			if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && !slices.Contains(ids, n) {
				ids = append(ids, n)
			}
		}
	}
	for _, id := range slotIDs {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	_, err := tx.Exec(`UPDATE releases SET state = 'turned_down', hold_until = NULL,
		verdict = json_set(json_set(COALESCE(verdict, '{}'), '$.turned_down_slot_ids', json(?)), '$.turned_down_reason', ?), updated_at = ?, updated_by = ? WHERE id = ?`,
		jsonIDs(ids), "the files did not fill these episodes", database.FormatTime(time.Now()), actorText(by), releaseID)
	return err
}

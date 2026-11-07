package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// FileRules are the settings for where files go that are not naming: how
// long removed and replaced files stay in the holding folder, and the free
// space an import must leave on the disk.
type FileRules struct {
	// HoldingDays is how long files stay in the holding folder; 0 keeps them.
	HoldingDays int `json:"holding_days"`
	// MinFreeMB is the free space an import must leave (100 at least).
	MinFreeMB int `json:"min_free_mb"`
	// SkipFreeSpaceCheck turns the check at import off.
	SkipFreeSpaceCheck bool `json:"skip_free_space_check"`
}

// Errors from saving FileRules.
var (
	ErrMinFree     = errors.New("min_free_mb must be between 100 and 1048576")
	ErrHoldingDays = errors.New("holding_days must be between 0 (keep) and 3650")
)

// DefaultFileRules mirror Radarr: 7 days, 100 MB, check on.
func DefaultFileRules() FileRules { return FileRules{HoldingDays: 7, MinFreeMB: 100} }

// Validate says what is wrong with r.
func (r FileRules) Validate() error {
	if r.MinFreeMB < 100 || r.MinFreeMB > 1<<20 {
		return ErrMinFree
	}
	if r.HoldingDays < 0 || r.HoldingDays > 3650 {
		return ErrHoldingDays
	}
	return nil
}

// ReadFileRules reads the rules; unset or broken values keep their default.
func ReadFileRules(ctx context.Context, db store.Lister) (FileRules, error) {
	r := DefaultFileRules()
	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE grp = 'where_files_go' AND key IN ('holding_days', 'min_free_mb', 'skip_free_space_check')")
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return r, err
		}
		n, nerr := strconv.Atoi(v)
		switch k {
		case "holding_days":
			if nerr == nil && n >= 0 && n <= 3650 {
				r.HoldingDays = n
			}
		case "min_free_mb":
			if nerr == nil && n >= 100 && n <= 1<<20 {
				r.MinFreeMB = n
			}
		case "skip_free_space_check":
			r.SkipFreeSpaceCheck = v == "true"
		}
	}
	return r, rows.Err()
}

// SaveFileRules stores valid rules.
func SaveFileRules(ctx context.Context, s *store.Store, r FileRules, by domain.Actor) error {
	if err := r.Validate(); err != nil {
		return err
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		now := store.FormatTime(time.Now())
		for k, v := range map[string]string{"holding_days": strconv.Itoa(r.HoldingDays), "min_free_mb": strconv.Itoa(r.MinFreeMB),
			"skip_free_space_check": strconv.FormatBool(r.SkipFreeSpaceCheck)} {
			if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('where_files_go', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, v, now, actorText(by), now, actorText(by)); err != nil {
				return err
			}
		}
		return nil
	})
}

// SpaceError is a refusal for lack of free space.
type SpaceError struct {
	Dir              string
	Need, Keep, Free int64
}

func (e *SpaceError) Error() string {
	return fmt.Sprintf("not enough free space in %s: this file needs %d MB and %d MB must stay free, only %d MB are free",
		e.Dir, (e.Need+mib-1)/mib, e.Keep/mib, e.Free/mib)
}

const mib = 1 << 20

// CheckSpaceMin refuses writing size bytes into dir when the disk would be
// left with less than minMB megabytes. Unknown free space never blocks.
func CheckSpaceMin(dir string, size int64, minMB int) error {
	return checkSpaceReserved(existingDir(dir), size, 0, minMB)
}

// existingDir is dir or its nearest folder that exists.
func existingDir(dir string) string {
	for p := dir; ; {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		up := filepath.Dir(p)
		if up == p {
			return dir
		}
		p = up
	}
}

// checkSpaceReserved is CheckSpaceMin for a disk that already gives
// reserved bytes to earlier files of the same plan.
func checkSpaceReserved(dir string, size, reserved int64, minMB int) error {
	keep := int64(minMB) * mib
	free := freeSpaceOf(dir)
	if free >= 0 && free-reserved < size+keep {
		return &SpaceError{Dir: dir, Need: size, Keep: keep, Free: max(free-reserved, 0)}
	}
	return nil
}

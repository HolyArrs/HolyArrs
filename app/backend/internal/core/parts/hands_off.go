package parts

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Hands-off mode: HolyArrs picks a release and sends it to the Downloader,
// then leaves the files alone. No Library folder is needed, nothing is
// imported, renamed, moved, scanned or deleted, and nothing is written
// outside the config folder. Subtitles need the files, so that part is off
// while the mode is on.

// settings row of hands-off mode ("1" on).
const handsOffKey = "hands_off"

// HandsOffFilesText is what a route that works on files answers while
// hands-off mode is on.
const HandsOffFilesText = "Hands-off mode is on, so HolyArrs leaves the downloaded files alone. Turn hands-off mode off in Settings to import, rename, move or scan files."

// HandsOffKinds are the Job kinds that read or change files in a Library
// folder. While hands-off mode is on they are not queued, and ones already
// queued wait until it is turned off.
var HandsOffKinds = map[string]bool{
	"check_library_folders": true, "scan_title": true, "rescan_title": true, "move_title": true, "rename_files": true,
	"clean_holding": true, "scan_library_folder": true, "adopt_library": true, "rescan_library": true, "migrate_move": true,
}

// HandsOffRoutes are the routes that read or change files in a Library
// folder; while hands-off mode is on they answer HandsOffFilesText.
var HandsOffRoutes = map[string]bool{
	"POST /api/v1/holding/clean": true, "POST /api/v1/holding/restore": true,
	"POST /api/v1/library-folders/{id}/adopt": true, "POST /api/v1/library-folders/{id}/scan": true,
	"POST /api/v1/library-folders/{id}/tidy": true, "POST /api/v1/library-folders/{id}/tidy/undo": true,
	"POST /api/v1/manual-import": true, "POST /api/v1/manual-import/scan": true, "POST /api/v1/manual-import/folders": true, "POST /api/v1/rename": true,
	"POST /api/v1/migrate/moves": true, "POST /api/v1/migrate/moves/{id}/undo": true,
	"POST /api/v1/titles/{id}/leftovers/match": true, "POST /api/v1/titles/{id}/leftovers/unmatch": true,
	"PUT /api/v1/titles/{id}/leftovers/{fid}": true, "POST /api/v1/titles/{id}/move": true,
	"POST /api/v1/titles/{id}/rename": true, "POST /api/v1/titles/{id}/rescan": true,
	"POST /api/v1/titles/{id}/versions/{vid}/adopt": true,
}

// HandsOffSubtitlesReason is why Subtitles is off while hands-off mode is on.
const HandsOffSubtitlesReason = "Hands-off mode is on, so HolyArrs never opens the downloaded files. Turn hands-off mode off in Settings to use subtitles."

// ErrHandsOffSubtitles refuses turning Subtitles on while hands-off mode is on.
var ErrHandsOffSubtitles = errors.New("subtitles need the downloaded files, and hands-off mode is on; turn hands-off mode off in Settings first")

// HandsOff reports whether hands-off mode is on (off when unreadable).
func HandsOff(ctx context.Context, q database.Querier) bool {
	return database.SettingOr(ctx, q, settingGroup, handsOffKey, "") == "1"
}

// SetHandsOff saves hands-off mode and applies it to the running parts (c
// may be nil). Turning it off moves nothing: normal behaviour returns.
func SetHandsOff(ctx context.Context, s *database.Store, c *Control, on bool, by domain.Actor) error {
	v := "0"
	if on {
		v = "1"
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingGroup, handsOffKey, v, by, time.Now().UTC())
	}); err != nil {
		return err
	}
	if c != nil {
		c.mu.Lock()
		c.init()
		c.handsOff = on
		c.flag()
		c.mu.Unlock()
		c.apply()
	}
	return nil
}

// HandsOffOn reports whether hands-off mode is on for the running parts.
func (c *Control) HandsOffOn() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handsOff
}

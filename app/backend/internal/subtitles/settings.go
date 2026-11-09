package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Schedule kinds.
const (
	KindMissing = "subtitle_missing" // search every File missing a wanted subtitle
	KindBetter  = "subtitle_better"  // search again to improve recently saved subtitles
	KindSearch  = "subtitle_search"  // search a single File
	KindLook    = "subtitle_look"    // read what Files have after a wish changed, without asking any site
)

// Settings holds the subtitle settings in the what_to_pick group. The two
// schedule intervals live on their schedules rows instead.
type Settings struct {
	// Languages is the default wish for every movie and show without its own.
	Languages []Want `json:"languages"`
	// Pass marks, from 0 to 100.
	PassMarkMovie   int `json:"pass_mark_movie"`
	PassMarkEpisode int `json:"pass_mark_episode"`
	// UseBuiltIn counts subtitle tracks inside the video toward the wish.
	// IgnorePictureTracks leaves picture tracks such as PGS and VobSub out of it.
	UseBuiltIn          bool `json:"use_built_in"`
	IgnorePictureTracks bool `json:"ignore_picture_tracks"`
	// HISuffix is the tag used in hearing-impaired file names: hi, cc or sdh.
	HISuffix string `json:"hi_suffix"`
	// SearchHours is how often missing subtitles are searched for.
	SearchHours int `json:"search_hours"`
	// Better turns on the search for better subtitles. BetterHours is how often
	// it runs and BetterDays is the most age a saved subtitle may have to qualify.
	Better      bool `json:"better"`
	BetterHours int  `json:"better_hours"`
	BetterDays  int  `json:"better_days"`
	// Batch caps the Files one scheduled run searches, to spread site load.
	Batch int `json:"batch"`
	// Adaptive drops a wish that keeps finding nothing to a weekly try after
	// three weeks, to spare the sites.
	Adaptive bool `json:"adaptive"`
	// SyncFromFPS and SyncToFPS are the frame-rate change a Sync fix offers first,
	// such as a subtitle timed for 25 fps on a 23.976 fps video. SyncStepMS is the
	// step of the offset buttons. A person can change all three in the fix.
	SyncFromFPS float64 `json:"sync_from_fps"`
	SyncToFPS   float64 `json:"sync_to_fps"`
	SyncStepMS  int     `json:"sync_step_ms"`
}

// DefaultSettings returns the settings in use before anyone changes them.
func DefaultSettings() Settings {
	return Settings{Languages: []Want{}, PassMarkMovie: 70, PassMarkEpisode: 90, UseBuiltIn: true, HISuffix: "hi",
		SearchHours: 6, Better: true, BetterHours: 12, BetterDays: 7, Batch: 100, Adaptive: true,
		SyncFromFPS: 25, SyncToFPS: 23.976, SyncStepMS: 500}
}

// Validate checks every field.
func (s *Settings) Validate() error {
	ws, err := CleanWants(s.Languages)
	if err != nil {
		return err
	}
	s.Languages = ws
	switch {
	case s.PassMarkMovie < 0 || s.PassMarkMovie > 100, s.PassMarkEpisode < 0 || s.PassMarkEpisode > 100:
		return fmt.Errorf("pass marks must be 0 to 100")
	case s.HISuffix != "hi" && s.HISuffix != "cc" && s.HISuffix != "sdh":
		return fmt.Errorf("hi_suffix must be hi, cc or sdh")
	case s.SearchHours < 1 || s.SearchHours > 168:
		return fmt.Errorf("search_hours must be 1 to 168")
	case s.BetterHours < 6 || s.BetterHours > 168:
		return fmt.Errorf("better_hours must be 6 to 168")
	case s.BetterDays < 0 || s.BetterDays > 30:
		return fmt.Errorf("better_days must be 0 to 30")
	case s.Batch < 1 || s.Batch > 1000:
		return fmt.Errorf("batch must be 1 to 1000")
	case s.SyncFromFPS < 1 || s.SyncFromFPS > 120 || s.SyncToFPS < 1 || s.SyncToFPS > 120:
		return fmt.Errorf("sync_from_fps and sync_to_fps must be 1 to 120")
	case s.SyncStepMS < 10 || s.SyncStepMS > 60000:
		return fmt.Errorf("sync_step_ms must be 10 to 60000")
	}
	return nil
}

var settingKeys = []string{"subtitle_languages", "subtitle_pass_mark_movie", "subtitle_pass_mark_episode", "subtitle_use_built_in",
	"subtitle_ignore_picture_tracks", "subtitle_hi_suffix", "subtitle_better", "subtitle_better_days", "subtitle_batch", "subtitle_adaptive",
	"subtitle_sync_from_fps", "subtitle_sync_to_fps", "subtitle_sync_step_ms"}

// ReadSettings reads the subtitle settings. Unset or odd values keep their
// defaults so a bad row never breaks subtitles.
func ReadSettings(ctx context.Context, db *sql.DB) Settings {
	s := DefaultSettings()
	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE grp = 'what_to_pick' AND key LIKE 'subtitle_%'")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if rows.Scan(&k, &v) != nil {
				continue
			}
			n, nerr := strconv.Atoi(v)
			b, berr := strconv.ParseBool(v)
			fl, ferr := strconv.ParseFloat(v, 64)
			switch k {
			case "subtitle_languages":
				var ws []Want
				if json.Unmarshal([]byte(v), &ws) == nil {
					if c, err := CleanWants(ws); err == nil {
						s.Languages = c
					}
				}
			case "subtitle_pass_mark_movie":
				if nerr == nil && n >= 0 && n <= 100 {
					s.PassMarkMovie = n
				}
			case "subtitle_pass_mark_episode":
				if nerr == nil && n >= 0 && n <= 100 {
					s.PassMarkEpisode = n
				}
			case "subtitle_use_built_in":
				if berr == nil {
					s.UseBuiltIn = b
				}
			case "subtitle_ignore_picture_tracks":
				if berr == nil {
					s.IgnorePictureTracks = b
				}
			case "subtitle_hi_suffix":
				if v == "hi" || v == "cc" || v == "sdh" {
					s.HISuffix = v
				}
			case "subtitle_better":
				if berr == nil {
					s.Better = b
				}
			case "subtitle_better_days":
				if nerr == nil && n >= 0 && n <= 30 {
					s.BetterDays = n
				}
			case "subtitle_batch":
				if nerr == nil && n >= 1 && n <= 1000 {
					s.Batch = n
				}
			case "subtitle_adaptive":
				if berr == nil {
					s.Adaptive = b
				}
			case "subtitle_sync_from_fps":
				if ferr == nil && fl >= 1 && fl <= 120 {
					s.SyncFromFPS = fl
				}
			case "subtitle_sync_to_fps":
				if ferr == nil && fl >= 1 && fl <= 120 {
					s.SyncToFPS = fl
				}
			case "subtitle_sync_step_ms":
				if nerr == nil && n >= 10 && n <= 60000 {
					s.SyncStepMS = n
				}
			}
		}
	}
	for kind, dst := range map[string]*int{KindMissing: &s.SearchHours, KindBetter: &s.BetterHours} {
		var secs int64
		if db.QueryRowContext(ctx, "SELECT every_seconds FROM schedules WHERE kind = ?", kind).Scan(&secs) == nil && secs >= 3600 {
			*dst = int(secs / 3600)
		}
	}
	return s
}

// SaveSettings stores the settings and updates both schedules. When then is
// given, it runs in the same transaction.
func SaveSettings(ctx context.Context, st *database.Store, s Settings, by domain.Actor, then ...func(tx *sql.Tx) error) error {
	if err := s.Validate(); err != nil {
		return err
	}
	langs, _ := json.Marshal(s.Languages)
	vals := map[string]string{
		"subtitle_languages": string(langs), "subtitle_pass_mark_movie": strconv.Itoa(s.PassMarkMovie),
		"subtitle_pass_mark_episode": strconv.Itoa(s.PassMarkEpisode), "subtitle_use_built_in": strconv.FormatBool(s.UseBuiltIn),
		"subtitle_ignore_picture_tracks": strconv.FormatBool(s.IgnorePictureTracks), "subtitle_hi_suffix": s.HISuffix,
		"subtitle_better": strconv.FormatBool(s.Better), "subtitle_better_days": strconv.Itoa(s.BetterDays),
		"subtitle_batch": strconv.Itoa(s.Batch), "subtitle_adaptive": strconv.FormatBool(s.Adaptive),
		"subtitle_sync_from_fps": strconv.FormatFloat(s.SyncFromFPS, 'f', -1, 64), "subtitle_sync_to_fps": strconv.FormatFloat(s.SyncToFPS, 'f', -1, 64),
		"subtitle_sync_step_ms": strconv.Itoa(s.SyncStepMS),
	}
	actor, _ := json.Marshal(by)
	ts := database.FormatTime(time.Now())
	return st.Write(ctx, func(tx *sql.Tx) error {
		for _, k := range settingKeys {
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', ?, ?, ?, ?, ?, ?)
				ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				k, vals[k], ts, string(actor), ts, string(actor)); err != nil {
				return err
			}
		}
		for _, x := range []struct {
			kind    string
			hours   int
			enabled bool
		}{{KindMissing, s.SearchHours, true}, {KindBetter, s.BetterHours, s.Better}} {
			// A new interval starts counting from now.
			if _, err := tx.ExecContext(ctx, `INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES (?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (kind) DO UPDATE SET next_run = CASE WHEN every_seconds != excluded.every_seconds THEN ? ELSE next_run END,
				every_seconds = excluded.every_seconds, enabled = excluded.enabled, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
				ts, string(actor), ts, string(actor), x.kind, x.hours*3600, x.enabled,
				database.FormatTime(time.Now().Add(time.Duration(x.hours)*time.Hour))); err != nil {
				return err
			}
		}
		for _, f := range then {
			if err := f(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

// PassMark returns the pass mark for a kind of Item, movie or episode.
func (s Settings) PassMark(itemKind string) int {
	if itemKind == "episode" {
		return s.PassMarkEpisode
	}
	return s.PassMarkMovie
}

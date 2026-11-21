package youtube

import (
	"context"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Settings group and keys. Stored names: never rename them. The settings
// table takes only a fixed list of groups, so the keys live in this_app
// under a youtube_ prefix.
const (
	settingsGroup     = "this_app"
	keyFolder         = "youtube_folder"
	keyMinFreeGB      = "youtube_min_free_gb"
	keyYTDLPUpdatedAt = "youtube_ytdlp_updated_at"
	keyYTDLPUpdateErr = "youtube_ytdlp_update_error"
)

// Free-space floor bounds, in GB.
const (
	DefaultMinFreeGB = 10
	MaxFreeGB        = 100000
)

// Settings are the owner's YouTube settings and how the last yt-dlp
// self-update went. Only Folder and MinFreeGB are set from outside.
type Settings struct {
	Folder      string `json:"folder"`
	MinFreeGB   int    `json:"min_free_gb"`
	UpdatedAt   string `json:"ytdlp_updated_at"`
	UpdateError string `json:"ytdlp_update_error"`
}

// LoadSettings reads the YouTube settings; a missing or unreadable value
// takes its default.
func LoadSettings(ctx context.Context, q database.Querier) Settings {
	gb, err := strconv.Atoi(database.SettingOr(ctx, q, settingsGroup, keyMinFreeGB, ""))
	if err != nil || gb < 1 || gb > MaxFreeGB {
		gb = DefaultMinFreeGB
	}
	return Settings{
		Folder:      database.SettingOr(ctx, q, settingsGroup, keyFolder, ""),
		MinFreeGB:   gb,
		UpdatedAt:   database.SettingOr(ctx, q, settingsGroup, keyYTDLPUpdatedAt, ""),
		UpdateError: database.SettingOr(ctx, q, settingsGroup, keyYTDLPUpdateErr, ""),
	}
}

// floorBytes is the free-space floor in bytes.
func (s Settings) floorBytes() int64 { return int64(s.MinFreeGB) << 30 }

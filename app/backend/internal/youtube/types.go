package youtube

import (
	"errors"
	"time"
)

// Follow kinds. A one-off video has no follow row, so KindVideo names only
// what a pasted link points at.
const (
	KindPlaylist = "playlist"
	KindChannel  = "channel"
	KindVideo    = "video"
)

// Video states.
const (
	StateQueued      = "queued"
	StateDownloading = "downloading"
	StateDone        = "done"
	StateFailed      = "failed"
	StateSkipped     = "skipped"
	StateRemoved     = "removed"
)

// Job kinds and the live topic. Stored names: never rename them.
const (
	KindCheck    = "youtube_check"
	KindDownload = "youtube_download"
	KindAdopt    = "youtube_adopt"
	LiveTopic    = "youtube"
)

// ErrDuplicateFollow is returned when a person already follows the same
// playlist or channel.
var ErrDuplicateFollow = errors.New("you already follow this")

// Follow is one playlist or channel a person follows.
type Follow struct {
	ID            int64  `db:"id" json:"id"`
	UserID        int64  `db:"user_id" json:"-"`
	URL           string `db:"url" json:"url"`
	Kind          string `db:"kind" json:"kind"`
	YTID          string `db:"yt_id" json:"yt_id"`
	Title         string `db:"title" json:"title"`
	MaxHeight     int    `db:"quality_max_height" json:"quality_max_height"`
	IncludeShorts bool   `db:"include_shorts" json:"include_shorts"`
	KeepLast      *int   `db:"keep_last" json:"keep_last"` // nil keeps every video
	Backfill      *int   `db:"backfill" json:"backfill"`   // nil takes every existing video
	Enabled       bool   `db:"enabled" json:"enabled"`
	// Mirror, for a playlist: videos that leave the playlist leave the
	// library too, and every video in it is taken.
	Mirror        bool       `db:"mirror" json:"mirror"`
	AddedAt       time.Time  `db:"added_at" json:"added_at"`
	LastCheckedAt *time.Time `db:"last_checked_at" json:"last_checked_at"`
	LastError     string     `db:"last_error" json:"last_error"`
}

// Video is one video kept for one person.
type Video struct {
	ID     int64 `db:"id" json:"id"`
	UserID int64 `db:"user_id" json:"-"`
	// FollowIDs are the follows that hold the video. Reads leave it empty
	// unless they say they fill it; InsertVideo links a new row to each.
	FollowIDs []int64 `json:"follow_ids"`
	// Manual is set once the person added the video by its own link: no
	// follow deletes it.
	Manual          bool       `db:"manual" json:"manual"`
	VideoID         string     `db:"video_id" json:"video_id"`
	Title           string     `db:"title" json:"title"`
	ChannelName     string     `db:"channel_name" json:"channel_name"`
	ChannelID       string     `db:"channel_id" json:"channel_id"`
	UploadedAt      *time.Time `db:"uploaded_at" json:"uploaded_at"`
	DurationSeconds int        `db:"duration_seconds" json:"duration_seconds"`
	MaxHeight       int        `db:"quality_max_height" json:"quality_max_height"`
	PositionSeconds int        `db:"position_seconds" json:"position_seconds"`
	RelativePath    string     `db:"relative_path" json:"-"` // relative to the YouTube folder, forward slashes
	SizeBytes       int64      `db:"size_bytes" json:"size_bytes"`
	State           string     `db:"state" json:"state"`
	Error           string     `db:"error" json:"error"`
	WatchedAt       *time.Time `db:"watched_at" json:"watched_at"`
	AddedAt         time.Time  `db:"added_at" json:"added_at"`
}

// FollowChange is a partial edit of a follow; nil fields stay as they are.
// KeepLast set to a nil *int turns keep_last off.
type FollowChange struct {
	MaxHeight     *int
	IncludeShorts *bool
	Enabled       *bool
	KeepLast      **int
	Mirror        *bool
}

// VideoFilter narrows a person's video list. Zero values do not filter;
// Limit 0 means no limit.
type VideoFilter struct {
	FollowID  int64
	ChannelID string
	Unwatched bool
	Limit     int
	Offset    int
}

// Totals is the whole library at a glance, for the owner and admins: no
// titles, only counts over downloaded videos.
type Totals struct {
	Videos int   `json:"videos"`
	Bytes  int64 `json:"bytes"`
	People int   `json:"people"`
}

// Meta is what a finished download tells us about its video.
type Meta struct {
	Title           string
	ChannelName     string
	ChannelID       string
	UploadedAt      *time.Time
	DurationSeconds int
}

// Package model holds the HolyArrs records (design/domain/model.md) as Go
// types. Field tags name the database columns; ",json" marks a JSON column.
package model

import (
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

// The row envelope lives in core/database; these names keep callers short.
type (
	Actor   = database.Actor
	Meta    = database.Meta
	Subject = database.Subject
	Event   = database.Event

	Job      = jobs.Job
	JobError = jobs.JobError
	Schedule = jobs.Schedule
)

// System is the actor for changes HolyArrs makes on its own.
var System = database.System

// PressingTrack is one track on a Pressing.
type PressingTrack struct {
	ID          string `json:"id"`           // MusicBrainz track id
	RecordingID string `json:"recording_id"` // the same song on every Pressing
	Disc        int    `json:"disc"`
	Number      int    `json:"number"`   // place on its disc
	Position    int    `json:"position"` // place on the whole Pressing
	Name        string `json:"name"`
	LengthMS    int    `json:"length_ms,omitempty"`
}

// AlbumPressing is one edition of an album (a MusicBrainz release). One per
// album is chosen; its tracks are the album's tracks.
type AlbumPressing struct {
	Meta
	PartID         int64           `db:"part_id"`
	MBID           string          `db:"mbid"`
	Name           string          `db:"name"`
	Status         string          `db:"status"`  // official, promotion, bootleg, pseudo-release, ""
	Country        string          `db:"country"` // as given, e.g. US, XW
	Date           string          `db:"date"`    // YYYY, YYYY-MM or YYYY-MM-DD
	Disambiguation string          `db:"disambiguation"`
	Formats        []string        `db:"formats,json"` // one per disc
	TrackCount     int             `db:"track_count"`
	Tracks         []PressingTrack `db:"tracks,json"`
	Chosen         bool            `db:"chosen"`
	ChosenBy       string          `db:"chosen_by"` // auto, person
	DroppedAt      *time.Time      `db:"dropped_at"`
}

// SourceUse says what a Source is used for.
type SourceUse struct {
	Feed       bool `json:"feed"`
	AutoSearch bool `json:"auto_search"`
	HandSearch bool `json:"hand_search"`
}

// SourceLimits caps how hard a Source is used.
type SourceLimits struct {
	SearchesPerWindow int `json:"searches_per_window,omitempty"`
	GrabsPerWindow    int `json:"grabs_per_window,omitempty"`
	WindowSeconds     int `json:"window_seconds,omitempty"`
	GapMS             int `json:"gap_ms,omitempty"`
	TimeoutMS         int `json:"timeout_ms,omitempty"`
}

// Source is a place HolyArrs searches for Releases.
type Source struct {
	Meta
	Name        string            `db:"name"`
	Protocol    string            `db:"protocol"` // torrent, usenet
	Format      string            `db:"format"`   // newznab, torznab, rss, bridge
	URL         string            `db:"url"`
	SecretRef   *int64            `db:"secret_ref"`
	UseFor      SourceUse         `db:"use_for,json"`
	Kinds       []string          `db:"kinds,json"`
	Categories  map[string][]int  `db:"categories,json"`
	MinSeeders  *int              `db:"min_seeders"`
	Limits      SourceLimits      `db:"limits,json"`
	ProxyID     *int64            `db:"proxy_id"`
	Priority    int               `db:"priority"`
	State       string            `db:"state"` // enabled, paused, disabled
	PauseReason *string           `db:"pause_reason"`
	PausedUntil *time.Time        `db:"paused_until"`
	Extra       map[string]string `db:"extra,json"`
	// AllowLocal lets this Source reach private, loopback, link-local and
	// CGNAT addresses (off by default; the lab's fake indexer needs it).
	AllowLocal bool `db:"allow_local_network"`
	// ProxyURL is an optional proxy (http, https or socks5) without its
	// password; the password is kept in the vault under ProxySecretRef.
	ProxyURL       string `db:"proxy_url"`
	ProxySecretRef *int64 `db:"proxy_secret_ref"`
	// ProxyAllowLocal lets the proxy itself be on the local network.
	ProxyAllowLocal bool `db:"proxy_allow_local_network"`
	// PathSecretRef holds the passkeys an rss address keeps in its path;
	// the stored URL has a marker in their place.
	PathSecretRef *int64 `db:"path_secret_ref"`
	// Tags: with any, this Source serves only titles that share one of them.
	Tags []string `db:"tags,json"`
}

// Release is one search result.
type Release struct {
	Meta
	SourceID       int64          `db:"source_id"`
	Name           string         `db:"name"`
	LinkRef        string         `db:"link_ref"`
	Protocol       string         `db:"protocol"`
	Size           int64          `db:"size"`
	PublishedAt    *time.Time     `db:"published_at"`
	Seeders        *int           `db:"seeders"`
	Peers          *int           `db:"peers"`
	InfoHash       *string        `db:"info_hash"`
	Flags          []string       `db:"flags,json"`
	Parsed         map[string]any `db:"parsed,json"`
	MatchedSlotIDs []int64        `db:"matched_slot_ids,json"`
	Verdict        map[string]any `db:"verdict,json"`
	FoundBy        string         `db:"found_by"` // feed, auto_search, hand_search
	JobID          *int64         `db:"job_id"`
	State          string         `db:"state"` // seen, judged, chosen, held, passed_over, refused, turned_down
	HoldUntil      *time.Time     `db:"hold_until"`
}

// PathMap maps a Downloader's path to ours.
type PathMap struct {
	Theirs string `json:"theirs"`
	Ours   string `json:"ours"`
}

// SeedGoal says when seeding is done.
type SeedGoal struct {
	Ratio   *float64 `json:"ratio,omitempty"`
	Minutes *int     `json:"minutes,omitempty"`
	// DeleteFiles: removing a torrent after seeding also deletes its download
	// files, only when that is safe.
	DeleteFiles bool `json:"delete_files,omitempty"`
}

// Downloader is an app that downloads for HolyArrs.
type Downloader struct {
	Meta
	Name      string `db:"name"`
	Adapter   string `db:"adapter"` // qbittorrent, sabnzbd
	Protocol  string `db:"protocol"`
	URL       string `db:"url"`
	SecretRef *int64 `db:"secret_ref"`
	Category  string `db:"category"`
	// Category for one kind of title; empty uses Category.
	CategoryMovies string    `db:"category_movies"`
	CategoryShows  string    `db:"category_shows"`
	CategoryMusic  string    `db:"category_music"`
	Priority       int       `db:"priority"`
	PathMap        []PathMap `db:"path_map,json"`
	AfterImport    string    `db:"after_import"` // leave, remove, recategorize
	SeedGoal       SeedGoal  `db:"seed_goal,json"`
	State          string    `db:"state"`    // ok, unreachable, auth_failed, disabled (paused by a person)
	Username       string    `db:"username"` // qBittorrent sign-in name; the password is the secret
	// AllowLocal lets HolyArrs reach this Downloader on this machine or the
	// local network.
	AllowLocal   bool       `db:"allow_local_network"`
	BackoffUntil *time.Time `db:"backoff_until"` // resting after failures until then
	FailCount    int        `db:"fail_count"`
	LastError    string     `db:"last_error"`
	CheckedAt    *time.Time `db:"checked_at"`
	Removed      bool       `db:"removed"` // removed by a person but kept for its Download history
	// Writable is "HolyArrs can change this Downloader"; off, HolyArrs only reads it.
	Writable bool `db:"writable"`
	// ClearFinished removes a Usenet job and its files from the client once
	// its Import fully succeeded.
	ClearFinished bool `db:"clear_finished"`
	// Tags: titles with one of these go only to Downloaders that have it.
	Tags []string `db:"tags,json"`
}

// CategoryFor returns the category for a title kind (movie, show, artist).
func (d Downloader) CategoryFor(kind string) string {
	c := ""
	switch kind {
	case "movie":
		c = d.CategoryMovies
	case "show":
		c = d.CategoryShows
	case "artist":
		c = d.CategoryMusic
	}
	if c == "" {
		return d.Category
	}
	return c
}

// Categories returns the main category and every set kind category, once each.
func (d Downloader) Categories() []string {
	out := []string{d.Category}
	for _, c := range []string{d.CategoryMovies, d.CategoryShows, d.CategoryMusic} {
		if c != "" && !slices.ContainsFunc(out, func(x string) bool { return strings.EqualFold(x, c) }) {
			out = append(out, c)
		}
	}
	return out
}

// SeenFile is one file a Download reports. Progress runs from 0 to 1 and
// Priority is the client's file priority (0 skipped); both are kept for a
// season pack fetched a few episodes at a time, where some files are skipped.
type SeenFile struct {
	Name     string  `json:"name"`
	Size     int64   `json:"size"`
	Progress float64 `json:"progress,omitempty"`
	Priority int     `json:"priority,omitempty"`
}

// PackFile is one episode file of a season pack fetched a few episodes at a
// time: its index in the torrent, its name, the Slots and episode Items it
// fills, their episode numbers, and whether it is switched on.
type PackFile struct {
	I     int     `json:"i"`
	Name  string  `json:"name"`
	Slots []int64 `json:"slots"`
	Items []int64 `json:"items"`
	Eps   []int   `json:"eps"`
	On    bool    `json:"on"`
}

// Download is one job in a Downloader.
type Download struct {
	Meta
	DownloaderID     int64      `db:"downloader_id"`
	ReleaseID        *int64     `db:"release_id"`
	SlotIDs          []int64    `db:"slot_ids,json"`
	ExternalID       string     `db:"external_id"`
	Progress         float64    `db:"progress"`
	Speed            int64      `db:"speed"`
	UpSpeed          int64      `db:"up_speed"`
	Seeds            int        `db:"seeds"`
	Peers            int        `db:"peers"`
	ETA              *int64     `db:"eta"`
	State            string     `db:"state"`
	StateReason      *string    `db:"state_reason"`
	FilesSeen        []SeenFile `db:"files_seen,json"`
	SavePath         string     `db:"save_path"`
	StartedAt        *time.Time `db:"started_at"`
	FinishedAt       *time.Time `db:"finished_at"`
	StartedBy        Actor      `db:"started_by,json"`
	ProgressAt       *time.Time `db:"progress_at"` // last time progress went up; the stall timers count from here
	ConfirmedEdition string     `db:"confirmed_edition"`
	// PackFiles is set (not nil) for a season pack fetched a few episodes at a
	// time, in episode order; empty while its file list is not known yet.
	PackFiles []PackFile `db:"pack_files,json"`
}

// User is a person who signs in: the one owner, admins and members.
type User struct {
	Meta
	Name         string `db:"name"`
	Role         string `db:"role"`          // owner, admin, member
	PasswordHash string `db:"password_hash"` // empty when they sign in only through Plex, Jellyfin or Emby
	State        string `db:"state"`         // active, locked, disabled
	Perms        string `db:"perms"`         // JSON list; the owner has every permission
	// Kid limit: the highest age rating a member may see and ask for (""
	// means no limit) and the country whose ratings apply (only US for now;
	// "" reads as US).
	MaxRating     string `db:"max_rating"`
	RatingCountry string `db:"rating_country"`
}

// Setting is one typed key-value entry in a setting group.
type Setting struct {
	Meta
	Group string `db:"grp"` // links, where_files_go, what_to_pick, this_app, alerts
	Key   string `db:"key"`
	Value string `db:"value"`
}

// OutsideList is a TMDB or Trakt list whose movies or shows are added on a schedule.
type OutsideList struct {
	Meta
	Name            string     `db:"name"`
	Kind            string     `db:"kind"`       // tmdb_list, trakt_list
	TitleKind       string     `db:"title_kind"` // movie, show
	Ref             string     `db:"ref"`        // TMDB list number, or Trakt "user/slug"
	LibraryFolderID int64      `db:"library_folder_id"`
	PickRulesID     *int64     `db:"pick_rules_id"`
	KeepUpToDate    bool       `db:"keep_up_to_date"`
	SearchOnAdd     bool       `db:"search_on_add"`
	ReleaseGate     string     `db:"release_gate"` // empty: the default
	Seasons         string     `db:"seasons"`      // shows: all, future, latest, first, none
	NewSeasons      string     `db:"new_seasons"`  // shows: all, none
	Enabled         bool       `db:"enabled"`
	SyncEveryHours  int        `db:"sync_every_hours"`
	LastSyncedAt    *time.Time `db:"last_synced_at"`
	LastResult      string     `db:"last_result"`
	LastError       string     `db:"last_error"`
}

// Ask states.
const (
	AskAsked      = "asked"
	AskApproved   = "approved"
	AskTurnedDown = "turned_down"
	AskAvailable  = "available"
)

// Ask is a person's ask for a movie or a show on the Requests page. It follows
// its Title from asked to approved (or turned down) to available.
type Ask struct {
	Meta
	Kind         string     `db:"kind"` // movie, show, artist
	TMDBID       string     `db:"tmdb_id"`
	Name         string     `db:"name"`
	Year         *int       `db:"year"`
	Poster       string     `db:"poster"`
	State        string     `db:"state"`
	TitleID      *int64     `db:"title_id"`
	AskedBy      int64      `db:"asked_by"`
	AskedAt      time.Time  `db:"asked_at"`
	DecidedBy    *int64     `db:"decided_by"`
	DecidedAt    *time.Time `db:"decided_at"`
	Note         string     `db:"note"`
	AutoApproved bool       `db:"auto_approved"`
	AvailableAt  *time.Time `db:"available_at"`
	// Ref is the id the Title is added by: tmdb:<n> for movies, any of
	// tmdb:, tvdb:, tvmaze:, imdb: for shows.
	Ref string `db:"ref"`
	// SourceIDs are every id known for it (finds the same show by any id).
	SourceIDs map[string]string `db:"source_ids,json"`
	// Seasons are the seasons asked for; empty is the whole show (and
	// always empty for movies).
	Seasons []int `db:"seasons,json"`
	// Episodes are single episodes asked for, by season, in seasons not
	// asked for whole. A show ask with no Seasons and no Episodes is the
	// whole show.
	Episodes []AskEpisodes `db:"episodes,json"`
	// FutureOnly asks a show for the seasons and episodes still to air.
	FutureOnly bool `db:"future_only"`
	// Albums are the albums of an artist asked for; empty is the whole
	// artist (and always empty for movies and shows).
	Albums []AskAlbum `db:"albums,json"`
	// FourK asks a movie for a 4K copy; approving it adds a 4K Version.
	FourK bool `db:"four_k"`
}

// Report states.
const (
	ReportOpen  = "open"
	ReportFixed = "fixed"
)

// ReportKinds are the problem kinds a Report can name, in the order screens
// show them.
var ReportKinds = []string{"picture", "sound", "subtitles", "wont_play", "other"}

// Report is a person's report of a problem with a title in the Library. It
// can narrow to an album, a season, an episode or track (ItemID) and a
// Version; FileID is the file that was there when it was made.
type Report struct {
	Meta
	TitleID   int64      `db:"title_id"`
	AlbumID   *int64     `db:"album_id"`
	Season    *int       `db:"season"`
	ItemID    *int64     `db:"item_id"`
	VersionID *int64     `db:"version_id"`
	FileID    *int64     `db:"file_id"`
	Kind      string     `db:"kind"` // picture, sound, subtitles, wont_play, other
	Note      string     `db:"note"`
	State     string     `db:"state"` // open, fixed
	MadeBy    *int64     `db:"made_by"`
	ClosedBy  *int64     `db:"closed_by"`
	ClosedAt  *time.Time `db:"closed_at"`
	Reply     string     `db:"reply"` // the optional reply sent to the reporter when fixed
}

// AskEpisodes are the episodes of one season asked for (episode numbers).
type AskEpisodes struct {
	Season   int   `json:"season"`
	Episodes []int `json:"episodes"`
}

// AskAlbum is one album asked for: its MusicBrainz release-group id, name
// and year.
type AskAlbum struct {
	MBID string `json:"mbid"`
	Name string `json:"name"`
	Year int    `json:"year,omitempty"`
	// Edition is the release (MusicBrainz id) picked on the album page; empty: the usual choice.
	Edition string `json:"edition,omitempty"`
	// Songs are the songs (MusicBrainz recording ids) the approver kept of
	// the album; empty keeps every song. SongsDone: the pick is applied to
	// the Library's tracks (they come with the album's refresh).
	Songs     []string `json:"songs,omitempty"`
	SongsDone bool     `json:"songs_done,omitempty"`
}

// Alert is an outgoing message channel: a Discord link or an email server.
type Alert struct {
	Meta
	Name             string   `db:"name"`
	Kind             string   `db:"kind"` // discord, email, webhook, telegram, ntfy, gotify, pushover
	Enabled          bool     `db:"enabled"`
	Moments          []string `db:"moments,json"`
	URLSecretID      *int64   `db:"url_secret_id"`
	SMTPHost         string   `db:"smtp_host"`
	SMTPPort         int      `db:"smtp_port"`
	SMTPSecurity     string   `db:"smtp_security"` // starttls, tls, none
	SMTPFrom         string   `db:"smtp_from"`
	SMTPUser         string   `db:"smtp_user"`
	PasswordSecretID *int64   `db:"password_secret_id"`
	StaffTo          string   `db:"staff_to"`
	// AllowLocal lets this Alert reach this machine or the local network
	// (and a Discord link off discord.com).
	AllowLocal bool       `db:"allow_local_network"`
	LastSentAt *time.Time `db:"last_sent_at"`
	LastError  string     `db:"last_error"`
	// WebhookAuth says what a webhook does with its secret: sign (an HMAC
	// of each body), bearer (an Authorization header) or none.
	WebhookAuth string `db:"webhook_auth"`
	// ChatID is the Telegram chat a telegram Alert writes to: a number
	// (negative for groups) or @channelname.
	ChatID string `db:"chat_id"`
	// ServerURL is an ntfy or Gotify server (http or https, may hold a path).
	ServerURL string `db:"server_url"`
	// Topic is the ntfy topic a ntfy Alert posts to.
	Topic string `db:"topic"`
	// UserKey is the Pushover user (or group) key a pushover Alert writes to.
	UserKey string `db:"user_key"`
}

// HealthCheck is the latest result of one health check about one thing.
type HealthCheck struct {
	ID        int64     `db:"id" json:"id"`
	Key       string    `db:"check_key" json:"key"` // e.g. downloader:3, backups
	Kind      string    `db:"kind" json:"kind"`     // library_folder, source, downloader, disk_space, backups, update
	Subject   string    `db:"subject" json:"subject"`
	State     string    `db:"state" json:"state"` // ok, warning, error
	Message   string    `db:"message" json:"message"`
	Since     time.Time `db:"since" json:"since"`
	CheckedAt time.Time `db:"checked_at" json:"checked_at"`
	Streak    int       `db:"streak" json:"-"`
	Confirmed string    `db:"confirmed" json:"confirmed"` // the state Alerts were told about; Home shows this one
}

// MediaServer is a Plex, Jellyfin or Emby server HolyArrs tells to look at
// a movie folder again after an import, rename or delete.
type MediaServer struct {
	Meta
	Name          string     `db:"name"`
	Kind          string     `db:"kind"` // plex, jellyfin, emby
	URL           string     `db:"url"`
	TokenSecretID *int64     `db:"token_secret_id"`
	Enabled       bool       `db:"enabled"`
	OnImport      bool       `db:"on_import"`
	OnRename      bool       `db:"on_rename"`
	OnDelete      bool       `db:"on_delete"`
	PathFrom      string     `db:"path_from"` // HolyArrs' start of a path ...
	PathTo        string     `db:"path_to"`   // ... as the media server sees it
	AllowLocal    bool       `db:"allow_local_network"`
	LastOKAt      *time.Time `db:"last_ok_at"`
	LastError     string     `db:"last_error"`
}

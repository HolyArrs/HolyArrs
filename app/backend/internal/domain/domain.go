// Package domain holds the HolyArrs records (design/domain/model.md) as Go
// types. Field tags name the database columns; ",json" marks a JSON column.
package domain

import "time"

// Actor says who made a change: a user, a job, a linked app or the system.
type Actor struct {
	Kind string `json:"kind"` // user, job, link, system
	ID   int64  `json:"id,omitempty"`
}

// System is the actor for changes HolyArrs makes on its own.
var System = Actor{Kind: "system"}

// Meta is on every record.
type Meta struct {
	ID        int64     `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	CreatedBy Actor     `db:"created_by,json"`
	UpdatedAt time.Time `db:"updated_at"`
	UpdatedBy Actor     `db:"updated_by,json"`
}

// LibraryFolder is a folder on disk where finished files of one kind are kept.
type LibraryFolder struct {
	Meta
	Path             string     `db:"path"`
	Kind             string     `db:"kind"` // movie, show, music
	Label            string     `db:"label"`
	DefaultPickRules *int64     `db:"default_pick_rules_id"`
	FreeSpace        int64      `db:"free_space"`
	Health           string     `db:"health"` // ok, missing, read_only, no_hardlink, unknown
	HealthCheckedAt  *time.Time `db:"health_checked_at"`
	SameDiskAs       []int64    `db:"same_disk_as,json"`
	HealthReason     string     `db:"health_reason"`
}

// Title is one movie, show or artist in the Library.
type Title struct {
	Meta
	Kind             string            `db:"kind"` // movie, show, artist
	Name             string            `db:"name"`
	SortName         string            `db:"sort_name"`
	OriginalName     *string           `db:"original_name"`
	Year             *int              `db:"year"`
	Overview         *string           `db:"overview"`
	OriginalLanguage *string           `db:"original_language"`
	Aliases          []string          `db:"aliases,json"`
	SourceIDs        map[string]string `db:"source_ids,json"`
	MetadataSource   string            `db:"metadata_source"` // tmdb, tvmaze, tvdb, musicbrainz
	EpisodeOrder     *string           `db:"episode_order"`
	KeepUpToDate     bool              `db:"keep_up_to_date"`
	NewPartsPolicy   string            `db:"new_parts_policy"` // all, future_only, none
	ReleaseGate      string            `db:"release_gate"`     // announced, in_cinemas, released
	GateDelayDays    int               `db:"gate_delay_days"`
	MusicTypes       []string          `db:"music_types,json"`
	WhyAdded         string            `db:"why_added"`
	Artwork          []string          `db:"artwork,json"`
	RefreshedAt      *time.Time        `db:"refreshed_at"`
	Status           string            `db:"status"` // announced, running, ended, released
	State            string            `db:"state"`  // added, looked_up, active, removed, lookup_failed
	// Release dates from the metadata source (movies).
	CinemaDate   *time.Time `db:"cinema_date"`
	DigitalDate  *time.Time `db:"digital_date"`
	PhysicalDate *time.Time `db:"physical_date"`
	// GateDriver names the date that drove the Release gate; GateOpensAt is
	// when searching may start (nil: never, until dates are known).
	GateDriver  string     `db:"gate_driver"` // announced, cinema, digital, physical, cinema_plus_90, none
	GateOpensAt *time.Time `db:"gate_opens_at"`
	// DroppedAt is set when the metadata source no longer knows the Title.
	DroppedAt *time.Time `db:"dropped_at"`
	// Shows: the season choice made at add time (all, future, latest,
	// none), and where and when the show airs.
	SeasonChoice *string `db:"season_choice"`
	// FutureFrom is set for Future seasons only: episodes that aired by
	// then are not wanted when a refresh finds them.
	FutureFrom *time.Time `db:"future_from"`
	Network    *string    `db:"network"`
	AirTime    *string    `db:"air_time"`  // local, HH:MM
	TimeZone   *string    `db:"time_zone"` // IANA name
	// Artists: the album choice made at add time (all, future, missing,
	// latest, first, none), the extra album kinds and Pressing statuses
	// that count (MusicTypes holds the main kinds), and details.
	AlbumChoice    *string  `db:"album_choice"`
	MusicSecondary []string `db:"music_secondary,json"`
	MusicStatuses  []string `db:"music_statuses,json"`
	ArtistType     *string  `db:"artist_type"`
	Disambiguation *string  `db:"disambiguation"`
	// Details for screens, from the metadata source: wide picture and logo
	// URLs, genre names, runtime in minutes, the US rating and the people
	// (top cast, then directors).
	Backdrop       string   `db:"backdrop"`
	Logo           string   `db:"logo"`
	Genres         []string `db:"genres,json"`
	RuntimeMinutes int      `db:"runtime_minutes"`
	Rating         string   `db:"rating"`
	People         []string `db:"people,json"`
	// Directors are a movie's directors alone (People mixes them in after the cast).
	Directors []string `db:"directors,json"`
	// Studio is a movie's first production company.
	Studio string `db:"studio"`
	// Tags route this title's grabs and searches (tagset.Allows).
	Tags []string `db:"tags,json"`
}

// NeverAdd is one entry of the Never-add list.
type NeverAdd struct {
	Meta
	Kind      string            `db:"kind"` // movie, show, artist
	SourceIDs map[string]string `db:"source_ids,json"`
	Name      string            `db:"name"`
	Reason    string            `db:"reason"`
}

// Pressing is one specific release of an album.
type Pressing struct {
	Name       string `json:"name"`
	TrackCount int    `json:"track_count"`
	Format     string `json:"format"`
	Country    string `json:"country"`
	SourceID   string `json:"source_id"`
	Preferred  bool   `json:"preferred"`
}

// Part is a season of a show or an album of an artist.
type Part struct {
	Meta
	TitleID     int64      `db:"title_id"`
	Kind        string     `db:"kind"` // season, album
	Number      *int       `db:"number"`
	Name        string     `db:"name"`
	ReleaseType *string    `db:"release_type"`
	ReleaseDate *time.Time `db:"release_date"`
	// ReleaseDatePrecision: day, month or year (albums; nil when unknown).
	ReleaseDatePrecision *string           `db:"release_date_precision"`
	KeepUpToDate         bool              `db:"keep_up_to_date"`
	SourceIDs            map[string]string `db:"source_ids,json"`
	Pressings            []Pressing        `db:"pressings,json"`
	DroppedAt            *time.Time        `db:"dropped_at"` // the details source no longer lists the season or album
	// Albums: extra kinds (live, compilation, ...), whether the album passes
	// the artist's album kinds, and the note telling same-named albums apart.
	SecondaryTypes []string `db:"secondary_types,json"`
	Included       bool     `db:"included"`
	Disambiguation *string  `db:"disambiguation"`
}

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

// Numbers locates an Item inside its Title.
type Numbers struct {
	Season     *int `json:"season,omitempty"`
	Episode    *int `json:"episode,omitempty"`
	Absolute   *int `json:"absolute,omitempty"`
	AltSeason  *int `json:"alt_season,omitempty"`
	AltEpisode *int `json:"alt_episode,omitempty"`
	Disc       *int `json:"disc,omitempty"`
	Track      *int `json:"track,omitempty"`
}

// Item is the smallest wanted unit. A movie is its own single Item.
type Item struct {
	Meta
	TitleID        int64      `db:"title_id"`
	PartID         *int64     `db:"part_id"`
	Kind           string     `db:"kind"` // movie, episode, track
	Numbers        Numbers    `db:"numbers,json"`
	Name           *string    `db:"name"`
	AirDate        *time.Time `db:"air_date"`
	Runtime        *int       `db:"runtime"`
	KeepUpToDate   bool       `db:"keep_up_to_date"`
	LastSearchedAt *time.Time `db:"last_searched_at"`
	RecordingID    *string    `db:"recording_id"`
	// Episodes: the local air day (daily order key; AirDate is the exact
	// UTC time when AirTimed), per-source ids, and when the source stopped
	// listing it.
	AirDay *string `db:"air_day"`
	// Episodes: AirDate is the exact air time; false when only the day is
	// known and AirDate stands in for the day's end.
	AirTimed  bool              `db:"air_timed"`
	SourceIDs map[string]string `db:"source_ids,json"`
	DroppedAt *time.Time        `db:"dropped_at"`
	// Tracks: the length in milliseconds.
	LengthMS *int `db:"length_ms"`
	// Episodes: the still picture URL from the metadata source.
	Still string `db:"still"`
}

// Version is one wanted copy of a Title, such as "HD" or "4K".
type Version struct {
	Meta
	TitleID         int64    `db:"title_id"`
	Label           string   `db:"label"`
	PickRulesID     int64    `db:"pick_rules_id"`
	LibraryFolderID int64    `db:"library_folder_id"`
	FolderName      string   `db:"folder_name"`
	KeepUpToDate    bool     `db:"keep_up_to_date"`
	IsMain          bool     `db:"is_main"`
	Edition         string   `db:"edition"`
	AudioLanguages  []string `db:"audio_languages,json"`
}

// Slot is one Item in one Version; the core loop state lives here.
type Slot struct {
	Meta
	ItemID           int64      `db:"item_id"`
	VersionID        int64      `db:"version_id"`
	State            SlotState  `db:"state"`
	FileID           *int64     `db:"file_id"`
	GoodEnough       bool       `db:"good_enough"`
	HeldReleaseID    *int64     `db:"held_release_id"`
	ChosenReleaseID  *int64     `db:"chosen_release_id"`
	ActiveDownloadID *int64     `db:"active_download_id"`
	LastReason       *string    `db:"last_reason"`
	NextTryAt        *time.Time `db:"next_try_at"`
}

// PickRules is one named rule set. The rule body is a JSON column.
type PickRules struct {
	Meta
	Name     string        `db:"name"`
	Kinds    []string      `db:"kinds,json"`
	Revision int           `db:"revision"`
	Preset   bool          `db:"preset"`
	Body     PickRulesBody `db:"body,json"`
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
}

// Downloader is an app that downloads for HolyArrs.
type Downloader struct {
	Meta
	Name        string    `db:"name"`
	Adapter     string    `db:"adapter"` // qbittorrent, sabnzbd
	Protocol    string    `db:"protocol"`
	URL         string    `db:"url"`
	SecretRef   *int64    `db:"secret_ref"`
	Category    string    `db:"category"`
	Priority    int       `db:"priority"`
	PathMap     []PathMap `db:"path_map,json"`
	AfterImport string    `db:"after_import"` // leave, remove, recategorize
	SeedGoal    SeedGoal  `db:"seed_goal,json"`
	State       string    `db:"state"`    // ok, unreachable, auth_failed, disabled (paused by a person)
	Username    string    `db:"username"` // qBittorrent sign-in name; the password is the secret
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

// SeenFile is one file a Download reports.
type SeenFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
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
}

// PlanStep is one file in an Import plan.
type PlanStep struct {
	SourceFile     string   `json:"source_file"`
	TargetSlotIDs  []int64  `json:"target_slot_ids"`
	TargetPath     string   `json:"target_path"`
	Method         string   `json:"method"` // hardlink, move, copy
	ReplacesFileID *int64   `json:"replaces_file_id,omitempty"`
	Confidence     float64  `json:"confidence"`
	Why            []string `json:"why"`
}

// PlaceResult is the outcome for one file of an Import.
type PlaceResult struct {
	SourceFile string `json:"source_file"`
	Outcome    string `json:"outcome"`
	PlacedBy   string `json:"placed_by"`
	FellBack   bool   `json:"fell_back"`
	Error      string `json:"error,omitempty"`
	HeldAside  string `json:"held_aside,omitempty"` // where the replaced copy was moved before this file was placed
	// HeldAsideMore lists where other replaced copies went (an episode
	// file that replaces several, such as two single episodes).
	HeldAsideMore []string `json:"held_aside_more,omitempty"`
}

// Import is one attempt to place files into the library.
type Import struct {
	Meta
	DownloadID *int64        `db:"download_id"`
	FromPath   string        `db:"from_path"`
	Kind       string        `db:"kind"`
	Plan       []PlanStep    `db:"plan,json"`
	Mode       string        `db:"mode"` // auto, dry_run, by_hand
	Result     []PlaceResult `db:"result,json"`
	DecidedBy  *Actor        `db:"decided_by,json"`
	State      string        `db:"state"`
}

// Quality is our parsed form of a file's quality.
type Quality struct {
	Resolution int      `json:"resolution,omitempty"`
	Source     string   `json:"source,omitempty"`
	Codec      string   `json:"codec,omitempty"`
	HDR        []string `json:"hdr,omitempty"`
	Audio      string   `json:"audio,omitempty"`
}

// File is a file on disk that HolyArrs knows about.
type File struct {
	Meta
	Protected    bool           `db:"protected"`
	SlotIDs      []int64        `db:"slot_ids,json"`
	VersionID    int64          `db:"version_id"`
	RelativePath string         `db:"relative_path"`
	Size         int64          `db:"size"`
	AddedAt      time.Time      `db:"added_at"`
	Quality      Quality        `db:"quality,json"`
	Languages    []string       `db:"languages,json"`
	MediaInfo    map[string]any `db:"media_info,json"`
	ReleaseName  *string        `db:"release_name"`
	Group        *string        `db:"release_group"`
	Origin       string         `db:"origin"`    // imported, adopted, found
	PlacedBy     string         `db:"placed_by"` // hardlink, move, copy, none
	FellBack     bool           `db:"fell_back"`
	Fingerprint  *string        `db:"fingerprint"`
	CheckedAt    *time.Time     `db:"checked_at"`
	Role         string         `db:"role"` // main, subtitle, artwork, info, lyrics
	ParentFileID *int64         `db:"parent_file_id"`
	State        string         `db:"state"` // present, missing, gone, replaced
}

// Subject is what a Job is about.
type Subject struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
}

// JobError says why a Job failed.
type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Job is background work stored in the database.
type Job struct {
	Meta
	Kind         string         `db:"kind"`
	Subject      *Subject       `db:"subject,json"`
	Params       map[string]any `db:"params,json"`
	Priority     string         `db:"priority"` // now, normal, idle
	State        string         `db:"state"`    // queued, running, done, failed, cancelled
	Progress     float64        `db:"progress"`
	ProgressNote *string        `db:"progress_note"`
	Attempt      int            `db:"attempt"`
	MaxAttempts  int            `db:"max_attempts"`
	RunAfter     time.Time      `db:"run_after"`
	StartedAt    *time.Time     `db:"started_at"`
	EndedAt      *time.Time     `db:"ended_at"`
	Result       map[string]any `db:"result,json"`
	Error        *JobError      `db:"error,json"`
	AskedBy      Actor          `db:"asked_by,json"`
	DedupeKey    *string        `db:"dedupe_key"`
}

// Schedule runs a Job kind on a timer.
type Schedule struct {
	Meta
	Kind         string     `db:"kind"`
	EverySeconds int64      `db:"every_seconds"`
	LastRun      *time.Time `db:"last_run"`
	NextRun      *time.Time `db:"next_run"`
	Enabled      bool       `db:"enabled"`
}

// Event is one line of history. Events live in the events database.
type Event struct {
	Meta
	At         time.Time      `db:"at"`
	Actor      Actor          `db:"actor,json"`
	Kind       string         `db:"kind"`
	TitleID    *int64         `db:"title_id"`
	SlotID     *int64         `db:"slot_id"`
	DownloadID *int64         `db:"download_id"`
	JobID      *int64         `db:"job_id"`
	Summary    string         `db:"summary"`
	Data       map[string]any `db:"data,json"`
	GroupKey   string         `db:"group_key"`
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

package mediafiles

import (
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// LibraryFolder is a folder on disk where finished files of one kind are kept.
type LibraryFolder struct {
	database.Meta
	Path             string     `db:"path"`
	Kind             string     `db:"kind"` // movie, show, music
	Label            string     `db:"label"`
	DefaultPickRules *int64     `db:"default_pick_rules_id"`
	FreeSpace        int64      `db:"free_space"`
	Health           string     `db:"health"` // ok, missing, read_only, no_hardlink, unknown
	HealthCheckedAt  *time.Time `db:"health_checked_at"`
	SameDiskAs       []int64    `db:"same_disk_as,json"`
	HealthReason     string     `db:"health_reason"`
	CopiesExpected   bool       `db:"copies_expected"` // copying without hardlinks is wanted
}

// Title is one movie, show or artist in the Library.
type Title struct {
	database.Meta
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
	// Score is the members' average out of 10 (nil until the first lookup
	// after scores were kept; 0 when the source has none) and Votes how many voted.
	Score *float64 `db:"score"`
	Votes int      `db:"votes"`
	// Directors are a movie's directors alone (People mixes them in after the cast).
	Directors []string `db:"directors,json"`
	// Studio is a movie's first production company.
	Studio string `db:"studio"`
	// Tags route this title's grabs and searches (Allows).
	Tags []string `db:"tags,json"`
}

// NeverAdd is one entry of the Never-add list.
type NeverAdd struct {
	database.Meta
	Kind      string            `db:"kind"` // movie, show, artist
	SourceIDs map[string]string `db:"source_ids,json"`
	Name      string            `db:"name"`
	Reason    string            `db:"reason"`
}

// Part is a season of a show or an album of an artist.
type Part struct {
	database.Meta
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
	database.Meta
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
	database.Meta
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
	database.Meta
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
	database.Meta
	Name     string        `db:"name"`
	Kinds    []string      `db:"kinds,json"`
	Revision int           `db:"revision"`
	Preset   bool          `db:"preset"`
	Body     PickRulesBody `db:"body,json"`
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
	database.Meta
	DownloadID *int64          `db:"download_id"`
	FromPath   string          `db:"from_path"`
	Kind       string          `db:"kind"`
	Plan       []PlanStep      `db:"plan,json"`
	Mode       string          `db:"mode"` // auto, dry_run, by_hand
	Result     []PlaceResult   `db:"result,json"`
	DecidedBy  *database.Actor `db:"decided_by,json"`
	State      string          `db:"state"`
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
	database.Meta
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

// Pressing is one specific release of an album.
type Pressing struct {
	Name       string `json:"name"`
	TrackCount int    `json:"track_count"`
	Format     string `json:"format"`
	Country    string `json:"country"`
	SourceID   string `json:"source_id"`
	Preferred  bool   `json:"preferred"`
}

package migrate

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

// Snapshot is what an old app holds, as plain data. It never holds a
// secret: a key, password, token or cookie the app had is named in
// NeedsReentry instead, for the owner to type again during the move.
type Snapshot struct {
	Kind    Kind      `json:"kind"`
	Via     string    `json:"via"`
	Version string    `json:"version"`
	Schema  int       `json:"schema,omitempty"`
	ReadAt  time.Time `json:"read_at"`

	// Library (Radarr movies, Sonarr shows, Lidarr artists).
	Titles []Title `json:"titles,omitempty"`
	Tags   []Tag   `json:"tags,omitempty"`
	// Folders are the app's library folders.
	Folders []Folder `json:"folders,omitempty"`

	// Rules.
	QualityProfiles []Profile `json:"quality_profiles,omitempty"`
	CustomFormats   []Named   `json:"custom_formats,omitempty"`
	// ReleaseProfiles are release word rules: words a release must or must
	// not hold, with their tag and indexer scope.
	ReleaseProfiles  []Named          `json:"release_profiles,omitempty"`
	DelayProfiles    []DelayProfile   `json:"delay_profiles,omitempty"`
	Naming           map[string]any   `json:"naming,omitempty"`
	MetadataProfiles []Named          `json:"metadata_profiles,omitempty"`
	LanguageProfiles []Named          `json:"language_profiles,omitempty"`
	Exclusions       []Exclusion      `json:"exclusions,omitempty"`
	Subtitles        *SubtitleSetup   `json:"subtitles,omitempty"`
	AppProfiles      []map[string]any `json:"app_profiles,omitempty"`

	// Links to other services (Sources, Downloaders, lists, Alerts).
	Indexers        []Provider `json:"indexers,omitempty"`
	DownloadClients []Provider `json:"download_clients,omitempty"`
	ImportLists     []Provider `json:"import_lists,omitempty"`
	Notifications   []Provider `json:"notifications,omitempty"`
	Metadata        []Provider `json:"metadata,omitempty"`
	// AppLinks are Prowlarr's links to the apps it fills.
	AppLinks []Provider `json:"app_links,omitempty"`
	// IndexerProxies are Prowlarr's proxies, each used by the indexers
	// sharing one of its tags.
	IndexerProxies []Provider `json:"indexer_proxies,omitempty"`
	// Downloader is qBittorrent's or SABnzbd's own connection details.
	Downloader *Downloader `json:"downloader,omitempty"`
	// RemotePathMappings are Sonarr's, Radarr's and Lidarr's path rewrites
	// for a download client: the folder as the client on host reports it,
	// and the same folder as the app sees it.
	RemotePathMappings []RemotePathMapping `json:"remote_path_mappings,omitempty"`

	// Seerr and Overseerr.
	Users    []User    `json:"users,omitempty"`
	Requests []Request `json:"requests,omitempty"`

	// Settings are app-wide plain settings (Seerr's default limits,
	// Radarr's availability delay).
	Settings map[string]any `json:"settings,omitempty"`

	// NeedsReentry names every secret left behind ("indexer \"Nyaa\": apiKey").
	NeedsReentry []string `json:"needs_reentry,omitempty"`
	// Notes are things the reader skipped or could not tell.
	Notes []string `json:"notes,omitempty"`
}

// RemotePathMapping is one path rewrite for the download client on Host.
type RemotePathMapping struct {
	Host       string `json:"host"`
	RemotePath string `json:"remote_path"`
	LocalPath  string `json:"local_path"`
}

// Title is one movie, show or artist.
type Title struct {
	SourceID int64             `json:"source_id"`
	Kind     string            `json:"kind"` // movie, show, artist
	Name     string            `json:"name"`
	Year     int               `json:"year,omitempty"`
	IDs      map[string]string `json:"ids,omitempty"` // tmdb, tvdb, imdb, musicbrainz
	Path     string            `json:"path,omitempty"`
	Folder   string            `json:"folder,omitempty"`
	// Monitored is the app's "watch for this" flag.
	Monitored bool `json:"monitored"`
	// Profile is the quality profile's name (ProfileID its id there).
	Profile   string   `json:"profile,omitempty"`
	ProfileID int64    `json:"profile_id,omitempty"`
	Added     string   `json:"added,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Seasons   []Season `json:"seasons,omitempty"`
	// MusicAlbums keeps per-album monitoring and the selected pressing for
	// Lidarr artists. It contains public MusicBrainz identifiers only.
	MusicAlbums []MusicAlbum `json:"music_albums,omitempty"`
	// Extra holds app-specific plain settings (minimum availability,
	// show type, new-item rule, metadata profile).
	Extra map[string]any `json:"extra,omitempty"`
	// Files are the title's files with the quality the old app recorded.
	Files []TitleFile `json:"files,omitempty"`
}

// MusicAlbum is one Lidarr album belonging to an artist.
type MusicAlbum struct {
	MBID           string `json:"mbid"`
	Monitored      bool   `json:"monitored"`
	WatchedEdition string `json:"watched_edition,omitempty"`
}

// Season is one season of a show and whether it is watched.
type Season struct {
	Number    int  `json:"number"`
	Monitored bool `json:"monitored"`
}

// Tag is one tag.
type Tag struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

// Folder is one library folder.
type Folder struct {
	Path  string         `json:"path"`
	Extra map[string]any `json:"extra,omitempty"`
}

// Named is a named rule kept as plain settings.
type Named struct {
	ID       int64          `json:"id,omitempty"`
	Name     string         `json:"name"`
	Settings map[string]any `json:"settings,omitempty"`
	// Items are a rule's parts (a custom format's conditions), plain values only.
	Items []map[string]any `json:"items,omitempty"`
}

// Profile is a quality profile: its name, cutoff and allowed qualities.
type Profile struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	UpgradeAllowed bool     `json:"upgrade_allowed"`
	Cutoff         string   `json:"cutoff,omitempty"`
	Allowed        []string `json:"allowed,omitempty"`
	MinScore       int      `json:"min_score,omitempty"`
	CutoffScore    int      `json:"cutoff_score,omitempty"`
	// Scores maps custom format names to their score in this profile.
	Scores map[string]int `json:"scores,omitempty"`
}

// DelayProfile is a wait-before-grab rule.
type DelayProfile struct {
	ID                int64    `json:"id"`
	EnableUsenet      bool     `json:"enable_usenet"`
	EnableTorrent     bool     `json:"enable_torrent"`
	PreferredProtocol string   `json:"preferred_protocol,omitempty"`
	UsenetDelay       int      `json:"usenet_delay"`
	TorrentDelay      int      `json:"torrent_delay"`
	Order             int      `json:"order"`
	Tags              []string `json:"tags,omitempty"`
	// BypassIfHighestQuality skips the wait at the top allowed quality;
	// BypassIfAboveScore skips it at a format score (a snapshot kept before
	// they were read has both false).
	BypassIfHighestQuality bool `json:"bypass_if_highest_quality,omitempty"`
	BypassIfAboveScore     bool `json:"bypass_if_above_score,omitempty"`
}

// Exclusion is a title the app was told never to add.
type Exclusion struct {
	IDs  map[string]string `json:"ids"`
	Name string            `json:"name,omitempty"`
	Year int               `json:"year,omitempty"`
}

// Provider is one link to another service (indexer, download client,
// list, notification, metadata writer, Prowlarr app link, usenet server).
// Settings holds only plain values; secrets are named in NeedsReentry.
type Provider struct {
	ID           int64          `json:"id,omitempty"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind"` // the app's implementation name
	Enabled      bool           `json:"enabled"`
	Priority     int            `json:"priority,omitempty"`
	Protocol     string         `json:"protocol,omitempty"`
	Tags         []string       `json:"tags,omitempty"`
	Settings     map[string]any `json:"settings,omitempty"`
	NeedsReentry []string       `json:"needs_reentry,omitempty"`
}

// SubtitleSetup is Bazarr's: languages, language profiles and sites.
type SubtitleSetup struct {
	Languages []string          `json:"languages,omitempty"` // enabled, two-letter codes where known
	Profiles  []SubtitleProfile `json:"profiles,omitempty"`
	Providers []Provider        `json:"providers,omitempty"`
	Defaults  map[string]any    `json:"defaults,omitempty"`
}

// SubtitleProfile is one Bazarr language profile.
type SubtitleProfile struct {
	ID        int64              `json:"id"`
	Name      string             `json:"name"`
	Cutoff    string             `json:"cutoff,omitempty"`
	Languages []SubtitleLanguage `json:"languages,omitempty"`
	Tag       string             `json:"tag,omitempty"`
}

// SubtitleLanguage is one language in a Bazarr profile.
type SubtitleLanguage struct {
	Language string `json:"language"`
	Forced   bool   `json:"forced"`
	HI       bool   `json:"hi"`
}

// Downloader is qBittorrent's or SABnzbd's own connection details: never
// the password or key ("needs your password").
type Downloader struct {
	Host          string     `json:"host,omitempty"`
	Port          int        `json:"port,omitempty"`
	URLBase       string     `json:"url_base,omitempty"`
	TLS           bool       `json:"tls"`
	Username      string     `json:"username,omitempty"`
	NeedsPassword bool       `json:"needs_password"`
	SavePath      string     `json:"save_path,omitempty"`
	CompletePath  string     `json:"complete_path,omitempty"`
	Categories    []Category `json:"categories,omitempty"`
	// Servers are SABnzbd's usenet servers (passwords left behind).
	Servers []Provider `json:"servers,omitempty"`
}

// Category is one download category and its folder.
type Category struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// User is one Seerr or Overseerr person: name, email, role and limits
// only (no avatar, no sign-in tokens or hashes).
type User struct {
	SourceID int64 `json:"source_id"`
	// ExternalID is the stable identity supplied by the person's sign-in
	// provider. It is used only to keep move references stable; a move never
	// creates a sign-in link or grants access.
	ExternalID string `json:"external_id,omitempty"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	Kind       string `json:"kind,omitempty"` // plex, jellyfin, local, emby
	Admin      bool   `json:"admin"`
	// Owner says the app marks this person as its owner.
	Owner       bool     `json:"owner,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	// Limits: asks per Days. LimitSet distinguishes an explicit source zero,
	// whose semantics are not assumed, from an absent/inherited limit.
	MovieLimit    int    `json:"movie_limit,omitempty"`
	MovieDays     int    `json:"movie_days,omitempty"`
	MovieLimitSet bool   `json:"movie_limit_set,omitempty"`
	ShowLimit     int    `json:"show_limit,omitempty"`
	ShowDays      int    `json:"show_days,omitempty"`
	ShowLimitSet  bool   `json:"show_limit_set,omitempty"`
	Created       string `json:"created,omitempty"`
}

// Request is one Seerr or Overseerr ask.
type Request struct {
	SourceID  int64             `json:"source_id"`
	Kind      string            `json:"kind"` // movie, show
	IDs       map[string]string `json:"ids"`  // tmdb, tvdb, imdb
	State     string            `json:"state"`
	Seasons   []int             `json:"seasons,omitempty"`
	By        int64             `json:"by"` // User.SourceID
	ByName    string            `json:"by_name,omitempty"`
	DecidedBy int64             `json:"decided_by,omitempty"`
	FourK     bool              `json:"four_k,omitempty"`
	Created   string            `json:"created,omitempty"`
	Updated   string            `json:"updated,omitempty"`
}

// Counts sums the Snapshot for lists and screens.
func (s *Snapshot) Counts() map[string]int {
	c := map[string]int{
		"titles": len(s.Titles), "tags": len(s.Tags), "folders": len(s.Folders),
		"quality_profiles": len(s.QualityProfiles), "custom_formats": len(s.CustomFormats),
		"delay_profiles": len(s.DelayProfiles), "indexers": len(s.Indexers),
		"download_clients": len(s.DownloadClients), "import_lists": len(s.ImportLists),
		"notifications": len(s.Notifications), "app_links": len(s.AppLinks),
		"users": len(s.Users), "requests": len(s.Requests), "needs_reentry": len(s.NeedsReentry),
	}
	if s.Subtitles != nil {
		c["subtitle_languages"] = len(s.Subtitles.Languages)
		c["subtitle_profiles"] = len(s.Subtitles.Profiles)
		c["subtitle_sites"] = len(s.Subtitles.Providers)
	}
	for k, v := range c {
		if v == 0 {
			delete(c, k)
		}
	}
	return c
}

// finish gathers the per-link secrets into NeedsReentry and sorts notes.
func (s *Snapshot) finish() {
	if s.ReadAt.IsZero() {
		s.ReadAt = time.Now().UTC()
	}
	add := func(what string, ps []Provider) {
		for _, p := range ps {
			for _, f := range p.NeedsReentry {
				s.NeedsReentry = append(s.NeedsReentry, what+" \""+p.Name+"\": "+f)
			}
		}
	}
	add("indexer", s.Indexers)
	add("download client", s.DownloadClients)
	add("import list", s.ImportLists)
	add("notification", s.Notifications)
	add("metadata", s.Metadata)
	add("app link", s.AppLinks)
	add("indexer proxy", s.IndexerProxies)
	if s.Subtitles != nil {
		add("subtitle site", s.Subtitles.Providers)
	}
	if s.Downloader != nil {
		add("usenet server", s.Downloader.Servers)
		if s.Downloader.NeedsPassword {
			s.NeedsReentry = append(s.NeedsReentry, "downloader: needs your password or key")
		}
	}
	s.NeedsReentry = noteList(s.NeedsReentry)
	s.Notes = noteList(s.Notes)
	sort.SliceStable(s.Titles, func(i, j int) bool { return s.Titles[i].SourceID < s.Titles[j].SourceID })
}

// ErrHeldKey means a Snapshot still held a typed key or password.
var ErrHeldKey = errors.New("the snapshot still held a key or password; nothing was kept")

// mustNotHold refuses a Snapshot whose JSON holds one of the typed secrets.
func (s *Snapshot) mustNotHold(secrets ...string) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	for _, x := range secrets {
		if x == "" {
			continue
		}
		// The JSON may hold the key escaped (< for <, \" for ").
		esc, _ := json.Marshal(x)
		if strings.Contains(string(b), x) || strings.Contains(string(b), string(esc[1:len(esc)-1])) {
			return ErrHeldKey
		}
	}
	return nil
}

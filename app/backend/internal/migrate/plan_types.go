package migrate

import (
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A Plan says what a move would bring over from one or more Snapshots:
// grouped items HolyArrs would create or change, and every setting that
// has no HolyArrs match, with the reason (NotMoved). BuildPlan is pure: it
// looks at no disk and no database. The dry run and the move itself
// (Mover) check the Plan against this server.

// Groups of a Plan, in the order the move screen shows them.
const (
	GroupFolders     = "folders"
	GroupRules       = "pick_rules"
	GroupTitles      = "titles"
	GroupNeverAdd    = "never_add"
	GroupLists       = "outside_lists"
	GroupSources     = "sources"
	GroupDownloaders = "downloaders"
	GroupNaming      = "naming"
	GroupSubtitles   = "subtitles"
	GroupPeople      = "people"
	GroupRequests    = "wishlist"
	GroupAlerts      = "alerts"
	// GroupOther holds only not-moved entries (tags, lists, app links).
	GroupOther = "other"
)

// GroupOrder lists the groups in the order they are shown and moved.
var GroupOrder = []string{GroupFolders, GroupRules, GroupTitles, GroupNeverAdd, GroupLists, GroupSources, GroupDownloaders,
	GroupNaming, GroupSubtitles, GroupPeople, GroupRequests, GroupAlerts}

// GroupLabels are the groups' names on the move screen.
var GroupLabels = map[string]string{
	GroupFolders: "Library folders", GroupRules: "Pick rules", GroupTitles: "Titles", GroupNeverAdd: "Never-add list", GroupLists: "Outside lists",
	GroupSources: "Sources", GroupDownloaders: "Downloaders", GroupNaming: "Naming", GroupSubtitles: "Subtitles",
	GroupPeople: "People", GroupRequests: "Requests", GroupAlerts: "Alerts", GroupOther: "Other settings",
}

// Caps on what one Plan holds; anything past them is listed as not moved.
const (
	MaxPlanTitles = 50000
	MaxPlanOthers = 2000
	maxName       = 200
	maxPath       = 1024
)

// PathRule maps a folder as the old app saw it (From) to the same folder
// as HolyArrs sees it (To).
type PathRule struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Choices are what the owner changes on the move screen.
type Choices struct {
	PathRules []PathRule `json:"path_rules"`
	// Skip names groups left out of the move.
	Skip []string `json:"skip"`
	// People are the owner's choices per person key: "owner" (this was
	// the old app's owner and is me), "admin" (keep their admin role),
	// "merge" (the same person as the one here with that name), "skip".
	People map[string]string `json:"people,omitempty"`
	// ApprovedHash is written only by Start after it verifies the owner's
	// preview. A queued move must reproduce this exact canonical plan before
	// it may change domain data.
	ApprovedHash string `json:"approved_hash,omitempty"`
}

// Person choices on the move screen.
const (
	PersonOwner = "owner"
	PersonAdmin = "admin"
	PersonMerge = "merge"
	PersonSkip  = "skip"
)

// Input is one kept Snapshot to plan from.
type Input struct {
	ID   int64
	Snap *Snapshot
}

// Plan is the move, before it runs.
type Plan struct {
	Snapshots []int64  `json:"snapshots"`
	Apps      []string `json:"apps"`
	Groups    []Group  `json:"groups"`
	// NotMoved lists every setting with no HolyArrs match and why.
	NotMoved []NotMoved `json:"not_moved"`
	// NeedsInput lists the keys and passwords to type again afterwards.
	NeedsInput []string `json:"needs_input"`
	// Notes are the readers' notes, kept for the report.
	Notes []string `json:"notes,omitempty"`
	// Mapped names source fields folded into a combined plan item.
	Mapped []string `json:"mapped,omitempty"`
	// Paths are the old library folders the path rules must cover.
	Paths []string `json:"old_paths"`
	// covered names the Snapshot fields the plan looked at ("titles").
	covered map[string]bool
}

// Group is one kind of thing the move brings.
type Group struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	Skipped bool   `json:"skipped"`
	Items   []Item `json:"items"`
}

// Item is one thing the move would create or change.
type Item struct {
	// Key names it across moves, so a second move adds nothing twice.
	Key        string   `json:"key"`
	LegacyKeys []string `json:"legacy_keys,omitempty"`
	Name       string   `json:"name"`
	// From says where it came from ("radarr titles[3]").
	From   string `json:"from"`
	Detail string `json:"detail,omitempty"`
	// Why explains a match (which Pick rules and why).
	Why string `json:"why,omitempty"`
	// Needs says what the owner must give after the move ("your key").
	Needs string `json:"needs,omitempty"`

	Folder      *FolderPlan       `json:"folder,omitempty"`
	Rules       *RulesPlan        `json:"rules,omitempty"`
	Protocol    *ProtocolPlan     `json:"protocol,omitempty"`
	Title       *TitlePlan        `json:"title,omitempty"`
	NeverAdd    *NeverAddPlan     `json:"never_add,omitempty"`
	OutsideList *OutsideListPlan  `json:"outside_list,omitempty"`
	Source      *SourcePlan       `json:"source,omitempty"`
	Downloader  *DownloaderPlan   `json:"downloader,omitempty"`
	Naming      map[string]string `json:"naming,omitempty"`
	Subtitles   *SubtitlePlan     `json:"subtitles,omitempty"`
	Person      *PersonPlan       `json:"person,omitempty"`
	AskSettings *AskSettingsPlan  `json:"ask_settings,omitempty"`
	Ask         *AskPlan          `json:"ask,omitempty"`
	Alert       *AlertPlan        `json:"alert,omitempty"`
}

// NotMoved is one setting the move leaves behind, and why.
type NotMoved struct {
	Group string `json:"group"`
	From  string `json:"from"`
	What  string `json:"what"`
	Why   string `json:"why"`
}

// FolderPlan is a Library folder to add.
type FolderPlan struct {
	OldPath string `json:"old_path"`
	Path    string `json:"path"`
	Kind    string `json:"kind"` // movie, show, music
	Label   string `json:"label"`
}

// RulesPlan matches one of their profiles to our Pick rules.
type RulesPlan struct {
	App     string `json:"app"`
	Profile string `json:"profile"`
	Preset  string `json:"preset"`
}

// ProtocolPlan is the wait an old app's untagged default wait rule gives a
// ready-made Pick rules (its protocol block), set only while that block is
// still as it came.
type ProtocolPlan struct {
	Preset           string `json:"preset"`
	Prefer           string `json:"prefer"` // torrent, usenet, either
	WaitUsenet       int    `json:"wait_minutes_usenet"`
	WaitTorrent      int    `json:"wait_minutes_torrent"`
	SkipAtGoodEnough bool   `json:"skip_wait_at_good_enough"`
	Part             string `json:"part,omitempty"` // movies, shows or music when only that part's apps set it
}

// OutsideListPlan is a Radarr TMDB or public Trakt list to add as an
// Outside list, always paused.
type OutsideListPlan struct {
	Kind string `json:"kind"` // tmdb_list, trakt_list
	Ref  string `json:"ref"`
	// Library is the movie Library folder's path here.
	Library        string `json:"library"`
	Preset         string `json:"pick_rules,omitempty"`
	KeepUpToDate   bool   `json:"keep_up_to_date"`
	SearchOnAdd    bool   `json:"search_on_add"`
	ReleaseGate    string `json:"release_gate,omitempty"`
	SyncEveryHours int    `json:"sync_every_hours"`
}

// TitlePlan is a movie, show or artist to add, adopting its folder.
type TitlePlan struct {
	Kind string            `json:"kind"` // movie, show, artist
	ID   string            `json:"id"`   // tmdb:603, tvdb:81189, mbid:<id>
	IDs  map[string]string `json:"ids"`
	Year int               `json:"year,omitempty"`
	// Library is the Library folder's path here; Folder its folder in it.
	Library      string `json:"library"`
	Folder       string `json:"folder"`
	KeepUpToDate bool   `json:"keep_up_to_date"`
	Preset       string `json:"pick_rules,omitempty"`
	// Seasons left off (shows): season numbers not kept up to date.
	SeasonsOff []int `json:"seasons_off,omitempty"`
	// Unresolved says why the folder could not be placed ("" when it can).
	Unresolved string           `json:"unresolved,omitempty"`
	Music      *MusicPolicyPlan `json:"music,omitempty"`
	// ReleaseGate and GateDelayDays are a movie's Release gate from
	// Radarr's release stage and availability delay ("" and nil: this
	// server's defaults).
	ReleaseGate   string `json:"release_gate,omitempty"`
	GateDelayDays *int   `json:"gate_delay_days,omitempty"`
	// Qualities is the quality the old app recorded for each file, by the
	// file's base name ("WEBDL-1080p").
	Qualities map[string]string `json:"qualities,omitempty"`
}

// MusicPolicyPlan is the supported part of an artist's Lidarr monitoring
// policy, including explicit per-album and pressing choices.
type MusicPolicyPlan struct {
	NewAlbums       *string          `json:"new_albums,omitempty"`
	AlbumTypes      []string         `json:"album_types,omitempty"`
	SecondaryTypes  []string         `json:"secondary_types,omitempty"`
	ReleaseStatuses []string         `json:"release_statuses,omitempty"`
	Albums          []MusicAlbumPlan `json:"albums,omitempty"`
}

// MusicAlbumPlan is one album the move adds under a music artist.
type MusicAlbumPlan struct {
	MBID         string `json:"mbid"`
	KeepUpToDate bool   `json:"keep_up_to_date"`
	PressingMBID string `json:"pressing_mbid,omitempty"`
}

// NeverAddPlan is one entry for the Never-add list.
type NeverAddPlan struct {
	Kind string            `json:"kind"`
	IDs  map[string]string `json:"ids"`
}

// SourcePlan is a Source to add.
type SourcePlan struct {
	Format     string           `json:"format"`   // torznab, newznab, rss
	Protocol   string           `json:"protocol"` // torrent, usenet
	URL        string           `json:"url"`
	Kinds      []string         `json:"kinds"`
	Categories map[string][]int `json:"categories,omitempty"`
	MinSeeders *int             `json:"min_seeders,omitempty"`
	Priority   int              `json:"priority"`
	NeedsKey   bool             `json:"needs_key"`
	On         bool             `json:"on"`
}

// DownloaderPlan is a Downloader to add.
type DownloaderPlan struct {
	Adapter    string `json:"kind"` // qbittorrent, transmission, deluge, sabnzbd, nzbget
	URL        string `json:"url"`
	Username   string `json:"username,omitempty"`
	Category   string `json:"category,omitempty"`
	Priority   int    `json:"priority"`
	NeedsInput bool   `json:"needs_input"`
	// PathMap is the old apps' path rewrites for this client's host.
	PathMap []domain.PathMap `json:"path_map,omitempty"`
}

// SubtitlePlan is the default subtitle languages or one Subtitle site.
type SubtitlePlan struct {
	Languages []SubtitleWant `json:"languages,omitempty"`
	SiteKind  string         `json:"site_kind,omitempty"`
	SiteURL   string         `json:"site_url,omitempty"`
	// NeedsAccount says the site needs your account before it is used.
	NeedsAccount bool `json:"needs_account,omitempty"`
}

// SubtitleWant is one wanted language (hearing impaired: normal, hi, either).
type SubtitleWant struct {
	Language        string `json:"language"`
	HearingImpaired string `json:"hearing_impaired"`
	Forced          bool   `json:"forced"`
}

// PersonPlan is one person from Seerr or Overseerr.
type PersonPlan struct {
	App      string `json:"app"`
	SourceID int64  `json:"source_id"`
	// StableIdentity says the source supplied a provider-scoped identity that
	// survives numeric source-id reuse. Numeric-only identities are safe to
	// repeat within one move, but require an explicit owner choice across moves.
	StableIdentity bool     `json:"stable_identity,omitempty"`
	Email          string   `json:"email,omitempty"`
	Role           string   `json:"role"` // member; admin or owner only when the owner confirmed it
	Perms          []string `json:"perms"`
	PerWeek        *int     `json:"per_week,omitempty"`
	MoviesPerWeek  *int     `json:"movies_per_week,omitempty"`
	ShowsPerWeek   *int     `json:"shows_per_week,omitempty"`
	// WasAdmin and WasOwner say what they were in the old app; the move
	// screen asks the owner to confirm each.
	WasAdmin bool `json:"was_admin,omitempty"`
	WasOwner bool `json:"was_owner,omitempty"`
	// Choice is the owner's choice for them (merge or skip when a person
	// with the same name is here).
	Choice string `json:"choice,omitempty"`
}

// AskPlan is one open request.
type AskPlan struct {
	App      string            `json:"app"`
	Kind     string            `json:"kind"` // movie, show
	Ref      string            `json:"ref"`
	IDs      map[string]string `json:"ids"`
	Seasons  []int             `json:"seasons,omitempty"`
	Approved bool              `json:"approved"`
	// By is the asker's Person key; ByName their name there.
	By        string `json:"by"`
	ByName    string `json:"by_name,omitempty"`
	Decider   string `json:"decided_by,omitempty"`
	AskedAt   string `json:"asked_at,omitempty"`
	DecidedAt string `json:"decided_at,omitempty"`
	Note      string `json:"note,omitempty"`
}

// AskSettingsPlan is the subset of the old app's default quotas represented
// by HolyArrs without widening one media kind into another.
type AskSettingsPlan struct {
	MoviesPerWeek *int `json:"movies_per_week,omitempty"`
	ShowsPerWeek  *int `json:"shows_per_week,omitempty"`
}

// AlertPlan is an Alert to add (always off until its secret is typed).
type AlertPlan struct {
	Kind         string `json:"kind"` // discord, webhook, email
	SMTPHost     string `json:"smtp_host,omitempty"`
	SMTPPort     int    `json:"smtp_port,omitempty"`
	SMTPSecurity string `json:"smtp_security,omitempty"`
	SMTPFrom     string `json:"smtp_from,omitempty"`
	SMTPUser     string `json:"smtp_user,omitempty"`
	StaffTo      string `json:"staff_to,omitempty"`
}

// Group returns the named group (nil when there is none).
func (p *Plan) Group(name string) *Group {
	for i := range p.Groups {
		if p.Groups[i].Name == name {
			return &p.Groups[i]
		}
	}
	return nil
}

// Counts sums items per group and the not-moved and needs-input lists.
func (p *Plan) Counts() map[string]int {
	c := map[string]int{"not_moved": len(p.NotMoved), "needs_input": len(p.NeedsInput)}
	for _, g := range p.Groups {
		c[g.Name] = len(g.Items)
	}
	return c
}

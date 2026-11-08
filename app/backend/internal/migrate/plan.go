package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/lists"
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

// builder carries a plan being built.
type builder struct {
	p          *Plan
	rules      []PathRule
	skip       map[string]bool
	items      map[string]*Group
	keys       map[string]bool
	folders    []FolderPlan      // every folder planned (for title placement)
	presets    map[string]string // app + profile id -> preset
	titles     map[string]string // kind:id -> name (asks take names from here)
	planned    map[string]bool   // person keys planned
	personRefs map[string]string // app/source id -> stable person key
	choices    map[string]string // the owner's choices per person key
	// waits gathers the default wait rules per ready-made Pick rules name,
	// in the order first seen, so apps that disagree are seen once all are read.
	waits     map[string][]waitFrom
	waitOrder []string
	listNames map[string]bool // Outside list names planned (lower case)
	app       Kind
	snapID    int64
	paths     []pathRow // usable path rewrites from every app, placed by pathMaps
}

// waitFrom is one app's untagged default wait rule as Pick rules waits.
type waitFrom struct {
	app    Kind
	from   string
	plan   ProtocolPlan
	values string
	notes  string
}

// clip cleans a name from an old app: no control characters, trimmed,
// at most n runes.
func clip(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		s = strings.TrimSpace(string(r[:n]))
	}
	return s
}

// oldPath cleans a path as the old app saw it into slash form; "" when it
// is not an absolute path or climbs out of itself.
func oldPath(p string) string {
	p = clip(p, maxPath)
	if p == "" {
		return ""
	}
	if strings.Contains(p, `\`) || (len(p) >= 2 && p[1] == ':') {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	if !strings.HasPrefix(p, "/") && !(len(p) >= 3 && p[1] == ':' && p[2] == '/') {
		return ""
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return ""
		}
	}
	c := path.Clean(p)
	if len(c) > 1 {
		c = strings.TrimSuffix(c, "/")
	}
	return c
}

// under says whether p is base or inside it.
func under(p, base string) bool {
	if p == base {
		return true
	}
	if base == "/" {
		return strings.HasPrefix(p, "/")
	}
	return strings.HasPrefix(p, base+"/")
}

// MapPath maps an old path through the rules (the longest matching From
// wins); without a match the path stays as it is. ok is false when the
// path is not usable (relative, or climbing out of itself).
func MapPath(rules []PathRule, p string) (string, bool) {
	op := oldPath(p)
	if op == "" {
		return "", false
	}
	best, bestLen := -1, -1
	for i, r := range rules {
		from := oldPath(r.From)
		if from == "" || oldPath(r.To) == "" {
			continue
		}
		if under(op, from) && len(from) > bestLen {
			best, bestLen = i, len(from)
		}
	}
	if best < 0 {
		return op, true
	}
	from, to := oldPath(rules[best].From), oldPath(rules[best].To)
	rest := strings.TrimPrefix(op, from)
	if from == "/" {
		rest = op
	}
	return path.Clean(to + "/" + strings.TrimPrefix(rest, "/")), true
}

// ValidRules checks the owner's path rules: both sides absolute paths
// that never climb out of themselves, at most 100 rules.
func ValidRules(rules []PathRule) error {
	if len(rules) > 100 {
		return &RuleError{"at most 100 path rules"}
	}
	for _, r := range rules {
		if oldPath(r.From) == "" || oldPath(r.To) == "" {
			return &RuleError{fmt.Sprintf("path rule %q to %q: both sides must be full paths without ..", clip(r.From, 100), clip(r.To, 100))}
		}
	}
	return nil
}

// RuleError is a path rule the move cannot use; its text is safe to show.
type RuleError struct{ Text string }

// Error returns the rule text.
func (e *RuleError) Error() string { return e.Text }

// BuildPlan turns Snapshots into a Plan.
func BuildPlan(in []Input, ch Choices) *Plan {
	p := &Plan{covered: map[string]bool{}, Snapshots: []int64{}, Apps: []string{}, NotMoved: []NotMoved{}, NeedsInput: []string{}, Paths: []string{}}
	b := &builder{p: p, rules: ch.PathRules, skip: map[string]bool{}, items: map[string]*Group{}, keys: map[string]bool{},
		presets: map[string]string{}, titles: map[string]string{}, planned: map[string]bool{}, personRefs: map[string]string{}, choices: ch.People,
		waits: map[string][]waitFrom{}, listNames: map[string]bool{}}
	for _, s := range ch.Skip {
		b.skip[s] = true
	}
	for _, name := range GroupOrder {
		p.Groups = append(p.Groups, Group{Name: name, Label: GroupLabels[name], Skipped: b.skip[name], Items: []Item{}})
	}
	for i := range p.Groups {
		b.items[p.Groups[i].Name] = &p.Groups[i]
	}
	// Order: rules and folders before titles; titles before asks; people before asks.
	order := func(k Kind) int {
		switch k {
		case Seerr, Overseerr:
			return 2
		case Prowlarr, Bazarr, QBittorrent, SABnzbd:
			return 1
		}
		return 0
	}
	sorted := append([]Input(nil), in...)
	sort.SliceStable(sorted, func(i, j int) bool { return order(sorted[i].Snap.Kind) < order(sorted[j].Snap.Kind) })
	for _, x := range sorted {
		if x.Snap == nil {
			continue
		}
		b.app, b.snapID = x.Snap.Kind, x.ID
		p.Snapshots = append(p.Snapshots, x.ID)
		p.Apps = append(p.Apps, string(x.Snap.Kind))
		b.snapshot(x.Snap)
	}
	b.waitItems()
	b.pathMaps()
	p.NeedsInput = noteList(p.NeedsInput)
	p.Notes = noteList(p.Notes)
	p.Paths = noteList(p.Paths)
	return p
}

func (b *builder) from(field string, i int) string {
	if i < 0 {
		return string(b.app) + " " + field
	}
	return fmt.Sprintf("%s %s[%d]", b.app, field, i)
}

func (b *builder) not(group, from, what, why string) {
	b.p.NotMoved = append(b.p.NotMoved, NotMoved{Group: group, From: from, What: clip(what, maxName), Why: why})
}

// add puts an item in its group once; a second item with the same key is
// listed as not moved (the first one wins).
func (b *builder) add(group string, it Item) {
	it.Name = clip(it.Name, maxName)
	if it.Name == "" {
		it.Name = "(no name)"
	}
	g := b.items[group]
	limit := MaxPlanOthers
	if group == GroupTitles {
		limit = MaxPlanTitles
	}
	switch {
	case b.keys[it.Key]:
		b.not(group, it.From, it.Name, "the same thing came from another app or twice from this one; it is moved once")
		return
	case len(g.Items) >= limit:
		b.not(group, it.From, it.Name, fmt.Sprintf("more than %d in one move; move again after this one", limit))
		return
	}
	b.keys[it.Key] = true
	g.Items = append(g.Items, it)
}

func (b *builder) cover(fields ...string) {
	for _, f := range fields {
		b.p.covered[f] = true
	}
}

func (b *builder) snapshot(s *Snapshot) {
	// The read's own facts are not settings.
	b.cover("kind", "via", "version", "schema", "read_at")
	// An import list's sign-in has nowhere to go here: its own Not moved
	// line says so (leftBehind) instead of asking for it.
	var needs []string
	for _, n := range s.NeedsReentry {
		if !strings.HasPrefix(n, `import list "`) {
			needs = append(needs, n)
		}
	}
	b.p.NeedsInput = append(b.p.NeedsInput, prefix(string(s.Kind), needs)...)
	b.cover("needs_reentry")
	b.p.Notes = append(b.p.Notes, prefix(string(s.Kind), s.Notes)...)
	b.cover("notes")

	b.folderList(s)
	b.profiles(s)
	b.titleList(s)
	b.exclusions(s)
	b.indexers(s)
	b.downloaders(s)
	b.pathMappings(s)
	b.naming(s)
	b.subtitles(s)
	b.people(s)
	b.asks(s)
	b.alerts(s)
	b.leftBehind(s)
}

func prefix(app string, in []string) []string {
	out := make([]string, 0, len(in))
	for _, x := range in {
		out = append(out, app+": "+clip(x, 300))
	}
	return out
}

// libraryKind is what kind of Library folder an app's folders are.
func libraryKind(k Kind) string {
	switch k {
	case Radarr:
		return "movie"
	case Sonarr:
		return "show"
	case Lidarr:
		return "music"
	}
	return ""
}

func (b *builder) folderList(s *Snapshot) {
	b.cover("folders")
	kind := libraryKind(s.Kind)
	for i, f := range s.Folders {
		from := b.from("folders", i)
		op := oldPath(f.Path)
		if kind == "" || op == "" {
			b.not(GroupFolders, from, f.Path, "not a full path to a library folder")
			continue
		}
		np, _ := MapPath(b.rules, op)
		b.p.Paths = append(b.p.Paths, op)
		label := path.Base(np)
		b.folders = append(b.folders, FolderPlan{OldPath: op, Path: np, Kind: kind, Label: label})
		detail := "at " + np
		if np != op {
			detail = op + " becomes " + np
		}
		b.add(GroupFolders, Item{Key: "folder:" + np, Name: label, From: from, Detail: detail,
			Folder: &FolderPlan{OldPath: op, Path: np, Kind: kind, Label: clip(label, 100)}})
		keys := make([]string, 0, len(f.Extra))
		for k := range f.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.not(GroupFolders, from+"."+k, k, "a library folder here has a path, a kind and default Pick rules only")
		}
	}
}

// matchPreset finds the closest Pick rules preset for one of their
// profiles and says why.
func matchPreset(k Kind, p Profile) (string, string) {
	all := strings.ToLower(p.Cutoff + " " + strings.Join(p.Allowed, " "))
	scores := ""
	if len(p.Scores) > 0 {
		scores = fmt.Sprintf("; its %d custom format scores are not moved (Pick rules score releases their own way)", len(p.Scores))
	}
	if k == Lidarr {
		lossless := strings.Contains(all, "flac") || strings.Contains(all, "alac") || strings.Contains(all, "wav") || strings.Contains(all, "lossless")
		lossy := strings.Contains(all, "mp3") || strings.Contains(all, "aac") || strings.Contains(all, "ogg") || strings.Contains(all, "v0") || strings.Contains(all, "320")
		switch {
		case lossless && strings.Contains(strings.ToLower(p.Cutoff), "flac") || lossless && !lossy:
			return "Lossless first", "it wants lossless audio first (" + clip(p.Cutoff, 60) + ")" + scores
		case lossy && (strings.Contains(all, "320") || strings.Contains(all, "v0")) && !lossless:
			return "Lossy 320 or V0", "it allows only good lossy audio (" + clip(p.Cutoff, 60) + ")" + scores
		}
		return "Any music", "it allows a mix of audio kinds" + scores
	}
	cut := strings.ToLower(p.Cutoff)
	if strings.Contains(cut, "2160") || strings.Contains(cut, "4k") || strings.Contains(cut, "uhd") {
		return "4K", "it aims for 2160p (" + clip(p.Cutoff, 60) + ")" + scores
	}
	why := "it aims for " + clip(p.Cutoff, 60) + " or lower; Standard picks the best copy up to 1080p"
	if p.Cutoff == "" {
		why = "it names no top quality; Standard picks the best copy up to 1080p"
	}
	if !p.UpgradeAllowed {
		why += "; it never looked for better copies, Standard does until the top step"
	}
	return "Standard", why + scores
}

func (b *builder) profiles(s *Snapshot) {
	b.cover("quality_profiles", "custom_formats", "release_profiles", "delay_profiles", "language_profiles", "metadata_profiles")
	for i, p := range s.QualityProfiles {
		preset, why := matchPreset(s.Kind, p)
		b.presets[string(s.Kind)+":"+strconv.FormatInt(p.ID, 10)] = preset
		b.add(GroupRules, Item{Key: fmt.Sprintf("pick_rules:%d:%s:%d", b.snapID, s.Kind, p.ID), Name: p.Name, From: b.from("quality_profiles", i),
			Detail: "becomes " + preset, Why: why, Rules: &RulesPlan{App: string(s.Kind), Profile: clip(p.Name, maxName), Preset: preset}})
	}
	for i, f := range s.CustomFormats {
		b.not(GroupRules, b.from("custom_formats", i), f.Name, "format scoring rules do not move; Pick rules score a fixed list of traits")
	}
	for i, rw := range s.ReleaseProfiles {
		name := rw.Name
		if name == "" {
			name = "release words"
		}
		var parts []string
		if w := wordList(rw.Settings["required"]); w != "" {
			parts = append(parts, "must contain "+w)
		}
		if w := wordList(rw.Settings["ignored"]); w != "" {
			parts = append(parts, "must not contain "+w)
		}
		// The words are cut short enough that the scope always fits maxName.
		what := name
		if len(parts) > 0 {
			what += ": " + strings.Join(parts, " / ")
		}
		scope := tagScope(wordList(rw.Settings["tags"]))
		if w := wordList(rw.Settings["excluded_tags"]); w != "" {
			scope = strings.TrimSuffix(scope, ")") + " except tags: " + clip(w, 40) + ")"
		}
		what = clip(what, 90) + " " + scope
		why := "Pick rules test a fixed list of traits, not free words; add a must-have or never trait under Pick rules if one fits"
		if !boolean(rw.Settings["enabled"]) {
			why = "it was turned off there"
		}
		b.not(GroupRules, b.from("release_profiles", i), what, why)
	}
	b.waitRules(s)
	for i, n := range s.LanguageProfiles {
		b.not(GroupRules, b.from("language_profiles", i), n.Name, "language profiles are not moved; Pick rules set the languages")
	}
	// Lidarr metadata profiles are accounted where a moved artist actually
	// consumes their individual fields in titleList.
}

// maxWait is the longest wait Pick rules allow, in minutes (7 days).
const maxWait = 10080

// waitValues says a wait rule's values ("prefer torrent, Usenet 0 min,
// torrent 120 min, Usenet off").
func waitValues(d DelayProfile) string {
	prefer := d.PreferredProtocol
	if prefer == "" {
		prefer = "either"
	}
	values := fmt.Sprintf("prefer %s, Usenet %d min, torrent %d min", prefer, d.UsenetDelay, d.TorrentDelay)
	if !d.EnableUsenet {
		values += ", Usenet off"
	}
	if !d.EnableTorrent {
		values += ", torrent off"
	}
	return values
}

// waitWords says Pick rules waits in the plan and the report.
func waitWords(p ProtocolPlan) string {
	s := fmt.Sprintf("prefer %s, Usenet %d min, torrent %d min", p.Prefer, p.WaitUsenet, p.WaitTorrent)
	if p.SkipAtGoodEnough {
		s += ", no wait once a release is Good enough"
	}
	return s
}

func clampWait(n int) (int, bool) {
	switch {
	case n > maxWait:
		return maxWait, true
	case n < 0:
		return 0, false
	}
	return n, false
}

// waitRules handles an app's wait rules. Its untagged default rule (the
// untagged one with the highest order) is gathered for the ready-made Pick
// rules its quality profiles matched (waitItems), unless it is the old app's
// untouched default, which would only switch them to prefer Usenet. Tagged
// rules are listed with their values.
func (b *builder) waitRules(s *Snapshot) {
	def := -1
	for i, d := range s.DelayProfiles {
		if len(d.Tags) == 0 && (def < 0 || d.Order > s.DelayProfiles[def].Order) {
			def = i
		}
	}
	var presets []string
	for _, p := range s.QualityProfiles {
		if pr := b.presets[string(s.Kind)+":"+strconv.FormatInt(p.ID, 10)]; pr != "" && !slices.Contains(presets, pr) {
			presets = append(presets, pr)
		}
	}
	for i, d := range s.DelayProfiles {
		from := b.from("delay_profiles", i)
		values := waitValues(d)
		what := "wait before grabbing " + tagScope(strings.Join(d.Tags, ", "))
		switch {
		case len(d.Tags) > 0:
			b.not(GroupRules, from, what, "applied only to titles tagged "+clip(strings.Join(d.Tags, ", "), 100)+"; HolyArrs has no tags, so its waits ("+values+") were not moved; set them on a copy of the Pick rules if those titles need them")
			continue
		case i != def || mediaWord(s.Kind) == "":
			b.not(GroupRules, from, what, "HolyArrs has waits in Pick rules (prefer, Usenet and torrent minutes) but this one was not carried over: "+values+"; set them under Pick rules")
			continue
		}
		waits := d.UsenetDelay > 0 || d.TorrentDelay > 0
		if d.BypassIfAboveScore && waits {
			b.not(GroupRules, from+".bypass_score", "skip the wait at a format score", "format scores do not move; turn on \"Skip the wait once good enough\" in Pick rules instead")
		}
		if d.EnableUsenet && d.EnableTorrent && strings.EqualFold(d.PreferredProtocol, "usenet") && !waits {
			// The old app's untouched default (its skip switches do nothing
			// without a wait): it grabs at once, which is what the ready-made
			// Pick rules do, so it only takes part in the disagreement check.
			if len(presets) == 0 {
				b.p.Mapped = append(b.p.Mapped, from)
			}
			for _, pr := range presets {
				if _, ok := b.waits[pr]; !ok {
					b.waitOrder = append(b.waitOrder, pr)
				}
				b.waits[pr] = append(b.waits[pr], waitFrom{app: s.Kind, from: from, plan: ProtocolPlan{Preset: pr, Prefer: "either"}, values: values})
			}
			continue
		}
		w := ProtocolPlan{Prefer: "either", SkipAtGoodEnough: d.BypassIfHighestQuality && waits}
		if p := strings.ToLower(d.PreferredProtocol); p == "usenet" || p == "torrent" {
			w.Prefer = p
		}
		notes := ""
		var cut bool
		if w.WaitUsenet, cut = clampWait(d.UsenetDelay); cut {
			notes += fmt.Sprintf("; Usenet %d min cut to %d (HolyArrs allows up to 7 days)", d.UsenetDelay, maxWait)
		}
		if w.WaitTorrent, cut = clampWait(d.TorrentDelay); cut {
			notes += fmt.Sprintf("; torrent %d min cut to %d (HolyArrs allows up to 7 days)", d.TorrentDelay, maxWait)
		}
		if w.SkipAtGoodEnough {
			notes += "; HolyArrs skips the wait at Good enough, which can be below the top allowed quality"
		}
		switch {
		case d.EnableUsenet && !d.EnableTorrent:
			w.Prefer = "usenet"
			b.not(GroupRules, from+".enable_torrent", "torrent off", "Pick rules cannot turn a protocol off, so pause the torrent Sources instead; Usenet is preferred")
		case d.EnableTorrent && !d.EnableUsenet:
			w.Prefer = "torrent"
			b.not(GroupRules, from+".enable_usenet", "Usenet off", "Pick rules cannot turn a protocol off, so pause the Usenet Sources instead; torrent is preferred")
		case !d.EnableTorrent && !d.EnableUsenet:
			b.not(GroupRules, from+".enable_usenet", "Usenet and torrent off", "Pick rules cannot turn a protocol off, so pause those Sources instead")
		}
		if len(presets) == 0 {
			b.not(GroupRules, from, what, "no quality profile came over to carry it: "+values+"; set them under Pick rules")
			continue
		}
		for _, pr := range presets {
			wp := w
			wp.Preset = pr
			if _, ok := b.waits[pr]; !ok {
				b.waitOrder = append(b.waitOrder, pr)
			}
			b.waits[pr] = append(b.waits[pr], waitFrom{app: s.Kind, from: from, plan: wp, values: values, notes: notes})
		}
	}
}

// waitItems plans, once every app is read, one wait change per ready-made
// Pick rules. Apps that set different waits for the same Pick rules change
// nothing there, and each one is listed.
func (b *builder) waitItems() {
	for _, preset := range b.waitOrder {
		ws := b.waits[preset]
		same := true
		for _, w := range ws[1:] {
			same = same && w.plan == ws[0].plan
		}
		if !same {
			for _, w := range ws {
				b.not(GroupRules, w.from, "wait before grabbing (every title)",
					fmt.Sprintf("the old apps set different waits for %s, so this server's wait stays; %s had %s", preset, w.app, w.values))
			}
			continue
		}
		p := ws[0].plan
		if p == (ProtocolPlan{Preset: preset, Prefer: "either"}) {
			for _, w := range ws {
				if !slices.Contains(b.p.Mapped, w.from) {
					b.p.Mapped = append(b.p.Mapped, w.from)
				}
			}
			continue
		}
		var apps []string
		for i, w := range ws {
			if !slices.Contains(apps, string(w.app)) {
				apps = append(apps, string(w.app))
			}
			// The part the change belongs to, when every app that set it
			// has the same one; "" (always moved) when they differ.
			part := map[Kind]string{Radarr: "movies", Sonarr: "shows", Lidarr: "music"}[w.app]
			if i == 0 {
				p.Part = part
			} else if p.Part != part {
				p.Part = ""
			}
		}
		b.add(GroupRules, Item{Key: "pick_rules_protocol:" + preset, Name: preset + " wait", From: ws[0].from,
			Detail:   "from the default wait rule of " + strings.Join(apps, " and ") + ": " + waitWords(p) + ws[0].notes,
			Why:      "set only while the wait on these ready-made Pick rules is still as it came; undo puts it back",
			Protocol: &p})
		for _, w := range ws[1:] {
			if !slices.Contains(b.p.Mapped, w.from) {
				b.p.Mapped = append(b.p.Mapped, w.from)
			}
		}
	}
}

// tagScope says which titles a rule served: "(tags: a, b)" or "(every title)".
func tagScope(tags string) string {
	if tags == "" {
		return "(every title)"
	}
	return "(tags: " + clip(tags, 100) + ")"
}

// wordList joins a kept list of words or names ("" when there are none).
func wordList(v any) string {
	var out []string
	for _, e := range list(v) {
		if w := str(e); w != "" {
			out = append(out, w)
		}
	}
	return clip(strings.Join(out, ", "), 120)
}

// mediaWord is the kind of title an app's links served ("" for the others).
func mediaWord(k Kind) string {
	return map[Kind]string{Radarr: "movie", Sonarr: "show", Lidarr: "music"}[k]
}

// tagNote is the Detail words for a moved Source, Downloader or Alert of
// Radarr, Sonarr or Lidarr that served only tagged titles there ("" when
// it had no tags).
func tagNote(k Kind, tags []string) string {
	if mediaWord(k) == "" || len(tags) == 0 {
		return ""
	}
	return "; only for titles tagged " + clip(strings.Join(tags, ", "), 100) + " there; here it serves every " + mediaWord(k) + " title"
}

// addTagged adds a Source, Downloader or Alert and, once it is planned and
// had tags, lists its tags as not moved.
func (b *builder) addTagged(k Kind, group string, it Item, tags []string) {
	note := tagNote(k, tags)
	it.Detail += note
	before := len(b.items[group].Items)
	b.add(group, it)
	if note != "" && len(b.items[group].Items) > before {
		check := "HolyArrs has no tags; check it before you resume it"
		if group == GroupAlerts {
			check = "HolyArrs has no tags; check it before you turn it on"
		}
		b.not(group, it.From+".tags", clip(it.Name, 80)+" (tags: "+clip(strings.Join(tags, ", "), 100)+")", check)
	}
}

// sharesTag says whether two tag lists have a name in common.
func sharesTag(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if strings.EqualFold(x, y) {
				return true
			}
		}
	}
	return false
}

func allowedMusicValues(v any, names map[string]string) (out, unsupported []string) {
	for _, raw := range list(v) {
		m := lower(obj(raw))
		if !boolean(m["allowed"]) {
			continue
		}
		name := str(m["name"])
		if n := names[strings.ToLower(name)]; n != "" {
			out = append(out, n)
		} else {
			unsupported = append(unsupported, name)
		}
	}
	return out, unsupported
}

type musicIssue struct{ field, what, why string }

func musicPolicy(s *Snapshot, t Title) (*MusicPolicyPlan, []musicIssue) {
	if s.Kind != Lidarr {
		return nil, nil
	}
	p := &MusicPolicyPlan{Albums: []MusicAlbumPlan{}}
	var issues []musicIssue
	switch strings.ToLower(str(anyKey(t.Extra, "new_albums"))) {
	case "new":
		v := "future_only"
		p.NewAlbums = &v
	case "none":
		v := "none"
		p.NewAlbums = &v
	case "all":
		v := "all"
		p.NewAlbums = &v
	case "":
		issues = append(issues, musicIssue{"new_albums", "(missing)", "the source did not provide a new-album policy, so it is left unchanged"})
	default:
		issues = append(issues, musicIssue{"new_albums", str(anyKey(t.Extra, "new_albums")), "the new-album policy is unknown, so it is left unchanged"})
	}
	profileID := num(anyKey(t.Extra, "metadata_profile_id"))
	found := false
	if profileID == 0 {
		issues = append(issues, musicIssue{"metadata_profile_id", "(missing)", "the source did not identify a metadata profile, so its album filters are left unchanged"})
	}
	for _, mp := range s.MetadataProfiles {
		if mp.ID != profileID {
			continue
		}
		found = true
		var bad []string
		p.AlbumTypes, bad = allowedMusicValues(anyKey(mp.Settings, "primaryalbumtypes"), map[string]string{
			"album": "album", "ep": "ep", "single": "single", "broadcast": "broadcast", "other": "other",
		})
		for _, v := range bad {
			issues = append(issues, musicIssue{"metadata_profile.primaryalbumtypes", v, "this allowed album kind has no exact HolyArrs match"})
		}
		if len(p.AlbumTypes) == 0 {
			issues = append(issues, musicIssue{"metadata_profile.primaryalbumtypes", "(none supported)", "no supported allowed album kind was provided, so this filter is left unchanged"})
		}
		p.SecondaryTypes, bad = allowedMusicValues(anyKey(mp.Settings, "secondaryalbumtypes"), map[string]string{
			"studio": "studio", "live": "live", "compilation": "compilation", "remix": "remix", "soundtrack": "soundtrack", "spokenword": "spokenword", "audiobook": "audiobook", "interview": "interview", "demo": "demo",
		})
		for _, v := range bad {
			issues = append(issues, musicIssue{"metadata_profile.secondaryalbumtypes", v, "this allowed extra album kind has no exact HolyArrs match"})
		}
		if len(p.SecondaryTypes) == 0 {
			issues = append(issues, musicIssue{"metadata_profile.secondaryalbumtypes", "(none supported)", "no supported allowed extra album kind was provided, so this filter is left unchanged"})
		}
		p.ReleaseStatuses, bad = allowedMusicValues(anyKey(mp.Settings, "releasestatuses"), map[string]string{
			"official": "official", "promotion": "promotion", "bootleg": "bootleg", "pseudo-release": "pseudo-release", "pseudorelease": "pseudo-release",
		})
		for _, v := range bad {
			issues = append(issues, musicIssue{"metadata_profile.releasestatuses", v, "this allowed release status has no exact HolyArrs match"})
		}
		if len(p.ReleaseStatuses) == 0 {
			issues = append(issues, musicIssue{"metadata_profile.releasestatuses", "(none supported)", "no supported allowed release status was provided, so this filter is left unchanged"})
		}
		break
	}
	if profileID != 0 && !found {
		issues = append(issues, musicIssue{"metadata_profile_id", strconv.FormatInt(profileID, 10), "the referenced metadata profile was not read, so its filters are left unchanged"})
	}
	for i, a := range t.MusicAlbums {
		if !isMBID(a.MBID) {
			issues = append(issues, musicIssue{fmt.Sprintf("music_albums[%d].mbid", i), a.MBID, "the album id is invalid, so its monitoring choice is left unchanged"})
			continue
		}
		ap := MusicAlbumPlan{MBID: strings.ToLower(a.MBID), KeepUpToDate: a.Monitored}
		if isMBID(a.WatchedEdition) {
			ap.PressingMBID = strings.ToLower(a.WatchedEdition)
		} else if a.WatchedEdition != "" {
			issues = append(issues, musicIssue{fmt.Sprintf("music_albums[%d].watched_edition", i), a.WatchedEdition, "the pressing id is invalid, so the watched pressing is left unchanged"})
		}
		p.Albums = append(p.Albums, ap)
	}
	sort.Slice(p.Albums, func(i, j int) bool { return p.Albums[i].MBID < p.Albums[j].MBID })
	return p, issues
}

// idFor picks the id a title is added by.
func idFor(k Kind, t Title) (kind, id string) {
	switch k {
	case Radarr:
		if v := clip(t.IDs["tmdb"], 20); v != "" && isDigits(v) {
			return "movie", "tmdb:" + v
		}
		return "movie", ""
	case Sonarr:
		for _, src := range []string{"tvdb", "tmdb"} {
			if v := clip(t.IDs[src], 20); v != "" && isDigits(v) {
				return "show", src + ":" + v
			}
		}
		return "show", ""
	case Lidarr:
		if v := clip(t.IDs["musicbrainz"], 40); v != "" && isMBID(v) {
			return "artist", "mbid:" + strings.ToLower(v)
		}
		return "artist", ""
	}
	return "", ""
}

func isDigits(s string) bool {
	if s == "" || s == "0" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isMBID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, r := range strings.ToLower(s) {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// place finds the Library folder (here) and the one folder in it that hold
// a title's old path.
func (b *builder) place(t Title) (lib, folder, why string) {
	op := oldPath(t.Path)
	if op == "" {
		return "", "", "its folder is not a full path"
	}
	best := -1
	for i, f := range b.folders {
		if under(op, f.OldPath) && op != f.OldPath && (best < 0 || len(f.OldPath) > len(b.folders[best].OldPath)) {
			best = i
		}
	}
	if best < 0 {
		return "", "", "its folder " + op + " is in none of the library folders read"
	}
	f := b.folders[best]
	rest := strings.TrimPrefix(strings.TrimPrefix(op, f.OldPath), "/")
	if f.OldPath == "/" {
		rest = strings.TrimPrefix(op, "/")
	}
	if rest == "" || strings.Contains(rest, "/") || strings.HasPrefix(rest, ".") {
		return "", "", "its folder is not directly in its library folder"
	}
	return f.Path, rest, ""
}

// releaseGates are Radarr's release stages (words or the backup's numbers)
// as Release gate choices.
var releaseGates = map[string]string{"tba": "announced", "0": "announced", "announced": "announced", "1": "announced",
	"incinemas": "in_cinemas", "2": "in_cinemas", "released": "released", "3": "released"}

// gateWords say a Release gate choice in the plan's details.
var gateWords = map[string]string{"announced": "announced", "in_cinemas": "in cinemas", "released": "released", "": "this server's default"}

func (b *builder) titleList(s *Snapshot) {
	b.cover("titles")
	extras := map[string]int{}
	tags := 0
	usedMetadata := map[int64]bool{}
	// Radarr's availability delay is stamped on each moved movie (0 to 365
	// days); this server's default for new movies is left as it is.
	var delay *int
	delayNote := ""
	if v, ok := s.Settings["availability_delay"]; ok && s.Kind == Radarr {
		d := int(num(v))
		switch {
		case d > 365:
			d, delayNote = 365, ", cut to 365"
		case d < 0:
			d = 0
		}
		delay = &d
	}
	gated := 0
	for i, t := range s.Titles {
		from := b.from("titles", i)
		kind, id := idFor(s.Kind, t)
		if kind == "" {
			b.not(GroupTitles, from, t.Name, "this app has no library")
			continue
		}
		name := clip(t.Name, maxName)
		if id == "" {
			b.not(GroupTitles, from, name, "no "+map[string]string{"movie": "TMDB", "show": "TVDB or TMDB", "artist": "MusicBrainz"}[kind]+" id is known for it")
			continue
		}
		ids := map[string]string{}
		for k, v := range t.IDs {
			if k = clip(k, 20); k != "" && len(ids) < 10 {
				ids[k] = clip(v, 60)
			}
		}
		music, issues := musicPolicy(s, t)
		for _, issue := range issues {
			b.not(GroupTitles, from+"."+issue.field, issue.what, issue.why)
		}
		tp := &TitlePlan{Kind: kind, ID: id, IDs: ids, Year: t.Year, KeepUpToDate: t.Monitored,
			Preset: b.presets[string(s.Kind)+":"+strconv.FormatInt(t.ProfileID, 10)], Music: music}
		if tp.Preset == "" && kind != "artist" {
			tp.Preset = "Standard"
		}
		for _, se := range t.Seasons {
			if !se.Monitored && se.Number >= 0 && se.Number < 10000 {
				tp.SeasonsOff = append(tp.SeasonsOff, se.Number)
			}
		}
		gateNote := ""
		if kind == "movie" {
			if v := str(anyKey(t.Extra, "minimum_availability")); v != "" {
				if g, ok := releaseGates[strings.ToLower(v)]; ok {
					tp.ReleaseGate = g
				} else {
					b.not(GroupTitles, from+".minimum_availability", clip(name, 140)+": release stage "+clip(v, 40), "unknown release stage "+clip(v, 40)+"; the title takes this server's default")
				}
			}
			tp.GateDelayDays = delay
			if tp.ReleaseGate != "" || delay != nil {
				gateNote = "; release gate: " + gateWords[tp.ReleaseGate]
				if delay != nil {
					gateNote += fmt.Sprintf(" (+%d days%s)", *delay, delayNote)
				}
			}
		}
		tp.Library, tp.Folder, tp.Unresolved = b.place(t)
		b.titles[kind+":"+id] = name
		for _, v := range t.IDs {
			if v != "" {
				b.titles[kind+":ids:"+clip(v, 60)] = name
			}
		}
		if tp.Unresolved != "" {
			b.not(GroupTitles, from, name, "not placed: "+tp.Unresolved+"; add a path rule and plan again")
			continue
		}
		detail := "uses the folder " + path.Join(tp.Library, tp.Folder) + " where it is"
		if !t.Monitored {
			detail += "; Keep up to date off"
		}
		detail += gateNote
		if t.Year > 0 {
			name = fmt.Sprintf("%s (%d)", name, t.Year)
		}
		key := "title:" + kind + ":" + id
		before := len(b.items[GroupTitles].Items)
		b.add(GroupTitles, Item{Key: key, Name: name, From: from, Detail: detail, Title: tp})
		if tp.GateDelayDays != nil && len(b.items[GroupTitles].Items) > before {
			gated++
		}
		if s.Kind == Lidarr && len(b.items[GroupTitles].Items) > before {
			profileID := num(anyKey(t.Extra, "metadata_profile_id"))
			for j, mp := range s.MetadataProfiles {
				if mp.ID != profileID {
					continue
				}
				usedMetadata[profileID] = true
				fields := []struct {
					name   string
					values []string
				}{{"primaryalbumtypes", tp.Music.AlbumTypes}, {"secondaryalbumtypes", tp.Music.SecondaryTypes}, {"releasestatuses", tp.Music.ReleaseStatuses}}
				known := map[string]bool{}
				for _, field := range fields {
					known[field.name] = true
					fromProfile := b.from("metadata_profiles", j) + "." + field.name
					if len(field.values) > 0 {
						b.p.Mapped = append(b.p.Mapped, fromProfile)
					} else {
						b.not(GroupRules, fromProfile, mp.Name+" "+field.name, "no supported allowed value was represented on the moved artist")
					}
				}
				settingKeys := make([]string, 0, len(mp.Settings))
				for field := range mp.Settings {
					settingKeys = append(settingKeys, field)
				}
				sort.Strings(settingKeys)
				for _, field := range settingKeys {
					if !known[field] {
						b.not(GroupRules, b.from("metadata_profiles", j)+"."+field, mp.Name+" "+field, "this metadata profile field has no HolyArrs match")
					}
				}
				break
			}
		}
		for k := range t.Extra {
			if s.Kind == Lidarr && (k == "new_albums" || k == "metadata_profile_id") || s.Kind == Radarr && k == "minimum_availability" {
				continue
			}
			extras[k]++
		}
		tags += len(t.Tags)
	}
	keys := make([]string, 0, len(extras))
	for k := range extras {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.not(GroupTitles, b.from("titles", -1)+"."+k, fmt.Sprintf("%s (on %d titles)", k, extras[k]), "no HolyArrs setting matches it; titles take HolyArrs's own defaults")
	}
	if tags > 0 {
		b.not(GroupTitles, b.from("titles", -1)+".tags", fmt.Sprintf("tags on titles (%d)", tags), "HolyArrs has no tags")
	}
	if delay != nil {
		from := b.from("settings", -1) + ".availability_delay"
		if gated > 0 {
			b.p.Mapped = append(b.p.Mapped, from)
			b.p.Notes = append(b.p.Notes, fmt.Sprintf("%s: the availability delay (%d days) is set on each moved movie; this server's default for new movies stays as it is", s.Kind, *delay))
		} else {
			b.not(GroupTitles, from, "availability delay", "no moved movie takes it; set the delay under Settings > Movies and TV defaults if you want it")
		}
	}
	if s.Kind == Lidarr {
		for i, mp := range s.MetadataProfiles {
			if !usedMetadata[mp.ID] {
				b.not(GroupRules, b.from("metadata_profiles", i), mp.Name, "no moved artist uses this metadata profile")
			}
		}
	}
}

func (b *builder) exclusions(s *Snapshot) {
	b.cover("exclusions")
	kind := map[Kind]string{Radarr: "movie", Sonarr: "show", Lidarr: "artist"}[s.Kind]
	// A live read whose exclusions route failed says so in the reader's
	// notes (a read kept before live reads had the backup-only note).
	if slices.Contains(s.Notes, "the app has no exclusions list; skipped") || slices.Contains(s.Notes, "exclusion lists are read from a backup only") {
		b.not(GroupNeverAdd, b.from("exclusions", -1), "exclusions", "they could not be read through the API; read a backup to bring them")
	}
	for i, e := range s.Exclusions {
		from := b.from("exclusions", i)
		ids := map[string]string{}
		var key string
		for _, src := range []string{"tmdb", "tvdb", "musicbrainz", "imdb"} {
			if v := clip(e.IDs[src], 60); v != "" {
				ids[src] = v
				if key == "" {
					key = src + ":" + v
				}
			}
		}
		if kind == "" || key == "" || (kind == "movie" && ids["tmdb"] == "") {
			b.not(GroupNeverAdd, from, e.Name, "no id HolyArrs can match")
			continue
		}
		b.add(GroupNeverAdd, Item{Key: "never_add:" + kind + ":" + key, Name: e.Name, From: from, Detail: "kept off the Library",
			NeverAdd: &NeverAddPlan{Kind: kind, IDs: ids}})
	}
}

// sourceKinds are the media kinds a Source from an app serves.
func sourceKinds(k Kind) []string {
	switch k {
	case Radarr:
		return []string{"movie"}
	case Sonarr:
		return []string{"tv"}
	case Lidarr:
		return []string{"music"}
	}
	return []string{"movie", "tv", "music"}
}

func setStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		for mk, v := range m {
			if strings.EqualFold(mk, k) {
				if s := str(v); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func setNum(m map[string]any, keys ...string) (int64, bool) {
	for _, k := range keys {
		for mk, v := range m {
			if strings.EqualFold(mk, k) && v != nil {
				return num(v), true
			}
		}
	}
	return 0, false
}

func hasField(fields []string, names ...string) bool {
	for _, f := range fields {
		for _, n := range names {
			if strings.EqualFold(f, n) {
				return true
			}
		}
	}
	return false
}

// webURL checks an address: http or https with a host, no user part.
func webURL(raw string) string {
	raw = clip(raw, 500)
	low := strings.ToLower(raw)
	if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
		return ""
	}
	rest := raw[strings.Index(raw, "://")+3:]
	host := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host = rest[:i]
	}
	if host == "" || strings.Contains(host, "@") || strings.ContainsAny(raw, " \t") {
		return ""
	}
	return raw
}

func (b *builder) indexers(s *Snapshot) {
	b.cover("indexers", "indexer_proxies")
	for i, px := range s.IndexerProxies {
		b.not(GroupSources, b.from("indexer_proxies", i), px.Name, "HolyArrs sets one proxy address per Source; set it on the Sources that used it")
	}
	for i, ix := range s.Indexers {
		from := b.from("indexers", i)
		kind := strings.ToLower(ix.Kind)
		var format string
		switch kind {
		case "torznab":
			format = "torznab"
		case "newznab":
			format = "newznab"
		case "torrentrssindexer":
			format = "rss"
		}
		if format == "" {
			why := "HolyArrs reads Torznab, Newznab and RSS Sources; add this one through the Sources bridge"
			if s.Kind == Prowlarr {
				why = "a Prowlarr site definition; HolyArrs reaches such sites through the Sources bridge (Settings > Sources)"
			}
			b.not(GroupSources, from, ix.Name, why)
			continue
		}
		u := webURL(setStr(ix.Settings, "baseUrl", "feedUrl", "url"))
		if u == "" {
			b.not(GroupSources, from, ix.Name, "no usable address (http or https)")
			continue
		}
		if metadataHost(u) {
			b.not(GroupSources, from, ix.Name, "a link-local or cloud metadata address; HolyArrs never reaches those")
			continue
		}
		if p := setStr(ix.Settings, "apiPath"); format != "rss" && p != "" && p != "/api" {
			u = strings.TrimSuffix(u, "/") + "/" + strings.TrimPrefix(clip(p, 100), "/")
		}
		protocol := "torrent"
		if format == "newznab" || ix.Protocol == "usenet" {
			protocol = "usenet"
		}
		sp := &SourcePlan{Format: format, Protocol: protocol, URL: u, Kinds: sourceKinds(s.Kind), On: ix.Enabled}
		if ix.Priority >= 1 && ix.Priority <= 50 {
			sp.Priority = ix.Priority
		} else {
			sp.Priority = 25
		}
		if n, ok := setNum(ix.Settings, "minimumSeeders"); ok && n >= 0 && n < 10000 && protocol == "torrent" {
			v := int(n)
			sp.MinSeeders = &v
		}
		if cats := list(anyKey(ix.Settings, "categories")); len(cats) > 0 && len(sp.Kinds) == 1 {
			var cs []int
			for _, c := range cats {
				if n := num(c); n > 0 && n < 1000000 && len(cs) < 50 {
					cs = append(cs, int(n))
				}
			}
			if len(cs) > 0 {
				sp.Categories = map[string][]int{sp.Kinds[0]: cs}
			}
		}
		sp.NeedsKey = hasField(ix.NeedsReentry, "apiKey", "passkey", "cookie")
		it := Item{Key: "source:" + strings.ToLower(clip(ix.Name, 80)), Name: clip(ix.Name, 80), From: from, Source: sp,
			Detail: format + " at " + u}
		it.Detail += "; added paused: check it and resume it yourself"
		if sp.NeedsKey {
			it.Needs = "your key"
			it.Detail = format + " at " + u + "; added paused until you type its key and resume it"
		}
		// Prowlarr's indexer tags only pick apps and proxies: a proxy
		// sharing one of them was used for this indexer.
		for _, px := range s.IndexerProxies {
			if sharesTag(ix.Tags, px.Tags) {
				it.Detail += "; used proxy " + clip(px.Name, 80) + " there; set a proxy on this Source"
			}
		}
		b.addTagged(s.Kind, GroupSources, it, ix.Tags)
		keys := make([]string, 0, len(ix.Settings))
		for k := range ix.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch strings.ToLower(k) {
			case "baseurl", "feedurl", "url", "apipath", "minimumseeders", "categories":
				continue
			}
			if strings.HasPrefix(k, "link.") {
				continue
			}
			b.not(GroupSources, from+"."+k, ix.Name+": "+k, "no Source setting matches it")
		}
	}
}

func anyKey(m map[string]any, key string) any {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return nil
}

func hostURL(host string, port int, tls bool, base string) string {
	host = clip(host, 200)
	if host == "" || strings.ContainsAny(host, "/@?# ") {
		return ""
	}
	scheme := "http"
	if tls {
		scheme = "https"
	}
	u := scheme + "://" + host
	if port > 0 && port < 65536 {
		u += ":" + strconv.Itoa(port)
	}
	if base = strings.Trim(clip(base, 100), "/"); base != "" && !strings.Contains(base, "..") {
		u += "/" + base
	}
	return webURL(u)
}

func (b *builder) downloaders(s *Snapshot) {
	b.cover("download_clients", "downloader")
	for i, d := range s.DownloadClients {
		from := b.from("download_clients", i)
		var adapter string
		switch k := strings.ToLower(d.Kind); k {
		case "qbittorrent", "sabnzbd", "transmission", "deluge", "nzbget":
			adapter = k
		}
		if adapter == "" {
			b.not(GroupDownloaders, from, d.Name, "HolyArrs talks to qBittorrent, Transmission, Deluge, SABnzbd and NZBGet only")
			continue
		}
		port, _ := setNum(d.Settings, "port")
		tls := strings.EqualFold(setStr(d.Settings, "useSsl"), "true")
		u := hostURL(setStr(d.Settings, "host"), int(port), tls, setStr(d.Settings, "urlBase"))
		if u == "" || metadataHost(u) {
			b.not(GroupDownloaders, from, d.Name, "no usable address")
			continue
		}
		cat := setStr(d.Settings, "movieCategory", "tvCategory", "musicCategory", "category")
		locked := hasField(d.NeedsReentry, "password", "apiKey")
		b.addDownloader(s.Kind, from, d.Name, &DownloaderPlan{Adapter: adapter, URL: u, Username: clip(setStr(d.Settings, "username"), 100),
			Category: clean64(cat), Priority: d.Priority, NeedsInput: locked}, d.Tags)
	}
	if d := s.Downloader; d != nil {
		adapter := map[Kind]string{QBittorrent: "qbittorrent", SABnzbd: "sabnzbd"}[s.Kind]
		from := b.from("downloader", -1)
		u := hostURL(d.Host, d.Port, d.TLS, d.URLBase)
		if u == "" || adapter == "" || metadataHost(u) {
			b.not(GroupDownloaders, from, string(s.Kind), "no usable address")
		} else {
			b.addDownloader(s.Kind, from, map[string]string{"qbittorrent": "qBittorrent", "sabnzbd": "SABnzbd"}[adapter],
				&DownloaderPlan{Adapter: adapter, URL: u, Username: clip(d.Username, 100), NeedsInput: d.NeedsPassword || adapter == "sabnzbd"}, nil)
		}
		if d.SavePath != "" || d.CompletePath != "" {
			b.not(GroupDownloaders, from+".save_path", "download folders", "the Downloader keeps its own folders; HolyArrs reads where each download lands")
		}
		for i, c := range d.Categories {
			b.not(GroupDownloaders, b.from("downloader.categories", i), c.Name, "HolyArrs uses one category of its own per Downloader (holyarrs)")
		}
		for i, sv := range d.Servers {
			b.not(GroupDownloaders, b.from("downloader.servers", i), sv.Name, "usenet servers stay set up in SABnzbd")
		}
	}
}

func clean64(s string) string {
	s = clip(s, 64)
	if strings.ContainsAny(s, `/\`) {
		return ""
	}
	return s
}

func (b *builder) addDownloader(app Kind, from, name string, d *DownloaderPlan, tags []string) {
	if d.Priority < 1 || d.Priority > 50 {
		d.Priority = 1
	}
	it := Item{Key: "downloader:" + d.URL, Name: clip(name, 80), From: from, Downloader: d, Detail: d.Adapter + " at " + d.URL}
	it.Detail += "; added paused: check it and resume it yourself"
	if d.NeedsInput {
		it.Needs = "your password"
		if d.Adapter == "sabnzbd" {
			it.Needs = "your key"
		}
		it.Detail = d.Adapter + " at " + d.URL + "; added paused until you type " + it.Needs + " and resume it"
	}
	b.addTagged(app, GroupDownloaders, it, tags)
}

// namingKeys maps their naming settings (lower case) to ours.
var namingKeys = map[string]string{
	"renamemovies": "rename_on_import", "renameepisodes": "rename_on_import", "renametracks": "rename_on_import",
	"colonreplacementformat": "colon", "replaceillegalcharacters": "illegal",
	"standardmovieformat": "file_pattern", "moviefolderformat": "folder_pattern",
	"seriesfolderformat": "show_folder_pattern", "seasonfolderformat": "season_folder_pattern",
	"standardepisodeformat": "episode_pattern", "dailyepisodeformat": "daily_pattern", "animeepisodeformat": "anime_pattern",
	"multiepisodestyle": "multi_episode_style",
}

// patternWords maps their pattern words to ours, longest first.
var patternWords = [][2]string{
	{"S{season:00}E{episode:00}", "{episode}"}, {"s{season:00}e{episode:00}", "{episode}"},
	{"{Movie TitleThe}", "{sort_title}"}, {"{Movie Title}", "{title}"}, {"{Movie CleanTitle}", "{title}"},
	{"{Series TitleThe}", "{sort_title}"}, {"{Series TitleYear}", "{title} ({year})"}, {"{Series Title}", "{title}"}, {"{Series CleanTitle}", "{title}"},
	{"{Release Year}", "{year}"}, {"{Series Year}", "{year}"}, {"{Quality Full}", "{quality}"}, {"{Quality Title}", "{quality}"},
	{"{Edition Tags}", "{edition}"}, {"{Release Group}", "{group}"}, {"{TmdbId}", "{tmdb}"}, {"{ImdbId}", "{imdb}"},
	{"{Original Title}", "{release}"}, {"{Episode Title}", "{episode_title}"}, {"{Episode CleanTitle}", "{episode_title}"},
	{"{Air-Date}", "{air_date}"}, {"{Air Date}", "{air_date}"}, {"{absolute:000}", "{absolute}"}, {"{absolute:00}", "{absolute}"},
	{"{season:00}", "{season_padded}"}, {"{season:0}", "{season}"},
}

// translatePattern rewrites one of their patterns in our words; ok is
// false when a word has no match.
func translatePattern(p string) (string, string, bool) {
	out := clip(p, 200)
	for _, w := range patternWords {
		out = strings.ReplaceAll(out, w[0], w[1])
	}
	// Every word left must be one of ours.
	for rest := out; ; {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			break
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			return "", "an unclosed {", false
		}
		w := rest[i+1 : i+j]
		if !ourWord(w) {
			return "", "uses {" + clip(w, 40) + "}, which has no HolyArrs word", false
		}
		rest = rest[i+j+1:]
	}
	return out, "", true
}

var ourWords = map[string]bool{"title": true, "sort_title": true, "year": true, "quality": true, "resolution": true, "source": true,
	"edition": true, "group": true, "tmdb": true, "imdb": true, "release": true, "season": true, "episode": true,
	"episode_title": true, "air_date": true, "absolute": true, "season_padded": true}

func ourWord(w string) bool { return ourWords[w] }

func (b *builder) naming(s *Snapshot) {
	b.cover("naming")
	if len(s.Naming) == 0 {
		return
	}
	keys := make([]string, 0, len(s.Naming))
	for k := range s.Naming {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]string{}
	var froms []string
	for _, k := range keys {
		v := s.Naming[k]
		from := b.from("naming", -1) + "." + k
		ours, ok := namingKeys[strings.ToLower(k)]
		if !ok {
			b.not(GroupNaming, from, k, "no HolyArrs naming setting matches it")
			continue
		}
		switch ours {
		case "rename_on_import":
			out[ours] = strconv.FormatBool(boolean(v))
		case "illegal":
			out[ours] = map[bool]string{true: "replace", false: "remove"}[boolean(v)]
		case "colon":
			c := map[string]string{"0": "remove", "delete": "remove", "1": "dash", "dash": "dash", "2": "space_dash", "spacedash": "space_dash",
				"3": "space_dash_space", "spacedashspace": "space_dash_space", "4": "smart", "smart": "smart"}[strings.ToLower(str(v))]
			if c == "" {
				b.not(GroupNaming, from, k, "this colon choice has no HolyArrs match")
				continue
			}
			out[ours] = c
		case "multi_episode_style":
			m := map[string]string{"0": "each", "extend": "each", "1": "each_dash", "duplicate": "each_dash", "2": "each", "repeat": "each",
				"3": "each_dash", "scene": "each_dash", "4": "range", "range": "range", "5": "short_range", "prefixedrange": "range"}[strings.ToLower(str(v))]
			if m == "" {
				b.not(GroupNaming, from, k, "this style has no HolyArrs match")
				continue
			}
			out[ours] = m
		default:
			pat, why, ok := translatePattern(str(v))
			if !ok || pat == "" || strings.ContainsAny(pat, `/\`) {
				if why == "" {
					why = "it names more than one folder"
				}
				b.not(GroupNaming, from, k, why)
				continue
			}
			out[ours] = pat
		}
		froms = append(froms, k)
	}
	if len(out) == 0 {
		return
	}
	b.add(GroupNaming, Item{Key: fmt.Sprintf("naming:%s", s.Kind), Name: GroupLabels[GroupNaming] + " from " + string(s.Kind), From: b.from("naming", -1),
		Detail: fmt.Sprintf("%d settings: %s", len(out), strings.Join(froms, ", ")), Why: "only settings with a HolyArrs match; nothing on disk is renamed", Naming: out})
}

// subtitleSiteKinds maps their subtitle site names to ours.
var subtitleSiteKinds = map[string]string{"podnapisi": "podnapisi", "gestdown": "gestdown"}

func (b *builder) subtitles(s *Snapshot) {
	b.cover("subtitles")
	sub := s.Subtitles
	if sub == nil {
		return
	}
	// The default profile's languages become the default wishes.
	def := num(sub.Defaults["movie_default_profile"])
	if def == 0 {
		def = num(sub.Defaults["serie_default_profile"])
	}
	var wants []SubtitleWant
	used := -1
	for i, p := range sub.Profiles {
		if p.ID == def && def != 0 {
			used = i
			for _, l := range p.Languages {
				if code := clip(l.Language, 8); code != "" && len(wants) < 20 {
					hi := "normal"
					if l.HI {
						hi = "hi"
					}
					wants = append(wants, SubtitleWant{Language: code, HearingImpaired: hi, Forced: l.Forced})
				}
			}
		}
	}
	langFrom := b.from("subtitles.languages", -1)
	if used < 0 {
		for _, l := range sub.Languages {
			if code := clip(l, 8); code != "" && len(wants) < 20 {
				wants = append(wants, SubtitleWant{Language: code, HearingImpaired: "normal"})
			}
		}
	} else {
		langFrom = b.from("subtitles.profiles", used)
		if len(sub.Languages) > 0 {
			b.not(GroupSubtitles, b.from("subtitles.languages", -1), strings.Join(sub.Languages, ", "), "the default profile's languages were moved; the other enabled languages are only choices in Bazarr")
		}
	}
	if len(wants) > 0 {
		var names []string
		for _, w := range wants {
			names = append(names, w.Language)
		}
		b.add(GroupSubtitles, Item{Key: "subtitles:languages", Name: "Default subtitle languages", From: langFrom,
			Detail: strings.Join(names, ", "), Why: "new movies and shows wish for these languages", Subtitles: &SubtitlePlan{Languages: wants}})
	}
	for i, p := range sub.Profiles {
		if i == used {
			continue
		}
		b.not(GroupSubtitles, b.from("subtitles.profiles", i), p.Name, "HolyArrs has one default list of languages and a list per title; set this one on the titles that used it")
	}
	for i, pr := range sub.Providers {
		from := b.from("subtitles.providers", i)
		kind := subtitleSiteKinds[strings.ToLower(pr.Kind)]
		if kind == "" {
			why := "HolyArrs has no subtitle site of this kind yet"
			if strings.EqualFold(pr.Kind, "addic7ed") {
				why = "HolyArrs reaches Addic7ed subtitles through Gestdown, with no account; add Gestdown under Settings > Subtitles"
			}
			b.not(GroupSubtitles, from, pr.Name, why)
			continue
		}
		b.add(GroupSubtitles, Item{Key: "subtitle_site:" + kind, Name: clip(pr.Name, 100), From: from, Detail: "subtitle site " + kind,
			Subtitles: &SubtitlePlan{SiteKind: kind}})
	}
	keys := make([]string, 0, len(sub.Defaults))
	for k := range sub.Defaults {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		from := b.from("subtitles.defaults", -1) + "." + k
		switch k {
		case "movie_default_profile", "serie_default_profile":
			if used >= 0 {
				continue
			}
		}
		b.not(GroupSubtitles, from, k, "no HolyArrs subtitle setting matches it")
	}
}

// personKey names a person from an app.
func personKey(app Kind, id int64) string { return fmt.Sprintf("person:%s:%d", app, id) }

func stablePersonKey(app Kind, u User) string {
	kind := strings.ToLower(clip(u.Kind, 20))
	external := clip(u.ExternalID, 100)
	if external == "" || kind == "" || strings.ContainsAny(kind+external, ":/\\") {
		return personKey(app, u.SourceID)
	}
	return fmt.Sprintf("person:%s:%s:%s", app, kind, external)
}

func weekly(limit, days int) *int {
	if limit <= 0 || limit > 1000 {
		return nil
	}
	if days != 7 {
		return nil
	}
	w := limit
	return &w
}

// defaultWeekly differs from a person's quota: an explicit zero in the old
// app means unlimited, while a missing value means there is no setting to move.
func defaultWeekly(settings map[string]any, kind string) *int {
	limit, ok := settings["default_"+kind+"_limit"]
	if !ok {
		return nil
	}
	n := int(num(limit))
	if n == 0 {
		return &n
	}
	return weekly(n, int(num(settings["default_"+kind+"_days"])))
}

func sourceTime(v string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func (b *builder) people(s *Snapshot) {
	b.cover("users", "settings")
	for i, u := range s.Users {
		from := b.from("users", i)
		name := clip(u.Name, 64)
		if name == "" {
			name = clip(u.Email, 64)
		}
		if name == "" {
			b.not(GroupPeople, from, "(no name)", "a person needs a name")
			continue
		}
		key := stablePersonKey(s.Kind, u)
		legacyKey := personKey(s.Kind, u.SourceID)
		b.personRefs[legacyKey] = key
		pp := &PersonPlan{App: string(s.Kind), SourceID: u.SourceID, StableIdentity: key != legacyKey, Email: clip(u.Email, 200), Role: "member", Perms: []string{}}
		has := func(n ...string) bool { return hasField(u.Permissions, n...) }
		// Roles cap at member: the owner confirms an admin, or that the old
		// app's owner is them, person by person on the move screen.
		pp.WasOwner = u.Owner
		pp.WasAdmin = !u.Owner && (u.Admin || has("ADMIN"))
		choice := b.choices[key]
		owner := pp.WasOwner && choice == PersonOwner
		switch {
		case owner:
			pp.Role = "owner"
		case pp.WasAdmin && choice == PersonAdmin:
			pp.Role = "admin"
		}
		if pp.WasAdmin && pp.Role == "admin" {
			b.not(GroupPeople, from+".permissions", name+": ADMIN", "the confirmed HolyArrs admin role keeps account administration; other source-admin capabilities are not granted automatically")
		}
		if choice == PersonMerge || choice == PersonSkip {
			pp.Choice = choice
		}
		add := func(p string, ok bool) {
			if ok {
				pp.Perms = append(pp.Perms, p)
			}
		}
		add("ask", has("REQUEST") || pp.Role != "member")
		add("auto_approve", has("AUTO_APPROVE"))
		add("approve", has("MANAGE_REQUESTS"))
		add("settings", has("MANAGE_SETTINGS") && pp.Role == "admin")
		if has("MANAGE_SETTINGS") && pp.Role != "admin" {
			b.not(GroupPeople, from+".permissions", name+": MANAGE_SETTINGS", "settings access requires a confirmed HolyArrs admin role, so it is not granted")
		}
		var dropped []string
		for _, p := range u.Permissions {
			switch p {
			case "ADMIN", "REQUEST", "AUTO_APPROVE", "MANAGE_REQUESTS", "MANAGE_SETTINGS":
				continue
			}
			dropped = append(dropped, clip(p, 40))
		}
		if len(dropped) > 0 {
			b.not(GroupPeople, from+".permissions", name+": "+strings.Join(dropped, ", "), "no HolyArrs permission matches these")
		}
		pp.MoviesPerWeek = weekly(u.MovieLimit, u.MovieDays)
		pp.ShowsPerWeek = weekly(u.ShowLimit, u.ShowDays)
		if u.MovieLimit < 0 || u.MovieLimit > 1000 {
			b.not(GroupPeople, from+".movie_quota", name, "the per-person quota is outside HolyArrs' supported range of 1 to 1000, so it is not approximated")
		} else if u.MovieLimitSet && u.MovieLimit == 0 {
			b.not(GroupPeople, from+".movie_quota", name, "an explicit zero per-person quota has no established source meaning, so it is left at the HolyArrs default")
		} else if u.MovieLimit > 0 && pp.MoviesPerWeek == nil {
			b.not(GroupPeople, from+".movie_quota", name, "only an exact seven-day quota can be moved without changing its access window")
		}
		if u.ShowLimit < 0 || u.ShowLimit > 1000 {
			b.not(GroupPeople, from+".show_quota", name, "the per-person quota is outside HolyArrs' supported range of 1 to 1000, so it is not approximated")
		} else if u.ShowLimitSet && u.ShowLimit == 0 {
			b.not(GroupPeople, from+".show_quota", name, "an explicit zero per-person quota has no established source meaning, so it is left at the HolyArrs default")
		} else if u.ShowLimit > 0 && pp.ShowsPerWeek == nil {
			b.not(GroupPeople, from+".show_quota", name, "only an exact seven-day quota can be moved without changing its access window")
		}
		detail := "Comes over turned off as a " + pp.Role + ". Invite them to sign in; their password stays behind."
		why := ""
		switch {
		case owner:
			detail = "This is you. Their requests become yours."
			why = "you confirmed they are you (the owner in " + string(s.Kind) + ")"
		case pp.WasOwner:
			why = "Owner in " + kindName(s.Kind) + ". Turn on This is me if it's you."
		case pp.WasAdmin && pp.Role == "admin":
			why = "an admin in " + string(s.Kind) + "; you confirmed the admin role"
		case pp.WasAdmin:
			why = "an admin in " + string(s.Kind) + "; comes over as a member unless you confirm the admin role"
		}
		if pp.MoviesPerWeek != nil {
			detail += fmt.Sprintf("; %d movie asks a week", *pp.MoviesPerWeek)
		}
		if pp.ShowsPerWeek != nil {
			detail += fmt.Sprintf("; %d show asks a week", *pp.ShowsPerWeek)
		}
		b.planned[key] = true
		var legacy []string
		if legacyKey != key {
			legacy = []string{legacyKey}
		}
		b.add(GroupPeople, Item{Key: key, LegacyKeys: legacy, Name: name, From: from, Detail: detail, Why: why, Person: pp,
			Needs: map[bool]string{true: "", false: "an invite"}[owner]})
	}
	recognized := map[string]bool{}
	movies := defaultWeekly(s.Settings, "movie")
	shows := defaultWeekly(s.Settings, "show")
	accountDefault := func(kind string, planned *int) {
		limitKey := "default_" + kind + "_limit"
		daysKey := "default_" + kind + "_days"
		limitRaw, hasLimit := s.Settings[limitKey]
		_, hasDays := s.Settings[daysKey]
		for _, key := range []string{limitKey, daysKey} {
			if _, ok := s.Settings[key]; ok {
				recognized[key] = true
			}
		}
		from := b.from("settings", -1)
		if planned != nil {
			b.p.Mapped = append(b.p.Mapped, from+"."+limitKey)
			if *planned > 0 && hasDays {
				b.p.Mapped = append(b.p.Mapped, from+"."+daysKey)
			} else if hasDays {
				b.not(GroupRequests, from+"."+daysKey, kind+" default", "the quota is unlimited, so its old access-window length is not used")
			}
			return
		}
		if hasLimit {
			n := int(num(limitRaw))
			reason := "only an exact seven-day quota can be moved without changing its access window"
			if n < 0 || n > 1000 {
				reason = "the default quota is outside HolyArrs' supported range of 0 to 1000, so it is not approximated"
			}
			b.not(GroupRequests, from+"."+limitKey, kind+" default", reason)
		}
		if hasDays {
			reason := "the access-window length has no matching default quota to move"
			if hasLimit {
				reason = "the default quota pair cannot be moved without changing its meaning"
			}
			b.not(GroupRequests, from+"."+daysKey, kind+" default", reason)
		}
	}
	accountDefault("movie", movies)
	accountDefault("show", shows)
	if movies != nil || shows != nil {
		as := &AskSettingsPlan{MoviesPerWeek: movies, ShowsPerWeek: shows}
		b.add(GroupRequests, Item{Key: "wishlist_defaults", Name: "Request defaults", From: b.from("settings", -1),
			Detail: "default weekly limits", AskSettings: as})
	}
	// Radarr's availability delay is a Release gate setting (titleList).
	recognized["availability_delay"] = s.Kind == Radarr
	keys := make([]string, 0, len(s.Settings))
	for k := range s.Settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if recognized[k] {
			continue
		}
		b.not(GroupPeople, b.from("settings", -1)+"."+k, k, "app-wide settings of the old app are not moved; set the Request defaults under Settings > Request rules")
	}
}

func (b *builder) asks(s *Snapshot) {
	b.cover("requests")
	for i, r := range s.Requests {
		from := b.from("requests", i)
		var kind, ref string
		ids := map[string]string{}
		for _, src := range []string{"tmdb", "tvdb", "imdb"} {
			if v := clip(r.IDs[src], 20); v != "" && (src == "imdb" || isDigits(v)) {
				ids[src] = v
			}
		}
		switch r.Kind {
		case "movie":
			kind = "movie"
			if ids["tmdb"] != "" {
				ref = "tmdb:" + ids["tmdb"]
			}
		case "show":
			kind = "show"
			for _, src := range []string{"tvdb", "tmdb"} {
				if ids[src] != "" && ref == "" {
					ref = src + ":" + ids[src]
				}
			}
		}
		if ref == "" {
			b.not(GroupRequests, from, "ask "+strconv.FormatInt(r.SourceID, 10), "no id HolyArrs can find it by")
			continue
		}
		name := b.titles[kind+":"+ref]
		if name == "" {
			for _, v := range ids {
				if n := b.titles[kind+":ids:"+v]; n != "" {
					name = n
				}
			}
		}
		if name == "" {
			name = strings.ToUpper(ref[:strings.IndexByte(ref, ':')]) + " " + ref[strings.IndexByte(ref, ':')+1:]
		}
		byKey := b.personRefs[personKey(s.Kind, r.By)]
		ap := &AskPlan{App: string(s.Kind), Kind: kind, Ref: ref, IDs: ids, Approved: r.State == "approved",
			By: byKey, ByName: clip(r.ByName, 64), AskedAt: clip(r.Created, 40), DecidedAt: clip(r.Updated, 40)}
		if ap.AskedAt != "" {
			if asked, ok := sourceTime(ap.AskedAt); !ok {
				b.not(GroupRequests, from+".created", ap.AskedAt, "the source timestamp format is invalid, so the move cannot preserve it")
				ap.AskedAt = ""
			} else if s.ReadAt.IsZero() {
				b.not(GroupRequests, from+".created", ap.AskedAt, "this kept read has no capture time, so the source timestamp cannot be verified")
				ap.AskedAt = ""
			} else if asked.After(s.ReadAt) {
				b.not(GroupRequests, from+".created", ap.AskedAt, "the source timestamp is after this kept read, so the move cannot preserve it")
				ap.AskedAt = ""
			}
		} else {
			b.not(GroupRequests, from+".created", "(missing)", "the source has no request timestamp, so the move cannot preserve it")
		}
		if ap.Approved && ap.DecidedAt == "" {
			b.not(GroupRequests, from+".updated", "(missing)", "the source has no decision timestamp, so the move cannot preserve it")
		} else if ap.DecidedAt != "" {
			if decided, ok := sourceTime(ap.DecidedAt); !ok {
				b.not(GroupRequests, from+".updated", ap.DecidedAt, "the source decision timestamp format is invalid, so the move cannot preserve it")
				ap.DecidedAt = ""
			} else if s.ReadAt.IsZero() {
				b.not(GroupRequests, from+".updated", ap.DecidedAt, "this kept read has no capture time, so the source decision timestamp cannot be verified")
				ap.DecidedAt = ""
			} else if decided.After(s.ReadAt) {
				b.not(GroupRequests, from+".updated", ap.DecidedAt, "the source decision timestamp is after this kept read, so the move cannot preserve it")
				ap.DecidedAt = ""
			}
		}
		if !b.planned[ap.By] {
			ap.By = ""
		}
		if k := b.personRefs[personKey(s.Kind, r.DecidedBy)]; r.DecidedBy != 0 && b.planned[k] {
			ap.Decider = k
		}
		for _, n := range r.Seasons {
			if n >= 0 && n < 10000 && len(ap.Seasons) < 200 {
				ap.Seasons = append(ap.Seasons, n)
			}
		}
		if r.FourK {
			ap.Note = "asked for 4K in " + string(s.Kind)
		}
		state := "waiting"
		if ap.Approved {
			state = "approved"
		}
		by := ap.ByName
		if by == "" {
			by = "someone"
		}
		b.add(GroupRequests, Item{Key: fmt.Sprintf("ask:%s:%d", s.Kind, r.SourceID), Name: name, From: from,
			Detail: state + ", asked by " + by, Ask: ap})
	}
}

func (b *builder) alerts(s *Snapshot) {
	b.cover("notifications")
	for i, n := range s.Notifications {
		from := b.from("notifications", i)
		var ap *AlertPlan
		switch strings.ToLower(n.Kind) {
		case "discord":
			ap = &AlertPlan{Kind: "discord"}
		case "webhook":
			ap = &AlertPlan{Kind: "webhook"}
		case "email":
			port, _ := setNum(n.Settings, "port")
			sec := "starttls"
			switch strings.ToLower(setStr(n.Settings, "useEncryption", "requireEncryption")) {
			case "2", "never", "false":
				sec = "none"
			case "1", "always":
				sec = "tls"
			}
			if port <= 0 || port > 65535 {
				port = 587
			}
			to := setStr(n.Settings, "to")
			if v := list(anyKey(n.Settings, "to")); len(v) > 0 {
				to = str(v[0])
			}
			ap = &AlertPlan{Kind: "email", SMTPHost: clip(setStr(n.Settings, "server", "host"), 200), SMTPPort: int(port), SMTPSecurity: sec,
				SMTPFrom: clip(setStr(n.Settings, "from"), 200), SMTPUser: clip(setStr(n.Settings, "username", "user"), 200), StaffTo: clip(to, 200)}
		}
		if ap == nil {
			b.not(GroupAlerts, from, n.Name, "HolyArrs Alerts go to Discord, a webhook or email only")
			continue
		}
		needs := map[string]string{"discord": "its Discord link", "webhook": "its address", "email": "the mail password"}[ap.Kind]
		b.addTagged(s.Kind, GroupAlerts, Item{Key: "alert:" + strings.ToLower(clip(n.Name, 80)), Name: clip(n.Name, 80), From: from, Alert: ap,
			Detail: ap.Kind + "; added off until you type " + needs, Needs: needs,
			Why: "Alerts speak for every moment at first; pick the moments again"}, n.Tags)
	}
}

// leftBehind lists the parts of a Snapshot the move never brings.
func (b *builder) leftBehind(s *Snapshot) {
	b.cover("tags", "import_lists", "metadata", "app_links", "app_profiles")
	for i, t := range s.Tags {
		b.not(GroupOther, b.from("tags", i), t.Label, "HolyArrs has no tags")
	}
	for i, l := range s.ImportLists {
		if s.Kind == Radarr && b.outsideList(i, l) {
			continue
		}
		kind := strings.ToLower(l.Kind)
		var why string
		switch {
		case s.Kind == Sonarr:
			why = "HolyArrs Outside lists add movies only for now; this show list stays in Sonarr"
		case s.Kind == Lidarr:
			why = "HolyArrs Outside lists add movies only for now; this music list stays in Lidarr"
		case strings.Contains(kind, "radarr") || strings.Contains(kind, "sonarr"):
			why = "HolyArrs is one app; it does not copy from another instance"
		// Best effort: the TMDB list kind and the Trakt list kinds other than
		// popular lists can be added again here; the other kinds cannot.
		case kind == "tmdblistimport" || (strings.Contains(kind, "trakt") && !strings.Contains(kind, "popular")):
			why = "HolyArrs reads TMDB lists and public Trakt lists; add it again under Settings > Outside lists"
			if strings.Contains(kind, "trakt") {
				why += " (a private list needs to be made public first)"
			}
		default:
			why = "HolyArrs reads TMDB lists and public Trakt lists only; this list stays in Radarr"
		}
		if len(l.NeedsReentry) > 0 {
			why += "; its sign-in does not come over"
		}
		b.not(GroupOther, b.from("import_lists", i), l.Name, why)
	}
	for i, m := range s.Metadata {
		b.not(GroupOther, b.from("metadata", i), m.Name, "HolyArrs writes no metadata files for media players")
	}
	for i, l := range s.AppLinks {
		b.not(GroupOther, b.from("app_links", i), l.Name, "HolyArrs is one app; it needs no links between its parts")
	}
	for i := range s.AppProfiles {
		b.not(GroupOther, b.from("app_profiles", i), "sync profile", "HolyArrs is one app; nothing is synced between apps")
	}
}

// listSlug turns a Trakt list's name as Radarr keeps it into its address
// part: lower case, every run of other characters one dash.
func listSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// freeListName is name, or name with " (radarr)", " (radarr 2)" and so on
// when taken says that one is in use.
func freeListName(name string, taken func(string) bool) string {
	if !taken(name) {
		return name
	}
	for n := 1; n < 100; n++ {
		end := " (radarr)"
		if n > 1 {
			end = fmt.Sprintf(" (radarr %d)", n)
		}
		if c := clip(name, 80-len(end)) + end; !taken(c) {
			return c
		}
	}
	return name
}

// syncHours reads a list's refresh gap ("12:00:00" or "1.00:00:00") as
// Outside list hours, 6 to 168; 12 when it cannot be read.
func syncHours(v any) int {
	s := strings.TrimSpace(str(v))
	days := 0
	if d, rest, ok := strings.Cut(s, "."); ok && strings.Count(rest, ":") == 2 {
		days, _ = strconv.Atoi(d)
		s = rest
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 12
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || days < 0 || h < 0 || m < 0 {
		return 12
	}
	hours := days*24 + h
	if m > 0 {
		hours++
	}
	return min(max(hours, 6), 168)
}

// outsideList plans one Radarr list as a paused Outside list: a TMDB list,
// a Trakt list or a Trakt watchlist (lists.NormalRef checks its reference).
// It says false for every other kind, which leftBehind lists.
func (b *builder) outsideList(i int, l Provider) bool {
	from := b.from("import_lists", i)
	set := l.Settings
	name := clip(l.Name, 80)
	var kind, ref string
	switch strings.ToLower(l.Kind) {
	case "tmdblistimport":
		kind, ref = lists.KindTMDB, str(anyKey(set, "listId"))
	case "traktlistimport":
		kind, ref = lists.KindTrakt, setStr(set, "username", "authUser")+"/"+listSlug(str(anyKey(set, "listname")))
	case "traktuserimport":
		switch str(anyKey(set, "traktListType")) {
		case "0", "userWatchList", "":
			kind, ref = lists.KindTrakt, setStr(set, "username", "authUser")+"/watchlist"
		default:
			b.not(GroupLists, from, l.Name, "HolyArrs reads Trakt lists and watchlists; a watched or collection list stays in Radarr")
			return true
		}
	default:
		return false
	}
	norm, err := lists.NormalRef(kind, ref)
	if err != nil {
		why := "its TMDB list number did not come over; add it again under Settings > Outside lists"
		if kind == lists.KindTrakt {
			why = "its Trakt user or list name did not come over; add it again under Settings > Outside lists"
		}
		b.not(GroupLists, from, l.Name, why)
		return true
	}
	folder := oldPath(str(anyKey(set, "link.rootFolderPath")))
	lib := ""
	for _, f := range b.folders {
		if f.Kind == "movie" && f.OldPath == folder && folder != "" {
			lib = f.Path
		}
	}
	if lib == "" {
		b.not(GroupLists, from, l.Name, "its library folder "+clip(folder, 200)+" did not come over")
		return true
	}
	if name == "" {
		name = kind + " " + norm
	}
	name = freeListName(name, func(n string) bool { return b.listNames[strings.ToLower(n)] })
	b.listNames[strings.ToLower(name)] = true
	op := &OutsideListPlan{Kind: kind, Ref: norm, Library: lib, KeepUpToDate: true, SearchOnAdd: boolean(anyKey(set, "link.searchOnAdd")),
		Preset: b.presets[string(Radarr)+":"+strconv.FormatInt(num(anyKey(set, "link.qualityProfileId")), 10)], SyncEveryHours: syncHours(anyKey(set, "link.minRefreshInterval"))}
	switch strings.ToLower(str(anyKey(set, "link.monitor"))) {
	case "none", "2":
		op.KeepUpToDate = false
	case "movieandcollection", "1":
		b.not(GroupLists, from+".monitor", l.Name+": its movie series", "lists add only the movies on the list, not the rest of their movie series; add those by hand")
	}
	if v := str(anyKey(set, "link.minimumAvailability")); v != "" {
		op.ReleaseGate = releaseGates[strings.ToLower(v)]
	}
	// Radarr reads a list only while it is on, and adds its movies only
	// while its automatic add is on too.
	on := l.Enabled
	if v := anyKey(set, "link.enabled"); v != nil {
		on = boolean(v)
	}
	was := "was off there"
	switch auto := anyKey(set, "link.enableAuto"); {
	case on && auto != nil && !boolean(auto):
		was = "was on there but added no movies by itself"
	case on:
		was = "was on there"
	}
	detail := fmt.Sprintf("%s %s into %s; added paused (%s), every %d hours", map[string]string{lists.KindTMDB: "TMDB list", lists.KindTrakt: "Trakt list"}[kind], norm, lib, was, op.SyncEveryHours)
	if op.ReleaseGate != "" {
		detail += "; release gate: " + gateWords[op.ReleaseGate]
	}
	if !op.KeepUpToDate {
		detail += "; Keep up to date off"
	}
	if kind == lists.KindTrakt {
		detail += "; needs your Trakt key, and the list must be public"
	}
	b.add(GroupLists, Item{Key: "outside_list:" + kind + ":" + strings.ToLower(norm), Name: name, From: from, Detail: detail,
		Why: "added paused, like Sources and Downloaders: check its folder and Pick rules, then turn it on", OutsideList: op})
	return true
}

// metadataHost says an address names a link-local or cloud metadata
// address (the list NotMetadata in core/outbound refuses), which no Source or
// Downloader may reach.
func metadataHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	h := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.Equal(net.ParseIP("fd00:ec2::254")) || ip.Equal(net.ParseIP("100.100.100.200"))
	}
	return h == "metadata" || strings.HasPrefix(h, "metadata.") || strings.HasPrefix(h, "instance-data")
}

// PlanHash names a Plan exactly: a move starts only from the plan its dry
// run showed, and another one is refused (ErrPlanChanged).
func PlanHash(p *Plan) string {
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

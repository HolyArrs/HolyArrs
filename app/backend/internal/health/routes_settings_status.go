package health

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// StatusItem is one thing in Settings that needs attention. Group is one of
// general, library, copies, people, security, linked, maintenance, account;
// Severity is info, warning or error.
type StatusItem struct {
	Group    string `json:"group"`
	Page     string `json:"page"`
	Text     string `json:"text"`
	Severity string `json:"severity"`
}

// StatusAnswer is the answer of GET /settings/status.
type StatusAnswer struct {
	Items []StatusItem `json:"items"`
	// Unset lists, per group, the pages not set up yet.
	Unset map[string][]string `json:"unset"`
	// Known lists the pages whose status is known.
	Known []string `json:"known"`
	// MetadataUp says the metadata server answers.
	MetadataUp bool `json:"metadata_up"`
}

type NamedState struct{ Name, State string }

// StatusInput is what the status rules look at. A nil field is not known.
type StatusInput struct {
	Checks  []domain.HealthCheck
	States  []jobs.Status
	Folders *int
	Sources []NamedState
	// SourcesNear are the words for Sources close to a search or pick limit.
	SourcesNear []string
	Downloaders []NamedState
	// NewestBackup is the newest backup's time (RFC 3339), "" for none.
	NewestBackup *string
	MetadataUp   *bool
	UpdateNewer  *bool
	// GoneDefaults are the plain-words notes for saved Movies and TV
	// defaults whose folder or Pick rules are gone.
	GoneDefaults []string
	// DefaultsKnown says the Movies and TV defaults were read.
	DefaultsKnown bool
	// SkipLocal says sign-in is skipped on the local network.
	SkipLocal *bool
	// Kids is how many people have an age-rating limit.
	Kids *int
	// OutsideJoin says new people can join by signing in with Plex,
	// Jellyfin, Emby or single sign-on.
	OutsideJoin *bool
	Now         time.Time
}

type statusTarget struct{ page, group, what string }

var checkTarget = map[string]statusTarget{
	"library_folder": {"/settings/library-folders", "library", ""},
	"disk_space":     {"/settings/library-folders", "library", ""},
	"source":         {"/settings/sources", "copies", ""},
	"downloader":     {"/settings/downloaders", "copies", ""},
	"backups":        {"/settings/backups", "maintenance", ""},
	"update":         {"/settings/this-app", "general", ""},
}

var stateTarget = map[string]statusTarget{
	"media_server":  {"/settings/media-servers", "linked", "media server"},
	"subtitle_site": {"/settings/subtitles", "library", "subtitle site"},
	"stage":         {"/settings/parts", "general", "stage"},
}

// settingsStatus lists what needs a fix and which groups are not set up yet.
func SettingsStatus(in StatusInput) StatusAnswer {
	a := StatusAnswer{Items: []StatusItem{}, Unset: map[string][]string{}, Known: []string{}}
	seen := map[string]bool{}
	known := func(p string) {
		if !slices.Contains(a.Known, p) {
			a.Known = append(a.Known, p)
		}
	}
	add := func(i StatusItem, key string) {
		if seen[key] {
			return
		}
		seen[key] = true
		a.Items = append(a.Items, i)
	}
	for _, c := range in.Checks {
		to, ok := checkTarget[c.Kind]
		if !ok {
			continue
		}
		known(to.page)
		if c.State == OK {
			continue
		}
		text := c.Message
		if text == "" {
			text = "needs a look"
		}
		if c.Subject != "" {
			text = c.Subject + ": " + text
		}
		sev := "warning"
		if c.State == "error" {
			sev = "error"
		}
		add(StatusItem{to.group, to.page, text, sev}, c.Kind+":"+c.Subject)
	}
	for _, s := range in.States {
		to, ok := stateTarget[s.Kind]
		if !ok {
			continue
		}
		known(to.page)
		if s.State != jobs.Failed && !s.Stuck {
			continue
		}
		text, sev := "The "+to.what+" "+s.Name+" failed", "error"
		if s.Stuck {
			text, sev = "The "+to.what+" "+s.Name+" is stuck", "warning"
		}
		add(StatusItem{to.group, to.page, text, sev}, "state:"+s.Key)
	}
	if in.Sources != nil {
		known("/settings/sources")
		for _, s := range in.Sources {
			switch s.State {
			case sources.StateUnreachable, sources.StateAuthFailed:
				add(StatusItem{"copies", "/settings/sources", "The Source " + s.Name + " is not answering", "error"}, "source:"+s.Name)
			case sources.KindRateLimited:
				add(StatusItem{"copies", "/settings/sources", "The Source " + s.Name + " asked HolyArrs to slow down, so it waits a while", "warning"}, "source:"+s.Name)
			case sources.KindBlocked:
				add(StatusItem{"copies", "/settings/sources", "The Source " + s.Name + " is blocked from the local network", "warning"}, "source:"+s.Name)
			case sources.BlockBotCheck:
				add(StatusItem{"copies", "/settings/sources", "The Source " + s.Name + " is behind a bot check HolyArrs cannot pass", "warning"}, "source:"+s.Name)
			case sources.BlockSiteBan:
				add(StatusItem{"copies", "/settings/sources", "The site behind the Source " + s.Name + " refuses HolyArrs's address", "warning"}, "source:"+s.Name)
			}
		}
		for _, text := range in.SourcesNear {
			add(StatusItem{"copies", "/settings/sources", text, "warning"}, "source-near:"+text)
		}
	}
	if in.Downloaders != nil {
		known("/settings/downloaders")
		for _, d := range in.Downloaders {
			if d.State == "not_answering" || d.State == "sign_in_refused" {
				add(StatusItem{"copies", "/settings/downloaders", "The Downloader " + d.Name + " is not answering", "error"}, "downloader:"+d.Name)
			}
		}
	}
	a.MetadataUp = in.MetadataUp == nil || *in.MetadataUp
	if in.MetadataUp != nil {
		known("/settings/tmdb")
		if !*in.MetadataUp {
			add(StatusItem{"library", "/settings/tmdb", metadata.Down, "warning"}, "metadata")
		}
	}
	if in.NewestBackup != nil {
		known("/settings/backups")
		if *in.NewestBackup == "" {
			add(StatusItem{"maintenance", "/settings/backups", "No backup yet", "warning"}, "backup-age")
		} else if at, err := time.Parse(time.RFC3339, *in.NewestBackup); err == nil && in.Now.Sub(at) > 7*24*time.Hour {
			add(StatusItem{"maintenance", "/settings/backups", "No backup in the last 7 days", "warning"}, "backup-age")
		}
	}
	if in.UpdateNewer != nil && *in.UpdateNewer {
		known("/settings/this-app")
		add(StatusItem{"general", "/settings/this-app", "A newer HolyArrs is out", "info"}, "update:")
	}
	if in.DefaultsKnown {
		known("/settings/title-defaults")
		for _, text := range in.GoneDefaults {
			add(StatusItem{"library", "/settings/title-defaults", text, "warning"}, "title-defaults:"+text)
		}
	}
	if in.SkipLocal != nil && in.Kids != nil {
		known("/settings/sign-in")
		if *in.SkipLocal && *in.Kids > 0 {
			add(StatusItem{"security", "/settings/sign-in", "Sign-in is skipped on the local network while a Kid account exists, so anyone at home is the owner and the age limit means nothing; turn skipping off", "error"}, "kid-skip-local")
		}
	}
	if in.OutsideJoin != nil && in.Kids != nil {
		known("/settings/users")
		if *in.OutsideJoin && *in.Kids > 0 {
			add(StatusItem{"people", "/settings/users", "New people who sign in with Plex, Jellyfin, Emby or single sign-on get no age limit; link a Kid's outside accounts to their Kid account.", "warning"}, "kid-outside-join")
		}
	}
	if in.Folders != nil {
		if *in.Folders == 0 {
			a.Unset["library"] = []string{"/settings/library-folders"}
		}
		known("/settings/library-folders")
	}
	var copies []string
	if in.Sources != nil && len(in.Sources) == 0 {
		copies = append(copies, "/settings/sources")
	}
	if in.Downloaders != nil && len(in.Downloaders) == 0 {
		copies = append(copies, "/settings/downloaders")
	}
	if len(copies) > 0 {
		a.Unset["copies"] = copies
	}
	return a
}

// StatusCache keeps the status answer for 30 seconds, for everyone.
type StatusCache struct {
	mu     sync.Mutex
	at     time.Time
	answer *StatusAnswer
	Now    func() time.Time // nil: time.Now
}

const statusFor = 30 * time.Second

func (c *StatusCache) clock() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Forget drops the kept answer.
func (c *StatusCache) Forget() {
	c.mu.Lock()
	c.answer = nil
	c.mu.Unlock()
}

// statusWrites are the paths whose writes can change a status input:
// Settings, Library folders, Sources, Downloaders, backups, health checks
// and Pick rules.
var statusWrites = []string{"/api/v1/settings/", "/api/v1/library-folders", "/api/v1/sources", "/api/v1/downloaders",
	"/api/v1/backups", "/api/v1/health/", "/api/v1/pick-rules"}

// ForgetOnWrite drops the kept status answer after each successful write
// to a statusWrites path, so Settings home shows the change at once.
func (c *StatusCache) ForgetOnWrite(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || !slices.ContainsFunc(statusWrites, func(p string) bool { return strings.HasPrefix(r.URL.Path, p) }) {
			h.ServeHTTP(w, r)
			return
		}
		sw := &webserver.StatusWriter{ResponseWriter: w}
		h.ServeHTTP(sw, r)
		if sw.Code < 400 { // 0: nothing written, a plain 200
			c.Forget()
		}
	})
}

// StatusHandlers serve GET /settings/status. Its reads of other modules
// (backups, the update check, sign-in) come in as funcs; a nil one is not
// known.
type StatusHandlers struct {
	Store  *database.Store
	Engine *Engine
	States *jobs.Registry
	// PartStatuses lists the parts' switches (nil: none).
	PartStatuses func() []jobs.Status
	// NewestBackup is the newest backup's time ("" none); ok false when
	// backups are not known.
	NewestBackup func() (newest string, ok bool)
	// MetadataUp says whether the metadata server answers; known false
	// when that is not known.
	MetadataUp  func(context.Context) (up, known bool)
	UpdateNewer func(context.Context) bool
	// SkipLocal and OutsideJoin are the sign-in settings (nil: no sign-in).
	SkipLocal   func() bool
	OutsideJoin func(context.Context) bool
	Cache       *StatusCache
}

// Routes mounts GET /api/v1/settings/status.
func (a *StatusHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/status", a.settingsStatusRoute)
}

func (a *StatusHandlers) settingsStatusRoute(w http.ResponseWriter, r *http.Request) {
	c := a.Cache
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	if c.answer == nil || now.Sub(c.at) >= statusFor {
		in, err := a.statusInput(r.Context(), now)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		ans := SettingsStatus(in)
		c.answer, c.at = &ans, now
	}
	out := *c.answer
	if NotOwner(r) {
		// The answer is shared; reword the key kit warning on a copy:
		// only the owner can download the kit.
		out.Items = slices.Clone(out.Items)
		for i := range out.Items {
			if strings.Contains(out.Items[i].Text, "Download your key kit") {
				out.Items[i].Text = "Backups: " + KeyKitForOthers
			}
		}
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// StatusInput gathers what the rules look at, including whether the
// metadata server answers.
func (a *StatusHandlers) statusInput(ctx context.Context, now time.Time) (StatusInput, error) {
	in := StatusInput{Now: now}
	db := a.Store.Read()
	checks, err := List(ctx, a.Store)
	if err != nil {
		return in, err
	}
	in.Checks = checks
	var states []jobs.Status
	if a.States != nil {
		states = a.States.List()
	}
	if a.PartStatuses != nil {
		states = append(states, a.PartStatuses()...)
	}
	links, err := a.Engine.Links(ctx)
	if err != nil {
		return in, err
	}
	in.States = append(states, links...)
	var folders int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM library_folders").Scan(&folders); err != nil {
		return in, err
	}
	in.Folders = &folders
	srcs, err := database.Select[domain.Source](ctx, db, "sources", "1=1 ORDER BY priority, name")
	if err != nil {
		return in, err
	}
	in.Sources = []NamedState{}
	for _, s := range srcs {
		state := sources.HealthOf(s).State
		// Its own backoff for slowing down or a blocked address is a warning,
		// not a Source that stopped answering.
		if s.State == "paused" && s.PauseReason != nil && (*s.PauseReason == sources.KindRateLimited || *s.PauseReason == sources.KindBlocked) {
			state = *s.PauseReason
		}
		if b := sources.BlockOf(s); b != "" {
			state = b // a site's bot check or ban, not the local network
		}
		in.Sources = append(in.Sources, NamedState{s.Name, state})
		if s.State != "disabled" {
			if near, text := sources.NearCap(ctx, a.Store, s, now); near {
				in.SourcesNear = append(in.SourcesNear, text)
			}
		}
	}
	dls, err := database.Select[domain.Downloader](ctx, db, "downloaders", "removed = 0 ORDER BY priority, name")
	if err != nil {
		return in, err
	}
	in.Downloaders = []NamedState{}
	for _, d := range dls {
		in.Downloaders = append(in.Downloaders, NamedState{d.Name, downloads.HealthOf(d, now).State})
	}
	if a.NewestBackup != nil {
		if newest, ok := a.NewestBackup(); ok {
			in.NewestBackup = &newest
		}
	}
	if a.MetadataUp != nil {
		if up, known := a.MetadataUp(ctx); known {
			in.MetadataUp = &up
		}
	}
	if a.UpdateNewer != nil {
		newer := a.UpdateNewer(ctx)
		in.UpdateNewer = &newer
	}
	gone, err := goneDefaults(ctx, a.Store)
	if err != nil {
		return in, err
	}
	in.GoneDefaults, in.DefaultsKnown = gone, true
	var kids int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE max_rating != ''").Scan(&kids); err != nil {
		return in, err
	}
	in.Kids = &kids
	if a.SkipLocal != nil {
		skip := a.SkipLocal()
		in.SkipLocal = &skip
	}
	if a.OutsideJoin != nil {
		join := a.OutsideJoin(ctx)
		in.OutsideJoin = &join
	}
	return in, nil
}

// goneDefaults words each saved Movies and TV default whose folder or Pick
// rules no longer exist (adds then use the built-in choice).
func goneDefaults(ctx context.Context, st *database.Store) ([]string, error) {
	d, saved, err := library.ReadTitleDefaults(ctx, st.Read())
	if err != nil {
		return nil, err
	}
	var out []string
	for _, k := range []struct {
		kind, many, folder string
		kd                 library.KindDefaults
	}{{"movie", "movies", "movie", d.Movie}, {"show", "shows", "show", d.Show}} {
		if saved[k.kind+"_library_folder_id"] && k.kd.LibraryFolderID != 0 && !library.UsableDefault(ctx, st.Read(), "library_folders", k.kind, k.kd.LibraryFolderID) {
			out = append(out, "The Library folder saved for new "+k.many+" is gone, so new "+k.many+" go to the first "+k.folder+" folder")
		}
		if saved[k.kind+"_pick_rules_id"] && k.kd.PickRulesID != 0 && !library.UsableDefault(ctx, st.Read(), "pick_rules", k.kind, k.kd.PickRulesID) {
			out = append(out, "The Pick rules saved for new "+k.many+" are gone, so new "+k.many+" use their folder's own Pick rules")
		}
	}
	return out, nil
}

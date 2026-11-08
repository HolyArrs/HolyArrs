package health

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
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

// SettingsStatus lists what needs a fix and which groups are not set up yet.
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

// statusCache keeps the status answer for 30 seconds, for everyone.
type statusCache struct {
	mu     sync.Mutex
	at     time.Time
	answer *StatusAnswer
	now    func() time.Time // nil: time.Now
}

const statusFor = 30 * time.Second

func (c *statusCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// forget drops the kept answer.
func (c *statusCache) forget() {
	c.mu.Lock()
	c.answer = nil
	c.mu.Unlock()
}

// statusWrites are the paths whose writes can change a status input:
// Settings, Library folders, Sources, Downloaders, backups, health checks
// and Pick rules.
var statusWrites = []string{"/api/v1/settings/", "/api/v1/library-folders", "/api/v1/sources", "/api/v1/downloaders",
	"/api/v1/backups", "/api/v1/health/", "/api/v1/pick-rules"}

// StatusHandlers serves GET /settings/status, keeping the answer for 30
// seconds; Input gathers what the rules look at (the other modules' states).
type StatusHandlers struct {
	Input func(ctx context.Context, now time.Time) (StatusInput, error)
	cache statusCache
}

// Forget drops the kept answer.
func (h *StatusHandlers) Forget() { h.cache.forget() }

// SetClock replaces the cache's clock (tests); nil is time.Now.
func (h *StatusHandlers) SetClock(now func() time.Time) { h.cache.now = now }

// Routes mounts the settings status route.
func (h *StatusHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/settings/status", h.route)
}

// ForgetOnWrite drops the kept status answer after each successful
// write to a statusWrites path, so Settings home shows the change at once.
func (h *StatusHandlers) ForgetOnWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || !slices.ContainsFunc(statusWrites, func(p string) bool { return strings.HasPrefix(r.URL.Path, p) }) {
			next.ServeHTTP(w, r)
			return
		}
		sw := &webserver.StatusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.Code < 400 { // 0: nothing written, a plain 200
			h.cache.forget()
		}
	})
}

func (h *StatusHandlers) route(w http.ResponseWriter, r *http.Request) {
	c := &h.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	if c.answer == nil || now.Sub(c.at) >= statusFor {
		in, err := h.Input(r.Context(), now)
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

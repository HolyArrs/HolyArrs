package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/sites"
)

// SiteLimits holds one site's limits.
type SiteLimits struct {
	GapMS          int `json:"gap_ms"`
	PerWindow      int `json:"per_window"`
	WindowSeconds  int `json:"window_seconds"`
	DownloadsToDay int `json:"downloads_per_day"` // 0 means no cap
}

// SiteRow is one subtitle site as stored.
type SiteRow struct {
	domain.Meta
	Name              string     `db:"name" json:"name"`
	Kind              string     `db:"kind" json:"kind"`
	URL               string     `db:"url" json:"url"`
	SecretRef         *int64     `db:"secret_ref" json:"-"`
	State             string     `db:"state" json:"state"` // enabled, paused or disabled
	PauseReason       *string    `db:"pause_reason" json:"pause_reason"`
	PausedUntil       *time.Time `db:"paused_until" json:"paused_until"`
	Failures          int        `db:"failures" json:"failures"`
	Limits            SiteLimits `db:"limits,json" json:"limits"`
	DownloadsToday    int        `db:"downloads_today" json:"downloads_today"`
	DownloadsResetAt  *time.Time `db:"downloads_reset_at" json:"downloads_reset_at"`
	AllowLocalNetwork bool       `db:"allow_local_network" json:"allow_local_network"`
	Priority          int        `db:"priority" json:"priority"`
	// MachineMade lets in the files the site marks as AI-translated or
	// machine-translated. They score low and are used only when no file made
	// by a person passes, so a person's file always wins and replaces them.
	MachineMade bool `db:"machine_made" json:"machine_made"`
	// HasAccount reports that the site's account details are in the vault. The
	// details themselves are never sent back.
	HasAccount bool `json:"has_account"`
}

// Account is what a site that needs an account receives. It is kept in the
// vault as one sealed value.
type Account struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
}

func (a Account) has(field string) bool {
	switch field {
	case "username":
		return a.Username != ""
	case "password":
		return a.Password != ""
	case "api_key":
		return a.APIKey != ""
	}
	return false
}

func (a Account) empty() bool { return a == Account{} }

// SiteInput is what a person sends to add or change a site.
type SiteInput struct {
	Name              string      `json:"name"`
	Kind              string      `json:"kind"`
	URL               string      `json:"url"`
	Enabled           *bool       `json:"enabled"`
	Limits            *SiteLimits `json:"limits"`
	AllowLocalNetwork bool        `json:"allow_local_network"`
	Priority          int         `json:"priority"`
	// MachineMade turns the machine-made switch on or off; left out, a new
	// site starts off and a changed one keeps its switch.
	MachineMade *bool `json:"machine_made"`
	// Account details for kinds that need an account. Left empty on a change,
	// the stored details stay.
	Username string `json:"username"`
	Password string `json:"password"`
	APIKey   string `json:"api_key"`
}

func (in *SiteInput) account() Account {
	return Account{in.Username, in.Password, in.APIKey} // already trimmed by check
}

// ErrNeedsAccount refuses a site that needs an account until its details are
// given.
var ErrNeedsAccount = errors.New("this subtitle site needs your account: fill in its account details first")

// ErrNoVault means account details cannot be stored because the secrets key
// is not set up.
var ErrNoVault = errors.New("account details cannot be kept: the secrets key is not set up")

// missingAccount lists the account details kind k asks for that a lacks.
func missingAccount(k sites.Kind, a Account) []string {
	var out []string
	for _, f := range k.Account {
		if !a.has(f) {
			out = append(out, f)
		}
	}
	return out
}

func (in *SiteInput) check() (sites.Kind, error) {
	k, ok := sites.KindOf(in.Kind)
	if !ok {
		return k, fmt.Errorf("unknown kind of subtitle site %q", in.Kind)
	}
	if k.Later {
		return k, fmt.Errorf("%s is planned for later and cannot be added yet", k.Name)
	}
	for _, f := range []*string{&in.Username, &in.APIKey} {
		*f = strings.TrimSpace(*f)
	}
	if !k.NeedsAccount && !in.account().empty() {
		return k, errors.New("this kind of subtitle site takes no account details")
	}
	for _, v := range []string{in.Username, in.Password, in.APIKey} {
		if len(v) > 500 {
			return k, errors.New("account details are too long")
		}
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		in.Name = k.Name
	}
	if len(in.Name) > 100 {
		return k, errors.New("name is too long")
	}
	if in.URL == "" {
		in.URL = k.DefaultURL
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(in.URL) > 500 {
		return k, errors.New("url must be an http or https address without a user name")
	}
	if in.Limits == nil {
		in.Limits = &SiteLimits{GapMS: 1000, PerWindow: 60, WindowSeconds: 3600}
		if k.Kind == "addic7ed" { // a page site used gently: one call per 3 s and 100 a day
			in.Limits = &SiteLimits{GapMS: 3000, PerWindow: 100, WindowSeconds: 86400}
		}
	}
	l := in.Limits
	if l.GapMS < 0 || l.GapMS > 60000 || l.PerWindow < 1 || l.PerWindow > 10000 || l.WindowSeconds < 60 || l.WindowSeconds > 86400 || l.DownloadsToDay < 0 || l.DownloadsToDay > 100000 {
		return k, errors.New("limits: gap_ms 0-60000, per_window 1-10000, window_seconds 60-86400, downloads_per_day 0-100000")
	}
	if in.Priority == 0 {
		in.Priority = 25
	}
	if in.Priority < 1 || in.Priority > 100 {
		return k, errors.New("priority must be 1 to 100")
	}
	return k, nil
}

// Sites lists the subtitle sites by priority.
func Sites(ctx context.Context, db *sql.DB) ([]SiteRow, error) {
	rs, err := database.Select[SiteRow](ctx, db, "subtitle_sites", "1=1 ORDER BY priority, id")
	if rs == nil {
		rs = []SiteRow{}
	}
	for i := range rs {
		rs[i].HasAccount = rs[i].SecretRef != nil
	}
	return rs, err
}

// putAccount seals account details into the vault within tx and returns the
// new secret's id. When old is set, that secret is removed.
func putAccount(tx *sql.Tx, v *secrets.Vault, a Account, old *int64, by domain.Actor) (*int64, error) {
	if v == nil {
		return nil, ErrNoVault
	}
	b, _ := json.Marshal(a)
	id, err := v.PutTx(tx, "subtitle_site_account", string(b), by)
	if err != nil {
		return nil, err
	}
	if old != nil {
		if _, err := tx.Exec("DELETE FROM secrets WHERE id = ?", *old); err != nil {
			return nil, err
		}
	}
	return &id, nil
}

// AddSite stores a new site and puts its account details in the vault. A site
// that needs an account is refused until every detail it asks for is given.
// When then is given, it runs in the same transaction with the new id.
func AddSite(ctx context.Context, st *database.Store, v *secrets.Vault, in SiteInput, by domain.Actor, then ...func(tx *sql.Tx, id int64) error) (SiteRow, error) {
	k, err := in.check()
	if err != nil {
		return SiteRow{}, err
	}
	acct := in.account()
	if k.NeedsAccount && len(missingAccount(k, acct)) > 0 {
		return SiteRow{}, ErrNeedsAccount
	}
	r := SiteRow{Name: in.Name, Kind: in.Kind, URL: in.URL, State: "enabled", Limits: *in.Limits, AllowLocalNetwork: in.AllowLocalNetwork, Priority: in.Priority,
		MachineMade: in.MachineMade != nil && *in.MachineMade}
	if (in.Enabled != nil && !*in.Enabled) || (in.Enabled == nil && k.DefaultOff) {
		r.State = "disabled"
	}
	err = st.Write(ctx, func(tx *sql.Tx) error {
		if k.NeedsAccount {
			ref, err := putAccount(tx, v, acct, nil, by)
			if err != nil {
				return err
			}
			r.SecretRef = ref
		}
		id, err := database.Insert(tx, "subtitle_sites", &r, by)
		r.ID = id
		if err != nil {
			return err
		}
		for _, f := range then {
			if err := f(tx, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return r, errors.New("a subtitle site with that name exists")
	}
	r.HasAccount = r.SecretRef != nil
	return r, err
}

// EditSite changes a site, and turning it on clears any pause. Account details
// left empty keep the stored ones, while any detail given replaces them all.
func EditSite(ctx context.Context, st *database.Store, v *secrets.Vault, id int64, in SiteInput, by domain.Actor) (SiteRow, error) {
	var r SiteRow
	if err := database.Get(ctx, st.Read(), "subtitle_sites", id, &r); err != nil {
		return r, ErrNotFound
	}
	if in.Kind == "" {
		in.Kind = r.Kind
	}
	if in.Kind != r.Kind {
		return r, errors.New("the kind of a site cannot change; add a new one")
	}
	k, err := in.check()
	if err != nil {
		return r, err
	}
	acct := in.account()
	if k.NeedsAccount && acct.empty() && r.SecretRef == nil {
		return r, ErrNeedsAccount
	}
	if k.NeedsAccount && !acct.empty() && len(missingAccount(k, acct)) > 0 {
		return r, ErrNeedsAccount
	}
	// A new address keeps the account only when it is sent again, so details never
	// follow a site to another host.
	if k.NeedsAccount && acct.empty() && originOf(in.URL) != originOf(r.URL) {
		return r, errors.New("the address moved to another site: give the account details again")
	}
	r.Name, r.URL, r.Limits, r.AllowLocalNetwork, r.Priority = in.Name, in.URL, *in.Limits, in.AllowLocalNetwork, in.Priority
	if in.MachineMade != nil {
		r.MachineMade = *in.MachineMade
	}
	if in.Enabled != nil {
		if *in.Enabled {
			r.State, r.PauseReason, r.PausedUntil, r.Failures = "enabled", nil, nil, 0
		} else {
			r.State = "disabled"
		}
	}
	err = st.Write(ctx, func(tx *sql.Tx) error {
		if k.NeedsAccount && !acct.empty() {
			ref, err := putAccount(tx, v, acct, r.SecretRef, by)
			if err != nil {
				return err
			}
			r.SecretRef = ref
		}
		return database.Update(tx, "subtitle_sites", &r, by)
	})
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return r, errors.New("a subtitle site with that name exists")
	}
	r.HasAccount = r.SecretRef != nil
	return r, err
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// DeleteSite removes a site and its account details. Saved subtitles keep its
// name.
func DeleteSite(ctx context.Context, st *database.Store, id int64) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		var ref sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT secret_ref FROM subtitle_sites WHERE id = ?", id).Scan(&ref); err != nil {
			return ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM subtitle_sites WHERE id = ?", id); err != nil {
			return err
		}
		if ref.Valid {
			if _, err := tx.ExecContext(ctx, "DELETE FROM secrets WHERE id = ?", ref.Int64); err != nil {
				return err
			}
		}
		return nil
	})
}

// account reads a site's account details from the vault.
func (s *Service) account(ctx context.Context, r SiteRow) (Account, error) {
	var a Account
	if r.SecretRef == nil {
		return a, nil
	}
	if s.Vault == nil {
		return a, ErrNoVault
	}
	raw, err := s.Vault.Get(ctx, *r.SecretRef)
	if err != nil {
		return a, err
	}
	err = json.Unmarshal([]byte(raw), &a)
	return a, err
}

// TestResult is the outcome of testing a site, in plain words.
type TestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Found   int    `json:"found"`
}

// TestSite runs one search on a site for a known open movie, or a known show
// on a shows-only site, and reports how it went. It changes nothing, so a
// failing test never pauses the site.
func (s *Service) TestSite(ctx context.Context, id int64) (TestResult, error) {
	var r SiteRow
	if err := database.Get(ctx, s.Store.Read(), "subtitle_sites", id, &r); err != nil {
		return TestResult{}, ErrNotFound
	}
	k, _ := sites.KindOf(r.Kind)
	if k.NeedsAccount && r.SecretRef == nil {
		return TestResult{Message: "This site needs your account: fill in its account details first."}, nil
	}
	site, err := s.client(ctx, r)
	if err != nil {
		return TestResult{Message: "HolyArrs could not set up this site: " + err.Error()}, nil
	}
	q := sites.Query{Kind: "movie", Title: "Big Buck Bunny", Year: 2008, IMDb: "tt1254207", Languages: []string{"en"}}
	if !k.Movies {
		q = sites.Query{Kind: "episode", Title: "The Office (US)", Year: 2005, Season: 1, Episode: 1, Languages: []string{"en"}}
	}
	cs, err := site.Search(ctx, q)
	if err != nil {
		return TestResult{Message: TestWords(err)}, nil
	}
	return TestResult{OK: true, Found: len(cs), Message: fmt.Sprintf("The site answered: %d subtitles for a test title.", len(cs))}, nil
}

// TestWords explains in plain words why a site call failed.
func TestWords(err error) string {
	var fe *sites.Error
	if !errors.As(err, &fe) {
		return "The test failed: " + err.Error()
	}
	switch fe.Kind {
	case sites.KindLoginRefused:
		return "The site refused the account details. Check them and save again."
	case sites.KindRateLimited:
		return "The site asked HolyArrs to slow down. Try again later."
	case sites.KindDownloadLimit:
		return "The site's download limit for today is reached."
	case sites.KindBlocked:
		return "The request was blocked (" + fe.Msg + ")."
	case sites.KindUnreadable:
		return "The site answered, but HolyArrs could not read the answer (" + fe.Msg + ")."
	default:
		return "The site could not be reached (" + fe.Msg + ")."
	}
}

// usable lists the sites a search may ask now: enabled ones, and paused ones
// whose pause is over.
func usable(ctx context.Context, db *sql.DB, at time.Time) ([]SiteRow, error) {
	return database.Select[SiteRow](ctx, db, "subtitle_sites",
		"state = 'enabled' OR (state = 'paused' AND paused_until <= ?) ORDER BY priority, id", database.FormatTime(at))
}

// client keeps one sites.Site per site row so its limits hold across Jobs.
func (s *Service) client(ctx context.Context, r SiteRow) (sites.Site, error) {
	ref := int64(0)
	if r.SecretRef != nil {
		ref = *r.SecretRef
	}
	key := fmt.Sprintf("%d|%s|%s|%v|%+v|%d", r.ID, r.Kind, r.URL, r.AllowLocalNetwork, r.Limits, ref)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients == nil {
		s.clients = map[int64]cached{}
	}
	if c, ok := s.clients[r.ID]; ok && c.key == key {
		return c.site, nil
	}
	cfg := sites.Config{Name: r.Name, URL: r.URL, GapMS: r.Limits.GapMS, PerWindow: r.Limits.PerWindow,
		Window: time.Duration(r.Limits.WindowSeconds) * time.Second, AllowLocal: r.AllowLocalNetwork, HTTP: s.HTTP}
	if k, _ := sites.KindOf(r.Kind); k.NeedsAccount {
		a, err := s.account(ctx, r)
		if err != nil {
			return nil, err
		}
		if a.empty() {
			return nil, ErrNeedsAccount
		}
		cfg.Key, cfg.Username, cfg.Password = a.APIKey, a.Username, a.Password
	}
	site, err := sites.New(r.Kind, cfg)
	if err != nil {
		return nil, err
	}
	s.clients[r.ID] = cached{key: key, site: site}
	return site, nil
}

type cached struct {
	key  string
	site sites.Site
}

// pause rests a failing site and records an Event.
func (s *Service) pause(ctx context.Context, r SiteRow, cause error) {
	var n int
	var kind string
	var until time.Time
	paused := false
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT failures FROM subtitle_sites WHERE id = ? AND state != 'disabled'", r.ID).Scan(&n); err != nil {
			return err
		}
		n++
		var d time.Duration
		kind, d = sites.PauseFor(cause, n, now())
		until = now().Add(d)
		res, err := tx.ExecContext(ctx, `UPDATE subtitle_sites SET state = 'paused', pause_reason = ?, paused_until = ?, failures = ?, updated_at = ?
			WHERE id = ? AND state != 'disabled'`, kind, database.FormatTime(until), n, database.FormatTime(now()), r.ID)
		if err != nil {
			return err
		}
		k, _ := res.RowsAffected()
		paused = k == 1
		return nil
	})
	if !paused {
		return
	}
	s.event(ctx, &domain.Event{Kind: EventSitePaused, Summary: fmt.Sprintf("Paused Subtitle site %s until %s (%s)", r.Name, until.Format(time.RFC3339), kind),
		Data: map[string]any{"site_id": r.ID, "site": r.Name, "reason": kind, "failures": n, "until": until}})
}

// recovered clears a site's failures after a good answer.
func (s *Service) recovered(ctx context.Context, r SiteRow) {
	if r.State != "paused" && r.Failures == 0 {
		return
	}
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE subtitle_sites SET state = CASE WHEN state = 'paused' THEN 'enabled' ELSE state END,
			pause_reason = NULL, paused_until = NULL, failures = 0, updated_at = ? WHERE id = ?`, database.FormatTime(now()), r.ID)
		return err
	})
}

// countDownload counts one download against the site's daily cap. It returns
// false once the cap is reached, and the site then rests until the day resets.
func (s *Service) countDownload(ctx context.Context, r SiteRow) bool {
	if r.Limits.DownloadsToDay <= 0 {
		return true
	}
	ok := false
	day := now().Truncate(24 * time.Hour).Add(24 * time.Hour)
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		var used int
		var reset sql.NullString
		if err := tx.QueryRowContext(ctx, "SELECT downloads_today, downloads_reset_at FROM subtitle_sites WHERE id = ?", r.ID).Scan(&used, &reset); err != nil {
			return err
		}
		if !reset.Valid || reset.String <= database.FormatTime(now()) {
			used = 0
		}
		if used >= r.Limits.DownloadsToDay {
			return nil
		}
		ok = true
		_, err := tx.ExecContext(ctx, "UPDATE subtitle_sites SET downloads_today = ?, downloads_reset_at = ? WHERE id = ?", used+1, database.FormatTime(day), r.ID)
		return err
	})
	if !ok {
		s.pause(ctx, r, &sites.Error{Kind: sites.KindDownloadLimit, Msg: "the day's downloads are used up"})
	}
	return ok
}

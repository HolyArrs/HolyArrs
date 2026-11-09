package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// DefaultReleaseFeed is the HolyArrs project's newest-release address.
// Nothing is published there yet: a 404 or no release is quiet.
const DefaultReleaseFeed = "https://api.github.com/repos/" + projectRepo + "/releases/latest"

// projectRepo is the project's GitHub repository. A release's "what is new"
// link is kept only when it is a page under ReleasePages.
const projectRepo = "HolyArrs/HolyArrs"

// ReleasePages is the only prefix a "what is new" link may have.
const ReleasePages = "https://github.com/" + projectRepo + "/releases/"

// maxRedirects caps the hops the update check follows.
const maxRedirects = 3

// Update check settings in the this_app group.
const (
	settingUpdateOn    = "update_check"            // "off" turns the check off (no request at all); default on
	settingUpdateLocal = "update_check_local"      // "true": the feed may be on the local network
	settingUpdateState = "update_state"            // the last answer, so a restart does not ask again
	settingUpdatePre   = "update_prerelease"       // "true": a pre-release (v1.2.0-rc1) may give a notice; default off
	settingUpdateDays  = "update_check_every_days" // days between checks, one of UpdateDays; default 1
	settingUpdateURL   = "update_check_url"        // where the newest version is read; empty uses ReleaseFeed
)

// Checker asks the signed update list or the release feed whether a newer
// HolyArrs is out, at most once per update_check_every_days, and keeps the
// last answer so a restart does not ask again.
type Checker struct {
	Store *database.Store
	// Version is this program's version.
	Version string
	// Client reads the release feed (tests); nil uses ClientFor.
	Client *http.Client
	// ClientFor makes the release feed's client: public addresses only, or
	// the local network too when allowed (nil: a plain client, tests only).
	ClientFor func(allowLocal bool) *http.Client
	// ReleaseFeed is the release feed used when the setting is empty
	// (main sets DefaultReleaseFeed; empty: no check without a setting).
	ReleaseFeed string
	// ListKeys are the trusted update list keys (tests); nil means Pinned().
	// With no key the GitHub releases check is used.
	ListKeys []PublicKey
	// ListURL and ListCopyURL are the update list and its Pages copy
	// (tests); empty means DefaultURL and PagesURL.
	ListURL, ListCopyURL string
	// Now is the clock (nil means time.Now).
	Now func() time.Time

	updMu sync.Mutex
	upd   *UpdateInfo
}

// UpdateDays are the allowed values of update_check_every_days.
var UpdateDays = []int{1, 3, 7, 14, 30}

// updateEvery is the time between update checks: the setting, else UpdateEvery.
func (e *Checker) updateEvery(ctx context.Context) time.Duration {
	n := e.intSetting(ctx, settingUpdateDays, 1, 1, 30)
	if !slices.Contains(UpdateDays, n) {
		n = 1
	}
	return time.Duration(n) * UpdateEvery
}

// Update check limits.
const (
	UpdateEvery   = 24 * time.Hour // one day: the default, and the unit of update_check_every_days
	updateTimeout = 15 * time.Second
	updateMaxBody = 512 << 10
)

// UpdateInfo is what This app shows about newer releases.
type UpdateInfo struct {
	// On is false when the check is turned off: no request is made.
	On bool `json:"on"`
	// Feed is the address asked (the default when the setting is empty).
	Feed      string     `json:"feed"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	// Latest is the newest release's version; empty when none is published.
	Latest string `json:"latest,omitempty"`
	// Newer says Latest is newer than this version.
	Newer bool `json:"newer"`
	// Link is the release's page, only when it is under ReleasePages.
	Link string `json:"link,omitempty"`
	// NoRelease says the feed answered that nothing is published yet.
	NoRelease bool `json:"no_release,omitempty"`
	// Notes is the update list's short notes, plain text.
	Notes string `json:"notes,omitempty"`
	// Problem is why the last check did not answer, in plain words.
	Problem string `json:"problem,omitempty"`
	Version string `json:"version"`
}

func (e *Checker) updateOn(ctx context.Context) bool {
	return e.setting(ctx, settingUpdateOn) != "off"
}

// feed is the release feed to ask: the setting, else the engine's default.
func (e *Checker) feed(ctx context.Context) string {
	if v := strings.TrimSpace(e.setting(ctx, settingUpdateURL)); v != "" {
		return v
	}
	return e.ReleaseFeed
}

// Update answers the update check's state without asking the feed.
func (e *Checker) Update(ctx context.Context) UpdateInfo {
	e.updMu.Lock()
	defer e.updMu.Unlock()
	return e.updateLocked(ctx)
}

func (e *Checker) updateLocked(ctx context.Context) UpdateInfo {
	if e.upd == nil {
		e.upd = &UpdateInfo{}
		if raw := e.setting(ctx, settingUpdateState); raw != "" {
			_ = json.Unmarshal([]byte(raw), e.upd)
		}
	}
	out := *e.upd
	out.On, out.Feed, out.Version = e.updateOn(ctx), e.feed(ctx), e.Version
	out.Newer = out.Latest != "" && Newer(out.Latest, e.Version) &&
		(!IsPreRelease(out.Latest) || e.setting(ctx, settingUpdatePre) == "true")
	return out
}

// CheckUpdate asks the feed now (when the check is on and has a feed),
// whatever the last check's time.
func (e *Checker) CheckUpdate(ctx context.Context) UpdateInfo {
	e.updMu.Lock()
	defer e.updMu.Unlock()
	return e.checkLocked(ctx, true)
}

func (e *Checker) checkLocked(ctx context.Context, force bool) UpdateInfo {
	cur := e.updateLocked(ctx)
	listMode := e.listMode()
	if !cur.On || (cur.Feed == "" && !listMode) {
		return cur
	}
	if !force && cur.CheckedAt != nil && e.now().Sub(*cur.CheckedAt) < e.updateEvery(ctx) {
		return cur
	}
	now := e.now()
	next := UpdateInfo{CheckedAt: &now}
	var latest, link string
	var none bool
	var err error
	if listMode {
		latest, link, next.Notes, none, err = e.readList(ctx)
	} else {
		latest, link, none, err = e.readLatest(ctx, cur.Feed)
	}
	switch {
	case err != nil:
		// Keep what the last good check found.
		next.Latest, next.Link, next.Notes, next.NoRelease, next.Problem = e.upd.Latest, e.upd.Link, e.upd.Notes, e.upd.NoRelease, err.Error()
	case none:
		next.NoRelease = true
	default:
		next.Latest, next.Link = latest, link
	}
	e.upd = &next
	if b, err := json.Marshal(next); err == nil {
		_ = e.saveSetting(ctx, settingUpdateState, string(b))
	}
	return e.updateLocked(ctx)
}

// ResetUpdate forgets the last answer (the feed or its switches changed),
// so the next check asks the feed again.
func (e *Checker) ResetUpdate(ctx context.Context) {
	e.updMu.Lock()
	defer e.updMu.Unlock()
	e.upd = &UpdateInfo{}
	_ = e.saveSetting(ctx, settingUpdateState, "")
}

// Scheduled is the regular check's part: it asks the feed when the last
// answer is older than update_check_every_days. ran is false when the check
// is off or has nowhere to ask, and then nothing is asked.
func (e *Checker) Scheduled(ctx context.Context) (info UpdateInfo, ran bool) {
	e.updMu.Lock()
	defer e.updMu.Unlock()
	if !e.updateOn(ctx) || (e.feed(ctx) == "" && !e.listMode()) {
		return UpdateInfo{}, false
	}
	return e.checkLocked(ctx, false), true
}

// Every is the time between update checks.
func (e *Checker) Every(ctx context.Context) time.Duration { return e.updateEvery(ctx) }

func (e *Checker) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e *Checker) client(ctx context.Context) *http.Client {
	if e.Client != nil {
		return e.Client
	}
	if e.ClientFor != nil {
		return e.ClientFor(e.setting(ctx, settingUpdateLocal) == "true")
	}
	return &http.Client{Timeout: updateTimeout}
}

// readLatest asks the feed for the newest release: GitHub's releases
// answer (tag_name, html_url), any JSON with a version, or plain text. A
// 404 or an empty list means nothing is published yet.
func (e *Checker) readLatest(ctx context.Context, link string) (latest, page string, none bool, err error) {
	caller := ctx
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "update_feed", Identity: []string{link}})
	if err != nil {
		return "", "", false, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	ctx, cancel := op.AttemptContext(ctx, updateTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		outcome = outbound.TerminalFailure
		return "", "", false, errors.New("the release feed is not a link")
	}
	req.Header.Set("Accept", "application/vnd.github+json, application/json;q=0.9, text/plain;q=0.8")
	req.Header.Set("User-Agent", "HolyArrs/"+e.Version)
	c := *e.client(ctx)
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		if r.URL.Scheme != via[0].URL.Scheme {
			return errors.New("redirect changes the scheme")
		}
		return nil
	}
	resp, err := c.Do(req)
	if err != nil {
		outcome = outbound.HTTPOutcome(0, err, caller)
		return "", "", false, errors.New("the release feed did not answer")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, updateMaxBody+1))
	if closeErr := resp.Body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		outcome = outbound.HTTPOutcome(resp.StatusCode, err, caller)
		return "", "", false, errors.New("the release feed's answer could not be read")
	}
	outcome = outbound.HTTPOutcome(resp.StatusCode, nil, caller)
	if resp.StatusCode == http.StatusNotFound {
		return "", "", true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", false, fmt.Errorf("the release feed answered %d", resp.StatusCode)
	}
	if len(b) > updateMaxBody {
		outcome = outbound.TerminalFailure
		return "", "", false, errors.New("the release feed's answer is too large")
	}
	text := strings.TrimSpace(string(b))
	if text == "" || text == "[]" {
		return "", "", true, nil
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		type release struct {
			Tag        string `json:"tag_name"`
			Version    string `json:"version"`
			HTML       string `json:"html_url"`
			Draft      bool   `json:"draft"`
			Prerelease bool   `json:"prerelease"`
		}
		var one release
		if strings.HasPrefix(text, "[") {
			var list []release
			if err := json.Unmarshal(b, &list); err != nil {
				outcome = outbound.TerminalFailure
				return "", "", false, errors.New("the release feed's answer is not readable")
			}
			for _, r := range list {
				if !r.Draft && !r.Prerelease {
					one = r
					break
				}
			}
			if one.Tag == "" && one.Version == "" {
				return "", "", true, nil
			}
		} else if err := json.Unmarshal(b, &one); err != nil {
			outcome = outbound.TerminalFailure
			return "", "", false, errors.New("the release feed's answer is not readable")
		}
		latest = one.Tag
		if latest == "" {
			latest = one.Version
		}
		page = releasePage(one.HTML)
	} else {
		latest = text
	}
	if latest == "" || len(latest) > 40 || strings.ContainsAny(latest, " \n\t<>\"") {
		outcome = outbound.TerminalFailure
		return "", "", false, errors.New("the release feed's answer has no version")
	}
	outcome = outbound.Success
	return latest, page, false, nil
}

// releasePage keeps a "what is new" link only when it is a plain page under
// ReleasePages: https, github.com without a port or user, the project's
// releases path with no dot segments, query or fragment.
func releasePage(link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		u.Opaque != "" || strings.Contains(u.Path, "..") || strings.Contains(u.Path, "//") {
		return ""
	}
	if s := u.String(); strings.HasPrefix(s, ReleasePages) && s == link {
		return s
	}
	return ""
}

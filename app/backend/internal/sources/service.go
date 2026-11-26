// Package sources talks to the Sources a person connects, the Torznab, Newznab
// and rss feeds that list releases. It reads their caps, searches them, turns
// their XML into plain items, runs the search jobs that judge and pick what
// they return, keeps each Source's health, and connects the Jackett or
// Prowlarr bridges that bring Sources in.
package sources

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Errors for a Source that cannot be saved. Each message is shown to the person as it is.
var (
	ErrName     = errors.New("give the Source a name of 1 to 80 characters")
	ErrNameUsed = errors.New("another Source has that name")
	ErrFormat   = errors.New("format must be torznab, newznab, rss or site")
	ErrSite     = errors.New("site must be one of the built-in sites: knaben, torrentscsv, subsplease or iptorrents")
	ErrProtocol = errors.New("protocol must be torrent or usenet")
	ErrURL      = errors.New("the address must start with http:// or https:// and name a host")
	// ErrQuery matches ErrURL under errors.Is, so callers that check ErrURL treat it the same.
	ErrQuery      error = queryErr{}
	ErrPriority         = errors.New("priority must be 1 to 50 (1 is asked first)")
	ErrLanguages        = errors.New("languages need codes release names show, such as uk, it or en")
	ErrLimits           = errors.New("limits must be 0 or more: searches_per_window up to 10000, window_seconds up to 7 days, gap_ms up to 10 minutes, timeout_ms up to 5 minutes")
	ErrKinds            = errors.New("kinds may be movie, tv and music")
	ErrUse              = errors.New("an rss Source can only be used for the feed; it cannot search")
	ErrProxy            = errors.New("the proxy must look like http://host:port, https://host:port or socks5://host:port")
	ErrKeyAgain         = errors.New("the address changed to another host; type the key again so it is never sent to a new host by itself")
	ErrProxyAgain       = errors.New("the proxy changed to another host or name; type its password again")
	// ErrKeyAgainProxy is ErrKeyAgain for a plain http Source whose proxy changed, because the new proxy would see the key.
	ErrKeyAgainProxy = errors.New("the proxy changed and the Source address is plain http, so the new proxy would see the key; type the key again")
	ErrNoVault       = errors.New("keys cannot be stored: the secrets vault is not open")
	ErrSeeders       = errors.New("min_seeders must be 0 or more")
	ErrSiteUser      = errors.New("site_user is your user id on the site: digits only")
	ErrSiteLogin     = errors.New("site_login is passkey or cookie, and site_agent is one line of up to 500 characters")
)

// AllKinds lists the media types a Source may serve.
var AllKinds = []string{"movie", "tv", "music"}

// DefaultCategories maps each kind to its Torznab and Newznab category number.
var DefaultCategories = map[string][]int{"movie": {2000}, "tv": {5000}, "music": {3000}}

// Input is what a person sets on a Source.
// APIKey and ProxyPassword change only when sent, and empty keeps the stored value.
// Nil fields keep the stored value, or the default for a new Source.
type Input struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	// Site names the built-in site a site-format Source reads (knaben,
	// torrentscsv, subsplease, iptorrents). It is set when the Source is added.
	Site string `json:"site,omitempty"`
	// SiteUser is the person's user id on a built-in site that asks for one
	// beside its key (IPTorrents: the passkey goes in APIKey). Nil keeps it.
	SiteUser *string `json:"site_user,omitempty"`
	// SiteLogin is how a Source signs in to IPTorrents: "passkey" (user id +
	// passkey, the default) or "cookie" (a browser cookie, kept as the key).
	SiteLogin *string `json:"site_login,omitempty"`
	// SiteAgent is the browser user agent sent with that cookie; optional.
	SiteAgent       *string              `json:"site_agent,omitempty"`
	Protocol        string               `json:"protocol"`
	URL             string               `json:"url"`
	APIKey          string               `json:"api_key"`
	ClearKey        bool                 `json:"clear_key"`
	UseFor          *domain.SourceUse    `json:"use_for"`
	Kinds           []string             `json:"kinds"`
	Categories      map[string][]int     `json:"categories"`
	MinSeeders      *int                 `json:"min_seeders"`
	Limits          *domain.SourceLimits `json:"limits"`
	Priority        *int                 `json:"priority"`
	AllowLocal      *bool                `json:"allow_local_network"`
	Proxy           *string              `json:"proxy"`
	ProxyPassword   string               `json:"proxy_password"`
	ProxyAllowLocal *bool                `json:"proxy_allow_local_network"`
	// Tags limit the Source to titles that share one of them. Nil keeps the stored tags.
	Tags []string `json:"tags"`
	// MultiLanguages are what MULTi means on this Source. Nil keeps the stored list.
	MultiLanguages []string `json:"multi_languages"`
	// NoLanguageLanguages are the audio languages a release here carries when
	// its name shows none. Nil keeps the stored list.
	NoLanguageLanguages []string `json:"no_language_languages"`
	// DownloaderID picks the one Downloader this Source's grabs go to; 0 is
	// any Downloader and nil keeps the stored choice.
	DownloaderID *int64 `json:"downloader_id"`
}

// queryErr is the error behind ErrQuery.
type queryErr struct{}

// Add stores a new Source and sends its key and proxy password to the vault.
// The then callbacks run in the same transaction with the new id.
func Add(ctx context.Context, s *database.Store, v *secrets.Vault, in Input, by domain.Actor, then ...func(tx *sql.Tx, id int64) error) (domain.Source, error) {
	var src domain.Source
	key, pass, err := fill(in, &src, true)
	if err != nil {
		return src, err
	}
	var path []string
	if src.Format == "rss" {
		if src.URL, path, _, err = hidePath(src.URL, nil); err != nil {
			return src, err
		}
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if err := nameUsed(tx, src.Name, 0); err != nil {
			return err
		}
		if err := putSecret(tx, v, &src.SecretRef, "source_api_key", key, by); err != nil {
			return err
		}
		if err := putSecret(tx, v, &src.PathSecretRef, "source_path_key", strings.Join(path, "/"), by); err != nil {
			return err
		}
		if err := putSecret(tx, v, &src.ProxySecretRef, "source_proxy_password", pass, by); err != nil {
			return err
		}
		id, err := database.Insert(tx, "sources", &src, by)
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
	return src, err
}

// Change saves a Source. A stored key is never sent to a new host on its own, so a
// changed host needs the key typed again, and the same holds for the proxy password.
// The then callbacks may edit the Source in the same transaction, just before it is written.
func Change(ctx context.Context, s *database.Store, v *secrets.Vault, id int64, in Input, by domain.Actor, then ...func(tx *sql.Tx, src *domain.Source) error) (domain.Source, error) {
	var src domain.Source
	if err := database.Get(ctx, s.Read(), "sources", id, &src); err != nil {
		return src, err
	}
	old := src
	key, pass, err := fill(in, &src, false)
	if err != nil {
		return old, err
	}
	// A sent address is checked for path passkeys again. A masked one keeps the stored value, and only on the same host.
	urlSent := strings.TrimSpace(in.URL) != ""
	var path, oldPath []string
	pathKept := src.PathSecretRef != nil && !urlSent
	if urlSent && src.Format != "rss" && hasMasked(src.URL) {
		return old, ErrPathKeyRSS
	}
	if urlSent && src.Format == "rss" {
		if old.PathSecretRef != nil && hasMasked(src.URL) {
			if hostOf(src.URL) != hostOf(old.URL) {
				return old, ErrPathKey
			}
			if v == nil {
				return old, ErrNoVault
			}
			plain, err := v.Get(ctx, *old.PathSecretRef)
			if err != nil {
				return old, fmt.Errorf("reading the Source passkey: %w", err)
			}
			oldPath = splitKeys(plain)
		}
		var reused bool
		if src.URL, path, reused, err = hidePath(src.URL, oldPath); err != nil {
			return old, err
		}
		pathKept = reused
	}
	if src.SecretRef != nil && key == "" && !in.ClearKey && hostOf(src.URL) != hostOf(old.URL) {
		return old, ErrKeyAgain
	}
	// The stored proxy address has no password, so a changed scheme, name or host needs it typed again.
	if src.ProxySecretRef != nil && pass == "" && src.ProxyURL != "" && src.ProxyURL != old.ProxyURL {
		return old, ErrProxyAgain
	}
	// A plain http Source sends its key in the clear to whatever proxy carries it.
	if (src.SecretRef != nil && key == "" && !in.ClearKey || pathKept) && strings.HasPrefix(src.URL, "http://") &&
		src.ProxyURL != "" && src.ProxyURL != old.ProxyURL {
		return old, ErrKeyAgainProxy
	}
	err = s.Write(ctx, func(tx *sql.Tx) error {
		if err := nameUsed(tx, src.Name, src.ID); err != nil {
			return err
		}
		if in.ClearKey && key == "" {
			if err := dropSecret(tx, &src.SecretRef); err != nil {
				return err
			}
		}
		if err := putSecret(tx, v, &src.SecretRef, "source_api_key", key, by); err != nil {
			return err
		}
		if urlSent && !(pathKept && slices.Equal(path, oldPath)) {
			if err := dropSecret(tx, &src.PathSecretRef); err != nil {
				return err
			}
			if err := putSecret(tx, v, &src.PathSecretRef, "source_path_key", strings.Join(path, "/"), by); err != nil {
				return err
			}
		}
		if src.ProxyURL == "" {
			if err := dropSecret(tx, &src.ProxySecretRef); err != nil {
				return err
			}
		} else if err := putSecret(tx, v, &src.ProxySecretRef, "source_proxy_password", pass, by); err != nil {
			return err
		}
		for _, f := range then {
			if err := f(tx, &src); err != nil {
				return err
			}
		}
		return database.Update(tx, "sources", &src, by)
	})
	if err != nil {
		return old, err
	}
	return src, nil
}

// removeBatch is how many Releases or calls of a removed Source one write
// deletes, so a Source that gathered a great many never holds the database
// past the write cap.
const removeBatch = 2000

// Remove deletes a Source together with its secrets, Releases and call record.
// The Releases and calls go first, a batch per write; the Source row last.
func Remove(ctx context.Context, s *database.Store, id int64) error {
	var src domain.Source
	if err := database.Get(ctx, s.Read(), "sources", id, &src); err != nil {
		return err
	}
	for _, table := range []string{"source_calls", "releases"} {
		for {
			var n int64
			err := s.Write(ctx, func(tx *sql.Tx) (err error) {
				n, err = deleteSourceRowsTx(ctx, tx, table, id, removeBatch)
				return err
			})
			if err != nil {
				return err
			}
			if n < removeBatch {
				break
			}
		}
	}
	return s.Write(ctx, func(tx *sql.Tx) error {
		var src domain.Source
		if err := database.Get(ctx, tx, "sources", id, &src); err != nil {
			return err
		}
		if err := dropSecret(tx, &src.SecretRef); err != nil {
			return err
		}
		if err := dropSecret(tx, &src.ProxySecretRef); err != nil {
			return err
		}
		if err := dropSecret(tx, &src.PathSecretRef); err != nil {
			return err
		}
		return deleteSource(ctx, tx, id)
	})
}

// SetPaused pauses a Source by hand or resumes it.
// Resuming also ends a back-off pause, so the Source is tried again at once.
func SetPaused(ctx context.Context, s *database.Store, id int64, paused bool, by domain.Actor) (domain.Source, error) {
	var src domain.Source
	err := s.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "sources", id, &src); err != nil {
			return err
		}
		if paused {
			src.State = "disabled"
		} else {
			src.State = "enabled"
			if src.Extra != nil {
				delete(src.Extra, "failures")
			}
		}
		src.PauseReason, src.PausedUntil = nil, nil
		return database.Update(tx, "sources", &src, by)
	})
	return src, err
}

// ConfigFor builds the Client settings for a Source and opens its key, passkey and proxy password.
func ConfigFor(ctx context.Context, src domain.Source, sec SecretReader) (Config, error) {
	cfg := Config{Site: src.Extra[extraSiteKind], SiteUser: src.Extra[extraSiteUser], SiteLogin: src.Extra[extraSiteLogin], SiteAgent: src.Extra[extraSiteAgent], URL: src.URL, AllowLocal: src.AllowLocal, Protocol: src.Protocol, Format: src.Format,
		GapMS: src.Limits.GapMS, PerWindow: src.Limits.SearchesPerWindow, ProxyAllowLocal: src.ProxyAllowLocal,
		Window: time.Duration(src.Limits.WindowSeconds) * time.Second, Timeout: time.Duration(src.Limits.TimeoutMS) * time.Millisecond}
	open := func(ref *int64) (string, error) {
		if ref == nil || sec == nil {
			return "", nil
		}
		return sec.Get(ctx, *ref)
	}
	var err error
	if cfg.APIKey, err = open(src.SecretRef); err != nil {
		return cfg, fmt.Errorf("reading the Source key: %w", err)
	}
	if strings.Contains(src.URL, pathMark) {
		plain, err := open(src.PathSecretRef)
		if err != nil {
			return cfg, fmt.Errorf("reading the Source passkey: %w", err)
		}
		cfg.PathKeys = splitKeys(plain)
		if cfg.URL, err = revealPath(src.URL, cfg.PathKeys); err != nil {
			return cfg, err
		}
	}
	if src.ProxyURL != "" {
		p, err := url.Parse(src.ProxyURL)
		if err != nil {
			return cfg, ErrProxy
		}
		pass, err := open(src.ProxySecretRef)
		if err != nil {
			return cfg, fmt.Errorf("reading the proxy password: %w", err)
		}
		if pass != "" {
			p.User = url.UserPassword(p.User.Username(), pass)
		}
		cfg.Proxy = p
	}
	return cfg, nil
}

// CauseOf names why a call to a Source failed, in the doctor's causes
// shared with Downloaders and media servers. A bot check or a slow-down ask
// has no cause there: it is the site, not the connection.
func CauseOf(err error) doctor.Cause {
	var se *Error
	if !errors.As(err, &se) {
		return doctor.Of(err)
	}
	switch se.Kind {
	case KindRateLimited:
		return doctor.Unknown
	case KindLoginRefused:
		return doctor.Auth
	case KindBlocked:
		if se.Detail != "" || strings.Contains(se.Msg, "looked up") {
			return doctor.Unknown
		}
		return doctor.Blocked
	case KindBroken:
		if se.cause != nil && doctor.Of(se.cause) == doctor.WrongApp {
			return doctor.WrongApp
		}
		if c := doctor.OfText(se.Msg); c == doctor.NotFound {
			return c
		}
		if strings.Contains(se.Msg, "not valid XML") {
			return doctor.WrongApp
		}
		return doctor.Unknown
	}
	if se.cause != nil {
		if c := doctor.Of(se.cause); c != doctor.Unknown {
			return c
		}
	}
	return doctor.OfText(se.Msg)
}

// CauseAt is CauseOf for a call to rawURL: a connection closed without an
// answer reads as an http/https or port mix-up. dial is ProbeDial.
func CauseAt(err error, rawURL string, dial doctor.Dial) doctor.Cause {
	if err == nil {
		return doctor.Unknown
	}
	return doctor.At(CauseOf(err), err.Error(), rawURL, dial)
}

// ProbeDial is the way a second look at x may connect: through its network
// rule, and not at all when a proxy carries its calls.
func ProbeDial(x domain.Source) doctor.Dial {
	if x.ProxyURL != "" {
		return nil
	}
	allow := outbound.PublicOnly
	if x.AllowLocal {
		allow = outbound.NotMetadata
	}
	return outbound.PolicyDialer(allow).DialContext
}

// probeDialOf is ProbeDial; tests swap it to count the looks.
var probeDialOf = ProbeDial

// metadataLine answers a refused cloud metadata address, which no switch allows.
const metadataLine = "That address is a cloud metadata address, which HolyArrs never connects to."

// Explain turns a failed call into plain words for the person.
// It never includes the address or the key.
func Explain(err error) string {
	if se := (*Error)(nil); errors.As(err, &se) && strings.Contains(se.Msg, "cloud metadata address") {
		return metadataLine
	}
	if line := doctor.Line(CauseOf(err)); line != "" {
		return line
	}
	var se *Error
	if !errors.As(err, &se) {
		return "HolyArrs could not reach the Source."
	}
	if t := blockText[se.Detail]; t != "" {
		return t
	}
	switch se.Kind {
	case KindRateLimited:
		return "The Source asked HolyArrs to slow down. Try again later."
	case KindBlocked:
		return "HolyArrs could not look up the Source's name, so it cannot check where it leads; the proxy was not asked."
	case KindBroken:
		return "The Source answered, but not with a feed HolyArrs can read (" + se.Msg + ")."
	}
	return "HolyArrs could not reach the Source (" + se.Msg + ")."
}

// ExplainVia is Explain for one Source: when it searches through Prowlarr or
// Jackett and that app does not answer, it names that app and what to do.
func ExplainVia(sourceURL string, err error, dial doctor.Dial) string {
	return explainWith(sourceURL, err, CauseAt(err, sourceURL, dial), dial)
}

// explainWith is ExplainVia with the cause already worked out.
func explainWith(sourceURL string, err error, c doctor.Cause, dial doctor.Dial) string {
	text := Explain(err)
	if text == metadataLine {
		return text
	}
	if line := doctor.Line(c); line != "" {
		text = line
	}
	if se := (*Error)(nil); errors.As(err, &se) && se.mainApp != "" {
		return mainAddressWords[se.mainApp]
	}
	if c == doctor.WrongApp {
		if app := mainAddressOf(sourceURL); app != "" {
			return mainAddressWords[app]
		}
	}
	switch {
	case c == doctor.Refused, c == doctor.Timeout, c == doctor.DNS:
	case c == doctor.Unknown && strings.HasPrefix(text, "HolyArrs could not reach the Source"):
	default:
		return text
	}
	via := bridgeOf(sourceURL)
	if via == "" {
		if line := doctor.NameLine(c, sourceURL, dial); line != "" {
			return line
		}
		return text
	}
	return via + " isn't answering. Sources brought in from " + via + " search through it, so keep " + via + " running, or change the address."
}

// mainAddressWords say what to do when a Source address is Prowlarr's or Jackett's main page.
var mainAddressWords = map[string]string{
	"Prowlarr": "That is Prowlarr's main address, not one site's feed. Connect Prowlarr under Prowlarr or Jackett on this page to bring in all its sites, or paste one site's Torznab feed address from Prowlarr.",
	"Jackett":  "That is Jackett's main address, not one site's feed. Connect Jackett under Prowlarr or Jackett on this page to bring in all its sites, or paste one site's Torznab feed address from Jackett.",
}

// mainAddressOf names Prowlarr or Jackett when a Source address is that app's
// own main page (no site path) on its usual port; such a page is not a feed.
func mainAddressOf(sourceURL string) string {
	u, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || u.Host == "" {
		return ""
	}
	if p := strings.Trim(u.Path, "/"); p != "" && p != "api" {
		return ""
	}
	switch u.Port() {
	case findPorts[BridgeKindProwlarr]:
		return "Prowlarr"
	case findPorts[BridgeKindJackett]:
		return "Jackett"
	}
	return ""
}

// bridgeOf names the app a Source's address goes through ("" for a site of its own).
func bridgeOf(sourceURL string) string {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	p := strings.TrimSuffix(u.Path, "/")
	switch {
	case strings.Contains(p, "/api/v2.0/indexers/"):
		return "Jackett"
	case prowlarrSitePath.MatchString(strings.TrimSuffix(p, "/api")):
		return "Prowlarr"
	}
	return ""
}

// splitRSS takes the query out of an rss address, because feeds keep their passkey there.
// The query is stored as the Source's key.
func splitRSS(raw string) (base, query string) {
	base, query, _ = strings.Cut(raw, "?")
	return base, query
}

func checkURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err == nil && u.Scheme == "plugin" {
		// A plugin search source: plugin://<id>, nothing else.
		if !pluginHostRe.MatchString(u.Host) || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, ErrURL
		}
		return u, nil
	}
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return nil, ErrURL
	}
	return u, nil
}

// parseProxy checks a proxy address and returns it without the password, which comes back apart.
func parseProxy(raw string) (clean string, pass string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") || u.Port() == "" || u.Hostname() == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return "", "", ErrProxy
	}
	if n, err := strconv.Atoi(u.Port()); err != nil || n < 1 || n > 65535 {
		return "", "", ErrProxy
	}
	out := url.URL{Scheme: u.Scheme, Host: u.Host}
	if u.User != nil {
		pass, _ = u.User.Password()
		out.User = url.User(u.User.Username())
	}
	return out.String(), pass, nil
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

// fill validates in and copies it onto src.
// It returns the key and proxy password to store, and empty means keep the stored one.
func fill(in Input, src *domain.Source, isNew bool) (key, proxyPass string, err error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 80 {
		return "", "", ErrName
	}
	src.Name = name
	if isNew || in.Format != "" {
		if !slices.Contains([]string{"torznab", "newznab", "rss", "site"}, in.Format) {
			return "", "", ErrFormat
		}
		src.Format = in.Format
	}
	var site Site
	if src.Format == "site" {
		id := in.Site
		if id == "" {
			id = src.Extra[extraSiteKind]
		}
		var ok bool
		if site, ok = SiteByID(id); !ok {
			return "", "", ErrSite
		}
		if in.Protocol != "" && in.Protocol != "torrent" {
			return "", "", ErrProtocol
		}
		if isNew && strings.TrimSpace(in.URL) == "" {
			in.URL = site.URL
		}
		if isNew && in.Limits == nil {
			in.Limits = &domain.SourceLimits{GapMS: site.GapMS, SearchesPerWindow: site.PerHour, WindowSeconds: 3600, TimeoutMS: site.TimeoutMS}
		}
		if isNew && in.Kinds == nil {
			in.Kinds = site.Kinds
		}
	}
	proto := in.Protocol
	if proto == "" && isNew {
		proto = "torrent"
		if src.Format == "newznab" {
			proto = "usenet"
		}
	}
	if proto != "" {
		if proto != "torrent" && proto != "usenet" {
			return "", "", ErrProtocol
		}
		src.Protocol = proto
	}
	key = in.APIKey
	if raw := strings.TrimSpace(in.URL); raw != "" || isNew {
		// A typed address must never hold a real path marker, or a stored passkey would be filled into it.
		raw = strings.ReplaceAll(raw, pathMark, "HOLYARRS-PATHKEY")
		if src.Format == "rss" {
			var q string
			raw, q = splitRSS(raw)
			if q != "" {
				key = q
			}
		}
		u, err := checkURL(raw)
		if err != nil {
			return "", "", err
		}
		// Torznab and Newznab requests add their own query, so a key typed here would be stored in the clear.
		if src.Format != "rss" && (u.RawQuery != "" || u.ForceQuery) {
			return "", "", ErrQuery
		}
		src.URL = raw
	}
	if in.UseFor != nil {
		src.UseFor = *in.UseFor
	} else if isNew {
		src.UseFor = domain.SourceUse{Feed: true, AutoSearch: src.Format != "rss", HandSearch: src.Format != "rss"}
		if site.ID == "torrentscsv" {
			// TorrentsCSV only searches; it has no newest list to watch.
			src.UseFor.Feed = false
		}
	}
	if src.Format == "rss" && (src.UseFor.AutoSearch || src.UseFor.HandSearch) {
		return "", "", ErrUse
	}
	if in.Kinds != nil {
		for _, k := range in.Kinds {
			if !slices.Contains(AllKinds, k) {
				return "", "", ErrKinds
			}
		}
		src.Kinds = slices.Clone(in.Kinds)
	} else if isNew {
		src.Kinds = slices.Clone(AllKinds)
	}
	if in.Categories != nil {
		src.Categories = in.Categories
	} else if isNew {
		src.Categories = map[string][]int{}
		for k, v := range DefaultCategories {
			src.Categories[k] = slices.Clone(v)
		}
	}
	if in.MinSeeders != nil {
		if *in.MinSeeders < 0 {
			return "", "", ErrSeeders
		}
		src.MinSeeders = in.MinSeeders
	} else if isNew && src.Protocol == "torrent" {
		one := 1
		src.MinSeeders = &one
	}
	if in.Limits != nil {
		l := *in.Limits
		if l.SearchesPerWindow < 0 || l.SearchesPerWindow > 10000 || l.GrabsPerWindow < 0 || l.GrabsPerWindow > 10000 ||
			l.WindowSeconds < 0 || l.WindowSeconds > 7*86400 || l.GapMS < 0 || l.GapMS > 600000 || l.TimeoutMS < 0 || l.TimeoutMS > 300000 {
			return "", "", ErrLimits
		}
		src.Limits = l
	}
	if in.Tags != nil || isNew || src.Tags == nil {
		src.Tags = mediafiles.CleanTags(in.Tags)
	}
	if in.MultiLanguages != nil || isNew || src.MultiLanguages == nil {
		src.MultiLanguages = CleanLanguages(in.MultiLanguages)
	}
	if in.NoLanguageLanguages != nil || isNew || src.NoLanguageLanguages == nil {
		langs, err := checkLanguages(in.NoLanguageLanguages)
		if err != nil {
			return "", "", err
		}
		src.NoLanguageLanguages = langs
	}
	if in.DownloaderID != nil {
		src.DownloaderID = nil
		if *in.DownloaderID > 0 {
			id := *in.DownloaderID
			src.DownloaderID = &id
		}
	}
	if in.Priority != nil {
		if *in.Priority < 1 || *in.Priority > 50 {
			return "", "", ErrPriority
		}
		src.Priority = *in.Priority
	} else if isNew {
		src.Priority = 25
	}
	if in.AllowLocal != nil {
		src.AllowLocal = *in.AllowLocal
	}
	if in.ProxyAllowLocal != nil {
		src.ProxyAllowLocal = *in.ProxyAllowLocal
	}
	proxyPass = in.ProxyPassword
	if in.Proxy != nil {
		clean, pass, err := parseProxy(*in.Proxy)
		if err != nil {
			return "", "", err
		}
		if pass != "" {
			proxyPass = pass
		}
		src.ProxyURL = clean
	}
	if isNew {
		src.State = "enabled"
		src.Extra = map[string]string{}
	}
	if site.ID != "" {
		if src.Extra == nil {
			src.Extra = map[string]string{}
		}
		src.Extra[extraSiteKind] = site.ID
		if in.SiteUser != nil {
			user := strings.TrimSpace(*in.SiteUser)
			if len(user) > 20 || strings.Trim(user, "0123456789") != "" {
				return "", "", ErrSiteUser
			}
			if user == "" {
				delete(src.Extra, extraSiteUser)
			} else {
				src.Extra[extraSiteUser] = user
			}
		}
		if in.SiteLogin != nil {
			switch strings.TrimSpace(*in.SiteLogin) {
			case "", "passkey":
				delete(src.Extra, extraSiteLogin)
			case "cookie":
				if site.ID != "iptorrents" {
					return "", "", ErrSiteLogin
				}
				src.Extra[extraSiteLogin] = "cookie"
			default:
				return "", "", ErrSiteLogin
			}
		}
		if in.SiteAgent != nil {
			agent := strings.TrimSpace(*in.SiteAgent)
			if len(agent) > 500 || strings.ContainsAny(agent, "\r\n") {
				return "", "", ErrSiteLogin
			}
			if agent == "" {
				delete(src.Extra, extraSiteAgent)
			} else {
				src.Extra[extraSiteAgent] = agent
			}
		}
	}
	return key, proxyPass, nil
}

func nameUsed(tx *sql.Tx, name string, id int64) error {
	n, err := sourcesNamed(tx, name, id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrNameUsed
	}
	return nil
}

func (queryErr) Error() string {
	return "leave the query (?apikey=...) out of the address; put the key in the key field"
}

func (queryErr) Is(t error) bool { return t == ErrURL }

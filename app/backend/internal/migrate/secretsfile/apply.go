package secretsfile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// Where the Settings screens keep the outside-service keys (the API's
// handlers use the same group and keys; a test checks they agree).
const (
	LinksGroup     = "links"
	TMDBTokenKey   = "tmdb_token"
	TraktClientKey = "trakt_client_id"
	TVDBKeyKey     = "tvdb_key"
)

// EventImported is the Event a finished import records (counts only).
const EventImported = "secrets_imported"

// Results.
const (
	Set       = "set"
	WouldSet  = "would set"
	Unchanged = "unchanged"
	Empty     = "skipped (empty)"
)

// Line is one item's result. Neither field ever holds a value from the
// file.
type Line struct {
	Item   string `json:"item"`
	Result string `json:"result"`
}

// Failed says whether the line is a failure.
func (l Line) Failed() bool { return strings.HasPrefix(l.Result, "failed") }

// AllOK says whether no line failed.
func AllOK(ls []Line) bool {
	for _, l := range ls {
		if l.Failed() {
			return false
		}
	}
	return true
}

// Deps is what an import writes with.
type Deps struct {
	Store *database.Store
	Vault *secrets.Vault
	By    domain.Actor
}

type run struct {
	ctx    context.Context
	d      Deps
	f      *File
	dry    bool
	lines  []Line
	hidden []string
}

// Apply stores every filled-in item of f and says, item by item, what it
// did. dry only says what it would do. Each item is on its own: one that
// fails leaves the others as they are.
func Apply(ctx context.Context, d Deps, f *File, dry bool) []Line {
	r := &run{ctx: ctx, d: d, f: f, dry: dry, hidden: f.Values()}
	if d.Vault == nil {
		r.add("everything", "failed: the secrets vault is not open (no master key)")
		return r.lines
	}
	r.settings()
	r.subtitleSites()
	r.bridges()
	r.indexers()
	r.downloaders()
	r.mediaServers()
	r.moveCredentials()
	r.alertItems()
	if f.HolyArrs.Address != "" || f.HolyArrs.OwnerAPIKey != "" {
		r.add("holyarrs", "skipped (only used to reach a running HolyArrs)")
	} else {
		r.add("holyarrs", Empty)
	}
	if !dry {
		n := map[string]int{}
		for _, l := range r.lines {
			switch {
			case l.Failed():
				n["failed"]++
			case strings.HasPrefix(l.Result, Set):
				n["set"]++
			case l.Result == Unchanged:
				n["unchanged"]++
			}
		}
		_ = d.Store.AddEvent(ctx, &domain.Event{Actor: d.By, Kind: EventImported,
			Summary: fmt.Sprintf("Imported settings from a secrets file: %d set, %d unchanged, %d failed", n["set"], n["unchanged"], n["failed"]),
			Data:    map[string]any{"set": n["set"], "unchanged": n["unchanged"], "failed": n["failed"]}})
	}
	return r.lines
}

func (r *run) add(item, result string) { r.lines = append(r.lines, Line{Item: item, Result: result}) }

// fail records a failure in plain words with every file value hidden.
func (r *run) fail(item string, err error) {
	r.add(item, "failed: "+r.say(err.Error()))
}

func (r *run) say(msg string) string {
	for _, v := range r.hidden {
		if len(v) >= 3 {
			msg = strings.ReplaceAll(msg, v, "[hidden]")
			msg = strings.ReplaceAll(msg, url.QueryEscape(v), "[hidden]")
		}
	}
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	return msg
}

// done records a set (or would-set) with an optional note.
func (r *run) done(item, note string) {
	res := Set
	if r.dry {
		res = WouldSet
	}
	if note != "" {
		res += " (" + note + ")"
	}
	r.add(item, res)
}

func empty(vs ...string) bool {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

func trim(s string) string { return strings.TrimSpace(s) }

func cleanURL(s string) string { return strings.TrimRight(trim(s), "/") }

func hostOf(raw string) string {
	u, err := url.Parse(trim(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// secretIs says whether the vault secret at ref holds plain.
func (r *run) secretIs(ref *int64, plain string) bool {
	if ref == nil {
		return plain == ""
	}
	v, err := r.d.Vault.Get(r.ctx, *ref)
	return err == nil && v == plain
}

// --- links settings -----------------------------------------------------

func (r *run) settings() {
	r.setting("tmdb.read_token", TMDBTokenKey, r.f.TMDB.ReadToken)
	if empty(r.f.TMDB.APIKey) {
		r.add("tmdb.api_key", Empty)
	} else {
		r.add("tmdb.api_key", "skipped (not used: HolyArrs uses the TMDB read token)")
	}
	r.setting("tvdb", TVDBKeyKey, r.f.TVDB.APIKey)
	r.setting("trakt", TraktClientKey, r.f.Trakt.ClientID)
	if !empty(r.f.TVDB.UniqueUserKey) {
		r.add("tvdb.unique_user_key", "skipped (not used: HolyArrs uses the TVDB api_key)")
	}
	if !empty(r.f.Trakt.RedirectURLs) {
		r.add("trakt.redirect_urls", "skipped (not used: HolyArrs uses the Trakt client_id)")
	}
}

func (r *run) setting(item, key, val string) {
	val = trim(val)
	if val == "" {
		r.add(item, Empty)
		return
	}
	cur, err := r.d.Vault.Setting(r.ctx, LinksGroup, key)
	if err == nil && cur == val {
		r.add(item, Unchanged)
		return
	}
	if !r.dry {
		if err := r.d.Vault.SetSetting(r.ctx, LinksGroup, key, val, r.d.By); err != nil {
			r.fail(item, err)
			return
		}
	}
	r.done(item, "")
}

// --- subtitle sites -----------------------------------------------------

func (r *run) subtitleSites() {
	os := r.f.OpenSubtitles
	ok, sk := trim(os.APIKey), trim(r.f.Subdl.APIKey)
	r.site("opensubtitles", "opensubtitles", subtitles.Account{Password: os.Password, APIKey: ok, Username: trim(os.Username)})
	r.site("subdl", "subdl", subtitles.Account{APIKey: sk})
	addic := r.f.Addic7ed
	r.site("addic7ed", "addic7ed", subtitles.Account{
		Username: trim(addic.Username),
		Password: addic.Password,
	})
}

func (r *run) site(item, kind string, a subtitles.Account) {
	if a == (subtitles.Account{}) {
		r.add(item, Empty)
		return
	}
	rows, err := subtitles.Sites(r.ctx, r.d.Store.Read())
	if err != nil {
		r.fail(item, err)
		return
	}
	var row *subtitles.SiteRow
	for i := range rows {
		if rows[i].Kind == kind {
			row = &rows[i]
			break
		}
	}
	in := subtitles.SiteInput{APIKey: a.APIKey, Password: a.Password, Kind: kind, Username: a.Username}
	if row == nil {
		if r.dry {
			r.done(item, "")
			return
		}
		if _, err := subtitles.AddSite(r.ctx, r.d.Store, r.d.Vault, in, r.d.By); err != nil {
			r.fail(item, err)
			return
		}
		r.done(item, "")
		return
	}
	if row.SecretRef != nil {
		if raw, err := r.d.Vault.Get(r.ctx, *row.SecretRef); err == nil {
			var cur subtitles.Account
			if json.Unmarshal([]byte(raw), &cur) == nil && cur == a {
				r.add(item, Unchanged)
				return
			}
		}
	}
	if r.dry {
		r.done(item, "")
		return
	}
	// The same change the Settings screen sends: everything as it is,
	// with the new account details.
	lim := row.Limits
	in.Name, in.URL, in.Limits, in.AllowLocalNetwork, in.Priority = row.Name, row.URL, &lim, row.AllowLocalNetwork, row.Priority
	if _, err := subtitles.EditSite(r.ctx, r.d.Store, r.d.Vault, row.ID, in, r.d.By); err != nil {
		r.fail(item, err)
		return
	}
	r.done(item, "")
}

// --- Sources ------------------------------------------------------------

// bridges says what the Jackett and Prowlarr keys are used for: an
// indexers entry on the same host without a key of its own gets it.
func (r *run) bridges() {
	for _, b := range []struct {
		item string
		app  KeyApp
	}{{"jackett", r.f.Jackett}, {"prowlarr", r.f.Prowlarr}} {
		if empty(b.app.Address, b.app.APIKey) {
			r.add(b.item, Empty)
			continue
		}
		if empty(b.app.Address) || empty(b.app.APIKey) {
			r.add(b.item, "failed: needs both address and api_key")
			continue
		}
		n := 0
		for _, ix := range r.f.Indexers {
			if empty(ix.APIKey) && hostOf(ix.Address) == hostOf(b.app.Address) {
				n++
			}
		}
		note := fmt.Sprintf("its key fills in %d indexers entries on its address", n)
		if b.item == "prowlarr" {
			note += "; saved for the Move tool too (see the prowlarr line under old apps)"
		}
		if n == 0 && b.item == "jackett" {
			r.add(b.item, "skipped (no indexers entry on its address uses its key; add one per Jackett feed)")
			continue
		}
		r.add(b.item, "noted ("+note+")")
	}
}

func (r *run) bridgeKey(addr string) string {
	for _, b := range []KeyApp{r.f.Jackett, r.f.Prowlarr} {
		if !empty(b.Address, b.APIKey) && hostOf(b.Address) == hostOf(addr) {
			return trim(b.APIKey)
		}
	}
	return ""
}

func (r *run) indexers() {
	if len(r.f.Indexers) == 0 {
		r.add("indexers", Empty)
		return
	}
	for i, ix := range r.f.Indexers {
		item := fmt.Sprintf("indexers[%d]", i+1)
		if empty(ix.Name, ix.Address, ix.APIKey) {
			r.add(item, Empty)
			continue
		}
		if empty(ix.Name) || empty(ix.Address) {
			r.add(item, "failed: needs a name and an address")
			continue
		}
		key := trim(ix.APIKey)
		if key == "" {
			key = r.bridgeKey(ix.Address)
		}
		format := strings.ToLower(trim(ix.Format))
		if format == "" {
			format = "torznab"
		}
		if format != "torznab" && format != "newznab" {
			r.add(item, "failed: format must be torznab or newznab")
			continue
		}
		r.indexer(item, trim(ix.Name), cleanURL(ix.Address), key, format, ix.AllowLocalNetwork, ix.ClearKey)
	}
}

func (r *run) indexer(item, name, addr, key, format string, local, clearKey bool) {
	var src domain.Source
	var err error
	src.ID, err = sourceIDByName(r.ctx, r.d.Store.Read(), name)
	if errors.Is(err, sql.ErrNoRows) {
		if r.dry {
			r.done(item, "paused until you check and resume it")
			return
		}
		in := sources.Input{Name: name, Format: format, URL: addr, APIKey: key, AllowLocal: &local}
		if _, err := sources.Add(r.ctx, r.d.Store, r.d.Vault, in, r.d.By, createdPaused("sources")); err != nil {
			r.fail(item, err)
			return
		}
		r.done(item, "paused until you check and resume it in Settings > Sources")
		return
	}
	if err == nil {
		err = database.Get(r.ctx, r.d.Store.Read(), "sources", src.ID, &src)
	}
	if err != nil {
		r.fail(item, err)
		return
	}
	repair := src.State != "disabled" && r.importCreated("sources", src.ID)
	// An empty key keeps the stored one; only clear_key removes it.
	keySame := (key == "" && !clearKey) || (key == "" && src.SecretRef == nil) || (key != "" && r.secretIs(src.SecretRef, key))
	if cleanURL(src.URL) == addr && src.Format == format && src.AllowLocal == local && keySame {
		if !repair {
			r.add(item, Unchanged)
			return
		}
		if !r.dry {
			if _, err := sources.SetPaused(r.ctx, r.d.Store, src.ID, true, r.d.By); err != nil {
				r.fail(item, err)
				return
			}
		}
		r.done(item, repairNote)
		return
	}
	if r.dry {
		r.done(item, "")
		return
	}
	// A new address or key is paused first (a failed save then leaves it
	// paused, never live with half the change).
	note := ""
	if cleanURL(src.URL) != addr || !keySame {
		if _, err := sources.SetPaused(r.ctx, r.d.Store, src.ID, true, r.d.By); err != nil {
			r.fail(item, err)
			return
		}
		note = changedNote
	}
	in := sources.Input{Name: src.Name, Format: format, URL: addr, APIKey: key, ClearKey: key == "" && clearKey, AllowLocal: &local}
	if _, err := sources.Change(r.ctx, r.d.Store, r.d.Vault, src.ID, in, r.d.By); err != nil {
		r.fail(item, err)
		return
	}
	r.done(item, note)
}

// repairNote is said when a re-run pauses again an item the import made
// that was found active.
const repairNote = "paused again: the import made it and it was not paused"

// changedNote is said when an item's address or key changed: it is paused
// until the owner checks it again.
const changedNote = "changed, paused until you check and resume it"

// testFailCreate, set only by tests, fails the create step inside its
// transaction.
var testFailCreate func(table string) error

// createdPaused runs inside the transaction that makes a Source or
// Downloader: it starts paused and is marked as the import's, so no
// moment exists where it is active.
func createdPaused(table string) func(tx *sql.Tx, id int64) error {
	return func(tx *sql.Tx, id int64) error {
		if err := markPaused(tx, table, id); err != nil {
			return err
		}
		if testFailCreate != nil {
			return testFailCreate(table)
		}
		return nil
	}
}

// importCreated says whether the import made this item and the owner has
// not resumed it since.
func (r *run) importCreated(table string, id int64) bool {
	return importMade(r.ctx, r.d.Store.Read(), table, id) > 0
}

// --- Downloaders --------------------------------------------------------

func (r *run) downloaders() {
	q := r.f.QBittorrent
	if !empty(q.APIKey) {
		r.downloaderItem("qbittorrent", "qbittorrent", "qBittorrent", q.Address, "", q.APIKey, q.AllowLocalNetwork, false)
	} else {
		r.downloaderItem("qbittorrent", "qbittorrent", "qBittorrent", q.Address, q.Username, q.Password, q.AllowLocalNetwork, true)
	}
	s := r.f.SABnzbd
	r.downloaderItem("sabnzbd", "sabnzbd", "SABnzbd", s.Address, "", s.APIKey, s.AllowLocalNetwork, false)
	if !empty(s.NZBKey) {
		r.add("sabnzbd.nzb_key", "skipped (not used: SABnzbd needs its full api_key)")
	}
	if !empty(s.Username, s.Password) {
		r.add("sabnzbd.login", "skipped (not used: SABnzbd uses its api_key)")
	}
	for _, x := range []struct {
		item string
		vals []string
	}{
		{"nzbget", []string{r.f.NZBGet.Address, r.f.NZBGet.Username, r.f.NZBGet.Password}},
		{"transmission", []string{r.f.Transmission.Address, r.f.Transmission.Username, r.f.Transmission.Password}},
		{"deluge", []string{r.f.Deluge.Address, r.f.Deluge.Password}},
	} {
		if empty(x.vals...) {
			r.add(x.item, Empty)
		} else {
			r.add(x.item, "skipped (HolyArrs cannot use this Downloader yet; planned for later)")
		}
	}
}

func (r *run) downloaderItem(item, adapter, name, addr, user, secret string, local, hasUser bool) {
	addr, user = cleanURL(addr), trim(user)
	if empty(addr, user, secret) {
		r.add(item, Empty)
		return
	}
	if addr == "" || secret == "" {
		need := "an address and an api_key"
		if hasUser {
			need = "an address and a password"
		}
		r.add(item, "failed: needs "+need)
		return
	}
	var d domain.Downloader
	var err error
	d.ID, err = downloaderID(r.ctx, r.d.Store.Read(), adapter, addr, name)
	if errors.Is(err, sql.ErrNoRows) {
		if r.dry {
			r.done(item, "paused until you check and resume it")
			return
		}
		in := downloads.Input{Name: name, Adapter: adapter, URL: addr, Secret: secret, AllowLocal: &local}
		if hasUser {
			in.Username = &user
		}
		if _, err := downloads.Add(r.ctx, r.d.Store, r.d.Vault, in, r.d.By, createdPaused("downloaders")); err != nil {
			r.fail(item, err)
			return
		}
		r.done(item, "paused until you check and resume it in Settings > Downloaders")
		return
	}
	if err == nil {
		err = database.Get(r.ctx, r.d.Store.Read(), "downloaders", d.ID, &d)
	}
	if err != nil {
		r.fail(item, err)
		return
	}
	repair := d.State != "disabled" && r.importCreated("downloaders", d.ID)
	if cleanURL(d.URL) == addr && d.Username == user && d.AllowLocal == local && r.secretIs(d.SecretRef, secret) {
		if !repair {
			r.add(item, Unchanged)
			return
		}
		if !r.dry {
			if _, err := downloads.SetPaused(r.ctx, r.d.Store, d.ID, true, r.d.By); err != nil {
				r.fail(item, err)
				return
			}
		}
		r.done(item, repairNote)
		return
	}
	if r.dry {
		r.done(item, "")
		return
	}
	note := ""
	if cleanURL(d.URL) != addr || !r.secretIs(d.SecretRef, secret) {
		if _, err := downloads.SetPaused(r.ctx, r.d.Store, d.ID, true, r.d.By); err != nil {
			r.fail(item, err)
			return
		}
		note = changedNote
	}
	in := downloads.Input{Name: d.Name, URL: addr, Secret: secret, AllowLocal: &local}
	if hasUser {
		in.Username = &user
	}
	if _, err := downloads.Change(r.ctx, r.d.Store, r.d.Vault, d.ID, in, r.d.By); err != nil {
		r.fail(item, err)
		return
	}
	r.done(item, note)
}

// --- media servers ------------------------------------------------------

func (r *run) mediaServers() {
	r.mediaServer("jellyfin", "jellyfin", "Jellyfin", r.f.Jellyfin.Address, r.f.Jellyfin.APIKey, r.f.Jellyfin.AllowLocalNetwork)
	r.mediaServer("emby", "emby", "Emby", r.f.Emby.Address, r.f.Emby.APIKey, r.f.Emby.AllowLocalNetwork)
	r.mediaServer("plex", "plex", "Plex", r.f.Plex.Address, r.f.Plex.Token, r.f.Plex.AllowLocalNetwork)
}

func (r *run) mediaServer(item, kind, name, addr, tok string, local bool) {
	addr, tok = cleanURL(addr), trim(tok)
	if empty(addr, tok) {
		r.add(item, Empty)
		return
	}
	if addr == "" || tok == "" {
		r.add(item, "failed: needs an address and a key or token")
		return
	}
	var m domain.MediaServer
	var err error
	m.ID, err = mediaServerID(r.ctx, r.d.Store.Read(), kind, addr, name)
	if errors.Is(err, sql.ErrNoRows) {
		if r.dry {
			r.done(item, "off until you test it and turn it on")
			return
		}
		off := false
		if _, err := mediaservers.Add(r.ctx, r.d.Store, r.d.Vault, mediaservers.Input{Name: name, Kind: kind, URL: addr, Token: tok, Enabled: &off, AllowLocal: &local}, r.d.By); err != nil {
			r.fail(item, err)
			return
		}
		r.done(item, "off until you test it and turn it on in Settings > Media servers")
		return
	}
	if err == nil {
		err = database.Get(r.ctx, r.d.Store.Read(), "media_servers", m.ID, &m)
	}
	if err != nil {
		r.fail(item, err)
		return
	}
	if cleanURL(m.URL) == addr && m.AllowLocal == local && r.secretIs(m.TokenSecretID, tok) {
		r.add(item, Unchanged)
		return
	}
	if r.dry {
		r.done(item, "")
		return
	}
	if _, err := mediaservers.Change(r.ctx, r.d.Store, r.d.Vault, m.ID, mediaservers.Input{Name: m.Name, URL: addr, Token: tok, AllowLocal: &local,
		PathFrom: m.PathFrom, PathTo: m.PathTo}, r.d.By); err != nil {
		r.fail(item, err)
		return
	}
	r.done(item, "")
}

// --- old apps -----------------------------------------------------------

func (r *run) moveCredentials() {
	for _, x := range []struct {
		kind string
		app  KeyApp
	}{{"radarr", r.f.Radarr}, {"sonarr", r.f.Sonarr}, {"lidarr", r.f.Lidarr}, {"prowlarr", r.f.Prowlarr}, {"bazarr", r.f.Bazarr}, {"seerr", r.f.Seerr}} {
		item := x.kind + " (old app)"
		if empty(x.app.Address, x.app.APIKey) {
			r.add(item, Empty)
			continue
		}
		k := trim(x.app.APIKey)
		c := MoveCredential{APIKey: k, Address: cleanURL(x.app.Address), AllowLocalNetwork: x.app.AllowLocalNetwork}
		if c.Address == "" || c.APIKey == "" {
			r.add(item, "failed: needs both address and api_key")
			continue
		}
		address, err := moveAddress(c.Address)
		if err != nil {
			r.fail(item, err)
			continue
		}
		c.Address = address
		if cur, _, ok, err := ReadMoveCredentialAt(r.ctx, r.d.Store, r.d.Vault, x.kind, c.Address); err == nil && ok && cur == c {
			r.add(item, Unchanged)
			continue
		}
		if !r.dry {
			if err := SaveMoveCredential(r.ctx, r.d.Store, r.d.Vault, x.kind, c, r.d.By); err != nil {
				r.fail(item, err)
				continue
			}
		}
		r.done(item, "saved for the Move tool; forgotten when a move finishes")
	}
}

// --- Alerts -------------------------------------------------------------

func (r *run) alertItems() {
	r.smtp()
	if dsc := trim(r.f.Discord.WebhookURL); dsc == "" {
		r.add("discord", Empty)
	} else {
		in := alerts.Input{Name: "Discord", Kind: "discord", URL: dsc}
		r.alert("discord", in, func(a domain.Alert) bool { return r.secretIs(a.URLSecretID, dsc) })
	}
	w := r.f.Webhook
	if empty(w.URL, w.SigningSecret) {
		r.add("webhook", Empty)
		return
	}
	if empty(w.URL) {
		r.add("webhook", "skipped (no url yet; a signing secret alone makes no Alert)")
		return
	}
	in := alerts.Input{Name: "Webhook", Kind: "webhook", URL: trim(w.URL), Secret: w.SigningSecret, WebhookAuth: "sign", AllowLocal: &w.AllowLocalNetwork}
	if w.SigningSecret == "" {
		in.WebhookAuth = "none"
	}
	r.alert("webhook", in, func(a domain.Alert) bool {
		return a.AllowLocal == w.AllowLocalNetwork && a.WebhookAuth == in.WebhookAuth && r.secretIs(a.URLSecretID, in.URL) &&
			(w.SigningSecret == "" || r.secretIs(a.PasswordSecretID, w.SigningSecret))
	})
}

func (r *run) smtp() {
	s := r.f.SMTP
	if empty(s.Host, s.Port, s.Username, s.Password, s.From) {
		r.add("smtp", Empty)
		return
	}
	port := 587
	if p := trim(s.Port); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			r.add("smtp", "failed: port must be a number")
			return
		}
		port = n
	}
	if trim(s.Host) == "" || strings.ContainsAny(trim(s.Host), " /:") {
		r.add("smtp", "failed: host must be a server name without a scheme or port")
		return
	}
	if port < 1 || port > 65535 {
		r.add("smtp", "failed: port must be between 1 and 65535")
		return
	}
	if !alerts.CheckAddress(trim(s.From)) {
		r.add("smtp", "failed: from must be one plain email address")
		return
	}
	sec := "starttls"
	switch {
	case port == 465:
		sec = "tls"
	case port != 587 && s.Password == "":
		sec = "none" // a local mail catcher with no sign-in
	}
	in := alerts.Input{Name: "Email", Kind: "email", SMTPHost: trim(s.Host), SMTPPort: port, SMTPSecurity: sec, SMTPFrom: trim(s.From),
		SMTPUser: trim(s.Username), Password: s.Password, AllowLocal: &s.AllowLocalNetwork}
	r.alert("smtp", in, func(a domain.Alert) bool {
		return a.SMTPHost == in.SMTPHost && a.SMTPPort == port && a.SMTPSecurity == sec && a.SMTPFrom == in.SMTPFrom && a.SMTPUser == in.SMTPUser &&
			a.AllowLocal == s.AllowLocalNetwork && r.secretIs(a.PasswordSecretID, s.Password)
	})
}

// alert adds an Alert (off until the owner tests it and turns it on) or
// changes the one of that name and kind.
func (r *run) alert(item string, in alerts.Input, same func(domain.Alert) bool) {
	var a domain.Alert
	var err error
	a.ID, err = alertID(r.ctx, r.d.Store.Read(), in.Name, in.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		if r.dry {
			r.done(item, "off until you test it and turn it on")
			return
		}
		off := false
		in.Enabled = &off
		if _, err := alerts.Add(r.ctx, r.d.Store, r.d.Vault, in, r.d.By); err != nil {
			r.fail(item, err)
			return
		}
		r.done(item, "off until you test it and turn it on in Settings > Alerts")
		return
	}
	if err == nil {
		err = database.Get(r.ctx, r.d.Store.Read(), "alerts", a.ID, &a)
	}
	if err != nil {
		r.fail(item, err)
		return
	}
	if same(a) {
		r.add(item, Unchanged)
		return
	}
	if r.dry {
		r.done(item, "")
		return
	}
	on := a.Enabled
	in.Enabled, in.Moments = &on, a.Moments
	if _, err := alerts.Change(r.ctx, r.d.Store, r.d.Vault, a.ID, in, r.d.By); err != nil {
		r.fail(item, err)
		return
	}
	r.done(item, "")
}

package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	htmlesc "html"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Built-in sites: a Source with format "site" reads one of these sites
// directly, with no Prowlarr or Jackett in between. Extra["site"] names it.
const extraSiteKind = "site"

// extraSiteUser is the Extra key of a person's user id on a built-in site
// that asks for one beside the key (IPTorrents). It is not a secret.
const extraSiteUser = "site_user"

// extraSiteLogin is the Extra key of how a Source signs in to IPTorrents:
// "cookie" for a browser cookie, empty for user id + passkey (the default,
// and what every Source saved before the choice existed uses).
const extraSiteLogin = "site_login"

// extraSiteAgent is the Extra key of the browser user agent sent with an
// IPTorrents cookie. A cookie made behind the site's bot check only works
// with the browser that made it. It is not a secret.
const extraSiteAgent = "site_agent"

// iptCookieGone is what the person sees when the site no longer takes the cookie.
const iptCookieGone = "IPTorrents no longer takes the cookie; copy a fresh one or switch to user ID + passkey"

// Site describes one built-in site for the add form.
type Site struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Private bool   `json:"private"`
	// Kinds are the media types the site serves.
	Kinds []string `json:"kinds"`
	// Limits are polite defaults: one call at a time with a gap, a cap per hour.
	GapMS     int `json:"gap_ms"`
	PerHour   int `json:"per_hour"`
	TimeoutMS int `json:"timeout_ms"`
}

// Sites lists the built-in sites, in the order the add form shows them.
var Sites = []Site{
	{ID: "knaben", Name: "Knaben", URL: "https://api.knaben.org", Kinds: []string{"movie", "tv", "music"}, GapMS: 2000, PerHour: 100, TimeoutMS: 20000},
	{ID: "torrentscsv", Name: "TorrentsCSV", URL: "https://torrents-csv.com", Kinds: []string{"movie", "tv", "music"}, GapMS: 2000, PerHour: 100, TimeoutMS: 20000},
	{ID: "subsplease", Name: "SubsPlease", URL: "https://subsplease.org", Kinds: []string{"tv"}, GapMS: 2000, PerHour: 100, TimeoutMS: 20000},
	{ID: "iptorrents", Name: "IPTorrents", URL: "https://iptorrents.com", Private: true, Kinds: []string{"movie", "tv", "music"}, GapMS: 3000, PerHour: 60, TimeoutMS: 30000},
}

// SiteByID finds a built-in site.
func SiteByID(id string) (Site, bool) {
	for _, s := range Sites {
		if s.ID == id {
			return s, true
		}
	}
	return Site{}, false
}

func (c *Client) siteCaps() (Caps, error) {
	s, ok := SiteByID(c.cfg.Site)
	if !ok {
		return Caps{}, &Error{Kind: KindBroken, Msg: "unknown built-in site"}
	}
	caps := Caps{Search: true, TVSearch: slices.Contains(s.Kinds, "tv"), MovieSearch: slices.Contains(s.Kinds, "movie"),
		MusicSearch: slices.Contains(s.Kinds, "music"), Params: map[string][]string{"search": {"q"}}}
	for _, k := range s.Kinds {
		caps.Categories = append(caps.Categories, DefaultCategories[k]...)
	}
	return caps, nil
}

// siteText is the words a built-in site is searched with: the title and, for
// a show, the season and episode (S01E02, S01, or a daily date); for a movie
// the year; for an album the artist and album.
func siteText(q Query) string {
	if q.feed {
		return ""
	}
	if q.Music {
		return strings.TrimSpace(q.Artist + " " + q.Album)
	}
	t := textQuery(Query{Title: q.Title})
	if q.TV {
		if q.Words != "" {
			return strings.TrimSpace(t + " " + q.Words)
		}
		switch {
		case strings.Contains(q.Episode, "/"):
			return strings.TrimSpace(fmt.Sprintf("%s %d %s", t, q.Season, strings.ReplaceAll(q.Episode, "/", " ")))
		case q.Episode != "" && q.Season > 0:
			if n, err := strconv.Atoi(q.Episode); err == nil {
				return fmt.Sprintf("%s S%02dE%02d", t, q.Season, n)
			}
		case q.Season > 0:
			return fmt.Sprintf("%s S%02d", t, q.Season)
		}
		return t
	}
	if q.Year > 0 {
		return fmt.Sprintf("%s %d", t, q.Year)
	}
	return t
}

func (c *Client) siteSearch(ctx context.Context, q Query, observer func(string, time.Duration, error)) ([]Item, error) {
	op := "search"
	if q.feed {
		op = "feed"
	}
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	base := strings.TrimRight(c.cfg.URL, "/")
	text := siteText(q)
	start := time.Now()
	items, err := c.siteCall(ctx, base, text, q)
	if observer != nil && ctx.Err() == nil {
		observer(op, time.Since(start), err)
	}
	return items, err
}

func (c *Client) siteCall(ctx context.Context, base, text string, q Query) ([]Item, error) {
	switch c.cfg.Site {
	case "knaben":
		body := map[string]any{"search_type": "100%", "search_field": "title", "query": text, "size": 100,
			"hide_unsafe": true, "hide_xxx": true, "order_by": "seeders", "order_direction": "desc"}
		if text == "" {
			body["order_by"] = "date"
			delete(body, "search_field")
			delete(body, "query")
		}
		if cats := knabenCategories(q); len(cats) > 0 {
			body["categories"] = cats
		}
		payload, _ := json.Marshal(body)
		raw, err := c.send(ctx, http.MethodPost, base+"/v1", payload, http.Header{"Content-Type": {"application/json"}})
		if err != nil {
			return nil, err
		}
		return parseKnaben(raw)
	case "torrentscsv":
		if text == "" {
			// The site has no newest list; it only searches.
			return nil, nil
		}
		raw, err := c.send(ctx, http.MethodGet, base+"/service/search?"+url.Values{"q": {text}, "size": {"100"}}.Encode(), nil, nil)
		if err != nil {
			return nil, err
		}
		return parseTorrentsCSV(raw)
	case "subsplease":
		v := url.Values{"tz": {"UTC"}}
		if text == "" {
			v.Set("f", "latest")
		} else {
			// The site names episodes "Show - 05" and knows no S01E05, so search the title alone.
			v.Set("f", "search")
			v.Set("s", textQuery(Query{Title: q.Title}))
		}
		raw, err := c.send(ctx, http.MethodGet, base+"/api/?"+v.Encode(), nil, nil)
		if err != nil {
			return nil, err
		}
		return parseSubsPlease(raw)
	case "iptorrents":
		if c.cfg.SiteLogin == "cookie" {
			cookie := iptCookie(c.cfg.APIKey)
			if cookie == "" {
				return nil, &Error{Kind: KindLoginRefused, Msg: "IPTorrents needs your browser cookie"}
			}
			raw, err := c.send(ctx, http.MethodGet, base+"/t?"+iptSearchValues(text).Encode(), nil, iptCookieHeader(cookie, c.cfg.SiteAgent))
			if err != nil {
				var se *Error
				if errors.As(err, &se) && se.Kind == KindLoginRefused {
					return nil, &Error{Kind: KindLoginRefused, Msg: iptCookieGone}
				}
				return nil, err
			}
			if iptSignIn(string(raw)) {
				return nil, &Error{Kind: KindLoginRefused, Msg: iptCookieGone}
			}
			return parseIPTorrents(raw, base, "")
		}
		k := iptKeys(c.cfg.APIKey, c.cfg.SiteUser)
		if k.uid != "" && k.passkey != "" {
			// The site's own RSS: the newest list of the wanted categories, or a search of them.
			addr := base + "/t.rss?u=" + url.QueryEscape(k.uid) + ";tp=" + url.QueryEscape(k.passkey)
			for _, n := range iptCategories(q) {
				addr += ";" + strconv.Itoa(n)
			}
			addr += ";download"
			if text != "" {
				addr += ";q=" + url.QueryEscape(text)
			}
			raw, err := c.send(ctx, http.MethodGet, addr, nil, nil)
			if err != nil {
				// The site answers a passkey it does not know with "404 Not Found".
				var se *Error
				if errors.As(err, &se) && se.Kind == KindBroken && strings.HasPrefix(se.Msg, "404") {
					return nil, &Error{Kind: KindLoginRefused, Msg: iptRefused}
				}
				return nil, err
			}
			return parseIPTorrentsFeed(raw)
		}
		if k.cookie() == "" {
			return nil, &Error{Kind: KindLoginRefused, Msg: "IPTorrents needs your user id and your passkey"}
		}
		// A Source saved with a browser cookie and no passkey still reads the site's search page.
		raw, err := c.send(ctx, http.MethodGet, base+"/t?"+iptSearchValues(text).Encode(), nil, http.Header{"Cookie": {k.cookie()}})
		if err != nil {
			return nil, err
		}
		return parseIPTorrents(raw, base, k.passkey)
	}
	return nil, &Error{Kind: KindBroken, Msg: "unknown built-in site"}
}

// knabenCategories maps the query's media type to Knaben's top categories.
func knabenCategories(q Query) []int {
	switch {
	case q.feed:
		return nil
	case q.Music:
		return []int{1000000}
	case q.TV:
		return []int{2000000}
	default:
		return []int{3000000}
	}
}

func intPtr(n int) *int { return &n }

func parseKnaben(raw []byte) ([]Item, error) {
	var x struct {
		Hits []struct {
			Title    string `json:"title"`
			Bytes    int64  `json:"bytes"`
			Seeders  int    `json:"seeders"`
			Peers    int    `json:"peers"`
			Hash     string `json:"hash"`
			Magnet   string `json:"magnetUrl"`
			Link     string `json:"link"`
			Details  string `json:"details"`
			ID       string `json:"id"`
			Date     string `json:"date"`
			Category string `json:"category"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "Knaben answered with something that is not its search list"}
	}
	out := make([]Item, 0, len(x.Hits))
	for _, h := range x.Hits {
		it := Item{Title: strings.TrimSpace(h.Title), GUID: h.ID, Link: h.Magnet, Size: h.Bytes, Protocol: "torrent",
			Seeders: intPtr(h.Seeders), Peers: intPtr(h.Seeders + h.Peers)}
		if it.Link == "" {
			it.Link = h.Link
		}
		if it.GUID == "" {
			it.GUID = h.Details
		}
		it.InfoURL = webLink(h.Details)
		if hash := strings.ToLower(strings.TrimSpace(h.Hash)); releasename.ValidInfoHash(hash) {
			it.InfoHash = hash
		}
		if t, err := time.Parse(time.RFC3339, h.Date); err == nil {
			it.Published = t.UTC()
		}
		if it.Title != "" && it.Link != "" {
			out = append(out, it)
		}
	}
	return out, nil
}

func parseTorrentsCSV(raw []byte) ([]Item, error) {
	var x struct {
		Torrents []struct {
			Hash     string `json:"infohash"`
			Name     string `json:"name"`
			Size     int64  `json:"size_bytes"`
			Created  int64  `json:"created_unix"`
			Seeders  int    `json:"seeders"`
			Leechers int    `json:"leechers"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "TorrentsCSV answered with something that is not its search list"}
	}
	out := make([]Item, 0, len(x.Torrents))
	for _, t := range x.Torrents {
		hash := strings.ToLower(strings.TrimSpace(t.Hash))
		name := strings.TrimSpace(t.Name)
		if !releasename.ValidInfoHash(hash) || name == "" {
			continue
		}
		it := Item{Title: name, GUID: hash, InfoHash: hash, Size: t.Size, Protocol: "torrent",
			Link:    "magnet:?xt=urn:btih:" + hash + "&dn=" + url.QueryEscape(name),
			Seeders: intPtr(t.Seeders), Peers: intPtr(t.Seeders + t.Leechers)}
		if t.Created > 0 {
			it.Published = time.Unix(t.Created, 0).UTC()
		}
		out = append(out, it)
	}
	return out, nil
}

func parseSubsPlease(raw []byte) ([]Item, error) {
	// An empty search answers with an empty list rather than an object.
	if s := strings.TrimSpace(string(raw)); s == "[]" || s == "" {
		return nil, nil
	}
	var x map[string]struct {
		Release   string `json:"release_date"`
		Show      string `json:"show"`
		Episode   string `json:"episode"`
		Downloads []struct {
			Res    string `json:"res"`
			Magnet string `json:"magnet"`
		} `json:"downloads"`
	}
	if err := json.Unmarshal(raw, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "SubsPlease answered with something that is not its release list"}
	}
	keys := make([]string, 0, len(x))
	for k := range x {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []Item
	for _, k := range keys {
		e := x[k]
		var when time.Time
		if t, err := time.Parse(time.RFC1123Z, e.Release); err == nil {
			when = t.UTC()
		}
		for _, d := range e.Downloads {
			m, err := url.Parse(d.Magnet)
			if err != nil || m.Scheme != "magnet" {
				continue
			}
			mq := m.Query()
			name := strings.TrimSpace(mq.Get("dn"))
			if name == "" {
				name = fmt.Sprintf("[SubsPlease] %s - %s (%sp)", e.Show, e.Episode, d.Res)
			}
			it := Item{Title: name, GUID: d.Magnet, Link: d.Magnet, Protocol: "torrent", Published: when}
			it.Size, _ = strconv.ParseInt(mq.Get("xl"), 10, 64)
			if h := magnetHash(mq.Get("xt")); h != "" {
				it.InfoHash = h
			}
			out = append(out, it)
		}
	}
	return out, nil
}

// magnetHash reads the info hash of a magnet's xt, in hex or base32.
func magnetHash(xt string) string {
	h, ok := strings.CutPrefix(strings.ToLower(xt), "urn:btih:")
	if !ok {
		return ""
	}
	if releasename.ValidInfoHash(h) {
		return h
	}
	if len(h) == 32 {
		const alpha = "abcdefghijklmnopqrstuvwxyz234567"
		var bits, n uint
		var out []byte
		for _, r := range h {
			i := strings.IndexRune(alpha, r)
			if i < 0 {
				return ""
			}
			bits = bits<<5 | uint(i)
			n += 5
			if n >= 8 {
				n -= 8
				out = append(out, byte(bits>>n))
				bits &= 1<<n - 1
			}
		}
		return fmt.Sprintf("%x", out)
	}
	return ""
}

// IPTorrents category numbers. Each one holds its sub-categories; anime has
// its own, apart from movies and shows.
const (
	iptMovies = 72
	iptTV     = 73
	iptMusic  = 75
	iptAnime  = 60
)

// iptCategories gives the site's categories for a query: the one media type a
// search is for, or for the newest list the types the Source's categories
// name (all of them when it names none). The site's RSS answers with nothing
// when no category is asked for and there are no search words.
func iptCategories(q Query) []int {
	var movie, tv, music bool
	switch {
	case !q.feed:
		music = q.Music
		tv = q.TV && !q.Music
		movie = !tv && !music
	default:
		for _, n := range q.Categories {
			switch n / 1000 {
			case 2:
				movie = true
			case 3:
				music = true
			case 5:
				tv = true
			}
		}
		if !movie && !tv && !music {
			movie, tv, music = true, true, true
		}
	}
	var out []int
	if movie {
		out = append(out, iptMovies)
	}
	if tv {
		out = append(out, iptTV)
	}
	if music {
		out = append(out, iptMusic)
	}
	if movie || tv {
		out = append(out, iptAnime)
	}
	return out
}

// iptKey is what a person gives for IPTorrents: their user id and their
// passkey. A Source saved before the two had their own fields holds one line
// instead, such as "u=1; tp=def" or a browser cookie "uid=1; pass=abc".
type iptKey struct {
	pairs   [][2]string
	uid     string
	passkey string
}

// iptKeys reads the stored key. user is the user id kept beside it.
func iptKeys(raw, user string) iptKey {
	user = strings.TrimSpace(user)
	k := iptKey{uid: user}
	if !strings.Contains(raw, "=") {
		k.passkey = strings.TrimSpace(raw)
		return k
	}
	if _, query, ok := strings.Cut(raw, "?"); ok && strings.Contains(raw, "://") {
		// A pasted RSS or download link from the site: read its query.
		raw = query
	}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ';' || r == '&' || r == '\n' }) {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || name == "" || value == "" {
			continue
		}
		switch strings.ToLower(name) {
		case "tp", "torrent_pass", "passkey":
			k.passkey = value
		case "u":
			if user == "" {
				k.uid = value
			}
		default:
			if strings.EqualFold(name, "uid") && k.uid == "" {
				k.uid = value
			}
			k.pairs = append(k.pairs, [2]string{name, value})
		}
	}
	return k
}

// iptSearchValues is the search page's query: best seeded first for a search,
// newest first for the plain list.
func iptSearchValues(text string) url.Values {
	v := url.Values{"o": {"seeders"}}
	if text != "" {
		v.Set("q", text)
	} else {
		v.Set("o", "time")
	}
	return v
}

// iptCookie cleans a browser cookie as pasted: one line, without a leading
// "Cookie:" that some browsers copy along.
func iptCookie(raw string) string {
	s := strings.Join(strings.Fields(raw), " ")
	if len(s) > 7 && strings.EqualFold(s[:7], "cookie:") {
		s = strings.TrimSpace(s[7:])
	}
	return strings.Trim(s, "; ")
}

// iptCookieHeader is the Cookie header, with the browser's user agent when one was given.
func iptCookieHeader(cookie, agent string) http.Header {
	h := http.Header{"Cookie": {cookie}}
	if agent = strings.TrimSpace(agent); agent != "" {
		h.Set("User-Agent", agent)
	}
	return h
}

// cookie is the Cookie header, empty when no pass was given.
func (k iptKey) cookie() string {
	has := false
	parts := make([]string, 0, len(k.pairs))
	for _, p := range k.pairs {
		if strings.EqualFold(p[0], "pass") {
			has = true
		}
		parts = append(parts, p[0]+"="+p[1])
	}
	if !has {
		return ""
	}
	return strings.Join(parts, "; ")
}

var (
	iptRow = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	// The title link is the row's "hv" link; the site has pointed it at
	// /t/<id> and at /details.php?id=<id> over time, so either address counts.
	iptAnchor  = regexp.MustCompile(`(?is)<a\b([^>]*)>(.*?)</a>`)
	iptHref    = regexp.MustCompile(`(?is)\bhref\s*=\s*["']?([^"'\s>]+)`)
	iptHV      = regexp.MustCompile(`(?is)\bclass\s*=\s*["'][^"']*\bhv\b`)
	iptDetails = regexp.MustCompile(`(?i)^/(?:details\.php\?id=|t/)\d+`)
	iptDL      = regexp.MustCompile(`(?is)href="(/download\.php/[^"]+\.torrent)"`)
	iptCell    = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	iptTags    = regexp.MustCompile(`(?s)<[^>]+>`)
	iptSizeRe  = regexp.MustCompile(`(?i)([\d.,]+)\s*([KMGT]i?B)`)
	iptSeeders = regexp.MustCompile(`(?is)class="[^"]*t_seeders[^"]*"[^>]*>\s*([\d,]+)`)
	iptLeech   = regexp.MustCompile(`(?is)class="[^"]*t_leechers[^"]*"[^>]*>\s*([\d,]+)`)
)

func cellText(s string) string {
	return strings.TrimSpace(htmlesc.UnescapeString(iptTags.ReplaceAllString(s, " ")))
}

// parseSize reads "1.4 GB" style sizes.
func parseSize(s string) int64 {
	// HTML sizes can separate the value and unit with &nbsp; (or a narrow
	// nonbreaking space), which regexp's ASCII-only \s does not match.
	s = strings.Join(strings.Fields(cellText(s)), " ")
	m := iptSizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return 0
	}
	mult := map[byte]float64{'K': 1 << 10, 'M': 1 << 20, 'G': 1 << 30, 'T': 1 << 40}[strings.ToUpper(m[2])[0]]
	return int64(f * mult)
}

var iptSwarm = regexp.MustCompile(`\(S:\s*([\d,]+)\s+L:\s*([\d,]+)\)`)

const iptRefused = "IPTorrents refused the user id or the passkey; check both"

// iptSignIn says whether a page is the site's sign-in page, which it shows
// in place of an answer it refuses.
func iptSignIn(page string) bool {
	return strings.Contains(page, `name="password"`) || strings.Contains(strings.ToLower(page), "login.php") && !strings.Contains(page, "t_seeders")
}

// parseIPTorrentsFeed reads the site's RSS. Each item's description holds the
// size, the category and the swarm, as "1.46 GB; TV/x265 (S:12 L:3)"; the
// download link already carries the passkey.
func parseIPTorrentsFeed(raw []byte) ([]Item, error) {
	items, err := parseItems(raw, "torrent")
	if err != nil {
		if iptSignIn(string(raw)) {
			return nil, &Error{Kind: KindLoginRefused, Msg: iptRefused}
		}
		return nil, err
	}
	var x struct {
		Items []struct {
			Link      string `xml:"link"`
			Enclosure struct {
				URL string `xml:"url,attr"`
			} `xml:"enclosure"`
			Description string `xml:"description"`
		} `xml:"channel>item"`
	}
	if xml.Unmarshal(raw, &x) != nil {
		return items, nil
	}
	said := make(map[string]string, len(x.Items))
	for _, it := range x.Items {
		said[strings.TrimSpace(it.Link)] = it.Description
		if it.Enclosure.URL != "" {
			said[it.Enclosure.URL] = it.Description
		}
	}
	for i := range items {
		text := said[items[i].Link]
		if items[i].Size == 0 {
			items[i].Size = parseSize(text)
		}
		if m := iptSwarm.FindStringSubmatch(text); m != nil {
			seeders, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
			leechers, _ := strconv.Atoi(strings.ReplaceAll(m[2], ",", ""))
			items[i].Seeders, items[i].Peers = intPtr(seeders), intPtr(seeders+leechers)
		}
	}
	return items, nil
}

// parseIPTorrents reads the site's search page. Download links get the
// person's passkey when they gave one, so a Downloader can fetch them with no cookie.
func parseIPTorrents(raw []byte, base, passkey string) ([]Item, error) {
	page := string(raw)
	if iptSignIn(page) {
		return nil, &Error{Kind: KindLoginRefused, Msg: "IPTorrents shows its sign-in page; give the Source your user id and your passkey instead of a cookie"}
	}
	var out []Item
	links := 0
	for _, row := range iptRow.FindAllStringSubmatch(page, -1) {
		r := row[1]
		d := iptDL.FindStringSubmatch(r)
		if d == nil {
			continue
		}
		links++
		href, title := iptTitleLink(r)
		if href == "" {
			continue
		}
		it := Item{Title: title, GUID: base + href, Link: base + d[1], Protocol: "torrent"}
		if passkey != "" {
			it.Link += "?torrent_pass=" + url.QueryEscape(passkey)
		}
		for _, c := range iptCell.FindAllStringSubmatch(r, -1) {
			if n := parseSize(cellText(c[1])); n > 0 && it.Size == 0 && !strings.Contains(c[1], "details.php") {
				it.Size = n
			}
		}
		if m := iptSeeders.FindStringSubmatch(r); m != nil {
			n, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
			it.Seeders = intPtr(n)
			if l := iptLeech.FindStringSubmatch(r); l != nil {
				p, _ := strconv.Atoi(strings.ReplaceAll(l[1], ",", ""))
				it.Peers = intPtr(n + p)
			}
		}
		if it.Title != "" {
			out = append(out, it)
		}
	}
	if links > 0 && len(out) == 0 {
		// Rows with files but no readable name: the page changed shape. Say so
		// rather than report an empty, healthy search.
		return nil, &Error{Kind: KindBroken, Msg: "IPTorrents answered with a list HolyArrs could not read"}
	}
	return out, nil
}

// iptTitleLink finds a row's title link: the link classed "hv", or else the
// first link to a details page. It gives the link's address and its text.
func iptTitleLink(row string) (href, title string) {
	var fallback [2]string
	for _, a := range iptAnchor.FindAllStringSubmatch(row, -1) {
		h := iptHref.FindStringSubmatch(a[1])
		if h == nil {
			continue
		}
		addr, text := htmlesc.UnescapeString(h[1]), cellText(a[2])
		if text == "" || !strings.HasPrefix(addr, "/") || strings.HasPrefix(addr, "/download.php") {
			continue
		}
		if iptHV.MatchString(a[1]) {
			return addr, text
		}
		if fallback[0] == "" && iptDetails.MatchString(addr) {
			fallback = [2]string{addr, text}
		}
	}
	return fallback[0], fallback[1]
}

// siteTest reads the site's newest list; TorrentsCSV has none, so it runs one small search.
func (c *Client) siteTest(ctx context.Context) (int, error) {
	q := Query{feed: true}
	if c.cfg.Site == "torrentscsv" {
		q = Query{Title: "linux"}
	}
	items, err := c.siteSearch(ctx, q, c.cfg.Observe)
	return len(items), err
}

// FileHeader gives the headers src's own site needs to hand out a Release
// file: the IPTorrents sign-in cookie. Download links from a cookie-only
// search carry no passkey, so the Downloader could not fetch them; HolyArrs
// fetches the file with this cookie and sends the file instead. Nil for
// every other Source, and the cookie is never put in a link.
func FileHeader(ctx context.Context, src domain.Source, sec SecretReader) (http.Header, error) {
	if src.Extra[extraSiteKind] != "iptorrents" {
		return nil, nil
	}
	cfg, err := ConfigFor(ctx, src, sec)
	if err != nil {
		return nil, err
	}
	if cfg.SiteLogin == "cookie" {
		if c := iptCookie(cfg.APIKey); c != "" {
			return iptCookieHeader(c, cfg.SiteAgent), nil
		}
		return nil, nil
	}
	k := iptKeys(cfg.APIKey, cfg.SiteUser)
	if k.uid != "" && k.passkey != "" {
		// Links from the site's RSS carry the passkey; they need no cookie.
		return nil, nil
	}
	if c := k.cookie(); c != "" {
		return http.Header{"Cookie": {c}}, nil
	}
	return nil, nil
}

// FileHeaders is FileHeader reading keys from sec, as the grab step asks.
func FileHeaders(sec SecretReader) func(context.Context, domain.Source) (http.Header, error) {
	return func(ctx context.Context, src domain.Source) (http.Header, error) { return FileHeader(ctx, src, sec) }
}

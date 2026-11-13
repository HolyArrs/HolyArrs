package sources

import (
	"context"
	"encoding/json"
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
		k := iptKeys(c.cfg.APIKey)
		if k.cookie() == "" && (k.uid == "" || k.passkey == "") {
			return nil, &Error{Kind: KindLoginRefused, Msg: "IPTorrents needs your cookie (uid and pass) or your RSS key (u and tp)"}
		}
		if k.cookie() == "" {
			// Key only: the site's own RSS search.
			addr := base + "/t.rss?u=" + url.QueryEscape(k.uid) + ";tp=" + url.QueryEscape(k.passkey) + ";download"
			if text != "" {
				addr += ";q=" + url.QueryEscape(text)
			}
			raw, err := c.send(ctx, http.MethodGet, addr, nil, nil)
			if err != nil {
				return nil, err
			}
			items, err := parseItems(raw, "torrent")
			for i := range items {
				items[i].Size = iptSize(items[i].Size, items[i].GUID)
			}
			return items, err
		}
		v := url.Values{"o": {"seeders"}}
		if text != "" {
			v.Set("q", text)
		} else {
			v.Set("o", "time")
		}
		raw, err := c.send(ctx, http.MethodGet, base+"/t?"+v.Encode(), nil, http.Header{"Cookie": {k.cookie()}})
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

// iptKey is what a person gives for IPTorrents: their cookie (uid and pass,
// plus anything else the site set) and, optionally, their RSS key (u and tp),
// in one line such as "uid=1; pass=abc; tp=def".
type iptKey struct {
	pairs   [][2]string
	uid     string
	passkey string
}

func iptKeys(raw string) iptKey {
	var k iptKey
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
			k.uid = value
		default:
			if strings.EqualFold(name, "uid") && k.uid == "" {
				k.uid = value
			}
			k.pairs = append(k.pairs, [2]string{name, value})
		}
	}
	return k
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
	iptRow     = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	iptTitle   = regexp.MustCompile(`(?is)<a[^>]+href="(/details\.php\?id=(\d+))"[^>]*>(.*?)</a>`)
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

// iptSize keeps a size the RSS gave, or reads it from the item's text.
func iptSize(size int64, text string) int64 {
	if size > 0 {
		return size
	}
	return parseSize(text)
}

// parseIPTorrents reads the site's search page. Download links get the
// person's passkey when they gave one, so a Downloader can fetch them with no cookie.
func parseIPTorrents(raw []byte, base, passkey string) ([]Item, error) {
	page := string(raw)
	if strings.Contains(page, `name="password"`) || strings.Contains(strings.ToLower(page), "login.php") && !strings.Contains(page, "t_seeders") {
		return nil, &Error{Kind: KindLoginRefused, Msg: "IPTorrents shows its sign-in page; copy a fresh cookie from your browser"}
	}
	var out []Item
	for _, row := range iptRow.FindAllStringSubmatch(page, -1) {
		r := row[1]
		t := iptTitle.FindStringSubmatch(r)
		d := iptDL.FindStringSubmatch(r)
		if t == nil || d == nil {
			continue
		}
		it := Item{Title: cellText(t[3]), GUID: base + t[1], Link: base + d[1], Protocol: "torrent"}
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
	return out, nil
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
	if c := iptKeys(cfg.APIKey).cookie(); c != "" {
		return http.Header{"Cookie": {c}}, nil
	}
	return nil, nil
}

// FileHeaders is FileHeader reading keys from sec, as the grab step asks.
func FileHeaders(sec SecretReader) func(context.Context, domain.Source) (http.Header, error) {
	return func(ctx context.Context, src domain.Source) (http.Header, error) { return FileHeader(ctx, src, sec) }
}

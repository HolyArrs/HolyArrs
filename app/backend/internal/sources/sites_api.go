package sources

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	htmlesc "html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Built-in sites that have an API or a feed of their own: The Pirate Bay
// (apibay JSON), YTS (JSON), Bangumi Moe (JSON), Nyaa and Tokyo Toshokan
// (RSS). The sites in sites_pages.go are read from their search pages.

// siteFunc searches one built-in site; text is "" for its newest list.
type siteFunc func(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error)

var moreSites = map[string]siteFunc{
	"thepiratebay":    tpbSearch,
	"yts":             ytsSearch,
	"bangumimoe":      bangumiSearch,
	"nyaasi":          nyaaSearch,
	"tokyotosho":      tokyoSearch,
	"1337x":           x1337Search,
	"torrent9":        torrent9Search,
	"torrentdownload": torrentDownloadSearch,
	"limetorrents":    limeSearch,
	"bigfangroup":     bfgSearch,
}

// publicTrackers go on magnets HolyArrs builds from a bare info hash, as
// Prowlarr does, so a torrent app finds peers without waiting on DHT alone.
var publicTrackers = []string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.stealth.si:80/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://open.demonii.com:1337/announce",
}

// magnetFor builds a magnet from an info hash and a name.
func magnetFor(hash, name string) string {
	v := url.Values{"dn": {name}, "tr": publicTrackers}
	return "magnet:?xt=urn:btih:" + strings.ToLower(hash) + "&" + v.Encode()
}

// imdbOf is a title's IMDb id as tt and digits, or "".
func imdbOf(q Query) string {
	id := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(q.IMDb)), "tt")
	if id == "" {
		return ""
	}
	if _, err := strconv.Atoi(id); err != nil {
		return ""
	}
	return "tt" + id
}

// imdbDigits is an IMDb id as Item.IMDb keeps it: digits, no leading zeros.
func imdbDigits(id string) string {
	return strings.TrimLeft(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id)), "tt"), "0")
}

// siteGet makes one GET to a built-in site, waiting its turn first unless the
// caller already did (the first call of a search waits in siteSearch).
func (c *Client) siteGet(ctx context.Context, addr string, again bool) ([]byte, error) {
	if again {
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
	}
	return c.send(ctx, http.MethodGet, addr, nil, nil)
}

// tpbCategories are The Pirate Bay's video and audio categories for a query.
func tpbCategories(q Query) string {
	switch {
	case q.Music:
		return "101,104"
	case q.TV:
		return "205,208,212"
	default:
		return "201,202,207,209,211"
	}
}

func tpbSearch(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	if text == "" {
		raw, err := c.siteGet(ctx, base+"/precompiled/data_top100_recent.json", false)
		if err != nil {
			return nil, err
		}
		return parseTPB(raw)
	}
	ask := func(words string, again bool) ([]Item, error) {
		raw, err := c.siteGet(ctx, base+"/q.php?"+url.Values{"q": {words}, "cat": {tpbCategories(q)}}.Encode(), again)
		if err != nil {
			return nil, err
		}
		return parseTPB(raw)
	}
	// A movie is asked by its IMDb id first: the site files many uploads under it.
	if id := imdbOf(q); id != "" && !q.TV && !q.Music {
		items, err := ask(id, false)
		if err != nil || len(items) > 0 {
			return items, err
		}
		return ask(text, true)
	}
	return ask(text, false)
}

// tpbText is one apibay field: the search writes numbers as strings, the
// recent list as plain numbers.
type tpbText string

func (t *tpbText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = tpbText(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*t = tpbText(n.String())
	return nil
}

func parseTPB(raw []byte) ([]Item, error) {
	var rows []struct {
		ID       tpbText `json:"id"`
		Name     string  `json:"name"`
		Hash     string  `json:"info_hash"`
		Leechers tpbText `json:"leechers"`
		Seeders  tpbText `json:"seeders"`
		Size     tpbText `json:"size"`
		Files    tpbText `json:"num_files"`
		Added    tpbText `json:"added"`
		IMDb     string  `json:"imdb"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "The Pirate Bay answered with something that is not its search list"}
	}
	out := make([]Item, 0, len(rows))
	for _, r := range rows {
		hash := strings.ToLower(strings.TrimSpace(r.Hash))
		name := strings.TrimSpace(r.Name)
		// No results come back as one row with id 0 and an all-zero hash.
		if r.ID == "0" || !releasename.ValidInfoHash(hash) || strings.Trim(hash, "0") == "" || name == "" {
			continue
		}
		seeders, _ := strconv.Atoi(string(r.Seeders))
		leechers, _ := strconv.Atoi(string(r.Leechers))
		it := Item{Title: name, GUID: "https://thepiratebay.org/description.php?id=" + string(r.ID), Link: magnetFor(hash, name), InfoHash: hash,
			Protocol: "torrent", Seeders: intPtr(seeders), Peers: intPtr(seeders + leechers), IMDb: imdbDigits(r.IMDb)}
		it.Size, _ = strconv.ParseInt(string(r.Size), 10, 64)
		it.Files, _ = strconv.Atoi(string(r.Files))
		if n, err := strconv.ParseInt(string(r.Added), 10, 64); err == nil && n > 0 {
			it.Published = time.Unix(n, 0).UTC()
		}
		out = append(out, it)
	}
	return out, nil
}

func ytsSearch(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	// Without a limit the API sometimes answers nothing.
	v := url.Values{"limit": {"50"}, "sort_by": {"date_added"}, "order_by": {"desc"}}
	if text != "" {
		// YTS matches the title alone; a year in the words finds nothing.
		term := imdbOf(q)
		if term == "" {
			term = textQuery(Query{Title: q.Title})
		}
		v.Set("query_term", term)
	}
	raw, err := c.siteGet(ctx, base+"/api/v2/list_movies.json?"+v.Encode(), false)
	if err != nil {
		return nil, err
	}
	return parseYTS(raw)
}

func parseYTS(raw []byte) ([]Item, error) {
	var x struct {
		Status string `json:"status"`
		Data   struct {
			Movies []struct {
				URL       string `json:"url"`
				IMDb      string `json:"imdb_code"`
				Title     string `json:"title"`
				TitleLong string `json:"title_long"`
				Year      int    `json:"year"`
				Torrents  []struct {
					URL      string `json:"url"`
					Hash     string `json:"hash"`
					Quality  string `json:"quality"`
					Type     string `json:"type"`
					Codec    string `json:"video_codec"`
					Depth    string `json:"bit_depth"`
					Audio    string `json:"audio_channels"`
					Seeds    int    `json:"seeds"`
					Peers    int    `json:"peers"`
					Size     int64  `json:"size_bytes"`
					Uploaded int64  `json:"date_uploaded_unix"`
				} `json:"torrents"`
			} `json:"movies"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &x); err != nil || x.Status != "ok" {
		return nil, &Error{Kind: KindBroken, Msg: "YTS answered with something that is not its movie list"}
	}
	var out []Item
	for _, m := range x.Data.Movies {
		name := m.TitleLong
		if name == "" {
			name = fmt.Sprintf("%s (%d)", m.Title, m.Year)
		}
		name = strings.ReplaceAll(name, ":", "")
		for _, t := range m.Torrents {
			hash := strings.ToLower(t.Hash)
			if !releasename.ValidInfoHash(hash) {
				continue
			}
			// The site's own words for each copy, as Prowlarr names them.
			rip := "BRRip"
			if t.Type == "web" {
				rip = "WEBRip"
			}
			title := name + " " + t.Quality + " " + rip + " "
			if t.Audio == "5.1" {
				title += "5.1 "
			}
			if t.Depth == "10" {
				title += "10Bit "
			}
			title += t.Codec + " -YTS"
			it := Item{Title: strings.Join(strings.Fields(title), " "), GUID: t.URL, Link: magnetFor(hash, title), InfoHash: hash, Size: t.Size,
				Protocol: "torrent", Seeders: intPtr(t.Seeds), Peers: intPtr(t.Seeds + t.Peers), IMDb: imdbDigits(m.IMDb)}
			if it.GUID == "" {
				it.GUID = m.URL + "#" + hash
			}
			if t.Uploaded > 0 {
				it.Published = time.Unix(t.Uploaded, 0).UTC()
			}
			out = append(out, it)
		}
	}
	return out, nil
}

func bangumiSearch(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	var raw []byte
	var err error
	if text == "" {
		raw, err = c.siteGet(ctx, base+"/api/torrent/latest", false)
	} else {
		payload, _ := json.Marshal(map[string]string{"query": text})
		raw, err = c.send(ctx, http.MethodPost, base+"/api/v2/torrent/search", payload, http.Header{"Content-Type": {"application/json"}})
	}
	if err != nil {
		return nil, err
	}
	return parseBangumi(raw, base)
}

func parseBangumi(raw []byte, base string) ([]Item, error) {
	var x struct {
		Torrents []struct {
			ID       string `json:"_id"`
			Title    string `json:"title"`
			Size     string `json:"size"`
			Magnet   string `json:"magnet"`
			Hash     string `json:"infoHash"`
			Seeders  int    `json:"seeders"`
			Leechers int    `json:"leechers"`
			Time     string `json:"publish_time"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "Bangumi Moe answered with something that is not its torrent list"}
	}
	out := make([]Item, 0, len(x.Torrents))
	for _, t := range x.Torrents {
		hash := strings.ToLower(strings.TrimSpace(t.Hash))
		title := strings.TrimSpace(t.Title)
		if !releasename.ValidInfoHash(hash) || title == "" {
			continue
		}
		it := Item{Title: title, GUID: base + "/torrent/" + t.ID, Link: t.Magnet, InfoHash: hash, Size: parseSize(t.Size),
			Protocol: "torrent", Seeders: intPtr(t.Seeders), Peers: intPtr(t.Seeders + t.Leechers)}
		if !strings.HasPrefix(it.Link, "magnet:") {
			it.Link = magnetFor(hash, title)
		}
		if when, err := time.Parse(time.RFC3339, t.Time); err == nil {
			it.Published = when.UTC()
		}
		out = append(out, it)
	}
	return out, nil
}

func nyaaSearch(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	// 0_0 is every category and f=0 every upload, trusted or not, as Prowlarr asks.
	v := url.Values{"page": {"rss"}, "c": {"0_0"}, "f": {"0"}}
	if text != "" {
		v.Set("q", text)
	}
	raw, err := c.siteGet(ctx, base+"/?"+v.Encode(), false)
	if err != nil {
		return nil, err
	}
	return parseNyaa(raw)
}

func parseNyaa(raw []byte) ([]Item, error) {
	var x struct {
		Items []struct {
			Title    string `xml:"title"`
			Link     string `xml:"link"`
			GUID     string `xml:"guid"`
			PubDate  string `xml:"pubDate"`
			Seeders  int    `xml:"seeders"`
			Leechers int    `xml:"leechers"`
			Hash     string `xml:"infoHash"`
			Size     string `xml:"size"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal(raw, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "Nyaa answered with something that is not its feed"}
	}
	out := make([]Item, 0, len(x.Items))
	for _, r := range x.Items {
		title := strings.TrimSpace(r.Title)
		link := strings.TrimSpace(r.Link)
		if title == "" || link == "" {
			continue
		}
		it := Item{Title: title, GUID: strings.TrimSpace(r.GUID), Link: link, Size: parseSize(r.Size), Protocol: "torrent",
			Seeders: intPtr(r.Seeders), Peers: intPtr(r.Seeders + r.Leechers), Published: rssTime(r.PubDate)}
		if h := strings.ToLower(strings.TrimSpace(r.Hash)); releasename.ValidInfoHash(h) {
			it.InfoHash = h
		}
		out = append(out, it)
	}
	return out, nil
}

// rssTime reads an RSS date, with or without the weekday.
func rssTime(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, "02 Jan 2006 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func tokyoSearch(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	// The site's search page sits behind a bot check; its RSS search does not.
	addr := base + "/rss.php"
	if text != "" {
		addr += "?" + url.Values{"terms": {text}}.Encode()
	}
	raw, err := c.siteGet(ctx, addr, false)
	if err != nil {
		return nil, err
	}
	return parseTokyo(raw)
}

var (
	tokyoMagnet = regexp.MustCompile(`href="(magnet:\?[^"]+)"`)
	tokyoSize   = regexp.MustCompile(`(?i)Size:\s*([\d.,]+\s*[KMGT]i?B)`)
)

func parseTokyo(raw []byte) ([]Item, error) {
	var x struct {
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			Desc    string `xml:"description"`
			GUID    string `xml:"guid"`
			PubDate string `xml:"pubDate"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal(raw, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "Tokyo Toshokan answered with something that is not its feed"}
	}
	out := make([]Item, 0, len(x.Items))
	for _, r := range x.Items {
		title := strings.TrimSpace(r.Title)
		it := Item{Title: title, GUID: strings.TrimSpace(r.GUID), Link: strings.TrimSpace(r.Link), Protocol: "torrent", Published: rssTime(r.PubDate)}
		// The magnet is kept over the torrent link, which often points at another tracker's site.
		if m := tokyoMagnet.FindStringSubmatch(r.Desc); m != nil {
			magnet := htmlesc.UnescapeString(m[1])
			if mu, err := url.Parse(magnet); err == nil {
				if h := magnetHash(mu.Query().Get("xt")); h != "" {
					it.InfoHash = h
					it.Link = magnet
				}
			}
		}
		if m := tokyoSize.FindStringSubmatch(r.Desc); m != nil {
			it.Size = parseSize(m[1])
		}
		if it.Title != "" && it.Link != "" {
			out = append(out, it)
		}
	}
	return out, nil
}

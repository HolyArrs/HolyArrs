package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Built-in sites read from their own search pages: 1337x, Torrent9,
// TorrentDownload, LimeTorrents (its RSS search) and BigFANGroup. The page
// layouts follow what Prowlarr's definitions for them read; none of their
// files is copied here.

// now is the clock relative dates ("2 hours ago") count from; tests set it.
var now = time.Now

// words is a cell's text with its spaces made single.
func words(s string) string { return strings.Join(strings.Fields(cellText(s)), " ") }

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(s), ",", ""))
	return n
}

// pathWords puts search words into an address path, joined by sep.
func pathWords(text, sep string) string {
	w := strings.Fields(text)
	for i := range w {
		w[i] = url.PathEscape(w[i])
	}
	return strings.Join(w, sep)
}

var seasonOnly = regexp.MustCompile(`(?i)\bS(\d{1,3})$`)

// ---- 1337x ----

var (
	x1337Row   = regexp.MustCompile(`(?is)<tr>(.*?)</tr>`)
	x1337Name  = regexp.MustCompile(`(?is)<a href="(/torrent/(\d+)/([^"/]+)/)"[^>]*>(.*?)</a>`)
	x1337Cell  = regexp.MustCompile(`(?is)<td class="coll-([a-z0-9]+)[^"]*"[^>]*>(.*?)</td>`)
	x1337Words = regexp.MustCompile(`\bS(20\d{2})\b`)
)

func x1337Search(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	// A season named by its year (S2024) is searched as the year, as Prowlarr does.
	text = x1337Words.ReplaceAllString(text, "$1")
	cat := "Movies"
	switch {
	case q.Music:
		cat = "Music"
	case q.TV:
		cat = "TV"
	}
	raw, err := c.siteGet(ctx, base+"/sort-category-search/"+pathWords(text, "+")+"/"+cat+"/seeders/desc/1/", false)
	if err != nil {
		return nil, err
	}
	return parse1337x(raw, base, now()), nil
}

func parse1337x(raw []byte, base string, at time.Time) []Item {
	var out []Item
	for _, row := range x1337Row.FindAllStringSubmatch(string(raw), -1) {
		n := x1337Name.FindStringSubmatch(row[1])
		if n == nil {
			continue
		}
		title := words(n[4])
		// Long names are cut short on the page; the address keeps them whole.
		if strings.HasSuffix(title, "...") || strings.HasSuffix(title, "…") {
			if slug, err := url.PathUnescape(n[3]); err == nil {
				title = strings.ReplaceAll(slug, "-", " ")
			}
		}
		title = strings.TrimSpace(strings.TrimSuffix(title, "⭐"))
		it := Item{Title: title, GUID: base + n[1], Link: base + n[1], Protocol: "torrent"}
		var seeders, leechers int
		for _, cell := range x1337Cell.FindAllStringSubmatch(row[1], -1) {
			// The size cell repeats the seeders in a span; only its own text counts.
			body, _, _ := strings.Cut(cell[2], "<span")
			switch cell[1] {
			case "2":
				seeders = atoi(cellText(body))
			case "3":
				leechers = atoi(cellText(body))
			case "4":
				it.Size = parseSize(cellText(body))
			case "date":
				it.Published = date1337x(cellText(body), at)
			}
		}
		it.Seeders, it.Peers = intPtr(seeders), intPtr(seeders+leechers)
		if it.Title != "" {
			out = append(out, it)
		}
	}
	return out
}

var ordinal = regexp.MustCompile(`(\d)(st|nd|rd|th)\b`)

// date1337x reads the site's dates: "9:40am" today, "Mar. 5th" this year,
// "Apr. 21st '21" in another.
func date1337x(s string, at time.Time) time.Time {
	s = strings.TrimSpace(ordinal.ReplaceAllString(s, "$1"))
	if t, err := time.Parse("3:04pm", s); err == nil {
		y, m, d := at.UTC().Date()
		return time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, time.UTC)
	}
	if t, err := time.Parse("Jan. 2 '06", s); err == nil {
		return t
	}
	if t, err := time.Parse("Jan. 2", s); err == nil {
		t = t.AddDate(at.Year(), 0, 0)
		if t.After(at) {
			t = t.AddDate(-1, 0, 0)
		}
		return t
	}
	return time.Time{}
}

// ---- Torrent9 ----

var (
	t9Row   = regexp.MustCompile(`(?is)<tr>(.*?)</tr>`)
	t9Name  = regexp.MustCompile(`(?is)<a\s+title="([^"]*)"\s+href="(/torrent/[^"]+)"`)
	t9Cell  = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	t9Lang  = regexp.MustCompile(`(?i)\b(FRENCH|MULTI|TRUEFRENCH|VOSTFR|SUBFRENCH)\b(.+?)(\b(?:19|20)\d{2}\b)$`)
	t9Octet = regexp.MustCompile(`(?i)(\d)\s*([KMGT])o\b`)
	// t9Season is a French season ("Saison.8"), which release names write S08.
	t9Season = regexp.MustCompile(`(?i)\bSaison([ .])(\d{1,2})\b`)
)

func torrent9Search(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	// A season alone is "saison 1" on this French site.
	if m := seasonOnly.FindStringSubmatch(text); m != nil {
		n, _ := strconv.Atoi(m[1])
		text = seasonOnly.ReplaceAllString(text, "saison "+strconv.Itoa(n))
	}
	raw, err := c.siteGet(ctx, base+"/search_torrent/"+strings.ToLower(pathWords(text, "-"))+".html", false)
	if err != nil {
		return nil, err
	}
	return parseTorrent9(raw, base), nil
}

func parseTorrent9(raw []byte, base string) []Item {
	page := string(raw)
	if i := strings.Index(page, "table-striped"); i >= 0 {
		page = page[i:]
	} else {
		return nil
	}
	var out []Item
	for _, row := range t9Row.FindAllStringSubmatch(page, -1) {
		n := t9Name.FindStringSubmatch(row[1])
		cells := t9Cell.FindAllStringSubmatch(row[1], -1)
		if n == nil || len(cells) < 5 {
			continue
		}
		title := strings.TrimSpace(cellText(n[1]))
		// The language goes after the year, so the year ends the movie's name as release names have it.
		title = t9Lang.ReplaceAllString(title, "$3 $1$2")
		title = t9Season.ReplaceAllStringFunc(title, func(m string) string {
			p := t9Season.FindStringSubmatch(m)
			n, _ := strconv.Atoi(p[2])
			return fmt.Sprintf("S%02d", n)
		})
		title = strings.Join(strings.Fields(title), " ")
		it := Item{Title: title, GUID: base + n[2], Link: base + n[2], Protocol: "torrent",
			Size: parseSize(t9Octet.ReplaceAllString(cellText(cells[2][1]), "$1 ${2}B"))}
		if t, err := time.Parse("02/01/2006", cellText(cells[1][1])); err == nil {
			it.Published = t
		}
		seeders, leechers := atoi(cellText(cells[3][1])), atoi(cellText(cells[4][1]))
		it.Seeders, it.Peers = intPtr(seeders), intPtr(seeders+leechers)
		if it.Title != "" {
			out = append(out, it)
		}
	}
	return out
}

// ---- TorrentDownload ----

var (
	tdRow  = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	tdName = regexp.MustCompile(`(?is)<div class="tt-name"><a href="/([0-9A-Fa-f]{40})/([^"]*)"[^>]*>(.*?)</a>`)
	tdCell = regexp.MustCompile(`(?is)<td class="(tdnormal|tdseed|tdleech)"[^>]*>(.*?)</td>`)
)

func torrentDownloadSearch(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	// /search sorts by peers.
	raw, err := c.siteGet(ctx, base+"/search?"+url.Values{"q": {text}}.Encode(), false)
	if err != nil {
		return nil, err
	}
	return parseTorrentDownload(raw, base, now()), nil
}

func parseTorrentDownload(raw []byte, base string, at time.Time) []Item {
	var out []Item
	for _, row := range tdRow.FindAllStringSubmatch(string(raw), -1) {
		n := tdName.FindStringSubmatch(row[1])
		if n == nil {
			continue
		}
		hash := strings.ToLower(n[1])
		title := words(n[3])
		if !releasename.ValidInfoHash(hash) || title == "" {
			continue
		}
		it := Item{Title: title, GUID: base + "/" + n[1] + "/" + n[2], Link: magnetFor(hash, title), InfoHash: hash, Protocol: "torrent"}
		var normal []string
		var seeders, leechers int
		for _, cell := range tdCell.FindAllStringSubmatch(row[1], -1) {
			switch cell[1] {
			case "tdnormal":
				normal = append(normal, cellText(cell[2]))
			case "tdseed":
				seeders = atoi(cellText(cell[2]))
			case "tdleech":
				leechers = atoi(cellText(cell[2]))
			}
		}
		// The first plain cell is the age, the second the size.
		if len(normal) >= 2 {
			it.Published = ago(normal[0], at)
			it.Size = parseSize(normal[1])
		}
		it.Seeders, it.Peers = intPtr(seeders), intPtr(seeders+leechers)
		out = append(out, it)
	}
	return out
}

var agoWords = regexp.MustCompile(`(?i)^(\d+|a|an|last)\s+(minute|hour|day|week|month|year)s?\+?(\s+ago)?$`)

// ago reads a rough age such as "22 hours ago", "Yesterday", "Last Month"
// or "1 Year+". An age it cannot read gives no date.
func ago(s string, at time.Time) time.Time {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "yesterday") {
		return at.Add(-24 * time.Hour).UTC()
	}
	if strings.EqualFold(s, "today") || strings.HasSuffix(strings.ToLower(s), "just now") {
		return at.UTC()
	}
	m := agoWords.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}
	}
	n := 1
	if v, err := strconv.Atoi(m[1]); err == nil {
		n = v
	}
	switch strings.ToLower(m[2]) {
	case "minute":
		return at.Add(-time.Duration(n) * time.Minute).UTC()
	case "hour":
		return at.Add(-time.Duration(n) * time.Hour).UTC()
	case "day":
		return at.AddDate(0, 0, -n).UTC()
	case "week":
		return at.AddDate(0, 0, -7*n).UTC()
	case "month":
		return at.AddDate(0, -n, 0).UTC()
	default:
		return at.AddDate(-n, 0, 0).UTC()
	}
}

// ---- LimeTorrents ----

var (
	limeSeeds = regexp.MustCompile(`(?i)Seeds:\s*([\d,]+)\s*,\s*Leechers:?\s*([\d,]+)`)
	limeHash  = regexp.MustCompile(`(?i)/torrent/([0-9a-f]{40})\.torrent`)
)

func limeSearch(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	// A season without an episode finds little here, as Prowlarr notes; the title alone finds the packs.
	text = strings.TrimSpace(seasonOnly.ReplaceAllString(text, ""))
	raw, err := c.siteGet(ctx, base+"/searchrss/"+pathWords(text, "%20")+"/", false)
	if err != nil {
		return nil, err
	}
	return parseLime(raw)
}

func parseLime(raw []byte) ([]Item, error) {
	var x struct {
		Items []struct {
			Title     string `xml:"title"`
			PubDate   string `xml:"pubDate"`
			Link      string `xml:"link"`
			Size      int64  `xml:"size"`
			Desc      string `xml:"description"`
			Enclosure struct {
				URL string `xml:"url,attr"`
			} `xml:"enclosure"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal(raw, &x); err != nil {
		return nil, &Error{Kind: KindBroken, Msg: "LimeTorrents answered with something that is not its feed"}
	}
	out := make([]Item, 0, len(x.Items))
	for _, r := range x.Items {
		title := strings.TrimSpace(r.Title)
		h := limeHash.FindStringSubmatch(r.Enclosure.URL)
		if title == "" || h == nil {
			continue
		}
		hash := strings.ToLower(h[1])
		// A magnet, since the torrent file host the site links to is often down.
		it := Item{Title: title, GUID: strings.TrimSpace(r.Link), Link: magnetFor(hash, title), InfoHash: hash, Size: r.Size,
			Protocol: "torrent", Published: rssTime(r.PubDate)}
		if m := limeSeeds.FindStringSubmatch(r.Desc); m != nil {
			s, l := atoi(m[1]), atoi(m[2])
			it.Seeders, it.Peers = intPtr(s), intPtr(s+l)
		}
		out = append(out, it)
	}
	return out, nil
}

// ---- BigFANGroup ----

var (
	bfgRow     = regexp.MustCompile(`(?is)<tr[^>]*>(.*?)</tr>`)
	bfgName    = regexp.MustCompile(`(?is)<a href="details\.php\?id=(\d+)"><b>(.*?)</b></a>`)
	bfgCell    = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	bfgTime    = regexp.MustCompile(`(?is)<img src="pic/time\.png"[^>]*title="([^"]+)"`)
	bfgEpisode = regexp.MustCompile(`(?i)\bS\d{1,4}(E\d{1,4})?\b`)
)

func bfgSearch(c *Client, ctx context.Context, base, text string, q Query) ([]Item, error) {
	v := url.Values{"ajax": {"1"}}
	if text != "" {
		// The site does not know S01E02; it is searched by name, and the judge picks the episode.
		text = strings.Join(strings.Fields(bfgEpisode.ReplaceAllString(text, "")), " ")
		v = url.Values{"ajax": {"1"}, "search": {text}, "cat": {"0"}, "incldead": {"1"}, "year": {"0"}, "format": {"0"}, "s": {"added"}, "d": {"desc"}}
	}
	raw, err := c.siteGet(ctx, base+"/browse.php?"+v.Encode(), false)
	if err != nil {
		return nil, err
	}
	return parseBFG(raw, base), nil
}

// cp1251 turns the site's Windows-1251 page into UTF-8.
func cp1251(raw []byte) []byte {
	if utf8.Valid(raw) {
		return raw
	}
	out, err := charmap.Windows1251.NewDecoder().Bytes(raw)
	if err != nil {
		return raw
	}
	return out
}

func parseBFG(raw []byte, base string) []Item {
	page := string(cp1251(raw))
	if i := strings.Index(page, `id="highlighted"`); i >= 0 {
		page = page[i:]
	}
	var out []Item
	for _, row := range bfgRow.FindAllStringSubmatch(page, -1) {
		n := bfgName.FindStringSubmatch(row[1])
		cells := bfgCell.FindAllStringSubmatch(row[1], -1)
		if n == nil || len(cells) < 8 || !strings.Contains(row[1], "browse.php?cat=") {
			continue
		}
		title := bfgTitle(cellText(n[2]))
		it := Item{Title: title, GUID: base + "/details.php?id=" + n[1], Link: base + "/download.php?id=" + n[1], Protocol: "torrent",
			Size: parseSize(cellText(cells[5][1]))}
		seeders, leechers := atoi(cellText(cells[6][1])), atoi(cellText(cells[7][1]))
		it.Seeders, it.Peers = intPtr(seeders), intPtr(seeders+leechers)
		if m := bfgTime.FindStringSubmatch(row[1]); m != nil {
			it.Published = bfgDate(m[1])
		}
		if it.Title != "" {
			out = append(out, it)
		}
	}
	return out
}

var bfgMonths = strings.NewReplacer("января", "January", "февраля", "February", "марта", "March", "апреля", "April", "мая", "May",
	"июня", "June", "июля", "July", "августа", "August", "сентября", "September", "октября", "October", "ноября", "November", "декабря", "December")

// moscow is the site's clock, three hours ahead of UTC all year.
var moscow = time.FixedZone("MSK", 3*3600)

// bfgDate reads "26 марта 2025 в 14:51:34", in Moscow time.
func bfgDate(s string) time.Time {
	s = bfgMonths.Replace(strings.Replace(strings.TrimSpace(s), " в ", " ", 1))
	t, err := time.ParseInLocation("2 January 2006 15:04:05", s, moscow)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

var bfgNames = []struct {
	re   *regexp.Regexp
	with string
}{
	// "Сезон 1-2, Серии 1-19 из 19" is season 1 to 2, episodes 1 to 19 of 19, as Prowlarr writes it.
	{regexp.MustCompile(`(?i)[CС]езоны?[\s:]*(\d+(?:-\d+)?).+?(?:[CС]ери[ияй]|Эпизод|Выпуски?)[\s:]*(\d+(?:-\d+)?)\s*из\s*(\w*)`), "S${1}E${2} of ${3}"},
	{regexp.MustCompile(`(?i)[CС]езоны?[\s:]*(\d+(?:-\d+)?).+?(?:[CС]ери[ияй]|Эпизод|Выпуски?)[\s:]*(\d+(?:-\d+)?)`), "S${1}E${2}"},
	{regexp.MustCompile(`(?i)[CС]езоны?[\s:]*(\d+(?:-\d+)?)`), "S${1}"},
	{regexp.MustCompile(`(?i)(?:[CС]ери[ияй]|Эпизод|Выпуски?)[\s:]*(\d+(?:-\d+)?)\s*из\s*(\w*)`), "E${1} of ${2}"},
	// "... | LostFilm" names the group.
	{regexp.MustCompile(`\s\|\s(\w{4,})$`), "-${1}"},
	{regexp.MustCompile(`(?i)\bHDTV[-\s]?Rip\b`), "HDTV"},
	{regexp.MustCompile(`(?i)\bWEB[-\s]?DL[-\s]?Rip\b`), "WEB-DL"},
}

// bfgTitle writes the site's Russian season and episode words the way
// release names have them.
func bfgTitle(s string) string {
	for _, r := range bfgNames {
		s = r.re.ReplaceAllString(s, r.with)
	}
	return strings.Join(strings.Fields(s), " ")
}

package kworb

import (
	"errors"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ErrLayout: the page does not have the layout the parser knows.
var ErrLayout = errors.New("kworb: unknown page layout")

// ErrNoTotals: an artist's songs page lists songs but gives no totals
// (kworb does this for pages such as "Various Artists").
var ErrNoTotals = errors.New("kworb: songs page without totals")

// cell is one table cell: its text without tags and the links in it.
type cell struct {
	text  string
	links []string
}

// table is one HTML table: its header texts and its body rows.
type table struct {
	head []string
	rows [][]cell
}

var (
	tableRe = regexp.MustCompile(`(?s)<table[^>]*>(.*?)</table>`)
	headRe  = regexp.MustCompile(`(?s)<th[^>]*>(.*?)</th>`)
	rowRe   = regexp.MustCompile(`(?s)<tr[^>]*>(.*?)</tr>`)
	cellRe  = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	linkRe  = regexp.MustCompile(`<a [^>]*href="([^"]*)"`)
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
)

func text(s string) string {
	return strings.TrimSpace(spaceRe.ReplaceAllString(html.UnescapeString(tagRe.ReplaceAllString(s, "")), " "))
}

// tables reads every table of a page.
func tables(page string) []table {
	var out []table
	for _, m := range tableRe.FindAllStringSubmatch(page, -1) {
		body := m[1]
		var t table
		head, rest := body, body
		if i := strings.Index(body, "<tbody"); i >= 0 {
			head, rest = body[:i], body[i:]
		}
		for _, h := range headRe.FindAllStringSubmatch(head, -1) {
			t.head = append(t.head, text(h[1]))
		}
		for _, r := range rowRe.FindAllStringSubmatch(rest, -1) {
			var row []cell
			for _, c := range cellRe.FindAllStringSubmatch(r[1], -1) {
				cl := cell{text: text(c[1])}
				for _, l := range linkRe.FindAllStringSubmatch(c[1], -1) {
					cl.links = append(cl.links, l[1])
				}
				row = append(row, cl)
			}
			if len(row) > 0 {
				t.rows = append(t.rows, row)
			}
		}
		out = append(out, t)
	}
	return out
}

// find answers the first table whose header is exactly head.
func find(page string, head ...string) (table, bool) {
	for _, t := range tables(page) {
		if len(t.head) != len(head) {
			continue
		}
		same := true
		for i := range head {
			same = same && t.head[i] == head[i]
		}
		if same {
			return t, true
		}
	}
	return table{}, false
}

// number reads "1,234", "+1,234" or "-1,234" ("" reads 0).
func number(s string) (int64, bool) {
	s = strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(s), "+"), ",", "")
	if s == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

// millions reads "143,509.2" (millions) as a count.
func millions(s string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return int64(math.Round(f * 1e6)), true
}

var (
	artistPage = regexp.MustCompile(`artist/([0-9A-Za-z]{22})(?:_songs)?\.html$`)
	trackPage  = regexp.MustCompile(`track/([0-9A-Za-z]{22})\.html$`)
	chartDate  = regexp.MustCompile(`Spotify (?:Daily|Weekly) Chart - [^<]*?- (\d{4})/(\d{2})/(\d{2})`)
	pageDate   = regexp.MustCompile(`Last updated: (\d{4})/(\d{2})/(\d{2})`)
)

func idOf(re *regexp.Regexp, links []string) string {
	for _, l := range links {
		if m := re.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// Listener is one row of the monthly listeners page.
type Listener struct {
	Rank          int    `json:"rank"`
	Name          string `json:"name"`
	Spotify       string `json:"spotify"`
	Listeners     int64  `json:"listeners"`
	Change        int64  `json:"change"`
	Peak          int    `json:"peak"`
	PeakListeners int64  `json:"peak_listeners"`
}

// ParseListeners reads the top artists by monthly listeners: rank, artist
// (with its Spotify id from the row's link), listeners, the day's change,
// peak rank and peak listeners. ErrLayout when the table's columns are not
// the known ones or no row reads.
func ParseListeners(page string) ([]Listener, error) {
	t, ok := find(page, "#", "Artist", "Listeners", "Daily +/-", "Peak", "PkListeners")
	if !ok {
		return nil, ErrLayout
	}
	var out []Listener
	seen := map[string]bool{}
	for _, r := range t.rows {
		if len(r) != 6 {
			continue
		}
		rank, err := strconv.Atoi(r[0].text)
		l := Listener{Rank: rank, Name: r[1].text, Spotify: idOf(artistPage, r[1].links)}
		var ok1, ok2, ok3 bool
		l.Listeners, ok1 = number(r[2].text)
		l.Change, ok2 = number(r[3].text)
		l.Peak, _ = strconv.Atoi(r[4].text)
		l.PeakListeners, ok3 = number(r[5].text)
		if err != nil || !ok1 || !ok2 || !ok3 || rank < 1 || l.Listeners <= 0 || l.Name == "" || l.Spotify == "" || seen[l.Spotify] {
			continue
		}
		seen[l.Spotify] = true
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil, ErrLayout
	}
	return out, nil
}

// Streamed is one row of the most streamed artists page: all-time and the
// day's plays (the page gives millions).
type Streamed struct {
	Rank    int    `json:"rank"`
	Name    string `json:"name"`
	Spotify string `json:"spotify"`
	Total   int64  `json:"total"`
	Daily   int64  `json:"daily"`
}

// ParseArtists reads the most streamed artists of all time, ranked by the
// page's order.
func ParseArtists(page string) ([]Streamed, error) {
	t, ok := find(page, "Artist", "Streams", "Daily", "As lead", "Solo", "As feature")
	if !ok {
		return nil, ErrLayout
	}
	var out []Streamed
	seen := map[string]bool{}
	for _, r := range t.rows {
		if len(r) != 6 {
			continue
		}
		a := Streamed{Rank: len(out) + 1, Name: r[0].text, Spotify: idOf(artistPage, r[0].links)}
		var ok1, ok2 bool
		a.Total, ok1 = millions(r[1].text)
		a.Daily, ok2 = millions(r[2].text)
		if !ok1 || !ok2 || a.Total <= 0 || a.Name == "" || a.Spotify == "" || seen[a.Spotify] {
			continue
		}
		seen[a.Spotify] = true
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, ErrLayout
	}
	return out, nil
}

// Plays is an artist page's first table: total plays and the day's plays
// (all of the artist's songs), and the page's own date.
type Plays struct {
	Total int64  `json:"total"`
	Daily int64  `json:"daily"`
	AsOf  string `json:"as_of"` // YYYY-MM-DD
}

// ParseArtist reads an artist's songs page: the first table's total and
// daily plays, and the "Last updated" date. ErrLayout when the table or the
// date is not where the parser knows them.
func ParseArtist(page string) (Plays, error) {
	d := pageDate.FindStringSubmatch(page)
	ts := tables(page)
	if len(ts) > 0 && len(ts[0].head) > 0 && ts[0].head[0] == "Song Title" && (len(ts[0].head) < 2 || ts[0].head[1] != "Total") && strings.Contains(page, "Last updated: unknown") {
		return Plays{}, ErrNoTotals
	}
	if d == nil || len(ts) == 0 || len(ts[0].head) < 2 || ts[0].head[1] != "Total" {
		return Plays{}, ErrLayout
	}
	p := Plays{AsOf: d[1] + "-" + d[2] + "-" + d[3]}
	for _, r := range ts[0].rows {
		if len(r) < 2 {
			continue
		}
		n, ok := number(r[1].text)
		switch {
		case !ok:
		case r[0].text == "Streams":
			p.Total = n
		case r[0].text == "Daily":
			p.Daily = n
		}
	}
	if p.Total <= 0 {
		return Plays{}, ErrLayout
	}
	return p, nil
}

// ChartRow is one song of a country chart.
type ChartRow struct {
	Rank int `json:"rank"`
	// Move is the change since the last chart: "=", "+3", "-2", "NEW" or
	// "RE" (back on the chart).
	Move          string   `json:"move"`
	Title         string   `json:"title"`
	Artist        string   `json:"artist"`
	ArtistID      string   `json:"artist_id"`
	TrackID       string   `json:"track_id"`
	Featured      []string `json:"featured,omitempty"`
	Days          int      `json:"days"` // days (or weeks) on the chart
	Peak          int      `json:"peak"`
	Streams       int64    `json:"streams"`
	StreamsChange int64    `json:"streams_change"`
	Total         int64    `json:"total"`
}

// Chart is one country chart page.
type Chart struct {
	Day  string     `json:"day"` // the chart's own date, YYYY-MM-DD
	Rows []ChartRow `json:"rows"`
}

// ParseChart reads a daily or weekly country chart: position, move, the
// lead artist and song (with their Spotify ids), featured artists, days
// or weeks on the chart, peak, streams, their change and the total.
func ParseChart(page string, weekly bool) (Chart, error) {
	head := []string{"Pos", "P+", "Artist and Title", "Days", "Pk", "(x?)", "Streams", "Streams+", "7Day", "7Day+", "Total"}
	if weekly {
		head = []string{"Pos", "P+", "Artist and Title", "Wks", "Pk", "(x?)", "Streams", "Streams+", "Total"}
	}
	d := chartDate.FindStringSubmatch(page)
	t, ok := find(page, head...)
	if !ok || d == nil {
		return Chart{}, ErrLayout
	}
	c := Chart{Day: d[1] + "-" + d[2] + "-" + d[3]}
	for _, r := range t.rows {
		if len(r) != len(head) {
			continue
		}
		rank, err := strconv.Atoi(r[0].text)
		row := ChartRow{Rank: rank, Move: r[1].text, TrackID: idOf(trackPage, r[2].links)}
		var artists []string
		for _, l := range r[2].links {
			if m := artistPage.FindStringSubmatch(l); m != nil {
				artists = append(artists, m[1])
			}
		}
		// "Artist - Title (w/ Other, Another)": the lead artist's link comes
		// first, the song's next, the featured artists' after.
		lead, title, found := strings.Cut(r[2].text, " - ")
		if !found || len(artists) == 0 || row.TrackID == "" || err != nil || rank < 1 {
			continue
		}
		row.Artist, row.ArtistID = strings.TrimSpace(lead), artists[0]
		if i := strings.LastIndex(title, " (w/ "); i > 0 && strings.HasSuffix(title, ")") {
			for _, f := range strings.Split(title[i+5:len(title)-1], ",") {
				if f = strings.TrimSpace(f); f != "" {
					row.Featured = append(row.Featured, f)
				}
			}
			title = title[:i]
		}
		row.Title = strings.TrimSpace(title)
		row.Days, _ = strconv.Atoi(r[3].text)
		row.Peak, _ = strconv.Atoi(r[4].text)
		var ok1, ok2, ok3 bool
		row.Streams, ok1 = number(r[6].text)
		row.StreamsChange, ok2 = number(r[7].text)
		row.Total, ok3 = number(r[len(r)-1].text)
		if !ok1 || !ok2 || !ok3 || row.Title == "" || row.Artist == "" {
			continue
		}
		c.Rows = append(c.Rows, row)
	}
	if len(c.Rows) == 0 {
		return Chart{}, ErrLayout
	}
	return c, nil
}

package musictrend

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// Candidate is one MusicBrainz search result.
type Candidate struct {
	MBID       string
	Name       string   // artist name or release group title
	Artists    []string // credited artist names (release groups)
	ArtistMBID string
	// Date, Type and Secondary are a release group's first release date,
	// primary type (Album, Single, EP) and secondary types (Compilation,
	// Live, ...), when the search answer has them.
	Date      string
	Type      string
	Secondary []string
}

// Searcher searches MusicBrainz. kind is "artist" or "album".
type Searcher interface {
	Search(ctx context.Context, kind, name, artist string) ([]Candidate, error)
}

// retryNoMatch is how long a name MusicBrainz did not know waits before
// it is searched again.
const retryNoMatch = 7 * 24 * time.Hour

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS matches (kind TEXT NOT NULL, name TEXT NOT NULL, artist TEXT NOT NULL, mbid TEXT NOT NULL, artist_mbid TEXT NOT NULL, matched_at INTEGER NOT NULL, PRIMARY KEY(kind, name, artist)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS lists (kind TEXT NOT NULL, region TEXT NOT NULL, body BLOB NOT NULL, PRIMARY KEY(kind, region)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS apple_item_artists (kind TEXT NOT NULL, id TEXT NOT NULL, artist TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY(kind, id)) WITHOUT ROWID;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

var fold = strings.NewReplacer("á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "é", "e", "è", "e", "ê", "e", "ë", "e", "í", "i", "ì", "i", "î", "i", "ï", "i", "ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o", "ú", "u", "ù", "u", "û", "u", "ü", "u", "ñ", "n", "ç", "c", "&", "and", "$", "s")

// norm compares names without case, accents, spaces or punctuation.
func norm(s string) string {
	s = fold.Replace(strings.ToLower(strings.TrimSpace(s)))
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

var editionRe = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]\s*$|\s+-\s+(EP|Single|Deluxe.*)$`)

// baseTitle drops a trailing "(Deluxe)", "[Remastered]" or " - EP".
func baseTitle(s string) string {
	for {
		t := editionRe.ReplaceAllString(s, "")
		if t == s || t == "" {
			return s
		}
		s = t
	}
}

// pick takes the first result whose name (and, for albums, a credited
// artist) equals the charted one. A near miss is no match: a wrong id is
// worse than none.
func pick(kind, name, artist string, cs []Candidate) (Candidate, bool) {
	all := pickAll(kind, name, artist, cs)
	if len(all) == 0 {
		return Candidate{}, false
	}
	return all[0], true
}

// pickAll answers every result pick would take, in order.
func pickAll(kind, name, artist string, cs []Candidate) []Candidate {
	want := norm(baseTitle(name))
	if kind == "artist" {
		want = norm(name)
	}
	var out []Candidate
	for _, c := range cs {
		got := norm(c.Name)
		if kind == "album" {
			got = norm(baseTitle(c.Name))
		}
		if got != want || want == "" {
			continue
		}
		if kind == "artist" {
			out = append(out, c)
			continue
		}
		for _, a := range c.Artists {
			if norm(a) == norm(artist) {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// Matcher matches a charted name to a MusicBrainz id once and keeps it.
type Matcher struct {
	DB     *sql.DB
	Search Searcher
	// Unmatched counts searches that ended without an id (a name held back
	// is not searched and not counted); Held counts asks for a name held
	// back (no match within retryNoMatch). A failed search is neither
	// counted nor remembered.
	Unmatched int
	Held      int
}

// Known returns a stored match without searching.
func (m *Matcher) Known(ctx context.Context, kind, name, artist string) (Candidate, bool, bool) {
	var c Candidate
	var at int64
	err := m.DB.QueryRowContext(ctx, "SELECT mbid, artist_mbid, matched_at FROM matches WHERE kind = ? AND name = ? AND artist = ?", kind, name, artist).Scan(&c.MBID, &c.ArtistMBID, &at)
	if err != nil {
		return Candidate{}, false, false
	}
	fresh := c.MBID != "" || time.Since(time.Unix(at, 0)) < retryNoMatch
	return c, c.MBID != "", fresh
}

// Match returns the stored match or searches MusicBrainz once. A failed
// search is not remembered.
func (m *Matcher) Match(ctx context.Context, kind, name, artist string) (Candidate, bool) {
	if c, ok, fresh := m.Known(ctx, kind, name, artist); fresh {
		if !ok {
			m.Held++
		}
		return c, ok
	}
	cs, err := m.Search.Search(ctx, kind, name, artist)
	if err != nil {
		return Candidate{}, false
	}
	c, ok := pick(kind, name, artist, cs)
	if ok && kind == "album" {
		c = m.byArtistMatch(ctx, artist, pickAll(kind, name, artist, cs))
	}
	if !ok {
		m.Unmatched++
	}
	_, _ = m.DB.ExecContext(ctx, "INSERT OR REPLACE INTO matches (kind, name, artist, mbid, artist_mbid, matched_at) VALUES (?, ?, ?, ?, ?, ?)", kind, name, artist, c.MBID, c.ArtistMBID, time.Now().Unix())
	return c, ok
}

// Fetcher is the server's provider client.
type Fetcher interface {
	Fetch(context.Context, model.Query) (model.Result, error)
}

// MBSearch searches MusicBrainz through the server's own paced provider
// client (one request per 1.1 s, shared with every other lookup).
type MBSearch struct{ Fetcher Fetcher }

var luceneSpecial = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

func phrase(s string) string { return `"` + luceneSpecial.Replace(s) + `"` }

// Search asks /artist or /release-group with a quoted name (and artist).
func (m MBSearch) Search(ctx context.Context, kind, name, artist string) ([]Candidate, error) {
	q := model.Query{Provider: "musicbrainz", Kind: "artist", Operation: "search", Q: "artist:" + phrase(name), Page: 1, Limit: 5}
	if kind == "album" {
		q.Kind = "release-group"
		q.Q = "releasegroup:" + phrase(baseTitle(name)) + " AND artist:" + phrase(artist)
	}
	res, err := m.Fetcher.Fetch(ctx, q)
	if err != nil {
		return nil, err
	}
	var body struct {
		Items []struct {
			ID     string `json:"source_id"`
			Title  string `json:"title"`
			Source struct {
				Date      string   `json:"first-release-date"`
				Type      string   `json:"primary-type"`
				Secondary []string `json:"secondary-types"`
				Credit    []struct {
					Name   string `json:"name"`
					Artist struct {
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"artist"`
				} `json:"artist-credit"`
			} `json:"source_data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Data, &body); err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(body.Items))
	for _, it := range body.Items {
		c := Candidate{MBID: strings.ToLower(it.ID), Name: it.Title, Date: it.Source.Date, Type: it.Source.Type, Secondary: it.Source.Secondary}
		for i, cr := range it.Source.Credit {
			c.Artists = append(c.Artists, cr.Name, cr.Artist.Name)
			if i == 0 && validMBID(strings.ToLower(cr.Artist.ID)) {
				c.ArtistMBID = strings.ToLower(cr.Artist.ID)
			}
		}
		if validMBID(c.MBID) {
			out = append(out, c)
		}
	}
	return out, nil
}

// byArtistMatch chooses among same-titled albums by same-named artists (a
// parody's "Meteora" beside the band's): the one by the artist the artist
// match takes for that name, else the first.
func (m *Matcher) byArtistMatch(ctx context.Context, artist string, cs []Candidate) Candidate {
	if len(cs) == 1 {
		return cs[0]
	}
	differ := false
	for _, c := range cs[1:] {
		if c.ArtistMBID != cs[0].ArtistMBID {
			differ = true
		}
	}
	if !differ {
		return cs[0]
	}
	if a, ok := m.Match(ctx, "artist", artist, ""); ok {
		for _, c := range cs {
			if c.ArtistMBID == a.MBID {
				return c
			}
		}
	}
	return cs[0]
}

// RepairHomonyms sets each charted artist name right among same-named
// MusicBrainz artists: of the artist match and the artists its charted
// albums point to, the one the slim copy credits with the most of those
// albums wins (a tie keeps the artist match). The artist match and the
// album matches then follow it, an album only where the copy holds it
// under that artist (slim answers the release group of a title by an
// artist, "" when none). No search is made; it answers how many names
// moved.
func (m *Matcher) RepairHomonyms(ctx context.Context, slim func(ctx context.Context, artist, title string) string) (int, error) {
	type album struct{ name, mbid, artist string }
	rows, err := m.DB.QueryContext(ctx, `SELECT name, mbid FROM matches WHERE kind='artist' AND artist='' AND mbid<>''`)
	if err != nil {
		return 0, err
	}
	names := map[string]string{}
	for rows.Next() {
		var n, id string
		if rows.Scan(&n, &id) == nil {
			names[n] = id
		}
	}
	rows.Close()
	moved := 0
	for name, current := range names {
		ar, err := m.DB.QueryContext(ctx, `SELECT name, mbid, artist_mbid FROM matches WHERE kind='album' AND artist=? AND mbid<>''`, name)
		if err != nil {
			return moved, err
		}
		var albums []album
		cands := []string{current}
		for ar.Next() {
			var a album
			if ar.Scan(&a.name, &a.mbid, &a.artist) == nil {
				albums = append(albums, a)
				if a.artist != "" && !slices.Contains(cands, a.artist) {
					cands = append(cands, a.artist)
				}
			}
		}
		ar.Close()
		if len(cands) < 2 {
			continue
		}
		best, bestN := current, -1
		for _, c := range cands {
			n := 0
			for _, a := range albums {
				if slim(ctx, c, a.name) != "" {
					n++
				}
			}
			if n > bestN {
				best, bestN = c, n
			}
		}
		changed := false
		if best != current {
			if _, err := m.DB.ExecContext(ctx, `UPDATE matches SET mbid=? WHERE kind='artist' AND name=? AND artist=''`, best, name); err != nil {
				return moved, err
			}
			changed = true
		}
		for _, a := range albums {
			if a.artist == best {
				continue
			}
			if rg := slim(ctx, best, a.name); rg != "" {
				if _, err := m.DB.ExecContext(ctx, `UPDATE matches SET mbid=?, artist_mbid=? WHERE kind='album' AND name=? AND artist=?`, rg, best, a.name, name); err != nil {
					return moved, err
				}
				changed = true
			}
		}
		if changed {
			moved++
		}
	}
	return moved, nil
}

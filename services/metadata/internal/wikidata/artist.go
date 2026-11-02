package wikidata

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// An artist's item is found in this order: the Wikidata link on its
// MusicBrainz answer; else the item whose MusicBrainz id statement (P434)
// names it; else one name search (wbsearchentities) whose hit must carry
// the artist's name as its label or an alias, a description that says
// musician, singer, rapper, band, DJ, producer or group, MusicBrainz's
// country and begin year when MusicBrainz has them, and no other
// MusicBrainz id. Two hits that pass, or none, give no item: a wrong
// artist's bio is worse than none.

// ErrLater: the artist's MusicBrainz answer is not stored yet, so a name
// search cannot be checked; ask again later (never kept as "not found").
var ErrLater = errors.New("wikidata: musicbrainz answer not stored yet")

// MBArtist is what an artist's stored MusicBrainz answer says.
type MBArtist struct {
	Name    string
	Country string // ISO 3166-1, "" when none (or worldwide)
	Begin   string // life-span begin: YYYY, YYYY-MM or YYYY-MM-DD
	Genres  []string
	// Wikidata is the item its Wikidata link names ("" when none).
	Wikidata string
}

var qidRe = regexp.MustCompile(`^https?://(?:www\.)?wikidata\.org/wiki/(Q[0-9]+)$`)

// ParseMBArtist reads a stored MusicBrainz artist answer (MusicBrainz's
// own fields under source_data).
func ParseMBArtist(b []byte) (MBArtist, bool) {
	var d struct {
		Source struct {
			Name    string `json:"name"`
			Country string `json:"country"`
			Area    struct {
				Codes []string `json:"iso-3166-1-codes"`
			} `json:"area"`
			Life struct {
				Begin string `json:"begin"`
			} `json:"life-span"`
			Genres []struct {
				Name string `json:"name"`
			} `json:"genres"`
			Relations []struct {
				Type string `json:"type"`
				URL  struct {
					Resource string `json:"resource"`
				} `json:"url"`
			} `json:"relations"`
		} `json:"source_data"`
	}
	if json.Unmarshal(b, &d) != nil || d.Source.Name == "" {
		return MBArtist{}, false
	}
	s := d.Source
	a := MBArtist{Name: s.Name, Country: strings.ToUpper(s.Country), Begin: s.Life.Begin}
	if a.Country == "" && len(s.Area.Codes) > 0 {
		a.Country = strings.ToUpper(s.Area.Codes[0])
	}
	// XW (worldwide) and XE (Europe) are no country.
	if len(a.Country) != 2 || a.Country[0] == 'X' {
		a.Country = ""
	}
	for _, g := range s.Genres {
		if n := strings.TrimSpace(g.Name); n != "" {
			a.Genres = append(a.Genres, n)
		}
	}
	for _, r := range s.Relations {
		if m := qidRe.FindStringSubmatch(r.URL.Resource); m != nil && r.Type == "wikidata" {
			a.Wikidata = m[1]
			break
		}
	}
	return a, true
}

// ArtistFacts are the facts the artist route serves; Sources names where
// each came from ("musicbrainz" or "wikidata").
type ArtistFacts struct {
	Country string            `json:"country,omitempty"`
	Begin   string            `json:"begin,omitempty"`
	Genres  []string          `json:"genres"`
	Sources map[string]string `json:"sources"`
}

// mergeFacts takes each fact from MusicBrainz, and from Wikidata only when
// MusicBrainz has none.
func mergeFacts(mb MBArtist, e Entity) *ArtistFacts {
	f := &ArtistFacts{Genres: []string{}, Sources: map[string]string{}}
	pick := func(field, fromMB, fromWD string) string {
		switch {
		case fromMB != "":
			f.Sources[field] = "musicbrainz"
			return fromMB
		case fromWD != "":
			f.Sources[field] = "wikidata"
			return fromWD
		}
		return ""
	}
	f.Country = pick("country", mb.Country, e.Country)
	f.Begin = pick("begin", mb.Begin, e.Begin)
	switch {
	case len(mb.Genres) > 0:
		f.Genres, f.Sources["genres"] = mb.Genres, "musicbrainz"
	case len(e.Genres) > 0:
		f.Genres, f.Sources["genres"] = e.Genres, "wikidata"
	}
	return f
}

// SetArtistMB sets the stored MusicBrainz answer reader (call before
// serving).
func (s *Service) SetArtistMB(f func(ctx context.Context, mbid string) (MBArtist, bool)) {
	s.opts.ArtistMB = f
}

func (s *Service) artistMB(ctx context.Context, id string) (MBArtist, bool) {
	if s.opts.ArtistMB == nil {
		return MBArtist{}, false
	}
	return s.opts.ArtistMB(ctx, id)
}

// findArtist answers the artist's item and how it was found.
func (s *Service) findArtist(ctx context.Context, id string) (string, string, error) {
	mb, known := s.artistMB(ctx, id)
	if known && mb.Wikidata != "" {
		return mb.Wikidata, "musicbrainz", nil
	}
	qid, err := s.byStatement(ctx, "artist", id)
	if err == nil {
		return qid, "statement", nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", "", err
	}
	if !known {
		if s.opts.ArtistMB == nil {
			return "", "", ErrNotFound
		}
		return "", "", ErrLater
	}
	qid, err = s.searchArtist(ctx, id, mb)
	return qid, "name", err
}

var musicWord = regexp.MustCompile(`(?i)\b(musicians?|singers?|rappers?|bands?|djs?|producers?|groups?)\b`)

// foldName compares names without case, accents, spaces or punctuation.
func foldName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'á', 'à', 'â', 'ä', 'ã', 'å':
			r = 'a'
		case 'é', 'è', 'ê', 'ë':
			r = 'e'
		case 'í', 'ì', 'î', 'ï':
			r = 'i'
		case 'ó', 'ò', 'ô', 'ö', 'õ', 'ø':
			r = 'o'
		case 'ú', 'ù', 'û', 'ü':
			r = 'u'
		case 'ñ':
			r = 'n'
		case 'ç':
			r = 'c'
		case '&':
			b.WriteString("and")
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// searchArtist is the name search with its checks (see the top of this
// file). One search, one read of the hits that pass by name and
// description, and one read of their countries' ISO codes.
func (s *Service) searchArtist(ctx context.Context, id string, mb MBArtist) (string, error) {
	var found struct {
		Search []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Match       struct {
				Text string `json:"text"`
			} `json:"match"`
		} `json:"search"`
	}
	if err := s.api(ctx, url.Values{"action": {"wbsearchentities"}, "search": {mb.Name}, "language": {"en"}, "uselang": {"en"}, "type": {"item"}, "limit": {"10"}}, &found); err != nil {
		return "", err
	}
	want := foldName(mb.Name)
	var ids []string
	for _, h := range found.Search {
		if strings.HasPrefix(h.ID, "Q") && want != "" && foldName(h.Match.Text) == want && musicWord.MatchString(h.Description) {
			ids = append(ids, h.ID)
		}
	}
	if len(ids) == 0 {
		return "", ErrNotFound
	}
	var got struct {
		Entities map[string]item `json:"entities"`
	}
	if err := s.api(ctx, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids, "|")}, "props": {"claims"}}, &got); err != nil {
		return "", err
	}
	var codes map[string]string
	if mb.Country != "" {
		var all []string
		for _, it := range got.Entities {
			all = append(all, countryClaims(it)...)
		}
		var err error
		if codes, err = s.isoCodes(ctx, all); err != nil {
			return "", err
		}
	}
	pass := []string{}
	for _, q := range ids {
		it, ok := got.Entities[q]
		if !ok || it.Missing != nil {
			continue
		}
		if len(it.Claims["P434"]) > 0 && !hasValue(it.Claims["P434"], id) {
			continue // another MusicBrainz artist's item
		}
		if y := year(mb.Begin); y != "" && year(artistBegin(it)) != y {
			continue
		}
		if mb.Country != "" {
			// Either of two citizenships matches.
			hit := false
			for _, c := range countryClaims(it) {
				hit = hit || codes[c] == mb.Country
			}
			if !hit {
				continue
			}
		}
		pass = append(pass, q)
	}
	if len(pass) != 1 {
		return "", ErrNotFound
	}
	return pass[0], nil
}

func hasValue(cs []claim, v string) bool {
	for _, c := range cs {
		if strings.EqualFold(c.Mainsnak.str(), v) {
			return true
		}
	}
	return false
}

func year(date string) string {
	if len(date) < 4 {
		return ""
	}
	return date[:4]
}

// artistBegin is a person's birth date (P569) or a group's start (P571).
func artistBegin(it item) string {
	if d := best(it.Claims["P569"]).date(); d != "" {
		return d
	}
	return best(it.Claims["P571"]).date()
}

// countryClaims are an item's countries: citizenship (P27), then country
// of origin (P495).
func countryClaims(it item) []string {
	var out []string
	for _, p := range []string{"P27", "P495"} {
		for _, c := range it.Claims[p] {
			if q := c.Mainsnak.qid(); q != "" && c.Rank != "deprecated" {
				out = append(out, q)
			}
		}
	}
	return out
}

// countryOf is the item's first country as an ISO 3166-1 code (P297 of the
// country item), "" when it has none.
func (s *Service) countryOf(ctx context.Context, it item) (string, error) {
	cs := countryClaims(it)
	if len(cs) == 0 {
		return "", nil
	}
	codes, err := s.isoCodes(ctx, cs)
	if err != nil {
		return "", err
	}
	for _, q := range cs {
		if c := codes[q]; c != "" {
			return c, nil
		}
	}
	return "", nil
}

// isoCodes reads the ISO 3166-1 codes (P297) of country items, 50 a call.
func (s *Service) isoCodes(ctx context.Context, qids []string) (map[string]string, error) {
	out := map[string]string{}
	seen := map[string]bool{}
	var ids []string
	for _, q := range qids {
		if !seen[q] {
			seen[q] = true
			ids = append(ids, q)
		}
	}
	for len(ids) > 0 {
		n := min(50, len(ids))
		var got struct {
			Entities map[string]item `json:"entities"`
		}
		if err := s.api(ctx, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids[:n], "|")}, "props": {"claims"}}, &got); err != nil {
			return nil, err
		}
		for q, it := range got.Entities {
			if c := best(it.Claims["P297"]).str(); len(c) == 2 {
				out[q] = strings.ToUpper(c)
			}
		}
		ids = ids[n:]
	}
	return out, nil
}

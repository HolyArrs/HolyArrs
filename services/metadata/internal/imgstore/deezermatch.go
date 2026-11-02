package imgstore

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// Deezer matching by IDs: an artist or album is matched to Deezer from the
// slim MusicBrainz copy, strictest evidence first, and an unsure match is
// no match.
//
//	artist: Deezer link on the MusicBrainz artist > exact-name search
//	        with a clear fan lead (one candidate, or 10x the next)
//	album:  Deezer link on the release group > a release barcode (UPC) by
//	        the same artist > the matched artist's album with the same
//	        title, year within 1, track count within 2 and total length
//	        within 5%
//
// Matches and misses are kept in source_ids under source "deezer" (store:
// how it was found, ext: the Deezer id), tagged with the source and the
// Deezer id.

var deezerAlbumLink = regexp.MustCompile(`^https?://(?:www\.)?deezer\.com/(?:[a-z]{2}/)?album/([0-9]+)`)

// Match methods (kept as the store column).
const (
	MatchLink   = "link"
	MatchUPC    = "upc"
	MatchSearch = "search"
)

// mbRels is the part of a stored MusicBrainz answer the matcher reads.
type mbRels struct {
	Relations []struct {
		URL struct {
			Resource string `json:"resource"`
		} `json:"url"`
	} `json:"relations"`
}

func (m mbRels) find(re *regexp.Regexp) string {
	for _, r := range m.Relations {
		if s := re.FindStringSubmatch(r.URL.Resource); s != nil {
			return s[1]
		}
	}
	return ""
}

type mbArtistData struct {
	mbRels
	Name string `json:"name"`
}

type mbGroupData struct {
	mbRels
	Title        string `json:"title"`
	FirstRelease string `json:"first-release-date"`
	ArtistCredit []struct {
		Name   string `json:"name"`
		Artist struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artist"`
	} `json:"artist-credit"`
	Releases []struct {
		Barcode *string `json:"barcode"`
		Status  *string `json:"status"`
		Tracks  int     `json:"track-count"`
	} `json:"releases"`
	Media []struct {
		Tracks []struct {
			Length *int64 `json:"length"`
		} `json:"tracks"`
	} `json:"media"`
}

// tracks answers the chosen edition's track count and total length (ms;
// 0 when any track has no length).
func (g mbGroupData) tracks() (n int, ms int64) {
	known := true
	for _, m := range g.Media {
		for _, t := range m.Tracks {
			n++
			if t.Length == nil || *t.Length <= 0 {
				known = false
			} else {
				ms += *t.Length
			}
		}
	}
	if !known {
		ms = 0
	}
	return n, ms
}

// edition reports a track count within 2 of one of the group's official
// editions (the chosen edition can be a big deluxe one, or a small one).
func (g mbGroupData) edition(n int) bool {
	for _, r := range g.Releases {
		if r.Status != nil && *r.Status == "Official" && r.Tracks > 0 && abs(r.Tracks-n) <= 2 {
			return true
		}
	}
	return false
}

func year(d string) int {
	if len(d) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(d[:4])
	return y
}

// mbData reads the stored slim answer's source_data into out.
func (r *Resolver) mbData(ctx context.Context, kind, mbid string, out any) bool {
	if r.MB == nil {
		return false
	}
	b, _, ok := r.MB(ctx, kind, mbid)
	if !ok {
		return false
	}
	var env struct {
		Data json.RawMessage `json:"source_data"`
	}
	return json.Unmarshal(b, &env) == nil && len(env.Data) > 0 && json.Unmarshal(env.Data, out) == nil
}

// dzAlbum is a Deezer album as the matcher checks it.
type dzAlbum struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	UPC         string `json:"upc"`
	Cover       string `json:"cover_xl"`
	ReleaseDate string `json:"release_date"`
	Tracks      int    `json:"nb_tracks"`
	Duration    int64  `json:"duration"` // seconds
	Artist      struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"artist"`
}

// DeezerMatch answers the Deezer id matched to an artist or album ("" when
// none) and how it was found. ErrNotKnown when the slim copy lacks the
// item. Matches and misses are kept; a failed call keeps nothing.
func (r *Resolver) DeezerMatch(ctx context.Context, kind, mbid string) (id, how string, err error) {
	if st, ext, ok := r.kept(kind, mbid, "deezer"); ok && !(kind == KindArtist && r.deezerLinkOver(ctx, mbid, ext)) {
		return ext, st, nil
	}
	switch kind {
	case KindArtist:
		id, how, err = r.matchDeezerArtist(ctx, mbid)
	case KindAlbum:
		id, how, err = r.matchDeezerAlbum(ctx, mbid)
	default:
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if id == "" {
		how = ""
	}
	r.keep(kind, mbid, srctag.Tag{Source: "deezer", Ref: id}, how, id)
	if kind == KindArtist {
		r.via(mbid, "deezer", viaOf(how))
	}
	return id, how, nil
}

func (r *Resolver) matchDeezerArtist(ctx context.Context, mbid string) (string, string, error) {
	var a mbArtistData
	if !r.mbData(ctx, "artist", mbid, &a) {
		return "", "", ErrNotKnown
	}
	if id := a.find(deezerArtistLink); id != "" {
		var d deezerArtist
		ok, err := r.deezerGet(ctx, "/artist/"+url.PathEscape(id), nil, &d)
		if err != nil {
			return "", "", err
		}
		if ok && d.ID.String() == id {
			// MusicBrainz's link always wins over a searched id.
			return id, MatchLink, nil
		}
	}
	// No link: exact ids (barcodes, then Wikidata) before the name.
	if id, _, how, err := r.ExactID(ctx, "deezer", mbid, a.Name, a.find(wikidataLink), ""); err != nil || id != "" {
		return id, how, err
	}
	if a.Name == "" {
		return "", "", nil
	}
	var d struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Fans int64  `json:"nb_fan"`
		} `json:"data"`
	}
	if ok, err := r.deezerGet(ctx, "/search/artist", url.Values{"q": {a.Name}, "limit": {"10"}}, &d); err != nil || !ok {
		return "", "", err
	}
	var best, next int64 = -1, -1
	bestID := int64(0)
	for _, c := range d.Data {
		if !strings.EqualFold(strings.TrimSpace(c.Name), strings.TrimSpace(a.Name)) {
			continue
		}
		switch {
		case c.Fans > best:
			next, best, bestID = best, c.Fans, c.ID
		case c.Fans > next:
			next = c.Fans
		}
	}
	if bestID == 0 || (next >= 0 && best < 10*max(next, 1)) {
		return "", "", nil
	}
	id := strconv.FormatInt(bestID, 10)
	// A fan lead alone is no proof (an obscure "Low" or "Genesis" would
	// take the famous one): the winner must not belong to another artist
	// already, and must hold enough of the artist's album titles.
	if ok, err := r.nameMatchHolds(ctx, "deezer", "", id, mbid, a.Name); err != nil || !ok {
		return "", "", err
	}
	// Without same-name rivals (whose case nameMatchHolds weighed) the
	// winner still has to hold enough of the artist's album titles.
	if r.Homonyms != nil && len(r.Homonyms(ctx, a.Name, mbid)) > 0 {
		return id, MatchSearch, nil
	}
	if want := r.mbWant(ctx, mbid); len(want) > 0 {
		have, err := r.dzOverlap(ctx, id, want)
		if err != nil {
			return "", "", err
		}
		if have < min(weakLink, len(want)) {
			return "", "", nil
		}
	}
	return id, MatchSearch, nil
}

// mbWant answers the MusicBrainz artist's album titles (edition bases).
func (r *Resolver) mbWant(ctx context.Context, mbid string) map[string]bool {
	want := map[string]bool{}
	if r.MBTitles == nil {
		return want
	}
	for _, t := range r.MBTitles(ctx, mbid) {
		if b := EditionBase(t); b != "" {
			want[b] = true
		}
	}
	return want
}

// dzOverlap answers how many of want the Deezer artist's albums hold.
func (r *Resolver) dzOverlap(ctx context.Context, id string, want map[string]bool) (int, error) {
	var list struct {
		Data []dzAlbum `json:"data"`
	}
	ok, err := r.deezerGet(ctx, "/artist/"+url.PathEscape(id)+"/albums", url.Values{"limit": {"200"}}, &list)
	if err != nil || !ok {
		return 0, err
	}
	got := map[string]bool{}
	for _, c := range list.Data {
		if b := EditionBase(c.Title); want[b] {
			got[b] = true
		}
	}
	return len(got), nil
}

// weakLink is the title overlap under which a linked Deezer artist is
// checked against the top same-name search candidate.
const weakLink = 3

// deezerLinkOver reports a kept Deezer id that the artist's MusicBrainz
// link overrules.
func (r *Resolver) deezerLinkOver(ctx context.Context, mbid, kept string) bool {
	var a mbArtistData
	if !r.mbData(ctx, "artist", mbid, &a) {
		return false
	}
	l := a.find(deezerArtistLink)
	return l != "" && l != kept
}

func (r *Resolver) matchDeezerAlbum(ctx context.Context, mbid string) (string, string, error) {
	var g mbGroupData
	if !r.mbData(ctx, "release-group", mbid, &g) {
		return "", "", ErrNotKnown
	}
	artistName, artistMB := "", ""
	if len(g.ArtistCredit) > 0 {
		artistName, artistMB = g.ArtistCredit[0].Artist.Name, g.ArtistCredit[0].Artist.ID
	}
	// The artist's Deezer id, when it can be matched: it confirms barcode
	// hits and scopes the album search.
	artistDZ := ""
	if artistMB != "" {
		id, _, err := r.DeezerMatch(ctx, KindArtist, artistMB)
		if err != nil && err != ErrNotKnown {
			return "", "", err
		}
		artistDZ = id
	}
	sameArtist := func(a dzAlbum) bool {
		if artistDZ != "" {
			return strconv.FormatInt(a.Artist.ID, 10) == artistDZ
		}
		return artistName != "" && SameName(a.Artist.Name, artistName)
	}
	n, ms := g.tracks()
	// 1. A Deezer link on the release group.
	if id := g.find(deezerAlbumLink); id != "" {
		var a dzAlbum
		ok, err := r.deezerGet(ctx, "/album/"+url.PathEscape(id), nil, &a)
		if err != nil {
			return "", "", err
		}
		if ok && strconv.FormatInt(a.ID, 10) == id {
			return id, MatchLink, nil
		}
	}
	// 2. A release barcode, by the same artist, with about the same tracks.
	seen := map[string]bool{}
	asked := 0
	for _, rel := range g.Releases {
		if rel.Barcode == nil || asked >= 6 {
			continue
		}
		for _, bc := range upcForms(*rel.Barcode) {
			if seen[bc] {
				continue
			}
			seen[bc] = true
			asked++
			var a dzAlbum
			ok, err := r.deezerGet(ctx, "/album/upc:"+bc, nil, &a)
			if err != nil {
				return "", "", err
			}
			if ok && a.ID > 0 && sameArtist(a) && (n == 0 || abs(a.Tracks-n) <= 2) {
				return strconv.FormatInt(a.ID, 10), MatchUPC, nil
			}
			if ok && a.ID > 0 {
				break // found under this form; the other forms are the same album
			}
		}
	}
	// 3. The matched artist's album: same title, year within 1, tracks
	// within 2, length within 5%. Needs the artist pinned and MB tracks.
	if artistDZ == "" || g.Title == "" || n == 0 {
		return "", "", nil
	}
	var list struct {
		Data []dzAlbum `json:"data"`
	}
	if ok, err := r.deezerGet(ctx, "/artist/"+artistDZ+"/albums", url.Values{"limit": {"200"}}, &list); err != nil || !ok {
		return "", "", err
	}
	y := year(g.FirstRelease)
	// Same title by the pinned artist, earliest Deezer version first.
	// Another title tail ("(Live)") keeps the strict rule: year within 1
	// unless it is the only one of its name, tracks within 2, length within
	// 5%. A title that differs only by edition words ("(Remastered 2009)")
	// is the same base: its year may be later (Deezer dates reissues by the
	// reissue) and its track count is a soft signal, since editions add or
	// drop bonus tracks; the same count with another length is still
	// another album.
	var same []dzAlbum
	for _, c := range list.Data {
		if SameTitle(c.Title, g.Title) || SameBase(c.Title, g.Title) {
			same = append(same, c)
		}
	}
	sort.SliceStable(same, func(i, j int) bool { return same[i].ReleaseDate < same[j].ReleaseDate })
	// A self-titled album never gets the later-year allowance: artists
	// reuse their name for several albums (Diana Ross 1970 and 1976).
	selfTitled := SameName(g.Title, artistName)
	for _, c := range same {
		base := SameBase(c.Title, g.Title)
		cy := year(c.ReleaseDate)
		near := y == 0 || abs(cy-y) <= 1
		later := base && !selfTitled && (y == 0 || cy >= y-1)
		if !near && !later && len(same) > 1 {
			continue
		}
		var a dzAlbum
		ok, err := r.deezerGet(ctx, "/album/"+strconv.FormatInt(c.ID, 10), nil, &a)
		if err != nil {
			return "", "", err
		}
		// The artist's list holds albums it only appears on: the album's
		// own main artist must be the pinned one.
		if !ok || strconv.FormatInt(a.Artist.ID, 10) != artistDZ || a.Tracks <= 0 {
			continue
		}
		closeN := abs(a.Tracks-n) <= 2
		timed := ms > 0 && a.Duration > 0
		d := 0.0
		if timed {
			d = float64(a.Duration*1000-ms) / float64(ms)
		}
		lenOK := timed && d <= 0.05 && d >= -0.05
		if !base {
			if closeN && (lenOK || (!timed && near)) {
				return strconv.FormatInt(a.ID, 10), MatchSearch, nil
			}
			continue
		}
		// Same base. The length must not contradict the count: the same
		// count needs the same length, more tracks no less, fewer no more.
		if timed && ((a.Tracks == n && !lenOK) || (a.Tracks > n && d < -0.05) || (a.Tracks < n && d > 0.05)) {
			continue
		}
		if !near && !later && !lenOK {
			continue // the only one of its name, dated earlier: needs the length
		}
		if closeN || g.edition(a.Tracks) || (a.Tracks < n && 2*a.Tracks >= n) {
			return strconv.FormatInt(a.ID, 10), MatchSearch, nil
		}
	}
	return "", "", nil
}

// upcForms answers a barcode as Deezer may hold it: as given, then as a
// 12-digit UPC or 13-digit EAN (one leading zero dropped or added).
func upcForms(raw string) []string {
	b := strings.TrimSpace(raw)
	for _, c := range b {
		if c < '0' || c > '9' {
			return nil
		}
	}
	if len(b) < 8 {
		return nil
	}
	out := []string{b}
	switch {
	case len(b) == 13 && b[0] == '0':
		out = append(out, b[1:])
	case len(b) == 12:
		out = append(out, "0"+b)
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

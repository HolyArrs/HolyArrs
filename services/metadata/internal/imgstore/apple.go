package imgstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// AppleMusic is the Apple Music API source: the server signs its own
// developer token (ES256; kid the key id, iss the team id), at most 12
// hours long and made again before it ends. The key and the token are
// never logged and never put in an error.
type AppleMusic struct {
	TeamID, KeyID string
	Key           *ecdsa.PrivateKey
	// Base is the API (default https://api.music.apple.com); Storefront
	// the store asked first (default us).
	Base, Storefront string
	Now              func() time.Time

	mu    sync.Mutex
	token string
	until time.Time
}

// appleTokenLife is a developer token's lifetime (Apple allows up to six
// months; ours stays short and is made again an hour before it ends).
const appleTokenLife = 12 * time.Hour

// LoadAppleMusic reads the source's settings from the environment:
// APPLE_TEAM_ID, APPLE_MUSIC_KEY_ID and APPLE_MUSIC_KEY_FILE (the .p8 key,
// PKCS#8 PEM). It answers nil when any is missing: the source stays off.
func LoadAppleMusic(env func(string) string, readFile func(string) ([]byte, error)) (*AppleMusic, error) {
	team, kid, file := strings.TrimSpace(env("APPLE_TEAM_ID")), strings.TrimSpace(env("APPLE_MUSIC_KEY_ID")), strings.TrimSpace(env("APPLE_MUSIC_KEY_FILE"))
	if team == "" || kid == "" || file == "" {
		return nil, nil
	}
	raw, err := readFile(file)
	if err != nil {
		return nil, errors.New("apple music: key file unreadable")
	}
	key, err := ParseAppleKey(raw)
	if err != nil {
		return nil, err
	}
	return &AppleMusic{TeamID: team, KeyID: kid, Key: key}, nil
}

// ParseAppleKey reads a .p8 key (PKCS#8 PEM, P-256).
func ParseAppleKey(raw []byte) (*ecdsa.PrivateKey, error) {
	b, _ := pem.Decode(raw)
	if b == nil {
		return nil, errors.New("apple music: key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		return nil, errors.New("apple music: key is not PKCS#8")
	}
	ek, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("apple music: key is not ECDSA")
	}
	return ek, nil
}

func (a *AppleMusic) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Token answers the current developer token, signing a new one an hour
// before the old one ends.
func (a *AppleMusic) Token() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if a.token != "" && now.Before(a.until.Add(-time.Hour)) {
		return a.token, nil
	}
	enc := base64.RawURLEncoding
	head, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": a.KeyID, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": a.TeamID, "iat": now.Unix(), "exp": now.Add(appleTokenLife).Unix()})
	signing := enc.EncodeToString(head) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, a.Key, sum[:])
	if err != nil {
		return "", errors.New("apple music: signing failed")
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	a.token, a.until = signing+"."+enc.EncodeToString(sig), now.Add(appleTokenLife)
	return a.token, nil
}

func (a *AppleMusic) base() string {
	if a.Base != "" {
		return strings.TrimRight(a.Base, "/")
	}
	return "https://api.music.apple.com"
}

// storefronts answers the stores asked, in order: the first store (us),
// then the artist's home-country store when it is another real one.
func (a *AppleMusic) storefronts(country string) []string {
	first := a.Storefront
	if first == "" {
		first = "us"
	}
	out := []string{first}
	c := strings.ToLower(strings.TrimSpace(country))
	if len(c) != 2 || c == first || c[0] < 'a' || c[0] > 'z' || c[1] < 'a' || c[1] > 'z' {
		return out
	}
	// ISO's user-assigned codes (MusicBrainz's XW worldwide, XE Europe)
	// have no store.
	if c == "aa" || c == "zz" || c[0] == 'x' || c[0] == 'q' && c[1] >= 'm' {
		return out
	}
	return append(out, c)
}

// appleArtwork is an artwork answer: a {w}x{h} template and its full size.
type appleArtwork struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// sized fills the template at w by h, never past the original size.
func (w appleArtwork) sized(width, height int) string {
	if w.URL == "" {
		return ""
	}
	if w.Width > 0 && width > w.Width {
		height = height * w.Width / width
		width = w.Width
	}
	return strings.NewReplacer("{w}", strconv.Itoa(width), "{h}", strconv.Itoa(height), "{f}", "jpg").Replace(w.URL)
}

// square is the artwork as a cover or photo candidate: 500 first, the big
// size 1200 (or the original when smaller).
func (w appleArtwork) square() Candidate {
	c := Candidate{Provider: "applemusic", URL: w.sized(500, 500)}
	if w.Width == 0 || w.Width > 500 {
		c.Big = w.sized(1200, 1200)
	}
	return c
}

// appleGet calls the API through the source's pace and limits. ok is false
// when the source is off or has no such thing.
func (r *Resolver) appleGet(ctx context.Context, path string, q url.Values, out any) (bool, error) {
	a := r.AppleMusic
	if a == nil {
		return false, nil
	}
	tok, err := a.Token()
	if err != nil {
		return false, err
	}
	u := a.base() + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req := newReq(u)
	req.Header.Set("Authorization", "Bearer "+tok)
	b, err := r.call(ctx, "applemusic", req)
	if err != nil {
		return false, errors.New(Redact(err.Error(), tok))
	}
	if b == nil {
		return false, nil
	}
	return true, json.Unmarshal(b, out)
}

// ApplePhoto reports Apple artwork that is a real artist photo. Apple shows
// an album cover for an artist without one: that is a miss.
func ApplePhoto(u string) bool {
	return strings.Contains(u, "/AMCArtistImages") || strings.Contains(u, "/Features")
}

var appleArtistID = regexp.MustCompile(`^https?://(?:music|itunes)\.apple\.com/.*artist/(?:[^/]+/)?(?:id)?(\d+)`)

type appleArtist struct {
	ID         string `json:"id"`
	Attributes struct {
		Name    string       `json:"name"`
		Artwork appleArtwork `json:"artwork"`
	} `json:"attributes"`
}

// appleFound is an artist found in a store, and how (a Via name).
type appleFound struct {
	Store  string
	Artist appleArtist
	Via    string
}

// appleArtistOf finds the artist in Apple Music: by the kept match, else
// by the Apple id its MusicBrainz links name, else one search by its stored
// name taking only the same name; the US store first, then its home store.
// nil when none. A match or a miss is kept (an unknown subject's is not).
func (r *Resolver) appleArtistOf(ctx context.Context, mbid string) (*appleFound, error) {
	if r.AppleMusic == nil {
		return nil, nil
	}
	v, err := r.once("apple-artist:"+mbid, func() (any, error) {
		if st, id, ok := r.kept(KindArtist, mbid, "applemusic"); ok && !r.linkedOver(ctx, mbid, "applemusic", id) {
			if id == "" {
				return (*appleFound)(nil), nil
			}
			var d struct {
				Data []appleArtist `json:"data"`
			}
			ok, err := r.appleGet(ctx, "/v1/catalog/"+url.PathEscape(st)+"/artists/"+url.PathEscape(id), nil, &d)
			if err != nil {
				return nil, err
			}
			if ok && len(d.Data) > 0 {
				return &appleFound{st, d.Data[0], ""}, nil
			}
		}
		a, err := r.appleArtistFind(ctx, mbid)
		if err != nil {
			return nil, err
		}
		if a != nil {
			r.keep(KindArtist, mbid, srctag.Tag{Source: "applemusic"}, a.Store, a.Artist.ID)
			r.via(mbid, "applemusic", a.Via)
		} else if r.info(ctx, KindArtist, mbid).Known {
			r.keep(KindArtist, mbid, srctag.Tag{Source: "applemusic"}, "", "")
		}
		return a, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*appleFound), nil
}

// appleArtistChecked is appleArtistOf without an Apple artist that another
// MusicBrainz artist is matched to as well, unless MusicBrainz itself links
// this artist to it. Matches by name alone put a famous artist's pictures
// and similar artists on every other artist of the same name.
func (r *Resolver) appleArtistChecked(ctx context.Context, mbid string) (*appleFound, error) {
	a, err := r.appleArtistOf(ctx, mbid)
	if err != nil || a == nil || r.Shared == nil || !r.Shared(KindArtist, "applemusic", a.Artist.ID, mbid) {
		return a, err
	}
	for _, l := range r.info(ctx, KindArtist, mbid).Links {
		if m := appleArtistID.FindStringSubmatch(l); m != nil && m[1] == a.Artist.ID {
			return a, nil
		}
	}
	return nil, nil
}

// AppleNameOnly reports whether the artist's Apple match can only have come
// by name: no MusicBrainz link names that Apple id.
func AppleNameOnly(links []string, appleID string) bool {
	for _, l := range links {
		if m := appleArtistID.FindStringSubmatch(l); m != nil && m[1] == appleID {
			return false
		}
	}
	return true
}

// appleArtistFind is appleArtistOf's lookup by links, then by name.
func (r *Resolver) appleArtistFind(ctx context.Context, mbid string) (*appleFound, error) {
	s := r.info(ctx, KindArtist, mbid)
	var ids []string
	for _, l := range s.Links {
		if m := appleArtistID.FindStringSubmatch(l); m != nil {
			ids = append(ids, m[1])
		}
	}
	via := ViaLink
	if len(ids) == 0 {
		// No link: exact ids (barcodes, then Wikidata) before the name.
		id, st, how, err := r.ExactID(ctx, "applemusic", mbid, s.Name, LinkedQID(s.Links), s.Country)
		if err != nil {
			return nil, err
		}
		if id != "" {
			if st != "" {
				var d struct {
					Data []appleArtist `json:"data"`
				}
				ok, err := r.appleGet(ctx, "/v1/catalog/"+url.PathEscape(st)+"/artists/"+url.PathEscape(id), nil, &d)
				if err != nil {
					return nil, err
				}
				if ok && len(d.Data) > 0 {
					return &appleFound{st, d.Data[0], viaOf(how)}, nil
				}
			}
			ids, via = []string{id}, viaOf(how)
		}
	}
	for _, sf := range r.AppleMusic.storefronts(s.Country) {
		if len(ids) > 0 {
			var d struct {
				Data []appleArtist `json:"data"`
			}
			ok, err := r.appleGet(ctx, "/v1/catalog/"+url.PathEscape(sf)+"/artists/"+url.PathEscape(ids[0]), nil, &d)
			if err != nil {
				return nil, err
			}
			if ok && len(d.Data) > 0 {
				return &appleFound{sf, d.Data[0], via}, nil
			}
			continue
		}
		if s.Name == "" {
			break
		}
		var d struct {
			Results struct {
				Artists struct {
					Data []appleArtist `json:"data"`
				} `json:"artists"`
			} `json:"results"`
		}
		ok, err := r.appleGet(ctx, "/v1/catalog/"+url.PathEscape(sf)+"/search", url.Values{"types": {"artists"}, "term": {s.Name}, "limit": {"5"}}, &d)
		if err != nil {
			return nil, err
		}
		for _, x := range d.Results.Artists.Data {
			if ok && SameName(x.Attributes.Name, s.Name) {
				// A name alone is no proof while another MusicBrainz
				// artist carries it: the Apple artist's albums must then
				// fit this artist, clearly better than any of the others.
				if r.Homonyms != nil {
					if rivals := r.Homonyms(ctx, s.Name, mbid); len(rivals) > 0 {
						ev, err := r.AppleEvidenceOf(ctx, sf, x.ID, mbid, rivals)
						if err != nil {
							return nil, err
						}
						if !ev.OK {
							return nil, nil
						}
					}
				}
				return &appleFound{sf, x, ViaName}, nil
			}
		}
	}
	return nil, nil
}

// AppleEvidence is how well an Apple artist's albums fit a MusicBrainz
// artist against the same-name artists of the slim copy: Have is how many
// of the artist's release-group titles the Apple albums hold, Rival the
// most any other one's hold (RivalID: whose). OK when Have is at least 2
// (or the one title of a one-album side, MusicBrainz's or Apple's) and
// more than twice Rival.
type AppleEvidence struct {
	Have, Rival int
	RivalID     string
	Apple       []string // the source's album titles (comparison keys)
	// Single is the one title in common when the match rests on it, and
	// SingleHolders how many artists in the slim copy hold that title.
	Single        string
	SingleHolders int
	OK            bool
}

// rareTitleArtists is how many artists in the slim copy may hold a title
// for it to count alone: "Sukierae" (1) or "Chella Magale" (3) do, "What It
// Do" (5), "PARADISO" (62) or "Fall" (247) do not.
const rareTitleArtists = 3

// rareKeyLen is the shortest comparison key a lone title may have: "Ego",
// "Safar" or "10 AM" are too common a word to tell artists apart.
const rareKeyLen = 6

// AppleEvidenceOf is EvidenceOf for an Apple Music artist.
func (r *Resolver) AppleEvidenceOf(ctx context.Context, store, appleID, mbid string, rivals []string) (AppleEvidence, error) {
	return r.EvidenceOf(ctx, "applemusic", store, appleID, mbid, rivals)
}

// EvidenceOf asks the source for the artist's albums (one call: Apple
// Music, Deezer or Spotify) and weighs them against the artist's and each
// rival's release-group titles. store is Apple's storefront.
func (r *Resolver) EvidenceOf(ctx context.Context, source, store, id, mbid string, rivals []string) (AppleEvidence, error) {
	var ev AppleEvidence
	// A self-titled album is no evidence: every artist of the name may
	// have one.
	var self string
	if k := evidenceKeys(r.info(ctx, KindArtist, mbid).Name); len(k) > 0 {
		self = k[0]
	}
	want, raw := r.evidenceWant(ctx, mbid, self)
	if len(want) == 0 {
		return ev, nil
	}
	titles, ok, err := r.albumTitles(ctx, source, store, id)
	if err != nil || !ok {
		return ev, err
	}
	have := map[string]bool{}
	for _, t := range titles {
		for _, k := range evidenceKeys(t) {
			if !have[k] {
				have[k] = true
				ev.Apple = append(ev.Apple, k)
			}
		}
	}
	// A release group counts once, however many of its keys match.
	overlap := func(want [][]string) int {
		n := 0
		for _, keys := range want {
			for _, k := range keys {
				if have[k] {
					n++
					break
				}
			}
		}
		return n
	}
	ev.Have = overlap(want)
	for _, rv := range rivals {
		rw, _ := r.evidenceWant(ctx, rv, self)
		if n := overlap(rw); n > ev.Rival {
			ev.Rival, ev.RivalID = n, rv
		}
	}
	ev.OK = ev.Have >= min(2, len(want), len(titles)) && ev.Have > 2*ev.Rival
	if ev.OK && ev.Have == 1 {
		// One title alone: only a rare one tells the artists apart.
		for i, keys := range want {
			if slices.ContainsFunc(keys, func(k string) bool { return have[k] }) {
				ev.Single = raw[i]
				if len(keys[0]) < rareKeyLen || r.TitleArtists == nil {
					ev.OK = false
					break
				}
				ev.SingleHolders = r.TitleArtists(ctx, raw[i])
				ev.OK = ev.SingleHolders <= rareTitleArtists
				break
			}
		}
	}
	return ev, nil
}

// albumTitles answers an artist's album titles at a source (one call).
func (r *Resolver) albumTitles(ctx context.Context, source, store, id string) ([]string, bool, error) {
	var out []string
	switch source {
	case "applemusic":
		var d struct {
			Data []struct {
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		}
		ok, err := r.appleGet(ctx, "/v1/catalog/"+url.PathEscape(store)+"/artists/"+url.PathEscape(id)+"/albums", url.Values{"limit": {"100"}}, &d)
		if err != nil || !ok {
			return nil, false, err
		}
		for _, a := range d.Data {
			out = append(out, a.Attributes.Name)
		}
	case "deezer":
		var d struct {
			Data []dzAlbum `json:"data"`
		}
		ok, err := r.deezerGet(ctx, "/artist/"+url.PathEscape(id)+"/albums", url.Values{"limit": {"200"}}, &d)
		if err != nil || !ok {
			return nil, false, err
		}
		for _, a := range d.Data {
			out = append(out, a.Title)
		}
	case "spotify":
		var d struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		ok, err := r.spotifyGet(ctx, "/v1/artists/"+url.PathEscape(id)+"/albums?"+url.Values{"include_groups": {"album,single"}, "limit": {"50"}}.Encode(), &d)
		if err != nil || !ok {
			return nil, false, err
		}
		for _, a := range d.Items {
			out = append(out, a.Name)
		}
	default:
		return nil, false, nil
	}
	return out, true, nil
}

// LinkedID answers the artist's id at a source ("applemusic", "deezer",
// "spotify") that its MusicBrainz links name ("" when none). A link always
// wins over a searched id.
func LinkedID(links []string, source string) string {
	re := map[string]*regexp.Regexp{"applemusic": appleArtistID, "deezer": deezerArtistLink, "spotify": spotifyLink}[source]
	if re == nil {
		return ""
	}
	for _, l := range links {
		if m := re.FindStringSubmatch(l); m != nil && ValidSourceID(source, m[1]) {
			return m[1]
		}
	}
	return ""
}

// linkedOver reports a kept id that MusicBrainz's link for the source
// overrules: the link names another id.
func (r *Resolver) linkedOver(ctx context.Context, mbid, source, kept string) bool {
	l := LinkedID(r.info(ctx, KindArtist, mbid).Links, source)
	return l != "" && l != kept
}

// evidenceWant answers an artist's release groups as comparison keys, one
// distinct group title each, leaving out those titled self, and each
// group's title as MusicBrainz writes it.
func (r *Resolver) evidenceWant(ctx context.Context, mbid, self string) ([][]string, []string) {
	if r.MBTitles == nil {
		return nil, nil
	}
	var out [][]string
	var raw []string
	seen := map[string]bool{}
	for _, t := range r.MBTitles(ctx, mbid) {
		keys := slices.DeleteFunc(evidenceKeys(t), func(k string) bool { return k == self })
		if len(keys) == 0 || seen[keys[0]] {
			continue
		}
		seen[keys[0]] = true
		out = append(out, keys)
		raw = append(raw, t)
	}
	return out, raw
}

// appleTag is the kind Apple adds to a single's or EP's name.
var appleTag = regexp.MustCompile(`(?i)\s*-\s*(single|ep)\s*$`)

// guestTag is a bracketed guest or source note ("(feat. X)", "[with X]",
// "(From the Netflix Series …)"); guestTail an unbracketed " feat. X".
var (
	guestTag  = regexp.MustCompile(`(?i)\s*[(\[]\s*(feat\.?|ft\.?|featuring|with|from)\s[^()\[\]]*[)\]]`)
	guestTail = regexp.MustCompile(`(?i)\s+(feat\.?|ft\.?|featuring)\s.*$`)
)

// evidenceKeys answers a title's comparison keys: the whole title and each
// part of a double title ("A/B"), as edition bases without Apple's
// " - Single" or " - EP", without guest and source notes, and without
// spaces (so "Heavy-Metal Jazz" and "HeavyMetal Jazz" meet).
func evidenceKeys(t string) []string {
	t = appleTag.ReplaceAllString(t, "")
	t = guestTail.ReplaceAllString(guestTag.ReplaceAllString(t, ""), "")
	t = appleTag.ReplaceAllString(t, "")
	var out []string
	add := func(s string) {
		if k := strings.ReplaceAll(EditionBase(s), " ", ""); k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	add(t)
	if parts := strings.Split(t, "/"); len(parts) > 1 {
		for _, p := range parts {
			add(p)
		}
	}
	return out
}

// applePhoto is the artist's photo when Apple has a real one.
func (r *Resolver) applePhoto(ctx context.Context, mbid string) ([]Candidate, error) {
	a, err := r.appleArtistChecked(ctx, mbid)
	if err != nil || a == nil || !ApplePhoto(a.Artist.Attributes.Artwork.URL) {
		return nil, err
	}
	return []Candidate{a.Artist.Attributes.Artwork.square()}, nil
}

// appleVideoStills are stills of the artist's music videos (16:9, 1920 by
// 1080), the artist's own videos first, then those it is credited on.
func (r *Resolver) appleVideoStills(ctx context.Context, mbid string) ([]Candidate, error) {
	a, err := r.appleArtistChecked(ctx, mbid)
	if err != nil || a == nil {
		return nil, err
	}
	var d struct {
		Data []struct {
			Attributes struct {
				ArtistName string       `json:"artistName"`
				Artwork    appleArtwork `json:"artwork"`
			} `json:"attributes"`
		} `json:"data"`
	}
	ok, err := r.appleGet(ctx, "/v1/catalog/"+url.PathEscape(a.Store)+"/artists/"+url.PathEscape(a.Artist.ID)+"/music-videos", url.Values{"limit": {"10"}}, &d)
	if err != nil || !ok {
		return nil, err
	}
	var main, other []Candidate
	for _, v := range d.Data {
		w := v.Attributes.Artwork
		if w.URL == "" || w.Width > 0 && w.Height > 0 && w.Width*9 < w.Height*15 {
			continue // not wide
		}
		c := Candidate{Provider: "applemusic", URL: w.sized(1920, 1080)}
		if SameName(v.Attributes.ArtistName, a.Artist.Attributes.Name) {
			main = append(main, c)
		} else {
			other = append(other, c)
		}
	}
	return append(main, other...), nil
}

// appleAlbum finds an album's cover: the album appleAlbumMatch finds.
func (r *Resolver) appleAlbum(ctx context.Context, mbid string) ([]Candidate, error) {
	m, err := r.appleAlbumMatch(ctx, mbid)
	if err != nil || m == nil {
		return nil, err
	}
	return []Candidate{m.art.square()}, nil
}

// appleAlbumHit is one album appleAlbumMatch found.
type appleAlbumHit struct {
	store, id string
	art       appleArtwork
}

// appleAlbumMatch finds an album by catalog search, the US store first,
// then the artist's home store, taking only a confident match: the same
// title (editions in brackets aside), the artist's whole name among the
// album's artists, not a compilation, artwork, and the year within one
// when both are known. The match, or a miss when every store answered, is
// kept for the album details.
func (r *Resolver) appleAlbumMatch(ctx context.Context, mbid string) (*appleAlbumHit, error) {
	if r.AppleMusic == nil {
		return nil, nil
	}
	s := r.info(ctx, KindAlbum, mbid)
	if s.Name == "" || s.Title == "" {
		return nil, nil
	}
	for _, sf := range r.AppleMusic.storefronts(s.Country) {
		var d struct {
			Results struct {
				Albums struct {
					Data []struct {
						ID         string `json:"id"`
						Attributes struct {
							Name        string       `json:"name"`
							ArtistName  string       `json:"artistName"`
							ReleaseDate string       `json:"releaseDate"`
							IsCompil    bool         `json:"isCompilation"`
							Artwork     appleArtwork `json:"artwork"`
						} `json:"attributes"`
					} `json:"data"`
				} `json:"albums"`
			} `json:"results"`
		}
		ok, err := r.appleGet(ctx, "/v1/catalog/"+url.PathEscape(sf)+"/search", url.Values{"types": {"albums"}, "term": {s.Name + " " + s.Title}, "limit": {"10"}}, &d)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		for _, x := range d.Results.Albums.Data {
			at := x.Attributes
			if at.IsCompil || at.Artwork.URL == "" || !amongArtists(at.ArtistName, s.Name) || !SameTitle(at.Name, s.Title) {
				continue
			}
			if y, _ := strconv.Atoi(firstN(at.ReleaseDate, 4)); s.Year > 0 && y > 0 && (y-s.Year > 1 || s.Year-y > 1) {
				continue
			}
			r.keep(KindAlbum, mbid, srctag.Tag{Source: "applemusic"}, sf, x.ID)
			return &appleAlbumHit{sf, x.ID, at.Artwork}, nil
		}
	}
	r.keep(KindAlbum, mbid, srctag.Tag{Source: "applemusic"}, "", "")
	return nil, nil
}

func firstN(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// AppleBench answers whether an album's cover and an artist's photo are
// found, for the apple-bench command (hit rates only, no addresses kept).
func (r *Resolver) AppleBench(ctx context.Context, kind, mbid string) (bool, error) {
	var c []Candidate
	var err error
	if kind == KindAlbum {
		c, err = r.appleAlbum(ctx, mbid)
	} else {
		c, err = r.applePhoto(ctx, mbid)
	}
	return len(c) > 0, err
}

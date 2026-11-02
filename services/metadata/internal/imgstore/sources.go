package imgstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// caaAlbum is the Cover Art Archive's front of a release group (by ID, no
// names needed): 500 first, 1200 as the big size.
func (r *Resolver) caaAlbum(mbid string) []Candidate {
	b := base(r.CAA, "https://coverartarchive.org") + "/release-group/" + mbid
	return []Candidate{{Provider: "caa", URL: b + "/front-500", Big: b + "/front-1200"}}
}

// deezerBlank is the picture hash Deezer gives an artist without a photo
// (the MD5 of an empty file); an empty hash ("/artist//") means the same.
const deezerBlank = "d41d8cd98f00b204e9800998ecf8427e"

// deezerSized matches a Deezer picture link's size part.
var deezerSized = regexp.MustCompile(`^(https?://[a-z0-9-]+\.dzcdn\.net/images/(?:cover|artist)/)([0-9a-f]*)/\d+x\d+(-.*)?$`)

// deezerPicture is a Deezer picture link as a candidate at 500 and 1000
// (its largest); none for Deezer's blank stand-in.
func deezerPicture(u string) []Candidate {
	m := deezerSized.FindStringSubmatch(u)
	if m == nil || m[2] == "" || m[2] == deezerBlank {
		return nil
	}
	at := func(n string) string { return m[1] + m[2] + "/" + n + "x" + n + m[3] }
	return []Candidate{{Provider: "deezer", URL: at("500"), Big: at("1000")}}
}

func (r *Resolver) deezerGet(ctx context.Context, path string, q url.Values, out any) (bool, error) {
	u := base(r.Deezer, "https://api.deezer.com") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	b, err := r.call(ctx, "deezer", newReq(u))
	if err != nil || b == nil {
		return false, err
	}
	// Deezer answers its errors with 200 and an error object (code 800:
	// no such item; 4: quota, to try again later).
	var e struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error != nil {
		if e.Error.Code == 800 {
			return false, nil
		}
		r.Limits.Backoff("deezer", 0)
		return false, fmt.Errorf("deezer: error %d", e.Error.Code)
	}
	return true, json.Unmarshal(b, out)
}

// deezerAlbum finds an album's cover by one search on the artist's name
// and the title, taking only the same title by the same artist.
func (r *Resolver) deezerAlbum(ctx context.Context, mbid string) ([]Candidate, error) {
	if r.MB != nil {
		id, _, err := r.DeezerMatch(ctx, KindAlbum, mbid)
		if err == ErrNotKnown {
			return nil, nil
		}
		if err != nil || id == "" {
			return nil, err
		}
		var a dzAlbum
		if ok, err := r.deezerGet(ctx, "/album/"+url.PathEscape(id), nil, &a); err != nil || !ok {
			return nil, err
		}
		return deezerPicture(a.Cover), nil
	}
	s := r.info(ctx, KindAlbum, mbid)
	if s.Name == "" || s.Title == "" {
		return nil, nil
	}
	var d struct {
		Data []struct {
			Title  string `json:"title"`
			Cover  string `json:"cover_xl"`
			Artist struct {
				Name string `json:"name"`
			} `json:"artist"`
		} `json:"data"`
	}
	q := url.Values{"q": {`artist:"` + s.Name + `" album:"` + s.Title + `"`}, "limit": {"10"}}
	if ok, err := r.deezerGet(ctx, "/search/album", q, &d); err != nil || !ok {
		return nil, err
	}
	for _, a := range d.Data {
		if SameName(a.Artist.Name, s.Name) && SameTitle(a.Title, s.Title) {
			if c := deezerPicture(a.Cover); c != nil {
				return c, nil
			}
		}
	}
	return nil, nil
}

var deezerArtistLink = regexp.MustCompile(`^https?://(?:www\.)?deezer\.com/(?:[a-z]{2}/)?artist/([0-9]+)`)

// deezerArtist is a Deezer artist as the photo and the top songs read it.
type deezerArtist struct {
	ID      json.Number `json:"id"`
	Name    string      `json:"name"`
	Picture string      `json:"picture_xl"`
}

// deezerPhoto is the artist's Deezer picture.
func (r *Resolver) deezerPhoto(ctx context.Context, mbid string) ([]Candidate, error) {
	a, err := r.deezerArtistOf(ctx, mbid)
	if err != nil || a == nil {
		return nil, err
	}
	return deezerPicture(a.Picture), nil
}

// deezerArtistOf finds the artist on Deezer: by the Deezer id its
// MusicBrainz links name, else one search by its stored name taking only
// the same name. nil when none.
func (r *Resolver) deezerArtistOf(ctx context.Context, mbid string) (*deezerArtist, error) {
	v, err := r.once("deezer-artist:"+mbid, func() (any, error) {
		if r.MB != nil {
			id, _, err := r.DeezerMatch(ctx, KindArtist, mbid)
			if err == ErrNotKnown {
				err = nil
			}
			if err != nil || id == "" {
				return (*deezerArtist)(nil), err
			}
			var a deezerArtist
			if ok, err := r.deezerGet(ctx, "/artist/"+url.PathEscape(id), nil, &a); err != nil || !ok {
				return (*deezerArtist)(nil), err
			}
			return &a, nil
		}
		s := r.info(ctx, KindArtist, mbid)
		for _, l := range s.Links {
			if m := deezerArtistLink.FindStringSubmatch(l); m != nil {
				var a deezerArtist
				if ok, err := r.deezerGet(ctx, "/artist/"+m[1], nil, &a); err != nil || !ok {
					return (*deezerArtist)(nil), err
				}
				return &a, nil
			}
		}
		if s.Name == "" {
			return (*deezerArtist)(nil), nil
		}
		var d struct {
			Data []deezerArtist `json:"data"`
		}
		if ok, err := r.deezerGet(ctx, "/search/artist", url.Values{"q": {s.Name}, "limit": {"5"}}, &d); err != nil || !ok {
			return (*deezerArtist)(nil), err
		}
		for _, a := range d.Data {
			if SameName(a.Name, s.Name) {
				return &a, nil
			}
		}
		return (*deezerArtist)(nil), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*deezerArtist), nil
}

var spotifyLink = regexp.MustCompile(`^https?://open\.spotify\.com/(?:intl-[a-z]+/)?artist/([A-Za-z0-9]+)`)

type spotifyImage struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// spotifyCandidate is the largest image (640) as the one size: Spotify's
// copies are kept short (re-checked every 30 days).
func spotifyCandidate(imgs []spotifyImage) []Candidate {
	best := spotifyImage{}
	for _, im := range imgs {
		if im.Width >= best.Width {
			best = im
		}
	}
	if best.URL == "" {
		return nil
	}
	return []Candidate{{Provider: "spotify", URL: best.URL, Recheck: ShortRecheck}}
}

type spotifyArtist struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Images []spotifyImage `json:"images"`
}

// spotifyPhoto is the artist's Spotify picture.
func (r *Resolver) spotifyPhoto(ctx context.Context, mbid string) ([]Candidate, error) {
	a, err := r.spotifyArtistOf(ctx, mbid)
	if err != nil || a == nil {
		return nil, err
	}
	return spotifyCandidate(a.Images), nil
}

// spotifyGet reads one Spotify Web API answer with a client-credentials
// token (dropped on a 401); ok is false without the keys or on a 404.
func (r *Resolver) spotifyGet(ctx context.Context, path string, out any) (bool, error) {
	if r.Keys.SpotifyID == "" || r.Keys.SpotifySecret == "" {
		return false, nil
	}
	tok, err := r.bearer(ctx, "spotify", base(r.SpotifyAuth, "https://accounts.spotify.com/api/token"), r.Keys.SpotifyID, r.Keys.SpotifySecret)
	if err != nil {
		return false, err
	}
	req := newReq(base(r.SpotifyAPI, "https://api.spotify.com") + path)
	req.Header.Set("Authorization", "Bearer "+tok)
	b, err := r.call(ctx, "spotify", req)
	var se *statusError
	if errors.As(err, &se) && se.code == 401 {
		r.dropToken("spotify")
	}
	if err != nil || b == nil {
		return false, err
	}
	return true, json.Unmarshal(b, out)
}

// spotifyArtistOf finds the artist on Spotify: by the kept match, else by
// the linked Spotify id, else one search by its stored name taking only
// the same name. nil when none; a match (marked "link" or "search": how
// it was found) or a miss is kept.
func (r *Resolver) spotifyArtistOf(ctx context.Context, mbid string) (*spotifyArtist, error) {
	if r.Keys.SpotifyID == "" || r.Keys.SpotifySecret == "" {
		return nil, nil
	}
	v, err := r.once("spotify-artist:"+mbid, func() (any, error) {
		id, how := "", ""
		s := r.info(ctx, KindArtist, mbid)
		link := LinkedID(s.Links, "spotify")
		if st, ext, ok := r.kept(KindArtist, mbid, "spotify"); ok && (link == "" || link == ext) {
			if ext == "" {
				return (*spotifyArtist)(nil), nil
			}
			id, how = ext, st
		}
		if id == "" && link != "" {
			id, how = link, "link"
		}
		if id == "" {
			// No link: exact ids (barcodes, then Wikidata) before the name.
			x, _, h, err := r.ExactID(ctx, "spotify", mbid, s.Name, LinkedQID(s.Links), s.Country)
			if err != nil {
				return (*spotifyArtist)(nil), err
			}
			id, how = x, h
		}
		if id != "" {
			var a spotifyArtist
			if ok, err := r.spotifyGet(ctx, "/v1/artists/"+url.PathEscape(id), &a); err != nil || !ok {
				return (*spotifyArtist)(nil), err
			}
			r.keep(KindArtist, mbid, srctag.Tag{Source: "spotify"}, how, a.ID)
			r.via(mbid, "spotify", viaOf(how))
			return &a, nil
		}
		if s.Name == "" {
			return (*spotifyArtist)(nil), nil
		}
		var d struct {
			Artists struct {
				Items []spotifyArtist `json:"items"`
			} `json:"artists"`
		}
		if ok, err := r.spotifyGet(ctx, "/v1/search?"+url.Values{"type": {"artist"}, "q": {s.Name}, "limit": {"5"}}.Encode(), &d); err != nil || !ok {
			return (*spotifyArtist)(nil), err
		}
		for _, a := range d.Artists.Items {
			if !SameName(a.Name, s.Name) {
				continue
			}
			// A name alone is no proof: the id must be no other artist's,
			// and with same-name artists in the slim copy the albums must
			// fit this one clearly better.
			ok, err := r.nameMatchHolds(ctx, "spotify", "", a.ID, mbid, s.Name)
			if err != nil {
				return (*spotifyArtist)(nil), err
			}
			if !ok {
				break
			}
			r.keep(KindArtist, mbid, srctag.Tag{Source: "spotify"}, "search", a.ID)
			r.via(mbid, "spotify", ViaName)
			return &a, nil
		}
		r.keep(KindArtist, mbid, srctag.Tag{Source: "spotify"}, "", "")
		return (*spotifyArtist)(nil), nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*spotifyArtist), nil
}

// bearer answers a cached client-credentials token.
func (r *Resolver) bearer(ctx context.Context, name, authURL, id, secret string) (string, error) {
	r.tokMu.Lock()
	defer r.tokMu.Unlock()
	if t, ok := r.tokens[name]; ok && time.Now().Before(t.until) {
		return t.value, nil
	}
	req, err := http.NewRequest(http.MethodPost, authURL, strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", fmt.Errorf("%s: token address", name)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(id, secret)
	b, err := r.call(ctx, name, req)
	if err != nil {
		return "", fmt.Errorf("%w (token)", err)
	}
	var d struct {
		Token   string `json:"access_token"`
		Expires int    `json:"expires_in"`
	}
	if json.Unmarshal(b, &d) != nil || d.Token == "" {
		return "", fmt.Errorf("%s: no token", name)
	}
	if r.tokens == nil {
		r.tokens = map[string]token{}
	}
	r.tokens[name] = token{d.Token, time.Now().Add(time.Duration(max(d.Expires-60, 60)) * time.Second)}
	return d.Token, nil
}

func (r *Resolver) dropToken(name string) {
	r.tokMu.Lock()
	delete(r.tokens, name)
	r.tokMu.Unlock()
}

// fanart reads the art the fanart.tv source answers for an artist: the
// backgrounds (English or no language, newest first) as wide candidates,
// the logo (HD first) as the logo candidate.
func (r *Resolver) fanart(ctx context.Context, mbid, kind string) ([]Candidate, error) {
	if r.Fanart == nil {
		return nil, nil
	}
	v, err := r.once("fanart:"+mbid, func() (any, error) {
		b, err := r.Fanart(ctx, mbid)
		if errors.Is(err, ErrNoPicture) {
			return fanartArt{}, nil
		}
		if err != nil {
			return nil, err
		}
		var d fanartArt
		_ = json.Unmarshal(b, &d)
		return d, nil
	})
	if err != nil {
		return nil, err
	}
	d := v.(fanartArt)
	urls := d.Backgrounds
	if kind == KindLogo {
		urls = []string{d.Logo}
	}
	var out []Candidate
	for _, u := range urls {
		if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
			out = append(out, Candidate{Provider: "fanart", URL: u})
		}
	}
	return out, nil
}

type fanartArt struct {
	Logo        string   `json:"logo"`
	Backgrounds []string `json:"backgrounds"`
}

// youtubeStills are the largest stills (1280 by 720) of the artist's
// checked music videos, stored through our server so installs never
// contact YouTube for them.
func (r *Resolver) youtubeStills(ctx context.Context, mbid string) []Candidate {
	if r.Videos == nil {
		return nil
	}
	var out []Candidate
	for i, id := range r.Videos(ctx, mbid) {
		if i == 3 {
			break
		}
		out = append(out, Candidate{Provider: "youtube", URL: base(r.YTImg, "https://i.ytimg.com") + "/vi/" + url.PathEscape(id) + "/maxresdefault.jpg", Recheck: ShortRecheck})
	}
	return out
}

// nameMatchHolds checks a match found by name alone: an id another artist
// already holds is refused, and when other artists in the slim copy carry
// the name the source's albums must fit this one (EvidenceOf).
func (r *Resolver) nameMatchHolds(ctx context.Context, source, store, id, mbid, name string) (bool, error) {
	if r.Shared != nil && r.Shared(KindArtist, source, id, mbid) {
		return false, nil
	}
	if r.Homonyms == nil {
		return true, nil
	}
	rivals := r.Homonyms(ctx, name, mbid)
	if len(rivals) == 0 {
		return true, nil
	}
	ev, err := r.EvidenceOf(ctx, source, store, id, mbid, rivals)
	return ev.OK, err
}

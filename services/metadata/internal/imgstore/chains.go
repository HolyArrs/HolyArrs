package imgstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// Subject is what the chains know about an artist or album, read from the
// stored MusicBrainz answers and the catalog. Known is false when no
// stored answer was found: only the steps that go by MusicBrainz ID run,
// and the picture is looked at again soon.
type Subject struct {
	Known bool
	// Name is the artist's name (for an album, its first credited artist).
	Name  string
	Title string // album title
	Year  int    // album year (0 when unknown)
	// Country is the artist's home country (ISO 3166-1), for Apple's
	// second store.
	Country string
	// Links are the artist's MusicBrainz url-relations.
	Links []string
}

// Candidate is one picture a source offers.
type Candidate struct {
	Provider string
	// URL is the picture at the size stored first (500 for covers and
	// photos; the one picture for wide backgrounds and logos).
	URL string
	// Big is the same picture at its big size ("" when the source has
	// nothing bigger than URL).
	Big string
	// License and Author are recorded for Wikimedia pictures.
	License, Author string
	// Recheck is when the picture is looked at again (0: DefaultRecheck).
	Recheck time.Duration
	// Upgrade names the source that should replace this picture on its
	// recheck ("applemusic" for an overflow picture; "" for none).
	Upgrade string
}

// Step is one source in a chain.
type Step struct {
	Name string
	Get  func(ctx context.Context) ([]Candidate, error)
}

// Re-check times.
const (
	DefaultRecheck = 90 * 24 * time.Hour
	ShortRecheck   = 30 * 24 * time.Hour // Spotify, YouTube and Wikimedia
	MissRecheck    = 7 * 24 * time.Hour  // a real "not found"
	UnknownRecheck = 24 * time.Hour      // picked before the stored answer was there
	// UpgradeRecheck: an overflow picture is looked at again soon, so
	// Apple's version replaces it once Apple's queue is short.
	UpgradeRecheck = 3 * 24 * time.Hour
)

// DefaultAppleOverflow is the Apple queue wait past which new picture jobs
// skip Apple (25 jobs at Apple's 200 ms pace).
const DefaultAppleOverflow = 5 * time.Second

// appleBusy reports Apple is off, paused or its queue waits past the
// overflow threshold.
func (r *Resolver) appleBusy() bool {
	if r.AppleMusic == nil || !r.on("applemusic") {
		return true
	}
	t := r.AppleOverflow
	if t <= 0 {
		t = DefaultAppleOverflow
	}
	return r.Limits.Busy("applemusic", t)
}

// caaAsk is the Cover Art Archive's answer, asked beside Deezer.
type caaAsk struct {
	once sync.Once
	done chan struct{}
	c    []Candidate
}

// start asks the archive whether the release group has a front cover. A
// 404 drops the candidate; an error keeps it (the download decides).
func (a *caaAsk) start(ctx context.Context, r *Resolver, mbid string) {
	a.once.Do(func() {
		a.done = make(chan struct{})
		if !r.on("caa") {
			close(a.done)
			return
		}
		go func() {
			defer close(a.done)
			b, err := r.call(ctx, "caa", newReq(base(r.CAA, "https://coverartarchive.org")+"/release-group/"+mbid))
			if err == nil && b == nil {
				return
			}
			a.c = r.caaAlbum(mbid)
		}()
	})
}

func (a *caaAsk) wait(ctx context.Context, r *Resolver, mbid string) []Candidate {
	a.start(ctx, r, mbid)
	select {
	case <-a.done:
		return a.c
	case <-ctx.Done():
		return nil
	}
}

// Keys are the picture sources' credentials (from the environment; never
// logged, never in an error).
type Keys struct {
	YouTube                  string
	SpotifyID, SpotifySecret string
}

func (k Keys) secrets() []string { return []string{k.YouTube, k.SpotifyID, k.SpotifySecret} }

// Resolver lists a picture's candidates per source, in the decided order:
//
//	album:  Apple Music > Deezer > Cover Art Archive
//	artist: Apple Music (a real photo) > Spotify > Deezer
//	wide:   fanart.tv > Apple Music video still > YouTube video still
//	logo:   fanart.tv
type Resolver struct {
	HTTP      *http.Client
	UserAgent string
	// On reports whether a source is on (kill switch); nil: all on.
	On func(source string) bool
	// Info reads the subject of a picture (nil: nothing known).
	Info func(ctx context.Context, kind, mbid string) Subject
	// Fanart answers fanart.tv's music art (extras.Art JSON) for an artist;
	// ErrNoPicture when it has none, nil Fanart when the source is off.
	Fanart func(ctx context.Context, mbid string) ([]byte, error)
	// TitleFanart answers fanart.tv's art (extras.Art JSON) for a movie or
	// show by TMDB id; ErrNoPicture when it has none, nil when off.
	TitleFanart func(ctx context.Context, kind, tmdb string) ([]byte, error)
	// PersonPhoto answers the Commons file of a person's Wikidata photo
	// (P18) by TMDB person id ("" when none).
	PersonPhoto func(ctx context.Context, tmdb string) (string, error)
	// Videos answers an artist's checked music-video IDs, best first.
	Videos func(ctx context.Context, mbid string) []string
	// Keep and Kept store and read each source's match for a MusicBrainz
	// id (Store.KeepID, Store.KeptID); nil keeps none.
	Keep func(kind, mbid string, t srctag.Tag, store, ext string)
	Kept func(kind, mbid, source string) (store, ext string, at time.Time, ok bool)
	// Shared says another id keeps the same source id (Store.SharedID); nil
	// trusts every match.
	Shared func(kind, source, ext, except string) bool
	// Homonyms answers the other MusicBrainz artists that carry exactly
	// this name (mbread Reader.HomonymIDs); an Apple match by name alone
	// with any of them is only taken on album evidence (AppleEvidenceOf).
	// nil: no check.
	Homonyms func(ctx context.Context, name, except string) []string
	// TitleArtists answers how many artists in the slim copy hold a
	// release-group title (mbread Reader.TitleArtists): a match resting on
	// one title in common needs a rare one. nil: no such match is taken.
	TitleArtists func(ctx context.Context, title string) int
	// Barcodes answers up to n barcodes of the artist's own releases
	// (mbread Reader.ArtistBarcodes); WikidataIDs the Apple Music, Deezer
	// and Spotify ids a Wikidata item names (kept answers). Both are asked
	// after MusicBrainz's link and before a name search (ExactID); nil
	// skips that step. Via notes how each match was found (Store.SetVia).
	Barcodes    func(ctx context.Context, mbid string, n int) []string
	WikidataIDs func(ctx context.Context, qid string) (apple, deezer, spotify string, err error)
	Via         func(kind, mbid, source, via string)
	AppleMusic  *AppleMusic
	// MB answers the slim MusicBrainz copy's stored answer (mbread
	// Reader.Stored); with it Deezer is matched by IDs (DeezerMatch).
	MB func(ctx context.Context, kind, mbid string) ([]byte, string, bool)
	// MBTitles answers an artist's release-group titles in the slim copy;
	// with it a weak Deezer artist link is checked against the top search
	// candidate (nil: links are taken as they are).
	MBTitles func(ctx context.Context, artist string) []string
	// PoolHTTP is the client over the 7-IP egress pool, used for Deezer
	// and Spotify only (nil: HTTP). Apple and Google never use it.
	PoolHTTP *http.Client
	// AppleOverflow is the Apple queue wait past which covers and photos
	// overflow to Deezer and Spotify (0: DefaultAppleOverflow).
	AppleOverflow time.Duration
	Keys          Keys
	Limits        *Limits
	// Bases (tests point them at fakes).
	CAA, Deezer, SpotifyAPI, SpotifyAuth, YTImg, CommonsAPI string

	tokMu  sync.Mutex
	tokens map[string]token

	memoMu sync.Mutex
	memo   map[string]memoed
}

type token struct {
	value string
	until time.Time
}

type memoed struct {
	v   any
	err error
	at  time.Time
}

// memoFor is how long a source lookup is reused (the photo and wide chains
// of one artist share its Apple Music artist and its fanart.tv list).
const memoFor = 10 * time.Minute

func (r *Resolver) on(p string) bool { return r.On == nil || r.On(p) }

func base(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func (r *Resolver) info(ctx context.Context, kind, mbid string) Subject {
	if r.Info == nil {
		return Subject{}
	}
	return r.Info(ctx, kind, mbid)
}

// once runs f once per key within memoFor; an error is not kept.
func (r *Resolver) once(key string, f func() (any, error)) (any, error) {
	r.memoMu.Lock()
	if m, ok := r.memo[key]; ok && time.Since(m.at) < memoFor {
		r.memoMu.Unlock()
		return m.v, m.err
	}
	r.memoMu.Unlock()
	v, err := f()
	if err == nil {
		r.memoMu.Lock()
		if r.memo == nil || len(r.memo) > 5000 {
			r.memo = map[string]memoed{}
		}
		r.memo[key] = memoed{v: v, at: time.Now()}
		r.memoMu.Unlock()
	}
	return v, err
}

// Chain answers the steps for one picture, in order. An off source's step
// answers nothing.
func (r *Resolver) Chain(kind, mbid string) []Step {
	step := func(name string, f func(context.Context) ([]Candidate, error)) Step {
		return Step{Name: name, Get: func(ctx context.Context) ([]Candidate, error) {
			if !r.on(name) {
				return nil, nil
			}
			c, err := f(ctx)
			if err != nil {
				var se *statusError
				paused := errors.Is(err, ErrSourcePaused) || errors.As(err, &se) && se.code == 429
				return nil, &SourceError{Source: name, Err: errors.New(Redact(err.Error(), r.Keys.secrets()...)), Paused: paused}
			}
			return c, nil
		}}
	}
	// overflow is set when Apple is skipped for its queue (or is off):
	// the later sources' pictures are then tagged for an Apple upgrade.
	overflow := false
	apple := func(f func(context.Context) ([]Candidate, error)) Step {
		s := step("applemusic", f)
		get := s.Get
		// Busy is asked before step's on/off gate, so a switched-off
		// Apple also tags the later pictures.
		s.Get = func(ctx context.Context) ([]Candidate, error) {
			if r.appleBusy() {
				overflow = true
				return nil, nil
			}
			return get(ctx)
		}
		return s
	}
	tagged := func(name string, f func(context.Context) ([]Candidate, error)) Step {
		return step(name, func(ctx context.Context) ([]Candidate, error) {
			c, err := f(ctx)
			if overflow {
				for i := range c {
					c[i].Upgrade, c[i].Recheck = "applemusic", UpgradeRecheck
				}
			}
			return c, err
		})
	}
	switch kind {
	case KindAlbum:
		// Deezer and the Cover Art Archive are both fallbacks: both are
		// asked at once, Deezer's answer still goes first.
		var caa caaAsk
		return []Step{
			apple(func(ctx context.Context) ([]Candidate, error) { return r.appleAlbum(ctx, mbid) }),
			tagged("deezer", func(ctx context.Context) ([]Candidate, error) {
				caa.start(ctx, r, mbid)
				return r.deezerAlbum(ctx, mbid)
			}),
			tagged("caa", func(ctx context.Context) ([]Candidate, error) { return caa.wait(ctx, r, mbid), nil }),
		}
	case KindArtist:
		return []Step{
			apple(func(ctx context.Context) ([]Candidate, error) { return r.applePhoto(ctx, mbid) }),
			tagged("spotify", func(ctx context.Context) ([]Candidate, error) { return r.spotifyPhoto(ctx, mbid) }),
			tagged("deezer", func(ctx context.Context) ([]Candidate, error) { return r.deezerPhoto(ctx, mbid) }),
		}
	case KindWide:
		return []Step{
			step("fanart", func(ctx context.Context) ([]Candidate, error) { return r.fanart(ctx, mbid, KindWide) }),
			step("applemusic", func(ctx context.Context) ([]Candidate, error) { return r.appleVideoStills(ctx, mbid) }),
			step("youtube", func(ctx context.Context) ([]Candidate, error) { return r.youtubeStills(ctx, mbid), nil }),
		}
	case KindLogo:
		return []Step{
			step("fanart", func(ctx context.Context) ([]Candidate, error) { return r.fanart(ctx, mbid, KindLogo) }),
		}
	case KindTitleLogo, KindTitleBanner, KindTitleWide:
		return []Step{
			step("fanart", func(ctx context.Context) ([]Candidate, error) { return r.titleArt(ctx, mbid, kind) }),
		}
	case KindPerson:
		return []Step{
			step("wikimedia", func(ctx context.Context) ([]Candidate, error) { return r.personPhoto(ctx, mbid) }),
		}
	}
	return nil
}

// SourceError is a source that could not answer (busy, down, a timeout):
// the picture is asked for again later and never stored as missing.
type SourceError struct {
	Source string
	Err    error
	// Paused is a source paused or rate-limited for now: a picture not
	// stored yet skips it for the next source, tagged for an upgrade.
	Paused bool
}

func (e *SourceError) Error() string { return e.Source + ": " + e.Err.Error() }
func (e *SourceError) Unwrap() error { return e.Err }

// statusError is a source's answer other than 200.
type statusError struct {
	name string
	code int
}

func (e *statusError) Error() string { return fmt.Sprintf("%s: answered %d", e.name, e.code) }

// call makes one paced request to a source's API and reads its body. A 404
// (and 400, a store or item that does not exist) answers nil, nil: nothing
// there. A 429 backs the source off; failures count toward its pause. The
// error never holds the URL (keys may be in it).
func (r *Resolver) call(ctx context.Context, name string, req *http.Request) ([]byte, error) {
	if err := r.Limits.Allow(ctx, name); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if req.Header.Get("User-Agent") == "" && r.UserAgent != "" {
		req.Header.Set("User-Agent", r.UserAgent)
	}
	c := r.HTTP
	if r.PoolHTTP != nil && (name == "deezer" || name == "spotify") {
		c = r.PoolHTTP
	}
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Do(req.WithContext(ctx))
	if err != nil {
		r.Limits.Done(name, 0, err)
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("%s: request failed: %v", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		secs, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		r.Limits.Backoff(name, time.Duration(secs)*time.Second)
		return nil, &statusError{name, resp.StatusCode}
	}
	r.Limits.Done(name, resp.StatusCode, nil)
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest:
		return nil, nil
	case resp.StatusCode != 200:
		return nil, &statusError{name, resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, err)
	}
	return b, nil
}

func newReq(u string) *http.Request {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		req, _ = http.NewRequest(http.MethodGet, "http://invalid.invalid/", nil)
	}
	return req
}

// Package musictrend keeps the music lists: the paid list source's charts
// first (the lists package), Apple Music API charts per storefront, and
// the ListenBrainz fresh, rising and classic lists, matched once to
// MusicBrainz ids.
package musictrend

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// Options configure the music trending job.
type Options struct {
	// ExactArtist resolves a chart row's own artist id or barcode to a
	// MusicBrainz artist (SetExactArtist); it beats the name match.
	ExactArtist func(ctx context.Context, source, id, name string, codes []string) string
	// SharedName reports a name two or more MusicBrainz artists carry
	// (SetSharedName): without exact evidence such a row links to none.
	SharedName func(ctx context.Context, name string) bool
	// ListenBrainzURL feeds only the fresh, rising and classic extras; the
	// top albums and trending artists come from the paid list source's
	// charts (Lists), else Apple Music's.
	ListenBrainzURL string
	// Apple reads the Apple Music API (SetApple); its charts are read once
	// a day (AppleEvery). nil: no Apple charts.
	Apple      AppleAPI
	AppleEvery time.Duration
	// Lists is the paid list source's charts and genre pages (SetLists).
	Lists     ListCharts
	UserAgent string
	HTTP      *http.Client
	Search    Searcher
	// Every is how often the sources are checked (default 6 h); a failure
	// retries after 1 min, doubling up to Every.
	Every time.Duration
	// Pause is the wait between Apple Music storefront reads (default 250 ms).
	Pause time.Duration
	// Size is how many entries a list keeps (default 30).
	Size int
	// Report counts problems for the statistics, per source:
	// ("listenbrainz"|"apple", "fetch_failed") or ("musicbrainz", "unmatched").
	Report func(source, code string, n int)
	// Extras also builds the fresh, rising and classic lists (ListenBrainz
	// root = ListenBrainzURL without /stats/sitewide).
	Extras bool
	// Tags answers the slim MusicBrainz copy's genre and tag names of a
	// release group or artist (kind "release-group" or "artist"; no
	// call), for the genre pages (SetTags). nil: no genre pages.
	Tags func(ctx context.Context, kind, mbid string) []string
	// SlimAlbum answers an artist's release group whose title matches a
	// charted one, from the slim copy (SetSlimAlbum; no call). nil: none.
	SlimAlbum func(ctx context.Context, artistMBID, title string) string
}

type listKey struct{ Kind, Region string }

type list struct {
	body []byte
	etag string
}

type chart struct{ artists, albums, songs []Entry }

// Service keeps every list as ready JSON with a strong ETag.
type Service struct {
	// published: the lists were built since the start.
	published bool
	db        *sql.DB
	opts      Options
	matcher   *Matcher
	lists     atomic.Pointer[map[listKey]list]
	mu        sync.Mutex
	// apple is Apple Music's chart by storefront (lower case): the
	// most-played albums, and artists ranked by appearances in the
	// most-played songs and albums.
	apple   map[string]chart
	appleAt time.Time
	// amu guards apple for the chart readers (Charts, Genres, Suggest),
	// which do not wait for a running Refresh.
	amu sync.RWMutex
	// genres keeps the per-genre Apple charts; listsAt is when the list
	// source's charts last went into the lists.
	genres  genreCharts
	listsAt time.Time
	// lb is ListenBrainz's sitewide lists: the backup chart and the genre
	// pages' pool.
	lb lbCharts
	// Builds counts list rebuilds (for tests).
	Builds int
}

// Open opens (or creates) the store at path and loads the last good lists.
func Open(path string, o Options) (*Service, error) {
	if o.Every <= 0 {
		o.Every = 6 * time.Hour
	}
	if o.Pause < 0 {
		o.Pause = 0
	} else if o.Pause == 0 {
		o.Pause = 250 * time.Millisecond
	}
	if o.Size <= 0 {
		o.Size = 30
	}
	if o.AppleEvery <= 0 {
		o.AppleEvery = 24 * time.Hour
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: time.Minute}
	}
	if o.Report == nil {
		o.Report = func(string, string, int) {}
	}
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	// Each list carries its source and fetch time (the body's "source" and
	// "updated"), like every other music store table.
	if err := srctag.Stamp(db, srctag.Spec{Table: "lists", Key: []string{"kind", "region"}, Body: "body",
		Src:   "COALESCE(json_extract(CAST(@.body AS TEXT),'$.source'),'" + srctag.Untagged + "')",
		Known: "COALESCE(json_extract(CAST(@.body AS TEXT),'$.updated'),0)"}); err != nil {
		db.Close()
		return nil, err
	}
	s := &Service{db: db, opts: o, matcher: &Matcher{DB: db, Search: o.Search}, apple: map[string]chart{}}
	loaded := map[listKey]list{}
	rs, err := db.Query("SELECT kind, region, body FROM lists")
	if err != nil {
		db.Close()
		return nil, err
	}
	for rs.Next() {
		var k listKey
		var b []byte
		// Lists stored before the source tags are not served again.
		if rs.Scan(&k.Kind, &k.Region, &b) == nil && bodySchema(b) >= listsSchema {
			loaded[k] = list{b, etagOf(b)}
		}
	}
	rs.Close()
	s.lists.Store(&loaded)
	if err := openCharts(s); err != nil {
		db.Close()
		return nil, err
	}
	s.loadLB()
	return s, nil
}

// bodySchema answers a stored body's schema_version (0: none).
func bodySchema(b []byte) int {
	var v struct {
		V int `json:"schema_version"`
	}
	_ = json.Unmarshal(b, &v)
	return v.V
}

// SetTags sets the slim copy's tag reader (call before Run).
func (s *Service) SetTags(f func(ctx context.Context, kind, mbid string) []string) { s.opts.Tags = f }

// Close closes the store.
func (s *Service) Close() error { return s.db.Close() }

// SetApple sets the Apple Music API reader; SetLists the list source's
// charts (call both before Run).
func (s *Service) SetApple(a AppleAPI)   { s.opts.Apple = a }
func (s *Service) SetLists(l ListCharts) { s.opts.Lists = l }

// NameMatcher is a matcher on the chart matches (kept in this store), for
// another job's own name searches.
func (s *Service) NameMatcher() *Matcher { return &Matcher{DB: s.db, Search: s.opts.Search} }

func etagOf(b []byte) string {
	h := sha256.Sum256(b)
	return `"` + hex.EncodeToString(h[:16]) + `"`
}

// List returns one list's JSON and ETag; kind is artist or album, region
// is global or an ISO country code.
func (s *Service) List(kind, region string) ([]byte, string, bool) {
	l, ok := (*s.lists.Load())[listKey{kind, region}]
	return l.body, l.etag, ok
}

func (s *Service) get(ctx context.Context, url string, parse func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", s.opts.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.opts.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("musictrend: " + url + " answered " + resp.Status)
	}
	return parse(io.LimitReader(resp.Body, 8<<20))
}

// Storefronts are the Apple Music countries the charts are kept for.
func Storefronts() []string {
	out := make([]string, 0, len(countries))
	seen := map[string]bool{}
	for _, c := range countries {
		if !seen[c] {
			seen[c] = true
			out = append(out, strings.ToLower(c))
		}
	}
	sort.Strings(out)
	return out
}

// refreshApple reads Apple Music's most-played albums and songs once a
// day for every storefront ("us" is also the global list). A storefront
// that fails keeps its last chart; one Apple does not have is skipped.
func (s *Service) refreshApple(ctx context.Context) (bool, error) {
	if s.opts.Apple == nil || len(s.apple) > 0 && time.Since(s.appleAt) < s.opts.AppleEvery {
		return false, nil
	}
	changed := false
	var usErr error
	for i, st := range Storefronts() {
		if i > 0 && s.opts.Pause > 0 {
			select {
			case <-ctx.Done():
				return changed, ctx.Err()
			case <-time.After(s.opts.Pause):
			}
		}
		albums, songs, ok, err := readAppleChart(ctx, s.opts.Apple, st, "", 2)
		if err != nil {
			if ctx.Err() != nil {
				return changed, ctx.Err()
			}
			if st == "us" {
				usErr = err
			}
			continue
		}
		if !ok || len(albums) == 0 {
			continue
		}
		c := chart{artists: RankArtists(songs, albums), albums: albums, songs: songs}
		s.keepRanks(st, c, time.Now())
		if old, ok := s.apple[st]; !ok || !sameEntries(old.albums, c.albums) || !sameEntries(old.artists, c.artists) || !sameEntries(old.songs, c.songs) {
			s.amu.Lock()
			s.apple[st], changed = c, true
			s.amu.Unlock()
		}
	}
	if usErr == nil {
		s.appleAt = time.Now()
	}
	return changed, usErr
}

// RankArtists ranks the artists of the most-played songs and albums by how
// often they appear, ties by where they first appear.
func RankArtists(feeds ...[]Entry) []Entry {
	type tally struct {
		e     Entry
		n     int
		first int
	}
	by := map[string]*tally{}
	pos := 0
	for _, f := range feeds {
		for _, e := range f {
			pos++
			k := norm(e.Artist)
			if k == "" {
				continue
			}
			t := by[k]
			if t != nil {
				t.n++
			} else {
				t = &tally{e: Entry{Name: e.Artist}, n: 1, first: pos}
				by[k] = t
			}
			// The artist keeps its Apple id and its albums' barcodes, for
			// the exact resolution.
			if t.e.AppleArtist == "" {
				t.e.AppleArtist = e.AppleArtist
			}
			if c := EntryBarcode(e); c != "" && len(t.e.Codes) < 3 && !slices.Contains(t.e.Codes, c) {
				t.e.Codes = append(t.e.Codes, c)
			}
		}
	}
	list := make([]*tally, 0, len(by))
	for _, t := range by {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].first < list[j].first
	})
	out := make([]Entry, len(list))
	for i, t := range list {
		out[i] = t.e
	}
	return out
}

func sameEntries(a, b []Entry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].Artist != b[i].Artist || a[i].Cover != b[i].Cover {
			return false
		}
	}
	return true
}

// Refresh checks both sources and rebuilds the lists when either changed.
// A source that fails keeps the lists it built last time.
func (s *Service) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	apChanged, apErr := s.refreshApple(ctx)
	if apErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("musictrend: apple music: %v", apErr)
		s.opts.Report("apple", "fetch_failed", 1)
	}
	lbBefore := s.lbAt()
	lbErr := s.refreshLB(ctx)
	if lbErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("musictrend: listenbrainz charts: %v", lbErr)
	}
	listsNew := !s.lbAt().Equal(lbBefore) || len(*s.lists.Load()) == 0
	if s.opts.Lists != nil {
		if at := s.opts.Lists.Built(); at.After(s.listsAt) {
			s.listsAt, listsNew = at, true
		}
	}
	// Rows without an artist id get Apple's, asked once per item (kept).
	if s.fillAllAppleArtists(ctx) {
		apChanged = true
	}
	if s.mustPublish(apChanged, listsNew) {
		s.published = true
		// Serve what is already matched straight away, then match the rest
		// (MusicBrainz is paced, so a first run takes a while).
		if err := s.publish(ctx, false); err != nil {
			return err
		}
		before := s.matcher.Unmatched
		if err := s.publish(ctx, true); err != nil {
			return err
		}
		if n := s.matcher.Unmatched - before; n > 0 {
			s.opts.Report("musicbrainz", "unmatched", n)
		}
	}
	var extras map[listKey][]byte
	var exErr error
	if s.opts.Extras {
		extras, exErr = s.refreshExtras(ctx)
	}
	if exErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Printf("musictrend: listenbrainz extras: %v", exErr)
		s.opts.Report("listenbrainz", "fetch_failed", 1)
	}
	s.storeExtras(ctx, extras)
	// ListenBrainz is the backup: its failure is logged, never retried early.
	return errors.Join(apErr, exErr)
}

func (s *Service) lbAt() time.Time {
	s.lb.mu.RLock()
	defer s.lb.mu.RUnlock()
	return s.lb.at
}

// storeExtras swaps in the fresh, rising and classic lists that came back
// and saves them; the others keep their last copy.
func (s *Service) storeExtras(ctx context.Context, extras map[listKey][]byte) {
	if len(extras) == 0 {
		return
	}
	old := *s.lists.Load()
	out := make(map[listKey]list, len(old)+len(extras))
	for k, v := range old {
		out[k] = v
	}
	for k, b := range extras {
		out[k] = list{b, etagOf(b)}
	}
	s.lists.Store(&out)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	for k, b := range extras {
		_, _ = tx.Exec("DELETE FROM lists WHERE kind = ? AND region = ?", k.Kind, k.Region)
		_, _ = tx.Exec("INSERT INTO lists (kind, region, body) VALUES (?, ?, ?)", k.Kind, k.Region, b)
	}
	_ = tx.Commit()
}

type artistItem struct {
	Rank    int    `json:"rank"`
	MBID    string `json:"mbid"`
	Name    string `json:"name"`
	Listens int64  `json:"listen_count,omitempty"`
	// Image is the chart source's artist picture (500 px), ImageSizes the
	// same at 250/500/1200.
	Image      string            `json:"image,omitempty"`
	ImageSizes map[string]string `json:"image_sizes,omitempty"`
	Source     string            `json:"source"`
}

type albumItem struct {
	Rank       int    `json:"rank"`
	MBID       string `json:"mbid"`
	Title      string `json:"title"`
	Artist     string `json:"artist_name"`
	ArtistMBID string `json:"artist_mbid,omitempty"`
	CAARelease string `json:"caa_release_mbid,omitempty"`
	Listens    int64  `json:"listen_count,omitempty"`
	// Cover is a fast cover link: the chart source's (Apple Music or
	// Deezer) first, the Cover Art Archive last. CoverSizes is 250/500/1200.
	Cover      string            `json:"cover,omitempty"`
	CoverSizes map[string]string `json:"cover_sizes,omitempty"`
	Source     string            `json:"source"`
}

// artistKind reports a list of artist rows ("artist" in the Apple charts,
// "artists" in the list source's), where an unlinked artist is the row
// itself.
func artistKind(kind string) bool { return kind == "artist" || kind == "artists" }

// resolve gives an entry its MusicBrainz ids, searching only when search
// is true.
func (s *Service) resolve(ctx context.Context, kind string, e Entry, search bool) (Entry, bool) {
	if e.MBID != "" || e.Unlinked && artistKind(kind) {
		return e, true
	}
	artist := e.Artist
	if kind == "artist" {
		artist = ""
	}
	var c Candidate
	var ok bool
	if search {
		c, ok = s.matcher.Match(ctx, kind, e.Name, artist)
	} else {
		c, ok, _ = s.matcher.Known(ctx, kind, e.Name, artist)
	}
	if ok {
		e.MBID = c.MBID
		if kind == "album" {
			e.ArtistMBID = c.ArtistMBID
		}
	}
	// The row's own artist id (or its barcodes), resolved exactly, beats
	// the name match.
	if s.opts.ExactArtist != nil {
		codes := e.Codes
		if kind != "artist" {
			codes = nil
			if c := EntryBarcode(e); c != "" {
				codes = []string{c}
			}
		}
		name := e.Artist
		if kind == "artist" {
			name = e.Name
		}
		x := s.opts.ExactArtist(ctx, "applemusic", e.AppleArtist, name, codes)
		if x == "" {
			if s.shared(ctx, name) {
				if kind == "artist" {
					e.MBID, e.Unlinked, ok = "", true, true
				} else {
					e.ArtistMBID = ""
				}
			}
		}
		if x != "" {
			switch {
			case kind == "artist":
				e.MBID, ok = x, true
			case x != e.ArtistMBID:
				// The matched album is the other artist's: none beats it.
				e.ArtistMBID, e.MBID, ok = x, "", false
				if s.opts.SlimAlbum != nil {
					if rg := s.opts.SlimAlbum(ctx, x, e.Name); rg != "" {
						e.MBID, ok = rg, true
					}
				}
			}
		}
	}
	return e, ok
}

func (s *Service) items(ctx context.Context, kind, source string, entries []Entry, search bool) []any {
	out := []any{}
	seen := map[string]bool{}
	for _, e := range entries {
		if len(out) >= s.opts.Size || ctx.Err() != nil {
			break
		}
		e, ok := s.resolve(ctx, kind, e, search)
		key := e.MBID
		if e.Unlinked && e.MBID == "" {
			key = "name:" + norm(e.Name)
		}
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		if kind == "artist" {
			out = append(out, artistItem{Rank: len(out) + 1, MBID: e.MBID, Name: e.Name, Listens: e.Listens, Image: e.Cover, ImageSizes: CoverVariants(e.Cover), Source: source})
		} else {
			cover := e.Cover
			if cover == "" && e.CAARelease != "" {
				cover = CoverArtArchiveURL + "/release/" + e.CAARelease + "/front-500"
			} else if cover == "" {
				cover = CoverArtArchiveURL + "/release-group/" + e.MBID + "/front-500"
			}
			out = append(out, albumItem{Rank: len(out) + 1, MBID: e.MBID, Title: e.Name, Artist: e.Artist, ArtistMBID: e.ArtistMBID, CAARelease: e.CAARelease, Listens: e.Listens, Cover: cover, CoverSizes: CoverVariants(cover), Source: source})
		}
	}
	return out
}

// publish builds every list and swaps them in; lists that come out empty
// keep their last good version. A region's list comes from the list
// source's chart when it has one (its matched entries only), else from
// Apple Music's.
func (s *Service) publish(ctx context.Context, search bool) error {
	type src struct {
		region, source string
		entries        []Entry
	}
	have := map[string]bool{}
	for st := range s.apple {
		have[st] = true
	}
	listed := func(region, kind string) ([]Entry, bool) {
		if s.opts.Lists == nil {
			return nil, false
		}
		c, ok := s.opts.Lists.ChartList(ctx, region, kind)
		var out []Entry
		for _, e := range c.Entries {
			// An album whose artist the exact id corrected is found again
			// under that artist.
			if e.MBID == "" && e.ArtistMBID != "" && !artistKind(kind) && s.opts.SlimAlbum != nil {
				e.MBID = s.opts.SlimAlbum(ctx, e.ArtistMBID, e.Name)
			}
			// An album needs its own id; only an artist may be listed unlinked.
			if e.MBID != "" || e.Unlinked && artistKind(kind) {
				out = append(out, e)
			}
		}
		return out, ok && len(out) > 0
	}
	for _, r := range s.listRegions() {
		have[r] = true
	}
	regions := []string{}
	if have["us"] || have["global"] || len(s.lbList("album")) > 0 {
		regions = append(regions, "global")
	}
	for _, st := range Storefronts() {
		if have[st] {
			regions = append(regions, strings.ToUpper(st))
		}
	}
	srcs := map[string][]src{}
	for _, r := range regions {
		st := strings.ToLower(r)
		if r == "global" {
			st = "us"
		}
		c := s.apple[st]
		for _, k := range [][2]string{{"album", "albums"}, {"artist", "artists"}} {
			if es, ok := listed(strings.ToLower(r), k[1]); ok {
				srcs[k[0]] = append(srcs[k[0]], src{r, SrcLists, es})
			} else if k[0] == "album" && len(c.albums) > 0 {
				srcs[k[0]] = append(srcs[k[0]], src{r, SrcApple, c.albums})
			} else if k[0] == "artist" && len(c.artists) > 0 {
				srcs[k[0]] = append(srcs[k[0]], src{r, SrcApple, c.artists})
			} else if es := s.lbList(k[0]); len(es) > 0 {
				srcs[k[0]] = append(srcs[k[0]], src{r, SrcListenBrainz, es})
			}
		}
	}
	now := time.Now().Unix()
	old := *s.lists.Load()
	out := make(map[listKey]list, len(old))
	for k, v := range old {
		out[k] = v
	}
	for _, kind := range []string{"artist", "album"} {
		for _, sr := range srcs[kind] {
			items := s.items(ctx, kind, sr.source, sr.entries, search)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if len(items) == 0 {
				continue
			}
			b, err := json.Marshal(map[string]any{"schema_version": listsSchema, "kind": kind, "region": sr.region, "source": sr.source, "updated": now, "items": items, "regions": regions})
			if err != nil {
				return err
			}
			out[listKey{kind, sr.region}] = list{b, etagOf(b)}
			if search {
				// Matching is slow on a first run: serve each list as it is done.
				done := make(map[listKey]list, len(out))
				for k, v := range out {
					done[k] = v
				}
				s.lists.Store(&done)
			}
		}
	}
	s.lists.Store(&out)
	s.Builds++
	tx, err := s.db.BeginTx(ctx, nil)
	if err == nil {
		_, _ = tx.Exec("DELETE FROM lists")
		for k, l := range out {
			_, _ = tx.Exec("INSERT INTO lists (kind, region, body) VALUES (?, ?, ?)", k.Kind, k.Region, l.body)
		}
		_ = tx.Commit()
	}
	return nil
}

// lbList answers ListenBrainz's sitewide artists ("artist") or albums.
func (s *Service) lbList(kind string) []Entry {
	s.lb.mu.RLock()
	defer s.lb.mu.RUnlock()
	if kind == "artist" {
		return s.lb.artists
	}
	return s.lb.albums
}

// Run refreshes now and then every Every, with backoff after failures,
// until ctx ends.
func (s *Service) Run(ctx context.Context) {
	// Album matches kept before same-named artists were told apart are
	// re-pointed before the first publish (no search is made).
	if s.opts.SlimAlbum != nil {
		if n, err := s.NameMatcher().RepairHomonyms(ctx, s.opts.SlimAlbum); err != nil {
			log.Printf("musictrend: homonym repair: %v", err)
		} else {
			log.Printf("musictrend: homonym repair: %d album matches re-pointed", n)
		}
	}
	backoff := time.Minute
	for {
		wait := s.opts.Every
		if err := s.Refresh(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			wait, backoff = backoff, min(backoff*2, s.opts.Every)
		} else {
			backoff = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// SetSharedName sets Options.SharedName. Call before Run.
func (s *Service) SetSharedName(f func(ctx context.Context, name string) bool) {
	s.opts.SharedName = f
}

// shared reports a name several MusicBrainz artists carry.
func (s *Service) shared(ctx context.Context, name string) bool {
	return s.opts.SharedName != nil && name != "" && s.opts.SharedName(ctx, name)
}

// mustPublish: a source changed, or nothing was published since the start
// (a new build's resolution shows at once, not at the next change).
func (s *Service) mustPublish(apChanged, listsNew bool) bool {
	return apChanged || listsNew || !s.published
}

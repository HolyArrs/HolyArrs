package musictrend

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// FreshReleasesURL is the ListenBrainz fresh releases explorer (keyless).
const FreshReleasesURL = "https://api.listenbrainz.org/1/explore/fresh-releases/"

// New release windows served by GET /v1/music/new?window=.
const (
	WindowWeek     = "week"     // out in the last 7 days
	WindowRecent   = "recent"   // out 8 to 90 days ago
	WindowUpcoming = "upcoming" // out in the next 90 days
	WindowTracks   = "tracks"   // the song chart, fresh albums first
)

// Windows lists every window, in the order the app shows them.
var Windows = []string{WindowTracks, WindowWeek, WindowRecent, WindowUpcoming}

// NewOptions configure the new music job.
type NewOptions struct {
	// FreshURL is ListenBrainz's fresh releases, the backup source.
	FreshURL  string
	UserAgent string
	// Sources are the new-release sources read before ListenBrainz, in
	// SourceOrder (the list source, then Apple Music; SetSources).
	Sources []NewSource
	HTTP    *http.Client
	// Every is how often both sources are read (default 24 h).
	Every time.Duration
	// Pause is the wait between Deezer album reads (default 250 ms).
	Pause time.Duration
	// Size caps each window (default 60).
	Size int
	// Report counts problems for the statistics.
	Report func(source, code string, n int)
	// Now is the clock (tests).
	Now func() time.Time
}

// NewItem is one new album (or, in the tracks window, one track).
type NewItem struct {
	Rank        int    `json:"rank"`
	MBID        string `json:"mbid"`
	ArtistMBID  string `json:"artist_mbid,omitempty"`
	Title       string `json:"title"`
	Artist      string `json:"artist_name"`
	Track       string `json:"track,omitempty"`
	ReleaseDate string `json:"release_date,omitempty"`
	Type        string `json:"type,omitempty"`
	Cover       string `json:"cover,omitempty"`
	// CoverSizes is the cover at 250, 500 and 1200 px when its source has sizes.
	CoverSizes map[string]string `json:"cover_sizes,omitempty"`
	Explicit   bool              `json:"explicit"`
	Popularity int               `json:"popularity,omitempty"` // Deezer chart or editorial position, 1 = top
	Listens    int64             `json:"listen_count,omitempty"`
	DeezerID   string            `json:"deezer_id,omitempty"`
	Sources    []string          `json:"sources"`
	// Source is where the item came from (SourceOrder).
	Source string `json:"source"`
}

// ParseFreshReleases reads the ListenBrainz fresh releases answer.
func ParseFreshReleases(r io.Reader) ([]NewItem, error) {
	var body struct {
		Payload struct {
			Releases []struct {
				ArtistName  string      `json:"artist_credit_name"`
				ArtistMBIDs []string    `json:"artist_mbids"`
				CAARelease  string      `json:"caa_release_mbid"`
				CAAID       json.Number `json:"caa_id"`
				Date        string      `json:"release_date"`
				GroupMBID   string      `json:"release_group_mbid"`
				Type        string      `json:"release_group_primary_type"`
				Name        string      `json:"release_name"`
				Listens     int64       `json:"listen_count"`
			} `json:"releases"`
		} `json:"payload"`
	}
	d := json.NewDecoder(r)
	d.UseNumber()
	if err := d.Decode(&body); err != nil {
		return nil, err
	}
	var out []NewItem
	seen := map[string]bool{}
	for _, x := range body.Payload.Releases {
		id := strings.ToLower(x.GroupMBID)
		if !validMBID(id) || seen[id] || x.Name == "" || len(x.Date) < 10 {
			continue
		}
		seen[id] = true
		it := NewItem{MBID: id, Title: x.Name, Artist: x.ArtistName, ReleaseDate: x.Date[:10], Type: strings.ToLower(x.Type), Listens: x.Listens, Sources: []string{"listenbrainz"}}
		if len(x.ArtistMBIDs) > 0 && validMBID(strings.ToLower(x.ArtistMBIDs[0])) {
			it.ArtistMBID = strings.ToLower(x.ArtistMBIDs[0])
		}
		if rel := strings.ToLower(x.CAARelease); validMBID(rel) && x.CAAID.String() != "" {
			it.Cover = "https://coverartarchive.org/release/" + rel + "/" + x.CAAID.String() + "-500.jpg"
		}
		out = append(out, it)
	}
	if len(out) == 0 {
		return nil, errors.New("musictrend: no ListenBrainz fresh releases")
	}
	return out, nil
}

// NewMusic keeps the new release windows as ready JSON, rebuilt daily and
// shared by every caller.
type NewMusic struct {
	db    *sql.DB
	opts  NewOptions
	mu    sync.Mutex
	lists atomic.Pointer[map[string]list]
}

// OpenNew opens (or creates) the store at path and loads the last windows.
func OpenNew(path string, o NewOptions) (*NewMusic, error) {
	if o.Every <= 0 {
		o.Every = 6 * time.Hour
	}
	if o.Pause < 0 {
		o.Pause = 0
	} else if o.Pause == 0 {
		o.Pause = 250 * time.Millisecond
	}
	if o.Size <= 0 {
		o.Size = 60
	}
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: time.Minute}
	}
	if o.Report == nil {
		o.Report = func(string, string, int) {}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS windows (window TEXT PRIMARY KEY, body BLOB NOT NULL) WITHOUT ROWID;`)
	if err != nil {
		db.Close()
		return nil, err
	}
	// Each window names its real source (spotify81, Apple or ListenBrainz)
	// in its body; rows stamped before as listenbrainz are stamped again.
	const windowSrc = "COALESCE(json_extract(CAST(@.body AS TEXT),'$.source'),'listenbrainz')"
	if err := srctag.Stamp(db, srctag.Spec{Table: "windows", Key: []string{"window"}, Body: "body", Src: windowSrc}); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`UPDATE windows SET src=` + strings.ReplaceAll(windowSrc, "@.", "") + ` WHERE length(body)>0 AND src<>` + strings.ReplaceAll(windowSrc, "@.", "")); err != nil {
		db.Close()
		return nil, err
	}
	n := &NewMusic{db: db, opts: o}
	loaded := map[string]list{}
	rs, err := db.Query("SELECT window, body FROM windows")
	if err != nil {
		db.Close()
		return nil, err
	}
	for rs.Next() {
		var w string
		var b []byte
		// Windows stored before the source tags are not served again.
		if rs.Scan(&w, &b) == nil && bodySchema(b) >= listsSchema {
			b = upgradeWindowBody(b)
			loaded[w] = list{b, etagOf(b)}
		}
	}
	rs.Close()
	n.lists.Store(&loaded)
	return n, nil
}

// Close closes the store.
func (n *NewMusic) Close() error { return n.db.Close() }

// SetSources sets the sources read before ListenBrainz (call before Run).
func (n *NewMusic) SetSources(src ...NewSource) { n.opts.Sources = src }

// Window returns one window's JSON and ETag.
func (n *NewMusic) Window(window string) ([]byte, string, bool) {
	l, ok := (*n.lists.Load())[window]
	return l.body, l.etag, ok
}

func (n *NewMusic) get(ctx context.Context, url string, parse func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", n.opts.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := n.opts.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.New("musictrend: " + url + " answered " + resp.Status)
	}
	// The 90-day fresh releases answer is about 15 MB.
	return parse(io.LimitReader(resp.Body, 32<<20))
}

// NewSource is one new-release source in SourceOrder: Read answers its
// new albums (release group MBID and release date set) and its song chart
// for the tracks window, already matched to MusicBrainz ids from stored
// matches (no call at page time; Read runs on the schedule).
type NewSource struct {
	Name string
	Read func(ctx context.Context) (albums, tracks []NewItem, err error)
}

// readFresh is ListenBrainz's fresh releases, the backup source.
func (n *NewMusic) readFresh(ctx context.Context) ([]NewItem, []NewItem, error) {
	if n.opts.FreshURL == "" {
		return nil, nil, nil
	}
	today := n.opts.Now().UTC().Format("2006-01-02")
	var fresh []NewItem
	err := n.get(ctx, n.opts.FreshURL+"?release_date="+today+"&days=90&sort=release_date&past=true&future=true", func(r io.Reader) (err error) {
		fresh, err = ParseFreshReleases(r)
		return err
	})
	if err != nil {
		log.Printf("musictrend: fresh releases: %v", err)
		n.opts.Report(SrcListenBrainz, "fetch_failed", 1)
	}
	return fresh, nil, err
}

// sources answers the sources in SourceOrder: the configured ones, then
// ListenBrainz's fresh releases.
func (n *NewMusic) sources() []NewSource {
	out := append([]NewSource{}, n.opts.Sources...)
	return append(out, NewSource{Name: SrcListenBrainz, Read: n.readFresh})
}

// Refresh reads every source and rebuilds every window: each window comes
// from the first source in SourceOrder with items for it. A window no
// source fills keeps its last copy.
func (n *NewMusic) Refresh(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	type read struct {
		name           string
		albums, tracks []NewItem
	}
	var reads []read
	var errs error
	got := false
	for _, src := range n.sources() {
		albums, tracks, err := src.Read(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			errs = errors.Join(errs, err)
		}
		got = got || len(albums)+len(tracks) > 0
		reads = append(reads, read{src.Name, albums, tracks})
	}
	if !got {
		return errs
	}
	now := n.opts.Now().UTC()
	day := func(d int) string { return now.AddDate(0, 0, d).Format("2006-01-02") }
	windowOf := func(d string) string {
		switch {
		case d == "":
		case d > day(0):
			if d <= day(90) {
				return WindowUpcoming
			}
		case d >= day(-6):
			return WindowWeek
		case d >= day(-90):
			return WindowRecent
		}
		return ""
	}
	windows := map[string][]NewItem{}
	from := map[string]string{}
	for _, r := range reads {
		per := map[string][]NewItem{}
		seen := map[string]bool{}
		for _, it := range r.albums {
			if !validMBID(it.MBID) || seen[it.MBID] {
				continue
			}
			seen[it.MBID] = true
			if w := windowOf(it.ReleaseDate); w != "" {
				it.Source = r.name
				it.Sources = []string{r.name}
				per[w] = append(per[w], it)
			}
		}
		for w, items := range per {
			if _, done := windows[w]; !done && len(items) > 0 {
				windows[w], from[w] = items, r.name
			}
		}
		if _, done := windows[WindowTracks]; !done && len(r.tracks) > 0 {
			var ts []NewItem
			for _, it := range r.tracks {
				if !validMBID(it.ArtistMBID) {
					continue
				}
				it.Source = r.name
				it.Sources = []string{r.name}
				ts = append(ts, it)
			}
			if len(ts) > 0 {
				// Chart order, fresh albums (last 90 days) first.
				sort.SliceStable(ts, func(i, j int) bool {
					fi, fj := ts[i].ReleaseDate >= day(-90), ts[j].ReleaseDate >= day(-90)
					return fi && !fj
				})
				windows[WindowTracks], from[WindowTracks] = ts, r.name
			}
		}
	}
	for w, items := range windows {
		if w == WindowTracks {
			continue
		}
		upcoming := w == WindowUpcoming
		sort.SliceStable(items, func(i, j int) bool { return newer(items[i], items[j], upcoming) })
	}
	old := *n.lists.Load()
	out := make(map[string]list, len(old)+len(windows))
	for k, v := range old {
		out[k] = v
	}
	changed := map[string][]byte{}
	for _, w := range Windows {
		items := windows[w]
		if len(items) == 0 {
			continue
		}
		if len(items) > n.opts.Size {
			items = items[:n.opts.Size]
		}
		for i := range items {
			items[i].Rank = i + 1
			items[i].CoverSizes = CoverVariants(items[i].Cover)
		}
		b, err := json.Marshal(map[string]any{"schema_version": listsSchema, "window": w, "updated": now.Unix(), "source": from[w], "sources": []string{from[w]}, "items": items})
		if err != nil {
			return err
		}
		out[w] = list{b, etagOf(b)}
		changed[w] = b
	}
	n.lists.Store(&out)
	tx, err := n.db.BeginTx(ctx, nil)
	if err == nil {
		for w, b := range changed {
			_, _ = tx.Exec("INSERT OR REPLACE INTO windows (window, body) VALUES (?, ?)", w, b)
		}
		_ = tx.Commit()
	}
	return errs
}

// newer orders by release date (newest first, or soonest first for
// upcoming), then Deezer popularity, then ListenBrainz listens.
func newer(a, b NewItem, upcoming bool) bool {
	if a.ReleaseDate != b.ReleaseDate {
		if upcoming {
			return a.ReleaseDate < b.ReleaseDate
		}
		return a.ReleaseDate > b.ReleaseDate
	}
	pa, pb := a.Popularity, b.Popularity
	if pa == 0 {
		pa = 1 << 30
	}
	if pb == 0 {
		pb = 1 << 30
	}
	if pa != pb {
		return pa < pb
	}
	return a.Listens > b.Listens
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Run refreshes now and then every Every, with backoff after failures.
func (n *NewMusic) Run(ctx context.Context) {
	backoff := time.Minute
	for {
		wait := n.opts.Every
		if err := n.Refresh(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			wait, backoff = backoff, min(backoff*2, n.opts.Every)
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

// upgradeWindowBody adds cover_sizes to a window stored before they existed.
func upgradeWindowBody(b []byte) []byte {
	var m map[string]json.RawMessage
	var items []map[string]any
	if json.Unmarshal(b, &m) != nil || json.Unmarshal(m["items"], &items) != nil {
		return b
	}
	for _, it := range items {
		if u, _ := it["cover"].(string); u != "" {
			if v := CoverVariants(u); v != nil {
				it["cover_sizes"] = v
			}
		}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return b
	}
	m["items"] = raw
	out, err := json.Marshal(m)
	if err != nil {
		return b
	}
	return out
}

// Package herovideo builds each artist's list of music videos (YouTube
// IDs). Candidates come from YouTube Music first: the artist's page found
// by name (searched again in the artist's home country when the name
// misses), its Videos shelf and the first page of its full videos list,
// all with no API points. When YouTube Music has no such artist, the
// uploads of its official channel stand in: the Wikidata channel (P2397),
// else a MusicBrainz channel link, " - Topic" channels skipped.
//
// Every candidate goes through the same filters: no user uploads by
// YouTube Music's label, no audio, visualizer or lyric uploads by title, no
// phone-shaped videos and nothing of 60 seconds or less (one Data API call
// per 50 videos, with a key and quota left; otherwise the videos stay
// unchecked and are checked later), and no still picture over audio (three
// of YouTube's frames compared; unclear ones get a storyboard check later).
// Verdicts are kept for good. Lists are gathered again every 30 days and
// the known channels' feeds are read daily for new uploads. For the first
// few listed videos yt-dlp reads YouTube's "most replayed" peak and the
// storyboard, and the start is the peak when the first seconds from there
// move, else a moving moment near it (start.go); without a storyboard the
// peak as it is, and without a peak the start is left for the app (30% of
// the duration).
package herovideo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	_ "modernc.org/sqlite"
)

// Where a video was found (Video.Source).
const (
	SourceYouTubeMusic = "youtube_music"
	SourceWikidata     = "wikidata"
	SourceMusicBrainz  = "musicbrainz"
)

// Verdicts kept per video. VerdictVideo and VerdictPending are listed (a
// pending one until its storyboard check decides); the others are dropped.
const (
	VerdictVideo   = "video"   // a real video
	VerdictPending = "pending" // unclear frames: waits for the storyboard check
	VerdictStill   = "still"   // a still picture over audio
	VerdictUser    = "ugc"     // a user upload by YouTube Music's label
	VerdictTitle   = "title"   // audio, visualizer or lyric upload by its title
	VerdictShort   = "short"   // 60 seconds or less
	VerdictTall    = "tall"    // phone-shaped (a Short at any length)
	VerdictGone    = "gone"    // removed, private or not on YouTube
)

// Refresh rules: an artist's candidates are gathered again every
// RefreshEvery (YouTube keeps data 30 days at most), its channels' feeds
// are read every FeedEvery, and listed videos still unchecked are looked
// at again a day apart.
var (
	RefreshEvery = 30 * 24 * time.Hour
	FeedEvery    = 20 * time.Hour
	recheckEvery = 24 * time.Hour
)

// Source names in the shared limits (pauses after failures and 429s).
const (
	srcMusic = "youtube_music"
	srcAPI   = "youtube_api"
	srcFeed  = "youtube_feed"
)

var (
	// ErrNoArtist: the artist's stored answer (name, country, links) is not
	// there yet; the job waits for it.
	ErrNoArtist = errors.New("herovideo: artist answer not stored yet")
	// ErrPaused: a source the job needs is paused or switched off; the job
	// is tried again later and nothing is stored.
	ErrPaused = errors.New("herovideo: source paused or switched off")
)

// Video is one music video.
type Video struct {
	YouTubeID string   `json:"youtube_id"`
	Title     string   `json:"title,omitempty"`
	Source    string   `json:"source"` // youtube_music, wikidata or musicbrainz
	Start     *float64 `json:"most_replayed,omitempty"`
	Duration  float64  `json:"duration,omitempty"`
}

// Answer is what /v1/music/videos/{mbid} returns.
type Answer struct {
	ArtistMBID string    `json:"artist_mbid"`
	Videos     []Video   `json:"videos"`
	FetchedAt  time.Time `json:"fetched_at"`
}

// Options configure the service.
type Options struct {
	// HTTP makes every call (YouTube Music, the Data API, feeds, frames,
	// Wikidata); the caller paces it per host.
	HTTP      *http.Client
	UserAgent string
	// Bases (tests point them at fakes): YouTube Music, the Data API, the
	// channel feeds, YouTube's pictures and Wikidata.
	Music, API, Feeds, Thumbs, Wikidata string
	// Key is the YouTube Data API key ("": the shape check is skipped and
	// the videos stay unchecked). It is never logged.
	Key string
	// Limits holds the shared YouTube daily quota and pauses a source after
	// failures or a 429 (nil: no quota kept).
	Limits *imgstore.Limits
	// Artist reads the stored artist answer: name, home country and
	// MusicBrainz links (Known false while it is not stored).
	Artist func(ctx context.Context, mbid string) imgstore.Subject
	// Heatmap reads the most replayed start (nil: left for the app).
	Heatmap func(ctx context.Context, id string) (peak, duration float64, found bool, err error)
	// VideoJSON is yt-dlp's JSON of a video, for the storyboard check
	// (nil: unclear videos stay pending).
	VideoJSON func(ctx context.Context, id string) ([]byte, error)
	// Deep queues a video's storyboard check (DeepCheck); nil: the daily
	// pass finds it.
	Deep func(id string)
	// On reports whether a source is on (kill switch): youtube (all video
	// work), youtube_music, youtube_api, youtube_feed, youtube_frames,
	// youtube_storyboard, and wikidata (the channel claim).
	On func(name string) bool
	// Max videos kept per artist (default 12); HeatmapMax how many get a
	// start point read (default 5); Workers how many still checks run at
	// once (default 8).
	Max, HeatmapMax, Workers int
	Now                      func() time.Time
	Logf                     func(string, ...any)
}

// Service keeps answers, candidates, channels and verdicts in SQLite.
type Service struct {
	o  Options
	db *sql.DB

	mu        sync.Mutex
	ytdlpWait time.Time // yt-dlp reads are skipped until then
}

// Open opens (or creates) the store at path.
func Open(path string, o Options) (*Service, error) {
	if o.HTTP == nil {
		o.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	def := func(v *string, d string) {
		if *v == "" {
			*v = d
		}
		*v = strings.TrimRight(*v, "/")
	}
	def(&o.Music, "https://music.youtube.com")
	def(&o.API, "https://www.googleapis.com/youtube/v3")
	def(&o.Feeds, "https://www.youtube.com")
	def(&o.Thumbs, "https://i.ytimg.com")
	def(&o.Wikidata, "https://www.wikidata.org")
	if o.Max <= 0 {
		o.Max = 12
	}
	if o.HeatmapMax <= 0 {
		o.HeatmapMax = 5
	}
	if o.Workers <= 0 {
		o.Workers = 8
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	if o.Artist == nil {
		o.Artist = func(context.Context, string) imgstore.Subject { return imgstore.Subject{} }
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS videos(mbid TEXT PRIMARY KEY, body BLOB NOT NULL, fetched INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS artists(mbid TEXT PRIMARY KEY, source TEXT NOT NULL, page TEXT NOT NULL,
 listed INTEGER NOT NULL, feed INTEGER NOT NULL, judged INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS channels(mbid TEXT NOT NULL, channel TEXT NOT NULL, via TEXT NOT NULL, topic INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(mbid, channel));
CREATE TABLE IF NOT EXISTS cands(mbid TEXT NOT NULL, video TEXT NOT NULL, pos INTEGER NOT NULL, title TEXT NOT NULL, line TEXT NOT NULL,
 source TEXT NOT NULL, label TEXT NOT NULL, seconds INTEGER NOT NULL, short INTEGER NOT NULL, listed INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(mbid, video));
CREATE INDEX IF NOT EXISTS cands_video ON cands(video);
CREATE TABLE IF NOT EXISTS seen(channel TEXT NOT NULL, video TEXT NOT NULL, PRIMARY KEY(channel, video));
CREATE TABLE IF NOT EXISTS verdicts(video TEXT PRIMARY KEY, verdict TEXT NOT NULL, score REAL NOT NULL DEFAULT -1,
 shape INTEGER NOT NULL DEFAULT 0, seconds INTEGER NOT NULL DEFAULT 0, heat INTEGER NOT NULL DEFAULT 0, start REAL,
 checked INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS verdicts_kind ON verdicts(verdict, checked);`); err != nil {
		db.Close()
		return nil, err
	}
	// The start's judgement (start.go), added later: motion is its window's
	// local motion (NULL: not judged yet, -1: nothing to judge by), why the
	// reason it was chosen.
	for _, col := range []string{"motion REAL", "why TEXT NOT NULL DEFAULT ''"} {
		if _, err := db.Exec(`ALTER TABLE verdicts ADD COLUMN ` + col); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			return nil, err
		}
	}
	return &Service{o: o, db: db}, nil
}

// Close closes the store.
func (s *Service) Close() error { return s.db.Close() }

func (s *Service) on(name string) bool { return s.o.On == nil || s.o.On(name) }

func (s *Service) allow(ctx context.Context, src string) error {
	if s.o.Limits == nil {
		return nil
	}
	if err := s.o.Limits.Allow(ctx, src); err != nil {
		if errors.Is(err, imgstore.ErrSourcePaused) {
			return ErrPaused
		}
		return err
	}
	return nil
}

func (s *Service) done(src string, status int, err error) {
	if s.o.Limits != nil {
		s.o.Limits.Done(src, status, err)
	}
}

func (s *Service) backoff(src string, d time.Duration) {
	if s.o.Limits != nil {
		s.o.Limits.Backoff(src, d)
	}
}

// Get answers the stored JSON for an artist.
func (s *Service) Get(ctx context.Context, mbid string) ([]byte, bool) {
	var b []byte
	if err := s.db.QueryRowContext(ctx, `SELECT body FROM videos WHERE mbid=?`, mbid).Scan(&b); err != nil {
		return nil, false
	}
	return b, true
}

// Count answers how many artists have a stored answer.
func (s *Service) Count(ctx context.Context) (artists, withVideos int64) {
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(json_array_length(body,'$.videos')>0),0) FROM videos`).Scan(&artists, &withVideos)
	return
}

// StatusError is an upstream status other than 200.
type StatusError struct{ Status int }

func (e *StatusError) Error() string { return fmt.Sprintf("answered %d", e.Status) }

// cand is a candidate video of an artist.
type cand struct {
	ID, Title string
	// Line is the artists line YouTube Music shows ("" for channel uploads).
	Line   string
	Source string
	// Label is YouTube Music's label (MUSIC_VIDEO_TYPE_...).
	Label string
	// Seconds is the length YouTube Music shows (0: unknown).
	Seconds int
	// Short: linked as a Short in a channel feed.
	Short bool
}

// channel is an official channel of an artist.
type channel struct {
	ID, Via string
}

// artistRow is an artist's record: where its list came from (and the
// YouTube Music page), when it was gathered, its feeds read and its
// candidates last checked.
type artistRow struct {
	exists               bool
	source, page         string
	listed, feed, judged int64
}

func (s *Service) artist(ctx context.Context, mbid string) artistRow {
	var a artistRow
	a.exists = s.db.QueryRowContext(ctx, `SELECT source, page, listed, feed, judged FROM artists WHERE mbid=?`, mbid).
		Scan(&a.source, &a.page, &a.listed, &a.feed, &a.judged) == nil
	return a
}

// Collect brings an artist's list up to date (a picture-queue job): the
// candidates are gathered again when none are kept or they are
// RefreshEvery old, else the known channels' feeds are read when due; then
// the candidates that could make the list are checked and the list stored.
// Any failure (a 429, 5xx or timeout) is returned with nothing stored, so
// the job is tried again later.
func (s *Service) Collect(ctx context.Context, mbid string) error {
	if !s.on("youtube") {
		return nil
	}
	mbid = strings.ToLower(mbid)
	a := s.artist(ctx, mbid)
	now := s.o.Now()
	switch {
	case !a.exists || now.Sub(time.Unix(a.listed, 0)) >= RefreshEvery:
		subj := s.o.Artist(ctx, mbid)
		if !subj.Known {
			return ErrNoArtist
		}
		if err := s.gather(ctx, mbid, subj); err != nil {
			return err
		}
	case now.Sub(time.Unix(a.feed, 0)) >= FeedEvery && s.on("youtube_feed"):
		if err := s.readFeeds(ctx, mbid, a); err != nil {
			return err
		}
	}
	return s.rebuild(ctx, mbid)
}

// gather collects an artist's candidates in the decided order and keeps
// them with the artist's official channels (for the daily feed): YouTube
// Music's artist page; only when YouTube Music has no such artist, the
// uploads of the Wikidata channel, else of a MusicBrainz channel link.
func (s *Service) gather(ctx context.Context, mbid string, subj imgstore.Subject) error {
	var cands []cand
	source, page := "", ""
	if subj.Name != "" && s.on("youtube_music") {
		id, err := s.findArtist(ctx, subj.Name, subj.Country)
		if err != nil {
			return err
		}
		if id != "" {
			if cands, err = s.artistVideos(ctx, id); err != nil {
				return err
			}
			source, page = SourceYouTubeMusic, id
		}
	}
	chans, err := s.officialChannels(ctx, subj.Links)
	if err != nil {
		if source == "" {
			return err
		}
		// Here the channels only serve the daily feed: they are looked for
		// again at the next gathering.
		s.o.Logf("herovideo: %s channels: %v", mbid, err)
	}
	topics := map[string]bool{}
	if source == "" {
		if subj.Name != "" && !s.on("youtube_music") {
			// YouTube Music could not be asked: a miss here is not a real
			// "no videos" unless a backup finds some.
			err = ErrPaused
		}
		for _, ch := range chans {
			ups, topic, checked, uerr := s.uploads(ctx, ch)
			switch {
			case uerr != nil:
				return uerr
			case topic:
				topics[ch.ID] = true
			case !checked:
				err = ErrPaused
			case len(ups) > 0:
				cands, source = ups, ch.Via
			}
			if source != "" {
				break
			}
		}
		if source == "" && errors.Is(err, ErrPaused) {
			return ErrPaused
		}
	}
	now := s.o.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO artists(mbid, source, page, listed, feed) VALUES(?,?,?,?,?)
ON CONFLICT(mbid) DO UPDATE SET source=excluded.source, page=excluded.page, listed=excluded.listed, feed=excluded.feed`, mbid, source, page, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM channels WHERE mbid=?`, mbid); err != nil {
		return err
	}
	for _, ch := range chans {
		if _, err := tx.ExecContext(ctx, `INSERT INTO channels(mbid, channel, via, topic) VALUES(?,?,?,?)`, mbid, ch.ID, ch.Via, topics[ch.ID]); err != nil {
			return err
		}
	}
	if err := saveCands(ctx, tx, mbid, cands); err != nil {
		return err
	}
	return tx.Commit()
}

// saveCands replaces an artist's candidates, in order.
func saveCands(ctx context.Context, tx *sql.Tx, mbid string, cands []cand) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM cands WHERE mbid=?`, mbid); err != nil {
		return err
	}
	for i, c := range cands {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cands(mbid, video, pos, title, line, source, label, seconds, short) VALUES(?,?,?,?,?,?,?,?,?)`,
			mbid, c.ID, i, c.Title, c.Line, c.Source, c.Label, c.Seconds, c.Short); err != nil {
			return err
		}
	}
	return nil
}

// officialChannels answers the artist's official YouTube channels: the
// Wikidata item's P2397 first, then MusicBrainz channel links, each once.
// A Wikidata failure is returned with the MusicBrainz channels.
func (s *Service) officialChannels(ctx context.Context, links []string) ([]channel, error) {
	var out []channel
	seen := map[string]bool{}
	add := func(id, via string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, channel{ID: id, Via: via})
		}
	}
	var err error
	if s.on("wikidata") {
		for _, l := range links {
			if m := wikidataLink.FindStringSubmatch(l); m != nil {
				var ids []string
				ids, err = s.wikidataChannels(ctx, m[1])
				for _, id := range ids {
					add(id, SourceWikidata)
				}
				break
			}
		}
	}
	for _, l := range links {
		if m := channelLink.FindStringSubmatch(l); m != nil {
			add(m[1], SourceMusicBrainz)
		}
	}
	return out, err
}

// uploads lists a channel's newest uploads: its feed (free) and, when that
// holds fewer than a full list's worth or cannot be read, one playlistItems
// call (1 point). topic reports a " - Topic" channel (YouTube's automatic
// one for an artist's audio), whose uploads are left out. When neither
// answered (a failure, switched off, no key) checked is false, or the
// feed's failure is returned.
func (s *Service) uploads(ctx context.Context, ch channel) (ups []cand, topic, checked bool, err error) {
	var feedErr error
	if s.on("youtube_feed") {
		f, err := s.readFeed(ctx, ch.ID, ch.Via)
		switch {
		case err != nil:
			feedErr = err
		case f.topic():
			return nil, true, true, nil
		default:
			if err := s.markSeen(ctx, ch.ID, f.Entries); err != nil {
				return nil, false, false, err
			}
			ups, checked = f.Entries, true
		}
	}
	usable := 0
	for _, c := range ups {
		if !c.Short && !badTitle.MatchString(c.Title) {
			usable++
		}
	}
	if usable < s.o.Max {
		more, err := s.playlistUploads(ctx, ch.ID, ch.Via)
		switch {
		case errors.Is(err, errSkipped):
		case err != nil:
			return nil, false, false, err
		default:
			checked = true
			have := map[string]bool{}
			for _, c := range ups {
				have[c.ID] = true
			}
			for _, c := range more {
				if !have[c.ID] {
					ups = append(ups, c)
				}
			}
		}
	}
	if !checked && feedErr != nil {
		return nil, false, false, feedErr
	}
	return ups, false, checked, nil
}

// markSeen keeps a channel's feed entries, so the next read knows which
// uploads are new.
func (s *Service) markSeen(ctx context.Context, ch string, entries []cand) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM seen WHERE channel=?`, ch); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO seen(channel, video) VALUES(?,?)`, ch, e.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// readFeeds reads the artist's channels' feeds (free). A new upload that
// could be a music video (no Short, no audio title) means: for an artist
// found on YouTube Music, its page is read again (its order and labels);
// otherwise the uploads join the candidates first, newest first. A
// " - Topic" channel is marked and not read again; a feed that cannot be
// read now is read again the next day.
func (s *Service) readFeeds(ctx context.Context, mbid string, a artistRow) error {
	if a.source == SourceYouTubeMusic && !s.on("youtube_music") {
		return nil // read when YouTube Music is on again
	}
	rows, err := s.db.QueryContext(ctx, `SELECT channel, via FROM channels WHERE mbid=? AND topic=0`, mbid)
	if err != nil {
		return err
	}
	var chans []channel
	for rows.Next() {
		var ch channel
		if rows.Scan(&ch.ID, &ch.Via) == nil {
			chans = append(chans, ch)
		}
	}
	rows.Close()
	known := map[string]bool{}
	for _, c := range s.cands(ctx, mbid) {
		known[c.ID] = true
	}
	var fresh []cand
	feeds := map[string][]cand{}
	for _, ch := range chans {
		f, err := s.readFeed(ctx, ch.ID, ch.Via)
		if err != nil {
			continue // read again the next day; the list stays as it is
		}
		if f.topic() {
			if _, err := s.db.ExecContext(ctx, `UPDATE channels SET topic=1 WHERE mbid=? AND channel=?`, mbid, ch.ID); err != nil {
				return err
			}
			continue
		}
		seen := map[string]bool{}
		rows, err := s.db.QueryContext(ctx, `SELECT video FROM seen WHERE channel=?`, ch.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var v string
			if rows.Scan(&v) == nil {
				seen[v] = true
			}
		}
		rows.Close()
		for _, e := range f.Entries {
			if !known[e.ID] && !seen[e.ID] && !e.Short && !badTitle.MatchString(e.Title) {
				known[e.ID] = true
				fresh = append(fresh, e)
			}
		}
		feeds[ch.ID] = f.Entries
	}
	// The page is read before the transaction: the store has one connection,
	// which answers must not wait on.
	var page []cand
	reread := len(fresh) > 0 && a.source == SourceYouTubeMusic
	if reread {
		if page, err = s.artistVideos(ctx, a.page); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if len(fresh) > 0 {
		if reread {
			if err := saveCands(ctx, tx, mbid, page); err != nil {
				return err
			}
		} else {
			var low int
			_ = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(pos),0) FROM cands WHERE mbid=?`, mbid).Scan(&low)
			for i, c := range fresh {
				if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO cands(mbid, video, pos, title, line, source, label, seconds, short) VALUES(?,?,?,?,?,?,?,?,?)`,
					mbid, c.ID, low-len(fresh)+i, c.Title, "", c.Source, "", 0, false); err != nil {
					return err
				}
			}
			if a.source == "" {
				if _, err := tx.ExecContext(ctx, `UPDATE artists SET source=? WHERE mbid=?`, fresh[0].Source, mbid); err != nil {
					return err
				}
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE artists SET feed=? WHERE mbid=?`, s.o.Now().Unix(), mbid); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for ch, entries := range feeds {
		if err := s.markSeen(ctx, ch, entries); err != nil {
			return err
		}
	}
	return nil
}

// cands answers an artist's kept candidates in order.
func (s *Service) cands(ctx context.Context, mbid string) []cand {
	rows, err := s.db.QueryContext(ctx, `SELECT video, title, line, source, label, seconds, short FROM cands WHERE mbid=? ORDER BY pos`, mbid)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []cand
	for rows.Next() {
		var c cand
		if rows.Scan(&c.ID, &c.Title, &c.Line, &c.Source, &c.Label, &c.Seconds, &c.Short) == nil {
			out = append(out, c)
		}
	}
	return out
}

// guest marks a guest credit in a title.
var guest = regexp.MustCompile(`(?i)(?:^|[\s(\[])(?:feat\.?|ft\.?|featuring)\s`)

// mainAct reports a video where the artist is the main act: its artists
// line (YouTube Music), or the artist part of its title ("Artist - Song"),
// starts with the artist's name; a title naming the artist only after
// "feat." is a guest spot; a title not naming the artist (an upload on its
// own channel) counts as its own.
func mainAct(c cand, name string) bool {
	n := imgstore.FoldName(name)
	if n == "" {
		return true
	}
	first := func(s string) bool {
		f := imgstore.FoldName(s)
		return f == n || strings.HasPrefix(f, n+" ")
	}
	named := func(s string) bool { return strings.Contains(" "+imgstore.FoldName(s)+" ", " "+n+" ") }
	if c.Line != "" {
		return first(c.Line)
	}
	if line, _, ok := strings.Cut(c.Title, " - "); ok && named(line) {
		return first(line)
	}
	if m := guest.FindStringIndex(c.Title); m != nil && named(c.Title[m[0]:]) {
		return named(c.Title[:m[0]]) && first(c.Title)
	}
	return true
}

// check is a video's kept verdict.
type check struct {
	verdict string // "" while the still check has not run
	score   float64
	shape   bool // shape and length checked with the Data API
	seconds int
	heat    bool // most replayed read
	start   *float64
	motion  *float64 // the start window's local motion (nil: not judged; -1: nothing to judge by)
	why     string   // why the start was chosen (start.go)
}

func dropped(v string) bool {
	switch v {
	case "", VerdictVideo, VerdictPending:
		return false
	}
	return true
}

func (s *Service) checks(ctx context.Context, cands []cand) map[string]*check {
	out := make(map[string]*check, len(cands))
	for _, c := range cands {
		ck := &check{score: -1} // -1 until the still check scores it
		var start, motion sql.NullFloat64
		if s.db.QueryRowContext(ctx, `SELECT verdict, score, shape, seconds, heat, start, motion, why FROM verdicts WHERE video=?`, c.ID).
			Scan(&ck.verdict, &ck.score, &ck.shape, &ck.seconds, &ck.heat, &start, &motion, &ck.why) == nil {
			if start.Valid {
				ck.start = &start.Float64
			}
			if motion.Valid {
				ck.motion = &motion.Float64
			}
		}
		out[c.ID] = ck
	}
	return out
}

// keep stores a video's verdict (kept for good; a pending one is replaced
// by the storyboard check's).
func (s *Service) keep(ctx context.Context, id string, ck *check) error {
	var start, motion any
	if ck.start != nil {
		start = *ck.start
	}
	if ck.motion != nil {
		motion = *ck.motion
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO verdicts(video, verdict, score, shape, seconds, heat, start, motion, why, checked) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		id, ck.verdict, ck.score, ck.shape, ck.seconds, ck.heat, start, motion, ck.why, s.o.Now().Unix())
	return err
}

// rebuild checks the candidates that could make the artist's list and
// stores it: main-act videos first, each group in the source's order
// (YouTube Music's, or the channel's newest first), at most Max.
func (s *Service) rebuild(ctx context.Context, mbid string) error {
	cands := s.cands(ctx, mbid)
	name := s.o.Artist(ctx, mbid).Name
	main := make(map[string]bool, len(cands))
	for _, c := range cands {
		main[c.ID] = mainAct(c, name)
	}
	sort.SliceStable(cands, func(i, j int) bool { return main[cands[i].ID] && !main[cands[j].ID] })
	checks := s.checks(ctx, cands)
	if err := s.judge(ctx, cands, checks); err != nil {
		return err
	}
	return s.store(ctx, mbid, cands, checks)
}

// judge gives the candidates that could make the list their verdicts:
// first the free rules (YouTube Music's label, the title, a Short in a
// feed, a length YouTube Music shows), then, in list order, the shape and
// length check (one videos call per 50, with a key and quota left) and the
// still check, until the list is full. A check that cannot run now leaves
// its videos unchecked (listed, checked later); a failure is returned.
func (s *Service) judge(ctx context.Context, cands []cand, checks map[string]*check) error {
	for _, c := range cands {
		ck := checks[c.ID]
		if ck.seconds == 0 && c.Seconds > 0 {
			ck.seconds = c.Seconds
		}
		if ck.verdict != "" {
			continue
		}
		switch {
		case c.Label == labelUserUpload:
			ck.verdict = VerdictUser
		case badTitle.MatchString(c.Title):
			ck.verdict = VerdictTitle
		case c.Short:
			ck.verdict = VerdictTall
		case c.Seconds > 0 && c.Seconds <= shortMax:
			ck.verdict = VerdictShort
		default:
			continue
		}
		if err := s.keep(ctx, c.ID, ck); err != nil {
			return err
		}
	}
	tried := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var win []cand
		for _, c := range cands {
			if len(win) >= s.o.Max {
				break
			}
			if !dropped(checks[c.ID].verdict) {
				win = append(win, c)
			}
		}
		// Shape and length: the list's videos not checked yet, the call
		// filled up to 50 with the next ones (it costs the same).
		var ids []string
		need := func(c cand) bool {
			ck := checks[c.ID]
			return len(ids) < 50 && !ck.shape && !tried[c.ID] && !dropped(ck.verdict)
		}
		for _, c := range win {
			if need(c) {
				tried[c.ID] = true
				ids = append(ids, c.ID)
			}
		}
		for _, c := range cands {
			if len(ids) > 0 && need(c) {
				tried[c.ID] = true
				ids = append(ids, c.ID)
			}
		}
		if len(ids) > 0 {
			got, err := s.shapes(ctx, ids)
			switch {
			case errors.Is(err, errSkipped):
			case err != nil:
				return err
			default:
				cut := false
				for _, id := range ids {
					ck := checks[id]
					sh, ok := got[id]
					if !ok {
						ck.verdict = VerdictGone
					} else if v, secs, known := sh.verdict(); known {
						ck.shape, ck.seconds = true, secs
						if v != "" {
							ck.verdict = v
						}
					}
					cut = cut || dropped(ck.verdict)
					if err := s.keep(ctx, id, ck); err != nil {
						return err
					}
				}
				if cut {
					continue
				}
			}
		}
		if !s.on("youtube_frames") {
			return nil
		}
		var open []string
		for _, c := range win {
			if checks[c.ID].verdict == "" {
				open = append(open, c.ID)
			}
		}
		if len(open) == 0 {
			return nil
		}
		if err := s.stills(ctx, open, checks); err != nil {
			return err
		}
	}
}

// stills runs the still check on videos, Workers at once: a still is
// dropped, a video kept, an unclear one (or one whose frames say nothing)
// kept and queued for the storyboard check. Verdicts reached are kept even
// when another check fails.
func (s *Service) stills(ctx context.Context, ids []string, checks map[string]*check) error {
	scores, telling, errs := make([]float64, len(ids)), make([]bool, len(ids)), make([]error, len(ids))
	sem := make(chan struct{}, s.o.Workers)
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			scores[i], telling[i], errs[i] = s.frames(ctx, id)
		}()
	}
	wg.Wait()
	var first error
	for i, id := range ids {
		ck := checks[id]
		switch err := errs[i]; {
		case errors.Is(err, errGone):
			ck.verdict = VerdictGone
		case err != nil:
			if first == nil {
				first = err
			}
			continue
		case !telling[i]:
			ck.verdict, ck.score = VerdictPending, scores[i]
		default:
			ck.verdict, ck.score = verdictOf(scores[i]), scores[i]
		}
		if err := s.keep(ctx, id, ck); err != nil {
			return err
		}
		if ck.verdict == VerdictPending && s.o.Deep != nil && s.on("youtube_storyboard") {
			s.o.Deep(id)
		}
	}
	return first
}

// store keeps the artist's list: the candidates not dropped, in order, at
// most Max, with the start read for the first HeatmapMax (once per video).
func (s *Service) store(ctx context.Context, mbid string, cands []cand, checks map[string]*check) error {
	vids := []Video{}
	var listed []cand
	for _, c := range cands {
		if len(vids) >= s.o.Max {
			break
		}
		ck := checks[c.ID]
		if dropped(ck.verdict) {
			continue
		}
		if s.startReadable() && len(vids) < s.o.HeatmapMax && !ck.heat && !s.ytdlpPaused() && ctx.Err() == nil {
			err := s.readStart(ctx, c.ID, ck)
			switch {
			case err != nil && ctx.Err() == nil:
				// YouTube's bot check blocks datacenter addresses at times:
				// stop trying for a while so no job is slowed by failing reads.
				s.pauseYTDLP()
			case err == nil:
				if err := s.keep(ctx, c.ID, ck); err != nil {
					return err
				}
			}
		}
		vids = append(vids, Video{YouTubeID: c.ID, Title: c.Title, Source: c.Source, Start: ck.start, Duration: float64(ck.seconds)})
		listed = append(listed, c)
	}
	now := s.o.Now()
	body, _ := json.Marshal(Answer{ArtistMBID: mbid, Videos: vids, FetchedAt: now.UTC()})
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE cands SET listed=0 WHERE mbid=?`, mbid); err != nil {
		return err
	}
	for _, c := range listed {
		if _, err := tx.ExecContext(ctx, `UPDATE cands SET listed=1 WHERE mbid=? AND video=?`, mbid, c.ID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE artists SET judged=? WHERE mbid=?`, now.Unix(), mbid); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO videos(mbid, body, fetched) VALUES(?,?,?)`, mbid, body, now.Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// boardOn reports whether starts are judged on the storyboard.
func (s *Service) boardOn() bool { return s.o.VideoJSON != nil && s.on("youtube_storyboard") }

// startReadable reports whether a start can be read at all.
func (s *Service) startReadable() bool { return s.o.Heatmap != nil || s.boardOn() }

// readStart reads a video's start into ck with one yt-dlp read: judged on
// the storyboard when it is on (start.go), else the most replayed peak as
// it is. A sheet that cannot be read keeps the peak unjudged (judged again
// a day later); err is yt-dlp's failure, with ck unchanged.
func (s *Service) readStart(ctx context.Context, id string, ck *check) error {
	var (
		p         Pick
		dur, peak float64
		found     bool
		sheetErr  error
		err       error
		judge     = s.boardOn()
	)
	if judge {
		p, dur, peak, found, sheetErr, err = s.startOf(ctx, id)
	} else {
		peak, dur, found, err = s.o.Heatmap(ctx, id)
	}
	if err != nil {
		return err
	}
	ck.heat = true
	if ck.seconds == 0 {
		ck.seconds = int(dur + 0.5)
	}
	ck.start, ck.motion, ck.why = nil, nil, ""
	if found {
		ck.start = &peak
	}
	switch {
	case !judge:
		return nil
	case sheetErr == nil:
		start, motion := p.Start, p.Motion
		ck.start, ck.motion, ck.why = &start, &motion, p.Why
	case p.Why == WhyNoBoard:
		none := -1.0
		ck.motion, ck.why = &none, WhyNoBoard
	default:
		ck.why = WhyUnjudged
	}
	start, motion := -1.0, -1.0
	if ck.start != nil {
		start = *ck.start
	}
	if ck.motion != nil {
		motion = *ck.motion
	}
	s.o.Logf("herovideo: %s start %.2fs (%s; peak %.2fs found %v, motion %.1f, sheets %v)", id, start, ck.why, peak, found, motion, sheetErr)
	return nil
}

func (s *Service) ytdlpPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.o.Now().Before(s.ytdlpWait)
}

func (s *Service) pauseYTDLP() {
	s.mu.Lock()
	s.ytdlpWait = s.o.Now().Add(30 * time.Minute)
	s.mu.Unlock()
}

// DeepCheck runs the storyboard check on an unclear video (a picture-queue
// job) and keeps its verdict; a still is taken off the lists that show
// it. A check that fails is tried again by the daily pass a day later.
func (s *Service) DeepCheck(ctx context.Context, id string) error {
	if !s.on("youtube") || !s.on("youtube_storyboard") || s.o.VideoJSON == nil || !videoID.MatchString(id) || s.ytdlpPaused() {
		return nil
	}
	cks := s.checks(ctx, []cand{{ID: id}})
	ck := cks[id]
	if ck.verdict == VerdictVideo && ck.heat && ck.motion == nil {
		return s.rejudgeStart(ctx, id, ck)
	}
	if ck.verdict != VerdictPending {
		return nil
	}
	v, err := s.storyboard(ctx, id)
	if err != nil {
		var se *StatusError
		if !errors.As(err, &se) {
			s.pauseYTDLP()
		}
		return s.keep(ctx, id, ck) // checked again a day later
	}
	ck.verdict = v
	if err := s.keep(ctx, id, ck); err != nil {
		return err
	}
	if v != VerdictStill {
		return nil
	}
	return s.rebuildListing(ctx, id)
}

// rejudgeStart judges a listed video's stored start on its storyboard (a
// start read before the judgement, or one whose sheets failed): one yt-dlp
// read and the sheets, nothing else. A changed start is written into the
// lists that show the video; a failure is tried again a day later.
func (s *Service) rejudgeStart(ctx context.Context, id string, ck *check) error {
	old := ck.start
	if err := s.readStart(ctx, id, ck); err != nil {
		var se *StatusError
		if !errors.As(err, &se) {
			s.pauseYTDLP()
		}
		ck.why = WhyUnjudged
		return s.keep(ctx, id, ck) // judged again a day later
	}
	if err := s.keep(ctx, id, ck); err != nil {
		return err
	}
	if old == nil && ck.start == nil || old != nil && ck.start != nil && *old == *ck.start {
		return nil
	}
	return s.rebuildListing(ctx, id)
}

// rebuildListing stores again the lists that show a video.
func (s *Service) rebuildListing(ctx context.Context, id string) error {
	rows, err := s.db.QueryContext(ctx, `SELECT mbid FROM cands WHERE video=? AND listed=1`, id)
	if err != nil {
		return err
	}
	var artists []string
	for rows.Next() {
		var m string
		if rows.Scan(&m) == nil {
			artists = append(artists, m)
		}
	}
	rows.Close()
	for _, m := range artists {
		if err := s.rebuild(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// Due answers the background work: artists whose candidates are
// RefreshEvery old (or whose answer was built before this record was kept),
// artists whose channels' feeds are FeedEvery old, and artists with listed
// videos still unchecked once their check can run (a day apart); and
// unclear videos whose storyboard check is a day old, then listed videos
// whose start was never judged on the storyboard (a failed judgement a day
// later). At most limit of each.
func (s *Service) Due(ctx context.Context, limit int) (artists, deep []string) {
	if !s.on("youtube") {
		return nil, nil
	}
	now := s.o.Now()
	scan := func(q string, args ...any) []string {
		rows, err := s.db.QueryContext(ctx, q, args...)
		if err != nil {
			return nil
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var m string
			if rows.Scan(&m) == nil {
				out = append(out, m)
			}
		}
		return out
	}
	seen := map[string]bool{}
	add := func(ms []string) {
		for _, m := range ms {
			if !seen[m] && len(artists) < limit {
				seen[m] = true
				artists = append(artists, m)
			}
		}
	}
	add(scan(`SELECT v.mbid FROM videos v LEFT JOIN artists a ON a.mbid=v.mbid WHERE a.mbid IS NULL OR a.listed<=? ORDER BY COALESCE(a.listed,0) LIMIT ?`,
		now.Add(-RefreshEvery).Unix(), limit))
	// Gathered but never stored (the checks failed until the job died).
	add(scan(`SELECT a.mbid FROM artists a WHERE NOT EXISTS(SELECT 1 FROM videos v WHERE v.mbid=a.mbid) LIMIT ?`, limit))
	if s.on("youtube_feed") {
		add(scan(`SELECT a.mbid FROM artists a WHERE a.feed<=? AND EXISTS(SELECT 1 FROM channels c WHERE c.mbid=a.mbid AND c.topic=0) ORDER BY a.feed LIMIT ?`,
			now.Add(-FeedEvery).Unix(), limit))
	}
	var open []string
	if s.apiOn() && (s.o.Limits == nil || s.o.Limits.Status()["youtube"].State != "quota") {
		open = append(open, "v.shape=0")
	}
	if s.on("youtube_frames") {
		open = append(open, "v.verdict=''")
	}
	if len(open) > 0 {
		add(scan(`SELECT DISTINCT c.mbid FROM cands c JOIN artists a ON a.mbid=c.mbid LEFT JOIN verdicts v ON v.video=c.video
WHERE c.listed=1 AND a.judged<=? AND (v.video IS NULL OR `+strings.Join(open, " OR ")+`) LIMIT ?`, now.Add(-recheckEvery).Unix(), limit))
	}
	// While yt-dlp is paused a check does nothing, so none are queued;
	// the next pass picks them up.
	if s.o.VideoJSON != nil && s.on("youtube_storyboard") && !s.ytdlpPaused() {
		dayAgo := now.Add(-recheckEvery).Unix()
		deep = scan(`SELECT video FROM verdicts WHERE verdict=? AND checked<=? ORDER BY checked LIMIT ?`, VerdictPending, dayAgo, limit)
		if n := limit - len(deep); n > 0 {
			deep = append(deep, scan(`SELECT v.video FROM verdicts v WHERE v.verdict=? AND v.heat=1 AND v.motion IS NULL AND (v.why='' OR v.checked<=?)
 AND EXISTS(SELECT 1 FROM cands c WHERE c.video=v.video AND c.listed=1) ORDER BY v.checked LIMIT ?`, VerdictVideo, dayAgo, n)...)
		}
	}
	return artists, deep
}

// Stats are the numbers for /v1/status: artists per source, kept verdicts
// per kind, and listed videos still unchecked.
func (s *Service) Stats(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	group := func(prefix, q string) {
		rows, err := s.db.QueryContext(ctx, q)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			var n int64
			if rows.Scan(&k, &n) == nil {
				if k == "" {
					k = "none"
				}
				out[prefix+k] = n
			}
		}
	}
	group("source_", `SELECT source, COUNT(*) FROM artists GROUP BY source`)
	group("verdict_", `SELECT verdict, COUNT(*) FROM verdicts GROUP BY verdict`)
	var unchecked int64
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cands c LEFT JOIN verdicts v ON v.video=c.video WHERE c.listed=1 AND (v.video IS NULL OR v.shape=0)`).Scan(&unchecked)
	out["listed_shape_unchecked"] = unchecked
	var unjudged int64
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM verdicts WHERE verdict=? AND heat=1 AND motion IS NULL`, VerdictVideo).Scan(&unjudged)
	out["start_unjudged"] = unjudged
	return out
}

// Package musicdata answers an artist's top songs and, when MusicBrainz
// has none, discography, an album's details and an artist's similar
// artists on Apple Music, from the official Spotify Web API, the Apple Music API
// and Deezer, each read
// through the picture chains' resolver (its pace, limits, kill switch and
// kept matches). Only this server calls them, and every answer is kept in
// its own SQLite file, so the cost never grows with installs.
//
// Each element has its sources in a fixed order, and a source answers only
// when every source before it has none or failed. A 429, a 5xx or a
// timeout is a failure, never "none": nothing is kept as missing then, and
// an answer a later source gave is kept only an hour, so the better source
// is asked again soon.
package musicdata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	_ "modernc.org/sqlite"
)

var (
	// ErrNone: no source has the thing (kept a day).
	ErrNone = errors.New("musicdata: none")
	// ErrLater: a source failed or the stored MusicBrainz answer is not
	// there yet, and no later source answered; ask again later.
	ErrLater = errors.New("musicdata: ask again later")
)

// Sources is what the answers are read through (imgstore.Resolver).
type Sources interface {
	AppleJSON(ctx context.Context, path string, q url.Values, out any) (bool, error)
	SpotifyJSON(ctx context.Context, path string, out any) (bool, error)
	DeezerJSON(ctx context.Context, path string, q url.Values, out any) (bool, error)
	AppleArtist(ctx context.Context, mbid string) (store, id string, err error)
	AppleAlbumID(ctx context.Context, mbid string) (store, id string, err error)
	SpotifyArtist(ctx context.Context, mbid string) (string, error)
	DeezerArtist(ctx context.Context, mbid string) (string, error)
	Subject(ctx context.Context, kind, mbid string) imgstore.Subject
}

// Options configure the service.
type Options struct {
	Sources Sources
	// Stored reads a stored MusicBrainz answer ("release-group" or
	// "release"); no call is made. nil: none stored.
	Stored func(ctx context.Context, kind, mbid string) ([]byte, bool)
	Now    func() time.Time
}

// Kept lifetimes.
const (
	keepTopSongs    = 7 * 24 * time.Hour
	keepDiscography = 7 * 24 * time.Hour
	keepAlbum       = 30 * 24 * time.Hour
	keepNone        = 24 * time.Hour
	// keepAfterFail is an answer a later source gave while an earlier one
	// failed, or one made without a stored MusicBrainz answer.
	keepAfterFail = time.Hour
)

// Service keeps the answers.
type Service struct {
	db *sql.DB
	o  Options
}

// Open opens (or creates) the store at path.
func Open(path string, o Options) (*Service, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS answers (k TEXT PRIMARY KEY, body BLOB, until INTEGER NOT NULL) WITHOUT ROWID;`); err != nil {
		db.Close()
		return nil, err
	}
	if err := srctag.Stamp(db, AnswersTag); err != nil {
		db.Close()
		return nil, err
	}
	return &Service{db: db, o: o}, nil
}

// AnswersTag is how the answers table names its sources: top songs carry
// the source that gave them, Deezer answers are Deezer's, discographies
// and album details are MusicBrainz's.
var AnswersTag = srctag.Spec{Table: "answers", Key: []string{"k"}, Body: "body",
	Src: "CASE WHEN @.k LIKE 'top:%' THEN COALESCE(json_extract(CAST(@.body AS TEXT),'$.source'),'" + srctag.Untagged + "') WHEN @.k LIKE 'dz-%' THEN 'deezer' WHEN @.k LIKE 'disc:%' OR @.k LIKE 'album:%' THEN 'musicbrainz' ELSE '" + srctag.Untagged + "' END",
	// An empty answer ("the source has none") names its source the same way.
	Empty: "CASE WHEN @.k LIKE 'top:%' THEN '" + srctag.Untagged + "' WHEN @.k LIKE 'dz-%' THEN 'deezer' WHEN @.k LIKE 'disc:%' OR @.k LIKE 'album:%' THEN 'musicbrainz' ELSE '" + srctag.Untagged + "' END"}

// Retag fetches again the stored answers that have no fetch time or no
// known source (rows kept before tagging), one every pause, so each takes
// the tag of the source that answers. A "none" answer (empty body) is
// asked only while it has no fetch time: a "none" top-songs answer is
// always untagged. The old answer stays and is served
// until the new one is kept; an answer that fails stays untagged for the
// next pass. It answers how many it asked for.
func (s *Service) Retag(ctx context.Context, pause time.Duration) int {
	rows, err := s.db.QueryContext(ctx, `SELECT k FROM answers WHERE fetched_at<=0 OR (src IN ('', 'unknown-pre-tag') AND length(body)>0)`)
	if err != nil {
		return 0
	}
	var keys []string
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil {
			keys = append(keys, k)
		}
	}
	rows.Close()
	n := 0
	for _, k := range keys {
		if ctx.Err() != nil {
			break
		}
		kind, id, _ := strings.Cut(k, ":")
		// Expire it so the next read asks the source; a failed ask keeps the old answer.
		if _, err := s.db.ExecContext(ctx, `UPDATE answers SET until=0 WHERE k=?`, k); err != nil {
			continue
		}
		switch kind {
		case "top":
			_, _ = s.TopSongs(ctx, id)
		case "disc":
			_, _ = s.Discography(ctx, id)
		case "album":
			s.AlbumDetails(ctx, []string{id})
		case "dz-artist":
			_, _ = s.DeezerArtist(ctx, id)
		case "dz-album":
			if n, err := strconv.ParseInt(id, 10, 64); err == nil {
				_, _ = s.DeezerAlbum(ctx, n)
			}
		}
		n++
		select {
		case <-ctx.Done():
		case <-time.After(pause):
		}
	}
	return n
}

// Expire makes a kept answer due: the next read asks the sources again.
func (s *Service) Expire(ctx context.Context, k string) {
	_, _ = s.db.ExecContext(ctx, `UPDATE answers SET until=0 WHERE k=?`, k)
}

// Close closes the store.
func (s *Service) Close() error { return s.db.Close() }

// cached reads a kept answer into out; found is false when none is kept
// (or it ran out), none is true for a kept "no source has it".
func (s *Service) cached(ctx context.Context, k string, out any) (found, none bool) {
	var body []byte
	var until int64
	if s.db.QueryRowContext(ctx, `SELECT body, until FROM answers WHERE k = ?`, k).Scan(&body, &until) != nil || s.o.Now().Unix() >= until {
		return false, false
	}
	if body == nil {
		return true, true
	}
	return json.Unmarshal(body, out) == nil, false
}

// keep stores an answer (v nil: none) for d, even when the asking
// request has run out of time.
func (s *Service) keep(k string, v any, d time.Duration) {
	var body []byte
	if v != nil {
		var err error
		if body, err = json.Marshal(v); err != nil {
			return
		}
	}
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO answers (k, body, until) VALUES (?, ?, ?)`, k, body, s.o.Now().Add(d).Unix())
}

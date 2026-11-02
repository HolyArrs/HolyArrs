package imgstore

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

// Source ids: the artist or album each source matched for a MusicBrainz id
// (Apple Music's store and id, Spotify's and Deezer's id), kept so the
// music data routes (similar artists, top songs, album details,
// popularity) reuse the picture chains' matches instead of searching
// again. A miss is kept too (empty id) when the source answered and had
// none; a failed call keeps nothing.

// Kept-id lifetimes: a match is trusted for 90 days, a miss for 7.
const (
	idFresh = 90 * 24 * time.Hour
	idMiss  = 7 * 24 * time.Hour
)

// KeepID keeps a source's match ("" ext: the source has none). The tag
// names the source, when the match was fetched and through which address;
// a tag without a source keeps nothing.
func (s *Store) KeepID(kind, mbid string, t srctag.Tag, store, ext string) {
	if t.Check() != nil {
		return
	}
	now := s.o.Now()
	_, _ = s.db.Exec(`INSERT INTO source_ids(kind, mbid, source, store, ext, at, fetched_at, via_ip) VALUES(?,?,?,?,?,?,?,?)
ON CONFLICT(kind, mbid, source) DO UPDATE SET store=excluded.store, ext=excluded.ext, at=excluded.at, fetched_at=excluded.fetched_at, via_ip=excluded.via_ip,
 found_by=CASE WHEN source_ids.ext=excluded.ext THEN source_ids.found_by ELSE '' END`,
		kind, mbid, t.Source, store, ext, now.Unix(), t.At(now).Unix(), t.ViaIP)
}

// LinkedSpotify answers the Spotify id kept for an artist when it came
// from the artist's MusicBrainz link (never one a name search found).
func (s *Store) LinkedSpotify(mbid string) (string, bool) {
	how, ext, _, ok := s.KeptID(KindArtist, mbid, "spotify")
	return ext, ok && how == "link" && ext != ""
}

// LinkedArtist answers the artist a Spotify id was kept for from the
// artist's MusicBrainz link.
func (s *Store) LinkedArtist(spotify string) (string, bool) {
	var mbid string
	if s.db.QueryRow(`SELECT mbid FROM source_ids WHERE source = 'spotify' AND kind = ? AND ext = ? AND store = 'link' LIMIT 1`, KindArtist, spotify).Scan(&mbid) != nil {
		return "", false
	}
	return mbid, true
}

// KeptID answers a kept match (ext "" for a kept miss) and when it was made.
func (s *Store) KeptID(kind, mbid, source string) (store, ext string, at time.Time, ok bool) {
	var t int64
	if s.db.QueryRow(`SELECT store, ext, at FROM source_ids WHERE kind=? AND mbid=? AND source=?`, kind, mbid, source).Scan(&store, &ext, &t) != nil {
		return "", "", time.Time{}, false
	}
	return store, ext, time.Unix(t, 0), true
}

// SharedID says another MusicBrainz id of the same kind keeps the same
// source id (two artists of one name matched to one Apple artist).
func (s *Store) SharedID(kind, source, ext, except string) bool {
	var n int
	if ext == "" || s.db.QueryRow(`SELECT COUNT(*) FROM source_ids WHERE kind=? AND source=? AND ext=? AND mbid<>?`, kind, source, ext, except).Scan(&n) != nil {
		return false
	}
	return n > 0
}

func (r *Resolver) keep(kind, mbid string, t srctag.Tag, store, ext string) {
	if r.Keep != nil {
		r.Keep(kind, mbid, t, store, ext)
	}
}

// kept answers a fresh kept match: found is false for none kept (or one
// too old), ext "" for a kept miss.
func (r *Resolver) kept(kind, mbid, source string) (store, ext string, found bool) {
	if r.Kept == nil {
		return "", "", false
	}
	store, ext, at, ok := r.Kept(kind, mbid, source)
	if !ok {
		return "", "", false
	}
	life := idFresh
	if ext == "" {
		life = idMiss
	}
	if time.Since(at) >= life {
		return "", "", false
	}
	return store, ext, true
}

// Subject answers what the chains know about an artist or album.
func (r *Resolver) Subject(ctx context.Context, kind, mbid string) Subject {
	return r.info(ctx, kind, mbid)
}

// AppleJSON reads one Apple Music API answer into out. ok is false when
// the source is off or has no such thing (404); 429, 5xx and timeouts are
// errors.
func (r *Resolver) AppleJSON(ctx context.Context, path string, q url.Values, out any) (bool, error) {
	if !r.on("applemusic") {
		return false, nil
	}
	return r.appleGet(ctx, path, q, out)
}

// SpotifyJSON reads one Spotify Web API answer into out (path with its
// query), like AppleJSON; ok is false without the source's keys.
func (r *Resolver) SpotifyJSON(ctx context.Context, path string, out any) (bool, error) {
	if !r.on("spotify") {
		return false, nil
	}
	return r.spotifyGet(ctx, path, out)
}

// DeezerJSON reads one Deezer API answer into out, like AppleJSON.
func (r *Resolver) DeezerJSON(ctx context.Context, path string, q url.Values, out any) (bool, error) {
	if !r.on("deezer") {
		return false, nil
	}
	return r.deezerGet(ctx, path, q, out)
}

// ErrNotKnown: the artist's or album's stored MusicBrainz answer is not
// there yet and no match is kept, so no search can be made; ask again later.
var ErrNotKnown = errors.New("imgstore: subject not stored yet")

// AppleArtist answers the artist's Apple Music store and id ("" when Apple
// has none).
func (r *Resolver) AppleArtist(ctx context.Context, mbid string) (store, id string, err error) {
	if !r.on("applemusic") || r.AppleMusic == nil {
		return "", "", nil
	}
	if st, ext, ok := r.kept(KindArtist, mbid, "applemusic"); ok && ext == "" && !r.linkedOver(ctx, mbid, "applemusic", ext) {
		return st, ext, nil
	}
	a, err := r.appleArtistChecked(ctx, mbid)
	if err != nil {
		return "", "", err
	}
	if a == nil {
		if !r.info(ctx, KindArtist, mbid).Known {
			return "", "", ErrNotKnown
		}
		return "", "", nil
	}
	return a.Store, a.Artist.ID, nil
}

// AppleAlbumID answers the album's Apple Music store and id: the match the
// cover chain kept, else the same confident search the cover chain makes
// (and keeps). "" when Apple has none.
func (r *Resolver) AppleAlbumID(ctx context.Context, mbid string) (store, id string, err error) {
	if !r.on("applemusic") || r.AppleMusic == nil {
		return "", "", nil
	}
	if st, ext, ok := r.kept(KindAlbum, mbid, "applemusic"); ok {
		return st, ext, nil
	}
	if !r.info(ctx, KindAlbum, mbid).Known {
		return "", "", ErrNotKnown
	}
	m, err := r.appleAlbumMatch(ctx, mbid)
	if err != nil || m == nil {
		return "", "", err
	}
	return m.store, m.id, nil
}

// SpotifyArtist answers the artist's Spotify id ("" when Spotify has
// none): the kept match, else its MusicBrainz link (no call), else one
// search by name.
func (r *Resolver) SpotifyArtist(ctx context.Context, mbid string) (string, error) {
	if !r.on("spotify") || r.Keys.SpotifyID == "" || r.Keys.SpotifySecret == "" {
		return "", nil
	}
	if _, ext, ok := r.kept(KindArtist, mbid, "spotify"); ok && !r.linkedOver(ctx, mbid, "spotify", ext) {
		return ext, nil
	}
	s := r.info(ctx, KindArtist, mbid)
	if !s.Known {
		return "", ErrNotKnown
	}
	if l := LinkedID(s.Links, "spotify"); l != "" {
		r.keep(KindArtist, mbid, srctag.Tag{Source: "spotify"}, "link", l)
		r.via(mbid, "spotify", ViaLink)
		return l, nil
	}
	a, err := r.spotifyArtistOf(ctx, mbid)
	if err != nil || a == nil {
		return "", err
	}
	return a.ID, nil
}

// DeezerArtist answers the artist's Deezer id ("" when Deezer has none).
func (r *Resolver) DeezerArtist(ctx context.Context, mbid string) (string, error) {
	if !r.on("deezer") {
		return "", nil
	}
	if !r.info(ctx, KindArtist, mbid).Known {
		return "", ErrNotKnown
	}
	a, err := r.deezerArtistOf(ctx, mbid)
	if err != nil || a == nil {
		return "", err
	}
	return a.ID.String(), nil
}

// MusicKinds are the picture kinds that belong to music (TV and movie
// kinds, title-* and person, share the store and are never counted here).
var MusicKinds = []string{KindArtist, KindAlbum, KindWide, KindLogo}

// MusicBytes answers the bytes the music pictures take in the store.
func (s *Store) MusicBytes(ctx context.Context) (int64, error) {
	var n sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT SUM(bytes) FROM images WHERE kind IN (?,?,?,?)`,
		MusicKinds[0], MusicKinds[1], MusicKinds[2], MusicKinds[3]).Scan(&n)
	return n.Int64, err
}

// AppleArtistRow is an artist's kept Apple match.
type AppleArtistRow struct{ MBID, Store, AppleID string }

// AppleArtistRows answers every kept Apple artist match (misses left out).
func (s *Store) AppleArtistRows(ctx context.Context) ([]AppleArtistRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT mbid, store, ext FROM source_ids WHERE kind=? AND source='applemusic' AND ext<>''`, KindArtist)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppleArtistRow
	for rows.Next() {
		var r AppleArtistRow
		if err := rows.Scan(&r.MBID, &r.Store, &r.AppleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DropFrom removes a stored picture when it came from provider, and
// answers the removed record.
func (s *Store) DropFrom(ctx context.Context, kind, id, provider string) (Record, bool) {
	rec, ok := s.Record(ctx, kind, id)
	if !ok || rec.Provider != provider {
		return Record{}, false
	}
	s.drop(ctx, kind, id)
	return rec, true
}

// KV and SetKV read and write one of the store's own settings.
func (s *Store) KV(k string) string {
	var v string
	_ = s.db.QueryRow(`SELECT v FROM kv WHERE k=?`, k).Scan(&v)
	return v
}

func (s *Store) SetKV(k, v string) error {
	_, err := s.db.Exec(`INSERT INTO kv(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}

// Expire makes a picture due: its miss is cleared and a stored one's
// re-check is now, so the next queue job asks the sources again.
func (s *Store) Expire(ctx context.Context, kind, id string) {
	_, _ = s.db.ExecContext(ctx, `DELETE FROM misses WHERE kind=? AND mbid=?`, kind, id)
	_, _ = s.db.ExecContext(ctx, `UPDATE images SET recheck=0 WHERE kind=? AND mbid=?`, kind, id)
	s.forget(kind, id)
}

// AppleMissesBetween answers the artists whose Apple miss was kept in
// [from, to).
func (s *Store) AppleMissesBetween(ctx context.Context, from, to time.Time) []string {
	rows, err := s.db.QueryContext(ctx, `SELECT mbid FROM source_ids WHERE kind=? AND source='applemusic' AND ext='' AND at>=? AND at<?`, KindArtist, from.Unix(), to.Unix())
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	return out
}

// ForgetID drops a source's kept match or miss, so the next lookup asks
// the source again.
func (s *Store) ForgetID(kind, mbid, source string) {
	_, _ = s.db.Exec(`DELETE FROM source_ids WHERE kind=? AND mbid=? AND source=?`, kind, mbid, source)
}

// SourceRows answers every kept artist match of a source (misses left
// out); the Apple id field holds the source's id.
func (s *Store) SourceRows(ctx context.Context, source string) ([]AppleArtistRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT mbid, store, ext FROM source_ids WHERE kind=? AND source=? AND ext<>''`, KindArtist, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppleArtistRow
	for rows.Next() {
		var r AppleArtistRow
		if err := rows.Scan(&r.MBID, &r.Store, &r.AppleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetVia keeps how a source's match was found (a Via name).
func (s *Store) SetVia(kind, mbid, source, via string) {
	_, _ = s.db.Exec(`UPDATE source_ids SET found_by=? WHERE kind=? AND mbid=? AND source=?`, via, kind, mbid, source)
}

// FoundBy answers how a source's match was found ("" when not known).
func (s *Store) FoundBy(kind, mbid, source string) string {
	var v string
	_ = s.db.QueryRow(`SELECT found_by FROM source_ids WHERE kind=? AND mbid=? AND source=?`, kind, mbid, source).Scan(&v)
	return v
}

// UnverifiedRows answers a source's kept artist matches not yet
// classified by the exact-id order (found_by empty; misses left out).
func (s *Store) UnverifiedRows(ctx context.Context, source string) ([]AppleArtistRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT mbid, store, ext FROM source_ids WHERE kind=? AND source=? AND ext<>'' AND found_by=''`, KindArtist, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppleArtistRow
	for rows.Next() {
		var r AppleArtistRow
		if err := rows.Scan(&r.MBID, &r.Store, &r.AppleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UnverifiedMisses answers a source's kept artist misses not yet checked
// by the exact-id order (found_by empty).
func (s *Store) UnverifiedMisses(ctx context.Context, source string) ([]AppleArtistRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT mbid, store, ext FROM source_ids WHERE kind=? AND source=? AND ext='' AND found_by=''`, KindArtist, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppleArtistRow
	for rows.Next() {
		var r AppleArtistRow
		if err := rows.Scan(&r.MBID, &r.Store, &r.AppleID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ClearVia opens every artist match found a given way for classifying
// again (found_by emptied) and answers how many.
func (s *Store) ClearVia(ctx context.Context, via string) (int, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE source_ids SET found_by='' WHERE kind=? AND found_by=?`, KindArtist, via)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ExactHolders answers the artists holding a source's id that was found
// exactly: a MusicBrainz link, a barcode or Wikidata (never a name).
func (s *Store) ExactHolders(source, ext string) []string {
	rows, err := s.db.Query(`SELECT mbid FROM source_ids WHERE kind=? AND source=? AND ext=? AND found_by IN (?,?,?)`, KindArtist, source, ext, ViaLink, ViaBarcode, ViaWikidata)
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

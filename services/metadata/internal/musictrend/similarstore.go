package musictrend

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"time"
)

// Similar artists and artist data are kept in SQLite too (OpenStore), so a
// restart or a deploy serves them from storage instead of asking
// ListenBrainz and Apple Music again; memory stays the fast copy.
// Only real answers are kept: a list, or "the source has none". Failures
// and answers still being matched are never written, and a "none" or a
// short answer only for an artist MusicBrainz knows (SetKnown), so random
// ids stay in memory. Each table keeps at most twice Max rows (the oldest
// go first) and an hourly prune drops rows past their lifetime.

// storePruneEvery is how often kept rows are pruned; each table holds at
// most twice the memory cap (Max).
var storePruneEvery = time.Hour

// SimilarNoneSrc tags a kept "none": every similar source was asked and
// had none.
const SimilarNoneSrc = "applemusic+listenbrainz"

// OpenStore keeps answers in the SQLite file at path.
func (s *Similar) OpenStore(path string) error {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS similar (mbid TEXT PRIMARY KEY, body BLOB, none INTEGER NOT NULL, short INTEGER NOT NULL, at INTEGER NOT NULL) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS artist_data (mbid TEXT PRIMARY KEY, body BLOB, at INTEGER NOT NULL) WITHOUT ROWID;`); err != nil {
		db.Close()
		return err
	}
	for _, sp := range []srctag.Spec{
		// A "none" row is both sources (Apple Music, then ListenBrainz)
		// answering they have none; its tag says so.
		{Table: "similar", Key: []string{"mbid"}, Body: "body", Src: "COALESCE(json_extract(CAST(@.body AS TEXT),'$.source'),'listenbrainz')", Empty: "'" + SimilarNoneSrc + "'", Known: "at"},
		{Table: "artist_data", Key: []string{"mbid"}, Body: "body", Src: "'listenbrainz'", Empty: "'listenbrainz'", Known: "at"},
	} {
		if err := srctag.Stamp(db, sp); err != nil {
			db.Close()
			return err
		}
	}
	stop := make(chan struct{})
	s.mu.Lock()
	s.db, s.stop = db, stop
	s.mu.Unlock()
	go func() {
		t := time.NewTicker(storePruneEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				s.PruneStore()
			}
		}
	}()
	return nil
}

// SetKnown sets the check for an artist MusicBrainz knows (its answer is
// stored); a "none" or short answer is kept on disk only for those.
func (s *Similar) SetKnown(known func(mbid string) bool) {
	s.mu.Lock()
	s.known = known
	s.mu.Unlock()
}

func (s *Similar) storeMax() int { return 2 * s.max() }

// PruneStore drops kept rows past their lifetime (a "none" or short one
// after MissTTL, the rest after TTL) and the oldest past the cap.
func (s *Similar) PruneStore() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return
	}
	now := s.now()
	full := now.Add(-s.ttl(similarEntry{})).Unix()
	miss := now.Add(-s.ttl(similarEntry{short: true})).Unix()
	_, _ = s.db.Exec(`DELETE FROM similar WHERE at < ? OR ((none = 1 OR short = 1) AND at < ?)`, full, miss)
	_, _ = s.db.Exec(`DELETE FROM artist_data WHERE at < ? OR ((body IS NULL OR length(body) = 0) AND at < ?)`, full, miss)
	s.capLocked()
}

// capLocked keeps each table within storeMax rows, the newest; the caller
// holds mu.
func (s *Similar) capLocked() {
	n := s.storeMax()
	_, _ = s.db.Exec(`DELETE FROM similar WHERE mbid IN (SELECT mbid FROM similar ORDER BY at DESC, mbid LIMIT -1 OFFSET ?)`, n)
	_, _ = s.db.Exec(`DELETE FROM artist_data WHERE mbid IN (SELECT mbid FROM artist_data ORDER BY at DESC, mbid LIMIT -1 OFFSET ?)`, n)
}

// wroteLocked counts a write and keeps the cap every 100; the caller
// holds mu.
func (s *Similar) wroteLocked() {
	s.writes++
	if s.writes%100 == 0 {
		s.capLocked()
	}
}

// knownLocked reports an artist MusicBrainz knows; the caller holds mu.
func (s *Similar) knownLocked(mbid string) bool { return s.known != nil && s.known(mbid) }

// CloseStore closes the store.
func (s *Similar) CloseStore() error {
	s.mu.Lock()
	db, stop := s.db, s.stop
	s.db, s.stop = nil, nil
	s.mu.Unlock()
	if stop != nil {
		close(stop)
	}
	if db == nil {
		return nil
	}
	return db.Close()
}

// loadSimilar reads a kept answer into memory; the caller holds mu.
func (s *Similar) loadSimilarLocked(mbid string) (similarEntry, bool) {
	if s.db == nil {
		return similarEntry{}, false
	}
	var body []byte
	var none, short int
	var at int64
	if s.db.QueryRow(`SELECT body, none, short, at FROM similar WHERE mbid=?`, mbid).Scan(&body, &none, &short, &at) != nil {
		return similarEntry{}, false
	}
	e := similarEntry{body: body, at: time.Unix(at, 0), short: short == 1}
	if none == 1 {
		e.body, e.err = nil, ErrNoSimilar
	}
	s.setEntryLocked(mbid, e)
	return e, true
}

// keepSimilar writes an answer (a list, or none); the caller holds mu.
func (s *Similar) keepSimilarLocked(mbid string, e similarEntry) {
	if s.db == nil || e.err != nil && !errors.Is(e.err, ErrNoSimilar) {
		return
	}
	if (e.err != nil || e.short) && !s.knownLocked(mbid) {
		return // a miss for an id MusicBrainz may not know stays in memory
	}
	none, short := 0, 0
	if e.err != nil {
		none = 1
	}
	if e.short {
		short = 1
	}
	_, _ = s.db.Exec(`INSERT INTO similar(mbid, body, none, short, at) VALUES(?,?,?,?,?) ON CONFLICT(mbid) DO UPDATE SET body=excluded.body, none=excluded.none, short=excluded.short, at=excluded.at`, mbid, e.body, none, short, e.at.Unix())
	s.wroteLocked()
}

// loadDataLocked reads kept artist data into memory; the caller holds mu.
func (s *Similar) loadDataLocked(id string) bool {
	if s.db == nil {
		return false
	}
	var body []byte
	var at int64
	if s.db.QueryRow(`SELECT body, at FROM artist_data WHERE mbid=?`, id).Scan(&body, &at) != nil {
		return false
	}
	var d *ArtistData
	if len(body) > 0 {
		d = &ArtistData{}
		if json.Unmarshal(body, d) != nil {
			return false
		}
	}
	e := s.entry(id)
	e.data, e.dataErr, e.dataAt = d, d == nil, time.Unix(at, 0)
	return true
}

// keepDataLocked writes artist data (nil: the source has none); the
// caller holds mu.
func (s *Similar) keepDataLocked(id string, d *ArtistData, at time.Time) {
	if s.db == nil || d == nil && !s.knownLocked(id) {
		return
	}
	var body []byte
	if d != nil {
		var err error
		if body, err = json.Marshal(d); err != nil {
			return
		}
	}
	_, _ = s.db.Exec(`INSERT INTO artist_data(mbid, body, at) VALUES(?,?,?) ON CONFLICT(mbid) DO UPDATE SET body=excluded.body, at=excluded.at`, id, body, at.Unix())
	s.wroteLocked()
}

// StoreCounts answers how many answers are kept (status).
func (s *Similar) StoreCounts() map[string]int64 {
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	out := map[string]int64{}
	if db == nil {
		return out
	}
	var n int64
	if db.QueryRow(`SELECT COUNT(*) FROM similar`).Scan(&n) == nil {
		out["similar"] = n
	}
	if db.QueryRow(`SELECT COUNT(*) FROM artist_data`).Scan(&n) == nil {
		out["artist_data"] = n
	}
	return out
}

// DropSource removes an artist's kept similar list when it came from
// source (memory and storage) and answers the removed body (nil: none).
func (s *Similar) DropSource(mbid, source string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var body []byte
	if e, ok := s.entries[mbid]; ok && len(e.body) > 0 {
		body = e.body
	} else if s.db != nil {
		_ = s.db.QueryRow(`SELECT body FROM similar WHERE mbid=?`, mbid).Scan(&body)
	}
	if len(body) == 0 {
		return nil
	}
	var src struct {
		Source string `json:"source"`
	}
	if json.Unmarshal(body, &src) != nil || src.Source != source {
		return nil
	}
	s.deleteEntryLocked(mbid)
	if s.db != nil {
		_, _ = s.db.Exec(`DELETE FROM similar WHERE mbid=?`, mbid)
	}
	return body
}

// Expire makes an artist's kept list due (memory and storage): the next
// ask fetches it again, serving the old one should that fail.
func (s *Similar) Expire(mbid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[mbid]; ok {
		e.at = time.Time{}
		s.entries[mbid] = e
	}
	if s.db != nil {
		_, _ = s.db.Exec(`UPDATE similar SET at=0 WHERE mbid=?`, mbid)
	}
}

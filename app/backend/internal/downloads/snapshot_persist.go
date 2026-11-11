package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Persist timings.
const (
	flushEvery    = 5 * time.Second     // dirty rows are written at most this often
	progressEvery = 60 * time.Second    // progress-only changes are written at most this often
	goneKeep      = 30 * 24 * time.Hour // how long a removed item stays before the daily sweep deletes it
	torrentLogCap = 200                 // log lines kept per item; the API caps its view the same
)

// LogLine is one torrent log line in the API's line format.
type LogLine struct {
	Kind, Summary string
}

// LinesFunc returns the torrent log lines for one item going from prev to
// cur. The API supplies it so both sides share one line format.
type LinesFunc func(prev, cur Job) []LogLine

// itemKey identifies one item of one client.
type itemKey struct {
	id  int64
	key string
}

// itemWrite is a pending dl_items write; gone marks a removal.
type itemWrite struct {
	job  Job
	gone bool
}

// logWrite is pending torrent log lines for one item.
type logWrite struct {
	id    int64
	key   string
	at    time.Time
	lines []LogLine
}

// matchWrite is a pending match queue row; rename asks to retry an existing row.
type matchWrite struct {
	id             int64
	key, name, cat string
	priority       int
	rename         bool
	recat          bool
}

// persister batches what the reader saw into dl_items, torrent_log and
// dl_match_queue, so the database is hit in one transaction and only when
// something changed.
type persister struct {
	db    *database.Store
	lines LinesFunc
	now   func() time.Time

	mu      sync.Mutex
	dirty   map[itemKey]itemWrite
	logs    []logWrite
	matches []matchWrite
	written map[itemKey]time.Time
	urgent  bool
	flushed time.Time
	swept   time.Time
}

// note queues the writes for one read's changes. why is set when prev was
// loaded stale after a start, and is added to the log lines so they say the
// change was not seen live.
func (p *persister) note(id int64, ch Changes, prev, cur map[string]Job, why string) {
	if p == nil || ch.Empty() {
		return
	}
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.init()
	suffix := ""
	if why != "" {
		suffix = " (" + why + ")"
	}
	for _, k := range ch.Added {
		j := cur[k]
		p.dirty[itemKey{id, k}] = itemWrite{job: j}
		p.matches = append(p.matches, matchWrite{id: id, key: k, name: j.Name, cat: j.Category, priority: matchPriority(j)})
		p.urgent = true
	}
	moved := map[string]bool{}
	for _, t := range ch.Transitions {
		moved[t.ID] = true
	}
	for _, fc := range ch.Changed {
		k, j, old := fc.ID, cur[fc.ID], prev[fc.ID]
		ik := itemKey{id, k}
		big := moved[k]
		for _, f := range fc.Fields {
			switch f {
			case "Name", "Category", "SavePath", "Detail", "DoneAt":
				big = true
			}
		}
		if big || now.Sub(p.written[ik]) >= progressEvery {
			p.dirty[ik] = itemWrite{job: j}
			p.urgent = p.urgent || big
		}
		if j.Name != old.Name {
			p.matches = append(p.matches, matchWrite{id: id, key: k, name: j.Name, cat: j.Category, priority: matchPriority(j), rename: true})
		}
		if j.Category != old.Category {
			p.matches = append(p.matches, matchWrite{id: id, key: k, name: j.Name, cat: j.Category, priority: matchPriority(j), recat: true})
		}
		if p.lines == nil {
			continue
		}
		if ls := p.lines(old, j); len(ls) > 0 {
			at := now
			for i := range ls {
				ls[i].Summary += suffix
				if why != "" && ls[i].Kind == "completed" && j.DoneAt > 0 {
					at = time.Unix(j.DoneAt, 0).UTC()
				}
			}
			p.logs = append(p.logs, logWrite{id: id, key: k, at: at, lines: ls})
			p.urgent = true
		}
	}
	for _, k := range ch.Removed {
		p.dirty[itemKey{id, k}] = itemWrite{job: prev[k], gone: true}
		p.logs = append(p.logs, logWrite{id: id, key: k, at: now, lines: []LogLine{{Kind: "removed", Summary: "Removed outside HolyArrs" + suffix}}})
		p.urgent = true
	}
}

// due reports whether a flush should run now: a state moved, or flushEvery
// passed with something pending.
func (p *persister) due() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	pending := len(p.dirty) > 0 || len(p.logs) > 0 || len(p.matches) > 0
	return pending && (p.urgent || p.now().Sub(p.flushed) >= flushEvery)
}

// flush writes everything pending in one transaction. On failure the work is
// put back for the next flush.
func (p *persister) flush(ctx context.Context) error {
	if p == nil || p.db == nil {
		return nil
	}
	now := p.now()
	p.mu.Lock()
	p.init()
	dirty, logs, matches := p.dirty, p.logs, p.matches
	// Once a day, removed items older than goneKeep are deleted.
	sweep := now.Sub(p.swept) >= 24*time.Hour
	if len(dirty) == 0 && len(logs) == 0 && len(matches) == 0 && !sweep {
		p.mu.Unlock()
		return nil
	}
	p.dirty, p.logs, p.matches, p.urgent, p.flushed = map[itemKey]itemWrite{}, nil, nil, false, now
	p.mu.Unlock()
	at := now.UTC().Format(database.TimeLayout)
	err := p.db.Write(ctx, func(tx *sql.Tx) error {
		for k, w := range dirty {
			if w.gone {
				if _, err := tx.Exec("UPDATE dl_items SET gone_at = ?, last_change_at = ? WHERE downloader_id = ? AND key = ? AND gone_at IS NULL", at, at, k.id, k.key); err != nil {
					return err
				}
				continue
			}
			j := w.job
			data, _ := json.Marshal(j)
			if _, err := tx.Exec(`INSERT INTO dl_items (downloader_id, key, name, state, phase, size, done_bytes, uploaded, ratio, category, save_path, added_at, completed_at, last_change_at, gone_at, data)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)
				ON CONFLICT (downloader_id, key) DO UPDATE SET name = excluded.name, state = excluded.state, phase = excluded.phase, size = excluded.size,
				done_bytes = excluded.done_bytes, uploaded = excluded.uploaded, ratio = excluded.ratio, category = excluded.category, save_path = excluded.save_path,
				added_at = excluded.added_at, completed_at = excluded.completed_at, last_change_at = excluded.last_change_at, gone_at = NULL, data = excluded.data`,
				k.id, k.key, j.Name, j.State, j.Detail, j.Size, int64(float64(j.Size)*j.Progress), j.Uploaded, j.Ratio, j.Category, j.SavePath, j.AddedAt, j.DoneAt, at, string(data)); err != nil {
				return err
			}
		}
		for _, l := range logs {
			for _, line := range l.lines {
				if _, err := tx.Exec("INSERT INTO torrent_log (downloader_id, hash, at, kind, summary) VALUES (?, ?, ?, ?, ?)",
					l.id, strings.ToLower(l.key), l.at.UTC().Format(database.TimeLayout), line.Kind, line.Summary); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(`DELETE FROM torrent_log WHERE downloader_id = ? AND hash = ? AND id NOT IN
				(SELECT id FROM torrent_log WHERE downloader_id = ? AND hash = ? ORDER BY at DESC, id DESC LIMIT ?)`,
				l.id, strings.ToLower(l.key), l.id, strings.ToLower(l.key), torrentLogCap); err != nil {
				return err
			}
		}
		for _, m := range matches {
			if m.rename {
				// A new name deserves another try, even after nomatch.
				if _, err := tx.Exec(`UPDATE dl_match_queue SET name = ?, attempts = 0, next_try_at = ?, outcome = '', last_error = ''
					WHERE downloader_id = ? AND key = ? AND outcome IN ('', 'nomatch', 'unsure', 'not_media')`, m.name, at, m.id, m.key); err != nil {
					return err
				}
				continue
			}
			if m.recat {
				// The match decides from the category, so keep it current; a job
				// moved out of another app's category is looked up again.
				if _, err := tx.Exec(`UPDATE dl_match_queue SET category = ?,
					attempts = CASE WHEN outcome = ? THEN 0 ELSE attempts END,
					next_try_at = CASE WHEN outcome = ? THEN ? ELSE next_try_at END,
					outcome = CASE WHEN outcome = ? THEN '' ELSE outcome END
					WHERE downloader_id = ? AND key = ?`, m.cat, matchOtherApp, matchOtherApp, at, matchOtherApp, m.id, m.key); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.Exec(`INSERT INTO dl_match_queue (downloader_id, key, name, category, priority, next_try_at) VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT (downloader_id, key) DO NOTHING`, m.id, m.key, m.name, m.cat, m.priority, at); err != nil {
				return err
			}
		}
		if sweep {
			cut := now.Add(-goneKeep).UTC().Format(database.TimeLayout)
			if _, err := tx.Exec("DELETE FROM dl_items WHERE gone_at IS NOT NULL AND gone_at < ?", cut); err != nil {
				return err
			}
		}
		return nil
	})
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		// Put the work back in front of anything noted meanwhile, but keep a
		// newer write of the same item.
		for k, w := range dirty {
			if _, newer := p.dirty[k]; !newer {
				p.dirty[k] = w
			}
		}
		p.logs = append(logs, p.logs...)
		p.matches = append(matches, p.matches...)
		return err
	}
	if sweep {
		p.swept = now
	}
	for k := range dirty {
		p.written[k] = now
	}
	return nil
}

// load reads the last known items of every client, leaving out removed ones.
func (p *persister) load(ctx context.Context) (map[int64][]Job, error) {
	out := map[int64][]Job{}
	if p == nil || p.db == nil {
		return out, nil
	}
	rows, err := p.db.Read().QueryContext(ctx, "SELECT downloader_id, key, data FROM dl_items WHERE gone_at IS NULL")
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var key, data string
		if err := rows.Scan(&id, &key, &data); err != nil {
			return out, err
		}
		var j Job
		if json.Unmarshal([]byte(data), &j) != nil {
			continue
		}
		j.ID = key
		out[id] = append(out[id], j)
	}
	return out, rows.Err()
}

// init makes the maps on first use, so the zero value works.
func (p *persister) init() {
	if p.dirty == nil {
		p.dirty, p.written = map[itemKey]itemWrite{}, map[itemKey]time.Time{}
	}
}

// matchPriority orders lookups: active items first, then seeding ones, then
// the rest of the archive.
func matchPriority(j Job) int {
	switch j.State {
	case StateDownloading, StateQueued, StatePaused:
		return 1
	}
	if j.UpSpeed > 0 {
		return 2
	}
	return 3
}

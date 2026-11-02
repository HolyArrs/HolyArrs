package source

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Source states as people see them.
const (
	StateWorking     = "working"
	StateUnreachable = "unreachable"
	StateAuthFailed  = "auth_failed"
	StateDisabled    = "disabled"
)

// Health is a Source's state in plain words. Until is when HolyArrs tries
// it again on its own (nil when it is not waiting).
type Health struct {
	State string     `json:"state"`
	Text  string     `json:"text"`
	Until *time.Time `json:"until,omitempty"`
	// Reason tells waits apart: slowed, down, blocked or key (empty when
	// the Source is not waiting).
	Reason string `json:"reason,omitempty"`
}

// reasonWord is the short reason, by pause reason.
var reasonWord = map[string]string{
	KindDown:         "down",
	KindBroken:       "down",
	KindRateLimited:  "slowed",
	KindBlocked:      "blocked",
	KindLoginRefused: "key",
}

// TryFailed puts a Source tried by hand back on its back-off pause.
func (s *Searcher) TryFailed(ctx context.Context, src domain.Source, cause error) {
	s.pause(ctx, src, cause)
}

// reasonText says why a Source is waiting, by pause reason.
var reasonText = map[string]string{
	KindDown:        "HolyArrs could not reach it.",
	KindRateLimited: "It asked HolyArrs to slow down.",
	KindBroken:      "It answered with something HolyArrs could not read.",
	KindBlocked:     "Its address is on the local network, which this Source may not reach.",
}

// HealthOf says how a Source is doing.
func HealthOf(src domain.Source) Health {
	switch {
	case src.State == "disabled":
		return Health{State: StateDisabled, Text: "Paused by you. HolyArrs does not use it until you resume it."}
	case src.State == "paused" && src.PauseReason != nil && *src.PauseReason == KindLoginRefused:
		return Health{State: StateAuthFailed, Text: "It turned down the key. Check the key, then test it.", Until: src.PausedUntil, Reason: "key"}
	case src.State == "paused":
		text := reasonText[KindDown]
		if src.PauseReason != nil && reasonText[*src.PauseReason] != "" {
			text = reasonText[*src.PauseReason]
		}
		reason := "down"
		if src.PauseReason != nil && reasonWord[*src.PauseReason] != "" {
			reason = reasonWord[*src.PauseReason]
		}
		return Health{State: StateUnreachable, Text: text + " HolyArrs tries again on its own.", Until: src.PausedUntil, Reason: reason}
	}
	return Health{State: StateWorking, Text: "Working."}
}

// keepCalls is how long the call record is kept.
const keepCalls = 90 * 24 * time.Hour

// maxCallsPerSource caps the call record per Source.
var maxCallsPerSource = 50000

// Record keeps one call to a Source for its stats, and drops calls older
// than 90 days and the oldest past maxCallsPerSource.
func Record(ctx context.Context, s *store.Store, sourceID int64, op string, took time.Duration, err error) {
	kind := ""
	if err != nil {
		kind = KindDown
		var se *Error
		if errors.As(err, &se) {
			kind = se.Kind
		}
	}
	now := time.Now().UTC()
	_ = s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO source_calls(source_id, at, op, ok, ms, error_kind) VALUES (?, ?, ?, ?, ?, ?)",
			sourceID, store.FormatTime(now), op, err == nil, took.Milliseconds(), kind); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM source_calls WHERE source_id = ? AND at < ?", sourceID, store.FormatTime(now.Add(-keepCalls))); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM source_calls WHERE source_id = ? AND id <= (SELECT id FROM source_calls WHERE source_id = ?
			ORDER BY id DESC LIMIT 1 OFFSET ?)`, sourceID, sourceID, maxCallsPerSource)
		return err
	})
}

// Summary is a Source's last day at a glance.
type Summary struct {
	LastSuccess *string `json:"last_success"`
	LastFailure *string `json:"last_failure"`
	Calls       int     `json:"calls_24h"`
	Failures    int     `json:"failures_24h"`
	AvgMS       int     `json:"avg_ms_24h"`
}

// Summarise reads the last day of calls for every Source, by id. Every
// lookup is one Source and a time window, on the (source_id, at) index; the
// last success and failure are looked for within the kept record only.
func Summarise(ctx context.Context, db *sql.DB, now time.Time) (map[int64]Summary, error) {
	day, kept := since(now, 1), store.FormatTime(now.Add(-keepCalls))
	rows, err := db.QueryContext(ctx, `SELECT s.id,
		(SELECT max(at) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ? AND c.ok = 1),
		(SELECT max(at) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ? AND c.ok = 0),
		(SELECT count(*) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ?),
		(SELECT count(*) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ? AND c.ok = 0),
		(SELECT COALESCE(CAST(avg(ms) AS INTEGER), 0) FROM source_calls c WHERE c.source_id = s.id AND c.at >= ? AND c.ok = 1)
		FROM sources s`, kept, kept, day, day, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Summary{}
	for rows.Next() {
		var id int64
		var x Summary
		if err := rows.Scan(&id, &x.LastSuccess, &x.LastFailure, &x.Calls, &x.Failures, &x.AvgMS); err != nil {
			return nil, err
		}
		out[id] = x
	}
	return out, rows.Err()
}

func since(now time.Time, days int) string {
	return store.FormatTime(now.Add(-time.Duration(days) * 24 * time.Hour))
}

// Day is one day of a Source's calls and grabs (UTC).
type Day struct {
	Day      string `json:"day"`
	Calls    int    `json:"calls"`
	Failures int    `json:"failures"`
	Grabs    int    `json:"grabs"`
	AvgMS    int    `json:"avg_ms"`
}

// Call is one recorded call.
type Call struct {
	At        string `json:"at"`
	Op        string `json:"op"`
	OK        bool   `json:"ok"`
	MS        int    `json:"ms"`
	ErrorKind string `json:"error_kind"`
}

// Stats is a Source's record over time.
type Stats struct {
	Summary
	Days   []Day  `json:"days"`
	Recent []Call `json:"recent"`
}

// StatsFor reads a Source's last days (oldest first, every day listed) and
// its last 20 calls. A grab is a Release from it that was chosen.
func StatsFor(ctx context.Context, db *sql.DB, sourceID int64, days int, now time.Time) (Stats, error) {
	var st Stats
	all, err := Summarise(ctx, db, now)
	if err != nil {
		return st, err
	}
	st.Summary = all[sourceID]
	byDay := map[string]*Day{}
	for i := days - 1; i >= 0; i-- {
		d := now.UTC().AddDate(0, 0, -i).Format("2006-01-02")
		st.Days = append(st.Days, Day{Day: d})
	}
	for i := range st.Days {
		byDay[st.Days[i].Day] = &st.Days[i]
	}
	from := now.UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := db.QueryContext(ctx, `SELECT substr(at, 1, 10) d, count(*), count(CASE WHEN ok = 0 THEN 1 END),
		COALESCE(CAST(avg(CASE WHEN ok = 1 THEN ms END) AS INTEGER), 0)
		FROM source_calls WHERE source_id = ? AND at >= ? GROUP BY d`, sourceID, from)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var d string
		var x Day
		if err := rows.Scan(&d, &x.Calls, &x.Failures, &x.AvgMS); err != nil {
			rows.Close()
			return st, err
		}
		if p := byDay[d]; p != nil {
			p.Calls, p.Failures, p.AvgMS = x.Calls, x.Failures, x.AvgMS
		}
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT substr(updated_at, 1, 10) d, count(*) FROM releases
		WHERE source_id = ? AND state = 'chosen' AND updated_at >= ? GROUP BY d`, sourceID, from)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			rows.Close()
			return st, err
		}
		if p := byDay[d]; p != nil {
			p.Grabs = n
		}
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT at, op, ok, ms, error_kind FROM source_calls WHERE source_id = ? ORDER BY at DESC, id DESC LIMIT 20`, sourceID)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	st.Recent = []Call{}
	for rows.Next() {
		var c Call
		if err := rows.Scan(&c.At, &c.Op, &c.OK, &c.MS, &c.ErrorKind); err != nil {
			return st, err
		}
		st.Recent = append(st.Recent, c)
	}
	return st, rows.Err()
}

package sources

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// States of a Source as people see them.
const (
	StateWorking     = "working"
	StateUnreachable = "unreachable"
	StateAuthFailed  = "auth_failed"
	StateDisabled    = "disabled"
)

// keepCalls is how long the call record is kept.
const keepCalls = 90 * 24 * time.Hour

// reasonWord maps a pause reason to its short word.
var reasonWord = map[string]string{
	KindDown:         "down",
	KindBroken:       "down",
	KindRateLimited:  "slowed",
	KindBlocked:      "blocked",
	KindLoginRefused: "key",
}

// reasonText maps a pause reason to the sentence that says why the Source waits.
var reasonText = map[string]string{
	KindDown:        "HolyArrs could not reach it.",
	KindRateLimited: "It asked HolyArrs to slow down.",
	KindBroken:      "It answered with something HolyArrs could not read.",
	KindBlocked:     "Its address is on the local network, which this Source may not reach.",
}

// maxCallsPerSource caps the call record of one Source.
var maxCallsPerSource = 50000

// Health is a Source's state in plain words.
// Until is when HolyArrs tries the Source again on its own, nil when it is not waiting.
type Health struct {
	State string     `json:"state"`
	Text  string     `json:"text"`
	Until *time.Time `json:"until,omitempty"`
	// Reason tells the waits apart: slowed, down, blocked or key. It is empty when the Source is not waiting.
	Reason string `json:"reason,omitempty"`
	// Block is the kind of block when a site's protection stops the Source.
	Block string `json:"block,omitempty"`
}

// Summary is a Source's last day at a glance.
type Summary struct {
	LastSuccess *string `json:"last_success"`
	LastFailure *string `json:"last_failure"`
	Calls       int     `json:"calls_24h"`
	Failures    int     `json:"failures_24h"`
	AvgMS       int     `json:"avg_ms_24h"`
}

// Day is one day of a Source's calls and grabs, in UTC.
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

// TryFailed puts a Source that was tried by hand back on its back-off pause.
func (s *Searcher) TryFailed(ctx context.Context, src domain.Source, cause error) {
	s.pause(ctx, src, cause)
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
		if b := BlockOf(src); b != "" {
			text = blockText[b]
		}
		reason := "down"
		if src.PauseReason != nil && reasonWord[*src.PauseReason] != "" {
			reason = reasonWord[*src.PauseReason]
		}
		return Health{State: StateUnreachable, Text: text + " HolyArrs tries again on its own.", Until: src.PausedUntil, Reason: reason, Block: BlockOf(src)}
	}
	return Health{State: StateWorking, Text: "Working."}
}

// Record keeps one call to a Source for its stats.
// It drops calls older than the keep time and the oldest ones past maxCallsPerSource.
func Record(ctx context.Context, s *database.Store, sourceID int64, op string, took time.Duration, err error) {
	kind := ""
	if err != nil {
		kind = KindDown
		var se *Error
		if errors.As(err, &se) {
			kind = se.Kind
		}
	}
	storeCall(ctx, s, sourceID, op, took, err == nil, kind, time.Now().UTC())
}

// Summarise reads the last day of calls for every Source, by id.
// Each lookup is one Source and a time window on the (source_id, at) index.
// The last success and failure are searched within the kept record only.
func Summarise(ctx context.Context, db *sql.DB, now time.Time) (map[int64]Summary, error) {
	day, kept := since(now, 1), database.FormatTime(now.Add(-keepCalls))
	return callSummaries(ctx, db, kept, day)
}

// StatsFor reads a Source's last days, oldest first with every day listed, and its last 20 calls.
// A grab is a Release from the Source that was chosen.
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
	perDay, err := callsPerDay(ctx, db, sourceID, from)
	if err != nil {
		return st, err
	}
	for _, x := range perDay {
		if p := byDay[x.Day]; p != nil {
			p.Calls, p.Failures, p.AvgMS = x.Calls, x.Failures, x.AvgMS
		}
	}
	grabs, err := grabsPerDay(ctx, db, sourceID, from)
	if err != nil {
		return st, err
	}
	for d, n := range grabs {
		if p := byDay[d]; p != nil {
			p.Grabs = n
		}
	}
	st.Recent, err = recentCalls(ctx, db, sourceID)
	return st, err
}

// since formats the time that many days before now.
func since(now time.Time, days int) string {
	return database.FormatTime(now.Add(-time.Duration(days) * 24 * time.Hour))
}

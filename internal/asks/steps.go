package asks

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Step says where an approved ask is right now, from the server's own
// records (#139): what the Requests page shows when it has no live detail,
// and all the people who asked see (no percent, no download names).
type Step struct {
	// Kind is searching, downloading, importing, waiting_on_admin,
	// waiting (nothing found yet) or not_out (nothing out yet).
	Kind string `json:"kind"`
	// Text is why it waits, as the search last said it (people who approve only).
	Text string `json:"text,omitempty"`
	// SearchedAt is when it was last looked for (waiting only).
	SearchedAt *time.Time `json:"searched_at,omitempty"`
}

// AddSteps fills Step on approved asks that are not in the Library yet.
// staff keeps the search's own words for why an ask waits.
func (s *Service) AddSteps(ctx context.Context, vs []View, staff bool) {
	db := s.Store.Read()
	for i := range vs {
		v := &vs[i]
		if v.State != domain.AskApproved || v.TitleID == nil {
			continue
		}
		if st, ok := stepOf(ctx, db, *v.TitleID); ok {
			if !staff {
				st.Text = ""
			}
			v.Step = &st
		}
	}
}

// stepOf reads where the title's kept items are: a decision waiting on a
// person first, then an import, a download, a search, and else why
// nothing is coming yet. Nothing left to get (or a failed read): no step.
func stepOf(ctx context.Context, db *sql.DB, titleID int64) (Step, bool) {
	rows, err := db.QueryContext(ctx, `SELECT s.id, s.state, COALESCE(s.last_reason, ''), COALESCE(i.part_id, 0), i.last_searched_at
		FROM slots s JOIN items i ON i.id = s.item_id
		WHERE i.title_id = ? AND i.keep_up_to_date = 1 AND i.dropped_at IS NULL`, titleID)
	if err != nil {
		return Step{}, false
	}
	defer rows.Close()
	seen := map[domain.SlotState]bool{}
	var slotIDs, partIDs []any
	reason, missing := "", 0
	var searched *time.Time
	for rows.Next() {
		var id, part int64
		var state, why string
		var at sql.NullString
		if rows.Scan(&id, &state, &why, &part, &at) != nil {
			continue
		}
		st := domain.SlotState(state)
		seen[st] = true
		slotIDs = append(slotIDs, id)
		if part > 0 {
			partIDs = append(partIDs, part)
		}
		switch st {
		case domain.SlotWanted, domain.SlotBetterWanted, domain.SlotFailed, domain.SlotRejected:
			missing++
			if reason == "" && why != "" {
				reason = why
			}
			if t, ok := parseAt(at); ok && (searched == nil || t.After(*searched)) {
				searched = &t
			}
		}
	}
	if rows.Err() != nil {
		return Step{}, false
	}
	switch {
	case seen[domain.SlotNeedsADecision] || seen[domain.SlotHeld]:
		return Step{Kind: "waiting_on_admin"}, true
	case seen[domain.SlotImporting]:
		return Step{Kind: "importing"}, true
	case seen[domain.SlotChosen] || seen[domain.SlotDownloading] || seen[domain.SlotStalled]:
		return Step{Kind: "downloading"}, true
	case seen[domain.SlotSearching] || searchQueued(ctx, db, titleID, slotIDs, partIDs):
		return Step{Kind: "searching"}, true
	case missing > 0:
		return Step{Kind: "waiting", Text: reason, SearchedAt: searched}, true
	case seen[domain.SlotNotOutYet]:
		return Step{Kind: "not_out"}, true
	}
	return Step{}, false
}

// searchQueued says a search for the title is waiting or running: the
// one an approval starts (its scan or refresh first), or one for a slot,
// season or album of it.
func searchQueued(ctx context.Context, db *sql.DB, titleID int64, slotIDs, partIDs []any) bool {
	key := func(p string) string { return p + ":" + strconv.FormatInt(titleID, 10) }
	q := `SELECT count(*) FROM jobs WHERE state IN ('queued', 'running') AND (
		dedupe_key IN (?, ?)
		OR (kind IN ('refresh_title', 'scan_title') AND json_extract(subject, '$.type') = 'title' AND json_extract(subject, '$.id') = ? AND params LIKE '%search%')`
	args := []any{key("scan_then_search"), key("search_wanted"), titleID}
	if len(slotIDs) > 0 {
		q += ` OR (kind = 'search_slot' AND json_extract(subject, '$.id') IN (` + marks(len(slotIDs)) + `))`
		args = append(args, slotIDs...)
	}
	if len(partIDs) > 0 {
		q += ` OR (kind IN ('search_season', 'search_album') AND json_extract(subject, '$.id') IN (` + marks(len(partIDs)) + `))`
		args = append(args, partIDs...)
	}
	q += `)`
	var n int
	if err := db.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// parseAt reads a stored time (the store's layout, or any RFC 3339 one).
func parseAt(at sql.NullString) (time.Time, bool) {
	if !at.Valid || at.String == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(store.TimeLayout, at.String); err == nil {
		return t, true
	}
	t, err := time.Parse(time.RFC3339Nano, at.String)
	return t, err == nil
}

func marks(n int) string { return strings.TrimSuffix(strings.Repeat("?, ", n), ", ") }

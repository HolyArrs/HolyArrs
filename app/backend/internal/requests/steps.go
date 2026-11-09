package requests

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Step says where an approved ask is right now, from the server's own
// records: what the Requests page shows when it has no live detail,
// and all the people who asked see (no percent, no download names).
type Step struct {
	// Kind is searching, downloading, importing, waiting_on_admin,
	// waiting (nothing found yet) or not_out (nothing out yet).
	Kind string `json:"kind"`
	// Text is why it waits, as the search last said it (people who approve only).
	Text string `json:"text,omitempty"`
	// SearchedAt is when it was last looked for (waiting only).
	SearchedAt *time.Time `json:"searched_at,omitempty"`
	// NextSearch is when the search for missing looks next, for an ask not
	// looked for yet (waiting only; empty when that search is off).
	NextSearch *time.Time `json:"next_search,omitempty"`
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
		if st, ok := stepOf(ctx, db, *v); ok {
			if !staff {
				st.Text = ""
			}
			v.Step = &st
		}
	}
}

// stepOf reads where the ask's own part of the title is (its seasons or
// episodes, its albums, its 4K or plain copy), over the items the searcher
// would look for: a decision waiting on a person first, then an import, a
// download, a search (or a found release waiting out its delay), and else
// why nothing is coming yet. Nothing left to get (or a failed read): no step.
func stepOf(ctx context.Context, db *sql.DB, v View) (Step, bool) {
	titleID := *v.TitleID
	q := stepOfSQL
	args := []any{titleID}
	fourK := stepOfSQL2
	switch v.Kind {
	case "movie":
		if v.FourK {
			q += " AND s.version_id IN " + fourK
		} else {
			q += " AND s.version_id NOT IN " + fourK
		}
	case "show":
		var or []string
		if len(v.Seasons) > 0 {
			or = append(or, "p.number IN ("+marks(len(v.Seasons))+")")
			for _, n := range v.Seasons {
				args = append(args, n)
			}
		}
		for _, e := range v.Episodes {
			if len(e.Episodes) == 0 {
				continue
			}
			or = append(or, "(p.number = ? AND json_extract(i.numbers, '$.episode') IN ("+marks(len(e.Episodes))+"))")
			args = append(args, e.Season)
			for _, n := range e.Episodes {
				args = append(args, n)
			}
		}
		if len(or) > 0 {
			q += " AND (" + strings.Join(or, " OR ") + ")"
		}
	case "artist":
		var ids []any
		for _, al := range v.Albums {
			if al.MBID != "" {
				ids = append(ids, al.MBID)
			}
		}
		if len(ids) > 0 {
			q += " AND json_extract(p.source_ids, '$.musicbrainz') IN (" + marks(len(ids)) + ")"
			args = append(args, ids...)
		}
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return Step{}, false
	}
	defer rows.Close()
	seen := map[mediafiles.SlotState]bool{}
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
		st := mediafiles.SlotState(state)
		seen[st] = true
		slotIDs = append(slotIDs, id)
		if part > 0 {
			partIDs = append(partIDs, part)
		}
		switch st {
		case mediafiles.SlotWanted, mediafiles.SlotBetterWanted, mediafiles.SlotFailed, mediafiles.SlotRejected:
			missing++
			// Only a search's own words say why nothing came: a slot never
			// looked for still has the reason it is wanted ("aired …").
			if t, ok := parseAt(at); ok {
				if reason == "" && why != "" {
					reason = why
				}
				if searched == nil || t.After(*searched) {
					searched = &t
				}
			}
		}
	}
	if rows.Err() != nil {
		return Step{}, false
	}
	switch {
	case seen[mediafiles.SlotNeedsADecision]:
		return Step{Kind: "waiting_on_admin"}, true
	case seen[mediafiles.SlotImporting]:
		return Step{Kind: "importing"}, true
	case seen[mediafiles.SlotChosen] || seen[mediafiles.SlotDownloading] || seen[mediafiles.SlotStalled]:
		return Step{Kind: "downloading"}, true
	// Held: a release was found and waits out the Pick rules' delay; it is
	// grabbed by itself, nobody has to act.
	case seen[mediafiles.SlotSearching] || seen[mediafiles.SlotHeld] || searchQueued(ctx, db, titleID, slotIDs, partIDs):
		return Step{Kind: "searching"}, true
	case missing > 0:
		st := Step{Kind: "waiting", Text: reason, SearchedAt: searched}
		if searched == nil {
			var next sql.NullString
			var on bool
			if db.QueryRowContext(ctx, stepOfSQL3).Scan(&next, &on) == nil && on {
				if t, ok := parseAt(next); ok {
					st.NextSearch = &t
				}
			}
		}
		return st, true
	case seen[mediafiles.SlotNotOutYet]:
		return Step{Kind: "not_out"}, true
	}
	return Step{}, false
}

// searchQueued says a search for the title is waiting or running: the
// one an approval starts (its scan or refresh first), or one for a slot,
// season or album of it.
func searchQueued(ctx context.Context, db *sql.DB, titleID int64, slotIDs, partIDs []any) bool {
	key := func(p string) string { return p + ":" + strconv.FormatInt(titleID, 10) }
	q := searchQueuedSQL
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
	if t, err := time.Parse(database.TimeLayout, at.String); err == nil {
		return t, true
	}
	t, err := time.Parse(time.RFC3339Nano, at.String)
	return t, err == nil
}

func marks(n int) string { return strings.TrimSuffix(strings.Repeat("?, ", n), ", ") }

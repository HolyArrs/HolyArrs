package sources

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Errors of ImportTurnedDown.
var (
	ErrNoSourceToFile = errors.New("no Source is here yet to keep it under; add or link a Source, then import again")
	ErrNoSlotMatches  = errors.New("nothing of the title here matches its name")
)

// TurnDownIn is a release another app never takes again (its blocklist).
type TurnDownIn struct {
	TitleID  int64
	Name     string
	InfoHash string
	Protocol string // torrent or usenet
	Indexer  string // the old app's indexer name, to find the same Source here
	Reason   string
}

// ImportTurnedDown puts the release on the Turned down list for the title's
// Slots its name names (every Slot when it names no episode). A release is
// kept under a Source, so it goes under the Source of the old indexer's name
// or else the first one of its protocol; the list matches by name and info
// hash whichever Source finds it again. ok is false when the same name is
// already turned down for the title. Each then runs in the same transaction.
func ImportTurnedDown(ctx context.Context, s *database.Store, in TurnDownIn, by domain.Actor, then ...func(tx *sql.Tx, id int64) error) (int64, bool, error) {
	all, err := mediafiles.TitleSlotNumbers(ctx, s.Read(), in.TitleID)
	if err != nil {
		return 0, false, err
	}
	slots := matchNamedSlots(in.Name, all)
	if len(slots) == 0 {
		return 0, false, ErrNoSlotMatches
	}
	var id int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		ids := jsonText(slots)
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM releases WHERE state = 'turned_down' AND lower(name) = lower(?)
			AND EXISTS (SELECT 1 FROM json_each(matched_slot_ids) m WHERE m.value IN (SELECT value FROM json_each(?))) LIMIT 1`, in.Name, ids).Scan(&one)
		if err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		src, err := sourceToFile(ctx, tx, in.Indexer, in.Protocol)
		if err != nil {
			return err
		}
		rel := &domain.Release{SourceID: src, Name: in.Name, Protocol: in.Protocol, Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: slots, FoundBy: "hand_search", State: "turned_down",
			Verdict: map[string]any{"turned_down_reason": in.Reason, "imported": true}}
		if h := strings.ToLower(strings.TrimSpace(in.InfoHash)); h != "" {
			rel.InfoHash = &h
		}
		if id, err = database.Insert(tx, "releases", rel, by); err != nil {
			return err
		}
		for _, f := range then {
			if err := f(tx, id); err != nil {
				return err
			}
		}
		return nil
	})
	return id, id != 0, err
}

// sourceToFile picks the Source an imported release is kept under: the one
// named like the old indexer (Prowlarr adds " (Prowlarr)" to the name in the
// old apps), else the first Source of the protocol.
func sourceToFile(ctx context.Context, tx *sql.Tx, indexer, protocol string) (int64, error) {
	var id int64
	name := strings.TrimSpace(indexer)
	err := tx.QueryRowContext(ctx, `SELECT id FROM sources WHERE `+anyState+` AND (lower(name) = lower(?) OR lower(name) = lower(?)) ORDER BY id LIMIT 1`,
		name, strings.TrimSuffix(name, " (Prowlarr)")).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `SELECT id FROM sources WHERE `+anyState+` AND protocol = ? ORDER BY id LIMIT 1`, protocol).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoSourceToFile
	}
	return id, err
}

// matchNamedSlots keeps the Slots whose season and episodes a show
// release's name gives; a name with no numbers (a movie) keeps them all.
func matchNamedSlots(name string, all []mediafiles.NumberedSlot) []int64 {
	show := releasename.Parse(name, releasename.Hints{Show: true}).Show
	var out []int64
	for _, sl := range all {
		if show != nil && len(show.Seasons) > 0 && sl.Season+sl.Episode > 0 {
			if !slices.Contains(show.Seasons, sl.Season) || len(show.Episodes) > 0 && !slices.Contains(show.Episodes, sl.Episode) {
				continue
			}
		}
		out = append(out, sl.ID)
	}
	return out
}

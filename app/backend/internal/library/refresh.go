package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// lookupID is the id the metadata provider is asked for.
func lookupID(t mediafiles.Title) string {
	if id := t.SourceIDs["tmdb"]; id != "" {
		return "tmdb:" + id
	}
	if id := t.SourceIDs["imdb"]; id != "" {
		return "imdb:" + id
	}
	return ""
}

// RefreshOutcome says what one details refresh did.
type RefreshOutcome struct {
	Dropped     bool    `json:"dropped,omitempty"`
	GateDriver  string  `json:"gate_driver"`
	GateOpensAt *string `json:"gate_opens_at,omitempty"`
	Opened      []int64 `json:"gate_opened_slots,omitempty"`
	Closed      []int64 `json:"gate_closed_slots,omitempty"`
}

// RefreshTitle fetches fresh details for one Title, stores them, works out
// its Release gate again and moves its Slots across the gate. A movie the
// metadata source no longer knows is flagged (dropped_at), never deleted.
func (s *Service) RefreshTitle(ctx context.Context, titleID int64, by domain.Actor, jobID *int64) (RefreshOutcome, error) {
	var out RefreshOutcome
	var t mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil {
		return out, err
	}
	if t.Kind == "show" {
		return s.showParts().Refresh(ctx, titleID, by, jobID)
	}
	if t.Kind == "artist" {
		r, err := s.RefreshArtist(ctx, titleID, by, jobID)
		out.Dropped, out.GateDriver, out.Opened, out.Closed = r.Dropped, "release_date", r.Opened, r.Closed
		return out, err
	}
	id := lookupID(t)
	if id == "" {
		return out, errors.New("the Title has no metadata id")
	}
	now := s.now()
	m, err := s.Metadata.Movie(metadata.NoCache(ctx), id)
	if errors.Is(err, metadata.ErrNotFound) {
		out.Dropped = true
		first := t.DroppedAt == nil
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE titles SET dropped_at = COALESCE(dropped_at, ?), refreshed_at = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				database.FormatTime(now), database.FormatTime(now), database.FormatTime(now), ActorText(by), titleID)
			return err
		})
		if err == nil && first {
			s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleDropped, TitleID: &titleID, JobID: jobID,
				Summary: t.Name + " is no longer known to the metadata source; kept and flagged", GroupKey: GroupKey(titleID)})
		}
		return out, err
	}
	if err != nil {
		return out, err
	}
	unlock := s.Store.LockFiles()
	defer unlock()
	var opened, closed []int64
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		if t.State == "removed" {
			return database.ErrNotFound
		}
		if err := syncMovieCollection(ctx, tx, t.ID, m); err != nil {
			return err
		}
		applyMovie(&t, m, now)
		t.DroppedAt = nil
		t.RefreshedAt = &now
		t.GateDriver, t.GateOpensAt = Gate(t.ReleaseGate, t.GateDelayDays, DatesOf(t))
		if err := database.Update(tx, "titles", &t, by); err != nil {
			return err
		}
		if m.Runtime > 0 {
			if _, err := tx.Exec("UPDATE items SET runtime = ? WHERE title_id = ? AND kind = 'movie'", m.Runtime, titleID); err != nil {
				return err
			}
		}
		var err error
		opened, closed, err = applyGateTx(ctx, tx, t, now, by)
		return err
	})
	if err != nil {
		return out, err
	}
	out.GateDriver, out.Opened, out.Closed = t.GateDriver, opened, closed
	s.CachePoster(ctx, titleID, m.Poster, by)
	if t.GateOpensAt != nil {
		v := t.GateOpensAt.Format(time.RFC3339)
		out.GateOpensAt = &v
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleRefreshed, TitleID: &titleID, JobID: jobID,
		Summary: "Refreshed details of " + t.Name, GroupKey: GroupKey(titleID),
		Data: map[string]any{"gate_driver": t.GateDriver, "gate_opens_at": out.GateOpensAt, "cinema_date": t.CinemaDate,
			"digital_date": t.DigitalDate, "physical_date": t.PhysicalDate}})
	s.gateEvents(ctx, t, opened, closed, by, jobID)
	return out, nil
}

func GroupKey(titleID int64) string { return "title:" + strconv.FormatInt(titleID, 10) }

// applyGateTx moves the Title's Slots across its gate: not_out_yet to wanted
// once open, and wanted back to not_out_yet when the gate moved later.
func applyGateTx(ctx context.Context, tx *sql.Tx, t mediafiles.Title, now time.Time, by domain.Actor) (opened, closed []int64, err error) {
	slots, err := database.Select[mediafiles.Slot](ctx, tx, "slots",
		"item_id IN (SELECT id FROM items WHERE title_id = ?) AND state IN ('not_out_yet', 'wanted')", t.ID)
	if err != nil {
		return nil, nil, err
	}
	open := GateOpen(t.GateDriver, t.GateOpensAt, now)
	reason := gateReason(t.GateDriver, t.GateOpensAt)
	for _, sl := range slots {
		switch {
		case open && sl.State == mediafiles.SlotNotOutYet:
			if err := mediafiles.MoveSlotTx(tx, sl.ID, mediafiles.SlotWanted, reason, by); err != nil {
				return nil, nil, err
			}
			opened = append(opened, sl.ID)
		case !open && sl.State == mediafiles.SlotWanted:
			if err := mediafiles.MoveSlotTx(tx, sl.ID, mediafiles.SlotNotOutYet, reason, by); err != nil {
				return nil, nil, err
			}
			closed = append(closed, sl.ID)
		default:
			if _, err := tx.Exec("UPDATE slots SET last_reason = ? WHERE id = ? AND state = 'not_out_yet'", reason, sl.ID); err != nil {
				return nil, nil, err
			}
		}
	}
	return opened, closed, nil
}

func (s *Service) gateEvents(ctx context.Context, t mediafiles.Title, opened, closed []int64, by domain.Actor, jobID *int64) {
	for _, id := range opened {
		slot := id
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventGateOpened, TitleID: &t.ID, SlotID: &slot, JobID: jobID,
			Summary: t.Name + " is out: searching may start", Data: map[string]any{"gate_driver": t.GateDriver, "gate_opens_at": t.GateOpensAt},
			GroupKey: GroupKey(t.ID)})
		s.Publish("slot", map[string]any{"id": slot, "state": mediafiles.SlotWanted})
	}
	for _, id := range closed {
		slot := id
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventGateClosed, TitleID: &t.ID, SlotID: &slot, JobID: jobID,
			Summary: t.Name + " is not out yet: " + gateReason(t.GateDriver, t.GateOpensAt),
			Data:    map[string]any{"gate_driver": t.GateDriver, "gate_opens_at": t.GateOpensAt}, GroupKey: GroupKey(t.ID)})
		s.Publish("slot", map[string]any{"id": slot, "state": mediafiles.SlotNotOutYet})
	}
}

// handleRefreshTitle refreshes one Title, then runs the next step of its
// chain (a new movie: scan, then search when asked).
func (s *Service) handleRefreshTitle(ctx context.Context, r *jobs.Run) error {
	if r.Job.Subject == nil || r.Job.Subject.Type != "title" {
		return errors.New("refresh_title needs a title subject")
	}
	titleID := r.Job.Subject.ID
	if isNew, _ := r.Job.Params["new"].(bool); isNew {
		ctx = useCache(ctx) // an artist just added: its answers are fresh in the cache
	}
	out, err := s.RefreshTitle(ctx, titleID, JobActor(r), &r.Job.ID)
	if err != nil {
		return fmt.Errorf("refresh: %w", err)
	}
	r.SetResult(map[string]any{"dropped": out.Dropped, "gate_driver": out.GateDriver, "gate_opens_at": out.GateOpensAt,
		"gate_opened": len(out.Opened), "gate_closed": len(out.Closed)})
	return s.RunNextStep(ctx, r, titleID)
}

// cachePoster stores the poster in the artwork cache (a smaller size than
// the search answer shows) and records it on the Title. A failure only logs.
func (s *Service) CachePoster(ctx context.Context, titleID int64, poster string, by domain.Actor) {
	if s.Artwork == nil || poster == "" {
		return
	}
	name, err := s.Artwork.Fetch(ctx, strings.Replace(poster, "/w500/", "/w342/", 1))
	if err != nil {
		log.Printf("library: poster for title %d: %v", titleID, err)
		return
	}
	art := `["poster:` + name + `"]`
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET artwork = ?, updated_at = ?, updated_by = ? WHERE id = ? AND artwork != ?",
			art, database.FormatTime(s.now()), ActorText(by), titleID, art)
		return err
	})
	if err != nil {
		log.Printf("library: poster for title %d: %v", titleID, err)
	}
}

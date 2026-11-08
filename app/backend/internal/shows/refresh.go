package shows

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ShowRefresh says what one show details refresh did.
type ShowRefresh struct {
	Dropped bool   `json:"dropped,omitempty"`
	Source  string `json:"details_source"`
	ShowSync
}

// RefreshShow fetches a show's details and episodes from its details
// source and brings its seasons, episodes and Slots in line. A show the
// source no longer knows is flagged (dropped_at), never deleted.
func (s *Service) RefreshShow(ctx context.Context, titleID int64, by domain.Actor, jobID *int64) (ShowRefresh, error) {
	return s.refreshShow(ctx, titleID, "", by, jobID)
}

// SwitchSource makes a show follow another details source: the show is
// found there from the ids already known, then refreshed from it. When the
// new source does not know the show nothing changes.
func (s *Service) SwitchSource(ctx context.Context, titleID int64, source string, by domain.Actor) (ShowRefresh, error) {
	if !metadata.ValidShowSource(source) {
		return ShowRefresh{}, metadata.ErrUnknownSource
	}
	return s.refreshShow(ctx, titleID, source, by, nil)
}

func (s *Service) refreshShow(ctx context.Context, titleID int64, switchTo string, by domain.Actor, jobID *int64) (ShowRefresh, error) {
	var out ShowRefresh
	var t mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil {
		return out, err
	}
	if t.Kind != "show" {
		return out, library.ErrNotShow
	}
	source := t.MetadataSource
	if switchTo != "" {
		source = switchTo
	}
	out.Source = source
	p, err := s.showSource(source)
	if err != nil {
		return out, err
	}
	defs, err := library.ReadShowDefaults(ctx, s.Store.Read())
	if err != nil {
		return out, err
	}
	now := s.Clock()
	sh, err := lookupShow(metadata.NoCache(ctx), p, source, t.SourceIDs)
	if errors.Is(err, metadata.ErrShowNotFound) {
		if switchTo != "" && switchTo != t.MetadataSource {
			return out, fmt.Errorf("%s does not know this show: %w", sourceName(switchTo), err)
		}
		out.Dropped = true
		first := t.DroppedAt == nil
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE titles SET dropped_at = COALESCE(dropped_at, ?), refreshed_at = ?, updated_at = ?, updated_by = ? WHERE id = ?",
				database.FormatTime(now), database.FormatTime(now), database.FormatTime(now), library.ActorText(by), titleID)
			return err
		})
		if err == nil && first {
			s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleDropped, TitleID: &titleID, JobID: jobID,
				Summary: t.Name + " is no longer known to " + sourceName(source) + "; kept and flagged", GroupKey: library.GroupKey(titleID)})
		}
		return out, err
	}
	if err != nil {
		return out, err
	}
	for k, v := range t.SourceIDs {
		if sh.IDs != nil && sh.IDs[k] == "" {
			sh.IDs[k] = v // a TVMaze id learnt earlier saves a search
		}
	}
	s.addAirTimes(metadata.NoCache(ctx), &sh)
	was := t.MetadataSource
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		applyShow(&t, sh)
		t.MetadataSource = source
		t.DroppedAt = nil
		t.RefreshedAt = &now
		if err := database.Update(tx, "titles", &t, by); err != nil {
			return err
		}
		var err error
		out.ShowSync, err = syncShowTx(ctx, tx, t, sh, defs, "", now, by)
		return err
	})
	if err != nil {
		return out, err
	}
	s.CachePoster(ctx, titleID, sh.Poster, by)
	if was != source {
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventSourceSwitched, TitleID: &titleID, JobID: jobID,
			Summary: t.Name + " now follows " + sourceName(source), Data: map[string]any{"from": was, "to": source}, GroupKey: library.GroupKey(titleID)})
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventTitleRefreshed, TitleID: &titleID, JobID: jobID,
		Summary: "Refreshed details of " + t.Name + " from " + sourceName(source), GroupKey: library.GroupKey(titleID),
		Data: map[string]any{"details_source": source, "status": t.Status, "seasons_added": out.SeasonsAdded, "episodes_added": out.EpisodesAdded,
			"episodes_changed": out.EpisodesChanged, "episodes_removed": out.EpisodesRemoved, "episodes_dropped": out.EpisodesDropped}})
	if out.SeasonsAdded+out.EpisodesAdded+out.EpisodesRemoved+out.EpisodesDropped > 0 {
		s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventEpisodesChanged, TitleID: &titleID, JobID: jobID,
			Summary: fmt.Sprintf("%s: %d new seasons, %d new episodes, %d removed, %d no longer listed but kept", t.Name,
				out.SeasonsAdded, out.EpisodesAdded, out.EpisodesRemoved, out.EpisodesDropped), GroupKey: library.GroupKey(titleID),
			Data: map[string]any{"seasons_added": out.SeasonsAdded, "episodes_added": out.EpisodesAdded, "episodes_removed": out.EpisodesRemoved, "episodes_dropped": out.EpisodesDropped}})
	}
	s.episodeGateEvents(ctx, t, out.Opened, out.Closed, by, jobID)
	return out, nil
}

func sourceName(source string) string {
	switch source {
	case "tmdb":
		return "TMDB"
	case "tvmaze":
		return "TVMaze"
	case "tvdb":
		return "TVDB"
	}
	return source
}

// episodeGateEvents records each episode Slot that crossed the gate.
func (s *Service) episodeGateEvents(ctx context.Context, t mediafiles.Title, opened, closed []int64, by domain.Actor, jobID *int64) {
	for _, list := range []struct {
		ids  []int64
		kind string
		to   mediafiles.SlotState
		says string
	}{{opened, domain.EventGateOpened, mediafiles.SlotWanted, " has aired: it is wanted now"}, {closed, domain.EventGateClosed, mediafiles.SlotNotOutYet, " has not aired yet: its air date moved"}} {
		for _, id := range list.ids {
			slot := id
			name := t.Name + " " + library.EpisodeLabel(ctx, s.Store.Read(), slot)
			s.Emit(ctx, &domain.Event{Actor: by, Kind: list.kind, TitleID: &t.ID, SlotID: &slot, JobID: jobID, Summary: name + list.says, GroupKey: library.GroupKey(t.ID)})
			s.Publish("slot", map[string]any{"id": slot, "state": list.to})
		}
	}
}

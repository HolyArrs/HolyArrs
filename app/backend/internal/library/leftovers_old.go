package library

import (
	"context"
	"path"
	"strings"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// OldEpisode is one episode as the old app lists it: its numbers there,
// the day it aired and its name.
type OldEpisode struct {
	Season  int    `json:"season"`
	Number  int    `json:"number"`
	AirDate string `json:"air_date,omitempty"` // YYYY-MM-DD
	Title   string `json:"title,omitempty"`
}

// PlaceByOldEpisodes places the waiting files of a show whose seasons
// differ, by the old app's episode list read at import (by file base
// name). A file is placed only when every episode it holds there aired on
// the same day under the same name as exactly one episode here; any other
// file keeps waiting. It returns how many were placed.
func (s *Service) PlaceByOldEpisodes(ctx context.Context, titleID int64, old map[string][]OldEpisode, by domain.Actor) (int, error) {
	if len(old) == 0 {
		return 0, nil
	}
	ls, err := mediafiles.TitleLeftovers(ctx, s.Store.Read(), titleID)
	if err != nil {
		return 0, err
	}
	eps, err := database.Select[mediafiles.Item](ctx, s.Store.Read(), "items", "title_id = ? AND kind = 'episode' AND dropped_at IS NULL", titleID)
	if err != nil {
		return 0, err
	}
	var items []LeftoverMatch
	for _, l := range ls {
		f := l.File
		if f.LeftoverReason == nil || *f.LeftoverReason != "seasons_differ" || len(f.SlotIDs) > 0 {
			continue
		}
		if ids := sameEpisodes(old[path.Base(f.RelativePath)], eps); ids != nil {
			items = append(items, LeftoverMatch{FileID: f.ID, EpisodeIDs: ids})
		}
	}
	if len(items) == 0 {
		return 0, nil
	}
	out, err := s.matchLeftovers(ctx, titleID, items, false, by, false)
	n := 0
	for _, o := range out {
		if o.Linked {
			n++
		}
	}
	return n, err
}

// sameEpisodes is the episode here for each old one (same air day, same
// name once case, spaces and punctuation are set aside), or nil when any
// old episode has none, or more than one, or no day or name.
func sameEpisodes(old []OldEpisode, eps []mediafiles.Item) []int64 {
	if len(old) == 0 {
		return nil
	}
	var ids []int64
	for _, o := range old {
		name := plainName(o.Title)
		if o.AirDate == "" || name == "" {
			return nil
		}
		var found []int64
		for _, e := range eps {
			if e.Name != nil && airDay(e) == o.AirDate && plainName(*e.Name) == name {
				found = append(found, e.ID)
			}
		}
		if len(found) != 1 {
			return nil
		}
		for _, id := range ids {
			if id == found[0] {
				return nil
			}
		}
		ids = append(ids, found[0])
	}
	return ids
}

// airDay is the day an episode aired (YYYY-MM-DD), "" when not known.
func airDay(e mediafiles.Item) string {
	if e.AirDay != nil && *e.AirDay != "" {
		return *e.AirDay
	}
	if e.AirDate != nil {
		return e.AirDate.Format("2006-01-02")
	}
	return ""
}

// plainName is a name with only its letters and digits, in lower case.
func plainName(n string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(n) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

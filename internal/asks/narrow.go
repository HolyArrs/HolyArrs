package asks

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
)

// ErrNotAskedPart is a season or album the approver kept that the ask
// never asked for: approving can only make an ask smaller.
var ErrNotAskedPart = errors.New("approve only seasons or albums the ask is for")

// narrow is the ask as the approver approves it: only the seasons
// (opt.Seasons) or albums (opt.Albums) kept from what was asked. Empty
// keeps the whole ask. A whole show or whole artist narrows to any of its
// own seasons or albums; a future-only show ask cannot narrow.
func (s *Service) narrow(ctx context.Context, a domain.Ask, opt ApproveOptions) (domain.Ask, error) {
	switch {
	case a.Kind == "show" && len(opt.Seasons) > 0:
		if a.FutureOnly {
			return a, ErrNotAskedPart
		}
		var want []int
		if len(a.Seasons) > 0 {
			for _, n := range opt.Seasons {
				if !slices.Contains(a.Seasons, n) {
					return a, ErrNotAskedPart
				}
				if !slices.Contains(want, n) {
					want = append(want, n)
				}
			}
			slices.Sort(want)
		} else {
			defs, err := library.ReadShowDefaults(ctx, s.Store.Read())
			if err != nil {
				return a, err
			}
			sh, err := s.Library.LookupShow(ctx, a.Ref, defs.Source)
			if err != nil {
				return a, err
			}
			if want, err = pickSeasons(opt.Seasons, sh.Seasons); err != nil {
				return a, err
			}
		}
		a.Seasons = want
	case a.Kind == "artist" && len(opt.Albums) > 0:
		var want []domain.AskAlbum
		if len(a.Albums) > 0 {
			for _, id := range opt.Albums {
				id = strings.ToLower(strings.TrimSpace(id))
				i := slices.IndexFunc(a.Albums, func(w domain.AskAlbum) bool { return strings.EqualFold(w.MBID, id) })
				if i < 0 {
					return a, ErrNotAskedPart
				}
				if !slices.ContainsFunc(want, func(w domain.AskAlbum) bool { return w.MBID == a.Albums[i].MBID }) {
					want = append(want, a.Albums[i])
				}
			}
			// Keep the ask's own order (oldest first).
			slices.SortStableFunc(want, func(x, y domain.AskAlbum) int {
				return slices.IndexFunc(a.Albums, func(w domain.AskAlbum) bool { return w.MBID == x.MBID }) -
					slices.IndexFunc(a.Albums, func(w domain.AskAlbum) bool { return w.MBID == y.MBID })
			})
		} else {
			if s.Library.Artists == nil {
				return a, library.ErrNoMusicSource
			}
			ar, err := s.Library.Artists.Artist(ctx, a.SourceIDs["musicbrainz"])
			if err != nil {
				return a, err
			}
			if want, err = pickAlbums(opt.Albums, ar.Albums); err != nil {
				return a, err
			}
		}
		a.Albums = want
	case len(opt.Seasons) > 0 || len(opt.Albums) > 0:
		return a, ErrNotAskedPart
	}
	return a, nil
}

package asks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
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
	case a.Kind == "show" && (len(opt.Seasons) > 0 || len(opt.Episodes) > 0):
		if a.FutureOnly {
			return a, ErrNotAskedPart
		}
		// The seasons the ask touches: an approver may keep any of them
		// whole, or keep any episodes of them (adding ones not asked for,
		// leaving asked ones out).
		asked := slices.Clone(a.Seasons)
		for _, e := range a.Episodes {
			asked = append(asked, e.Season)
		}
		want := []int{}
		if !wholeShow(a) {
			for _, e := range opt.Episodes {
				if !slices.Contains(asked, e.Season) {
					return a, ErrNotAskedPart
				}
			}
			for _, n := range opt.Seasons {
				if !slices.Contains(asked, n) {
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
			if _, err = pickEpisodes(opt.Episodes, nil, sh.Seasons); err != nil {
				return a, err
			}
		}
		eps, err := pickEpisodes(opt.Episodes, want, nil)
		if err != nil {
			return a, err
		}
		a.Seasons, a.Episodes = want, eps
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
	case len(opt.Seasons) > 0 || len(opt.Albums) > 0 || len(opt.Episodes) > 0 || len(opt.Tracks) > 0 && a.Kind != "artist":
		return a, ErrNotAskedPart
	}
	return a, nil
}

// ErrNotAlbumTrack is a kept song that is not on the album named with it.
var ErrNotAlbumTrack = errors.New("keep only songs of the album they are on")

// keepTracks keeps only the picked songs of albums in the artist's Library
// entry: the album's other songs are switched off (the album itself stays
// kept). Songs are Library track ids; an album must be on the artist.
func (s *Service) keepTracks(ctx context.Context, titleID int64, picks []AlbumTracks, by Person) error {
	if len(picks) == 0 {
		return nil
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, p := range picks {
			var partID int64
			err := tx.QueryRowContext(ctx, "SELECT id FROM parts WHERE title_id = ? AND kind = 'album' AND json_extract(source_ids, '$.musicbrainz') = ?",
				titleID, strings.ToLower(strings.TrimSpace(p.Album))).Scan(&partID)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotAlbumTrack
			} else if err != nil {
				return err
			}
			var n int
			for _, id := range p.Tracks {
				if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM items WHERE id = ? AND part_id = ? AND kind = 'track'", id, partID).Scan(&n); err != nil {
					return err
				}
				if n == 0 {
					return ErrNotAlbumTrack
				}
			}
			ts, who := store.FormatTime(s.now()), by.actor()
			whoText, _ := json.Marshal(who)
			if _, err := tx.ExecContext(ctx, "UPDATE items SET keep_up_to_date = 0, updated_at = ?, updated_by = ? WHERE part_id = ? AND kind = 'track'", ts, string(whoText), partID); err != nil {
				return err
			}
			for _, id := range p.Tracks {
				if _, err := tx.ExecContext(ctx, "UPDATE items SET keep_up_to_date = 1, updated_at = ?, updated_by = ? WHERE id = ?", ts, string(whoText), id); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
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
			sh, err := shows.New(s.Library).LookupShow(ctx, a.Ref, defs.Source)
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
	case a.Kind == "artist" && len(opt.Songs) > 0 && len(a.Albums) == 0:
		// Songs are kept per album: name the albums too.
		return a, ErrNotAlbumTrack
	case len(opt.Seasons) > 0 || len(opt.Albums) > 0 || len(opt.Episodes) > 0 || len(opt.Songs) > 0 && a.Kind != "artist":
		return a, ErrNotAskedPart
	}
	if a.Kind == "artist" && len(opt.Songs) > 0 {
		var err error
		if a.Albums, err = pickSongs(a.Albums, opt.Songs); err != nil {
			return a, err
		}
	}
	return a, nil
}

// ErrNotAlbumTrack is a kept song on an album the approval does not keep.
var ErrNotAlbumTrack = errors.New("keep only songs of albums the approval keeps")

// pickSongs puts the approver's song picks on the kept albums.
func pickSongs(albums []domain.AskAlbum, picks []AlbumSongs) ([]domain.AskAlbum, error) {
	out := slices.Clone(albums)
	for _, p := range picks {
		id := strings.ToLower(strings.TrimSpace(p.Album))
		i := slices.IndexFunc(out, func(w domain.AskAlbum) bool { return strings.EqualFold(w.MBID, id) })
		if i < 0 || len(p.Recordings) == 0 {
			return nil, ErrNotAlbumTrack
		}
		var rs []string
		for _, r := range p.Recordings {
			r = strings.ToLower(strings.TrimSpace(r))
			if r != "" && !slices.Contains(rs, r) {
				rs = append(rs, r)
			}
		}
		out[i].Songs, out[i].SongsDone = rs, false
	}
	return out, nil
}

// applySongPicks switches off the songs approvers left out, for approved
// artist asks whose albums' tracks are now in the Library (matched by
// recording id). Each album's pick is applied once.
func (s *Service) applySongPicks(ctx context.Context) error {
	as, err := database.Select[domain.Ask](ctx, s.Store.Read(), "asks", "kind = 'artist' AND state IN ('approved', 'available') AND title_id IS NOT NULL AND albums LIKE '%\"songs\"%' ORDER BY id")
	if err != nil {
		return err
	}
	for _, a := range as {
		if err := s.applySongs(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) applySongs(ctx context.Context, a domain.Ask) error {
	if a.TitleID == nil || !slices.ContainsFunc(a.Albums, func(w domain.AskAlbum) bool { return len(w.Songs) > 0 && !w.SongsDone }) {
		return nil
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		cur, err := get(ctx, tx, a.ID)
		if err != nil {
			return err
		}
		changed := false
		ts := database.FormatTime(s.now())
		who, _ := json.Marshal(domain.System)
		for i, al := range cur.Albums {
			if len(al.Songs) == 0 || al.SongsDone {
				continue
			}
			var partID int64
			err := tx.QueryRowContext(ctx, applySongsSQL, *cur.TitleID, al.MBID).Scan(&partID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			} else if err != nil {
				return err
			}
			var n int
			if err := tx.QueryRowContext(ctx, applySongsSQL2, partID).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				continue // the tracks come with the album's refresh
			}
			marks := strings.TrimSuffix(strings.Repeat("?,", len(al.Songs)), ",")
			args := []any{ts, string(who), partID}
			for _, r := range al.Songs {
				args = append(args, r)
			}
			if _, err := tx.ExecContext(ctx, applySongsSQL3+marks+"))", args...); err != nil {
				return err
			}
			cur.Albums[i].SongsDone, changed = true, true
		}
		if !changed {
			return nil
		}
		return database.Update(tx, "asks", &cur, domain.System)
	})
}

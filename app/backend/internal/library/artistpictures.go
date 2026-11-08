package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// KindArtistPictures fills missing artist pictures: one artist (queued
// right after an add) or, with no subject, every artist missing one (at
// start and on its 6-hour schedule). It never waits on MusicBrainz.
const KindArtistPictures = "artist_pictures"

// SourceServer is the picture source recorded in the Job result: our
// metadata server (the only one).
const SourceServer = "holyarrs"

// emptyArtwork matches a title with no artwork in SQL.
const emptyArtwork = "(artwork IS NULL OR artwork IN ('', '[]', 'null'))"

// FillArtistPictures fills what is missing of an artist's pictures and
// returns the source that filled the artwork ("" when none did or it was
// already there). Pictures come only from our metadata server: its stored photo, wide background and logo, by MusicBrainz
// id. One it has not stored yet fills nothing now; the 6-hour pass asks
// again. Artwork, backdrop or logo already set is never replaced.
func (s *Service) FillArtistPictures(ctx context.Context, titleID int64, by domain.Actor) (string, error) {
	var t mediafiles.Title
	if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil {
		return "", err
	}
	if t.Kind != "artist" {
		return "", ErrNotArtist
	}
	mbid := t.SourceIDs["musicbrainz"]
	// Artwork that is only a stand-in (a grey person, one flat colour) is
	// swapped for the server's photo, or removed when it has none.
	healed := StandInNone
	if s.artworkHasStandIn(t.Artwork) {
		var err error
		if healed, err = s.healArtistArt(ctx, titleID, t.Name, mbid, t.Artwork, by); err != nil {
			return "", err
		}
		if err := database.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil {
			return "", err
		}
	}
	// A photo the server was just asked for is not asked for again in this run.
	needArt := len(t.Artwork) == 0 && healed != StandInEmptied
	needWide := t.Backdrop == "" || t.Logo == ""
	if s.Artwork == nil || s.ImageServer == "" || !metadata.ValidMBID(mbid) || !needArt && !needWide {
		return "", nil
	}
	if needWide {
		wide, logo := "", ""
		if t.Backdrop == "" {
			wide = s.serverLink(ctx, titleID, "wide", mbid, "1920.webp")
		}
		if t.Logo == "" {
			logo = s.serverLink(ctx, titleID, "logo", mbid, "1200.png")
		}
		if wide != "" || logo != "" {
			s.fillWide(ctx, titleID, wide, logo, by)
		}
	}
	filled := ""
	if healed == StandInReplaced {
		filled = SourceServer
	}
	if needArt {
		if name := s.serverPicture(ctx, titleID, "artist", mbid, "500.webp"); name != "" && s.fillArt(ctx, titleID, name, by) {
			filled = SourceServer
		}
	}
	return filled, ctx.Err()
}

// serverPicture stores one of the metadata server's pictures of an artist
// and answers its cache name ("" when the server has not stored it yet; a
// redirect or stand-in is never kept).
func (s *Service) serverPicture(ctx context.Context, titleID int64, kind, mbid, file string) string {
	name, err := s.Artwork.FetchStored(ctx, strings.TrimRight(s.ImageServer, "/")+"/"+kind+"/"+mbid+"/"+file, nil)
	if err != nil {
		log.Printf("library: %s picture for artist %d: %v", kind, titleID, err)
		return ""
	}
	return name
}

// serverLink is serverPicture as a local link.
func (s *Service) serverLink(ctx context.Context, titleID int64, kind, mbid, file string) string {
	if name := s.serverPicture(ctx, titleID, kind, mbid, file); name != "" {
		return "/api/v1/artwork/" + url.PathEscape(name)
	}
	return ""
}

// fillArt keeps a stored picture (its cache name) as the artwork if the
// artwork is still empty; it says whether the picture is now the artwork.
func (s *Service) fillArt(ctx context.Context, titleID int64, name string, by domain.Actor) bool {
	var n int64
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE titles SET artwork = ?, updated_at = ?, updated_by = ? WHERE id = ? AND "+emptyArtwork,
			`["poster:`+name+`"]`, database.FormatTime(s.now()), ActorText(by), titleID)
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	if err != nil {
		log.Printf("library: picture for artist %d: %v", titleID, err)
	}
	return n > 0
}

// fillWide keeps a backdrop and logo where the artist has none yet.
func (s *Service) fillWide(ctx context.Context, titleID int64, backdrop, logo string, by domain.Actor) {
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE titles SET backdrop = CASE WHEN backdrop = '' THEN ? ELSE backdrop END,
			logo = CASE WHEN logo = '' THEN ? ELSE logo END, updated_at = ?, updated_by = ?
			WHERE id = ? AND ((backdrop = '' AND ? != '') OR (logo = '' AND ? != ''))`,
			backdrop, logo, database.FormatTime(s.now()), ActorText(by), titleID, backdrop, logo)
		return err
	})
	if err != nil {
		log.Printf("library: backdrop for artist %d: %v", titleID, err)
	}
}

// ArtistPictures says what one picture run did: how many artists got
// artwork, from which source, and which artist from which source; and what
// the one-time stand-in repair did before it.
type ArtistPictures struct {
	Looked  int              `json:"looked"`
	Filled  int              `json:"filled"`
	Sources map[string]int   `json:"sources"`
	By      map[int64]string `json:"by_title"`
	StandIn StandInRepair    `json:"stand_in_repair"`
}

// FillAllArtistPictures fills every artist missing artwork (and its
// backdrop and logo on the way).
func (s *Service) FillAllArtistPictures(ctx context.Context, by domain.Actor, r *jobs.Run) (ArtistPictures, error) {
	out := ArtistPictures{Sources: map[string]int{}, By: map[int64]string{}}
	if s.Artwork == nil {
		return out, nil
	}
	// Once per install, before the empty ones are filled: an artist whose
	// picture is only a stand-in is given the real photo, or none.
	repair, err := s.RepairArtistStandIns(ctx, by)
	if err != nil {
		return out, err
	}
	out.StandIn = repair
	ts, err := database.Select[mediafiles.Title](ctx, s.Store.Read(), "titles", "kind = 'artist' AND state != 'removed' AND "+emptyArtwork+" ORDER BY sort_name, id")
	if err != nil {
		return out, err
	}
	for i, t := range ts {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if r != nil {
			_ = r.Progress(ctx, float64(i)/float64(len(ts)), fmt.Sprintf("pictures for artist %d of %d", i+1, len(ts)))
		}
		out.Looked++
		src, err := s.FillArtistPictures(ctx, t.ID, by)
		if err != nil && !errors.Is(err, ErrNotArtist) {
			return out, err
		}
		if src != "" {
			out.Filled++
			out.Sources[src]++
			out.By[t.ID] = src
		}
	}
	return out, nil
}

func (s *Service) handleArtistPictures(ctx context.Context, r *jobs.Run) error {
	by := JobActor(r)
	if sub := r.Job.Subject; sub != nil && sub.Type == "title" && sub.ID != 0 {
		src, err := s.FillArtistPictures(ctx, sub.ID, by)
		if err != nil {
			return err
		}
		r.SetResult(map[string]any{"title_id": sub.ID, "source": src})
		return nil
	}
	out, err := s.FillAllArtistPictures(ctx, by, r)
	if err != nil {
		return err
	}
	r.SetResult(map[string]any{"looked": out.Looked, "filled": out.Filled, "sources": out.Sources, "by_title": out.By, "stand_in_repair": out.StandIn})
	return nil
}

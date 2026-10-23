package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// KindArtistPictures fills missing artist pictures: one artist (queued
// right after an add) or, with no subject, every artist missing one (at
// start and on its 6-hour schedule). It never waits on MusicBrainz.
const KindArtistPictures = "artist_pictures"

// CoverArtBase is the Cover Art Archive (album covers).
const CoverArtBase = "https://coverartarchive.org"

// Picture sources, as recorded in the Job result.
const (
	SourceAudioDB  = "theaudiodb"
	SourceDeezer   = "deezer"
	SourceCoverArt = "coverartarchive"
)

// emptyArtwork matches a title with no artwork in SQL.
const emptyArtwork = "(artwork IS NULL OR artwork IN ('', '[]', 'null'))"

// FillArtistPictures fills what is missing of an artist's pictures and
// returns the source that filled the artwork ("" when none did or it was
// already there). The sources, in order: TheAudioDB (only with a key set;
// by MusicBrainz id, then by name), which also gives the wide backdrop
// and the logo; then the Deezer artist or Wikidata picture the artist's
// MusicBrainz links name (never a name search, never an album cover). Artwork, backdrop or
// logo already set is never replaced. A source that is down is skipped.
func (s *Service) FillArtistPictures(ctx context.Context, titleID int64, by domain.Actor) (string, error) {
	var t domain.Title
	if err := store.Get(ctx, s.Store.Read(), "titles", titleID, &t); err != nil {
		return "", err
	}
	if t.Kind != "artist" {
		return "", ErrNotArtist
	}
	needArt := len(t.Artwork) == 0 && s.Artwork != nil
	needWide := (t.Backdrop == "" || t.Logo == "") && s.AudioDB.On(ctx)
	if !needArt && !needWide {
		return "", nil
	}
	mbid := t.SourceIDs["musicbrainz"]
	filled := ""
	if needWide || needArt && s.AudioDB.On(ctx) {
		p, err := s.AudioDB.Pictures(ctx, mbid, t.Name)
		if err != nil {
			log.Printf("library: TheAudioDB pictures for artist %d: %v", titleID, err)
		}
		// The backdrop and logo are fetched through the artwork cache (its
		// allowed hosts only) and kept as local links, never as the remote
		// address TheAudioDB gave.
		if wide, logo := s.cachedLink(ctx, titleID, p.Fanart), s.cachedLink(ctx, titleID, p.Logo); wide != "" || logo != "" {
			s.fillWide(ctx, titleID, wide, logo, by)
		}
		if needArt && p.Thumb != "" && s.fillArt(ctx, titleID, p.Thumb, by) {
			filled = SourceAudioDB
		}
	}
	if needArt && filled == "" && s.ArtistImages != nil {
		if pic := s.artistPictureByIDs(ctx, mbid); pic != "" && s.fillArt(ctx, titleID, pic, by) {
			filled = SourceDeezer
		}
	}
	// No album cover stands in for the artist: no picture beats a wrong one.
	return filled, ctx.Err()
}

// fillArt caches the picture and keeps it as the artwork if the artwork is
// still empty; it says whether the picture is now the artwork.
func (s *Service) fillArt(ctx context.Context, titleID int64, pic string, by domain.Actor) bool {
	name, err := s.Artwork.Fetch(ctx, pic)
	if err != nil {
		log.Printf("library: picture for artist %d: %v", titleID, err)
		return false
	}
	var n int64
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE titles SET artwork = ?, updated_at = ?, updated_by = ? WHERE id = ? AND "+emptyArtwork,
			`["poster:`+name+`"]`, store.FormatTime(s.now()), actorText(by), titleID)
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

// cachedLink fetches a picture through the artwork cache, which refuses
// any host it does not allow, and returns its local link ("" when refused
// or not there).
func (s *Service) cachedLink(ctx context.Context, titleID int64, pic string) string {
	if pic == "" || s.Artwork == nil {
		return ""
	}
	name, err := s.Artwork.Fetch(ctx, pic)
	if err != nil {
		log.Printf("library: picture for artist %d: %v", titleID, err)
		return ""
	}
	return "/api/v1/artwork/" + url.PathEscape(name)
}

// fillWide keeps a backdrop and logo where the artist has none yet.
func (s *Service) fillWide(ctx context.Context, titleID int64, backdrop, logo string, by domain.Actor) {
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE titles SET backdrop = CASE WHEN backdrop = '' THEN ? ELSE backdrop END,
			logo = CASE WHEN logo = '' THEN ? ELSE logo END, updated_at = ?, updated_by = ?
			WHERE id = ? AND ((backdrop = '' AND ? != '') OR (logo = '' AND ? != ''))`,
			backdrop, logo, store.FormatTime(s.now()), actorText(by), titleID, backdrop, logo)
		return err
	})
	if err != nil {
		log.Printf("library: backdrop for artist %d: %v", titleID, err)
	}
}

// ArtistPictures says what one picture run did: how many artists got
// artwork, from which source, and which artist from which source.
type ArtistPictures struct {
	Looked  int              `json:"looked"`
	Filled  int              `json:"filled"`
	Sources map[string]int   `json:"sources"`
	By      map[int64]string `json:"by_title"`
}

// FillAllArtistPictures fills every artist missing artwork (and its
// backdrop and logo on the way).
func (s *Service) FillAllArtistPictures(ctx context.Context, by domain.Actor, r *jobs.Run) (ArtistPictures, error) {
	out := ArtistPictures{Sources: map[string]int{}, By: map[int64]string{}}
	if s.Artwork == nil {
		return out, nil
	}
	ts, err := store.Select[domain.Title](ctx, s.Store.Read(), "titles", "kind = 'artist' AND state != 'removed' AND "+emptyArtwork+" ORDER BY sort_name, id")
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
	by := jobActor(r)
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
	r.SetResult(map[string]any{"looked": out.Looked, "filled": out.Filled, "sources": out.Sources, "by_title": out.By})
	return nil
}

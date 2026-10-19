package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// KindArtistPictures fills missing artist pictures: one artist (queued
// right after an add) or, with no subject, every artist missing one (at
// start and on its 6-hour schedule). It never waits on MusicBrainz.
const KindArtistPictures = "artist_pictures"

// CoverArtBase is the Cover Art Archive, whose album covers are the last
// picture source for an artist.
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
// and the logo; Deezer;
// the cover of the artist's earliest studio album. Artwork, backdrop or
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
		if p.Fanart != "" || p.Logo != "" {
			s.fillWide(ctx, titleID, p.Fanart, p.Logo, by)
		}
		if needArt && p.Thumb != "" && s.fillArt(ctx, titleID, p.Thumb, by) {
			filled = SourceAudioDB
		}
	}
	if needArt && filled == "" && s.ArtistImages != nil {
		pic, err := s.ArtistImages.Image(ctx, t.Name)
		if err != nil {
			log.Printf("library: Deezer picture for artist %d: %v", titleID, err)
		}
		if pic != "" && s.fillArt(ctx, titleID, pic, by) {
			filled = SourceDeezer
		}
	}
	if needArt && filled == "" && s.CoverArtBase != "" {
		ids, err := studioAlbums(ctx, s.Store.Read(), titleID)
		if err != nil {
			return "", err
		}
		for _, rg := range ids {
			if s.fillArt(ctx, titleID, strings.TrimRight(s.CoverArtBase, "/")+"/release-group/"+url.PathEscape(rg)+"/front-500", by) {
				filled = SourceCoverArt
				break
			}
		}
	}
	return filled, ctx.Err()
}

// studioAlbums are the MusicBrainz ids of an artist's studio albums out
// already, earliest first (at most three).
func studioAlbums(ctx context.Context, db *sql.DB, titleID int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT json_extract(source_ids, '$.musicbrainz') FROM parts
		WHERE title_id = ? AND kind = 'album' AND lower(COALESCE(release_type, '')) = 'album' AND secondary_types = '[]'
		AND dropped_at IS NULL AND json_extract(source_ids, '$.musicbrainz') IS NOT NULL
		ORDER BY release_date IS NULL, release_date, id LIMIT 3`, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
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

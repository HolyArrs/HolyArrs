package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// What an ask is made of is stored once, when it is asked for: a show's
// episode lists per season and an album's song list. The Requests panel
// reads them with the ask, so opening a season or an album never waits on
// an outside source.

// PartEpisode is one episode of a stored season list.
type PartEpisode struct {
	Number int    `json:"number"`
	Name   string `json:"name,omitempty"`
	AirDay string `json:"air_day,omitempty"`
}

// PartSong is one song of a stored album list.
type PartSong struct {
	Disc        int    `json:"disc"`
	Number      int    `json:"number"`
	Name        string `json:"name"`
	LengthMS    int    `json:"length_ms,omitempty"`
	RecordingID string `json:"recording_id,omitempty"`
}

// Parts are an ask's stored lists: episodes by season number, songs by
// album MusicBrainz id. A list not stored yet is missing.
type Parts struct {
	Seasons map[string][]PartEpisode `json:"seasons"`
	Albums  map[string][]PartSong    `json:"albums"`
}

// partsBusy keeps one fetch per ask at a time.
var partsBusy sync.Map

// storePartsLater fetches and stores an open ask's lists in the
// background, once (lists already stored are not fetched again).
func (s *Service) storePartsLater(id int64) {
	if !s.PrefetchParts {
		return
	}
	if _, busy := partsBusy.LoadOrStore(id, true); busy {
		return
	}
	go func() {
		defer partsBusy.Delete(id)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := s.StoreParts(ctx, id); err != nil {
			log.Printf("asks: store lists of ask %d: %v", id, err)
		}
	}()
}

// BackfillParts stores the lists of every waiting ask that lacks them; run
// once at start.
func (s *Service) BackfillParts(ctx context.Context) {
	if !s.PrefetchParts {
		return
	}
	as, err := database.Select[domain.Ask](ctx, s.Store.Read(), "asks", "state = 'asked' AND kind IN ('show', 'artist') ORDER BY id")
	if err != nil {
		log.Printf("asks: backfill lists: %v", err)
		return
	}
	for _, a := range as {
		var n int
		_ = s.Store.Read().QueryRowContext(ctx, backfillPartsSQL, "show:"+a.Ref+":", "show:"+a.Ref+":\xff").Scan(&n)
		if a.Kind == "show" && n > 0 || a.Kind == "artist" && s.partStored(ctx, "askalbums:"+strconv.FormatInt(a.ID, 10)) {
			continue // stored before
		}
		if err := s.StoreParts(ctx, a.ID); err != nil {
			log.Printf("asks: backfill lists of ask %d: %v", a.ID, err)
		}
	}
}

func showKey(ref string, season int) string { return "show:" + ref + ":" + strconv.Itoa(season) }
func albumKey(mbid string) string           { return "album:" + mbid }

func (s *Service) partStored(ctx context.Context, key string) bool {
	var n int
	_ = s.Store.Read().QueryRowContext(ctx, partStoredSQL, key).Scan(&n)
	return n > 0
}

func (s *Service) putPart(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, putPartSQL,
			key, string(b), database.FormatTime(s.now()))
		return err
	})
}

// StoreParts fetches and stores the lists an ask needs that are not stored
// yet: every season of a show, every asked album of an artist (for a
// whole-artist ask, its albums and EPs).
func (s *Service) StoreParts(ctx context.Context, id int64) error {
	a, err := get(ctx, s.Store.Read(), id)
	if err != nil {
		return err
	}
	switch a.Kind {
	case "show":
		defs, err := library.ReadShowDefaults(ctx, s.Store.Read())
		if err != nil {
			return err
		}
		sh, err := shows.New(s.Library).LookupShow(ctx, a.Ref, defs.Source)
		if err != nil {
			return err
		}
		by := map[int][]PartEpisode{}
		for _, e := range sh.Episodes {
			by[e.Season] = append(by[e.Season], PartEpisode{Number: e.Number, Name: e.Name, AirDay: e.AirDay})
		}
		for _, se := range sh.Seasons {
			eps := by[se.Number]
			if eps == nil {
				eps = []PartEpisode{}
			}
			// Episode lists change while a season airs: store them again.
			if err := s.putPart(ctx, showKey(a.Ref, se.Number), eps); err != nil {
				return err
			}
		}
	case "artist":
		if s.Library.Artists == nil {
			return nil
		}
		var ids []string
		for _, al := range a.Albums {
			ids = append(ids, al.MBID)
		}
		if len(ids) == 0 {
			ar, err := s.Library.Artists.Artist(ctx, a.SourceIDs["musicbrainz"])
			if err != nil {
				return err
			}
			for _, al := range ar.Albums {
				if (al.PrimaryType == "album" || al.PrimaryType == "ep") && len(al.SecondaryTypes) == 0 {
					ids = append(ids, al.MBID)
				}
			}
		}
		if err := s.putPart(ctx, "askalbums:"+strconv.FormatInt(a.ID, 10), ids); err != nil {
			return err
		}
		for _, mbid := range ids {
			if s.partStored(ctx, albumKey(mbid)) {
				continue
			}
			ps, err := s.Library.Artists.Pressings(ctx, mbid)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return err
				}
				log.Printf("asks: songs of album %s: %v", mbid, err)
				continue
			}
			songs := []PartSong{}
			if p := mainPressing(ps); p != nil {
				for _, t := range p.Tracks {
					songs = append(songs, PartSong{Disc: t.Disc, Number: t.Number, Name: t.Name, LengthMS: t.LengthMS, RecordingID: t.RecordingID})
				}
			}
			if len(songs) == 0 {
				continue // nothing to show yet; a later look tries again
			}
			if err := s.putPart(ctx, albumKey(mbid), songs); err != nil {
				return err
			}
		}
	}
	return nil
}

// mainPressing is the usual edition of an album: an official, plain one
// with the most common track count, oldest first.
func mainPressing(ps []metadata.Pressing) *metadata.Pressing {
	count := map[int]int{}
	for _, p := range ps {
		if len(p.Tracks) > 0 {
			count[len(p.Tracks)]++
		}
	}
	var best *metadata.Pressing
	key := func(p metadata.Pressing) []int {
		off := 0
		if p.Status == "official" || p.Status == "" {
			off = 2
		}
		if p.Disambiguation == "" {
			off++
		}
		return []int{off, count[len(p.Tracks)]}
	}
	for i := range ps {
		p := ps[i]
		if len(p.Tracks) == 0 {
			continue
		}
		if best == nil {
			best = &ps[i]
			continue
		}
		c := slices.Compare(key(p), key(*best))
		if c > 0 || c == 0 && p.Date != "" && (best.Date == "" || p.Date < best.Date) {
			best = &ps[i]
		}
	}
	return best
}

// AskParts reads an ask's stored lists.
func (s *Service) AskParts(ctx context.Context, id int64) (Parts, error) {
	out := Parts{Seasons: map[string][]PartEpisode{}, Albums: map[string][]PartSong{}}
	a, err := get(ctx, s.Store.Read(), id)
	if err != nil {
		return out, err
	}
	prefix := "album:"
	if a.Kind == "show" {
		prefix = "show:" + a.Ref + ":"
	}
	var keys []string
	switch a.Kind {
	case "show":
		rows, err := s.Store.Read().QueryContext(ctx, askPartsSQL, prefix, prefix+"\xff")
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			var k, body string
			if err := rows.Scan(&k, &body); err != nil {
				return out, err
			}
			var eps []PartEpisode
			if json.Unmarshal([]byte(body), &eps) == nil {
				out.Seasons[k[len(prefix):]] = eps
			}
		}
		return out, rows.Err()
	case "artist":
		for _, al := range a.Albums {
			keys = append(keys, al.MBID)
		}
		if len(keys) == 0 {
			// A whole-artist ask: the albums stored for it.
			var body string
			if s.Store.Read().QueryRowContext(ctx, askPartsSQL2, "askalbums:"+strconv.FormatInt(a.ID, 10)).Scan(&body) == nil {
				_ = json.Unmarshal([]byte(body), &keys)
			}
		}
		for _, k := range keys {
			var body string
			if err := s.Store.Read().QueryRowContext(ctx, askPartsSQL3, albumKey(k)).Scan(&body); err != nil {
				continue
			}
			var songs []PartSong
			if json.Unmarshal([]byte(body), &songs) == nil {
				out.Albums[k] = songs
			}
		}
	}
	return out, nil
}

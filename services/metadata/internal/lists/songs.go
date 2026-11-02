package lists

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Song charts come first from the scheduled chart reader (the kworb
// package's daily country charts, read and kept on its own schedule):
// each song gets its album from the official Spotify API (GET /v1/tracks,
// 50 a call, each track kept for good) and is then matched like any other
// list entry. The source's own song charts are the backup, read only when
// the chart reader's last read of a region failed or did not parse; they
// are left out of the plan's budget, so their share goes to the rest.

// SongChart is one region's chart as the chart reader kept it.
type SongChart struct {
	Day string // the chart's own date
	At  time.Time
	// Failed: the reader's last read of the region failed or did not
	// parse (Rows, if any, are older).
	Failed bool
	Rows   []SongRow
}

// SongRow is one song of a chart.
type SongRow struct {
	Rank          int
	Move          string // "=", "+3", "-2", "NEW" or "RE"
	Title, Artist string
	ArtistID      string // Spotify artist id
	TrackID       string // Spotify track id
	Days, Peak    int
	Streams       int64
}

// TrackAlbum is a song's album as the Spotify API answers it.
type TrackAlbum struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"` // album, single, compilation
	ReleaseDate string `json:"release_date"`
	Image       string `json:"image"`
}

// tracksBatch is how many tracks one Spotify call reads.
const tracksBatch = 50

// albumsOf answers the albums of tracks: kept ones, then the rest asked
// 50 a call (a failed call leaves its tracks without an album this time).
func (s *Service) albumsOf(ctx context.Context, ids []string) map[string]TrackAlbum {
	out := map[string]TrackAlbum{}
	var ask []string
	for _, id := range ids {
		var b []byte
		if s.db.QueryRowContext(ctx, `SELECT body FROM tracks WHERE id = ?`, id).Scan(&b) == nil {
			var a TrackAlbum
			if json.Unmarshal(b, &a) == nil {
				out[id] = a
				continue
			}
		}
		ask = append(ask, id)
	}
	if s.opts.Tracks == nil {
		return out
	}
	for len(ask) > 0 && ctx.Err() == nil {
		n := min(tracksBatch, len(ask))
		got, err := s.opts.Tracks(ctx, ask[:n])
		if err != nil {
			s.opts.Log("lists: track albums: %v", err)
			break
		}
		for id, a := range got {
			out[id] = a
			if b, err := json.Marshal(a); err == nil {
				_, _ = s.db.Exec(`INSERT OR REPLACE INTO tracks (id, body) VALUES (?, ?)`, id, b)
			}
		}
		ask = ask[n:]
	}
	return out
}

// moveStatus reads a chart move as the previous rank and a status.
func moveStatus(rank int, move string) (prev int, status string) {
	switch m := strings.TrimSpace(move); {
	case m == "NEW":
		return 0, "new"
	case m == "RE":
		return 0, "back"
	case m == "=":
		return rank, "same"
	case strings.HasPrefix(m, "+"):
		n, _ := strconv.Atoi(m[1:])
		return rank + n, "up"
	case strings.HasPrefix(m, "-"):
		n, _ := strconv.Atoi(m[1:])
		return rank - n, "down"
	}
	return 0, ""
}

// songEntries turns a kept chart into list entries: the move as the
// previous rank and status, the day it entered (from its days on the
// chart), the day it was last number one (today's chart, else carried
// from the region's earlier stored days), and its album.
func (s *Service) songEntries(ctx context.Context, key string, c SongChart) []Entry {
	ids := make([]string, 0, len(c.Rows))
	for _, r := range c.Rows {
		if r.TrackID != "" {
			ids = append(ids, r.TrackID)
		}
	}
	albums := s.albumsOf(ctx, ids)
	peaks := map[string]string{}
	if old, _ := s.latest(key); len(old) > 0 {
		for _, e := range old {
			if e.PeakDate != "" {
				peaks[entryKey(e)] = e.PeakDate
			}
		}
	}
	day, _ := time.Parse("2006-01-02", c.Day)
	out := make([]Entry, 0, len(c.Rows))
	for _, r := range c.Rows {
		e := Entry{Rank: r.Rank, Peak: r.Peak, Days: r.Days, Streams: r.Streams, Title: r.Title, Artist: r.Artist, ExtArtist: r.ArtistID}
		e.Prev, e.Status = moveStatus(r.Rank, r.Move)
		if r.Days > 0 && !day.IsZero() {
			e.EntryDate = day.AddDate(0, 0, -(r.Days - 1)).Format("2006-01-02")
		}
		switch {
		case r.Rank == 1:
			e.PeakDate = c.Day
		case r.Peak == 1:
			e.PeakDate = peaks[entryKey(e)]
		}
		if a, ok := albums[r.TrackID]; ok {
			e.Album, e.AlbumType, e.ExtAlbum, e.ReleaseDate, e.Image = a.Name, a.Type, a.ID, a.ReleaseDate, a.Image
		}
		out = append(out, e)
	}
	return out
}

// songsFirst reads a due song chart from the chart reader. done is true
// when the reader's chart was taken (or it has not read the region yet:
// wait for it); false sends the read to the source's own chart.
func (s *Service) songsFirst(ctx context.Context, src Source) (done, changed bool) {
	c, ok := s.opts.Songs(src.Region)
	if !ok {
		return true, false // not read by the chart reader yet
	}
	if c.Failed {
		return false, false
	}
	var at int64
	_ = s.db.QueryRow(`SELECT fetched_at FROM lists_source WHERE key = ?`, src.Key).Scan(&at)
	if !c.At.After(time.Unix(at, 0)) || len(c.Rows) == 0 {
		return true, false // nothing newer than what is stored
	}
	es := s.songEntries(ctx, src.Key, c)
	body, err := json.Marshal(es)
	if err != nil {
		return true, false
	}
	return true, s.store(src, body)
}

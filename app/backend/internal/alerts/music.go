package alerts

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Artist names the artist a message is about.
type Artist struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	MusicBrainz string `json:"musicbrainz,omitempty"`
}

// Track names one track a message is about, with its album.
type Track struct {
	Album  string `json:"album"`
	Year   int    `json:"year,omitempty"`
	Disc   int    `json:"disc"`
	Number int    `json:"number"`
	Name   string `json:"name,omitempty"`
	Discs  int    `json:"-"`
}

// Label is "track 03", or "disc 2 track 01" on an album of more than one
// disc.
func (t Track) Label() string {
	if t.Discs > 1 {
		return fmt.Sprintf("disc %d track %02d", t.Disc, t.Number)
	}
	return fmt.Sprintf("track %02d", t.Number)
}

// albumName is "Album (Year)".
func (t Track) albumName() string {
	if t.Year > 0 {
		return fmt.Sprintf("%s (%d)", t.Album, t.Year)
	}
	return t.Album
}

// tracksOf lists the tracks of Slots, by album, disc and number, each once.
func tracksOf(ctx context.Context, db database.Querier, slotIDs []int64) []Track {
	var out []Track
	seen := map[int64]bool{}
	for _, id := range slotIDs {
		var itemID int64
		var disc, num, maxDisc sql.NullInt64
		var name, album, released sql.NullString
		err := db.QueryRowContext(ctx, `SELECT i.id, json_extract(i.numbers, '$.disc'), json_extract(i.numbers, '$.track'), i.name, p.name, p.release_date,
			(SELECT max(json_extract(o.numbers, '$.disc')) FROM items o WHERE o.part_id = i.part_id AND o.kind = 'track' AND o.dropped_at IS NULL)
			FROM slots s JOIN items i ON i.id = s.item_id LEFT JOIN parts p ON p.id = i.part_id WHERE s.id = ? AND i.kind = 'track'`, id).
			Scan(&itemID, &disc, &num, &name, &album, &released, &maxDisc)
		if err != nil || seen[itemID] {
			continue
		}
		seen[itemID] = true
		t := Track{Album: album.String, Disc: max(1, int(disc.Int64)), Number: int(num.Int64), Name: name.String, Discs: max(1, int(maxDisc.Int64))}
		if d, err := time.Parse(time.RFC3339Nano, released.String); err == nil {
			t.Year = d.Year()
		} else if len(released.String) >= 4 {
			fmt.Sscanf(released.String[:4], "%d", &t.Year)
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Album != b.Album {
			return a.Album < b.Album
		}
		if a.Disc != b.Disc {
			return a.Disc < b.Disc
		}
		return a.Number < b.Number
	})
	return out
}

// trackWords names tracks for a subject: "Album (Year)" when they are of
// one album (with the track when it is one), else how many.
func trackWords(ts []Track) string { return trackWordsIn("", ts) }

// trackWordsIn is trackWords in an app language.
func trackWordsIn(lang string, ts []Track) string {
	if len(ts) == 0 {
		return ""
	}
	for _, t := range ts[1:] {
		if t.Album != ts[0].Album {
			return say(lang, "tracks", "n", strconv.Itoa(len(ts)))
		}
	}
	if len(ts) == 1 {
		return ts[0].albumName() + " " + ts[0].Label()
	}
	return ts[0].albumName()
}

// trackLines lists tracks with their names, one per line.
func trackLines(ts []Track) string {
	lines := make([]string, len(ts))
	for i, t := range ts {
		lines[i] = strings.TrimSpace(t.Label() + " " + t.Name)
		if len(ts) > 1 && ts[0].Album != ts[len(ts)-1].Album {
			lines[i] = t.Album + ": " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

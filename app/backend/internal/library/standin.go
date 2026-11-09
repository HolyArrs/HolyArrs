package library

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// An artist "photo" that only stands in for a photo: the metadata server's
// grey stand-in, the generic grey person outline an old source sent, or a
// picture that is one flat colour. Such a picture is never saved as an
// artist's artwork, and one saved before this check is replaced by the real
// photo (or removed, so the screen shows its normal fallback).
//
// serverPlaceholders (placeholderheal.go) lists the known stand-ins by
// SHA-256; standInReason adds the two rules that need no list.

const (
	// flatSpread is the most the red, green or blue values of a flat picture
	// may spread (standard deviation, of 255): one colour is 0, the grey
	// person outline measures 10, a real photo is far above 30.
	flatSpread = 12.0
	// oneColour is the share of the picture one colour may fill (the colours
	// are grouped to 16 shades a channel) before it counts as flat.
	oneColour = 0.95
	// standInGrid is how many points a side the picture is sampled at.
	standInGrid = 64
	// standInMaxPixels is the largest picture judged (4096 × 4096).
	standInMaxPixels = 4096 * 4096
)

// StandInReason says why b (a picture file) is only a stand-in for an
// artist's photo, or "" when it is a real picture or cannot be judged (a
// file that is not an image is left to the caller's own checks).
func StandInReason(b []byte) string {
	sum := sha256.Sum256(b)
	if serverPlaceholders[hex.EncodeToString(sum[:])] {
		return "a known stand-in picture"
	}
	// Size first: a picture over standInMaxPixels is not decoded (a small
	// file can claim a huge size), and is left unjudged.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > standInMaxPixels {
		return ""
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return ""
	}
	return flatReason(img)
}

// flatReason judges a decoded picture by sampling it on a grid: "one flat
// colour" (no spread at all), "a flat picture" (spread so small that only a
// faint outline is left, like the grey person) or "one colour fills nearly
// all of it".
func flatReason(img image.Image) string {
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		return "an empty picture"
	}
	var sum, sumSq [3]float64
	shades := map[[3]uint8]int{}
	n := 0
	for gy := 0; gy < standInGrid; gy++ {
		for gx := 0; gx < standInGrid; gx++ {
			x := b.Min.X + (2*gx+1)*b.Dx()/(2*standInGrid)
			y := b.Min.Y + (2*gy+1)*b.Dy()/(2*standInGrid)
			r, g, bl, _ := img.At(x, y).RGBA()
			c := [3]float64{float64(r >> 8), float64(g >> 8), float64(bl >> 8)}
			for i, v := range c {
				sum[i] += v
				sumSq[i] += v * v
			}
			shades[[3]uint8{uint8(c[0]) >> 4, uint8(c[1]) >> 4, uint8(c[2]) >> 4}]++
			n++
		}
	}
	spread := 0.0
	for i := range sum {
		mean := sum[i] / float64(n)
		spread = math.Max(spread, math.Sqrt(math.Max(sumSq[i]/float64(n)-mean*mean, 0)))
	}
	top := 0
	for _, c := range shades {
		top = max(top, c)
	}
	switch {
	case spread == 0:
		return "one flat colour"
	case spread <= flatSpread:
		return "a flat picture with no photo in it"
	case float64(top)/float64(n) >= oneColour:
		return "one colour fills nearly all of it"
	}
	return ""
}

// isArtistPhotoURL says url is the metadata server's photo of an artist
// (<server>/v1/img/artist/<id>/<file>): the pictures the stand-in check
// applies to (a logo is mostly one colour and is fine).
func isArtistPhotoURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && strings.Contains(u.Path, "/v1/img/artist/")
}

// stored reads a cache file by name without marking it used; ok is false
// for a name that is not a cache file or a file that is not there.
func (c *ArtworkCache) stored(name string) ([]byte, bool) {
	if c == nil || c.Dir == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") || len(name) < 36 {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(c.Dir, name))
	return b, err == nil
}

// entryIsStandIn says one artwork entry of a title ("poster:<cache name>"
// or a bare cache name) is a stored stand-in picture.
func (c *ArtworkCache) entryIsStandIn(entry string) (string, bool) {
	name := entry
	if _, rest, ok := strings.Cut(entry, ":"); ok {
		name = rest
	}
	b, ok := c.stored(name)
	if !ok {
		return "", false
	}
	why := StandInReason(b)
	return why, why != ""
}

// artworkStandIns splits an artist's artwork into the entries to keep and
// whether any entry was only a stand-in.
func (s *Service) artworkStandIns(art []string) (keep []string, bad bool) {
	keep = []string{}
	for _, e := range art {
		if _, is := s.Artwork.entryIsStandIn(e); is {
			bad = true
			continue
		}
		keep = append(keep, e)
	}
	return keep, bad
}

// artworkHasStandIn says an artist's artwork holds a stored stand-in.
func (s *Service) artworkHasStandIn(art []string) bool {
	if s.Artwork == nil || len(art) == 0 {
		return false
	}
	_, bad := s.artworkStandIns(art)
	return bad
}

// artworkMissing says a picture fetch error means the server has no photo
// (a redirect, a 404 or another refusal, or a stand-in); any other error
// says nothing about the photo (network, rate limit, server trouble).
func artworkMissing(err error) bool {
	if errors.Is(err, ErrArtworkMoved) || errors.Is(err, ErrArtworkRejected) {
		return true
	}
	var se *ArtworkStatusError
	return errors.As(err, &se) && !se.Temporary()
}

// serverPhoto stores the metadata server's photo of an artist and answers
// its cache name. The error says why not (artworkMissing tells "the server
// has none" from "could not ask"). A stored photo that is itself a stand-in
// (kept before this check existed) is removed and counts as none.
func (s *Service) serverPhoto(ctx context.Context, mbid string) (string, error) {
	if s.Artwork == nil || s.ImageServer == "" || !metadata.ValidMBID(mbid) {
		return "", ErrArtworkURL
	}
	name, err := s.Artwork.FetchStored(ctx, strings.TrimRight(s.ImageServer, "/")+"/artist/"+mbid+"/500.webp", nil)
	if err != nil {
		return "", err
	}
	if b, ok := s.Artwork.stored(name); ok && StandInReason(b) != "" {
		_ = os.Remove(filepath.Join(s.Artwork.Dir, name))
		_ = os.Remove(filepath.Join(s.Artwork.Dir, checkName(name)))
		return "", ErrArtworkRejected
	}
	return name, nil
}

// StandInOutcome says what healArtistArt did.
type StandInOutcome string

const (
	StandInNone     StandInOutcome = ""         // the artwork is real (or empty): nothing to do
	StandInReplaced StandInOutcome = "replaced" // the server's real photo took its place
	StandInEmptied  StandInOutcome = "emptied"  // no photo to put there: left empty, the screen's fallback shows
	StandInTrimmed  StandInOutcome = "trimmed"  // a stand-in was dropped; a real picture stays
)

// healArtistArt looks at one artist's stored artwork and, when an entry is
// only a stand-in, drops it and puts the server's photo there (or leaves
// the artwork empty when the server has none or cannot be asked: the
// ordinary picture pass fills an empty one later). The change is made only
// if the artwork is still what was read.
func (s *Service) healArtistArt(ctx context.Context, id int64, name, mbid string, art []string, by domain.Actor) (StandInOutcome, error) {
	if s.Artwork == nil || len(art) == 0 {
		return StandInNone, nil
	}
	keep, bad := s.artworkStandIns(art)
	if !bad {
		return StandInNone, nil
	}
	out := StandInTrimmed
	if len(keep) == 0 {
		out = StandInEmptied
		if pic, err := s.serverPhoto(ctx, mbid); err == nil {
			keep, out = []string{"poster:" + pic}, StandInReplaced
		} else if ctx.Err() != nil {
			return StandInNone, ctx.Err()
		} else if !artworkMissing(err) {
			log.Printf("library: stand-in picture of artist %d (%s): server not asked: %v", id, name, err)
		}
	}
	oldJSON, _ := json.Marshal(art)
	newJSON, _ := json.Marshal(keep)
	var n int64
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("UPDATE titles SET artwork = ?, updated_at = ?, updated_by = ? WHERE id = ? AND artwork = ?",
			string(newJSON), database.FormatTime(s.now()), ActorText(by), id, string(oldJSON))
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	if err != nil || n == 0 {
		return StandInNone, err
	}
	log.Printf("library: artist %d (%s): stand-in picture %s", id, name, out)
	return out, nil
}

// standInRepairedSetting records that the one-time repair has run.
const standInRepairedSetting = "artist_standins_repaired"

// StandInRepair says what the one-time repair did, by artist name.
type StandInRepair struct {
	Looked   int      `json:"looked"`
	Replaced []string `json:"replaced,omitempty"`
	Emptied  []string `json:"emptied,omitempty"`
	Trimmed  []string `json:"trimmed,omitempty"`
}

// Fixed is how many artists the repair changed.
func (r StandInRepair) Fixed() int { return len(r.Replaced) + len(r.Emptied) + len(r.Trimmed) }

// RepairArtistStandIns runs once per install: every Library artist whose
// stored artwork is only a stand-in (a known stand-in, one flat colour or the
// grey person outline) gets the server's real photo, or no artwork when the
// server has none, so the screen's own fallback shows. Artists saved before
// stand-ins were refused all need it. It runs inside the artist picture
// Job (at start and every six hours) and is recorded when done.
func (s *Service) RepairArtistStandIns(ctx context.Context, by domain.Actor) (StandInRepair, error) {
	var out StandInRepair
	if s.Artwork == nil || s.Artwork.Dir == "" {
		return out, nil
	}
	var done string
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", standInRepairedSetting).Scan(&done)
	if done != "" {
		return out, nil
	}
	ts, err := database.Select[mediafiles.Title](ctx, s.Store.Read(), "titles", "kind = 'artist' AND state != 'removed' AND NOT "+emptyArtwork+" ORDER BY sort_name, id")
	if err != nil {
		return out, err
	}
	for _, t := range ts {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		out.Looked++
		switch res, err := s.healArtistArt(ctx, t.ID, t.Name, t.SourceIDs["musicbrainz"], t.Artwork, by); {
		case err != nil:
			return out, fmt.Errorf("artist %d: %w", t.ID, err)
		case res == StandInReplaced:
			out.Replaced = append(out.Replaced, t.Name)
		case res == StandInEmptied:
			out.Emptied = append(out.Emptied, t.Name)
		case res == StandInTrimmed:
			out.Trimmed = append(out.Trimmed, t.Name)
		}
	}
	now, who := database.FormatTime(s.now()), ActorText(by)
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			standInRepairedSetting, now, now, who, now, who)
		return err
	})
	if out.Fixed() > 0 {
		log.Printf("library: stand-in artist pictures repaired: %d replaced %v, %d emptied %v, %d trimmed %v",
			len(out.Replaced), out.Replaced, len(out.Emptied), out.Emptied, len(out.Trimmed), out.Trimmed)
	}
	return out, err
}

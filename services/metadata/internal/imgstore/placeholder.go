package imgstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Artist photos that are no photo (audit 2026-10-29): a source's blank
// default picture, or one of the artist's own album covers sent as the
// photo. Both are refused when stored, so the chain goes on to the next
// source; PurgeBadArtistPhotos clears the ones stored before.

// ErrNotAPhoto: the picture is a blank default or an album cover; the
// chain tries its next source.
var ErrNotAPhoto = errors.New("imgstore: not an artist photo")

// MinArtistCard is the smallest artist card (500 px WebP) taken as a
// photo: a real photo is ten times this, the sources' blank defaults
// (a grey square, a silhouette) are 840 to 1,900 bytes.
const MinArtistCard = 2048

// blankCards are the 500 px cards of the sources' blank default pictures
// found in the store on 2026-10-29 (sha256 of the stored file), each
// shared by many unrelated artists.
var blankCards = map[string]bool{
	"7f35eb2563ea85b594129efa83c132392e5dc2038be3a2d3e3513ed47262afa7": true, // Deezer, 139 artists
	"fb0c3c9163607415ea99e2fadf2d553b31facf094a93be15628857280ae578fb": true, // Deezer, 12
	"ae8fd2a6b906f178c573a3b0a72c3a9173850fd127d872c246265d3548eecd8b": true, // Spotify, 6
	"1d62b9b29543fcc3e58894f76a1f493bcc943c544972982189873dbae6148ec3": true, // Deezer, 5
	"fc8f9f82892179620e8e1ec956f1f1068e4424ccf5bf00ad1b70979c49883522": true, // Deezer, 4
	"3e27adf9e84cc1d0dbad29e2724099e546e4cddd8b3b523984a7a91083bac901": true, // Deezer, 3
}

func sumHex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// artistCardIssue answers why an artist card is no photo: "placeholder"
// (a known blank, or too small to be a photo), "cover" (byte for byte one
// of the artist's album covers), or "".
func (s *Store) artistCardIssue(ctx context.Context, id string, card []byte) string {
	if len(card) < MinArtistCard || blankCards[sumHex(card)] {
		return "placeholder"
	}
	if s.o.AlbumsOf == nil {
		return ""
	}
	for _, al := range s.o.AlbumsOf(ctx, id) {
		p := s.SlotPath(KindAlbum, al, SlotCard)
		if fi, err := os.Stat(p); err != nil || fi.Size() != int64(len(card)) {
			continue // only a file of the same size can be the same
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Equal(b, card) {
			return "cover"
		}
	}
	return ""
}

// SetAlbumsOf sets Options.AlbumsOf (before the store is in use).
func (s *Store) SetAlbumsOf(f func(ctx context.Context, artist string) []string) { s.o.AlbumsOf = f }

// PurgeResult counts what PurgeBadArtistPhotos dropped.
type PurgeResult struct {
	Checked      int `json:"checked"`
	Placeholders int `json:"placeholders"`
	Covers       int `json:"covers"`
}

// PurgeBadArtistPhotos drops the stored artist photos that are a blank
// default or one of the artist's album covers. Each dropped picture's
// files are copied to backupDir/<mbid>/ and its row to backupDir/rows.json
// first; requeue then asks for the artist's photo again (the store-time
// check makes the chain pass over the same picture).
func (s *Store) PurgeBadArtistPhotos(ctx context.Context, backupDir string, requeue func(id string)) (PurgeResult, error) {
	var res PurgeResult
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordCols+` FROM images WHERE kind=?`, KindArtist)
	if err != nil {
		return res, err
	}
	var recs []Record
	for rows.Next() {
		if r, err := scanRecord(rows); err == nil {
			recs = append(recs, r)
		}
	}
	rows.Close()
	type dropped struct {
		Record
		Issue string `json:"issue"`
	}
	var out []dropped
	for _, r := range recs {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		card, err := os.ReadFile(s.SlotPath(KindArtist, r.ID, SlotCard))
		if err != nil {
			continue
		}
		res.Checked++
		issue := s.artistCardIssue(ctx, r.ID, card)
		if issue == "" {
			continue
		}
		if err := copyDir(s.Dir(KindArtist, r.ID), filepath.Join(backupDir, r.ID)); err != nil {
			return res, err
		}
		out = append(out, dropped{r, issue})
		if issue == "cover" {
			res.Covers++
		} else {
			res.Placeholders++
		}
	}
	if len(out) == 0 {
		return res, nil
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return res, err
	}
	if err := os.WriteFile(filepath.Join(backupDir, "rows.json"), b, 0o644); err != nil {
		return res, err
	}
	for _, d := range out {
		s.drop(ctx, KindArtist, d.ID)
		if requeue != nil {
			requeue(d.ID)
		}
	}
	return res, nil
}

func copyDir(from, to string) error {
	es, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	for _, e := range es {
		if e.IsDir() {
			continue
		}
		in, err := os.Open(filepath.Join(from, e.Name()))
		if err != nil {
			return err
		}
		o, err := os.Create(filepath.Join(to, e.Name()))
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(o, in)
		in.Close()
		if cerr := o.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

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
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"image"
	_ "image/jpeg" // decoders for the picture check
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrBadArtwork refuses a picture a plugin sent.
var ErrBadArtwork = errors.New("send a JPEG or PNG picture of 100 to 4000 pixels a side and at most 8 MB")

const pluginArtMax = 8 << 20

// pluginArtPixels caps a plugin picture at 16 megapixels.
const pluginArtPixels = 16_000_000

// keepName is the hidden file that keeps a picture out of the cache trim:
// a picture a plugin sent can't be fetched again.
func keepName(name string) string { return ".keep-" + name }

// SetPluginArtwork stores a picture a plugin sent as a title's poster or
// backdrop. Only the bytes are taken (never a link), checked as a real
// JPEG or PNG of sane size, kept out of the cache trim and put first in the
// title's artwork for its kind.
func (s *Service) SetPluginArtwork(ctx context.Context, titleID int64, kind string, pic []byte, by domain.Actor) error {
	if kind != "poster" && kind != "backdrop" {
		return fmt.Errorf("%w: kind is poster or backdrop", ErrBadArtwork)
	}
	if s.Artwork == nil || s.Artwork.Dir == "" {
		return errors.New("pictures are not set up")
	}
	if len(pic) == 0 || len(pic) > pluginArtMax {
		return ErrBadArtwork
	}
	// Only the header is read: a full decode of a huge picture could run the
	// server out of memory. The pixel cap bounds what any later resize reads.
	cfg, format, err := image.DecodeConfig(bytes.NewReader(pic))
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width < 100 || cfg.Height < 100 || cfg.Width > 4000 || cfg.Height > 4000 ||
		cfg.Width*cfg.Height > pluginArtPixels {
		return ErrBadArtwork
	}
	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}
	sum := sha256.Sum256(pic)
	name := hex.EncodeToString(sum[:16]) + ext
	dir := s.Artwork.Dir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".tmp-"+name)
	if err := os.WriteFile(tmp, pic, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, keepName(name)), nil, 0o600); err != nil {
		return err
	}
	old, err := mediafiles.TitleArtwork(ctx, s.Store.Read(), titleID)
	if err != nil {
		return err
	}
	art := []string{kind + ":" + name}
	for _, a := range old {
		if !strings.HasPrefix(a, kind+":") && !(kind == "poster" && !strings.Contains(a, ":")) {
			art = append(art, a)
		}
	}
	raw, _ := json.Marshal(art)
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		return mediafiles.SetTitleArtwork(ctx, tx, titleID, string(raw), database.FormatTime(s.now()), ActorText(by))
	}); err != nil {
		return err
	}
	tid := titleID
	summary := "New poster"
	if kind == "backdrop" {
		summary = "New background picture"
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: "title_artwork_set", TitleID: &tid, Summary: summary})
	return nil
}

package accounts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // registers the PNG decoder
	"io"
	"net/http"
	"strconv"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // registers the WebP decoder

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const (
	avatarMaxUpload = 5 << 20 // bytes
	avatarSide      = 256     // pixels, the stored picture is a square
	avatarMaxSide   = 8000    // pixels, longest side we agree to decode
	avatarMaxPixels = 25e6    // a 24 MP camera photo fits; bounds decode memory
)

var errAvatarImage = errors.New("send a JPEG, PNG or WebP picture")

var avatarSlot = make(chan struct{}, 1)

func (a *Auth) avatarRoutes(m *http.ServeMux) {
	m.HandleFunc("PUT /api/v1/account/avatar", a.putAvatar)
	m.HandleFunc("DELETE /api/v1/account/avatar", a.deleteAvatar)
	m.HandleFunc("GET /api/v1/users/{id}/avatar", a.getAvatar)
}

// avatarURL is the address of a person's picture, or nil when they have none.
// The tag in the query string changes with the picture, so caches let go.
func avatarURL(id int64, etag sql.NullString) *string {
	if !etag.Valid || etag.String == "" {
		return nil
	}
	u := "/api/v1/users/" + strconv.FormatInt(id, 10) + "/avatar?v=" + etag.String
	return &u
}

func (a *Auth) avatarOf(ctx context.Context, id int64) *string {
	etag, err := avatarTag(ctx, a.s.Read(), id)
	if err != nil {
		return nil
	}
	return avatarURL(id, etag)
}

// makeAvatar decodes an upload, crops the middle square, shrinks it to
// 256 by 256 and writes a JPEG.
func makeAvatar(raw []byte) ([]byte, error) {
	// One decode at a time: a big picture can need a few hundred MB.
	avatarSlot <- struct{}{}
	defer func() { <-avatarSlot }()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 {
		return nil, errAvatarImage
	}
	if cfg.Width > avatarMaxSide || cfg.Height > avatarMaxSide || float64(cfg.Width)*float64(cfg.Height) > avatarMaxPixels {
		return nil, errors.New("that picture is too large; use one under 8000 pixels on each side")
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errAvatarImage
	}
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	square := image.Rect(x0, y0, x0+side, y0+side)
	// A transparent PNG lands on white, since JPEG has no transparency.
	dst := image.NewRGBA(image.Rect(0, 0, avatarSide, avatarSide))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, square, xdraw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 86}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (a *Auth) putAvatar(w http.ResponseWriter, r *http.Request) {
	who, _ := webserver.From(r.Context())
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, avatarMaxUpload))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, http.StatusRequestEntityTooLarge, "that picture is over 5 MB")
			return
		}
		fail(w, http.StatusBadRequest, "could not read the picture")
		return
	}
	jpg, err := makeAvatar(raw)
	if err != nil {
		if err == errAvatarImage || len(raw) == 0 {
			fail(w, http.StatusUnsupportedMediaType, errAvatarImage.Error())
			return
		}
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	sum := sha256.Sum256(jpg)
	etag := hex.EncodeToString(sum[:6])
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return putAvatar(r.Context(), tx, who.UserID, jpg, etag, database.FormatTime(a.now()))
	})
	if err != nil {
		serverError(w, "avatar", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"avatar": avatarURL(who.UserID, sql.NullString{String: etag, Valid: true})})
}

func (a *Auth) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	who, _ := webserver.From(r.Context())
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		return deleteAvatarOf(r.Context(), tx, who.UserID)
	})
	if err != nil {
		serverError(w, "avatar", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) getAvatar(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, "no picture")
		return
	}
	jpg, etag, err := avatarOfActive(r.Context(), a.s.Read(), id)
	if err != nil {
		fail(w, http.StatusNotFound, "no picture")
		return
	}
	tag := `"` + etag + `"`
	h := w.Header()
	h.Set("ETag", tag)
	h.Set("Cache-Control", "private, max-age=3600")
	h.Set("X-Content-Type-Options", "nosniff")
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", "image/jpeg")
	h.Set("Content-Length", strconv.Itoa(len(jpg)))
	_, _ = w.Write(jpg)
}

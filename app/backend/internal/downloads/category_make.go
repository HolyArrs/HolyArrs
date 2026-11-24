package downloads

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrCannotMakeCategory: this kind of Downloader has no way for HolyArrs to
// make a category in it.
var ErrCannotMakeCategory = errors.New("HolyArrs cannot make a category in this kind of Downloader; add it in the download app")

// categoryMaker is a client HolyArrs can make a category in.
type categoryMaker interface {
	MakeCategory(ctx context.Context, name string) error
}

// MakeCategory makes the category name in SABnzbd with its default
// settings (its folder follows the completed folder), as set_config does
// from SABnzbd's own Categories page.
func (s *Sabnzbd) MakeCategory(ctx context.Context, name string) error {
	if c, err := checkCategory(name, "sabnzbd"); err != nil || c == "" || c != name || name == "*" {
		return ErrCategory
	}
	if _, err := s.call(ctx, "set_config", url.Values{"section": {"categories"}, "name": {name}}, true); err != nil {
		return err
	}
	if ok, err := s.HasCategory(ctx, name); err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("%w (%s)", ErrNoCategory, name)
		}
		return err
	}
	return nil
}

// MakeCategory makes the category name in qBittorrent with no save path of
// its own, so it saves where qBittorrent saves by default. One that is
// there already is fine.
func (q *Qbittorrent) MakeCategory(ctx context.Context, name string) error {
	if name == "" || len(name) > 64 || strings.ContainsAny(name, `/\`) {
		return ErrCategory
	}
	_, err := q.call(ctx, http.MethodPost, "torrents/createCategory", url.Values{"category": {name}, "savePath": {""}}, http.StatusConflict)
	return err
}

// rawEngine is d's client without the guard and read only wrappers.
func rawEngine(e Engine) Engine {
	if g, ok := e.(*guardedEngine); ok {
		e = g.Engine
	}
	return unwrapReadOnly(e)
}

// MakeCategory makes name, one of d's categories, in its client. It needs
// "HolyArrs can change this Downloader".
func (m *Manager) MakeCategory(ctx context.Context, d domain.Downloader, name string) error {
	if !hasCategory(d.Categories(), name) {
		return ErrCategory
	}
	if readOnlyFor(d) {
		return ErrReadOnly
	}
	e, err := m.engine(ctx, d)
	if err != nil {
		return err
	}
	mk, ok := rawEngine(e).(categoryMaker)
	if !ok {
		return ErrCannotMakeCategory
	}
	return mk.MakeCategory(ctx, name)
}

// madeImported makes c in s when an import set it on d and d may be
// changed; it says whether c is there now.
func madeImported(ctx context.Context, s *Sabnzbd, d domain.Downloader, c string) bool {
	if readOnlyFor(d) || !hasCategory(d.CategoriesImported, c) {
		return false
	}
	return s.MakeCategory(ctx, c) == nil
}

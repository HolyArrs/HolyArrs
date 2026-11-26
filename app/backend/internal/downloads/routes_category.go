package downloads

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// makeCategory is "Create it" on a Test that found a category missing: it
// makes one of the Downloader's own categories in its client.
func (a *Web) makeCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !webserver.Decode(w, r, &in, "send name, one of this Downloader's categories") {
		return
	}
	var d domain.Downloader
	if err := database.Get(r.Context(), a.Store.Read(), "downloaders", id, &d); err != nil || d.Removed {
		downloaderError(w, database.ErrNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	switch err := a.Manager().MakeCategory(ctx, d, in.Name); {
	case errors.Is(err, ErrReadOnly), errors.Is(err, ErrCannotMakeCategory):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrCategory):
		webserver.Fail(w, http.StatusBadRequest, ErrCategory.Error())
	case err != nil:
		_, text := explainDownloader(err, d.URL, d.AllowLocal)
		webserver.Fail(w, http.StatusBadGateway, text)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// missingCategory adds to a Test's answer the category it found missing and
// whether HolyArrs may make it there.
func missingCategory(out map[string]any, err error, d domain.Downloader) {
	if !errors.Is(err, ErrNoCategory) {
		return
	}
	name := categoryInError(err)
	if name == "" {
		return
	}
	out["category"] = name
	// make_off: only the setting stops it; a kind HolyArrs can't make one on
	// (NZBGet, Transmission, Deluge) gets neither, so the page doesn't point at a setting that won't help.
	can := d.Adapter == "sabnzbd" || d.Adapter == "qbittorrent"
	off := readOnlyFor(d)
	out["can_make"] = can && !off
	out["make_off"] = can && off
}

// clientCategories is the client's category list for a Downloader, with the
// one that fits each kind (movie, show, artist). listed is false when the
// client can't be read; the page then keeps plain text boxes.
func (a *Web) clientCategories(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	var d domain.Downloader
	if err := database.Get(r.Context(), a.Store.Read(), "downloaders", id, &d); err != nil || d.Removed {
		downloaderError(w, database.ErrNotFound)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	have, err := a.Manager().ClientCategories(ctx, d)
	if err != nil || have == nil {
		have = []string{}
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"listed": err == nil, "categories": have, "picks": CategoryPicks(have)})
}

// settleCategories fits a just saved Downloader's categories to its
// client and keeps any picks. It never fails the save.
func (a *Web) settleCategories(r *http.Request, d domain.Downloader, pick bool) domain.Downloader {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 8*time.Second)
	defer cancel()
	picked := a.Manager().SettleCategories(ctx, d, pick)
	if len(picked) == 0 {
		return d
	}
	in := Input{Name: d.Name}
	for kind, c := range picked {
		v := c
		switch kind {
		case "movie":
			in.CategoryMovies = &v
		case "show":
			in.CategoryShows = &v
		case "artist":
			in.CategoryMusic = &v
		}
	}
	if nd, err := Change(ctx, a.Store, a.Secrets, d.ID, in, actor(r)); err == nil {
		a.Manager().Forget(d.ID)
		return nd
	}
	return d
}

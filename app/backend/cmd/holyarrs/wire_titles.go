package main

import (
	"context"
	"errors"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// SubtitleFileView is one main File with its subtitles.
type SubtitleFileView = subtitles.FileView

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// SlotView, TitleView, VersionView and ArtistInfo are the titleview module's views.
type (
	SlotView    = discover.SlotView
	TitleView   = discover.TitleView
	VersionView = discover.VersionView
	ArtistInfo  = discover.ArtistInfo
)

// showError answers show refusals; anything else goes to library.WriteError.
func showError(w http.ResponseWriter, err error) { shows.ShowError(w, err, library.WriteError) }

// EpisodeSlot and EpisodeView are the shows module's episode views.
type (
	EpisodeSlot = shows.EpisodeSlot
	EpisodeView = shows.EpisodeView
)

// ShowInfo is what a show's page needs besides the shared Title fields.
type ShowInfo = shows.Info

// StateLabels are the words people see for Slot states.
var StateLabels = mediafiles.StateLabels

// artworkLinks is discover.ArtworkLinks.
var artworkLinks = discover.ArtworkLinks

func (a *handlers) titleView(ctx context.Context, t mediafiles.Title) (TitleView, error) {
	return a.tv().View(ctx, t)
}

// tv is the title view builder over this API's services.
func (a *handlers) tv() *discover.ViewHandlers {
	return discover.NewView(discover.ViewDeps{Store: a.s.Store, Asks: a.s.Asks, Live: a.s.Live})
}

// publishTitle sends the changed title to live pages (discover.PublishTitle).
func (a *handlers) publishTitle(r *http.Request, id int64) { a.tv().PublishTitle(r.Context(), id) }

func (a *handlers) listTitles(w http.ResponseWriter, r *http.Request) {
	ts, err := mediafiles.SearchTitles(r.Context(), a.s.Store, r.URL.Query().Get("q"))
	if errors.Is(err, mediafiles.ErrTitleSearchQuery) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if r.URL.Query().Has("brief") {
		a.listTitlesBrief(w, r, ts)
		return
	}
	out := []TitleView{}
	who, signed := webserver.From(r.Context())
	manage := !signed || who.Has(webserver.PermManageLibrary)
	ctx, err := discover.WithTitleFacts(discover.WithLookupMemo(r.Context()), a.s.Store.Read())
	if err == nil && len(ts) > 0 {
		ctx, err = a.tv().WithTitleBatch(ctx)
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for _, t := range ts {
		if p, ok := parts.ForTitleKind(t.Kind); ok && !a.partIsOn(p) && !a.s.Parts.SafeMode {
			continue // its part is off: hidden, not removed (safe mode lists every title)
		}
		if !a.titleAllowed(r, t) {
			continue // above a Kid's age-rating limit
		}
		v, err := a.titleView(ctx, t)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if v.Artist != nil {
			v.Artist.Bio = mediafiles.KeptArtistBio(r.Context(), v.Artist.MBID, t.Name, a.mh().SideKept)
		}
		if !manage {
			v.Copy = nil
		}
		out = append(out, v)
	}
	webserver.WriteTaggedJSON(w, r, map[string]any{"titles": out})
}

func (a *handlers) titleID(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool) {
	return a.tv().TitleID(w, r)
}

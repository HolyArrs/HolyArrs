package arrapi

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// seriesIn is what an app sends to add or change a show. Other fields
// come from the Library or the details source, as in the old apps.
type seriesIn struct {
	ID               int64  `json:"id"`
	TvdbID           int64  `json:"tvdbId"`
	Title            string `json:"title"`
	QualityProfileID int64  `json:"qualityProfileId"`
	Seasons          []struct {
		SeasonNumber int  `json:"seasonNumber"`
		Monitored    bool `json:"monitored"`
	} `json:"seasons"`
	Monitored       *bool  `json:"monitored"`
	MonitorNewItems string `json:"monitorNewItems"`
	RootFolderPath  string `json:"rootFolderPath"`
	SeriesType      string `json:"seriesType"`
	AddOptions      struct {
		SearchForMissingEpisodes bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// addSeries adds a show and keeps the seasons sent as monitored up to
// date. With searchForMissingEpisodes the search waits for the folder
// scan, so episodes already on disk are not grabbed again.
func (h *handler) addSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in seriesIn
	if !decode(w, r, &in) {
		return
	}
	if in.TvdbID <= 0 {
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "send the show's TVDB id", AttemptedValue: in.TvdbID})
		return
	}
	if id, err := library.ShowByIDs(ctx, h.Store.Read(), map[string]string{"tvdb": itoa(in.TvdbID)}); err != nil {
		internalError(w, err)
		return
	} else if id != 0 {
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "this show is already in the HolyArrs Library", AttemptedValue: in.TvdbID, ErrorCode: "AlreadyInLibrary"})
		return
	}
	folder, ok := h.folderByPath(ctx, in.RootFolderPath)
	if !ok {
		refuse(w, failure{PropertyName: "RootFolderPath", ErrorMessage: "no TV show Library folder has this path", AttemptedValue: in.RootFolderPath})
		return
	}
	// A missing monitored counts as false, as in the old apps.
	keep := in.Monitored != nil && *in.Monitored
	opt := shows.ShowAddOptions{LibraryFolderID: folder, PickRulesID: in.QualityProfileID, KeepUpToDate: &keep,
		NewSeasons: "all", EpisodeOrder: episodeOrder(in.SeriesType), SearchOnAdd: in.AddOptions.SearchForMissingEpisodes}
	if strings.EqualFold(in.MonitorNewItems, "none") {
		opt.NewSeasons = "none"
	}
	if len(in.Seasons) == 0 {
		opt.Seasons = library.SeasonsAll // no list means every season but the specials
	} else {
		opt.SeasonNumbers = []int{}
		for _, se := range in.Seasons {
			if se.Monitored {
				opt.SeasonNumbers = append(opt.SeasonNumbers, se.SeasonNumber)
			}
		}
	}
	id, _, err := shows.New(h.Library).AddShow(ctx, "tvdb:"+itoa(in.TvdbID), opt, actor(r))
	var dup *shows.ShowAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "this show is already in the HolyArrs Library", AttemptedValue: in.TvdbID, ErrorCode: "AlreadyInLibrary"})
		return
	case errors.Is(err, shows.ErrShowNeverAdd):
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "this show is on HolyArrs' Never-add list", AttemptedValue: in.TvdbID})
		return
	case errors.Is(err, metadata.ErrShowNotFound), errors.Is(err, metadata.ErrNoTVDBKey):
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "no show with this TVDB id was found", AttemptedValue: in.TvdbID})
		return
	case err != nil:
		if f, ok := addFailure(err, in.QualityProfileID, in.RootFolderPath); ok {
			refuse(w, f)
			return
		}
		internalError(w, err)
		return
	}
	s, _, err := h.oneSeries(ctx, id)
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Location", "/"+h.side+"/api/v3/series/"+itoa(id))
	writeJSON(w, http.StatusCreated, s)
}

// addFailure turns a Library refusal of the folder or Pick rules into a
// failed check the app can show.
func addFailure(err error, profile int64, folder string) (failure, bool) {
	var fe *library.FolderError
	switch {
	case errors.Is(err, library.ErrNoPickRules), errors.Is(err, library.ErrNotVideoRules):
		return failure{PropertyName: "QualityProfileId", ErrorMessage: err.Error(), AttemptedValue: profile}, true
	case errors.As(err, &fe), errors.Is(err, library.ErrNoShowFolder), errors.Is(err, library.ErrNotShowFolder),
		errors.Is(err, library.ErrNoFolder), errors.Is(err, library.ErrNotMovieFolder):
		return failure{PropertyName: "RootFolderPath", ErrorMessage: err.Error(), AttemptedValue: folder}, true
	}
	return failure{}, false
}

// putSeries changes a show already in the Library. The id comes from the
// path, or from the body as Seerr sends it. Only the show and season
// switches, the new-season choice and the quality profile are taken.
// Switching a season switches all its episodes, as in the old apps. The
// folder, show type and tags in the body are ignored.
func (h *handler) putSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in seriesIn
	if !decode(w, r, &in) {
		return
	}
	if v := r.PathValue("id"); v != "" {
		in.ID = atoi64(v)
	}
	t, err := h.title(ctx, in.ID, "show")
	if err != nil {
		internalError(w, err)
		return
	}
	by := actor(r)
	var c library.TitleChange
	if in.Monitored != nil && *in.Monitored != t.KeepUpToDate {
		c.KeepUpToDate = in.Monitored
	}
	if p := strings.ToLower(in.MonitorNewItems); (p == "all" || p == "none") && p != t.NewPartsPolicy {
		c.NewSeasons = &p
	}
	mains, err := h.mainVersions(ctx, []int64{t.ID})
	if err != nil {
		internalError(w, err)
		return
	}
	if in.QualityProfileID > 0 && in.QualityProfileID != mains[t.ID].PickRulesID {
		c.PickRulesID = &in.QualityProfileID
	}
	if c != (library.TitleChange{}) {
		if _, err := h.Library.Edit(ctx, t.ID, c, false, by); err != nil {
			if f, ok := addFailure(err, in.QualityProfileID, in.RootFolderPath); ok {
				refuse(w, f)
				return
			}
			internalError(w, err)
			return
		}
	}
	parts, err := database.Select[mediafiles.Part](ctx, h.Store.Read(), "parts", "title_id = ? AND kind = 'season'", t.ID)
	if err != nil {
		internalError(w, err)
		return
	}
	for _, se := range in.Seasons {
		for _, p := range parts {
			if p.Number != nil && *p.Number == se.SeasonNumber && p.KeepUpToDate != se.Monitored {
				if err := shows.New(h.Library).SetSeason(ctx, t.ID, se.SeasonNumber, se.Monitored, by); err != nil {
					internalError(w, err)
					return
				}
			}
		}
	}
	s, _, err := h.oneSeries(ctx, t.ID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s)
}

// deleteTitle takes a show or movie out of the Library. The row stays as
// removed, so its id is never reused. deleteFiles takes the files too,
// but only when the setting allows it, and they go to the holding folder
// first like every delete. An import exclusion puts the title on the
// Never-add list. A gone id answers 404, which Seerr reads as already
// removed.
func (h *handler) deleteTitle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	kind := "movie"
	if h.side == "sonarr" {
		kind = "show"
	}
	if _, err := h.title(ctx, id, kind); err != nil {
		internalError(w, err)
		return
	}
	c := library.RemoveChoice{NeverAdd: queryBool(r, "addImportExclusion") || queryBool(r, "addImportListExclusion")}
	c.DeleteFiles = queryBool(r, "deleteFiles") && h.DeleteFiles != nil && h.DeleteFiles(ctx)
	_, err := h.Library.Remove(ctx, id, c, true, actor(r))
	switch {
	case errors.Is(err, library.ErrAlreadyRemoved):
		notFound(w)
	case err != nil:
		log.Printf("arrapi: remove title %d: %v", id, err)
		message(w, http.StatusConflict, removeReason(err))
	default:
		writeJSON(w, http.StatusOK, map[string]any{})
	}
}

// removeReason says in plain words why a delete was refused. Unknown
// errors get a short generic line so no inner detail leaks out.
func removeReason(err error) string {
	switch {
	case errors.Is(err, library.ErrProtectedCopy):
		return library.ErrProtectedCopy.Error()
	case strings.Contains(err.Error(), "to the holding folder"):
		return "a file could not be moved to the holding folder"
	default:
		return "this title could not be removed"
	}
}

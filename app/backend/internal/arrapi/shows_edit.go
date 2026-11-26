package arrapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// seriesIn is what an app sends to add or change a show. Other fields
// come from the Library or the details source, as in the old apps.
type seriesIn struct {
	ID               int64   `json:"id"`
	TvdbID           int64   `json:"tvdbId"`
	Title            string  `json:"title"`
	QualityProfileID looseID `json:"qualityProfileId"`
	Seasons          []struct {
		SeasonNumber int  `json:"seasonNumber"`
		Monitored    bool `json:"monitored"`
	} `json:"seasons"`
	Monitored       *bool  `json:"monitored"`
	MonitorNewItems string `json:"monitorNewItems"`
	RootFolderPath  string `json:"rootFolderPath"`
	SeriesType      string `json:"seriesType"`
	// Tags are tag ids, as numbers or number text.
	Tags       []looseID `json:"tags"`
	AddOptions struct {
		Monitor                  string `json:"monitor"`
		SearchForMissingEpisodes bool   `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// Duplicate refusals keep the old apps' phrase, which Pulsarr reads as
// "already there".
const (
	showAlreadyAdded  = "this show has already been added to the HolyArrs Library"
	movieAlreadyAdded = "this movie has already been added to the HolyArrs Library"
)

// addSeries adds a show and keeps the seasons of the season choice, or
// the seasons sent as monitored, up to date. With searchForMissingEpisodes
// the search waits for the folder scan, so episodes already on disk are
// not grabbed again. The tags sent are put on the show.
func (h *handler) addSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in seriesIn
	if !decode(w, r, &in) {
		return
	}
	id, f, err := h.addOneSeries(ctx, in, actor(r))
	if f != nil {
		refuse(w, *f)
		return
	}
	if err != nil {
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

// addOneSeries adds one show. A refusal the app can show comes back as a
// failure; anything else as an error.
func (h *handler) addOneSeries(ctx context.Context, in seriesIn, by domain.Actor) (int64, *failure, error) {
	if in.TvdbID <= 0 {
		return 0, &failure{PropertyName: "TvdbId", ErrorMessage: "send the show's TVDB id", AttemptedValue: in.TvdbID}, nil
	}
	if in.QualityProfileID < 0 {
		return 0, &failure{PropertyName: "QualityProfileId", ErrorMessage: "send the Pick rules id as a number"}, nil
	}
	if id, err := library.ShowByIDs(ctx, h.Store.Read(), map[string]string{"tvdb": itoa(in.TvdbID)}); err != nil {
		return 0, nil, err
	} else if id != 0 {
		return 0, &failure{PropertyName: "TvdbId", ErrorMessage: showAlreadyAdded, AttemptedValue: in.TvdbID, ErrorCode: "AlreadyInLibrary"}, nil
	}
	folder, ok := h.folderByPath(ctx, in.RootFolderPath)
	if !ok {
		return 0, &failure{PropertyName: "RootFolderPath", ErrorMessage: "no TV show Library folder has this path", AttemptedValue: in.RootFolderPath}, nil
	}
	// A missing monitored counts as false, as in the old apps.
	keep := in.Monitored != nil && *in.Monitored
	opt := shows.ShowAddOptions{LibraryFolderID: folder, PickRulesID: int64(in.QualityProfileID), KeepUpToDate: &keep,
		NewSeasons: "all", EpisodeOrder: episodeOrder(in.SeriesType), SearchOnAdd: in.AddOptions.SearchForMissingEpisodes}
	if strings.EqualFold(in.MonitorNewItems, "none") {
		opt.NewSeasons = "none"
	}
	if given, noSearch := seasonPlan(in.AddOptions.Monitor, h.now(), &opt); given {
		opt.SearchOnAdd = opt.SearchOnAdd && !noSearch
	} else if len(in.Seasons) == 0 {
		opt.Seasons = library.SeasonsAll // no list means every season but the specials
	} else {
		opt.SeasonNumbers = []int{}
		for _, se := range in.Seasons {
			if se.Monitored {
				opt.SeasonNumbers = append(opt.SeasonNumbers, se.SeasonNumber)
			}
		}
	}
	id, _, err := shows.New(h.Library).AddShow(ctx, "tvdb:"+itoa(in.TvdbID), opt, by)
	var dup *shows.ShowAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		return 0, &failure{PropertyName: "TvdbId", ErrorMessage: showAlreadyAdded, AttemptedValue: in.TvdbID, ErrorCode: "AlreadyInLibrary"}, nil
	case errors.Is(err, shows.ErrShowNeverAdd):
		return 0, &failure{PropertyName: "TvdbId", ErrorMessage: "this show is on HolyArrs' Never-add list", AttemptedValue: in.TvdbID}, nil
	case errors.Is(err, metadata.ErrShowNotFound), errors.Is(err, metadata.ErrNoTVDBKey):
		return 0, &failure{PropertyName: "TvdbId", ErrorMessage: "no show with this TVDB id was found", AttemptedValue: in.TvdbID}, nil
	case err != nil:
		if f, ok := addFailure(err, int64(in.QualityProfileID), in.RootFolderPath); ok {
			return 0, &f, nil
		}
		return 0, nil, err
	}
	if err := linkTags(ctx, h.Store, id, tagIDs(in.Tags)); err != nil {
		return 0, nil, err
	}
	return id, nil, nil
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
	if f, err := h.putSeriesOne(ctx, t, in, actor(r)); f != nil {
		refuse(w, *f)
		return
	} else if err != nil {
		internalError(w, err)
		return
	}
	s, _, err := h.oneSeries(ctx, t.ID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s)
}

// putSeriesOne applies the changes in to one show. A refusal the app can show
// comes back as a failure.
func (h *handler) putSeriesOne(ctx context.Context, t mediafiles.Title, in seriesIn, by domain.Actor) (*failure, error) {
	var c library.TitleChange
	if in.Monitored != nil && *in.Monitored != t.KeepUpToDate {
		c.KeepUpToDate = in.Monitored
	}
	if p := strings.ToLower(in.MonitorNewItems); (p == "all" || p == "none") && p != t.NewPartsPolicy {
		c.NewSeasons = &p
	}
	mains, err := h.mainVersions(ctx, []int64{t.ID})
	if err != nil {
		return nil, err
	}
	if p := int64(in.QualityProfileID); p > 0 && p != mains[t.ID].PickRulesID {
		c.PickRulesID = &p
	}
	if c != (library.TitleChange{}) {
		if _, err := h.Library.Edit(ctx, t.ID, c, false, by); err != nil {
			if f, ok := addFailure(err, int64(in.QualityProfileID), in.RootFolderPath); ok {
				return &f, nil
			}
			return nil, err
		}
	}
	parts, err := database.Select[mediafiles.Part](ctx, h.Store.Read(), "parts", "title_id = ? AND kind = 'season'", t.ID)
	if err != nil {
		return nil, err
	}
	for _, se := range in.Seasons {
		for _, p := range parts {
			if p.Number != nil && *p.Number == se.SeasonNumber && p.KeepUpToDate != se.Monitored {
				if err := shows.New(h.Library).SetSeason(ctx, t.ID, se.SeasonNumber, se.Monitored, by); err != nil {
					return nil, err
				}
			}
		}
	}
	return nil, nil
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

package arrapi

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// The bulk routes: adding many titles at once, the editor that changes or
// removes many, and the season pass. Kometa and the old apps' own library
// screens call them. Each takes the same fields and gives the same
// refusals as the single add, change and delete.

// bulkMax caps the titles one bulk call may name, so one call cannot hold
// the Library for long.
const bulkMax = 500

// editorIn is the editor body. Shows come in seriesIds, movies in
// movieIds. The folder, move, show type and season folder fields are
// read but left alone, as the single change leaves them.
type editorIn struct {
	SeriesIDs              []int64   `json:"seriesIds"`
	MovieIDs               []int64   `json:"movieIds"`
	Monitored              *bool     `json:"monitored"`
	MonitorNewItems        string    `json:"monitorNewItems"`
	QualityProfileID       looseID   `json:"qualityProfileId"`
	MinimumAvailability    string    `json:"minimumAvailability"`
	RootFolderPath         string    `json:"rootFolderPath"`
	Tags                   []looseID `json:"tags"`
	ApplyTags              string    `json:"applyTags"`
	DeleteFiles            bool      `json:"deleteFiles"`
	AddImportExclusion     bool      `json:"addImportExclusion"`
	AddImportListExclusion bool      `json:"addImportListExclusion"`
}

// checkIDs refuses an empty or too long id list. It returns false after
// sending a 400.
func checkIDs(w http.ResponseWriter, name string, n int) bool {
	if n == 0 || n > bulkMax {
		refuse(w, failure{PropertyName: name, ErrorMessage: "send between 1 and 500 ids", AttemptedValue: n})
		return false
	}
	return true
}

// editorTitles reads the editor body and every title it names. Nothing
// is changed when one id is unknown: that answers 404.
func (h *handler) editorTitles(w http.ResponseWriter, r *http.Request) (editorIn, []mediafiles.Title, bool) {
	var in editorIn
	if !decode(w, r, &in) {
		return in, nil, false
	}
	ids, name := in.MovieIDs, "MovieIds"
	if h.side == "sonarr" {
		ids, name = in.SeriesIDs, "SeriesIds"
	}
	if !checkIDs(w, name, len(ids)) {
		return in, nil, false
	}
	ts, ok := h.titlesOf(w, r, ids)
	return in, ts, ok
}

// titlesOf reads each title once, in the order given. An unknown id
// answers 404.
func (h *handler) titlesOf(w http.ResponseWriter, r *http.Request, ids []int64) ([]mediafiles.Title, bool) {
	var out []mediafiles.Title
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		t, err := h.title(r.Context(), id, h.folderKind())
		if err != nil {
			internalError(w, err)
			return nil, false
		}
		out = append(out, t)
	}
	return out, true
}

func titleIDs(ts []mediafiles.Title) []int64 {
	out := make([]int64, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

// putEditor changes many titles. The tags change for all of them in one
// transaction. The answer lists the titles as they now are.
func (h *handler) putEditor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, ts, ok := h.editorTitles(w, r)
	if !ok {
		return
	}
	mode := strings.ToLower(in.ApplyTags)
	if mode == "" {
		mode = "add"
	}
	if !slices.Contains([]string{"add", "remove", "replace"}, mode) {
		refuse(w, failure{PropertyName: "ApplyTags", ErrorMessage: "applyTags is add, remove or replace", AttemptedValue: in.ApplyTags})
		return
	}
	by := actor(r)
	for _, t := range ts {
		var f *failure
		var err error
		if h.side == "sonarr" {
			f, err = h.putSeriesOne(ctx, t, seriesIn{Monitored: in.Monitored, MonitorNewItems: in.MonitorNewItems, QualityProfileID: in.QualityProfileID}, by)
		} else {
			f, err = h.putMovieOne(ctx, t, movieIn{Monitored: in.Monitored, MinimumAvailability: in.MinimumAvailability, QualityProfileID: in.QualityProfileID}, by)
		}
		if f != nil {
			refuse(w, *f)
			return
		}
		if err != nil {
			internalError(w, err)
			return
		}
	}
	ids := titleIDs(ts)
	if in.Tags != nil {
		if err := applyTags(ctx, h.Store, ids, tagIDs(in.Tags), mode); err != nil {
			internalError(w, err)
			return
		}
	}
	h.writeTitles(w, r, ids, http.StatusAccepted)
}

// writeTitles answers with these titles in this side's shape.
func (h *handler) writeTitles(w http.ResponseWriter, r *http.Request, ids []int64, code int) {
	if len(ids) == 0 {
		writeJSON(w, code, []any{})
		return
	}
	var out any
	var err error
	if h.side == "sonarr" {
		out, err = h.loadSeries(r.Context(), "id IN "+idList(ids))
	} else {
		out, err = h.loadMovies(r.Context(), false, "id IN "+idList(ids))
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, code, out)
}

// deleteEditor removes many titles, each the way the single delete does:
// files only when the setting allows it, the Never-add list when asked.
func (h *handler) deleteEditor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in, ts, ok := h.editorTitles(w, r)
	if !ok {
		return
	}
	c := library.RemoveChoice{NeverAdd: in.AddImportExclusion || in.AddImportListExclusion}
	c.DeleteFiles = in.DeleteFiles && h.DeleteFiles != nil && h.DeleteFiles(ctx)
	by := actor(r)
	for _, t := range ts {
		_, err := h.Library.Remove(ctx, t.ID, c, true, by)
		if err != nil && !errors.Is(err, library.ErrAlreadyRemoved) {
			log.Printf("arrapi: remove title %d: %v", t.ID, err)
			message(w, http.StatusConflict, removeReason(err))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// importSeries adds many shows. Ones the single add would refuse (already
// there, on the Never-add list, unknown) are skipped, as the old apps
// skip them, and the answer lists the shows added.
func (h *handler) importSeries(w http.ResponseWriter, r *http.Request) {
	var in []seriesIn
	if !decode(w, r, &in) || !checkIDs(w, "Series", len(in)) {
		return
	}
	var ids []int64
	for _, s := range in {
		id, f, err := h.addOneSeries(r.Context(), s, actor(r))
		if err != nil {
			internalError(w, err)
			return
		}
		if f == nil {
			ids = append(ids, id)
		}
	}
	h.writeTitles(w, r, ids, http.StatusOK)
}

// importMovies adds many movies, skipping the ones the single add would
// refuse.
func (h *handler) importMovies(w http.ResponseWriter, r *http.Request) {
	var in []movieIn
	if !decode(w, r, &in) || !checkIDs(w, "Movies", len(in)) {
		return
	}
	var ids []int64
	for _, m := range in {
		id, f, err := h.addOneMovie(r.Context(), m, actor(r))
		if err != nil {
			internalError(w, err)
			return
		}
		if f == nil {
			ids = append(ids, id)
		}
	}
	h.writeTitles(w, r, ids, http.StatusOK)
}

// seasonPassIn is the season pass body: per show its switch and season
// switches, and one season choice for them all.
type seasonPassIn struct {
	Series            []seriesIn `json:"series"`
	MonitoringOptions struct {
		Monitor string `json:"monitor"`
	} `json:"monitoringOptions"`
}

// seasonPass switches shows and their seasons. The choice "all" keeps
// every season but the specials, "none" keeps none; other choices leave
// the seasons as the per-season switches set them.
func (h *handler) seasonPass(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in seasonPassIn
	if !decode(w, r, &in) || !checkIDs(w, "Series", len(in.Series)) {
		return
	}
	ids := make([]int64, 0, len(in.Series))
	for _, s := range in.Series {
		ids = append(ids, s.ID)
	}
	ts, ok := h.titlesOf(w, r, ids)
	if !ok {
		return
	}
	byID := map[int64]mediafiles.Title{}
	for _, t := range ts {
		byID[t.ID] = t
	}
	by := actor(r)
	choice := strings.ToLower(in.MonitoringOptions.Monitor)
	for _, s := range in.Series {
		t := byID[s.ID]
		if f, err := h.putSeriesOne(ctx, t, seriesIn{Monitored: s.Monitored, Seasons: s.Seasons}, by); f != nil {
			refuse(w, *f)
			return
		} else if err != nil {
			internalError(w, err)
			return
		}
		if choice != "all" && choice != "none" {
			continue
		}
		parts, err := database.Select[mediafiles.Part](ctx, h.Store.Read(), "parts", "title_id = ? AND kind = 'season'", t.ID)
		if err != nil {
			internalError(w, err)
			return
		}
		for _, p := range parts {
			if p.Number == nil {
				continue
			}
			keep := choice == "all" && *p.Number > 0
			if p.KeepUpToDate != keep {
				if err := shows.New(h.Library).SetSeason(ctx, t.ID, *p.Number, keep, by); err != nil {
					internalError(w, err)
					return
				}
			}
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{})
}

// getTag answers one tag. An unknown id is 404.
func (h *handler) getTag(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := tagByID(r.Context(), h.Store.Read(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			notFound(w)
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

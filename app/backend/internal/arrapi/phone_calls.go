package arrapi

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// The deeper read calls phone apps such as Helmarr and nzb360 make: one
// episode and its monitor switch, the history of one title, movie files
// and the upgrade list.

// getEpisode answers one episode of a Library show.
func (h *handler) getEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	eps, err := h.episodesWhere(r.Context(), "id = ?", id)
	if err == nil && len(eps) == 1 {
		err = h.withFiles(r.Context(), eps)
	}
	if err == nil && len(eps) == 1 {
		err = h.withSeries(r.Context(), eps)
	}
	switch {
	case err != nil:
		internalError(w, err)
	case len(eps) != 1:
		notFound(w)
	default:
		writeJSON(w, http.StatusOK, eps[0])
	}
}

// putEpisode applies only the monitored flag, as the old apps do; the
// rest of the body is ignored.
func (h *handler) putEpisode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Monitored *bool `json:"monitored"`
	}
	if !decode(w, r, &in) {
		return
	}
	eps, err := h.episodesWhere(ctx, "id = ?", id)
	if err != nil {
		internalError(w, err)
		return
	}
	if len(eps) != 1 {
		notFound(w)
		return
	}
	if in.Monitored != nil && *in.Monitored != eps[0].Monitored {
		if _, err := shows.New(h.Library).SetEpisode(ctx, id, *in.Monitored, actor(r)); err != nil {
			internalError(w, err)
			return
		}
	}
	if eps, err = h.episodesWhere(ctx, "id = ?", id); err == nil {
		err = h.withFiles(ctx, eps)
	}
	if err != nil || len(eps) != 1 {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, eps[0])
}

// titleHistory answers /history/series and /history/movie: every row of
// one title, newest first, not paged. Sonarr also takes seasonNumber and
// episodeId; both take eventType by name or number.
func (h *handler) titleHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	kind, param := "show", "seriesId"
	if h.side == "radarr" {
		kind, param = "movie", "movieId"
	}
	if query(r, param) == "" {
		message(w, http.StatusBadRequest, "send "+param)
		return
	}
	t, err := h.title(ctx, atoi64(query(r, param)), kind)
	if err != nil {
		internalError(w, err)
		return
	}
	rows, err := h.historyRows(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	var season map[int64]bool
	if sn := query(r, "seasonNumber"); sn != "" && kind == "show" {
		n, err := strconv.Atoi(sn)
		if err != nil {
			message(w, http.StatusBadRequest, "seasonNumber must be a number")
			return
		}
		eps, err := h.episodesWhere(ctx, "title_id = ? AND json_extract(numbers, '$.season') = ?", t.ID, n)
		if err != nil {
			internalError(w, err)
			return
		}
		season = map[int64]bool{}
		for _, e := range eps {
			season[e.ID] = true
		}
	}
	ev, ep := query(r, "eventType"), atoi64(query(r, "episodeId"))
	rows = slices.DeleteFunc(rows, func(x historyRow) bool {
		switch {
		case x.SeriesID != t.ID && x.MovieID != t.ID:
			return true
		case season != nil && !season[x.EpisodeID], ep > 0 && x.EpisodeID != ep:
			return true
		case ev != "":
			return !strings.EqualFold(x.EventType, ev) && ev != historyNumber(h.side, x.EventType)
		}
		return false
	})
	if err := h.historyInclude(ctx, r, rows); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

// movieFiles answers /moviefile: the files of the movies in movieId
// (repeatable) or of movieFileIds. With neither it answers 400.
func (h *handler) movieFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ids := idsParam(r, "movieId")
	files := idsParam(r, "movieFileIds")
	if len(ids) == 0 && len(files) == 0 {
		message(w, http.StatusBadRequest, "send movieId or movieFileIds")
		return
	}
	if len(files) > 0 {
		more, err := mediafiles.MoviesOfFiles(ctx, h.Store.Read(), files)
		if err != nil {
			internalError(w, err)
			return
		}
		ids = append(ids, more...)
	}
	if len(idsParam(r, "movieId")) > 0 {
		files = nil // a movie asked for brings all its files
	}
	out, err := h.movieFilesOf(r, ids, files)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// movieFile answers one movie file.
func (h *handler) movieFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ids, err := mediafiles.MoviesOfFiles(r.Context(), h.Store.Read(), []int64{id})
	if err != nil {
		internalError(w, err)
		return
	}
	out, err := h.movieFilesOf(r, ids, []int64{id})
	switch {
	case err != nil:
		internalError(w, err)
	case len(out) == 0:
		notFound(w)
	default:
		writeJSON(w, http.StatusOK, out[0])
	}
}

// movieFilesOf reads the main-Version files of the movies. Non-empty
// fileIDs keeps only those files.
func (h *handler) movieFilesOf(r *http.Request, movieIDs, fileIDs []int64) ([]mediaFile, error) {
	out := []mediaFile{}
	if len(movieIDs) == 0 {
		return out, nil
	}
	ms, err := h.loadMovies(r.Context(), false, "id IN "+idList(movieIDs))
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		if m.MovieFile == nil || len(fileIDs) > 0 && !slices.Contains(fileIDs, m.MovieFile.ID) {
			continue
		}
		out = append(out, *m.MovieFile)
	}
	return out, nil
}

// cutoffUnmet pages the upgrade list: the episodes or movies whose file
// HolyArrs still wants a better copy of. monitored works as on the
// missing list.
func (h *handler) cutoffUnmet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pg := pagingOf(r)
	kind, key := "show", "episodes.airDateUtc"
	if h.side == "radarr" {
		kind, key = "movie", "movieMetadata.sortTitle"
	}
	ids, err := mediafiles.BetterWanted(ctx, h.Store.Read(), kind, pg.kept)
	if err != nil {
		internalError(w, err)
		return
	}
	from := min((pg.page-1)*pg.size, len(ids))
	to := min(from+pg.size, len(ids))
	page := ids[from:to]
	var records any = []episode{}
	if len(page) > 0 {
		if kind == "movie" {
			ms, err := h.loadMovies(ctx, false, "id IN "+idList(page))
			if err != nil {
				internalError(w, err)
				return
			}
			records = moviesInOrder(ms, page)
		} else {
			eps, err := h.episodesWhere(ctx, "id IN "+idList(page))
			if err == nil {
				eps = inOrder(eps, page)
				err = h.include(ctx, r, eps)
			}
			if err != nil {
				internalError(w, err)
				return
			}
			records = eps
		}
	} else if kind == "movie" {
		records = []movie{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": pg.page, "pageSize": pg.size, "sortKey": key, "sortDirection": pg.word,
		"totalRecords": len(ids), "records": records})
}

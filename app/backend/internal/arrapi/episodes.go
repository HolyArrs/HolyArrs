package arrapi

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// episode is one episode as the Sonarr side answers it.
type episode struct {
	ID                    int64  `json:"id"`
	SeriesID              int64  `json:"seriesId"`
	TvdbID                int64  `json:"tvdbId"`
	EpisodeFileID         int64  `json:"episodeFileId"`
	SeasonNumber          int    `json:"seasonNumber"`
	EpisodeNumber         int    `json:"episodeNumber"`
	Title                 string `json:"title,omitempty"`
	AirDate               string `json:"airDate,omitempty"`
	AirDateUtc            *utc   `json:"airDateUtc,omitempty"`
	Runtime               int    `json:"runtime"`
	HasFile               bool   `json:"hasFile"`
	Monitored             bool   `json:"monitored"`
	AbsoluteEpisodeNumber *int   `json:"absoluteEpisodeNumber,omitempty"`
	UnverifiedScene       bool   `json:"unverifiedSceneNumbering"`
	// Set only when asked for with includeEpisodeFile or includeSeries.
	EpisodeFile *mediaFile `json:"episodeFile,omitempty"`
	Series      *series    `json:"series,omitempty"`
}

// episodesWhere reads the episodes of Library shows that match where.
// Each carries the file of its main Version.
func (h *handler) episodesWhere(ctx context.Context, where string, args ...any) ([]episode, error) {
	return h.episodesOf(ctx, library.EpisodeFilter{Where: where, Args: args})
}

// episodesOf runs episodesWhere with a typed filter.
func (h *handler) episodesOf(ctx context.Context, f library.EpisodeFilter) ([]episode, error) {
	its, err := library.EpisodeRows(ctx, h.Store.Read(), f)
	if err != nil || len(its) == 0 {
		return []episode{}, err
	}
	ids := make([]int64, len(its))
	for i, it := range its {
		ids[i] = it.ID
	}
	files, err := library.MainFileOfItems(ctx, h.Store.Read(), ids)
	if err != nil {
		return nil, err
	}
	out := make([]episode, 0, len(its))
	for _, it := range its {
		e := episode{ID: it.ID, SeriesID: it.TitleID, TvdbID: sourceID(it.SourceIDs, "tvdb"), EpisodeFileID: files[it.ID],
			AirDateUtc: utcPtr(it.AirDate), Monitored: it.KeepUpToDate, AbsoluteEpisodeNumber: it.Numbers.Absolute}
		e.HasFile = e.EpisodeFileID > 0
		if it.Numbers.Season != nil {
			e.SeasonNumber = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			e.EpisodeNumber = *it.Numbers.Episode
		}
		if it.Name != nil {
			e.Title = *it.Name
		}
		if it.AirDay != nil {
			e.AirDate = *it.AirDay
		}
		if it.Runtime != nil {
			e.Runtime = *it.Runtime
		}
		out = append(out, e)
	}
	return out, nil
}

// idsParam reads ids sent as one comma list or as a repeated parameter,
// since clients do both.
func idsParam(r *http.Request, name string) []int64 {
	var out []int64
	for k, vs := range r.URL.Query() {
		if !strings.EqualFold(k, name) {
			continue
		}
		for _, v := range vs {
			for _, x := range strings.Split(v, ",") {
				if n := atoi64(x); n > 0 {
					out = append(out, n)
				}
			}
		}
	}
	return out
}

// listEpisodes needs seriesId, with an optional seasonNumber, or
// episodeIds. With neither it answers 400.
func (h *handler) listEpisodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sid := query(r, "seriesId"); sid != "" {
		t, err := h.title(ctx, atoi64(sid), "show")
		if err != nil {
			internalError(w, err)
			return
		}
		where, args := "title_id = ?", []any{t.ID}
		if sn := query(r, "seasonNumber"); sn != "" {
			n, err := strconv.Atoi(sn)
			if err != nil {
				message(w, http.StatusBadRequest, "seasonNumber must be a number")
				return
			}
			where += " AND json_extract(numbers, '$.season') = ?"
			args = append(args, n)
		}
		out, err := h.episodesWhere(ctx, where, args...)
		if err == nil {
			err = h.include(ctx, r, out)
		}
		if err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if ids := idsParam(r, "episodeIds"); len(ids) > 0 {
		out, err := h.episodesWhere(ctx, "id IN "+idList(ids))
		if err == nil {
			err = h.include(ctx, r, out)
		}
		if err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	message(w, http.StatusBadRequest, "send seriesId or episodeIds")
}

// monitorEpisodes switches episodes on or off. Every id must be an
// episode of a Library show. If one is not, nothing changes and it
// answers 404.
func (h *handler) monitorEpisodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		EpisodeIDs []int64 `json:"episodeIds"`
		Monitored  bool    `json:"monitored"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.EpisodeIDs == nil {
		message(w, http.StatusBadRequest, "send episodeIds and monitored")
		return
	}
	if len(in.EpisodeIDs) == 0 {
		writeJSON(w, http.StatusAccepted, []episode{})
		return
	}
	slices.Sort(in.EpisodeIDs)
	in.EpisodeIDs = slices.Compact(in.EpisodeIDs)
	found, err := h.episodesWhere(ctx, "id IN "+idList(in.EpisodeIDs))
	if err != nil {
		internalError(w, err)
		return
	}
	if len(found) != len(in.EpisodeIDs) {
		notFound(w)
		return
	}
	by := actor(r)
	for _, e := range found {
		if e.Monitored == in.Monitored {
			continue
		}
		if _, err := shows.New(h.Library).SetEpisode(ctx, e.ID, in.Monitored, by); err != nil {
			internalError(w, err)
			return
		}
	}
	out, err := h.episodesWhere(ctx, "id IN "+idList(in.EpisodeIDs))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

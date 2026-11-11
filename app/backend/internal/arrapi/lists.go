package arrapi

import (
	"context"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The read-only lists that start pages such as Homepage and subtitle apps
// such as Bazarr poll on the Sonarr side: episodes with their file or
// show, the missing list, the calendar and the unpaged queue. None of them
// writes, so polling stays cheap.

// include adds what the caller asked for to each episode: its file with
// includeEpisodeFile, which Bazarr's sync uses, and its show with
// includeSeries. Each takes a fixed number of queries.
func (h *handler) include(ctx context.Context, r *http.Request, eps []episode) error {
	if queryBool(r, "includeEpisodeFile") {
		if err := h.withFiles(ctx, eps); err != nil {
			return err
		}
	}
	if queryBool(r, "includeSeries") {
		return h.withSeries(ctx, eps)
	}
	return nil
}

// withFiles puts each episode's main-Version file on it. An episode with
// none gets no file.
func (h *handler) withFiles(ctx context.Context, eps []episode) error {
	var ids []int64
	for _, e := range eps {
		if e.EpisodeFileID > 0 {
			ids = append(ids, e.EpisodeFileID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	db := h.Store.Read()
	fs, err := database.Select[mediafiles.File](ctx, db, "files", "id IN "+idList(ids))
	if err != nil || len(fs) == 0 {
		return err
	}
	vids := make([]int64, 0, len(fs))
	for _, f := range fs {
		vids = append(vids, f.VersionID)
	}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "id IN "+idList(vids))
	if err != nil {
		return err
	}
	versions := map[int64]mediafiles.Version{}
	for _, v := range vs {
		versions[v.ID] = v
	}
	folders, err := h.folders(ctx)
	if err != nil {
		return err
	}
	files := map[int64]*mediaFile{}
	for _, f := range fs {
		v := versions[f.VersionID]
		files[f.ID] = mediaFileOf(f, folders[v.LibraryFolderID].Path, v.FolderName)
	}
	for i := range eps {
		if mf := files[eps[i].EpisodeFileID]; mf != nil {
			c := *mf
			c.SeriesID, c.SeasonNumber = eps[i].SeriesID, &eps[i].SeasonNumber
			eps[i].EpisodeFile = &c
		}
	}
	return nil
}

// withSeries puts each episode's show on it.
func (h *handler) withSeries(ctx context.Context, eps []episode) error {
	ids := make([]int64, 0, len(eps))
	for _, e := range eps {
		ids = append(ids, e.SeriesID)
	}
	shows, err := h.seriesByID(ctx, ids)
	if err != nil {
		return err
	}
	for i := range eps {
		if s, ok := shows[eps[i].SeriesID]; ok {
			eps[i].Series = &s
		}
	}
	return nil
}

// seriesByID reads the shows in ids, keyed by id.
func (h *handler) seriesByID(ctx context.Context, ids []int64) (map[int64]series, error) {
	out := map[int64]series{}
	if len(ids) == 0 {
		return out, nil
	}
	ids = slices.Clone(ids)
	slices.Sort(ids)
	list, err := h.loadSeries(ctx, "id IN "+idList(slices.Compact(ids)))
	for _, s := range list {
		out[s.ID] = s
	}
	return out, err
}

// inOrder puts eps back in the order of ids, which is the sorted order.
func inOrder(eps []episode, ids []int64) []episode {
	at := map[int64]int{}
	for i, id := range ids {
		if _, ok := at[id]; !ok {
			at[id] = i
		}
	}
	slices.SortStableFunc(eps, func(a, b episode) int { return at[a.ID] - at[b.ID] })
	return eps
}

// missingSorts maps each lower-case sort key of the missing list to the
// name answered and the column sorted on.
var missingSorts = map[string][2]string{
	"episodes.airdateutc":     {"episodes.airDateUtc", "i.air_date"},
	"episodes.lastsearchtime": {"episodes.lastSearchTime", "i.last_searched_at"},
	"series.sorttitle":        {"series.sortTitle", "t.sort_name"},
}

// missingDefaultSize is the missing list's page size when none is sent.
const missingDefaultSize = 10

// missingEpisodes pages the aired episodes with no file in the main
// Version. monitored defaults to true and keeps the episodes kept up to
// date with their season and show. false gives the others. Sorting
// follows the old apps' paging rules. No direction means descending, and
// only "default" uses the endpoint's own ascending order. An unknown key
// falls back to the air date. A page past the end is empty and keeps the
// asked number.
func (h *handler) missingEpisodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sort, ok := missingSorts[strings.ToLower(query(r, "sortKey"))]
	if !ok {
		sort = missingSorts["episodes.airdateutc"]
	}
	dir := strings.ToLower(query(r, "sortDirection"))
	asc := dir == "ascending" || dir == "default"
	word := "descending"
	if asc {
		word = "ascending"
	}
	size := missingDefaultSize
	if n, err := strconv.Atoi(query(r, "pageSize")); err == nil && n > 0 {
		size = min(n, 10000)
	}
	page := 1
	if n, err := strconv.ParseInt(query(r, "page"), 10, 64); err == nil && n > 0 {
		page = int(min(n, math.MaxInt)) // past the end on a 32-bit build too
	}
	kept := true
	if v := query(r, "monitored"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			kept = b
		}
	}
	found, err := library.MissingEpisodes(ctx, h.Store.Read(), kept, sort[1], !asc, h.now(), page, size)
	if err != nil {
		internalError(w, err)
		return
	}
	total, ids := found.Total, found.IDs
	records := []episode{}
	if len(ids) > 0 {
		if records, err = h.episodesWhere(ctx, "id IN "+idList(ids)); err == nil {
			records = inOrder(records, ids)
			err = h.include(ctx, r, records)
		}
		if err != nil {
			internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": page, "pageSize": size, "sortKey": sort[0], "sortDirection": word,
		"totalRecords": total, "records": records})
}

// queryTime reads a start or end value: a date, or a date and time with
// or without a zone. No zone means UTC.
func queryTime(r *http.Request, name string) (time.Time, bool) {
	v := strings.TrimSpace(query(r, name))
	if v == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// calendarEpisodes lists the episodes airing from start to end by air
// time. The default window is today to two days on. Without
// unmonitored=true only kept episodes of kept shows are listed.
// includeSeries and includeEpisodeFile add the show and the file.
func (h *handler) calendarEpisodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := h.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from, ok := queryTime(r, "start")
	if !ok {
		from = today
	}
	to, ok := queryTime(r, "end")
	if !ok {
		to = today.AddDate(0, 0, 2)
	}
	out := []episode{}
	if !to.After(from) {
		writeJSON(w, http.StatusOK, out)
		return
	}
	es, err := library.Calendar(ctx, h.Store.Read(), from, to, library.CalendarShows)
	if err != nil {
		internalError(w, err)
		return
	}
	var ids []int64
	for _, e := range es {
		if e.ItemID > 0 {
			ids = append(ids, e.ItemID)
		}
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	f := library.EpisodeFilter{Where: "id IN " + idList(ids), KeptShows: !queryBool(r, "unmonitored")}
	if out, err = h.episodesOf(ctx, f); err == nil {
		out = inOrder(out, ids)
		err = h.include(ctx, r, out)
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// queueDetails answers the download list unpaged. It gives every row, or
// those of one show with seriesId, or of some episodes with episodeIds.
// includeEpisode and includeSeries add the episode and the show.
func (h *handler) queueDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.queueRows(ctx, queryBool(r, "includeEpisode"))
	if err != nil {
		internalError(w, err)
		return
	}
	if v := query(r, "seriesId"); v != "" {
		id := atoi64(v)
		rows = slices.DeleteFunc(rows, func(q queueRow) bool { return q.SeriesID != id })
	}
	if ids := idsParam(r, "episodeIds"); len(ids) > 0 {
		rows = slices.DeleteFunc(rows, func(q queueRow) bool { return !slices.Contains(ids, q.EpisodeID) })
	}
	if queryBool(r, "includeSeries") && len(rows) > 0 {
		ids := make([]int64, 0, len(rows))
		for _, q := range rows {
			ids = append(ids, q.SeriesID)
		}
		shows, err := h.seriesByID(ctx, ids)
		if err != nil {
			internalError(w, err)
			return
		}
		for i := range rows {
			if s, ok := shows[rows[i].SeriesID]; ok {
				rows[i].Series = &s
			}
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

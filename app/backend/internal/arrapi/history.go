package arrapi

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// The calls below are the ones phone apps such as Helmarr make for their
// History, disk and episode-file screens.

// historyKinds maps the Events a History row is made from to the old apps'
// event types. Other Events have no row there.
var historyKinds = map[string]string{
	"download_sent":      "grabbed",
	"import_done":        "downloadFolderImported",
	"import_partly_done": "downloadFolderImported",
	"download_failed":    "downloadFailed",
	"import_failed":      "downloadFailed",
	"renamed":            "fileRenamed", // side-specific name set in historyType
}

func historyType(side, kind string) string {
	t := historyKinds[kind]
	if t == "fileRenamed" {
		if side == "sonarr" {
			return "episodeFileRenamed"
		}
		return "movieFileRenamed"
	}
	return t
}

// historyRow is one History row as the old apps answer it.
type historyRow struct {
	ID                  int64          `json:"id"`
	EpisodeID           int64          `json:"episodeId,omitempty"`
	SeriesID            int64          `json:"seriesId,omitempty"`
	MovieID             int64          `json:"movieId,omitempty"`
	SourceTitle         string         `json:"sourceTitle"`
	Languages           []language     `json:"languages"`
	Quality             map[string]any `json:"quality"`
	CustomFormats       []any          `json:"customFormats"`
	CustomFormatScore   int            `json:"customFormatScore"`
	QualityCutoffNotMet bool           `json:"qualityCutoffNotMet"`
	Date                utc            `json:"date"`
	DownloadID          string         `json:"downloadId,omitempty"`
	EventType           string         `json:"eventType"`
	Data                map[string]any `json:"data"`
	Episode             *episode       `json:"episode,omitempty"`
	Series              *series        `json:"series,omitempty"`
	Movie               *movie         `json:"movie,omitempty"`
}

// historyMax caps how many Events one History answer reads. The old apps
// page through history, and phone apps ask for at most a few hundred rows.
const historyMax = 5000

// history pages this side's download and import Events, newest first.
func (h *handler) history(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := pagingOf(r)
	if query(r, "pageSize") == "" {
		p.size = 10
	}
	if query(r, "sortDirection") == "" {
		p.asc, p.word = false, "descending"
	}
	rows, err := h.historyRows(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	if p.asc {
		slices.Reverse(rows)
	}
	if v := query(r, "eventType"); v != "" {
		rows = slices.DeleteFunc(rows, func(x historyRow) bool {
			return !strings.EqualFold(x.EventType, v) && v != historyNumber(h.side, x.EventType)
		})
	}
	if ids := idsParam(r, "episodeId"); len(ids) > 0 {
		rows = slices.DeleteFunc(rows, func(x historyRow) bool { return !slices.Contains(ids, x.EpisodeID) })
	}
	if ids := idsParam(r, "movieIds"); len(ids) > 0 {
		rows = slices.DeleteFunc(rows, func(x historyRow) bool { return !slices.Contains(ids, x.MovieID) })
	}
	from := min((p.page-1)*p.size, len(rows))
	to := min(from+p.size, len(rows))
	page := rows[from:to]
	if err := h.historyInclude(ctx, r, page); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": p.page, "pageSize": p.size, "sortKey": "date", "sortDirection": p.word,
		"totalRecords": len(rows), "records": page})
}

// historyNumber is the number the old apps also accept for an event type.
func historyNumber(side, t string) string {
	n := map[string]int{"grabbed": 1, "downloadFolderImported": 3, "downloadFailed": 4}[t]
	if t == "episodeFileRenamed" || t == "movieFileRenamed" {
		n = 6
	}
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// historyRows reads this side's rows, newest first.
func (h *handler) historyRows(ctx context.Context) ([]historyRow, error) {
	kind := map[string]string{"sonarr": "show", "radarr": "movie"}[h.side]
	db := h.Store.Read()
	kinds, err := mediafiles.TitleKinds(ctx, db)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(historyKinds))
	for k := range historyKinds {
		keys = append(keys, k)
	}
	evs, err := h.Store.TitleEventsOfKinds(ctx, keys, historyMax)
	if err != nil {
		return nil, err
	}
	// Episodes come from the Event's Slot.
	var slots []int64
	for _, e := range evs {
		if e.SlotID != nil {
			slots = append(slots, *e.SlotID)
		}
	}
	items, err := mediafiles.SlotItems(ctx, db, slots)
	if err != nil {
		return nil, err
	}
	var dls []int64
	for _, e := range evs {
		if e.DownloadID != nil {
			dls = append(dls, *e.DownloadID)
		}
	}
	// What was read before a failure still shows; the rest goes without.
	downloadIDs, _ := downloads.ExternalIDs(ctx, db, dls)
	// The release each send came from, for the details Sonarr keeps.
	var relIDs []int64
	for _, e := range evs {
		if id := int64Of(e.Data["release_id"]); id > 0 {
			relIDs = append(relIDs, id)
		}
	}
	facts, _ := sources.ReleaseFactsOf(ctx, db, relIDs)
	rels := map[int64]historyRelease{}
	for id, f := range facts {
		rels[id] = historyRelease{size: strconv.FormatInt(f.Size, 10), protocol: arrProtocol(f.Protocol), published: f.PublishedAt, hash: f.InfoHash,
			indexer: f.SourceName, info: f.InfoURL}
	}
	out := []historyRow{}
	for _, e := range evs {
		if kinds[*e.TitleID] != kind {
			continue
		}
		row := historyRow{ID: e.ID, Date: utc(e.At), EventType: historyType(h.side, e.Kind), Languages: []language{}, CustomFormats: []any{},
			Data: map[string]any{}}
		if kind == "show" {
			row.SeriesID = *e.TitleID
			if e.SlotID != nil {
				row.EpisodeID = items[*e.SlotID]
			}
		} else {
			row.MovieID = *e.TitleID
		}
		name, _ := e.Data["release"].(string)
		if name == "" {
			name = e.Summary
		}
		row.SourceTitle = name
		row.Quality = qualityOfRelease(h.side, name)
		if e.DownloadID != nil {
			row.DownloadID = strings.ToUpper(downloadIDs[*e.DownloadID])
		}
		historyData(row.Data, e, name, rels[int64Of(e.Data["release_id"])])
		out = append(out, row)
	}
	return out, nil
}

// historyRelease is what History tells of the release a send came from.
type historyRelease struct{ size, protocol, published, hash, indexer, info string }

// historyData fills a row's data the way the old apps do: every value is
// text, and only what HolyArrs knows is there.
func historyData(d map[string]any, e database.Event, release string, r historyRelease) {
	set := func(k, v string) {
		if v != "" {
			d[k] = v
		}
	}
	str := func(k string) string { s, _ := e.Data[k].(string); return s }
	set("downloadClient", str("downloader"))
	set("downloadClientName", str("downloader"))
	set("reason", str("reason"))
	set("releaseGroup", releasename.Parse(release, releasename.Hints{}).Group.Value)
	set("size", r.size)
	set("protocol", r.protocol)
	set("publishedDate", r.published)
	set("torrentInfoHash", r.hash)
	set("indexer", r.indexer)
	set("nzbInfoUrl", r.info)
	switch e.Kind {
	case "import_done", "import_partly_done":
		set("droppedPath", str("from"))
		if ps, ok := e.Data["paths"].([]any); ok && len(ps) > 0 {
			p, _ := ps[0].(string)
			set("importedPath", p)
		}
	case "download_failed", "import_failed":
		set("message", e.Summary)
	}
}

// qualityOfRelease names a release's quality the way mediaFileOf names a
// file's, from what the release name says.
func qualityOfRelease(side, name string) map[string]any {
	p := releasename.Parse(name, releasename.Hints{})
	return fullQuality(side, qualityName(p.Source.Value, p.Resolution.Value), p.Resolution.Value)
}

// historyInclude fills the episode, series or movie of each row when the
// caller asks for them.
func (h *handler) historyInclude(ctx context.Context, r *http.Request, rows []historyRow) error {
	if h.side == "radarr" {
		if !queryBool(r, "includeMovie") {
			return nil
		}
		cache := map[int64]*movie{}
		for i := range rows {
			id := rows[i].MovieID
			if _, ok := cache[id]; !ok {
				m, found, err := h.oneMovie(ctx, id, false)
				if err != nil {
					return err
				}
				if found {
					cache[id] = &m
				} else {
					cache[id] = nil
				}
			}
			rows[i].Movie = cache[id]
		}
		return nil
	}
	if queryBool(r, "includeSeries") {
		var ids []int64
		for _, x := range rows {
			ids = append(ids, x.SeriesID)
		}
		by, err := h.seriesByID(ctx, ids)
		if err != nil {
			return err
		}
		for i := range rows {
			if s, ok := by[rows[i].SeriesID]; ok {
				rows[i].Series = &s
			}
		}
	}
	if queryBool(r, "includeEpisode") {
		var ids []int64
		for _, x := range rows {
			if x.EpisodeID > 0 {
				ids = append(ids, x.EpisodeID)
			}
		}
		if len(ids) == 0 {
			return nil
		}
		eps, err := h.episodesWhere(ctx, "id IN "+idList(ids))
		if err != nil {
			return err
		}
		by := map[int64]episode{}
		for _, e := range eps {
			by[e.ID] = e
		}
		for i := range rows {
			if e, ok := by[rows[i].EpisodeID]; ok {
				rows[i].Episode = &e
			}
		}
	}
	return nil
}

// eachRow runs a query and hands each row to f.
func eachRow(ctx context.Context, db *sql.DB, q string, f func(*sql.Rows) error, args ...any) error {
	rs, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return err
	}
	defer rs.Close()
	for rs.Next() {
		if err := f(rs); err != nil {
			return err
		}
	}
	return rs.Err()
}

// diskSpace lists the disks under this side's Library folders, once each.
func (h *handler) diskSpace(w http.ResponseWriter, r *http.Request) {
	fs, err := database.Select[mediafiles.LibraryFolder](r.Context(), h.Store.Read(), "library_folders", "kind = ? ORDER BY id", h.folderKind())
	if err != nil {
		internalError(w, err)
		return
	}
	out := []map[string]any{}
	seen := map[[2]int64]bool{}
	for _, f := range fs {
		free, total := importer.FreeSpace(f.Path), importer.DiskSize(f.Path)
		if free < 0 || total < 0 {
			continue
		}
		key := [2]int64{free, total}
		if seen[key] { // the same disk under another folder
			continue
		}
		seen[key] = true
		out = append(out, map[string]any{"id": len(out) + 1, "path": f.Path, "label": f.Path, "freeSpace": free, "totalSpace": total})
	}
	writeJSON(w, http.StatusOK, out)
}

// customFilters answers that no saved filters exist; HolyArrs keeps its
// own in the page address.
func (h *handler) customFilters(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}

// episodeFiles lists the main-Version files of a show's episodes. It
// needs seriesId.
func (h *handler) episodeFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var eps []episode
	var err error
	switch {
	case query(r, "seriesId") != "":
		var t mediafiles.Title
		t, err = h.title(ctx, atoi64(query(r, "seriesId")), "show")
		if err == nil {
			eps, err = h.episodesWhere(ctx, "title_id = ?", t.ID)
		}
	default:
		message(w, http.StatusBadRequest, "send seriesId")
		return
	}
	if err == nil {
		err = h.withFiles(ctx, eps)
	}
	if err != nil {
		internalError(w, err)
		return
	}
	out := []mediaFile{}
	seen := map[int64]bool{}
	for _, e := range eps {
		if e.EpisodeFile == nil || seen[e.EpisodeFile.ID] {
			continue
		}
		seen[e.EpisodeFile.ID] = true
		f := *e.EpisodeFile
		f.SeriesID = e.SeriesID
		if f.SeasonNumber == nil {
			f.SeasonNumber = ptr(e.SeasonNumber)
		}
		out = append(out, f)
	}
	writeJSON(w, http.StatusOK, out)
}

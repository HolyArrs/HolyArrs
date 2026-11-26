package arrapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Queue removal, the blocklist and the system lists phone apps read.

// queueDownload is the download behind a queue row id: every row of one
// download is its id times 1000 plus the row's place.
func queueDownload(rowID int64) int64 { return rowID / 1000 }

// downloadOnSide reads a download and reports whether every one of its Slots
// is an episode (Sonarr) or a movie (Radarr), so a key of one side never
// removes the other side's downloads.
func (h *handler) downloadOnSide(ctx context.Context, id int64) (bool, error) {
	var d domain.Download
	if err := database.Get(ctx, h.Store.Read(), "downloads", id, &d); err != nil {
		return false, err
	}
	rows, err := library.SlotItems(ctx, h.Store.Read(), d.SlotIDs)
	if err != nil {
		return false, err
	}
	want := map[string]string{"sonarr": "episode", "radarr": "movie"}[h.side]
	if len(rows) == 0 {
		return false, nil
	}
	for _, x := range rows {
		if x.ItemKind != want {
			return false, nil
		}
	}
	return true, nil
}

// removeChoice reads the old apps' remove options. removeFromClient
// defaults to true, as in Sonarr and Radarr: the download and its files
// leave the Downloader. removeFromClient=false keeps the files;
// blocklist turns the Release down; then it searches again unless
// skipRedownload. changeCategory is not offered, so it is ignored.
func removeChoice(r *http.Request) downloads.RemoveChoice {
	on := func(name string, def bool) bool {
		switch strings.ToLower(query(r, name)) {
		case "true":
			return true
		case "false":
			return false
		}
		return def
	}
	c := downloads.RemoveChoice{DeleteFiles: on("removeFromClient", true), TurnDown: on("blocklist", false)}
	c.SearchAgain = c.TurnDown && !on("skipRedownload", false)
	return c
}

// removeQueue answers DELETE /queue/{id}.
func (h *handler) removeQueue(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.removeDownloads(w, r, []int64{queueDownload(id)})
}

// removeQueueBulk answers DELETE /queue/bulk with "ids" in the body.
func (h *handler) removeQueueBulk(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > bulkMax {
		message(w, http.StatusBadRequest, "send between 1 and 500 ids")
		return
	}
	var dls []int64
	for _, id := range in.IDs {
		if d := queueDownload(id); d > 0 && !slices.Contains(dls, d) {
			dls = append(dls, d)
		}
	}
	h.removeDownloads(w, r, dls)
}

// removeDownloads removes downloads the way the Downloads page's Remove
// does. Every one is checked first: one unknown or of the other side
// answers 404 and removes nothing.
func (h *handler) removeDownloads(w http.ResponseWriter, r *http.Request, ids []int64) {
	ctx := r.Context()
	if len(ids) == 0 {
		notFound(w)
		return
	}
	for _, id := range ids {
		on, err := h.downloadOnSide(ctx, id)
		if errors.Is(err, database.ErrNotFound) || errors.Is(err, sql.ErrNoRows) || err == nil && !on {
			notFound(w)
			return
		}
		if err != nil {
			internalError(w, err)
			return
		}
	}
	if h.Downloads == nil || h.Downloads() == nil {
		message(w, http.StatusServiceUnavailable, "Downloads are not running in HolyArrs right now.")
		return
	}
	c := removeChoice(r)
	who := ""
	if p, ok := webserver.From(ctx); ok {
		who = p.Name
	}
	for _, id := range ids {
		_, err := h.Downloads().RemoveSaid(ctx, id, c, actor(r), who)
		switch {
		case errors.Is(err, downloads.ErrNotActive), errors.Is(err, downloads.ErrImporting), errors.Is(err, downloads.ErrDownloaderReadOnly):
			message(w, http.StatusConflict, err.Error())
			return
		case err != nil:
			internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// blockRow is one Turned down Release as the old apps' blocklist row.
type blockRow struct {
	ID            int64          `json:"id"`
	SeriesID      int64          `json:"seriesId,omitempty"`
	EpisodeIDs    []int64        `json:"episodeIds,omitempty"`
	MovieID       int64          `json:"movieId,omitempty"`
	SourceTitle   string         `json:"sourceTitle"`
	Languages     []language     `json:"languages"`
	Quality       map[string]any `json:"quality"`
	CustomFormats []any          `json:"customFormats"`
	Date          utc            `json:"date"`
	Protocol      string         `json:"protocol"`
	Indexer       string         `json:"indexer"`
	Message       string         `json:"message"`
}

// blockMax caps how many Turned down Releases one blocklist answer reads.
const blockMax = 5000

// blockRows reads this side's Turned down Releases, newest first.
func (h *handler) blockRows(ctx context.Context) ([]blockRow, error) {
	out := []blockRow{}
	if h.Search == nil {
		return out, nil
	}
	list, _, err := h.Search.List(ctx, "turned_down", blockMax, 0)
	if err != nil {
		return nil, err
	}
	db := h.Store.Read()
	kinds, err := mediafiles.TitleKinds(ctx, db)
	if err != nil {
		return nil, err
	}
	kind := map[string]string{"sonarr": "show", "radarr": "movie"}[h.side]
	var slots []int64
	for _, x := range list {
		slots = append(slots, x.MatchedSlotIDs...)
	}
	items, err := mediafiles.SlotItems(ctx, db, slots)
	if err != nil {
		return nil, err
	}
	for _, x := range list {
		if x.TitleID == 0 || kinds[x.TitleID] != kind {
			continue
		}
		why, _ := x.Verdict["turned_down_reason"].(string)
		row := blockRow{ID: x.ID, SourceTitle: x.Name, Languages: []language{}, Quality: qualityOfRelease(h.side, x.Name), CustomFormats: []any{},
			Date: utc(x.UpdatedAt), Protocol: arrProtocol(x.Protocol), Indexer: x.SourceName, Message: why}
		if kind == "show" {
			row.SeriesID = x.TitleID
			for _, sl := range x.MatchedSlotIDs {
				if it := items[sl]; it > 0 && !slices.Contains(row.EpisodeIDs, it) {
					row.EpisodeIDs = append(row.EpisodeIDs, it)
				}
			}
		} else {
			row.MovieID = x.TitleID
		}
		out = append(out, row)
	}
	return out, nil
}

// blocklist pages this side's Turned down list, newest first.
func (h *handler) blocklist(w http.ResponseWriter, r *http.Request) {
	rows, err := h.blockRows(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	pg := pagingOf(r)
	if query(r, "pageSize") == "" {
		pg.size = 10
	}
	if query(r, "sortDirection") == "" {
		pg.asc, pg.word = false, "descending"
	}
	if pg.asc {
		slices.Reverse(rows)
	}
	from := min((pg.page-1)*pg.size, len(rows))
	to := min(from+pg.size, len(rows))
	writeJSON(w, http.StatusOK, map[string]any{"page": pg.page, "pageSize": pg.size, "sortKey": "date", "sortDirection": pg.word,
		"totalRecords": len(rows), "records": rows[from:to]})
}

// unblock answers DELETE /blocklist/{id}.
func (h *handler) unblock(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.unblockMany(w, r, []int64{id})
}

// unblockBulk answers DELETE /blocklist/bulk with "ids" in the body.
func (h *handler) unblockBulk(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(in.IDs) == 0 || len(in.IDs) > bulkMax {
		message(w, http.StatusBadRequest, "send between 1 and 500 ids")
		return
	}
	h.unblockMany(w, r, in.IDs)
}

// unblockMany takes Releases off the Turned down list, as the app's list
// does. Each must be on this side's list, or nothing changes and it
// answers 404.
func (h *handler) unblockMany(w http.ResponseWriter, r *http.Request, ids []int64) {
	ctx := r.Context()
	rows, err := h.blockRows(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	for _, id := range ids {
		if !slices.ContainsFunc(rows, func(x blockRow) bool { return x.ID == id }) {
			notFound(w)
			return
		}
	}
	for _, id := range ids {
		if _, err := h.Search.RemoveTurnedDown(ctx, id, actor(r)); err != nil {
			internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// taskRow is one scheduled task as the old apps list it. interval is in
// minutes.
type taskRow struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	TaskName      string `json:"taskName"`
	Interval      int64  `json:"interval"`
	LastExecution *utc   `json:"lastExecution,omitempty"`
	LastStartTime *utc   `json:"lastStartTime,omitempty"`
	NextExecution *utc   `json:"nextExecution,omitempty"`
	LastDuration  string `json:"lastDuration"`
}

// taskName writes a Job kind the old apps' way: search_missing is
// SearchMissing.
func taskName(kind string) string {
	var b strings.Builder
	for _, w := range strings.Split(kind, "_") {
		if w != "" {
			b.WriteString(strings.ToUpper(w[:1]) + w[1:])
		}
	}
	return b.String()
}

// taskRows reads the scheduled Job kinds as tasks.
func (h *handler) taskRows(ctx context.Context) ([]taskRow, error) {
	db := h.Store.Read()
	scs, err := jobs.Schedules(ctx, db)
	if err != nil {
		return nil, err
	}
	out := []taskRow{}
	for _, sc := range scs {
		t := taskRow{ID: sc.ID, Name: taskName(sc.Kind), TaskName: taskName(sc.Kind), Interval: sc.EverySeconds / 60,
			LastExecution: utcPtr(sc.LastRun), NextExecution: utcPtr(sc.NextRun), LastDuration: "00:00:00"}
		if !sc.Enabled {
			t.Interval = 0
		}
		started, ended, err := jobs.LastFinished(ctx, db, sc.Kind)
		if err != nil {
			return nil, err
		}
		s, e := parseTime(&started), parseTime(&ended)
		t.LastStartTime = utcPtr(s)
		if s != nil && e != nil && !e.Before(*s) {
			t.LastDuration = timeSpan(int64(e.Sub(*s) / time.Second))
		}
		out = append(out, t)
	}
	return out, nil
}

// systemTasks answers GET /system/task.
func (h *handler) systemTasks(w http.ResponseWriter, r *http.Request) {
	out, err := h.taskRows(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// systemTask answers GET /system/task/{id}.
func (h *handler) systemTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	out, err := h.taskRows(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	for _, t := range out {
		if t.ID == id {
			writeJSON(w, http.StatusOK, t)
			return
		}
	}
	notFound(w)
}

// logPage answers GET /log with an empty page. HolyArrs' log is for
// admins only (Settings > Logs), and a key of this API may only manage the
// Library, so its lines are never sent here.
func (h *handler) logPage(w http.ResponseWriter, r *http.Request) {
	pg := pagingOf(r)
	writeJSON(w, http.StatusOK, map[string]any{"page": pg.page, "pageSize": pg.size, "sortKey": "time", "sortDirection": "descending",
		"totalRecords": 0, "records": []any{}})
}

// updates answers GET /update with the running version only: HolyArrs
// updates itself from its own settings, never from an app's call.
func (h *handler) updates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]any{{"version": h.version, "branch": "main", "releaseDate": zeroDate, "fileName": "", "url": "",
		"installed": true, "installedOn": zeroDate, "installable": false, "latest": true, "changes": map[string]any{"new": []string{}, "fixed": []string{}}, "hash": ""}})
}

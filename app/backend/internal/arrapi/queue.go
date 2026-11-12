package arrapi

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// queueRow is one row of the download list. The Sonarr side has one row
// per episode, so a pack of three episodes is three rows with the same
// downloadId. The Radarr side has one row per download.
type queueRow struct {
	ID                      int64       `json:"id"`
	SeriesID                int64       `json:"seriesId,omitempty"`
	EpisodeID               int64       `json:"episodeId,omitempty"`
	SeasonNumber            *int        `json:"seasonNumber,omitempty"`
	Episode                 *episode    `json:"episode,omitempty"`
	Series                  *series     `json:"series,omitempty"`
	MovieID                 int64       `json:"movieId,omitempty"`
	Size                    int64       `json:"size"`
	Title                   string      `json:"title"`
	Sizeleft                int64       `json:"sizeleft"`
	Timeleft                string      `json:"timeleft,omitempty"`
	EstimatedCompletionTime *utc        `json:"estimatedCompletionTime,omitempty"`
	Added                   *utc        `json:"added,omitempty"`
	Status                  string      `json:"status"`
	TrackedDownloadStatus   string      `json:"trackedDownloadStatus,omitempty"`
	TrackedDownloadState    string      `json:"trackedDownloadState,omitempty"`
	StatusMessages          []statusMsg `json:"statusMessages"`
	DownloadID              string      `json:"downloadId,omitempty"`
	Protocol                string      `json:"protocol"`
	DownloadClient          string      `json:"downloadClient,omitempty"`
	Indexer                 string      `json:"indexer,omitempty"`
	OutputPath              string      `json:"outputPath,omitempty"`
	progress                float64
	eta                     *int64
}

type statusMsg struct {
	Title    string   `json:"title"`
	Messages []string `json:"messages"`
}

// timeSpan writes seconds as the old apps' time-span text, hh:mm:ss. From
// one day on, the days and a dot go in front.
func timeSpan(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	d, rest := sec/86400, sec%86400
	s := fmt.Sprintf("%02d:%02d:%02d", rest/3600, rest%3600/60, rest%60)
	if d > 0 {
		s = strconv.FormatInt(d, 10) + "." + s
	}
	return s
}

// outputPath is where a download's own files are, as HolyArrs sees them
// after the same path map the import uses. It is the save folder plus the
// one top folder all files share, or the single file. The save folder
// alone is shared by every download of its category, and Unpackerr
// unpacks whatever is under outputPath, so the narrower path matters.
// With no file list yet, or no shared top folder, it falls back to the
// save folder.
func outputPath(save string, maps []domain.PathMap, seen []domain.SeenFile) string {
	if save == "" {
		return ""
	}
	root, ok := domain.MapPath(save, maps)
	if !ok {
		return "" // a ".." part could point outside the save folder
	}
	top := ""
	for i, f := range seen {
		name := strings.Trim(strings.ReplaceAll(f.Name, `\`, "/"), "/")
		first, _, _ := strings.Cut(name, "/")
		if first == "" || first == "." || first == ".." || (i > 0 && first != top) {
			return root
		}
		top = first
	}
	if top == "" {
		return root
	}
	sep := "/"
	if strings.Contains(root, `\`) && !strings.Contains(root, "/") {
		sep = `\`
	}
	return strings.TrimRight(root, `/\`) + sep + top
}

// mapPath turns a Downloader's path into HolyArrs' own path. The first
// map entry whose Downloader side starts the path wins.
func mapPath(p string, maps []domain.PathMap) string {
	out, _ := domain.MapPath(p, maps)
	return out
}

// queueSlot is one Slot a download holds, with its episode or movie.
type queueSlot struct {
	state    string
	itemID   int64
	titleID  int64
	itemKind string
	season   *int
}

// queueDefaultSize is the page size when the caller sends none. The old
// apps use 10, but Seerr never asks for a second page, so the first page
// holds every row.
const queueDefaultSize = 1000

// queue lists what is downloading and what finished but is not in the
// Library yet. It only reads, because the background download reader
// already keeps the states fresh.
func (h *handler) queue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.queueRows(ctx, queryBool(r, "includeEpisode"))
	if err != nil {
		internalError(w, err)
		return
	}
	key := strings.ToLower(query(r, "sortKey"))
	switch key {
	case "timeleft", "title", "size", "progress", "status", "estimatedcompletiontime":
	default:
		key = "timeleft"
	}
	dir := strings.ToLower(query(r, "sortDirection"))
	asc := dir == "ascending" || dir == "default"
	if !asc {
		dir = "descending"
	}
	order := func(a, b queueRow) int {
		var c int
		switch key {
		case "title":
			c = strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		case "size":
			c = cmp.Compare(a.Size, b.Size)
		case "progress":
			c = cmp.Compare(a.progress, b.progress)
		case "status":
			c = strings.Compare(a.Status, b.Status)
		default: // a row with no time left counts as the longest
			ta, tb := int64(1<<62), int64(1<<62)
			if a.eta != nil {
				ta = *a.eta
			}
			if b.eta != nil {
				tb = *b.eta
			}
			c = cmp.Compare(ta, tb)
		}
		if !asc {
			c = -c
		}
		if c == 0 {
			c = cmp.Compare(b.progress, a.progress) // on a tie, the furthest along goes first
		}
		if c == 0 {
			c = cmp.Compare(a.ID, b.ID)
		}
		return c
	}
	slices.SortStableFunc(rows, order)
	size := queueDefaultSize
	if n, err := strconv.Atoi(query(r, "pageSize")); err == nil && n > 0 {
		size = min(n, 10000)
	}
	page := 1
	if n, err := strconv.ParseInt(query(r, "page"), 10, 32); (err == nil || errors.Is(err, strconv.ErrRange)) && n > 0 {
		page = int(min(n, math.MaxInt32)) // past the end on a 32-bit build too
	}
	// A page past the end shows the last page that has rows.
	if last := max(1, (len(rows)+size-1)/size); page > last {
		page = last
	}
	from := min((page-1)*size, len(rows))
	to := min(from+size, len(rows))
	writeJSON(w, http.StatusOK, map[string]any{"page": page, "pageSize": size, "sortKey": key, "sortDirection": dir,
		"totalRecords": len(rows), "records": rows[from:to]})
}

// queueRows reads this side's rows in a fixed number of queries, however
// long the list is.
func (h *handler) queueRows(ctx context.Context, withEpisode bool) ([]queueRow, error) {
	db := h.Store.Read()
	rows, err := downloads.QueueRows(ctx, db)
	if err != nil {
		return nil, err
	}
	type dl struct {
		row   queueRow
		state string
		why   string
		slots []int64
	}
	var dls []dl
	var allSlots []int64
	for _, r := range rows {
		d := dl{state: r.State, why: r.StateReason}
		d.row.ID, d.row.DownloadID, d.row.progress, d.row.eta = r.ID, r.ExternalID, r.Progress, r.ETA
		d.row.Title, d.row.Size, d.row.Indexer, d.row.DownloadClient, d.row.Protocol = r.ReleaseName, r.ReleaseSize, r.SourceName, r.DownloaderName, r.Protocol
		_ = json.Unmarshal([]byte(r.SlotIDs), &d.slots)
		var seen []domain.SeenFile
		var maps []domain.PathMap
		_ = json.Unmarshal([]byte(r.FilesSeen), &seen)
		_ = json.Unmarshal([]byte(r.PathMap), &maps)
		d.row.OutputPath = outputPath(r.SavePath, maps, seen)
		added := r.CreatedAt
		d.row.Added = utcPtr(parseTime(&added))
		allSlots = append(allSlots, d.slots...)
		dls = append(dls, d)
	}
	out := []queueRow{}
	if len(allSlots) == 0 {
		return out, nil
	}
	slots := map[int64]queueSlot{}
	slotItems, err := library.SlotItems(ctx, db, allSlots)
	if err != nil {
		return nil, err
	}
	var items []int64
	for _, si := range slotItems {
		slots[si.SlotID] = queueSlot{state: si.State, itemID: si.ItemID, titleID: si.TitleID, itemKind: si.ItemKind, season: si.Season}
		items = append(items, si.ItemID)
	}
	eps := map[int64]episode{}
	if withEpisode && h.side == "sonarr" && len(items) > 0 {
		list, err := h.episodesWhere(ctx, "id IN "+idList(items))
		if err != nil {
			return nil, err
		}
		for _, e := range list {
			eps[e.ID] = e
		}
	}
	now := h.now()
	for _, d := range dls {
		base := d.row
		if base.Protocol != "usenet" && base.Protocol != "torrent" {
			base.Protocol = "unknown"
		}
		base.Sizeleft = int64(float64(base.Size) * (1 - min(max(base.progress, 0), 1)))
		base.StatusMessages = []statusMsg{}
		blocked, importing := false, false
		for _, id := range d.slots {
			switch slots[id].state {
			case "needs_a_decision":
				blocked = true
			case "importing":
				importing = true
			}
		}
		base.TrackedDownloadStatus, base.TrackedDownloadState = "ok", "downloading"
		switch d.state {
		case "sent", "queued":
			base.Status = "queued"
		case "downloading":
			base.Status = "downloading"
		case "paused":
			base.Status = "paused"
		case "stalled":
			base.Status, base.TrackedDownloadStatus = "warning", "warning"
			base.StatusMessages = []statusMsg{{Title: base.Title, Messages: []string{"The download is stalled: " + d.why}}}
		default: // finished, waiting for its import
			base.Status, base.TrackedDownloadState = "completed", "importPending"
			zero := int64(0)
			base.eta = &zero
			switch {
			case blocked:
				base.TrackedDownloadStatus, base.TrackedDownloadState = "warning", "importBlocked"
				base.StatusMessages = []statusMsg{{Title: base.Title, Messages: []string{"HolyArrs needs a decision before it can import this."}}}
			case importing:
				base.TrackedDownloadState = "importing"
			}
		}
		if base.eta != nil && *base.eta >= 0 {
			base.Timeleft = timeSpan(*base.eta)
			done := utc(now.Add(time.Duration(*base.eta) * time.Second))
			base.EstimatedCompletionTime = &done
		}
		seen := map[int64]bool{}
		for n, id := range d.slots {
			q, ok := slots[id]
			if !ok || seen[q.itemID] {
				continue
			}
			row := base
			row.ID = d.row.ID*1000 + int64(n%1000)
			if h.side == "sonarr" {
				if q.itemKind != "episode" {
					continue
				}
				seen[q.itemID] = true
				row.SeriesID, row.EpisodeID, row.SeasonNumber = q.titleID, q.itemID, q.season
				if e, ok := eps[q.itemID]; ok {
					row.Episode = &e
				}
				out = append(out, row)
				continue
			}
			if q.itemKind == "movie" {
				row.MovieID = q.titleID
				out = append(out, row)
				break
			}
		}
	}
	return out, nil
}

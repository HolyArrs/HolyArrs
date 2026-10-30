package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Everything in the client: the Downloads screen lists every torrent a
// torrent Downloader holds, whatever its category, as the client's own
// screen does. Torrents HolyArrs did not send are read only: they never
// become download records, so the sync, failure and import logic never
// sees them. A person who may manage the Library can still steer them
// through the client, addressed by Downloader and info hash.

// clientKey is one item in one Downloader.
type clientKey struct {
	downloader int64
	hash       string
}

// clientListWait caps how long the Downloads screen waits for a client.
var clientListWait = 5 * time.Second

// clientRows reads every torrent Downloader's full list (one call each, side
// by side) and returns the items not already shown as HolyArrs rows.
func (a *API) clientRows(ctx context.Context, shown map[clientKey]bool) []DownloadRow {
	ds, err := a.manager().Downloaders(ctx)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, clientListWait)
	defer cancel()
	per := make([][]DownloadRow, len(ds))
	var wg sync.WaitGroup
	for i, d := range ds {
		if d.Protocol != "torrent" {
			continue
		}
		wg.Add(1)
		go func(i int, d domain.Downloader) {
			defer wg.Done()
			var jobs []downloader.Job
			_ = a.manager().ControlEngine(ctx, d, func(e downloader.Engine) error {
				l, ok := e.(downloader.AllLister)
				if !ok {
					return downloader.ErrNotSupported
				}
				var err error
				jobs, err = l.ListAll(ctx)
				return err
			})
			out := make([]DownloadRow, 0, len(jobs))
			for _, j := range jobs {
				if j.ID == "" || shown[clientKey{d.ID, j.ID}] {
					continue
				}
				out = append(out, clientRow(d, j))
			}
			per[i] = out
		}(i, d)
	}
	wg.Wait()
	var rows []DownloadRow
	defer func() { a.matchQueue(context.WithoutCancel(ctx), rows) }()
	for i, out := range per {
		if len(out) > 0 {
			a.knownTitles(ctx, ds[i].ID, out)
			a.linkedTitles(ctx, ds[i].ID, out)
			rows = append(rows, out...)
		}
	}
	return rows
}

// clientRow is one client item in the Downloads screen's words.
func clientRow(d domain.Downloader, j downloader.Job) DownloadRow {
	ph, st, active := clientPhase(j)
	row := DownloadRow{
		DownloadView: DownloadView{State: st, Label: PhaseLabels[ph], Progress: j.Progress, Speed: j.Speed, UpSpeed: j.UpSpeed, Seeds: j.Seeds, Peers: j.Peers, Active: active},
		Phase:        ph, Name: j.Name, Downloader: d.Name, DownloaderKind: d.Adapter, Protocol: d.Protocol, Size: j.Size,
		UpdatedAt: time.Now().UTC(), CanAct: true, ClientItem: true, Outside: true, DownloaderID: d.ID, Hash: j.ID,
		Category: j.Category, Ratio: j.Ratio, Limits: j.Limits, ClientHint: clientHint(j.Detail),
	}
	if j.ETA > 0 && active {
		eta := j.ETA
		row.ETA = &eta
	}
	if j.AddedAt > 0 {
		t := time.Unix(j.AddedAt, 0).UTC()
		row.StartedAt = &t
	}
	if ph == "done" {
		row.Label = "Done"
	}
	return row
}

// clientHint is the finer word for a client torrent's own state.
func clientHint(state string) string {
	switch state {
	case "checkingDL", "checkingUP", "checkingResumeData":
		return "checking"
	case "moving":
		return "moving"
	case "error":
		return "errored"
	case "missingFiles":
		return "missing"
	case "stalledUP":
		return "idle_seed"
	}
	return ""
}

// clientPhase maps a client item onto the Downloads screen's phase and
// state: a finished torrent still shared is done and seeding.
func clientPhase(j downloader.Job) (phase, state string, active bool) {
	switch j.State {
	case downloader.StateQueued:
		return "waiting", "queued", true
	case downloader.StatePaused:
		return "paused", "paused", true
	case downloader.StateFailed:
		return "failed", "failed", false
	case downloader.StateFinished:
		switch j.Detail {
		case "pausedUP", "stoppedUP":
			return "done", "paused", false
		}
		return "done", "seeding", false
	}
	if j.Detail == "stalledDL" || j.Detail == "metaDL" && j.Speed == 0 {
		return "stalled", "stalled", true
	}
	return "downloading", "downloading", true
}

// knownTitles names the client items HolyArrs already knows by hash (its
// own downloads older than the list), and drops their outside mark.
func (a *API) knownTitles(ctx context.Context, downloaderID int64, rows []DownloadRow) {
	at := map[string]int{}
	for i, r := range rows {
		at[r.Hash] = i
	}
	hashes := make([]string, 0, len(rows))
	for h := range at {
		hashes = append(hashes, h)
	}
	db := a.s.Store.Read()
	for len(hashes) > 0 {
		n := min(len(hashes), 400)
		part := hashes[:n]
		hashes = hashes[n:]
		args := []any{downloaderID}
		for _, h := range part {
			args = append(args, h)
		}
		ds, err := store.Select[domain.Download](ctx, db, "downloads", "downloader_id = ? AND lower(external_id) IN (?"+strings.Repeat(", ?", len(part)-1)+")", args...)
		if err != nil {
			continue
		}
		for _, d := range ds {
			i, ok := at[strings.ToLower(d.ExternalID)]
			if !ok {
				continue
			}
			row := &rows[i]
			row.Outside = false
			if len(d.SlotIDs) == 0 {
				continue
			}
			_ = db.QueryRowContext(ctx, `SELECT t.id, t.name, t.kind, v.label FROM slots s JOIN items i ON i.id = s.item_id
				JOIN titles t ON t.id = i.title_id JOIN versions v ON v.id = s.version_id WHERE s.id = ?`, d.SlotIDs[0]).Scan(&row.TitleID, &row.Title, &row.Kind, &row.Version)
		}
	}
}

// Client item routes: the client controls, addressed by Downloader id and
// the client's own item id (a torrent's info hash).
func (a *API) clientItemRoutes(m *http.ServeMux) {
	write := func(f func(ctx context.Context, e downloader.Engine, id string) error) http.HandlerFunc {
		return a.itemAct(true, func(r *http.Request, e downloader.Engine, id string) error { return f(r.Context(), e, id) })
	}
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/pause", write(func(ctx context.Context, e downloader.Engine, id string) error {
		return asController(e, func(c downloader.Controller) error { return c.PauseItem(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/resume", write(func(ctx context.Context, e downloader.Engine, id string) error {
		return asController(e, func(c downloader.Controller) error { return c.ResumeItem(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/top", write(func(ctx context.Context, e downloader.Engine, id string) error {
		return asController(e, func(c downloader.Controller) error { return c.MoveTop(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/recheck", write(func(ctx context.Context, e downloader.Engine, id string) error {
		return asTorrent(e, func(t downloader.TorrentController) error { return t.Recheck(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/reannounce", write(func(ctx context.Context, e downloader.Engine, id string) error {
		return asTorrent(e, func(t downloader.TorrentController) error { return t.Reannounce(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/remove", a.removeItem)
	m.HandleFunc("GET /api/v1/downloaders/{id}/items/{hash}/detail", a.itemDetail)
	m.HandleFunc("GET /api/v1/downloaders/{id}/items/{hash}/swarm", a.itemSwarm)
	m.HandleFunc("GET /api/v1/downloaders/{id}/items/{hash}/files", a.itemFiles)
	m.HandleFunc("PUT /api/v1/downloaders/{id}/items/{hash}/files", a.setItemFiles)
}

// itemAct runs a call on one client item and answers ok.
func (a *API) itemAct(write bool, f func(r *http.Request, e downloader.Engine, id string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.onItem(r, write, func(e downloader.Engine, id string) error { return f(r, e, id) }); err != nil {
			itemError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (a *API) onItem(r *http.Request, write bool, call func(e downloader.Engine, id string) error) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return downloader.ErrNoDownloader
	}
	return a.manager().ControlItem(r.Context(), id, r.PathValue("hash"), write, call)
}

func itemError(w http.ResponseWriter, err error) {
	if errors.Is(err, downloader.ErrNoDownloader) {
		fail(w, http.StatusNotFound, err.Error())
		return
	}
	clientControlError(w, err)
}

// removeItem takes one item out of the client, with or without its files.
func (a *API) removeItem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeleteFiles bool `json:"delete_files"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in, `body is {"delete_files": true or false}`) {
		return
	}
	// Removing a torrent HolyArrs did not send is a Downloader owner's call:
	// it needs the settings permission (and an admin-scoped key).
	if id, err := strconv.ParseInt(r.PathValue("id"), 10, 64); err == nil && a.outsideHash(r.Context(), id, r.PathValue("hash")) {
		if who, ok := auth.From(r.Context()); ok && (!who.Has(auth.PermSettings) || !who.HasScope(auth.ScopeAdmin)) {
			fail(w, http.StatusForbidden, "removing a torrent HolyArrs did not send needs the settings permission")
			return
		}
	}
	a.itemAct(true, func(r *http.Request, e downloader.Engine, id string) error {
		return e.Remove(r.Context(), id, in.DeleteFiles)
	})(w, r)
}

// itemDetail is a client item's Info, read live; it has no HolyArrs log.
func (a *API) itemDetail(w http.ResponseWriter, r *http.Request) {
	out := DownloadDetailView{Log: []DownloadLogLine{}}
	err := a.onItem(r, false, func(e downloader.Engine, id string) error {
		return asInspector(e, func(t downloader.TorrentInspector) error {
			p, err := t.Properties(r.Context(), id)
			if err == nil {
				out.Torrent = &p
			}
			return err
		})
	})
	if errors.Is(err, downloader.ErrNoDownloader) || errors.Is(err, downloader.ErrNotFound) {
		itemError(w, err)
		return
	}
	if err != nil && !errors.Is(err, downloader.ErrNotSupported) {
		out.TorrentErr = err.Error()
	}
	if id, perr := strconv.ParseInt(r.PathValue("id"), 10, 64); perr == nil {
		_ = a.s.Store.Read().QueryRowContext(r.Context(), "SELECT name FROM downloaders WHERE id = ?", id).Scan(&out.Downloader)
	}
	writeJSON(w, http.StatusOK, out)
}

// itemSwarm lists a client item's trackers and peers.
func (a *API) itemSwarm(w http.ResponseWriter, r *http.Request) {
	out := struct {
		Trackers []downloader.Tracker `json:"trackers"`
		Peers    []downloader.Peer    `json:"peers"`
	}{}
	err := a.onItem(r, false, func(e downloader.Engine, id string) error {
		return asTorrent(e, func(t downloader.TorrentController) error {
			var err error
			if out.Trackers, err = t.Trackers(r.Context(), id); err != nil {
				return err
			}
			out.Peers, err = t.Peers(r.Context(), id)
			return err
		})
	})
	if err != nil {
		itemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// itemFiles lists a client item's files with their priority.
func (a *API) itemFiles(w http.ResponseWriter, r *http.Request) {
	var files []downloader.TorrentFile
	err := a.onItem(r, false, func(e downloader.Engine, id string) error {
		return asInspector(e, func(t downloader.TorrentInspector) error {
			var err error
			files, err = t.TorrentFiles(r.Context(), id)
			return err
		})
	})
	if err != nil {
		itemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// setItemFiles sets which files of a client item to fetch and how soon.
func (a *API) setItemFiles(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Indexes  []int `json:"indexes"`
		Priority int   `json:"priority"`
	}
	if !decode(w, r, &in, `body is {"indexes": [file index...], "priority": 0 (skip), 1 (normal), 6 (high) or 7 (maximum)}`) {
		return
	}
	if len(in.Indexes) == 0 || len(in.Indexes) > 10000 {
		fail(w, http.StatusBadRequest, "indexes needs 1 to 10000 file indexes")
		return
	}
	switch in.Priority {
	case 0, 1, 6, 7:
	default:
		fail(w, http.StatusBadRequest, downloader.ErrFilePriority.Error())
		return
	}
	a.itemAct(true, func(r *http.Request, e downloader.Engine, id string) error {
		return asInspector(e, func(t downloader.TorrentInspector) error {
			return t.SetFilePriority(r.Context(), id, in.Indexes, in.Priority)
		})
	})(w, r)
}

package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// The client controls: what the Downloaders themselves do with a download
// (pause, resume, recheck, move to top) and with everything (pause all,
// speed limit). The state shown afterwards always comes from the client.
func (a *API) downloadControlRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/downloads/{id}/pause", a.downloadAct(func(ctx context.Context, e downloader.Engine, id string) error {
		return asController(e, func(c downloader.Controller) error { return c.PauseItem(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloads/{id}/resume", a.downloadAct(func(ctx context.Context, e downloader.Engine, id string) error {
		return asController(e, func(c downloader.Controller) error { return c.ResumeItem(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloads/{id}/top", a.downloadAct(func(ctx context.Context, e downloader.Engine, id string) error {
		return asController(e, func(c downloader.Controller) error { return c.MoveTop(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloads/{id}/recheck", a.downloadAct(func(ctx context.Context, e downloader.Engine, id string) error {
		return asTorrent(e, func(t downloader.TorrentController) error { return t.Recheck(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloads/{id}/reannounce", a.downloadAct(func(ctx context.Context, e downloader.Engine, id string) error {
		return asTorrent(e, func(t downloader.TorrentController) error { return t.Reannounce(ctx, id) })
	}))
	m.HandleFunc("GET /api/v1/downloads/{id}/swarm", a.downloadSwarm)
	m.HandleFunc("GET /api/v1/downloads/{id}/detail", a.downloadDetail)
	m.HandleFunc("GET /api/v1/downloads/{id}/files", a.downloadFiles)
	m.HandleFunc("PUT /api/v1/downloads/{id}/files", a.setDownloadFiles)
	m.HandleFunc("GET /api/v1/downloads/clients", a.downloadClients)
	m.HandleFunc("POST /api/v1/downloads/pause-all", a.allAct(true))
	m.HandleFunc("POST /api/v1/downloads/resume-all", a.allAct(false))
	m.HandleFunc("PUT /api/v1/downloaders/{id}/speed-limit", a.speedLimit)
	a.clientItemRoutes(m)
	a.usenetRoutes(m)
	a.clientManageRoutes(m)
}

func asController(e downloader.Engine, f func(downloader.Controller) error) error {
	c, ok := e.(downloader.Controller)
	if !ok {
		return downloader.ErrNotSupported
	}
	return f(c)
}

func asTorrent(e downloader.Engine, f func(downloader.TorrentController) error) error {
	t, ok := e.(downloader.TorrentController)
	if !ok {
		return downloader.ErrNotSupported
	}
	return f(t)
}

func clientControlError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "no such download")
	case errors.Is(err, downloader.ErrNotActive), errors.Is(err, downloader.ErrNotSupported), errors.Is(err, downloader.ErrReadOnly):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, downloader.ErrNotFound):
		fail(w, http.StatusNotFound, err.Error())
	default:
		// The Downloader's own answer, in our words (adapters never put
		// keys or addresses in errors).
		fail(w, http.StatusBadGateway, err.Error())
	}
}

func (a *API) downloadAct(f func(ctx context.Context, e downloader.Engine, id string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idIn(w, r, "download")
		if !ok {
			return
		}
		err := a.manager().Control(r.Context(), id, func(e downloader.Engine, ext string) error { return f(r.Context(), e, ext) })
		if err != nil {
			clientControlError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// downloadSwarm lists a torrent's trackers and peers.
func (a *API) downloadSwarm(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	out := struct {
		Trackers []downloader.Tracker `json:"trackers"`
		Peers    []downloader.Peer    `json:"peers"`
	}{}
	var hash string
	err := a.manager().Inspect(r.Context(), id, func(e downloader.Engine, ext string) error {
		hash = ext
		return asTorrent(e, func(t downloader.TorrentController) error {
			var err error
			if out.Trackers, err = t.Trackers(r.Context(), ext); err != nil {
				return err
			}
			out.Peers, err = t.Peers(r.Context(), ext)
			return err
		})
	})
	if err != nil {
		clientControlError(w, err)
		return
	}
	var dlr int64
	if a.s.Store.Read().QueryRowContext(r.Context(), "SELECT downloader_id FROM downloads WHERE id = ?", id).Scan(&dlr) == nil && hash != "" {
		a.noteTrackers(r.Context(), dlr, hash, out.Trackers)
	}
	writeJSON(w, http.StatusOK, out)
}

func asInspector(e downloader.Engine, f func(downloader.TorrentInspector) error) error {
	t, ok := e.(downloader.TorrentInspector)
	if !ok {
		return downloader.ErrNotSupported
	}
	return f(t)
}

// DownloadLogLine is one thing HolyArrs did or saw for a download.
type DownloadLogLine struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Summary string    `json:"summary"`
}

// DownloadDetailView is the Info and Log of one download: what HolyArrs
// knows, plus the torrent client's own detail while it still has it.
type DownloadDetailView struct {
	Downloader  string                    `json:"downloader"`
	RequestedBy string                    `json:"requested_by"`
	Torrent     *downloader.TorrentDetail `json:"torrent,omitempty"`
	TorrentErr  string                    `json:"torrent_error,omitempty"`
	Log         []DownloadLogLine         `json:"log"`
}

// downloadDetail reads one download's Info and Log. The client part is
// left out when the client is not asked (finished, usenet) or fails.
func (a *API) downloadDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	ctx := r.Context()
	db := a.s.Store.Read()
	var d domain.Download
	if err := store.Get(ctx, db, "downloads", id, &d); err != nil {
		clientControlError(w, err)
		return
	}
	out := DownloadDetailView{Log: []DownloadLogLine{}}
	var dr domain.Downloader
	if store.Get(ctx, db, "downloaders", d.DownloaderID, &dr) == nil {
		out.Downloader = dr.Name
	}
	switch d.StartedBy.Kind {
	case "user":
		_ = db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = ?", d.StartedBy.ID).Scan(&out.RequestedBy)
	case "", "system":
		// No asker: name the person who added the Library title, if a person did.
		if len(d.SlotIDs) > 0 {
			var titleID int64
			_ = db.QueryRowContext(ctx, "SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", d.SlotIDs[0]).Scan(&titleID)
			out.RequestedBy = a.titleOwner(ctx, titleID)
		}
	default:
		out.RequestedBy = "HolyArrs"
	}
	rows, err := a.s.Store.ReadEvents().QueryContext(ctx, "SELECT at, kind, summary FROM events WHERE download_id = ? ORDER BY at DESC, id DESC LIMIT 200", id)
	if err != nil {
		internal(w, err)
		return
	}
	for rows.Next() {
		var l DownloadLogLine
		var at string
		if rows.Scan(&at, &l.Kind, &l.Summary) == nil {
			l.At, _ = time.Parse(store.TimeLayout, at)
			out.Log = append(out.Log, l)
		}
	}
	rows.Close()
	// Read live whenever the client may still hold it: seeding and
	// finished torrents too.
	if dr.Protocol == "torrent" && d.ExternalID != "" && downloader.Inspectable(d.State) {
		err := a.manager().ControlEngine(ctx, dr, func(e downloader.Engine) error {
			return asInspector(e, func(t downloader.TorrentInspector) error {
				p, err := t.Properties(ctx, d.ExternalID)
				if err == nil {
					out.Torrent = &p
				}
				return err
			})
		})
		if err != nil && !errors.Is(err, downloader.ErrNotSupported) && (isActiveDownload(d.State) || !errors.Is(err, downloader.ErrNotFound)) {
			out.TorrentErr = err.Error()
		}
	}
	if dr.Protocol == "torrent" && d.ExternalID != "" {
		out.Log = a.torrentTimeline(ctx, d.DownloaderID, d.ExternalID, out.Torrent, out.Log)
	}
	writeJSON(w, http.StatusOK, out)
}

func isActiveDownload(state string) bool {
	switch state {
	case "sent", "queued", "downloading", "stalled", "paused":
		return true
	}
	return false
}

// downloadFiles lists a torrent's files with their priority.
func (a *API) downloadFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	var files []downloader.TorrentFile
	err := a.manager().Inspect(r.Context(), id, func(e downloader.Engine, ext string) error {
		return asInspector(e, func(t downloader.TorrentInspector) error {
			var err error
			files, err = t.TorrentFiles(r.Context(), ext)
			return err
		})
	})
	if err != nil {
		clientControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

// setDownloadFiles sets which files of a torrent to fetch and how soon.
func (a *API) setDownloadFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
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
	err := a.manager().Control(r.Context(), id, func(e downloader.Engine, ext string) error {
		return asInspector(e, func(t downloader.TorrentInspector) error {
			return t.SetFilePriority(r.Context(), ext, in.Indexes, in.Priority)
		})
	})
	if err != nil {
		clientControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ClientView is one Downloader with what it says about itself.
type ClientView struct {
	ID       int64                    `json:"id"`
	Name     string                   `json:"name"`
	Kind     string                   `json:"kind"`
	Protocol string                   `json:"protocol"`
	Controls bool                     `json:"controls"`
	Torrent  bool                     `json:"torrent_controls"`
	Status   *downloader.ClientStatus `json:"status,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

func (a *API) eachClient(ctx context.Context, f func(d domain.Downloader, e downloader.Engine, v *ClientView) error) ([]ClientView, error) {
	ds, err := a.manager().Downloaders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ClientView, len(ds))
	var wg sync.WaitGroup
	for i, d := range ds {
		out[i] = ClientView{ID: d.ID, Name: d.Name, Kind: d.Adapter, Protocol: d.Protocol}
		wg.Add(1)
		go func(i int, d domain.Downloader) {
			defer wg.Done()
			v := &out[i]
			err := a.manager().ControlEngine(ctx, d, func(e downloader.Engine) error {
				_, v.Controls = e.(downloader.Controller)
				_, v.Torrent = e.(downloader.TorrentController)
				return f(d, e, v)
			})
			if err != nil && !errors.Is(err, downloader.ErrNotSupported) {
				v.Error = err.Error()
			}
		}(i, d)
	}
	wg.Wait()
	return out, nil
}

// downloadClients shows each Downloader's state: paused, speeds, limits,
// totals and free space, where the client reports them.
func (a *API) downloadClients(w http.ResponseWriter, r *http.Request) {
	out, err := a.eachClient(r.Context(), func(d domain.Downloader, e downloader.Engine, v *ClientView) error {
		return asController(e, func(c downloader.Controller) error {
			if d.Adapter == "qbittorrent" && a.s.DLSync != nil {
				if st, ok := a.s.DLSync.ClientStatus(d.ID); ok {
					v.Status = &st
					return nil
				}
			}
			st, err := c.Status(r.Context())
			if err == nil {
				v.Status = &st
			}
			return err
		})
	})
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"clients": out})
}

// allAct pauses or resumes everything in every Downloader that can.
func (a *API) allAct(pause bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := a.eachClient(r.Context(), func(d domain.Downloader, e downloader.Engine, _ *ClientView) error {
			// A read-only Downloader is left alone, not reported as a failure of Pause all.
			if downloader.ReadOnlyFor(d) {
				return downloader.ErrNotSupported
			}
			return asController(e, func(c downloader.Controller) error {
				if pause {
					return c.PauseAll(r.Context())
				}
				return c.ResumeAll(r.Context())
			})
		})
		if err != nil {
			internal(w, err)
			return
		}
		a.manager().Poke()
		writeJSON(w, http.StatusOK, map[string]any{"clients": out})
	}
}

// speedLimit sets one Downloader's total speed cap in bytes per second;
// 0 removes it.
func (a *API) speedLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	var in struct {
		Down int64 `json:"down"`
		Up   int64 `json:"up"`
	}
	if !decode(w, r, &in, `body is {"down": bytes per second, "up": bytes per second}; 0 means no limit`) {
		return
	}
	if in.Down < 0 || in.Up < 0 {
		fail(w, http.StatusBadRequest, "limits are 0 (none) or more")
		return
	}
	var d domain.Downloader
	if err := store.Get(r.Context(), a.s.Store.Read(), "downloaders", id, &d); err != nil || d.Removed {
		fail(w, http.StatusNotFound, "no such Downloader")
		return
	}
	err := a.manager().ControlEngine(r.Context(), d, func(e downloader.Engine) error {
		return asController(e, func(c downloader.Controller) error { return c.SetSpeedLimit(r.Context(), in.Down, in.Up) })
	})
	if err != nil {
		clientControlError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

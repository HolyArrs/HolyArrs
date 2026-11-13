package downloads

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// The client controls: what the Downloaders themselves do with a download
// (pause, resume, recheck, move to top) and with everything (pause all,
// speed limit). The state shown afterwards always comes from the client.
func (a *Web) downloadControlRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/downloads/{id}/pause", a.downloadAct(func(ctx context.Context, e Engine, id string) error {
		return asController(e, func(c Controller) error { return c.PauseItem(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloads/{id}/resume", a.downloadAct(func(ctx context.Context, e Engine, id string) error {
		return asController(e, func(c Controller) error { return c.ResumeItem(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloads/{id}/top", a.downloadAct(func(ctx context.Context, e Engine, id string) error {
		return asController(e, func(c Controller) error { return c.MoveTop(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloads/{id}/recheck", a.downloadAct(func(ctx context.Context, e Engine, id string) error {
		return asTorrent(e, func(t TorrentController) error { return t.Recheck(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloads/{id}/reannounce", a.downloadAct(func(ctx context.Context, e Engine, id string) error {
		return asTorrent(e, func(t TorrentController) error { return t.Reannounce(ctx, id) })
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

func asController(e Engine, f func(Controller) error) error {
	c, ok := e.(Controller)
	if !ok {
		return ErrNotSupported
	}
	return f(c)
}

func asTorrent(e Engine, f func(TorrentController) error) error {
	t, ok := e.(TorrentController)
	if !ok {
		return ErrNotSupported
	}
	return f(t)
}

func clientControlError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, database.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		webserver.Fail(w, http.StatusNotFound, "no such download")
	case errors.Is(err, ErrNotOurs):
		webserver.Fail(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrNotActive), errors.Is(err, ErrNotSupported), errors.Is(err, ErrReadOnly):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	default:
		// The Downloader's own answer, in our words (adapters never put
		// keys or addresses in errors).
		webserver.Fail(w, http.StatusBadGateway, err.Error())
	}
}

func (a *Web) downloadAct(f func(ctx context.Context, e Engine, id string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idIn(w, r, "download")
		if !ok {
			return
		}
		err := a.Manager().Control(r.Context(), id, func(e Engine, ext string) error { return f(r.Context(), e, ext) })
		if err != nil {
			clientControlError(w, err)
			return
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// downloadSwarm lists a torrent's trackers and peers.
func (a *Web) downloadSwarm(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	out := struct {
		Trackers []Tracker `json:"trackers"`
		Peers    []Peer    `json:"peers"`
	}{}
	var hash string
	err := a.Manager().Inspect(r.Context(), id, func(e Engine, ext string) error {
		hash = ext
		return asTorrent(e, func(t TorrentController) error {
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
	if a.Store.Read().QueryRowContext(r.Context(), "SELECT downloader_id FROM downloads WHERE id = ?", id).Scan(&dlr) == nil && hash != "" {
		a.noteTrackers(r.Context(), dlr, hash, out.Trackers)
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

func asInspector(e Engine, f func(TorrentInspector) error) error {
	t, ok := e.(TorrentInspector)
	if !ok {
		return ErrNotSupported
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
	Downloader  string            `json:"downloader"`
	RequestedBy string            `json:"requested_by"`
	Torrent     *TorrentDetail    `json:"torrent,omitempty"`
	TorrentErr  string            `json:"torrent_error,omitempty"`
	Log         []DownloadLogLine `json:"log"`
}

// downloadDetail reads one download's Info and Log. The client part is
// left out when the client is not asked (finished, usenet) or fails.
func (a *Web) downloadDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	ctx := r.Context()
	db := a.Store.Read()
	var d domain.Download
	if err := database.Get(ctx, db, "downloads", id, &d); err != nil {
		clientControlError(w, err)
		return
	}
	out := DownloadDetailView{Log: []DownloadLogLine{}}
	var dr domain.Downloader
	if database.Get(ctx, db, "downloaders", d.DownloaderID, &dr) == nil {
		out.Downloader = dr.Name
	}
	if d.StartedBy.Kind == "user" {
		_ = db.QueryRowContext(ctx, "SELECT name FROM users WHERE id = ?", d.StartedBy.ID).Scan(&out.RequestedBy)
	}
	if out.RequestedBy == "" && len(d.SlotIDs) > 0 {
		// No person picked it: name the person who added the Library title, if a person did.
		var titleID int64
		_ = db.QueryRowContext(ctx, "SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", d.SlotIDs[0]).Scan(&titleID)
		out.RequestedBy = a.titleOwner(ctx, titleID)
	}
	if out.RequestedBy == "" {
		out.RequestedBy = "HolyArrs"
	}
	rows, err := a.Store.ReadEvents().QueryContext(ctx, "SELECT at, kind, summary FROM events WHERE download_id = ? ORDER BY at DESC, id DESC LIMIT 200", id)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	for rows.Next() {
		var l DownloadLogLine
		var at string
		if rows.Scan(&at, &l.Kind, &l.Summary) == nil {
			l.At, _ = time.Parse(database.TimeLayout, at)
			out.Log = append(out.Log, l)
		}
	}
	rows.Close()
	// Read live whenever the client may still hold it: seeding and
	// finished torrents too.
	if dr.Protocol == "torrent" && d.ExternalID != "" && Inspectable(d.State) {
		err := a.Manager().ControlEngine(ctx, dr, func(e Engine) error {
			return asInspector(e, func(t TorrentInspector) error {
				p, err := t.Properties(ctx, d.ExternalID)
				if err == nil {
					out.Torrent = &p
				}
				return err
			})
		})
		if err != nil && !errors.Is(err, ErrNotSupported) && (isActiveDownload(d.State) || !errors.Is(err, ErrNotFound)) {
			out.TorrentErr = err.Error()
		}
	}
	if dr.Protocol == "torrent" && d.ExternalID != "" {
		out.Log = a.TorrentTimeline(ctx, d.DownloaderID, d.ExternalID, out.Torrent, out.Log)
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

func isActiveDownload(state string) bool {
	switch state {
	case "sent", "queued", "downloading", "stalled", "paused":
		return true
	}
	return false
}

// downloadFiles lists a torrent's files with their priority.
func (a *Web) downloadFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	var files []TorrentFile
	err := a.Manager().Inspect(r.Context(), id, func(e Engine, ext string) error {
		return asInspector(e, func(t TorrentInspector) error {
			var err error
			files, err = t.TorrentFiles(r.Context(), ext)
			return err
		})
	})
	if err != nil {
		clientControlError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"files": files})
}

// setDownloadFiles sets which files of a torrent to fetch and how soon.
func (a *Web) setDownloadFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	var in struct {
		Indexes  []int `json:"indexes"`
		Priority int   `json:"priority"`
	}
	if !webserver.Decode(w, r, &in, `body is {"indexes": [file index...], "priority": 0 (skip), 1 (normal), 6 (high) or 7 (maximum)}`) {
		return
	}
	if len(in.Indexes) == 0 || len(in.Indexes) > 10000 {
		webserver.Fail(w, http.StatusBadRequest, "indexes needs 1 to 10000 file indexes")
		return
	}
	switch in.Priority {
	case 0, 1, 6, 7:
	default:
		webserver.Fail(w, http.StatusBadRequest, ErrFilePriority.Error())
		return
	}
	err := a.Manager().Control(r.Context(), id, func(e Engine, ext string) error {
		return asInspector(e, func(t TorrentInspector) error {
			return t.SetFilePriority(r.Context(), ext, in.Indexes, in.Priority)
		})
	})
	if err != nil {
		clientControlError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ClientView is one Downloader with what it says about itself.
type ClientView struct {
	ID       int64         `json:"id"`
	Name     string        `json:"name"`
	Kind     string        `json:"kind"`
	Protocol string        `json:"protocol"`
	Controls bool          `json:"controls"`
	Torrent  bool          `json:"torrent_controls"`
	Status   *ClientStatus `json:"status,omitempty"`
	Error    string        `json:"error,omitempty"`
}

func (a *Web) eachClient(ctx context.Context, f func(d domain.Downloader, e Engine, v *ClientView) error) ([]ClientView, error) {
	ds, err := a.Manager().Downloaders(ctx)
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
			err := a.Manager().ControlEngine(ctx, d, func(e Engine) error {
				_, v.Controls = e.(Controller)
				_, v.Torrent = e.(TorrentController)
				return f(d, e, v)
			})
			if err != nil && !errors.Is(err, ErrNotSupported) {
				v.Error = err.Error()
			}
		}(i, d)
	}
	wg.Wait()
	return out, nil
}

// downloadClients shows each Downloader's state: paused, speeds, limits,
// totals and free space, where the client reports them.
func (a *Web) downloadClients(w http.ResponseWriter, r *http.Request) {
	out, err := a.eachClient(r.Context(), func(d domain.Downloader, e Engine, v *ClientView) error {
		return asController(e, func(c Controller) error {
			if d.Adapter == "qbittorrent" && a.DLSync != nil {
				if st, ok := a.DLSync.ClientStatus(d.ID); ok {
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
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"clients": out})
}

// allAct pauses or resumes everything in every Downloader that can.
func (a *Web) allAct(pause bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := a.eachClient(r.Context(), func(d domain.Downloader, e Engine, _ *ClientView) error {
			// A read-only Downloader is left alone, not reported as a failure of Pause all.
			if ReadOnlyFor(d) {
				return ErrNotSupported
			}
			return asController(e, func(c Controller) error {
				if pause {
					return c.PauseAll(r.Context())
				}
				return c.ResumeAll(r.Context())
			})
		})
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		a.Manager().Poke()
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"clients": out})
	}
}

// speedLimit sets one Downloader's total speed cap in bytes per second;
// 0 removes it.
func (a *Web) speedLimit(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "Downloader")
	if !ok {
		return
	}
	var in struct {
		Down int64 `json:"down"`
		Up   int64 `json:"up"`
	}
	if !webserver.Decode(w, r, &in, `body is {"down": bytes per second, "up": bytes per second}; 0 means no limit`) {
		return
	}
	if in.Down < 0 || in.Up < 0 {
		webserver.Fail(w, http.StatusBadRequest, "limits are 0 (none) or more")
		return
	}
	var d domain.Downloader
	if err := database.Get(r.Context(), a.Store.Read(), "downloaders", id, &d); err != nil || d.Removed {
		webserver.Fail(w, http.StatusNotFound, "no such Downloader")
		return
	}
	err := a.Manager().ControlEngine(r.Context(), d, func(e Engine) error {
		return asController(e, func(c Controller) error { return c.SetSpeedLimit(r.Context(), in.Down, in.Up) })
	})
	if err != nil {
		clientControlError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

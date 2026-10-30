package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
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
	m.HandleFunc("GET /api/v1/downloads/clients", a.downloadClients)
	m.HandleFunc("POST /api/v1/downloads/pause-all", a.allAct(true))
	m.HandleFunc("POST /api/v1/downloads/resume-all", a.allAct(false))
	m.HandleFunc("PUT /api/v1/downloaders/{id}/speed-limit", a.speedLimit)
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
	case errors.Is(err, downloader.ErrNotActive), errors.Is(err, downloader.ErrNotSupported):
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
	err := a.manager().Control(r.Context(), id, func(e downloader.Engine, ext string) error {
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
	writeJSON(w, http.StatusOK, out)
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
	out, err := a.eachClient(r.Context(), func(_ domain.Downloader, e downloader.Engine, v *ClientView) error {
		return asController(e, func(c downloader.Controller) error {
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
		out, err := a.eachClient(r.Context(), func(_ domain.Downloader, e downloader.Engine, _ *ClientView) error {
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

package downloads

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Usenet extras on the Downloads page: Retry for a failed job, and the
// Servers and Repair tabs (which news servers sent the job, and how the
// verify, repair, unpack and script steps went).

func (a *Web) usenetRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/retry", a.retryItem)
	m.HandleFunc("GET /api/v1/downloaders/{id}/items/{hash}/usenet", a.itemUsenet)
	m.HandleFunc("GET /api/v1/downloads/{id}/usenet", a.downloadUsenet)
}

// JobServer is one news server's share of one job, as the Downloader wrote
// it (Amount is its own size words, such as "1.2 GB").
type JobServer struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

// UsenetView is the Servers and Repair tabs of one Usenet job.
type UsenetView struct {
	Stages     []JobStage   `json:"stages"`
	JobServers []JobServer  `json:"job_servers"`
	Servers    *ServerStats `json:"servers,omitempty"`
}

// jobServers reads the servers stage: one "name=amount" line per server.
func jobServers(stages []JobStage) []JobServer {
	out := []JobServer{}
	for _, s := range stages {
		if s.Stage != "servers" {
			continue
		}
		for _, l := range s.Lines {
			i := strings.LastIndex(l, "=")
			if i <= 0 || strings.TrimSpace(l[:i]) == "" {
				continue
			}
			out = append(out, JobServer{Name: strings.TrimSpace(l[:i]), Amount: strings.TrimSpace(l[i+1:])})
		}
	}
	return out
}

// usenetRead fills a UsenetView from the engine; the server counters are
// extra (a failure there leaves them out).
func usenetRead(ctx context.Context, e Engine, id string, out *UsenetView) error {
	ui, ok := e.(UsenetInspector)
	if !ok {
		return ErrNotSupported
	}
	st, err := ui.JobStages(ctx, id)
	if err != nil {
		return err
	}
	out.Stages, out.JobServers = st, jobServers(st)
	if ss, err := ui.ServerStats(ctx); err == nil {
		out.Servers = &ss
	}
	return nil
}

// itemUsenet is a client job's Servers and Repair tabs, read live.
func (a *Web) itemUsenet(w http.ResponseWriter, r *http.Request) {
	out := UsenetView{Stages: []JobStage{}, JobServers: []JobServer{}}
	err := a.onItem(r, false, func(e Engine, id string) error { return usenetRead(r.Context(), e, id, &out) })
	if err != nil {
		itemError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// downloadUsenet is the same for a download HolyArrs sent.
func (a *Web) downloadUsenet(w http.ResponseWriter, r *http.Request) {
	id, ok := idIn(w, r, "download")
	if !ok {
		return
	}
	out := UsenetView{Stages: []JobStage{}, JobServers: []JobServer{}}
	err := a.Manager().Inspect(r.Context(), id, func(e Engine, ext string) error { return usenetRead(r.Context(), e, ext, &out) })
	if err != nil {
		clientControlError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

const retryOwn = "HolyArrs already handled this failed download: it turned the copy down and searched again"

// retryItem sends a failed Usenet job back to the Downloader's queue. The
// job may come back under a new id: its log, its Library link and its
// place in the match queue follow it. A job HolyArrs sent is not retried
// here (its failure path already searched again). On a job HolyArrs did
// not send it needs the settings permission, like every other outside
// change beyond pause, resume and queue moves.
func (a *Web) retryItem(w http.ResponseWriter, r *http.Request) {
	did, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		itemError(w, ErrNoDownloader)
		return
	}
	old := r.PathValue("hash")
	if !a.outsideHash(r.Context(), did, old) {
		webserver.Fail(w, http.StatusConflict, retryOwn)
		return
	}
	if !a.itemChangeOK(w, r) {
		return
	}
	var newID string
	err = a.onItem(r, true, func(e Engine, id string) error {
		rt, ok := e.(Retrier)
		if !ok {
			return ErrNotSupported
		}
		var err error
		newID, err = rt.Retry(r.Context(), id)
		return err
	})
	if err != nil {
		itemError(w, err)
		return
	}
	if newID == "" {
		newID = old
	}
	a.itemChangedEvent(r, did, newID, "retry", "sent back to try again")
	if err := a.rekeyItem(r.Context(), did, old, newID); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "id": newID})
}

// rekeyItem moves what HolyArrs keeps about one client item to its new id
// and logs the retry.
func (a *Web) rekeyItem(ctx context.Context, downloaderID int64, oldID, newID string) error {
	o, n := strings.ToLower(oldID), strings.ToLower(newID)
	err := a.Store.Write(ctx, func(tx *sql.Tx) error {
		if o != n {
			for _, q := range []string{
				"UPDATE torrent_log SET hash = ? WHERE downloader_id = ? AND hash = ?",
				"DELETE FROM client_links WHERE downloader_id = ? AND hash = ?",
				"UPDATE client_links SET hash = ? WHERE downloader_id = ? AND hash = ?",
				"DELETE FROM dl_match_queue WHERE downloader_id = ? AND key = ?",
				"UPDATE dl_match_queue SET key = ? WHERE downloader_id = ? AND key = ?",
			} {
				args := []any{n, downloaderID, o}
				if strings.HasPrefix(q, "DELETE") {
					args = []any{downloaderID, n}
				}
				if _, err := tx.ExecContext(ctx, q, args...); err != nil {
					return err
				}
			}
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO torrent_log (downloader_id, hash, at, kind, summary) VALUES (?, ?, ?, ?, ?)",
			downloaderID, n, time.Now().UTC().Format(database.TimeLayout), "retried", "Sent back to the queue to try again")
		return err
	})
	if err == nil && o != n {
		a.forgetTorrents(ctx, downloaderID, []string{oldID})
	}
	return err
}

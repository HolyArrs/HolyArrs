package downloads

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
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

// ClientHealth is a client's state in the Downloads view, from the
// background reader (see syncHealth).
type ClientHealth struct {
	ID     int64     `json:"id"`
	Name   string    `json:"name"`
	Health string    `json:"health"`
	Stale  bool      `json:"stale,omitempty"`
	SeenAt time.Time `json:"seen_at,omitzero"`
}

// clientRows returns the items of every client not already shown as
// HolyArrs rows, and each client's health, from the background reader's
// snapshots: no client call, no write.
func (a *Web) clientRows(ctx context.Context, names map[int64]domain.Downloader, shown map[clientKey]bool) ([]DownloadRow, []ClientHealth) {
	if a.DLSync == nil {
		return nil, []ClientHealth{}
	}
	return a.storeRows(ctx, names, shown)
}

// downloaderNames reads every Downloader (removed ones too: old downloads
// still name them) by id, in one read.
func (a *Web) downloaderNames(ctx context.Context) map[int64]domain.Downloader {
	names := map[int64]domain.Downloader{}
	if all, err := database.Select[domain.Downloader](ctx, a.read(), "downloaders", "1=1"); err == nil {
		for _, d := range all {
			names[d.ID] = d
		}
	}
	return names
}

// clientRow is one client item in the Downloads screen's words.
func clientRow(d domain.Downloader, j Job) DownloadRow {
	ph, st, active := clientPhase(j)
	if d.Protocol == "usenet" && st == "seeding" {
		st = "done" // Usenet has no sharing after the bytes are in
	}
	row := DownloadRow{
		DownloadView: DownloadView{State: st, Label: PhaseLabels[ph], Progress: j.Progress, Speed: j.Speed, UpSpeed: j.UpSpeed, Seeds: j.Seeds, Peers: j.Peers, SeedsTotal: j.SeedsTotal, PeersTotal: j.PeersTotal, Active: active},
		Phase:        ph, Name: j.Name, Downloader: d.Name, DownloaderKind: d.Adapter, Protocol: d.Protocol, Size: j.Size,
		UpdatedAt: time.Now().UTC(), CanAct: !ReadOnlyFor(d), ClientLocked: ReadOnlyFor(d), ClientItem: true, Outside: true, DownloaderID: d.ID, Hash: j.ID,
		Category: j.Category, Ratio: &j.Ratio, Limits: j.Limits, ClientHint: clientHint(j.Detail),
		QueuePos: j.QueuePos, Tracker: j.Tracker, TrackerProblem: j.TrackerProblem,
		LimitDown: j.LimitDown, LimitUp: j.LimitUp, LiveKey: LiveKey(d.ID, j.ID),
	}
	row.Path = j.ContentPath
	if row.Path == "" {
		row.Path = j.SavePath
	}
	if j.Progress < 1 {
		row.Availability = j.Availability
	}
	if j.DoneAt > 0 {
		t := time.Unix(j.DoneAt, 0).UTC()
		row.FinishedAt = &t
	}
	if j.ETA > 0 && active {
		eta := j.ETA
		row.ETA = &eta
	}
	if j.AddedAt > 0 {
		t := time.Unix(j.AddedAt, 0).UTC()
		row.StartedAt = &t
	}
	if j.PostedAt > 0 && d.Protocol == "usenet" {
		t := time.Unix(j.PostedAt, 0).UTC()
		row.PostedAt = &t
	}
	if ph == "done" {
		row.Label = "Done"
	}
	if j.State == StateFailed && j.Detail != "" && row.ClientHint == "" {
		row.Text = j.Detail // the Downloader's own reason (SABnzbd's fail message)
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
	// SABnzbd's steps after the bytes are in (the row reads Importing).
	case "QuickCheck", "Verifying":
		return "verifying"
	case "Repairing":
		return "repairing"
	case "Extracting":
		return "unpacking"
	case "Moving":
		return "finishing"
	case "Running":
		return "script"
	// NZBGet's.
	case "LOADING_PARS", "VERIFYING_SOURCES", "VERIFYING_REPAIRED":
		return "verifying"
	case "REPAIRING":
		return "repairing"
	case "UNPACKING":
		return "unpacking"
	case "MOVING", "RENAMING", "POST_UNPACK_RENAMING", "PP_FINISHED":
		return "finishing"
	case "EXECUTING_SCRIPT":
		return "script"
	}
	return ""
}

// clientPhase maps a client item onto the Downloads screen's phase and
// state: a finished torrent still shared is done and seeding.
func clientPhase(j Job) (phase, state string, active bool) {
	switch j.State {
	case StateQueued:
		return "waiting", "queued", true
	case StatePaused:
		return "paused", "paused", true
	case StateFailed:
		return "failed", "failed", false
	case StateFinished:
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
func (a *Web) knownTitles(ctx context.Context, downloaderID int64, rows []DownloadRow) {
	at := map[string]int{}
	for i, r := range rows {
		at[r.Hash] = i
	}
	hashes := make([]string, 0, len(rows))
	for h := range at {
		hashes = append(hashes, h)
	}
	db := a.read()
	for len(hashes) > 0 {
		n := min(len(hashes), 400)
		part := hashes[:n]
		hashes = hashes[n:]
		args := []any{downloaderID}
		for _, h := range part {
			args = append(args, h)
		}
		ds, err := database.Select[domain.Download](ctx, db, "downloads", "downloader_id = ? AND lower(external_id) IN (?"+strings.Repeat(", ?", len(part)-1)+")", args...)
		if err != nil {
			continue
		}
		first, relIDs := []int64{}, []int64{}
		for _, d := range ds {
			if len(d.SlotIDs) > 0 {
				first = append(first, d.SlotIDs[0])
			}
			if d.ReleaseID != nil {
				relIDs = append(relIDs, *d.ReleaseID)
			}
		}
		slots := slotInfos(ctx, db, first)
		rels := releaseNames(ctx, db, relIDs)
		for _, d := range ds {
			i, ok := at[strings.ToLower(d.ExternalID)]
			if !ok {
				continue
			}
			row := &rows[i]
			row.Outside = false
			if d.ReleaseID != nil {
				if rl, ok := rels[*d.ReleaseID]; ok {
					row.SourceName, row.SourceID = rl.source, rl.sourceID
					if rl.posted != nil && row.Protocol == "usenet" {
						row.PostedAt = rl.posted
					}
				}
			}
			if len(d.SlotIDs) == 0 {
				continue
			}
			if si, ok := slots[d.SlotIDs[0]]; ok && si.full {
				row.TitleID, row.Title, row.Kind, row.Version = si.titleID, si.title, si.kind, si.version
			}
		}
	}
}

// Client item routes: the client controls, addressed by Downloader id and
// the client's own item id (a torrent's info hash).
func (a *Web) clientItemRoutes(m *http.ServeMux) {
	write := func(action, verb string, f func(ctx context.Context, e Engine, id string) error) http.HandlerFunc {
		return a.itemAct(true, func(r *http.Request, e Engine, id string) error {
			err := f(r.Context(), e, id)
			if err == nil {
				a.itemChanged(r, id, action, verb)
			}
			return err
		})
	}
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/pause", write("pause", "paused", func(ctx context.Context, e Engine, id string) error {
		return asController(e, func(c Controller) error { return c.PauseItem(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/resume", write("resume", "resumed", func(ctx context.Context, e Engine, id string) error {
		return asController(e, func(c Controller) error { return c.ResumeItem(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/top", write("top", "moved to the top", func(ctx context.Context, e Engine, id string) error {
		return asController(e, func(c Controller) error { return c.MoveTop(ctx, id) })
	}))
	a.usenetQueueRoutes(m)
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/recheck", write("recheck", "rechecked", func(ctx context.Context, e Engine, id string) error {
		return asTorrent(e, func(t TorrentController) error { return t.Recheck(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/reannounce", write("reannounce", "reannounced", func(ctx context.Context, e Engine, id string) error {
		return asTorrent(e, func(t TorrentController) error { return t.Reannounce(ctx, id) })
	}))
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/remove", a.removeItem)
	m.HandleFunc("GET /api/v1/downloaders/{id}/items/{hash}/detail", a.itemDetail)
	m.HandleFunc("GET /api/v1/downloaders/{id}/items/{hash}/swarm", a.itemSwarm)
	m.HandleFunc("GET /api/v1/downloaders/{id}/items/{hash}/files", a.itemFiles)
	m.HandleFunc("PUT /api/v1/downloaders/{id}/items/{hash}/files", a.setItemFiles)
}

// itemAct runs a call on one client item and answers ok.
func (a *Web) itemAct(write bool, f func(r *http.Request, e Engine, id string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.onItem(r, write, func(e Engine, id string) error { return f(r, e, id) }); err != nil {
			itemError(w, err)
			return
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func (a *Web) onItem(r *http.Request, write bool, call func(e Engine, id string) error) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return ErrNoDownloader
	}
	err = a.Manager().ControlItem(r.Context(), id, r.PathValue("hash"), write, call)
	if err == nil && write {
		a.Manager().Kick(id) // the reader shows the change at once
	}
	return err
}

func itemError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNoDownloader) {
		webserver.Fail(w, http.StatusNotFound, err.Error())
		return
	}
	clientControlError(w, err)
}

// removeItem takes one item out of the client, with or without its files.
func (a *Web) removeItem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DeleteFiles bool `json:"delete_files"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &in, `body is {"delete_files": true or false}`) {
		return
	}
	// Removing a torrent HolyArrs did not send is a Downloader owner's call:
	// it needs the settings permission (and an admin-scoped key).
	if !a.itemChangeOK(w, r) {
		return
	}
	a.itemAct(true, func(r *http.Request, e Engine, id string) error {
		err := e.Remove(r.Context(), id, in.DeleteFiles)
		if err == nil {
			if did, perr := strconv.ParseInt(r.PathValue("id"), 10, 64); perr == nil {
				a.forgetTorrents(r.Context(), did, []string{r.PathValue("hash")})
				a.itemRemovedEvent(r, did, id, in.DeleteFiles)
			}
		}
		return err
	})(w, r)
}

// itemDetail is a client item's Info, read live; it has no HolyArrs log.
func (a *Web) itemDetail(w http.ResponseWriter, r *http.Request) {
	out := DownloadDetailView{Log: []DownloadLogLine{}}
	err := a.onItem(r, false, func(e Engine, id string) error {
		return asInspector(e, func(t TorrentInspector) error {
			p, err := t.Properties(r.Context(), id)
			if err == nil {
				out.Torrent = &p
			}
			return err
		})
	})
	if errors.Is(err, ErrNoDownloader) || errors.Is(err, ErrNotFound) {
		itemError(w, err)
		return
	}
	if err != nil && !errors.Is(err, ErrNotSupported) {
		out.TorrentErr = err.Error()
	}
	if id, perr := strconv.ParseInt(r.PathValue("id"), 10, 64); perr == nil {
		db := a.Store.Read()
		var adapter string
		_ = db.QueryRowContext(r.Context(), "SELECT name, adapter FROM downloaders WHERE id = ?", id).Scan(&out.Downloader, &adapter)
		if a.outsideHash(r.Context(), id, r.PathValue("hash")) {
			// A torrent HolyArrs did not send: nobody asked for it here. When
			// a person added its Library title, that person is named.
			var titleID int64
			_ = db.QueryRowContext(r.Context(), "SELECT title_id FROM client_links WHERE downloader_id = ? AND hash = ? AND state = 'linked'",
				id, strings.ToLower(r.PathValue("hash"))).Scan(&titleID)
			if out.RequestedBy = a.titleOwner(r.Context(), titleID); out.RequestedBy == "" {
				out.RequestedBy = "Added in " + adapterWord(adapter)
			}
		}
		out.Log = a.TorrentTimeline(r.Context(), id, r.PathValue("hash"), out.Torrent, nil)
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// titleOwner names the person who added a Library title, or "" when
// HolyArrs added it on its own (an import or a match).
func (a *Web) titleOwner(ctx context.Context, titleID int64) string {
	if titleID == 0 {
		return ""
	}
	var name string
	_ = a.Store.Read().QueryRowContext(ctx, `SELECT u.name FROM titles t JOIN users u ON u.id = json_extract(t.created_by, '$.id')
		WHERE t.id = ? AND json_extract(t.created_by, '$.kind') = 'user'`, titleID).Scan(&name)
	return name
}

// rowOwners fills each row's Requested by from its Library title, in one
// read: the person who added the title, else "Added in <client>" for an
// outside item (nobody asked HolyArrs for it).
func (a *Web) rowOwners(ctx context.Context, rows []DownloadRow) {
	ids := []int64{}
	for _, r := range rows {
		if r.TitleID != 0 {
			ids = append(ids, r.TitleID)
		}
	}
	if len(ids) == 0 {
		return
	}
	who := map[int64]string{}
	inChunks(ids, func(part []any, marks string) {
		rs, err := a.read().QueryContext(ctx, `SELECT t.id, u.name FROM titles t JOIN users u ON u.id = json_extract(t.created_by, '$.id')
			WHERE t.id IN (`+marks+`) AND json_extract(t.created_by, '$.kind') = 'user'`, part...)
		if err != nil {
			return
		}
		defer rs.Close()
		for rs.Next() {
			var id int64
			var name string
			if rs.Scan(&id, &name) == nil {
				who[id] = name
			}
		}
	})
	for i := range rows {
		r := &rows[i]
		if r.TitleID == 0 {
			continue
		}
		if r.RequestedBy = who[r.TitleID]; r.RequestedBy == "" && r.Outside {
			r.RequestedBy = "Added in " + adapterWord(r.DownloaderKind)
		}
	}
}

// adapterWord is a torrent client's own name for plain sentences.
func adapterWord(adapter string) string {
	switch adapter {
	case "qbittorrent":
		return "qBittorrent"
	case "transmission":
		return "Transmission"
	case "deluge":
		return "Deluge"
	case "rtorrent":
		return "rTorrent"
	case "sabnzbd":
		return "SABnzbd"
	case "nzbget":
		return "NZBGet"
	}
	return "the Downloader"
}

// itemSwarm lists a client item's trackers and peers.
func (a *Web) itemSwarm(w http.ResponseWriter, r *http.Request) {
	out := struct {
		Trackers []Tracker `json:"trackers"`
		Peers    []Peer    `json:"peers"`
	}{}
	err := a.onItem(r, false, func(e Engine, id string) error {
		return asTorrent(e, func(t TorrentController) error {
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
	if id, perr := strconv.ParseInt(r.PathValue("id"), 10, 64); perr == nil {
		a.noteTrackers(r.Context(), id, r.PathValue("hash"), out.Trackers)
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

// itemFiles lists a client item's files with their priority.
func (a *Web) itemFiles(w http.ResponseWriter, r *http.Request) {
	var files []TorrentFile
	err := a.onItem(r, false, func(e Engine, id string) error {
		return asInspector(e, func(t TorrentInspector) error {
			var err error
			files, err = t.TorrentFiles(r.Context(), id)
			return err
		})
	})
	if err != nil {
		itemError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"files": files})
}

// setItemFiles sets which files of a client item to fetch and how soon.
func (a *Web) setItemFiles(w http.ResponseWriter, r *http.Request) {
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
	if !a.itemChangeOK(w, r) {
		return
	}
	a.itemAct(true, func(r *http.Request, e Engine, id string) error {
		return asInspector(e, func(t TorrentInspector) error {
			return t.SetFilePriority(r.Context(), id, in.Indexes, in.Priority)
		})
	})(w, r)
}

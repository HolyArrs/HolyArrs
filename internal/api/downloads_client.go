package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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

// clientListWait caps one list read of a client (it runs in the
// background, apart from any request).
var clientListWait = 5 * time.Second

// clientViewWait is how long the Downloads view waits for fresh lists
// before it answers with what it has.
var clientViewWait = 200 * time.Millisecond

// clientStaleFor is how long a client's last good list is still shown
// while the client does not answer.
const clientStaleFor = 5 * time.Minute

// ClientHealth says how a torrent client answered the Downloads view:
// ok (a fresh list), slow (no answer yet, its last list shown), offline
// (its last read failed) or waiting (no list yet).
type ClientHealth struct {
	ID     int64     `json:"id"`
	Name   string    `json:"name"`
	Health string    `json:"health"`
	Stale  bool      `json:"stale,omitempty"`
	SeenAt time.Time `json:"seen_at,omitzero"`
}

// clientLists holds the last list read of each torrent client and the read
// in flight, so slow clients are read once at a time and never in a request.
type clientLists struct {
	mu sync.Mutex
	by map[int64]*clientList
}

type clientList struct {
	jobs   []downloader.Job
	at     time.Time     // last good read
	failed bool          // the last read failed
	busy   chan struct{} // closed when the read in flight ends; nil when none
}

// readList starts a list read of d unless one is in flight, and returns a
// channel closed when that read ends; started says this call began it.
func (a *API) readList(d domain.Downloader) (done <-chan struct{}, started bool) {
	c := &a.lists
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.by == nil {
		c.by = map[int64]*clientList{}
	}
	l := c.by[d.ID]
	if l == nil {
		l = &clientList{}
		c.by[d.ID] = l
	}
	if l.busy != nil {
		return l.busy, false
	}
	busy := make(chan struct{})
	l.busy = busy
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), clientListWait)
		defer cancel()
		var jobs []downloader.Job
		err := a.manager().ControlEngine(ctx, d, func(e downloader.Engine) error {
			al, ok := e.(downloader.AllLister)
			if !ok {
				return downloader.ErrNotSupported
			}
			var err error
			jobs, err = al.ListAll(ctx)
			return err
		})
		if errors.Is(err, downloader.ErrNotSupported) {
			// No outside list for this engine: that is not "not answering".
			err, jobs = nil, nil
		}
		if err == nil {
			a.noteJobs(ctx, d.ID, jobs)
		}
		c.mu.Lock()
		if err == nil {
			l.jobs, l.at, l.failed = jobs, time.Now(), false
		} else {
			l.failed = true
		}
		l.busy = nil
		c.mu.Unlock()
		close(busy)
	}()
	return busy, true
}

// clientRows returns the items of every torrent Downloader not already shown
// as HolyArrs rows, in a stable order (Downloader priority, then newest
// first), and each client's health. Lists are read side by side in the
// background; the caller waits at most wait for them.
func (a *API) clientRows(ctx context.Context, names map[int64]domain.Downloader, shown map[clientKey]bool, wait time.Duration) ([]DownloadRow, []ClientHealth) {
	if a.syncOn() {
		return a.storeRows(ctx, names, shown)
	}
	health := []ClientHealth{}
	var ds []domain.Downloader
	for _, d := range names {
		if d.Protocol == "torrent" && !d.Removed {
			ds = append(ds, d)
		}
	}
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].Priority != ds[j].Priority {
			return ds[i].Priority < ds[j].Priority
		}
		if ds[i].Name != ds[j].Name {
			return ds[i].Name < ds[j].Name
		}
		return ds[i].ID < ds[j].ID
	})
	// Only reads this call started are waited for: a read still running
	// from an earlier view means the client is slow, and the view answers
	// at once with what it has.
	var waits []<-chan struct{}
	for _, d := range ds {
		if w, started := a.readList(d); started {
			waits = append(waits, w)
		}
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
wait:
	for _, w := range waits {
		select {
		case <-w:
		case <-timer.C:
			break wait
		case <-ctx.Done():
			break wait
		}
	}
	var failed []int64
	var rows []DownloadRow
	now := time.Now()
	for _, d := range ds {
		a.lists.mu.Lock()
		l := a.lists.by[d.ID]
		jobs, at, bad, busy := l.jobs, l.at, l.failed, l.busy != nil
		a.lists.mu.Unlock()
		h := ClientHealth{ID: d.ID, Name: d.Name, Health: "ok", SeenAt: at}
		fresh := !busy && !bad && !at.IsZero()
		switch {
		case fresh:
		case busy && !at.IsZero():
			h.Health, h.Stale = "slow", true
		case busy:
			h.Health = "waiting"
		default:
			h.Health, h.Stale = "offline", !at.IsZero()
		}
		if !fresh {
			failed = append(failed, d.ID)
		}
		if at.IsZero() || now.Sub(at) > clientStaleFor {
			h.Stale = false
			jobs = nil
		}
		health = append(health, h)
		out := make([]DownloadRow, 0, len(jobs))
		for _, j := range jobs {
			if j.ID == "" || shown[clientKey{d.ID, strings.ToLower(j.ID)}] {
				continue
			}
			out = append(out, clientRow(d, j))
		}
		sort.SliceStable(out, func(i, j int) bool {
			ai, aj := out[i].StartedAt, out[j].StartedAt
			switch {
			case ai == nil || aj == nil:
				if (ai == nil) != (aj == nil) {
					return aj == nil
				}
			case !ai.Equal(*aj):
				return ai.After(*aj)
			}
			return out[i].Hash < out[j].Hash
		})
		if len(out) > 0 {
			a.knownTitles(ctx, d.ID, out)
			a.linkedTitles(ctx, d.ID, out)
			rows = append(rows, out...)
		}
	}
	a.maybeSweep(ctx, failed)
	a.matchQueue(context.WithoutCancel(ctx), rows)
	return rows, health
}

// downloaderNames reads every Downloader (removed ones too: old downloads
// still name them) by id, in one read.
func (a *API) downloaderNames(ctx context.Context) map[int64]domain.Downloader {
	names := map[int64]domain.Downloader{}
	if all, err := store.Select[domain.Downloader](ctx, a.read(), "downloaders", "1=1"); err == nil {
		for _, d := range all {
			names[d.ID] = d
		}
	}
	return names
}

// clientRow is one client item in the Downloads screen's words.
func clientRow(d domain.Downloader, j downloader.Job) DownloadRow {
	ph, st, active := clientPhase(j)
	if d.Protocol == "usenet" && st == "seeding" {
		st = "done" // Usenet has no sharing after the bytes are in
	}
	row := DownloadRow{
		DownloadView: DownloadView{State: st, Label: PhaseLabels[ph], Progress: j.Progress, Speed: j.Speed, UpSpeed: j.UpSpeed, Seeds: j.Seeds, Peers: j.Peers, SeedsTotal: j.SeedsTotal, PeersTotal: j.PeersTotal, Active: active},
		Phase:        ph, Name: j.Name, Downloader: d.Name, DownloaderKind: d.Adapter, Protocol: d.Protocol, Size: j.Size,
		UpdatedAt: time.Now().UTC(), CanAct: !downloader.ReadOnlyFor(d), ClientItem: true, Outside: true, DownloaderID: d.ID, Hash: j.ID,
		Category: j.Category, Ratio: &j.Ratio, Limits: j.Limits, ClientHint: clientHint(j.Detail),
		QueuePos: j.QueuePos, Tracker: j.Tracker, TrackerProblem: j.TrackerProblem,
		LimitDown: j.LimitDown, LimitUp: j.LimitUp, LiveKey: liveKey(d.ID, j.ID),
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
	if ph == "done" {
		row.Label = "Done"
	}
	if j.State == downloader.StateFailed && j.Detail != "" && row.ClientHint == "" {
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
	db := a.read()
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
		first := []int64{}
		for _, d := range ds {
			if len(d.SlotIDs) > 0 {
				first = append(first, d.SlotIDs[0])
			}
		}
		slots := slotInfos(ctx, db, first)
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
			if si, ok := slots[d.SlotIDs[0]]; ok && si.full {
				row.TitleID, row.Title, row.Kind, row.Version = si.titleID, si.title, si.kind, si.version
			}
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
	err = a.manager().ControlItem(r.Context(), id, r.PathValue("hash"), write, call)
	if err == nil && write {
		a.manager().Kick(id) // the reader shows the change at once
	}
	return err
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
	if !a.itemChangeOK(w, r) {
		return
	}
	a.itemAct(true, func(r *http.Request, e downloader.Engine, id string) error {
		err := e.Remove(r.Context(), id, in.DeleteFiles)
		if err == nil {
			if did, perr := strconv.ParseInt(r.PathValue("id"), 10, 64); perr == nil {
				a.forgetTorrents(r.Context(), did, []string{r.PathValue("hash")})
			}
		}
		return err
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
		db := a.s.Store.Read()
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
		out.Log = a.torrentTimeline(r.Context(), id, r.PathValue("hash"), out.Torrent, nil)
	}
	writeJSON(w, http.StatusOK, out)
}

// titleOwner names the person who added a Library title, or "" when
// HolyArrs added it on its own (an import or a match).
func (a *API) titleOwner(ctx context.Context, titleID int64) string {
	if titleID == 0 {
		return ""
	}
	var name string
	_ = a.s.Store.Read().QueryRowContext(ctx, `SELECT u.name FROM titles t JOIN users u ON u.id = json_extract(t.created_by, '$.id')
		WHERE t.id = ? AND json_extract(t.created_by, '$.kind') = 'user'`, titleID).Scan(&name)
	return name
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
	}
	return "the Downloader"
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
	if id, perr := strconv.ParseInt(r.PathValue("id"), 10, 64); perr == nil {
		a.noteTrackers(r.Context(), id, r.PathValue("hash"), out.Trackers)
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
	if !a.itemChangeOK(w, r) {
		return
	}
	a.itemAct(true, func(r *http.Request, e downloader.Engine, id string) error {
		return asInspector(e, func(t downloader.TorrentInspector) error {
			return t.SetFilePriority(r.Context(), id, in.Indexes, in.Priority)
		})
	})(w, r)
}

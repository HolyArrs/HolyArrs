package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Hand steering of a torrent Downloader from the Downloads screen, the way
// the client's own screen does it: adding torrents by link or file. What
// these routes add or change is the client's, never a HolyArrs download.
func (a *API) clientManageRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/downloaders/{id}/torrents", a.addTorrents)
	m.HandleFunc("POST /api/v1/downloaders/{id}/bulk", a.bulkItems)
	m.HandleFunc("POST /api/v1/downloaders/{id}/share-limits", a.shareLimits)
	m.HandleFunc("POST /api/v1/downloaders/{id}/slow-mode", a.slowMode)
}

// addTorrentsMax caps one add: qBittorrent itself takes 64 MiB a request.
const addTorrentsMax = 64 << 20

// addTorrents adds magnet or http links and uploaded .torrent files to one
// torrent Downloader. Multipart: "links" (one per line), "category",
// "save_path", "paused" and any number of "torrents" files; or JSON with
// links, category, save_path and paused.
func (a *API) addTorrents(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, downloader.ErrNoDownloader.Error())
		return
	}
	var b downloader.AddBatch
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, addTorrentsMax)
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			if mb := (*http.MaxBytesError)(nil); errors.As(err, &mb) {
				fail(w, http.StatusRequestEntityTooLarge, "the files are too large together (64 MB at most)")
				return
			}
			fail(w, http.StatusBadRequest, "could not read the upload")
			return
		}
		b.Links = strings.Split(r.FormValue("links"), "\n")
		b.Category = r.FormValue("category")
		b.SavePath = r.FormValue("save_path")
		b.Paused = r.FormValue("paused") == "true"
		for _, fh := range r.MultipartForm.File["torrents"] {
			f, err := fh.Open()
			if err != nil {
				fail(w, http.StatusBadRequest, "could not read "+fh.Filename)
				return
			}
			data, err := io.ReadAll(io.LimitReader(f, addTorrentsMax))
			f.Close()
			if err != nil {
				fail(w, http.StatusBadRequest, "could not read "+fh.Filename)
				return
			}
			b.Files = append(b.Files, downloader.TorrentUpload{Name: fh.Filename, Data: data})
		}
	} else {
		var in struct {
			Links    []string `json:"links"`
			Category string   `json:"category"`
			SavePath string   `json:"save_path"`
			Paused   bool     `json:"paused"`
		}
		if !decode(w, r, &in, `body is {"links": [magnet or http link...], "category": "", "save_path": "", "paused": false}, or a multipart upload with torrents files`) {
			return
		}
		b = downloader.AddBatch{Links: in.Links, Category: in.Category, SavePath: in.SavePath, Paused: in.Paused}
	}
	if len(b.Links)+len(b.Files) > 500 {
		fail(w, http.StatusBadRequest, "add 500 torrents at most at once")
		return
	}
	var dr domain.Downloader
	if err := store.Get(r.Context(), a.s.Store.Read(), "downloaders", id, &dr); err != nil || dr.Removed {
		fail(w, http.StatusNotFound, downloader.ErrNoDownloader.Error())
		return
	}
	// Lines split on LF only; a CR or any other control character left in a
	// line refuses the add.
	links := []string{}
	for _, l := range b.Links {
		l = strings.Trim(l, " 	")
		if l == "" {
			continue
		}
		if err := checkLink(l); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		links = append(links, l)
	}
	b.Links = links
	b.AllowLocal = dr.AllowLocal
	b.Category, b.SavePath, err = a.clientFolder(r.Context(), id, b.Category, b.SavePath, true)
	if folderError(w, err) {
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	var out downloader.AddOutcome
	err = a.manager().ControlTorrentClient(r.Context(), id, true, func(e downloader.Engine) error {
		t, ok := e.(downloader.TorrentAdder)
		if !ok {
			return downloader.ErrNotSupported
		}
		var err error
		out, err = t.AddTorrents(r.Context(), b)
		return err
	})
	if errors.Is(err, downloader.ErrNothingToAdd) {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// validHash says s is a 40-character hex info hash.
func validHash(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// bulkIn is one action on many torrents of one Downloader.
type bulkIn struct {
	Action      string   `json:"action"`
	Hashes      []string `json:"hashes"`
	DeleteFiles bool     `json:"delete_files"`
}

const bulkHelp = `body is {"action": "start", "pause", "recheck", "reannounce", "top", "bottom", "up", "down", "force", "unforce" or "remove", "hashes": [info hash...], "delete_files": false}`

// bulkItems runs one action on many torrents of one Downloader, in one call
// to the client. Removing a torrent HolyArrs did not send needs the settings
// permission, as for one torrent.
func (a *API) bulkItems(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, downloader.ErrNoDownloader.Error())
		return
	}
	var in bulkIn
	if !decode(w, r, &in, bulkHelp) {
		return
	}
	if !a.bulkHashesOK(w, in.Hashes) {
		return
	}
	known := in.Action == "remove"
	for _, k := range downloader.BulkActions {
		known = known || k == in.Action
	}
	if !known {
		fail(w, http.StatusBadRequest, bulkHelp)
		return
	}
	if in.Action == "remove" && !a.mayRemoveOutside(r.Context(), id, in.Hashes) {
		fail(w, http.StatusForbidden, "removing a torrent HolyArrs did not send needs the settings permission")
		return
	}
	err = a.onBulk(r, id, func(t downloader.TorrentBulk) error {
		if in.Action == "remove" {
			return t.RemoveMany(r.Context(), in.Hashes, in.DeleteFiles)
		}
		return t.Bulk(r.Context(), in.Action, in.Hashes)
	})
	if errors.Is(err, downloader.ErrQueueOff) {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(in.Hashes)})
}

// bulkHashesOK checks a selection: 1 to 1000 info hashes.
func (a *API) bulkHashesOK(w http.ResponseWriter, hashes []string) bool {
	if len(hashes) == 0 || len(hashes) > 1000 {
		fail(w, http.StatusBadRequest, "hashes needs 1 to 1000 info hashes")
		return false
	}
	for _, h := range hashes {
		if !validHash(strings.ToLower(h)) {
			fail(w, http.StatusBadRequest, "hashes are 40-character info hashes")
			return false
		}
	}
	return true
}

// mayRemoveOutside says the person may remove these torrents: any of them
// that HolyArrs did not send needs the settings permission and an
// admin-scoped key.
func (a *API) mayRemoveOutside(ctx context.Context, downloaderID int64, hashes []string) bool {
	who, ok := auth.From(ctx)
	if !ok || (who.Has(auth.PermSettings) && who.HasScope(auth.ScopeAdmin)) {
		return true
	}
	for _, h := range hashes {
		if a.outsideHash(ctx, downloaderID, h) {
			return false
		}
	}
	return true
}

// onBulk runs a call on a torrent Downloader that acts on many torrents.
func (a *API) onBulk(r *http.Request, id int64, call func(t downloader.TorrentBulk) error) error {
	return a.manager().ControlTorrentClient(r.Context(), id, true, func(e downloader.Engine) error {
		t, ok := e.(downloader.TorrentBulk)
		if !ok {
			return downloader.ErrNotSupported
		}
		return call(t)
	})
}

const shareHelp = `body is {"hashes": [info hash...], "ratio": -2 (Downloader's rule), -1 (none) or 0 to 9998, "seeding_minutes": -2, -1 or 0 to 525600, "inactive_minutes": -2, -1 or 0 to 525600, "action": "", "stop", "remove", "remove_files" or "super_seed"}`

// shareLimits sets seeding rules on many torrents of one Downloader. On
// torrents HolyArrs did not send, rules that may end in a remove need the
// settings permission, as removing them does.
func (a *API) shareLimits(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, downloader.ErrNoDownloader.Error())
		return
	}
	var in struct {
		Hashes []string `json:"hashes"`
		downloader.ShareLimits
	}
	if !decode(w, r, &in, shareHelp) || !a.bulkHashesOK(w, in.Hashes) {
		return
	}
	l := in.ShareLimits
	minutesOK := func(n int64) bool { return n == -2 || n == -1 || n >= 0 && n <= 525600 }
	ratioOK := l.Ratio == -2 || l.Ratio == -1 || l.Ratio >= 0 && l.Ratio <= 9998
	switch l.Action {
	case "", "stop", "remove", "remove_files", "super_seed":
	default:
		ratioOK = false
	}
	if !ratioOK || !minutesOK(l.SeedingMinutes) || !minutesOK(l.InactiveMinutes) {
		fail(w, http.StatusBadRequest, shareHelp)
		return
	}
	// Any limit could trigger a remove: the torrent's own action, or the
	// client's global one when the action is left on the default (and on
	// qBittorrent before 5.2, which has no per-torrent action). Only an
	// explicit stop or super seed (which those clients refuse) or clearing
	// every limit needs no more than managing the Library.
	safe := l.Action == "stop" || l.Action == "super_seed" || l.Ratio == -1 && l.SeedingMinutes == -1 && l.InactiveMinutes == -1
	if !safe && !a.mayRemoveOutside(r.Context(), id, in.Hashes) {
		fail(w, http.StatusForbidden, "removing a torrent HolyArrs did not send needs the settings permission")
		return
	}
	err = a.manager().ControlTorrentClient(r.Context(), id, true, func(e downloader.Engine) error {
		t, ok := e.(downloader.ShareLimiter)
		if !ok {
			return downloader.ErrNotSupported
		}
		return t.SetShareLimits(r.Context(), in.Hashes, l)
	})
	if errors.Is(err, downloader.ErrShareAction) {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(in.Hashes)})
}

// slowMode turns a torrent Downloader's slower second set of speed limits
// on or off.
func (a *API) slowMode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusNotFound, downloader.ErrNoDownloader.Error())
		return
	}
	var in struct {
		On *bool `json:"on"`
	}
	if !decode(w, r, &in, `body is {"on": true or false}`) {
		return
	}
	if in.On == nil {
		fail(w, http.StatusBadRequest, `body is {"on": true or false}`)
		return
	}
	err = a.manager().ControlTorrentClient(r.Context(), id, true, func(e downloader.Engine) error {
		t, ok := e.(downloader.SlowModer)
		if !ok {
			return downloader.ErrNotSupported
		}
		return t.SetSlowMode(r.Context(), *in.On)
	})
	if err != nil {
		itemError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slow_mode": *in.On})
}

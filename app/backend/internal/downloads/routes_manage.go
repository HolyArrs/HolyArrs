package downloads

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Hand steering of a torrent Downloader from the Downloads screen, the way
// the client's own screen does it: adding torrents by link or file. What
// these routes add or change is the client's, never a HolyArrs download.
func (a *Web) clientManageRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/downloaders/{id}/torrents", a.addTorrents)
	m.HandleFunc("POST /api/v1/downloaders/{id}/bulk", a.bulkItems)
	m.HandleFunc("POST /api/v1/downloaders/{id}/share-limits", a.ShareLimits)
	m.HandleFunc("POST /api/v1/downloaders/{id}/slow-mode", a.slowMode)
	m.HandleFunc("POST /api/v1/downloaders/{id}/location", a.SetLocation)
	m.HandleFunc("POST /api/v1/downloaders/{id}/speed-limits", a.torrentLimits)
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/trackers", a.addTrackers)
	m.HandleFunc("PUT /api/v1/downloaders/{id}/items/{hash}/trackers", a.editTracker)
	m.HandleFunc("POST /api/v1/downloaders/{id}/items/{hash}/trackers/remove", a.removeTrackers)
}

// addTorrentsMax caps one add: qBittorrent itself takes 64 MiB a request.
const addTorrentsMax = 64 << 20

// addTorrents adds magnet or http links and uploaded .torrent files to one
// torrent Downloader. Multipart: "links" (one per line), "category",
// "save_path", "paused" and any number of "torrents" files; or JSON with
// links, category, save_path and paused.
func (a *Web) addTorrents(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, ErrNoDownloader.Error())
		return
	}
	var b AddBatch
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, addTorrentsMax)
		if err := r.ParseMultipartForm(16 << 20); err != nil {
			if mb := (*http.MaxBytesError)(nil); errors.As(err, &mb) {
				webserver.Fail(w, http.StatusRequestEntityTooLarge, "the files are too large together (64 MB at most)")
				return
			}
			webserver.Fail(w, http.StatusBadRequest, "could not read the upload")
			return
		}
		b.Links = strings.Split(r.FormValue("links"), "\n")
		b.Category = r.FormValue("category")
		b.SavePath = r.FormValue("save_path")
		b.Paused = r.FormValue("paused") == "true"
		for _, fh := range r.MultipartForm.File["torrents"] {
			f, err := fh.Open()
			if err != nil {
				webserver.Fail(w, http.StatusBadRequest, "could not read "+fh.Filename)
				return
			}
			data, err := io.ReadAll(io.LimitReader(f, addTorrentsMax))
			f.Close()
			if err != nil {
				webserver.Fail(w, http.StatusBadRequest, "could not read "+fh.Filename)
				return
			}
			b.Files = append(b.Files, TorrentUpload{Name: fh.Filename, Data: data})
		}
	} else {
		var in struct {
			Links    []string `json:"links"`
			Category string   `json:"category"`
			SavePath string   `json:"save_path"`
			Paused   bool     `json:"paused"`
		}
		if !webserver.Decode(w, r, &in, `body is {"links": [magnet or http link...], "category": "", "save_path": "", "paused": false}, or a multipart upload with torrents files`) {
			return
		}
		b = AddBatch{Links: in.Links, Category: in.Category, SavePath: in.SavePath, Paused: in.Paused}
	}
	if len(b.Links)+len(b.Files) > 500 {
		webserver.Fail(w, http.StatusBadRequest, "add 500 torrents at most at once")
		return
	}
	var dr domain.Downloader
	if err := database.Get(r.Context(), a.Store.Read(), "downloaders", id, &dr); err != nil || dr.Removed {
		webserver.Fail(w, http.StatusNotFound, ErrNoDownloader.Error())
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
			webserver.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		links = append(links, l)
	}
	b.Links = links
	b.AllowLocal = dr.AllowLocal
	b.Category, b.SavePath, err = a.ClientFolder(r.Context(), id, b.Category, b.SavePath, true)
	if folderError(w, err) {
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	var out AddOutcome
	err = a.Manager().ControlTorrentClient(r.Context(), id, true, func(e Engine) error {
		t, ok := e.(TorrentAdder)
		if !ok {
			return ErrNotSupported
		}
		var err error
		out, err = t.AddTorrents(r.Context(), b)
		return err
	})
	if errors.Is(err, ErrNothingToAdd) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
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
func (a *Web) bulkItems(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, ErrNoDownloader.Error())
		return
	}
	var in bulkIn
	if !webserver.Decode(w, r, &in, bulkHelp) {
		return
	}
	if !a.bulkHashesOK(w, r, id, in.Hashes) {
		return
	}
	known := in.Action == "remove"
	for _, k := range BulkActions {
		known = known || k == in.Action
	}
	if !known {
		webserver.Fail(w, http.StatusBadRequest, bulkHelp)
		return
	}
	if in.Action == "remove" && !a.changeOutsideOK(w, r, id, in.Hashes) {
		return
	}
	err = a.onBulk(r, id, func(t TorrentBulk) error {
		if in.Action == "remove" {
			err := t.RemoveMany(r.Context(), in.Hashes, in.DeleteFiles)
			if err == nil {
				if len(in.Hashes) == 1 {
					a.itemRemovedEvent(r, id, in.Hashes[0], in.DeleteFiles)
				} else {
					verb := "removed"
					if in.DeleteFiles {
						verb = "removed (with their files)"
					}
					a.manyChangedEvent(r, id, in.Hashes, "remove", verb, map[string]any{"delete_files": in.DeleteFiles})
				}
				a.forgetTorrents(r.Context(), id, in.Hashes)
			}
			return err
		}
		err := t.Bulk(r.Context(), in.Action, in.Hashes)
		if err == nil {
			a.manyChangedEvent(r, id, in.Hashes, in.Action, bulkVerbs[in.Action], nil)
		}
		return err
	})
	if errors.Is(err, ErrQueueOff) {
		webserver.Fail(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(in.Hashes)})
}

// bulkVerbs say a bulk action in History's words.
var bulkVerbs = map[string]string{
	BulkStart: "resumed", BulkPause: "paused", BulkRecheck: "rechecked", BulkReannounce: "reannounced",
	BulkTop: "moved to the top", BulkBottom: "moved to the bottom", BulkUp: "moved up", BulkDown: "moved down",
	BulkForce: "force-started", BulkUnforce: "stopped force-starting",
}

// bulkHashesOK checks a selection: 1 to 1000 info hashes, none of them
// another app's item on a Downloader that shows only its own.
func (a *Web) bulkHashesOK(w http.ResponseWriter, r *http.Request, downloaderID int64, hashes []string) bool {
	if len(hashes) == 0 || len(hashes) > 1000 {
		webserver.Fail(w, http.StatusBadRequest, "hashes needs 1 to 1000 info hashes")
		return false
	}
	for _, h := range hashes {
		if !validHash(strings.ToLower(h)) {
			webserver.Fail(w, http.StatusBadRequest, "hashes are 40-character info hashes")
			return false
		}
	}
	if a.notOurs(r.Context(), downloaderID, hashes) {
		webserver.Fail(w, http.StatusForbidden, ErrNotOurs.Error())
		return false
	}
	return true
}

// mayChangeOutside says the person may change or remove these torrents
// beyond pause, resume, recheck, reannounce and queue moves: any of them that
// HolyArrs did not send needs the settings permission and an admin-scoped
// key. No identity attached means no, unless the server runs without
// sign-in (Auth nil, tests only).
func (a *Web) mayChangeOutside(ctx context.Context, downloaderID int64, hashes []string) bool {
	who, ok := webserver.From(ctx)
	if !ok {
		return a.Auth == nil
	}
	if who.Has(webserver.PermSettings) && who.HasScope(webserver.ScopeAdmin) {
		return true
	}
	for _, h := range hashes {
		if a.outsideHash(ctx, downloaderID, h) {
			return false
		}
	}
	return true
}

const outsideRefused = "changing a torrent HolyArrs did not send needs the settings permission"

// changeOutsideOK answers 403 when mayChangeOutside says no.
func (a *Web) changeOutsideOK(w http.ResponseWriter, r *http.Request, downloaderID int64, hashes []string) bool {
	if a.mayChangeOutside(r.Context(), downloaderID, hashes) {
		return true
	}
	webserver.Fail(w, http.StatusForbidden, outsideRefused)
	return false
}

// itemChangeOK is changeOutsideOK for the torrent named in the path.
func (a *Web) itemChangeOK(w http.ResponseWriter, r *http.Request) bool {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return true // the route itself answers 404
	}
	return a.changeOutsideOK(w, r, id, []string{r.PathValue("hash")})
}

// onBulk runs a call on a torrent Downloader that acts on many torrents.
func (a *Web) onBulk(r *http.Request, id int64, call func(t TorrentBulk) error) error {
	return a.Manager().ControlTorrentClient(r.Context(), id, true, func(e Engine) error {
		t, ok := e.(TorrentBulk)
		if !ok {
			return ErrNotSupported
		}
		return call(t)
	})
}

const shareHelp = `body is {"hashes": [info hash...], "ratio": -2 (Downloader's rule), -1 (none) or 0 to 9998, "seeding_minutes": -2, -1 or 0 to 525600, "inactive_minutes": -2, -1 or 0 to 525600, "action": "", "stop", "remove", "remove_files" or "super_seed"}`

// ShareLimits sets seeding rules on many torrents of one Downloader. On
// torrents HolyArrs did not send, rules that may end in a remove need the
// settings permission, as removing them does.
func (a *Web) ShareLimits(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, ErrNoDownloader.Error())
		return
	}
	var in struct {
		Hashes []string `json:"hashes"`
		ShareLimits
	}
	if !webserver.Decode(w, r, &in, shareHelp) || !a.bulkHashesOK(w, r, id, in.Hashes) {
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
		webserver.Fail(w, http.StatusBadRequest, shareHelp)
		return
	}
	// Any limit could trigger a remove: the torrent's own action, or the
	// client's global one when the action is left on the default (and on
	// qBittorrent before 5.2, which has no per-torrent action). Only an
	// explicit stop or super seed (which those clients refuse) or clearing
	// every limit needs no more than managing the Library.
	safe := l.Action == "stop" || l.Action == "super_seed" || l.Ratio == -1 && l.SeedingMinutes == -1 && l.InactiveMinutes == -1
	if !safe && !a.changeOutsideOK(w, r, id, in.Hashes) {
		return
	}
	err = a.Manager().ControlTorrentClient(r.Context(), id, true, func(e Engine) error {
		t, ok := e.(ShareLimiter)
		if !ok {
			return ErrNotSupported
		}
		return t.SetShareLimits(r.Context(), in.Hashes, l)
	})
	if errors.Is(err, ErrShareAction) {
		webserver.Fail(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(in.Hashes)})
}

// slowMode turns a torrent Downloader's slower second set of speed limits
// on or off.
func (a *Web) slowMode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, ErrNoDownloader.Error())
		return
	}
	var in struct {
		On *bool `json:"on"`
	}
	if !webserver.Decode(w, r, &in, `body is {"on": true or false}`) {
		return
	}
	if in.On == nil {
		webserver.Fail(w, http.StatusBadRequest, `body is {"on": true or false}`)
		return
	}
	err = a.Manager().ControlTorrentClient(r.Context(), id, true, func(e Engine) error {
		t, ok := e.(SlowModer)
		if !ok {
			return ErrNotSupported
		}
		return t.SetSlowMode(r.Context(), *in.On)
	})
	if err != nil {
		itemError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "slow_mode": *in.On})
}

// onTrackers runs a tracker change on one torrent of a torrent Downloader.
func (a *Web) onTrackers(w http.ResponseWriter, r *http.Request, call func(t TrackerEditor, id string) error) {
	err := a.onItem(r, true, func(e Engine, id string) error {
		t, ok := e.(TrackerEditor)
		if !ok {
			return ErrNotSupported
		}
		return call(t, id)
	})
	if errors.Is(err, ErrTrackerTier) {
		webserver.Fail(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// trackerURLsOK checks typed tracker addresses under the Downloader's
// address policy.
func (a *Web) trackerURLsOK(w http.ResponseWriter, r *http.Request, urls []string) bool {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var dr domain.Downloader
	if err := database.Get(r.Context(), a.Store.Read(), "downloaders", id, &dr); err != nil || dr.Removed {
		webserver.Fail(w, http.StatusNotFound, ErrNoDownloader.Error())
		return false
	}
	for _, u := range urls {
		if !TrackerAllowed(u, dr.AllowLocal) {
			webserver.Fail(w, http.StatusBadRequest, "a tracker is an http, https or udp address on the internet (local ones need the Downloader's local network access): "+trackerShown(u))
			return false
		}
	}
	return true
}

// trackerShown is a typed address cut to its start, for an error.
func trackerShown(u string) string {
	if len(u) > 60 {
		return u[:60] + "…"
	}
	return u
}

// addTrackers adds tracker addresses to one torrent.
func (a *Web) addTrackers(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URLs []string `json:"urls"`
	}
	if !webserver.Decode(w, r, &in, `body is {"urls": [tracker address...]}`) {
		return
	}
	if !a.itemChangeOK(w, r) {
		return
	}
	urls := []string{}
	for _, u := range in.URLs {
		if u = strings.TrimSpace(u); u != "" {
			urls = append(urls, u)
		}
	}
	if len(urls) == 0 || len(urls) > 50 {
		webserver.Fail(w, http.StatusBadRequest, "urls needs 1 to 50 tracker addresses")
		return
	}
	if !a.trackerURLsOK(w, r, urls) {
		return
	}
	a.onTrackers(w, r, func(t TrackerEditor, id string) error { return t.AddTrackers(r.Context(), id, urls) })
}

// editTracker changes one tracker's address or tier, naming it by ref.
func (a *Web) editTracker(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ref  string `json:"ref"`
		URL  string `json:"url"`
		Tier *int   `json:"tier"`
	}
	const help = `body is {"ref": tracker ref, "url": new address or "", "tier": 0 to 255 or left out}`
	if !webserver.Decode(w, r, &in, help) {
		return
	}
	if !a.itemChangeOK(w, r) {
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	if in.Ref == "" || in.URL == "" && in.Tier == nil || in.Tier != nil && (*in.Tier < 0 || *in.Tier > 255) {
		webserver.Fail(w, http.StatusBadRequest, help)
		return
	}
	if in.URL != "" && !a.trackerURLsOK(w, r, []string{in.URL}) {
		return
	}
	a.onTrackers(w, r, func(t TrackerEditor, id string) error {
		return t.EditTracker(r.Context(), id, in.Ref, in.URL, in.Tier)
	})
}

// removeTrackers removes trackers of one torrent, naming them by ref.
func (a *Web) removeTrackers(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Refs []string `json:"refs"`
	}
	if !webserver.Decode(w, r, &in, `body is {"refs": [tracker ref...]}`) {
		return
	}
	if !a.itemChangeOK(w, r) {
		return
	}
	if len(in.Refs) == 0 || len(in.Refs) > 100 {
		webserver.Fail(w, http.StatusBadRequest, "refs needs 1 to 100 tracker refs")
		return
	}
	a.onTrackers(w, r, func(t TrackerEditor, id string) error { return t.RemoveTrackers(r.Context(), id, in.Refs) })
}

// onMover runs a call on a torrent Downloader that moves or caps torrents.
func (a *Web) onMover(r *http.Request, id int64, call func(t Mover) error) error {
	return a.Manager().ControlTorrentClient(r.Context(), id, true, func(e Engine) error {
		t, ok := e.(Mover)
		if !ok {
			return ErrNotSupported
		}
		return call(t)
	})
}

// SetLocation moves many torrents' data to one folder. Without the
// settings permission the folder is a subfolder of where the category (or
// the Downloader by default) saves, as for an add.
func (a *Web) SetLocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, ErrNoDownloader.Error())
		return
	}
	var in struct {
		Hashes   []string `json:"hashes"`
		Category string   `json:"category"`
		Folder   string   `json:"folder"`
	}
	const help = `body is {"hashes": [info hash...], "category": "", "folder": "a subfolder, or a full path with the settings permission"}`
	if !webserver.Decode(w, r, &in, help) || !a.bulkHashesOK(w, r, id, in.Hashes) {
		return
	}
	if !a.changeOutsideOK(w, r, id, in.Hashes) {
		return
	}
	if strings.TrimSpace(in.Folder) == "" {
		webserver.Fail(w, http.StatusBadRequest, help)
		return
	}
	_, folder, err := a.ClientFolder(r.Context(), id, in.Category, in.Folder, true)
	if folderError(w, err) {
		return
	}
	if err != nil {
		itemError(w, err)
		return
	}
	if err := a.onMover(r, id, func(t Mover) error { return t.SetLocation(r.Context(), in.Hashes, folder) }); err != nil {
		itemError(w, err)
		return
	}
	a.manyChangedEvent(r, id, in.Hashes, "move", "moved the files of", map[string]any{"folder": folder})
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(in.Hashes), "folder": folder})
}

// torrentLimits caps many torrents' download and upload speeds.
func (a *Web) torrentLimits(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		webserver.Fail(w, http.StatusNotFound, ErrNoDownloader.Error())
		return
	}
	var in struct {
		Hashes []string `json:"hashes"`
		Down   int64    `json:"down"`
		Up     int64    `json:"up"`
	}
	const help = `body is {"hashes": [info hash...], "down": bytes per second (0 no cap), "up": bytes per second (0 no cap)}`
	if !webserver.Decode(w, r, &in, help) || !a.bulkHashesOK(w, r, id, in.Hashes) {
		return
	}
	if !a.changeOutsideOK(w, r, id, in.Hashes) {
		return
	}
	if in.Down < 0 || in.Up < 0 || in.Down > 1<<40 || in.Up > 1<<40 {
		webserver.Fail(w, http.StatusBadRequest, help)
		return
	}
	if err := a.onMover(r, id, func(t Mover) error { return t.SetTorrentLimits(r.Context(), in.Hashes, in.Down, in.Up) }); err != nil {
		itemError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(in.Hashes)})
}

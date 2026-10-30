package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs/internal/downloader"
)

// Hand steering of a torrent Downloader from the Downloads screen, the way
// the client's own screen does it: adding torrents by link or file. What
// these routes add or change is the client's, never a HolyArrs download.
func (a *API) clientManageRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/downloaders/{id}/torrents", a.addTorrents)
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
	for _, l := range b.Links {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		low := strings.ToLower(l)
		if !strings.HasPrefix(low, "magnet:") && !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") && !validHash(low) {
			fail(w, http.StatusBadRequest, "each line is a magnet link, an http(s) link to a .torrent file, or an info hash")
			return
		}
	}
	b.Category, b.SavePath = strings.TrimSpace(b.Category), strings.TrimSpace(b.SavePath)
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

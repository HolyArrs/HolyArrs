package library

import (
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// FileDownloadHandlers lets a person save a movie or episode file to their
// own device.
type FileDownloadHandlers struct {
	Store *database.Store
}

// Routes registers GET /api/v1/files/{id}/download.
func (h *FileDownloadHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/files/{id}/download", h.download)
}

// videoTypes name a video's type outright: the server image has no
// mime.types file, and a guessed type can make a browser play the file
// instead of saving it.
var videoTypes = map[string]string{
	".mkv":  "video/x-matroska",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".avi":  "video/x-msvideo",
	".webm": "video/webm",
	".ts":   "video/mp2t",
}

// fileType is the Content-Type sent for a file name.
func fileType(name string) string {
	if t, ok := videoTypes[strings.ToLower(filepath.Ext(name))]; ok {
		return t
	}
	return "application/octet-stream"
}

// download sends one present main file as an attachment named after the
// file. A kid, a side file (subtitle, artwork) or a file not on disk is 404.
func (h *FileDownloadHandlers) download(w http.ResponseWriter, r *http.Request) {
	if c, ok := webserver.From(r.Context()); ok && c.MaxRating != "" {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	file, ok, err := h.mainFilePath(r, id)
	if err != nil {
		webserver.Internal(w, "library: file download", err)
		return
	}
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	name := filepath.Base(file)
	w.Header().Set("Content-Type", fileType(name))
	webserver.ServeAttachment(w, r, file, name, true)
}

// mainFilePath is where a present main file lies on disk. False for no
// such file, any other file, one without a Library folder, or a path that
// leaves it; an error only when the database fails.
func (h *FileDownloadHandlers) mainFilePath(r *http.Request, id int64) (string, bool, error) {
	ctx := r.Context()
	var f mediafiles.File
	if err := mediafiles.LoadFile(ctx, h.Store.Read(), id, &f); err != nil {
		return "", false, notFoundIsFalse(err)
	}
	if f.Role != "main" || f.State != "present" || f.RelativePath == "" {
		return "", false, nil
	}
	root, err := mediafiles.LibraryPathOfVersion(ctx, h.Store.Read(), f.VersionID)
	if err != nil || root == "" {
		return "", false, notFoundIsFalse(err)
	}
	root = filepath.Clean(root)
	file := filepath.Join(root, filepath.FromSlash(f.RelativePath))
	rel, err := filepath.Rel(root, file)
	if err != nil || !filepath.IsLocal(rel) {
		return "", false, nil
	}
	return file, true, nil
}

// notFoundIsFalse drops a missing-row error, which only means 404.
func notFoundIsFalse(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, database.ErrNotFound) {
		return nil
	}
	return err
}

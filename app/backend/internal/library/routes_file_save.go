package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// FileSaveHandlers let a person save a Library file to their own device,
// as it is: no conversion, with ranges so a broken save can resume.
type FileSaveHandlers struct {
	Store *database.Store
}

// Routes registers GET /api/v1/titles/{id}/files/{fid}/save (HEAD too).
// The permission gate asks for save_files; the Kid gate and the part gate
// answer for the {id} title before the handler runs.
func (h *FileSaveHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/files/{fid}/save", h.save) // save a Library file to your device, as it is
}

// fileTypes name a file's type outright: the server image has no mime.types
// file, and a guessed type can make a browser play a video instead of
// saving it.
var fileTypes = map[string]string{
	".mkv":  "video/x-matroska",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".avi":  "video/x-msvideo",
	".webm": "video/webm",
	".ts":   "video/mp2t",
	".flac": "audio/flac",
	".mp3":  "audio/mpeg",
	".m4a":  "audio/mp4",
}

// saveType is the Content-Type sent for a file name.
func saveType(name string) string {
	if t, ok := fileTypes[strings.ToLower(path.Ext(name))]; ok {
		return t
	}
	return "application/octet-stream"
}

// saveName is the file name the browser is told to use: the last part of
// the Library path without control characters, which could break the
// header. Non-ASCII goes as filename* (mime.FormatMediaType does that).
func saveName(rel string) string {
	name := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return -1
		}
		return r
	}, path.Base(filepath.ToSlash(rel)))
	if strings.Trim(name, ". ") == "" {
		return "file" + path.Ext(rel)
	}
	return name
}

// errNotSaveable is any reason a file is not there to save; the answer is
// the same 404 for all of them, so nothing about it leaks.
var errNotSaveable = errors.New("not saveable")

// save sends one present main file of a title the person may see. A Kid, a
// side file, a file of another title, one not on disk, a folder or a link
// out of the Library folder all answer 404.
func (h *FileSaveHandlers) save(w http.ResponseWriter, r *http.Request) {
	if c, ok := webserver.From(r.Context()); ok && (c.MaxRating != "" || c.Via == "plugin") {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	titleID, err1 := strconv.ParseInt(r.PathValue("id"), 10, 64)
	fileID, err2 := strconv.ParseInt(r.PathValue("fid"), 10, 64)
	if err1 != nil || err2 != nil || titleID <= 0 || fileID <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	f, st, rel, err := h.open(r.Context(), titleID, fileID)
	if errors.Is(err, errNotSaveable) {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		webserver.Internal(w, "library: save a file", err)
		return
	}
	defer f.Close()
	name := saveName(rel)
	hd := w.Header()
	hd.Set("Content-Type", saveType(name))
	hd.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	hd.Set("X-Content-Type-Options", "nosniff")
	hd.Set("Cache-Control", "private, no-store")
	// The tag follows the file's size and time, so a resume (If-Range) of a
	// file that changed since gets the whole new file instead of a mix.
	hd.Set("ETag", fmt.Sprintf(`"%d-%x-%x"`, fileID, st.Size(), st.ModTime().UnixNano()))
	if r.Method == http.MethodGet && fromStart(r.Header.Get("Range")) {
		h.record(r, titleID, fileID)
	}
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// fromStart says a request begins a save rather than resuming one: no
// range, or one from byte 0.
func fromStart(rng string) bool {
	return rng == "" || strings.HasPrefix(strings.ReplaceAll(rng, " ", ""), "bytes=0-")
}

// open finds the file through the Library's own rows only (the request
// names ids, never a path) and opens it inside its Library folder, so a
// ".." or a link that leads out of the folder can't be followed. It must be
// a present main file of a Version of that title, and a regular file.
func (h *FileSaveHandlers) open(ctx context.Context, titleID, fileID int64) (*os.File, os.FileInfo, string, error) {
	db := h.Store.Read()
	var t mediafiles.Title
	if err := mediafiles.LoadTitle(ctx, db, titleID, &t); err != nil {
		return nil, nil, "", missingIsNotSaveable(err)
	}
	var f mediafiles.File
	if err := mediafiles.LoadFile(ctx, db, fileID, &f); err != nil {
		return nil, nil, "", missingIsNotSaveable(err)
	}
	if t.State == "removed" || f.Role != "main" || f.State != "present" || f.RelativePath == "" {
		return nil, nil, "", errNotSaveable
	}
	var v mediafiles.Version
	if err := mediafiles.LoadVersion(ctx, db, f.VersionID, &v); err != nil {
		return nil, nil, "", missingIsNotSaveable(err)
	}
	if v.TitleID != titleID {
		return nil, nil, "", errNotSaveable
	}
	root, err := mediafiles.LibraryPathOfVersion(ctx, db, v.ID)
	if err != nil {
		return nil, nil, "", missingIsNotSaveable(err)
	}
	file, st, err := openInside(root, f.RelativePath)
	if err != nil {
		return nil, nil, "", errNotSaveable
	}
	return file, st, f.RelativePath, nil
}

// openInside opens rel under root through os.Root, which refuses absolute
// paths, ".." and links that lead out of root. It is checked to be a
// regular file before it is opened (a pipe would block the open) and again
// after, in case it was swapped in between.
func openInside(root, rel string) (*os.File, os.FileInfo, error) {
	if root == "" {
		return nil, nil, errNotSaveable
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, err
	}
	defer dir.Close()
	name := filepath.FromSlash(rel)
	before, err := dir.Stat(name)
	if err != nil || !before.Mode().IsRegular() {
		return nil, nil, errNotSaveable
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || !os.SameFile(before, st) {
		_ = f.Close()
		return nil, nil, errNotSaveable
	}
	return f, st, nil
}

// missingIsNotSaveable turns a missing row into the plain 404.
func missingIsNotSaveable(err error) error {
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, database.ErrNotFound) {
		return errNotSaveable
	}
	return err
}

// record writes a file_saved Event: who took which file out, by ids only.
func (h *FileSaveHandlers) record(r *http.Request, titleID, fileID int64) {
	_ = h.Store.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: webserver.Actor(r.Context()), Kind: domain.EventFileSaved,
		TitleID: &titleID, Summary: "A Library file was saved to a device",
		Data: map[string]any{"title_id": titleID, "file_id": fileID}})
}

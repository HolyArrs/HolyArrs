package downloads

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Guards for the hand steering routes: what a person may name as a folder
// or category, and which links a client may be told to fetch.

// mayPickAnyFolder says the person may name any folder or category on the
// Downloader's machine: the settings permission and an admin-scoped key.
// No identity attached means no, unless the server runs without sign-in
// (Auth nil, tests only).
func (a *Web) mayPickAnyFolder(ctx context.Context) bool {
	who, ok := webserver.From(ctx)
	if !ok {
		return a.Auth == nil
	}
	return who.Has(webserver.PermSettings) && who.HasScope(webserver.ScopeAdmin)
}

var ErrFolder = errors.New("the folder is a subfolder name inside the Downloader's own folder (no full paths, drive letters or ..)")
var ErrPickCategory = errors.New("pick one of the Downloader's own categories")

// subfolder checks a relative subfolder: no absolute path, drive letter,
// UNC path, .. part or control character.
func subfolder(p string) (string, error) {
	if hasControl(p) || strings.Contains(p, `\`) || strings.Contains(p, ":") {
		return "", ErrFolder
	}
	if strings.HasPrefix(p, "/") {
		return "", ErrFolder
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", ErrFolder
		}
	}
	c := path.Clean(p)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") || path.IsAbs(c) {
		return "", ErrFolder
	}
	return c, nil
}

// joinFolder puts a checked subfolder under a client folder, in that
// folder's own style (a Windows client uses backslashes).
func joinFolder(base, sub string) string {
	if strings.Contains(base, `\`) {
		return strings.TrimRight(base, `\`) + `\` + strings.ReplaceAll(sub, "/", `\`)
	}
	return path.Join(base, sub)
}

// ClientFolder settles the category and folder of a hand-made add or move.
// Someone with the settings permission may name any; anyone else picks one
// of the client's categories (or the Downloader's own) and at most a
// subfolder of where that category saves.
func (a *Web) ClientFolder(ctx context.Context, downloaderID int64, category, folder string, checkCategory bool) (string, string, error) {
	category, folder = strings.TrimSpace(category), strings.TrimSpace(folder)
	if hasControl(category) || hasControl(folder) {
		return "", "", ErrFolder
	}
	if a.mayPickAnyFolder(ctx) || (folder == "" && (!checkCategory || category == "")) {
		return category, folder, nil
	}
	var dr domain.Downloader
	if err := database.Get(ctx, a.Store.Read(), "downloaders", downloaderID, &dr); err != nil || dr.Removed {
		return "", "", ErrNoDownloader
	}
	var base SaveBase
	err := a.Manager().ControlTorrentClient(ctx, downloaderID, false, func(e Engine) error {
		t, ok := e.(TorrentFolders)
		if !ok {
			return ErrNotSupported
		}
		var err error
		base, err = t.SaveBase(ctx)
		return err
	})
	if err != nil {
		return "", "", err
	}
	if checkCategory && category != "" {
		if _, ok := base.Categories[category]; !ok && category != dr.Category {
			return "", "", ErrPickCategory
		}
	}
	if folder == "" {
		return category, "", nil
	}
	sub, err := subfolder(folder)
	if err != nil {
		return "", "", err
	}
	root := base.Default
	if p := base.Categories[category]; p != "" {
		root = p
	}
	if root == "" {
		return "", "", ErrFolder
	}
	return category, joinFolder(root, sub), nil
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

var errLink = errors.New("each line is a magnet link, an http(s) link to a .torrent file, or an info hash")

// checkLink accepts a magnet link, an http(s) link or a 40/64-hex hash, as
// written: no control characters or spaces inside.
func checkLink(l string) error {
	if hasControl(l) || strings.ContainsAny(l, " \t") {
		return errLink
	}
	if isHex(l, 40) || isHex(l, 64) {
		return nil
	}
	u, err := url.Parse(l)
	if err != nil {
		return errLink
	}
	switch strings.ToLower(u.Scheme) {
	case "magnet":
		if u.Opaque == "" && u.RawQuery == "" {
			return errLink
		}
		return nil
	case "http", "https":
	default:
		return errLink
	}
	if u.Hostname() == "" || u.User != nil {
		return errLink
	}
	// The address itself is checked when HolyArrs fetches the file (every
	// dialled address, redirects included); the client never gets it.
	return nil
}

func isHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range strings.ToLower(s) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// folderError answers a refused folder, category or link.
func folderError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, ErrFolder), errors.Is(err, ErrPickCategory), errors.Is(err, errLink):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return true
	}
	return false
}

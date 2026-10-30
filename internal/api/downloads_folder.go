package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Guards for the hand steering routes: what a person may name as a folder
// or category, and which links a client may be told to fetch.

// mayPickAnyFolder says the person may name any folder or category on the
// Downloader's machine: the settings permission and an admin-scoped key.
func mayPickAnyFolder(ctx context.Context) bool {
	who, ok := auth.From(ctx)
	return !ok || (who.Has(auth.PermSettings) && who.HasScope(auth.ScopeAdmin))
}

var errFolder = errors.New("the folder is a subfolder name inside the Downloader's own folder (no full paths, drive letters or ..)")
var errCategory = errors.New("pick one of the Downloader's own categories")

// subfolder checks a relative subfolder: no absolute path, drive letter,
// UNC path, .. part or control character.
func subfolder(p string) (string, error) {
	if hasControl(p) || strings.Contains(p, `\`) || strings.Contains(p, ":") {
		return "", errFolder
	}
	if strings.HasPrefix(p, "/") {
		return "", errFolder
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", errFolder
		}
	}
	c := path.Clean(p)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") || path.IsAbs(c) {
		return "", errFolder
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

// clientFolder settles the category and folder of a hand-made add or move.
// Someone with the settings permission may name any; anyone else picks one
// of the client's categories (or the Downloader's own) and at most a
// subfolder of where that category saves.
func (a *API) clientFolder(ctx context.Context, downloaderID int64, category, folder string, checkCategory bool) (string, string, error) {
	category, folder = strings.TrimSpace(category), strings.TrimSpace(folder)
	if hasControl(category) || hasControl(folder) {
		return "", "", errFolder
	}
	if mayPickAnyFolder(ctx) || (folder == "" && (!checkCategory || category == "")) {
		return category, folder, nil
	}
	var dr domain.Downloader
	if err := store.Get(ctx, a.s.Store.Read(), "downloaders", downloaderID, &dr); err != nil || dr.Removed {
		return "", "", downloader.ErrNoDownloader
	}
	var base downloader.SaveBase
	err := a.manager().ControlTorrentClient(ctx, downloaderID, false, func(e downloader.Engine) error {
		t, ok := e.(downloader.TorrentFolders)
		if !ok {
			return downloader.ErrNotSupported
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
			return "", "", errCategory
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
		return "", "", errFolder
	}
	return category, joinFolder(root, sub), nil
}

func hasControl(s string) bool {
	return strings.IndexFunc(s, unicode.IsControl) >= 0
}

var errLink = errors.New("each line is a magnet link, an http(s) link to a .torrent file, or an info hash")
var errLocalLink = errors.New("that link points at this machine or the local network; turn on local network access for this Downloader to allow it")

// checkLink accepts a magnet link, an http(s) link or a 40/64-hex hash.
// An http(s) link must reach a public address unless the Downloader may
// use the local network; a cloud metadata address is never allowed.
func checkLink(ctx context.Context, l string, allowLocal bool) error {
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
	host := u.Hostname()
	if host == "" || u.User != nil {
		return errLink
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addrs) == 0 {
			return errors.New("could not look up " + host)
		}
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}
	for _, ip := range ips {
		if !source.NotMetadata(ip) {
			return errLocalLink
		}
		if !allowLocal && !source.PublicOnly(ip) {
			return errLocalLink
		}
	}
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
	case errors.Is(err, errFolder), errors.Is(err, errCategory), errors.Is(err, errLink), errors.Is(err, errLocalLink):
		fail(w, http.StatusBadRequest, err.Error())
		return true
	}
	return false
}

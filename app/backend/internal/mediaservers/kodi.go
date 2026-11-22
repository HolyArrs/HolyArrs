package mediaservers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/doctor"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// kodiCall sends one JSON-RPC call to Kodi's web server (POST /jsonrpc, basic
// auth) and returns the result. Kodi answers a refused call with HTTP 200 and
// an error object, so that is read as well.
func (l *Linker) kodiCall(ctx context.Context, m domain.MediaServer, method string, params any) (json.RawMessage, error) {
	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		req["params"] = params
	}
	body, _ := json.Marshal(req)
	b, err := l.do(ctx, m, http.MethodPost, "/jsonrpc", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	// A page or answer that is not JSON-RPC means another app answers there.
	if json.Unmarshal(b, &out) != nil || out.JSONRPC == "" {
		return nil, doctor.Mark(doctor.WrongApp, nil)
	}
	if out.Error != nil {
		if out.Error.Code == -32601 || out.Error.Code == -32602 {
			return nil, fmt.Errorf("this Kodi does not accept %s as sent (%s); Kodi 19 or newer is needed", method, out.Error.Message)
		}
		return nil, fmt.Errorf("Kodi refused %s: %s", method, out.Error.Message)
	}
	return out.Result, nil
}

// kodiDir is a folder as Kodi wants a scan or clean path: with a closing
// separator, the way Kodi writes its own folders.
func kodiDir(p string) string {
	if strings.HasSuffix(p, "/") || strings.HasSuffix(p, `\`) {
		return p
	}
	if strings.Contains(p, `\`) && !strings.Contains(p, "/") {
		return p + `\`
	}
	return p + "/"
}

// kodiParent is the folder that holds dir, with its closing separator; "" when
// dir sits at a root (a bare share, a drive, "/") and has no folder worth listing.
func kodiParent(dir string) string {
	sep := "/"
	if strings.Contains(dir, `\`) && !strings.Contains(dir, "/") {
		sep = `\`
	}
	d := strings.TrimRight(dir, `/\`)
	i := strings.LastIndex(d, sep)
	if i < 0 {
		return ""
	}
	p := d[:i+1]
	if p == sep || strings.HasSuffix(p, "://") {
		return ""
	}
	return p
}

// kodiRefreshListing has Kodi read the folder again. Kodi keeps the last
// listing of a folder it read and, when asked to scan a folder inside it,
// trusts that listing: a folder made since then "does not exist" and the scan
// skips it with an OK answer. A fresh listing replaces the kept one. A failure
// is not reported here: the scan that follows meets the same problem and says so.
func (l *Linker) kodiRefreshListing(ctx context.Context, m domain.MediaServer, dir string) {
	_, _ = l.kodiCall(ctx, m, "Files.GetDirectory", map[string]any{"directory": dir, "media": "files", "limits": map[string]int{"start": 0, "end": 1}})
}

// kodiContent is the content type Kodi keeps a title's folder under.
func kodiContent(kind string) string {
	if kind == "show" {
		return "tvshows"
	}
	return "movies"
}

// tellKodi asks Kodi to scan each new or changed folder and to clean each
// deleted one, only that folder. Before a scan it reads the folder's parent
// again (see kodiRefreshListing). A clean names the content type: Kodi cleans
// a folder only when its stored content equals the one asked for, and "video",
// the default, never equals "movies" or "tvshows", so it would clean nothing
// and still answer OK. Music goes to Kodi's music library, which can scan a
// folder but cleans only as a whole, so a deleted music folder is left for
// Kodi's own clean.
func (l *Linker) tellKodi(ctx context.Context, m domain.MediaServer, us []Update) error {
	listed := map[string]bool{}
	for _, u := range us {
		lib := "VideoLibrary"
		if u.Kind == "artist" {
			lib = "AudioLibrary"
		}
		dir := kodiDir(u.Path)
		var err error
		switch u.Change {
		case Deleted:
			if lib == "AudioLibrary" {
				continue
			}
			_, err = l.kodiCall(ctx, m, lib+".Clean", map[string]any{"showdialogs": false, "content": kodiContent(u.Kind), "directory": dir})
		default:
			if p := kodiParent(dir); p != "" && !listed[p] {
				listed[p] = true
				l.kodiRefreshListing(ctx, m, p)
			}
			_, err = l.kodiCall(ctx, m, lib+".Scan", map[string]any{"showdialogs": false, "directory": dir})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// testKodi checks the address and login: Kodi must answer its version.
func (l *Linker) testKodi(ctx context.Context, m domain.MediaServer) (string, error) {
	r, err := l.kodiCall(ctx, m, "Application.GetProperties", map[string]any{"properties": []string{"name", "version"}})
	if err != nil {
		return "", err
	}
	var p struct {
		Name    string `json:"name"`
		Version struct {
			Major    int    `json:"major"`
			Minor    int    `json:"minor"`
			Revision any    `json:"revision"`
			Tag      string `json:"tag"`
		} `json:"version"`
	}
	if json.Unmarshal(r, &p) != nil || p.Version.Major == 0 {
		return "", doctor.Mark(doctor.WrongApp, nil)
	}
	if p.Version.Major < 19 {
		return "", errors.New("Kodi 19 or newer is needed to clean one folder")
	}
	name := p.Name
	if name == "" {
		name = "Kodi"
	}
	return fmt.Sprintf("%s %d.%d", name, p.Version.Major, p.Version.Minor), nil
}

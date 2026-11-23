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

// Kodi takes calls over JSON-RPC at /jsonrpc on its web server, with the
// web server's user name and password (basic auth). Like a Subsonic-style
// server it keeps both as one vault secret, the name first, then a new line.
// HolyArrs asks Kodi to scan each new or renamed folder and to clean
// removed ones; it never reads people, watch state or labels from Kodi.

// refreshOnly says m is only told about changed folders: never asked for
// people, watch state, labels or tracks.
func refreshOnly(m domain.MediaServer) bool { return m.Kind == "subsonic" || m.Kind == "kodi" }

// kodiCall sends one JSON-RPC call and returns its result. A refusal from
// Kodi itself comes back in Kodi's words.
func (l *Linker) kodiCall(ctx context.Context, m domain.MediaServer, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	b, err := l.do(ctx, m, http.MethodPost, "/jsonrpc", body)
	if err != nil {
		return nil, err
	}
	var a struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &a) != nil || (a.Result == nil && a.Error == nil) {
		return nil, doctor.Mark(doctor.WrongApp, nil)
	}
	if a.Error != nil {
		return nil, fmt.Errorf("Kodi refused the call: %s", strings.TrimSpace(a.Error.Message))
	}
	return a.Result, nil
}

// kodiDir is a folder as Kodi wants it for a scan or clean: ending with its
// separator.
func kodiDir(p string) string {
	if strings.HasSuffix(p, "/") || strings.HasSuffix(p, `\`) {
		return p
	}
	if strings.Contains(p, `\`) && !strings.Contains(p, "/") {
		return p + `\`
	}
	return p + "/"
}

// kodiTell scans each new or renamed folder and cleans each renamed or
// removed one, so its old entries leave the library. Music stays out: Kodi
// scans music only as a whole library.
func (l *Linker) kodiTell(ctx context.Context, m domain.MediaServer, us []Update) error {
	for _, u := range us {
		if u.Kind == "artist" {
			continue
		}
		dir := kodiDir(u.Path)
		var methods []string
		switch u.Change {
		case Created:
			methods = []string{"VideoLibrary.Scan"}
		case Deleted:
			methods = []string{"VideoLibrary.Clean"}
		default:
			methods = []string{"VideoLibrary.Scan", "VideoLibrary.Clean"}
		}
		for _, method := range methods {
			if _, err := l.kodiCall(ctx, m, method, map[string]any{"directory": dir, "showdialogs": false}); err != nil {
				return err
			}
		}
	}
	return nil
}

// kodiTest reads Kodi's name and version, which also checks the sign-in.
func (l *Linker) kodiTest(ctx context.Context, m domain.MediaServer) (string, error) {
	r, err := l.kodiCall(ctx, m, "Application.GetProperties", map[string]any{"properties": []string{"name", "version"}})
	if err != nil {
		return "", err
	}
	var p struct {
		Name    string `json:"name"`
		Version struct {
			Major int `json:"major"`
			Minor int `json:"minor"`
		} `json:"version"`
	}
	if json.Unmarshal(r, &p) != nil || p.Name == "" {
		return "", doctor.Mark(doctor.WrongApp, nil)
	}
	return fmt.Sprintf("%s %d.%d", p.Name, p.Version.Major, p.Version.Minor), nil
}

// ErrNoKodiUser asks for the user name and password of Kodi's web server.
var ErrNoKodiUser = errors.New("type the user name and password of Kodi's web server (Settings, Services, Control)")

// secretOf is what the vault keeps: the token, or for a Subsonic-style
// server or Kodi its user name and password.
func secretOf(kind string, in Input) (string, error) {
	if kind == "kodi" {
		s, err := subsonicSecret(in.Username, in.Token)
		if err != nil {
			return "", ErrNoKodiUser
		}
		return s, nil
	}
	if kind == "subsonic" {
		return subsonicSecret(in.Username, in.Token)
	}
	return strings.TrimSpace(in.Token), nil
}

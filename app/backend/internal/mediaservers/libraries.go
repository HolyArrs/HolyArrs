package mediaservers

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Library is one movie, TV or music library on a media server.
type Library struct {
	Key   string   `json:"key"`
	Title string   `json:"title"`
	Kind  string   `json:"kind"` // movie, show or music
	Paths []string `json:"paths"`
	On    bool     `json:"on"`
	// Count is how many movies, shows or artists the library holds; nil when the server did not say.
	Count *int `json:"count,omitempty"`
}

// countLibraries fills each library's Count, all at once; a count the
// server does not give is left out and never fails the list.
func (l *Linker) countLibraries(ctx context.Context, m domain.MediaServer, libs []Library) {
	var wg sync.WaitGroup
	for i := range libs {
		wg.Add(1)
		go func(lib *Library) {
			defer wg.Done()
			if n, ok := l.libraryCount(ctx, m, *lib); ok {
				lib.Count = &n
			}
		}(&libs[i])
	}
	wg.Wait()
}

func (l *Linker) libraryCount(ctx context.Context, m domain.MediaServer, lib Library) (int, bool) {
	if m.Kind == "plex" {
		b, err := l.do(ctx, m, http.MethodGet, "/library/sections/"+url.PathEscape(lib.Key)+"/all?X-Plex-Container-Start=0&X-Plex-Container-Size=0", nil)
		if err != nil {
			return 0, false
		}
		var mc struct {
			XMLName   xml.Name
			TotalSize *int `xml:"totalSize,attr"`
			Size      *int `xml:"size,attr"`
		}
		if xml.Unmarshal(b, &mc) != nil || mc.XMLName.Local != "MediaContainer" {
			return 0, false
		}
		if mc.TotalSize != nil {
			return *mc.TotalSize, true
		}
		return 0, false
	}
	types := map[string]string{"movie": "Movie", "show": "Series", "music": "MusicArtist"}[lib.Kind]
	b, err := l.do(ctx, m, http.MethodGet, "/Items?ParentId="+url.QueryEscape(lib.Key)+"&Recursive=true&IncludeItemTypes="+types+"&Limit=0", nil)
	if err != nil {
		return 0, false
	}
	var out struct {
		TotalRecordCount *int `json:"TotalRecordCount"`
	}
	if json.Unmarshal(b, &out) != nil || out.TotalRecordCount == nil {
		return 0, false
	}
	return *out.TotalRecordCount, true
}

// libKey starts the settings (in the links group) that keep, per media
// server id, the libraries the owner picked (their keys joined by newlines).
const libKey = "libraries:"

// Libraries lists the server's movie, TV and music libraries.
func (l *Linker) Libraries(ctx context.Context, m domain.MediaServer) ([]Library, error) {
	out := []Library{}
	switch m.Kind {
	case "plex":
		secs, err := l.plexSections(ctx, m)
		if err != nil {
			return nil, err
		}
		for _, s := range secs {
			lib := Library{Key: s.Key, Title: s.Title, Kind: s.Type, Paths: []string{}}
			if s.Type == "artist" {
				lib.Kind = "music"
			}
			for _, p := range s.Locations {
				lib.Paths = append(lib.Paths, p.Path)
			}
			out = append(out, lib)
		}
	case "jellyfin", "emby":
		b, err := l.do(ctx, m, http.MethodGet, "/Library/VirtualFolders", nil)
		if err != nil {
			return nil, err
		}
		var vf []struct {
			Name           string   `json:"Name"`
			CollectionType string   `json:"CollectionType"`
			Locations      []string `json:"Locations"`
			ItemID         string   `json:"ItemId"`
		}
		if err := json.Unmarshal(b, &vf); err != nil {
			return nil, errors.New("the server did not list its libraries")
		}
		for _, f := range vf {
			kind := map[string]string{"movies": "movie", "tvshows": "show", "music": "music"}[strings.ToLower(f.CollectionType)]
			if kind == "" || f.ItemID == "" {
				continue
			}
			paths := f.Locations
			if paths == nil {
				paths = []string{}
			}
			out = append(out, Library{Key: f.ItemID, Title: f.Name, Kind: kind, Paths: paths})
		}
	default:
		return nil, errors.New("this kind of server has no libraries to pick")
	}
	return out, nil
}

// Scan asks the server to look through the given libraries again.
func (l *Linker) Scan(ctx context.Context, m domain.MediaServer, keys []string) error {
	for _, k := range keys {
		var err error
		if m.Kind == "plex" {
			_, err = l.do(ctx, m, http.MethodGet, "/library/sections/"+url.PathEscape(k)+"/refresh", nil)
		} else {
			_, err = l.do(ctx, m, http.MethodPost, "/Items/"+url.PathEscape(k)+"/Refresh?Recursive=true", nil)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func pickedLibraries(ctx context.Context, q database.Querier, id int64) []string {
	v := database.SettingOr(ctx, q, watchGroup, libKey+strconv.FormatInt(id, 10), "")
	if v == "" {
		return nil
	}
	return strings.Split(v, "\n")
}

// libraries lists a server's libraries, each marked on when the owner
// picked it.
func (h *Handlers) libraries(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	m, err := getServer(r.Context(), h.Store.Read(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	libs, err := h.Linker.Libraries(ctx, m)
	if err != nil {
		webserver.Fail(w, http.StatusBadGateway, err.Error())
		return
	}
	h.Linker.countLibraries(ctx, m, libs)
	picked := pickedLibraries(r.Context(), h.Store.Read(), id)
	for i := range libs {
		libs[i].On = slices.Contains(picked, libs[i].Key)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"libraries": libs})
}

// pickLibraries saves which libraries are on and, with scan, asks the
// server to look through them now (in the background).
func (h *Handlers) pickLibraries(w http.ResponseWriter, r *http.Request) {
	if !webserver.RequireStrong(w, r) {
		return
	}
	id, ok := idParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Keys []string `json:"keys"`
		Scan bool     `json:"scan"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&in); err != nil || len(in.Keys) > 200 {
		webserver.Fail(w, http.StatusBadRequest, `send {"keys": ["..."], "scan": true}`)
		return
	}
	m, err := getServer(r.Context(), h.Store.Read(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	keys := []string{}
	for _, k := range in.Keys {
		k = strings.TrimSpace(k)
		if k != "" && len(k) <= 200 && !strings.Contains(k, "\n") && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	err = h.Store.Write(r.Context(), func(tx *sql.Tx) error {
		return database.PutSetting(tx, watchGroup, libKey+strconv.FormatInt(id, 10), strings.Join(keys, "\n"), actorOf(r), time.Now().UTC())
	})
	if err != nil {
		internalError(w, err)
		return
	}
	if in.Scan && len(keys) > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Minute)
			defer cancel()
			if err := h.Linker.Scan(ctx, m, keys); err != nil {
				log.Printf("mediaservers: library scan: %v", err)
			}
		}()
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"keys": keys, "scanning": in.Scan && len(keys) > 0})
}

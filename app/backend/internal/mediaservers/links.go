package mediaservers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// WatchLink is where a person opens one Library title on one media server.
type WatchLink struct {
	Server string `json:"server"` // the media server's name
	Kind   string `json:"kind"`   // jellyfin or emby
	URL    string `json:"url"`
}

// indexKeep is how long one server's list of movies or series is reused.
// A title not in a list older than indexRetry reads the list again, so a
// title that just arrived shows its button without the wait.
var (
	indexKeep  = 10 * time.Minute
	indexRetry = 30 * time.Second
)

// itemIndex is one server's movies or series by id (tmdb:1, tvdb:2,
// imdb:tt3) and the server's own id, read at a time.
type itemIndex struct {
	items    map[string]string
	serverID string
	at       time.Time
}

// watchCache keeps the item lists per server and title kind, so opening a
// title page does not read the whole library each time.
type watchCache struct {
	mu sync.Mutex
	m  map[string]itemIndex // "<server id>|movie" or "<server id>|show"
}

func (c *watchCache) get(key string) (itemIndex, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ix, ok := c.m[key]
	return ix, ok && time.Since(ix.at) < indexKeep
}

func (c *watchCache) put(key string, ix itemIndex) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]itemIndex{}
	}
	c.m[key] = ix
}

// WatchLinks are the switched-on Jellyfin and Emby servers that have the
// movie or show titleID, with the address that opens it there. Only servers
// with a browser address count. A server that fails is logged and left out,
// so one server down never hides the others.
func (l *Linker) WatchLinks(ctx context.Context, titleID int64) ([]WatchLink, error) {
	var t mediafiles.Title
	if err := mediafiles.LoadTitle(ctx, l.Store.Read(), titleID, &t); err != nil {
		return nil, err
	}
	out := []WatchLink{}
	if t.Kind != "movie" && t.Kind != "show" {
		return out, nil
	}
	ids := showIDs(t.SourceIDs)
	if len(ids) == 0 {
		return out, nil
	}
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil {
		return nil, err
	}
	for _, m := range ms {
		if m.WebURL == "" || m.TokenSecretID == nil || (m.Kind != "jellyfin" && m.Kind != "emby") {
			continue
		}
		item, sid, err := l.findItem(ctx, m, t.Kind, ids)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			log.Printf("media servers: %s: watch link: %v", m.Name, err)
			continue
		}
		if item != "" {
			out = append(out, WatchLink{Server: m.Name, Kind: m.Kind, URL: watchURL(m, item, sid)})
		}
	}
	return out, nil
}

// findItem returns the server's item id for the first of ids it knows, and
// the server's own id; an empty item id when the server lacks the title.
func (l *Linker) findItem(ctx context.Context, m domain.MediaServer, kind string, ids []string) (string, string, error) {
	key := strconv.FormatInt(m.ID, 10) + "|" + kind
	ix, fresh := l.watch.get(key)
	if fresh {
		if it := firstOf(ix.items, ids); it != "" || time.Since(ix.at) < indexRetry {
			return it, ix.serverID, nil
		}
	}
	itemType := "Movie"
	if kind == "show" {
		itemType = "Series"
	}
	items, err := l.jellyItems(ctx, m, itemType)
	if err != nil {
		return "", "", err
	}
	sid, err := l.jellyServerID(ctx, m)
	if err != nil {
		return "", "", err
	}
	l.watch.put(key, itemIndex{items: items, serverID: sid, at: time.Now()})
	return firstOf(items, ids), sid, nil
}

func firstOf(items map[string]string, ids []string) string {
	for _, id := range ids {
		if it := items[id]; it != "" {
			return it
		}
	}
	return ""
}

// jellyItems maps the ids of every item of itemType (Movie, Series) on a
// Jellyfin or Emby server to its item id, a page at a time. Jellyfin 10.11
// and later ignore AnyProviderIdEquals, so the list is read whole.
func (l *Linker) jellyItems(ctx context.Context, m domain.MediaServer, itemType string) (map[string]string, error) {
	out := map[string]string{}
	for start := 0; ; start += watchPage {
		b, err := l.do(ctx, m, http.MethodGet, "/Items?Recursive=true&IncludeItemTypes="+itemType+"&Fields=ProviderIds&EnableImages=false&EnableUserData=false"+
			"&StartIndex="+strconv.Itoa(start)+"&Limit="+strconv.Itoa(watchPage), nil)
		if err != nil {
			return nil, err
		}
		var r struct {
			Items []struct {
				ID          string            `json:"Id"`
				ProviderIDs map[string]string `json:"ProviderIds"`
			}
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, errors.New("the server's answer was not an item list")
		}
		for _, it := range r.Items {
			for k, v := range it.ProviderIDs {
				if k = strings.ToLower(k); v != "" && (k == "tvdb" || k == "tmdb" || k == "imdb") {
					out[k+":"+v] = it.ID
				}
			}
		}
		if len(r.Items) < watchPage {
			return out, nil
		}
	}
}

// jellyServerID is the server's own id, which its web app wants in a link.
func (l *Linker) jellyServerID(ctx context.Context, m domain.MediaServer) (string, error) {
	b, err := l.do(ctx, m, http.MethodGet, "/System/Info", nil)
	if err != nil {
		return "", err
	}
	var info struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(b, &info); err != nil || info.ID == "" {
		return "", errors.New("the server's answer was not its system info")
	}
	return info.ID, nil
}

// watchURL is the web app page of item: Jellyfin's details page, or
// Emby's item page.
func watchURL(m domain.MediaServer, item, serverID string) string {
	q := "id=" + url.QueryEscape(item) + "&serverId=" + url.QueryEscape(serverID)
	if m.Kind == "emby" {
		return m.WebURL + "/web/index.html#!/item?" + q
	}
	return m.WebURL + "/web/#/details?" + q
}

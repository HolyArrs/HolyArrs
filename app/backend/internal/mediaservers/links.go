package mediaservers

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// WatchLink is where one Library title opens on one media server.
type WatchLink struct {
	Server string `json:"server"` // the media server's name
	Kind   string `json:"kind"`   // plex, jellyfin or emby
	URL    string `json:"url"`
}

// A server's movie or series list is reused for indexKeep. A title missing
// from a list older than indexRetry reads the list again, so a title that
// just arrived gets its button without waiting for the list to expire.
var (
	indexKeep  = 10 * time.Minute
	indexRetry = 30 * time.Second
)

// itemIndex is one server's movies or series by id (tmdb:1, tvdb:2,
// imdb:tt3), with the server's own id and when they were read.
type itemIndex struct {
	items    map[string]string
	serverID string
	at       time.Time
}

// watchCache keeps the lists per server and title kind, so opening a title
// page does not read the whole library every time.
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

// WatchLinks are the switched-on servers that have the movie or show
// titleID, each with the link that opens it there: Plex always (in Plex's
// own web app at app.plex.tv, as Seerr does), Jellyfin and Emby once they
// have a browser address. A server that fails is logged and left out, so
// one server down never hides the others.
func (l *Linker) WatchLinks(ctx context.Context, titleID int64) ([]WatchLink, error) {
	var t mediafiles.Title
	if err := mediafiles.LoadTitle(ctx, l.Store.Read(), titleID, &t); err != nil {
		return nil, err
	}
	out := []WatchLink{}
	ids := showIDs(t.SourceIDs)
	if (t.Kind != "movie" && t.Kind != "show") || len(ids) == 0 {
		return out, nil
	}
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil {
		return nil, err
	}
	shown := map[string]bool{}
	for _, m := range ms {
		if m.Kind != "plex" {
			m.WebURL = browserAddress(m)
		}
		if m.TokenSecretID == nil || (m.Kind != "plex" && m.WebURL == "") || (m.Kind != "plex" && m.Kind != "jellyfin" && m.Kind != "emby") {
			continue
		}
		item, sid, err := l.findItem(ctx, m, t.Kind, ids)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			log.Printf("media servers: %s: open link: %v", m.Name, err)
			continue
		}
		// Two rows for one server give the same link; keep the first (#717).
		if u := watchURL(m, item, sid); item != "" && !shown[u] {
			shown[u] = true
			out = append(out, WatchLink{Server: m.Name, Kind: m.Kind, URL: u})
		}
	}
	return out, nil
}

// findItem returns the server's item id for the first of ids it knows (empty
// when it lacks the title) and the server's own id.
func (l *Linker) findItem(ctx context.Context, m domain.MediaServer, kind string, ids []string) (string, string, error) {
	key := strconv.FormatInt(m.ID, 10) + "|" + kind
	if ix, fresh := l.watch.get(key); fresh {
		if it := firstOf(ix.items, ids); it != "" || time.Since(ix.at) < indexRetry {
			return it, ix.serverID, nil
		}
	}
	var items map[string]string
	var sid string
	var err error
	if m.Kind == "plex" {
		items, err = l.plexKeys(ctx, m, kind)
		if err == nil {
			sid, err = l.plexMachineID(ctx, m)
		}
	} else {
		itemType := "Movie"
		if kind == "show" {
			itemType = "Series"
		}
		items, err = l.jellyItems(ctx, m, itemType)
		if err == nil {
			sid, err = l.jellyServerID(ctx, m)
		}
	}
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

// plexKeys maps the ids of every movie (or show) in the server's movie (or
// TV) libraries to its ratingKey, a page at a time.
func (l *Linker) plexKeys(ctx context.Context, m domain.MediaServer, kind string) (map[string]string, error) {
	if kind == "show" {
		return l.plexShows(ctx, m)
	}
	secs, err := l.plexSections(ctx, m)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, s := range secs {
		if s.Type != "movie" {
			continue
		}
		for start := 0; ; start += watchPage {
			b, err := l.do(ctx, m, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/all?type=1&includeGuids=1"+plexRange(start), nil)
			if err != nil {
				return nil, err
			}
			var mc struct {
				Videos []struct {
					RatingKey string `xml:"ratingKey,attr"`
					GUID      string `xml:"guid,attr"`
					Guids     []struct {
						ID string `xml:"id,attr"`
					} `xml:"Guid"`
				} `xml:"Video"`
			}
			if err := xml.Unmarshal(b, &mc); err != nil {
				return nil, errors.New("the server's answer was not a Plex movie list")
			}
			for _, v := range mc.Videos {
				gs := []string{v.GUID}
				for _, g := range v.Guids {
					gs = append(gs, g.ID)
				}
				for _, g := range gs {
					if id := plexGUID(g); id != "" {
						out[id] = v.RatingKey
					}
				}
			}
			if len(mc.Videos) < watchPage {
				break
			}
		}
	}
	return out, nil
}

// plexMachineID is the Plex server's machine identifier, which Plex's web
// app wants in a link.
func (l *Linker) plexMachineID(ctx context.Context, m domain.MediaServer) (string, error) {
	b, err := l.do(ctx, m, http.MethodGet, "/identity", nil)
	if err != nil {
		return "", err
	}
	var mc struct {
		ID string `xml:"machineIdentifier,attr"`
	}
	if err := xml.Unmarshal(b, &mc); err != nil || mc.ID == "" {
		return "", errors.New("the server's answer was not its Plex identity")
	}
	return mc.ID, nil
}

// watchURL is item's page in the server's web app: Plex's details page
// (at app.plex.tv, or the server's own web app when it has a browser
// address), Jellyfin's details page or Emby's item page.
func watchURL(m domain.MediaServer, item, serverID string) string {
	if m.Kind == "plex" {
		web := "https://app.plex.tv/desktop"
		if m.WebURL != "" {
			web = m.WebURL + "/web/index.html"
		}
		return web + "#!/server/" + url.PathEscape(serverID) + "/details?key=" + url.QueryEscape("/library/metadata/"+item)
	}
	q := "id=" + url.QueryEscape(item) + "&serverId=" + url.QueryEscape(serverID)
	if m.Kind == "emby" {
		return m.WebURL + "/web/index.html#!/item?" + q
	}
	return m.WebURL + "/web/#/details?" + q
}

// browserAddress is where browsers open a Jellyfin or Emby server: its
// browser address, else its connection address when a browser elsewhere can
// reach that too (an IP that is not loopback, or a dotted name). Docker-only
// names like http://jellyfin:8096 and localhost give none, so no button.
func browserAddress(m domain.MediaServer) string {
	if m.WebURL != "" {
		return m.WebURL
	}
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(m.URL), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(h); ip != nil {
		if ip.IsLoopback() || ip.IsUnspecified() {
			return ""
		}
	} else if !strings.Contains(strings.Trim(h, "."), ".") || strings.HasSuffix(h, ".localhost") {
		return ""
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/")
}

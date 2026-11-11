package mediaservers

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Advancer widens a show's partial season packs given its watched episodes
// (episode item ids); downloads.Manager.Advance.
type Advancer func(ctx context.Context, titleID int64, watched map[int64]bool, by domain.Actor) error

// watchedOn is what one server knows for a pass: its shows by id (tvdb:1,
// tmdb:2, imdb:tt3) and, for Jellyfin and Emby, its users.
type watchedOn struct {
	m      domain.MediaServer
	shows  map[string]string // show id -> Plex ratingKey or Jellyfin series id
	users  []string
	failed bool
	// history holds the episodes every Plex account watched, by show ratingKey.
	history map[string][][2]int
}

// watchPage is how many entries one list call asks a server for, so a large
// library is read a page at a time.
var watchPage = 500

// AdvancePacks asks every switched-on media server which episodes of each
// show in titles anyone has watched and hands them to advance. Only reads;
// a server that fails is logged once and skipped for the rest of the pass.
func (l *Linker) AdvancePacks(ctx context.Context, titles []int64, advance Advancer) error {
	if len(titles) == 0 {
		return nil
	}
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil || len(ms) == 0 {
		return err
	}
	servers := make([]*watchedOn, 0, len(ms))
	for _, m := range ms {
		if m.TokenSecretID != nil {
			servers = append(servers, &watchedOn{m: m})
		}
	}
	var errs []error
	for _, id := range titles {
		var t mediafiles.Title
		if err := mediafiles.LoadTitle(ctx, l.Store.Read(), id, &t); err != nil {
			errs = append(errs, err)
			continue
		}
		eps, err := mediafiles.EpisodesOfTitle(ctx, l.Store.Read(), id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		byNum := map[[2]int]int64{}
		for _, e := range eps {
			if e.Numbers.Season != nil && e.Numbers.Episode != nil {
				byNum[[2]int{*e.Numbers.Season, *e.Numbers.Episode}] = e.ID
			}
		}
		watched := map[int64]bool{}
		for _, w := range servers {
			if w.failed {
				continue
			}
			nums, err := l.watchedEpisodes(ctx, w, showIDs(t.SourceIDs))
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				w.failed = true
				log.Printf("media servers: %s: watched episodes: %v", w.m.Name, err)
				continue
			}
			for _, n := range nums {
				if it, ok := byNum[n]; ok {
					watched[it] = true
				}
			}
		}
		if len(watched) > 0 {
			if err := advance(ctx, id, watched, domain.System); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// showIDs are a title's ids as the servers name them.
func showIDs(src map[string]string) []string {
	var out []string
	for _, k := range []string{"tvdb", "tmdb", "imdb"} {
		if v := src[k]; v != "" {
			out = append(out, k+":"+v)
		}
	}
	return out
}

// watchedEpisodes returns the season and episode numbers of the show (found
// by any of ids) that anyone on the server has watched.
func (l *Linker) watchedEpisodes(ctx context.Context, w *watchedOn, ids []string) ([][2]int, error) {
	if w.shows == nil {
		var err error
		if w.m.Kind == "plex" {
			w.shows, err = l.plexShows(ctx, w.m)
		} else {
			w.shows, w.users, err = l.jellyShows(ctx, w.m)
		}
		if err != nil {
			return nil, err
		}
	}
	key := ""
	for _, id := range ids {
		if key = w.shows[id]; key != "" {
			break
		}
	}
	if key == "" {
		return nil, nil
	}
	if w.m.Kind == "plex" {
		if w.history == nil {
			var err error
			if w.history, err = l.plexHistory(ctx, w.m); err != nil {
				// Only the token's own views count then.
				log.Printf("media servers: %s: watch history of every account: %v", w.m.Name, err)
				w.history = map[string][][2]int{}
			}
		}
		out, err := l.plexWatched(ctx, w.m, key)
		return append(out, w.history[key]...), err
	}
	seen := map[[2]int]bool{}
	var out [][2]int
	for _, u := range w.users {
		b, err := l.do(ctx, w.m, http.MethodGet, "/Users/"+url.PathEscape(u)+"/Items?Recursive=true&IncludeItemTypes=Episode&IsPlayed=true&ParentId="+url.QueryEscape(key), nil)
		if err != nil {
			return nil, err
		}
		var r struct {
			Items []struct {
				ParentIndexNumber *int
				IndexNumber       *int
			}
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, errors.New("the server's answer was not an episode list")
		}
		for _, it := range r.Items {
			if it.ParentIndexNumber != nil && it.IndexNumber != nil {
				n := [2]int{*it.ParentIndexNumber, *it.IndexNumber}
				if !seen[n] {
					seen[n] = true
					out = append(out, n)
				}
			}
		}
	}
	return out, nil
}

// plexShows maps the ids of every show in the server's TV libraries to its
// ratingKey (new agents' Guid tags and old agents' guid attribute).
func (l *Linker) plexShows(ctx context.Context, m domain.MediaServer) (map[string]string, error) {
	secs, err := l.plexSections(ctx, m)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, s := range secs {
		if s.Type != "show" {
			continue
		}
		for start := 0; ; start += watchPage {
			b, err := l.do(ctx, m, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/all?type=2&includeGuids=1"+plexRange(start), nil)
			if err != nil {
				return nil, err
			}
			var mc struct {
				Directories []struct {
					RatingKey string `xml:"ratingKey,attr"`
					GUID      string `xml:"guid,attr"`
					Guids     []struct {
						ID string `xml:"id,attr"`
					} `xml:"Guid"`
				} `xml:"Directory"`
			}
			if err := xml.Unmarshal(b, &mc); err != nil {
				return nil, errors.New("the server's answer was not a Plex show list")
			}
			for _, d := range mc.Directories {
				gs := []string{d.GUID}
				for _, g := range d.Guids {
					gs = append(gs, g.ID)
				}
				for _, g := range gs {
					if id := plexGUID(g); id != "" {
						out[id] = d.RatingKey
					}
				}
			}
			if len(mc.Directories) < watchPage {
				break
			}
		}
	}
	return out, nil
}

// plexRange asks Plex for one page of a list from start.
func plexRange(start int) string {
	return "&X-Plex-Container-Start=" + strconv.Itoa(start) + "&X-Plex-Container-Size=" + strconv.Itoa(watchPage)
}

// plexHistoryPath is Plex's watch history of every account, a server path
// (kept without its leading slash, apart from page addresses).
const plexHistoryPath = "status/sessions/history/all"

// plexHistory reads the watch history of every account on the server (the
// admin token may read it) and keeps the episodes by show ratingKey.
func (l *Linker) plexHistory(ctx context.Context, m domain.MediaServer) (map[string][][2]int, error) {
	out := map[string][][2]int{}
	for start := 0; ; start += watchPage {
		b, err := l.do(ctx, m, http.MethodGet, "/"+plexHistoryPath+"?sort=viewedAt:desc"+plexRange(start), nil)
		if err != nil {
			return nil, err
		}
		var mc struct {
			Videos []struct {
				Show    string `xml:"grandparentKey,attr"`
				Season  string `xml:"parentIndex,attr"`
				Episode string `xml:"index,attr"`
			} `xml:"Video"`
		}
		if err := xml.Unmarshal(b, &mc); err != nil {
			return nil, errors.New("the server's answer was not a Plex watch history")
		}
		for _, v := range mc.Videos {
			key, ok := strings.CutPrefix(v.Show, "/library/metadata/")
			s, err1 := strconv.Atoi(v.Season)
			e, err2 := strconv.Atoi(v.Episode)
			if ok && err1 == nil && err2 == nil {
				out[key] = append(out[key], [2]int{s, e})
			}
		}
		if len(mc.Videos) < watchPage {
			return out, nil
		}
	}
}

// plexGUID turns tvdb://1, tmdb://2, imdb://tt3 or an old agent's
// com.plexapp.agents.thetvdb://1?lang=en into tvdb:1 (and so on).
func plexGUID(g string) string {
	scheme, rest, ok := strings.Cut(g, "://")
	if !ok {
		return ""
	}
	rest, _, _ = strings.Cut(rest, "?")
	rest, _, _ = strings.Cut(rest, "/")
	switch {
	case scheme == "tvdb" || strings.HasSuffix(scheme, ".thetvdb"):
		return "tvdb:" + rest
	case scheme == "tmdb" || strings.HasSuffix(scheme, ".themoviedb"):
		return "tmdb:" + rest
	case scheme == "imdb" || strings.HasSuffix(scheme, ".imdb"):
		return "imdb:" + rest
	}
	return ""
}

// plexWatched lists the show's episodes with a view count (the admin
// token's own watched state; plexHistory adds the other accounts).
func (l *Linker) plexWatched(ctx context.Context, m domain.MediaServer, key string) ([][2]int, error) {
	b, err := l.do(ctx, m, http.MethodGet, "/library/metadata/"+url.PathEscape(key)+"/allLeaves", nil)
	if err != nil {
		return nil, err
	}
	var mc struct {
		Videos []struct {
			Season    string `xml:"parentIndex,attr"`
			Episode   string `xml:"index,attr"`
			ViewCount int    `xml:"viewCount,attr"`
		} `xml:"Video"`
	}
	if err := xml.Unmarshal(b, &mc); err != nil {
		return nil, errors.New("the server's answer was not a Plex episode list")
	}
	var out [][2]int
	for _, v := range mc.Videos {
		s, err1 := strconv.Atoi(v.Season)
		e, err2 := strconv.Atoi(v.Episode)
		if v.ViewCount > 0 && err1 == nil && err2 == nil {
			out = append(out, [2]int{s, e})
		}
	}
	return out, nil
}

// jellyShows maps the ids of every series on a Jellyfin or Emby server to
// its item id, and lists the server's users.
func (l *Linker) jellyShows(ctx context.Context, m domain.MediaServer) (map[string]string, []string, error) {
	out := map[string]string{}
	for start := 0; ; start += watchPage {
		b, err := l.do(ctx, m, http.MethodGet, "/Items?Recursive=true&IncludeItemTypes=Series&Fields=ProviderIds&EnableImages=false&EnableUserData=false"+
			"&StartIndex="+strconv.Itoa(start)+"&Limit="+strconv.Itoa(watchPage), nil)
		if err != nil {
			return nil, nil, err
		}
		var r struct {
			Items []struct {
				ID          string            `json:"Id"`
				ProviderIDs map[string]string `json:"ProviderIds"`
			}
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, nil, errors.New("the server's answer was not a series list")
		}
		for _, it := range r.Items {
			for k, v := range it.ProviderIDs {
				if k = strings.ToLower(k); v != "" && (k == "tvdb" || k == "tmdb" || k == "imdb") {
					out[k+":"+v] = it.ID
				}
			}
		}
		if len(r.Items) < watchPage {
			break
		}
	}
	b, err := l.do(ctx, m, http.MethodGet, "/Users", nil)
	if err != nil {
		return nil, nil, err
	}
	var us []struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(b, &us); err != nil {
		return nil, nil, errors.New("the server's answer was not a user list")
	}
	users := make([]string, 0, len(us))
	for _, u := range us {
		users = append(users, u.ID)
	}
	return out, users, nil
}

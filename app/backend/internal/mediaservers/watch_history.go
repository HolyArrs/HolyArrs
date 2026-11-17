package mediaservers

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Watch history: what every account on Plex (the owner token's history of
// every account, as Tautulli reads it), Jellyfin and Emby watched, for the
// Library titles. Read on a schedule only while the owner keeps it on, and
// never sent to /live, Activity, alerts or Job results.

// watchStateKey is one row of a pass, before plays are added up.
type watchStateKey struct {
	account         string
	title           int64
	season, episode int
}

// watchPass adds up one server's answers into rows.
type watchPass struct {
	names map[string]string
	rows  map[watchStateKey]*watchRow
}

func newWatchPass() *watchPass {
	return &watchPass{names: map[string]string{}, rows: map[watchStateKey]*watchRow{}}
}

func (p *watchPass) add(account string, title int64, season, episode, plays int, at *time.Time) {
	k := watchStateKey{account, title, season, episode}
	r := p.rows[k]
	if r == nil {
		kind := "movie"
		if season != 0 || episode != 0 {
			kind = "episode"
		}
		r = &watchRow{account: account, title: title, kind: kind, season: season, episode: episode}
		p.rows[k] = r
	}
	r.plays += max(plays, 1)
	if at != nil && (r.last == nil || at.After(*r.last)) {
		r.last = at
	}
}

func (p *watchPass) list() []watchRow {
	out := make([]watchRow, 0, len(p.rows))
	for _, r := range p.rows {
		r.name = p.names[r.account]
		out = append(out, *r)
	}
	return out
}

// ReadWatchHistory reads every switched-on media server's watch history and
// keeps it in watch_state, a whole pass per server: a server with a failed
// or too-big page keeps the rows of its last whole pass.
func (l *Linker) ReadWatchHistory(ctx context.Context) error {
	l.reading.Lock()
	defer l.reading.Unlock()
	if !WatchHistoryOn(ctx, l.Store.Read()) {
		return nil
	}
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil || len(ms) == 0 {
		return err
	}
	keys, err := mediafiles.TitlesBySourceID(ctx, l.Store.Read())
	if err != nil {
		return err
	}
	var errs []error
	for _, m := range ms {
		if m.TokenSecretID == nil {
			continue
		}
		var p *watchPass
		if m.Kind == "plex" {
			p, err = l.plexWatchPass(ctx, m, keys)
		} else {
			p, err = l.jellyWatchPass(ctx, m, keys)
		}
		if err == nil {
			err = replaceWatchState(ctx, l.Store, m.ID, p.list(), time.Now().UTC())
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("media servers: %s: watch history: %v", m.Name, err)
			errs = append(errs, fmt.Errorf("%s: %w", m.Name, err))
		}
	}
	return errors.Join(errs...)
}

// ReadWatchHistorySoon reads the watch history now, in the background, so
// turning it on or adding a media server does not wait for the next
// scheduled read. Stop ends it; Wait waits for it.
func (l *Linker) ReadWatchHistorySoon() {
	if l.Paused {
		return
	}
	l.mu.Lock()
	if l.stopRead != nil {
		l.stopRead() // a newer read replaces one still going
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	l.stopRead = cancel
	l.wg.Add(1)
	l.mu.Unlock()
	go func() {
		defer l.wg.Done()
		defer cancel()
		jobs.Guard("media_servers", func() {
			if err := l.ReadWatchHistory(ctx); err != nil && ctx.Err() == nil {
				log.Printf("media servers: watch history: %v", err)
			}
		})
	}()
}

// Watched sums what each account watched, most plays first. An account
// linked to a person here carries that person; any other keeps only its
// media-server name.
func Watched(ctx context.Context, q watchReader) (WatchedSummary, error) {
	out, err := watchedRows(ctx, q)
	if err != nil {
		return out, err
	}
	links := map[string]map[string]accounts.LinkedPerson{}
	for i := range out.People {
		p := &out.People[i]
		byID, ok := links[p.ServerKind]
		if !ok {
			if byID, err = accounts.LinkedPeople(ctx, q, p.ServerKind); err != nil {
				return out, err
			}
			links[p.ServerKind] = byID
		}
		lp, ok := byID[p.account]
		if !ok && p.ServerKind == "plex" && p.account == "1" {
			// The server's own owner is account 1 there, not the plex.tv id
			// the link keeps; their Plex name still matches.
			lp, ok = byPlexName(byID, p.Name)
		}
		if ok {
			p.Linked, p.UserID, p.UserName = true, &lp.ID, lp.Name
		}
	}
	return out, nil
}

// byPlexName is the one Plex link with this Plex name.
func byPlexName(links map[string]accounts.LinkedPerson, name string) (accounts.LinkedPerson, bool) {
	var found accounts.LinkedPerson
	n := 0
	for _, lp := range links {
		if name != "" && strings.EqualFold(lp.ExtName, name) {
			found, n = lp, n+1
		}
	}
	return found, n == 1
}

// plexWatchPass reads the server's accounts, its movie and show keys and the
// history of every account, a page at a time.
func (l *Linker) plexWatchPass(ctx context.Context, m domain.MediaServer, keys map[string]int64) (*watchPass, error) {
	p := newWatchPass()
	b, err := l.do(ctx, m, http.MethodGet, "/accounts", nil)
	if err != nil {
		return nil, err
	}
	var ac struct {
		Accounts []struct {
			ID   string `xml:"id,attr"`
			Name string `xml:"name,attr"`
		} `xml:"Account"`
	}
	if err := xml.Unmarshal(b, &ac); err != nil {
		return nil, errors.New("the server's answer was not a Plex account list")
	}
	for _, a := range ac.Accounts {
		p.names[a.ID] = a.Name
	}
	movies, shows, err := l.plexTitleKeys(ctx, m, keys)
	if err != nil {
		return nil, err
	}
	for start := 0; ; start += watchPage {
		b, err := l.do(ctx, m, http.MethodGet, "/"+plexHistoryPath+"?sort=viewedAt:desc"+plexRange(start), nil)
		if err != nil {
			return nil, err
		}
		var mc struct {
			Videos []struct {
				Type      string `xml:"type,attr"`
				RatingKey string `xml:"ratingKey,attr"`
				Key       string `xml:"key,attr"`
				Show      string `xml:"grandparentKey,attr"`
				Season    string `xml:"parentIndex,attr"`
				Episode   string `xml:"index,attr"`
				Account   string `xml:"accountID,attr"`
				ViewedAt  int64  `xml:"viewedAt,attr"`
			} `xml:"Video"`
		}
		if err := xml.Unmarshal(b, &mc); err != nil {
			return nil, errors.New("the server's answer was not a Plex watch history")
		}
		for _, v := range mc.Videos {
			var at *time.Time
			if v.ViewedAt > 0 {
				t := time.Unix(v.ViewedAt, 0).UTC()
				at = &t
			}
			switch v.Type {
			case "movie":
				rk := v.RatingKey
				if rk == "" {
					rk = strings.TrimPrefix(v.Key, "/library/metadata/")
				}
				if id := movies[rk]; id != 0 && v.Account != "" {
					p.add(v.Account, id, 0, 0, 1, at)
				}
			case "episode":
				s, err1 := strconv.Atoi(v.Season)
				e, err2 := strconv.Atoi(v.Episode)
				id := shows[strings.TrimPrefix(v.Show, "/library/metadata/")]
				if id != 0 && v.Account != "" && err1 == nil && err2 == nil && e > 0 {
					p.add(v.Account, id, s, e, 1, at)
				}
			}
		}
		if len(mc.Videos) < watchPage {
			return p, nil
		}
	}
}

// plexTitleKeys maps the ratingKey of every movie and show on the server to
// its Library title, by the ids in its Guid tags (and old agents' guid).
func (l *Linker) plexTitleKeys(ctx context.Context, m domain.MediaServer, keys map[string]int64) (movies, shows map[string]int64, err error) {
	secs, err := l.plexSections(ctx, m)
	if err != nil {
		return nil, nil, err
	}
	movies, shows = map[string]int64{}, map[string]int64{}
	for _, s := range secs {
		kind, typ, into := "movie", "1", movies
		switch s.Type {
		case "movie":
		case "show":
			kind, typ, into = "show", "2", shows
		default:
			continue
		}
		for start := 0; ; start += watchPage {
			b, err := l.do(ctx, m, http.MethodGet, "/library/sections/"+url.PathEscape(s.Key)+"/all?type="+typ+"&includeGuids=1"+plexRange(start), nil)
			if err != nil {
				return nil, nil, err
			}
			type entry struct {
				RatingKey string `xml:"ratingKey,attr"`
				GUID      string `xml:"guid,attr"`
				Guids     []struct {
					ID string `xml:"id,attr"`
				} `xml:"Guid"`
			}
			var mc struct {
				Videos      []entry `xml:"Video"`
				Directories []entry `xml:"Directory"`
			}
			if err := xml.Unmarshal(b, &mc); err != nil {
				return nil, nil, errors.New("the server's answer was not a Plex library list")
			}
			list := append(mc.Videos, mc.Directories...)
			for _, d := range list {
				gs := []string{d.GUID}
				for _, g := range d.Guids {
					gs = append(gs, g.ID)
				}
				for _, g := range gs {
					if id := keys[kind+"|"+plexGUID(g)]; id != 0 {
						into[d.RatingKey] = id
						break
					}
				}
			}
			if len(list) < watchPage {
				break
			}
		}
	}
	return movies, shows, nil
}

// jellyItem is one played movie or episode in a Jellyfin or Emby answer.
type jellyItem struct {
	Type              string            `json:"Type"`
	ProviderIDs       map[string]string `json:"ProviderIds"`
	SeriesID          string            `json:"SeriesId"`
	ParentIndexNumber *int
	IndexNumber       *int
	UserData          struct {
		PlayCount      int    `json:"PlayCount"`
		LastPlayedDate string `json:"LastPlayedDate"`
	} `json:"UserData"`
}

// jellyWatchPass reads a Jellyfin or Emby server's series, its users and
// each user's played movies and episodes, a page at a time.
func (l *Linker) jellyWatchPass(ctx context.Context, m domain.MediaServer, keys map[string]int64) (*watchPass, error) {
	series, _, err := l.jellyShows(ctx, m)
	if err != nil {
		return nil, err
	}
	seriesTitle := map[string]int64{}
	for id, sid := range series {
		if t := keys["show|"+id]; t != 0 {
			seriesTitle[sid] = t
		}
	}
	b, err := l.do(ctx, m, http.MethodGet, "/Users", nil)
	if err != nil {
		return nil, err
	}
	var us []struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(b, &us); err != nil {
		return nil, errors.New("the server's answer was not a user list")
	}
	p := newWatchPass()
	for _, u := range us {
		p.names[u.ID] = u.Name
		for start := 0; ; start += watchPage {
			b, err := l.do(ctx, m, http.MethodGet, "/Users/"+url.PathEscape(u.ID)+"/Items?Recursive=true&IncludeItemTypes=Movie,Episode&IsPlayed=true&Fields=ProviderIds&EnableImages=false"+
				"&StartIndex="+strconv.Itoa(start)+"&Limit="+strconv.Itoa(watchPage), nil)
			if err != nil {
				return nil, err
			}
			var r struct{ Items []jellyItem }
			if err := json.Unmarshal(b, &r); err != nil {
				return nil, errors.New("the server's answer was not a played list")
			}
			for _, it := range r.Items {
				var at *time.Time
				if t, err := time.Parse(time.RFC3339Nano, it.UserData.LastPlayedDate); err == nil {
					t = t.UTC()
					at = &t
				}
				switch it.Type {
				case "Movie":
					for k, v := range it.ProviderIDs {
						if id := keys["movie|"+strings.ToLower(k)+":"+v]; id != 0 && v != "" {
							p.add(u.ID, id, 0, 0, it.UserData.PlayCount, at)
							break
						}
					}
				case "Episode":
					if id := seriesTitle[it.SeriesID]; id != 0 && it.ParentIndexNumber != nil && it.IndexNumber != nil && *it.IndexNumber > 0 {
						p.add(u.ID, id, *it.ParentIndexNumber, *it.IndexNumber, it.UserData.PlayCount, at)
					}
				}
			}
			if len(r.Items) < watchPage {
				break
			}
		}
	}
	return p, nil
}

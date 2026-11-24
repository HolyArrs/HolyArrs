package mediaservers

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// traktWatched is one movie or show in Trakt's watched lists.
type traktWatched struct {
	Plays         int    `json:"plays"`
	LastWatchedAt string `json:"last_watched_at"`
	Movie         *struct {
		IDs traktIDs `json:"ids"`
	} `json:"movie"`
	Show *struct {
		IDs traktIDs `json:"ids"`
	} `json:"show"`
	Seasons []struct {
		Number   int `json:"number"`
		Episodes []struct {
			Number        int    `json:"number"`
			Plays         int    `json:"plays"`
			LastWatchedAt string `json:"last_watched_at"`
		} `json:"episodes"`
	} `json:"seasons"`
}

// traktIDs are a Trakt title's ids; Trakt sends numbers for TMDB and TVDB.
type traktIDs struct {
	Trakt int64  `json:"trakt"`
	TMDB  int64  `json:"tmdb"`
	TVDB  int64  `json:"tvdb"`
	IMDB  string `json:"imdb"`
}

// traktProgress is a show's watched progress: Trakt's watched shows list
// no longer carries the seasons, so each Library show's episodes come from
// here.
type traktProgress struct {
	Seasons []struct {
		Number   int `json:"number"`
		Episodes []struct {
			Number        int    `json:"number"`
			Completed     bool   `json:"completed"`
			LastWatchedAt string `json:"last_watched_at"`
			Stats         struct {
				PlayCount int `json:"play_count"`
			} `json:"stats"`
		} `json:"episodes"`
	} `json:"seasons"`
}

// ref is how Trakt's show routes name the show: its Trakt id, else IMDb.
func (ids traktIDs) ref() string {
	if ids.Trakt > 0 {
		return strconv.FormatInt(ids.Trakt, 10)
	}
	return url.PathEscape(ids.IMDB)
}

// title finds the Library title of kind with one of the ids.
func (ids traktIDs) title(keys map[string]int64, kind string) int64 {
	for _, k := range []struct{ name, v string }{{"tmdb", num(ids.TMDB)}, {"tvdb", num(ids.TVDB)}, {"imdb", ids.IMDB}} {
		if k.v == "" {
			continue
		}
		if id := keys[kind+"|"+k.name+":"+k.v]; id != 0 {
			return id
		}
	}
	return 0
}

func num(n int64) string {
	if n <= 0 {
		return ""
	}
	return strconv.FormatInt(n, 10)
}

func traktTime(s string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}

// readTraktHistory reads what the person signed in with Trakt watched there
// (Trakt's watched movies and shows) and keeps it for the Library titles,
// a whole pass at a time: a failed read keeps the last whole pass. A
// sign-in Trakt ended drops the Trakt rows.
func (l *Linker) readTraktHistory(ctx context.Context, person int64, keys map[string]int64) error {
	var movies, shows []traktWatched
	err := l.Trakt.Read(ctx, "/sync/watched/movies", &movies)
	if err == nil {
		err = l.Trakt.Read(ctx, "/sync/watched/shows", &shows)
	}
	if errors.Is(err, metadata.ErrTraktSignedOut) {
		if cerr := clearTraktWatchState(ctx, l.Store); cerr != nil {
			return cerr
		}
	}
	if err != nil {
		return err
	}
	p := newWatchPass()
	for _, w := range movies {
		if w.Movie == nil {
			continue
		}
		if id := w.Movie.IDs.title(keys, "movie"); id != 0 {
			p.add("", id, 0, 0, w.Plays, traktTime(w.LastWatchedAt))
		}
	}
	for _, w := range shows {
		if w.Show == nil {
			continue
		}
		id := w.Show.IDs.title(keys, "show")
		if id == 0 {
			continue
		}
		if len(w.Seasons) > 0 {
			for _, s := range w.Seasons {
				for _, e := range s.Episodes {
					if e.Number > 0 {
						p.add("", id, s.Number, e.Number, e.Plays, traktTime(e.LastWatchedAt))
					}
				}
			}
			continue
		}
		ref := w.Show.IDs.ref()
		if ref == "" {
			continue
		}
		var pr traktProgress
		err := l.Trakt.Read(ctx, "/shows/"+ref+"/progress/watched?specials=true", &pr)
		if errors.Is(err, metadata.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		for _, s := range pr.Seasons {
			for _, e := range s.Episodes {
				if e.Number <= 0 || !e.Completed {
					continue
				}
				p.add("", id, s.Number, e.Number, max(e.Stats.PlayCount, 1), traktTime(e.LastWatchedAt))
			}
		}
	}
	return replaceTraktWatchState(ctx, l.Store, person, l.Trakt.Status(ctx).Username, p.list(), time.Now().UTC())
}

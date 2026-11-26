package mediaservers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The clean-up list only suggests: each title on it is removed through the
// title's own delete sheet, which the owner confirms. Nothing here deletes.

// cleanupDays is how long ago the last of the chosen people must have
// finished a title when the owner sends no number.
const cleanupDays = 30

// CleanupPerson is one account on a media server the list can wait for.
type CleanupPerson struct {
	// Key is "<server id>:<account id>", sent back in people= to choose.
	Key    string `json:"key"`
	Name   string `json:"name"`
	Server string `json:"server"`
	Chosen bool   `json:"chosen"`
}

// WatchedTitle is a title every chosen person finished: a suggestion to
// remove, never removed here.
type WatchedTitle struct {
	ID            int64  `json:"id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Year          int    `json:"year,omitempty"`
	Bytes         int64  `json:"bytes"`
	LastWatchedAt string `json:"last_watched_at"`
}

// WatchedCleanup is the clean-up list: titles with files that every chosen
// person finished at least Days ago, biggest first. A title somebody asked
// for stays off it until each person who asked finished it too, and a
// title with a protected copy never shows.
type WatchedCleanup struct {
	Enabled bool            `json:"enabled"`
	Days    int             `json:"days"`
	People  []CleanupPerson `json:"people"`
	Found   []WatchedTitle  `json:"found"`
}

// cleanupTitle is a movie or show with files, as the list judges it.
type cleanupTitle struct {
	id     int64
	kind   string
	name   string
	year   int
	bytes  int64
	eps    []seenKey // a show's episodes with files
	askers []int64   // people who asked for it
}

// seenKey is one movie (season and episode 0) or episode of a title.
type seenKey struct {
	title           int64
	season, episode int
}

// finishedAt says when the account finished the title: a movie when it was
// watched, a show when every episode with a file was, at the latest of them.
func finishedAt(t cleanupTitle, seen map[seenKey]time.Time) (time.Time, bool) {
	if t.kind == "movie" {
		at, ok := seen[seenKey{title: t.id}]
		return at, ok
	}
	if len(t.eps) == 0 {
		return time.Time{}, false
	}
	var last time.Time
	for _, e := range t.eps {
		at, ok := seen[e]
		if !ok {
			return time.Time{}, false
		}
		if at.After(last) {
			last = at
		}
	}
	return last, true
}

// pickWatched keeps the titles every chosen account finished no later than
// cutoff, whose askers each finished them on one of their own accounts. An
// asker with no account here keeps the title off the list: nobody can tell
// they watched it. The order of titles is kept.
func pickWatched(titles []cleanupTitle, seen map[watchAccount]map[seenKey]time.Time, chosen []watchAccount, askers map[int64][]watchAccount, cutoff time.Time) []WatchedTitle {
	out := []WatchedTitle{}
	if len(chosen) == 0 {
		return out
	}
next:
	for _, t := range titles {
		var last time.Time
		for _, a := range chosen {
			at, ok := finishedAt(t, seen[a])
			if !ok {
				continue next
			}
			if at.After(last) {
				last = at
			}
		}
		if last.After(cutoff) {
			continue
		}
		for _, u := range t.askers {
			done := false
			for _, a := range askers[u] {
				if _, ok := finishedAt(t, seen[a]); ok {
					done = true
					break
				}
			}
			if !done {
				continue next
			}
		}
		out = append(out, WatchedTitle{ID: t.id, Kind: t.kind, Name: t.name, Year: t.year, Bytes: t.bytes, LastWatchedAt: last.UTC().Format(time.RFC3339)})
	}
	return out
}

// accountKey is how a person is named in people=.
func accountKey(a watchAccount) string {
	return strconv.FormatInt(a.server, 10) + ":" + a.key
}

// parseCleanupAsk reads days= (0 to 3650, else the default) and people=
// (keys joined by commas; empty means everyone).
func parseCleanupAsk(days, people string) (int, map[string]bool) {
	n, err := strconv.Atoi(strings.TrimSpace(days))
	if err != nil || n < 0 || n > 3650 {
		n = cleanupDays
	}
	var keys map[string]bool
	for _, k := range strings.Split(people, ",") {
		if k = strings.TrimSpace(k); k != "" {
			if keys == nil {
				keys = map[string]bool{}
			}
			keys[k] = true
		}
	}
	return n, keys
}

// cleanupTitles turns the Library's titles with files into what the list
// judges, with who asked for each.
func cleanupTitles(ctx context.Context, q watchReader, askedBy map[int64][]int64) ([]cleanupTitle, error) {
	got, err := mediafiles.TitlesWithFilesUnprotected(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]cleanupTitle, 0, len(got))
	for _, g := range got {
		t := cleanupTitle{id: g.ID, kind: g.Kind, name: g.Name, year: g.Year, bytes: g.Bytes, askers: askedBy[g.ID]}
		for _, e := range g.Episodes {
			t.eps = append(t.eps, seenKey{g.ID, e[0], e[1]})
		}
		out = append(out, t)
	}
	return out, nil
}

// WatchedCleanupList builds the clean-up list from the last watch history
// read. people names the accounts to wait for; nil waits for everyone.
// askedBy holds, per title, the people who asked for it.
func WatchedCleanupList(ctx context.Context, q watchReader, days int, people map[string]bool, askedBy map[int64][]int64, now time.Time) (WatchedCleanup, error) {
	out := WatchedCleanup{Enabled: WatchHistoryOn(ctx, q), Days: days, People: []CleanupPerson{}, Found: []WatchedTitle{}}
	if !out.Enabled {
		return out, nil
	}
	all, err := cleanupPeople(ctx, q)
	if err != nil {
		return out, err
	}
	var chosen []watchAccount
	for _, p := range all {
		k := accountKey(p.acc)
		on := people == nil || people[k]
		if on {
			chosen = append(chosen, p.acc)
		}
		out.People = append(out.People, CleanupPerson{Key: k, Name: p.name, Server: p.server, Chosen: on})
	}
	seen, err := cleanupSeen(ctx, q)
	if err != nil {
		return out, err
	}
	titles, err := cleanupTitles(ctx, q, askedBy)
	if err != nil {
		return out, err
	}
	askers := map[int64][]watchAccount{}
	for _, t := range titles {
		for _, u := range t.askers {
			if _, done := askers[u]; done {
				continue
			}
			mine, err := accountsOf(ctx, q, u)
			if err != nil {
				return out, err
			}
			askers[u] = mine
		}
	}
	out.Found = pickWatched(titles, seen, chosen, askers, now.AddDate(0, 0, -days))
	return out, nil
}

// watchedCleanup answers the clean-up list, for people who manage the
// Library. It only lists; removing goes through the title's own delete.
func (h *Handlers) watchedCleanup(w http.ResponseWriter, r *http.Request) {
	days, people := parseCleanupAsk(r.URL.Query().Get("days"), r.URL.Query().Get("people"))
	askedBy := map[int64][]int64{}
	if h.AskedBy != nil {
		var err error
		if askedBy, err = h.AskedBy(r.Context()); err != nil {
			internalError(w, err)
			return
		}
	}
	out, err := WatchedCleanupList(r.Context(), h.Store.Read(), days, people, askedBy, time.Now().UTC())
	if err != nil {
		internalError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, out)
}

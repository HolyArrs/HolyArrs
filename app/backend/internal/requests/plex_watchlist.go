package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// Plex watchlist sync: people who turn it on have the
// newest entries of their Plex watchlist asked for as them, every few
// minutes, through the normal ask path (weekly limits, auto-approve,
// joining an open ask and age-rating limits all apply). Each entry is acted
// on once; only entries held back by the weekly limit, or that failed, are
// tried again. Taking an entry off the Plex watchlist changes nothing.

// KindPlexWatchlists is the scheduled Job that reads Plex watchlists.
const KindPlexWatchlists = "sync_plex_watchlists"

// DefaultPlexDiscover is Plex's discover service, where watchlists live.
const DefaultPlexDiscover = "https://discover.provider.plex.tv"

// WatchlistSize is how many of the newest watchlist entries My account shows.
const WatchlistSize = 20

// watchlistRead is how many entries one pass reads, newest added first.
// It is more than WatchlistSize so new entries are still covered if Plex
// ever ignores the sort.
const watchlistRead = 50

// watchlistTries is how often an entry that failed is tried.
const watchlistTries = 3

// Watchlist entry states.
const (
	WatchAsked     = "asked"
	WatchJoined    = "joined"
	WatchAlready   = "already"
	WatchInLibrary = "in_library"
	WatchSkipped   = "skipped"
	WatchLimit     = "limit"
	WatchFailed    = "failed"
	WatchNoMatch   = "no_match"
)

// Watchlists reads Plex watchlists and asks for what is new on them.
type Watchlists struct {
	Asks *Service
	// Token gives the Plex token HolyArrs keeps for a person ("" when it
	// keeps none). It is sent only in a header and never logged.
	Token func(ctx context.Context, userID int64) (string, error)
	// ClientID is this HolyArrs's id toward Plex (may be nil).
	ClientID func(ctx context.Context) string
	// Base is the discover service (DefaultPlexDiscover when empty).
	Base string
	HTTP *http.Client
}

func (w *Watchlists) base() string {
	if w.Base != "" {
		return strings.TrimRight(w.Base, "/")
	}
	return DefaultPlexDiscover
}

func (w *Watchlists) client() *http.Client {
	if w.HTTP != nil {
		return w.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// Register adds the Job handler and its schedule check.
func (w *Watchlists) Register(q *jobs.Queue) {
	q.Handle(KindPlexWatchlists, w.Run)
	q.ProbeSchedule(KindPlexWatchlists, w.Probe)
}

// HasToken says whether HolyArrs keeps a Plex token for the person.
func (w *Watchlists) HasToken(ctx context.Context, userID int64) bool {
	if w == nil || w.Token == nil {
		return false
	}
	tok, err := w.Token(ctx, userID)
	return err == nil && tok != ""
}

// Probe says whether a scheduled pass has anyone to read: the Requests
// setting is on and someone turned their watchlist on. It writes nothing.
func (w *Watchlists) Probe(ctx context.Context, _ time.Time) (bool, any, error) {
	ids, err := w.people(ctx)
	return len(ids) > 0, nil, err
}

// people are the users whose watchlist is read now (none while the
// Requests setting is off). With the everyone switch on, that is also
// everyone with a Plex token who has not chosen for themselves.
func (w *Watchlists) people(ctx context.Context) ([]int64, error) {
	db := w.Asks.Store.Read()
	set, err := ReadSettings(ctx, db)
	if err != nil || !set.PlexWatchlists {
		return nil, err
	}
	q := peopleSQL2
	if set.PlexWatchlistsEveryone {
		q = peopleSQL3
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	type who struct {
		id     int64
		chosen bool
	}
	var all []who
	for rows.Next() {
		p := who{chosen: true}
		dest := []any{&p.id}
		if set.PlexWatchlistsEveryone {
			dest = append(dest, &p.chosen)
		}
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return nil, err
		}
		all = append(all, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var ids []int64
	for _, p := range all {
		if p.chosen || w.HasToken(ctx, p.id) {
			ids = append(ids, p.id)
		}
	}
	return ids, nil
}

// Run reads one person's watchlist (params user_id, from their Check
// now) or everyone's who turned it on (the schedule). One person failing
// does not stop the others.
func (w *Watchlists) Run(ctx context.Context, r *jobs.Run) error {
	ids, err := w.people(ctx)
	if err != nil {
		return err
	}
	if v, ok := r.Job.Params["user_id"].(float64); ok && v > 0 {
		one := []int64{}
		for _, id := range ids {
			if id == int64(v) {
				one = append(one, id)
			}
		}
		ids = one
	}
	total := PassResult{}
	for _, id := range ids {
		res, err := w.Pass(ctx, id)
		if err != nil {
			log.Printf("plex watchlist: user %d: %v", id, err)
			total.Failed++
		}
		total.Read += res.Read
		total.Asked += res.Asked
	}
	r.SetResult(map[string]any{"people": len(ids), "entries_read": total.Read, "asked": total.Asked, "people_failed": total.Failed})
	return nil
}

// PassResult is what one pass did.
type PassResult struct {
	Read   int // watchlist entries read
	Asked  int // asks made or joined
	Failed int
}

// watchEntry is one movie or show on a Plex watchlist.
type watchEntry struct {
	key, kind, name string
	year            int
	ids             map[string]string // tmdb, tvdb, imdb
}

// Pass reads one person's watchlist now and acts on its new entries.
func (w *Watchlists) Pass(ctx context.Context, userID int64) (PassResult, error) {
	var res PassResult
	db := w.Asks.Store.Read()
	var name, role, perms, state, maxRating string
	var movies, shows, chosen bool
	err := db.QueryRowContext(ctx, passSQL, userID).Scan(&name, &role, &perms, &state, &maxRating, &movies, &shows, &chosen)
	if errors.Is(err, sql.ErrNoRows) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	// Not chosen for themselves: on when the owner turned it on for
	// everyone, and then skipped quietly when it cannot be read.
	quiet := false
	if !chosen {
		set, err := ReadSettings(ctx, db)
		if err != nil {
			return res, err
		}
		if set.PlexWatchlistsEveryone {
			movies, shows, quiet = true, true, true
		}
	}
	if !movies && !shows || state != "active" {
		return res, nil
	}
	p := personOf(userID, name, role, perms, maxRating)
	if p == nil {
		if quiet {
			return res, nil
		}
		return res, w.checked(ctx, userID, "Your account may not ask for titles, so your Plex watchlist is not read.")
	}
	tok := ""
	if w.Token != nil {
		if tok, err = w.Token(ctx, userID); err != nil {
			return res, err
		}
	}
	if tok == "" {
		if quiet {
			return res, nil
		}
		return res, w.checked(ctx, userID, "Link your Plex account so HolyArrs can read your watchlist.")
	}
	entries, err := w.fetch(ctx, tok)
	if err != nil {
		_ = w.checked(ctx, userID, err.Error())
		return res, err
	}
	res.Read = len(entries)
	now := database.FormatTime(w.Asks.now())
	for _, e := range entries {
		if e.kind == "movie" && !movies || e.kind == "show" && !shows {
			continue // left for when that kind is turned on
		}
		var was string
		var tries int
		err := db.QueryRowContext(ctx, passSQL2, userID, e.key).Scan(&was, &tries)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return res, err
		}
		seen := err == nil
		if seen && was != WatchLimit && !(was == WatchFailed && tries < watchlistTries) {
			if err := w.Asks.Store.Write(ctx, func(tx *sql.Tx) error {
				_, err := tx.Exec(passSQL3, now, userID, e.key)
				return err
			}); err != nil {
				return res, err
			}
			continue
		}
		var st, ref, note string
		var askID *int64
		lookupFailed := false
		if len(e.ids) == 0 {
			// An entry without ids: one look at its own page. When that
			// look fails (Plex down, timeout, 5xx) the entry is tried again
			// like any failed one; only a real answer without ids is no_match.
			more, err := w.fetchOne(ctx, tok, e.key)
			if err != nil {
				lookupFailed = true
				st, note = WatchFailed, "Plex did not answer when HolyArrs looked up its ids; it tries again at the next check."
			} else if len(more.ids) > 0 {
				e.ids = more.ids
			}
		}
		if !lookupFailed {
			st, ref, askID, note = w.act(ctx, *p, e)
		}
		if st == WatchAsked || st == WatchJoined {
			res.Asked++
		}
		if st == WatchFailed {
			tries++
		}
		var year *int
		if e.year > 0 {
			year = &e.year
		}
		if err := w.Asks.Store.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec(passSQL4,
				userID, e.key, e.kind, ref, truncateWords(e.name, 300), year, st, askID, truncateWords(note, 300), tries, now, now)
			return err
		}); err != nil {
			return res, err
		}
		if askID != nil && (st == WatchAsked || st == WatchJoined) {
			w.Asks.changed(ctx, *askID)
		}
	}
	return res, w.checked(ctx, userID, "")
}

// personOf builds the asker from a users row; nil when they may not ask.
func personOf(id int64, name, role, perms, maxRating string) *Person {
	has := func(p string) bool { return role == "owner" || strings.Contains(perms, `"`+p+`"`) }
	if !has("ask") {
		return nil
	}
	p := &Person{UserID: id, Name: name, AutoApprove: has("auto_approve"), NoLimit: has("approve")}
	if role == "member" {
		p.MaxRating = maxRating
	}
	return p
}

// act asks for one entry as p and says what came of it.
func (w *Watchlists) act(ctx context.Context, p Person, e watchEntry) (state, ref string, askID *int64, note string) {
	db := w.Asks.Store.Read()
	if e.kind == "movie" {
		tmdb := e.ids["tmdb"]
		if tmdb == "" {
			return WatchNoMatch, "", nil, "Plex gave no TMDB id for it."
		}
		ref = "tmdb:" + tmdb
		var before int64
		_ = db.QueryRowContext(ctx, actSQL, tmdb, p.UserID).Scan(&before)
		if before != 0 {
			return WatchAlready, ref, &before, ""
		}
		r, err := w.Asks.Ask(ctx, p, tmdb)
		return outcome(r, err, ref)
	}
	for _, k := range []string{"tvdb", "tmdb", "imdb"} {
		if e.ids[k] != "" {
			ref = k + ":" + e.ids[k]
			break
		}
	}
	if ref == "" {
		return WatchNoMatch, "", nil, "Plex gave no TVDB, TMDB or IMDb id for it."
	}
	var or []string
	args := []any{p.UserID}
	for _, k := range []string{"tmdb", "tvdb", "imdb"} {
		if e.ids[k] != "" {
			or = append(or, "json_extract(a.source_ids, '$."+k+"') = ?")
			args = append(args, e.ids[k])
		}
	}
	var before int64
	_ = db.QueryRowContext(ctx, actSQL2+strings.Join(or, " OR ")+`) ORDER BY a.id DESC LIMIT 1`, args...).Scan(&before)
	if before != 0 {
		return WatchAlready, ref, &before, ""
	}
	r, err := w.Asks.AskShow(ctx, p, ref, nil)
	return outcome(r, err, ref)
}

// outcome turns an ask's answer into an entry state.
func outcome(r Result, err error, ref string) (string, string, *int64, string) {
	var in *InLibraryError
	var le *LimitError
	switch {
	case err == nil && r.Already:
		id := r.Ask.ID
		return WatchAlready, ref, &id, ""
	case err == nil && r.Joined:
		id := r.Ask.ID
		return WatchJoined, ref, &id, ""
	case err == nil:
		id := r.Ask.ID
		return WatchAsked, ref, &id, ""
	case errors.As(err, &in):
		return WatchInLibrary, ref, nil, ""
	case errors.As(err, &le):
		return WatchLimit, ref, nil, err.Error()
	case errors.Is(err, library.ErrNeverAdd), errors.Is(err, shows.ErrShowNeverAdd):
		return WatchSkipped, ref, nil, "It is on the Never add list."
	case errors.Is(err, ErrAboveLimit):
		return WatchSkipped, ref, nil, err.Error()
	}
	return WatchFailed, ref, nil, err.Error()
}

// checked notes the pass time and its problem ("" for none).
func (w *Watchlists) checked(ctx context.Context, userID int64, problem string) error {
	return w.Asks.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(checkedSQL,
			userID, database.FormatTime(w.Asks.now()), problem)
		return err
	})
}

// fetch reads the newest watchlist entries with the person's token.
func (w *Watchlists) fetch(ctx context.Context, tok string) ([]watchEntry, error) {
	q := url.Values{"includeGuids": {"1"}, "sort": {"watchlistedAt:desc"},
		"X-Plex-Container-Start": {"0"}, "X-Plex-Container-Size": {strconv.Itoa(watchlistRead)}}
	var got plexContainer
	if err := w.get(ctx, tok, "/library/sections/watchlist/all?"+q.Encode(), &got); err != nil {
		return nil, err
	}
	return got.entries(), nil
}

// fetchOne reads one entry's own page (for its ids). An error means the
// look itself failed; an answer without the entry gives an empty one.
func (w *Watchlists) fetchOne(ctx context.Context, tok, key string) (watchEntry, error) {
	var got plexContainer
	if err := w.get(ctx, tok, "/library/metadata/"+url.PathEscape(key)+"?includeGuids=1", &got); err != nil {
		return watchEntry{}, err
	}
	if es := got.entries(); len(es) > 0 {
		return es[0], nil
	}
	return watchEntry{}, nil
}

// Errors from reading a watchlist, said to the person.
var (
	ErrPlexRefused = errors.New("Plex refused the saved sign-in; link your Plex account again")
	ErrPlexDown    = errors.New("Plex did not answer; HolyArrs tries again at the next check")
)

func (w *Watchlists) get(ctx context.Context, tok, path string, into any) error {
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "plex_watchlist", Identity: []string{w.base(), tok}})
	if err != nil {
		return ErrPlexDown
	}
	result := outbound.Canceled
	defer func() { op.Finish(result) }()
	rctx, cancel := op.AttemptContext(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, w.base()+path, nil)
	if err != nil {
		result = outbound.TerminalFailure
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", tok)
	req.Header.Set("X-Plex-Product", "HolyArrs")
	if w.ClientID != nil {
		if id := w.ClientID(ctx); id != "" {
			req.Header.Set("X-Plex-Client-Identifier", id)
		}
	}
	resp, err := w.client().Do(req)
	if err != nil {
		result = outbound.HTTPOutcome(0, err, ctx)
		return ErrPlexDown
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	result = outbound.HTTPOutcome(resp.StatusCode, err, ctx)
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrPlexRefused
	case resp.StatusCode != http.StatusOK || err != nil:
		return fmt.Errorf("Plex answered %d; HolyArrs tries again at the next check", resp.StatusCode)
	}
	if err := json.Unmarshal(body, into); err != nil {
		result = outbound.TerminalFailure
		return errors.New("Plex sent a watchlist HolyArrs could not read")
	}
	result = outbound.Success
	return nil
}

// plexContainer is the part of Plex's answer HolyArrs reads. Fields are
// read loosely: an id may come as a number or a string, and entries of
// other types or without a key are left out.
type plexContainer struct {
	MediaContainer struct {
		Metadata []struct {
			RatingKey flexText `json:"ratingKey"`
			GUID      flexText `json:"guid"`
			Type      string   `json:"type"`
			Title     string   `json:"title"`
			Year      flexText `json:"year"`
			Guid      []struct {
				ID string `json:"id"`
			} `json:"Guid"`
		} `json:"Metadata"`
	} `json:"MediaContainer"`
}

func (c plexContainer) entries() []watchEntry {
	var out []watchEntry
	for _, m := range c.MediaContainer.Metadata {
		kind := map[string]string{"movie": "movie", "show": "show"}[strings.ToLower(m.Type)]
		key := strings.TrimSpace(string(m.RatingKey))
		if key == "" {
			key = strings.TrimSpace(string(m.GUID))
		}
		if kind == "" || key == "" {
			continue
		}
		e := watchEntry{key: key, kind: kind, name: strings.TrimSpace(m.Title), ids: map[string]string{}}
		e.year, _ = strconv.Atoi(string(m.Year))
		for _, g := range m.Guid {
			src, id, ok := strings.Cut(strings.TrimSpace(g.ID), "://")
			src = strings.ToLower(src)
			if ok && id != "" && (src == "tmdb" || src == "tvdb" || src == "imdb") && e.ids[src] == "" {
				e.ids[src] = id
			}
		}
		out = append(out, e)
	}
	return out
}

// flexText takes a JSON string or number as text.
type flexText string

// UnmarshalJSON accepts both forms because the list feed mixes them for ids.
func (f *flexText) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*f = flexText(s)
		return nil
	}
	var n json.Number
	if json.Unmarshal(b, &n) == nil {
		*f = flexText(n.String())
	}
	return nil
}

func truncateWords(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// WatchlistSwitches are the kinds a person has HolyArrs read.
type WatchlistSwitches struct {
	Movies bool `json:"movies"`
	Shows  bool `json:"shows"`
}

// WatchlistItem is one watchlist entry as My account shows it.
type WatchlistItem struct {
	Name   string    `json:"name"`
	Year   *int      `json:"year,omitempty"`
	Kind   string    `json:"kind"`
	State  string    `json:"state"`
	AskID  *int64    `json:"ask_id,omitempty"`
	Note   string    `json:"note,omitempty"`
	SeenAt time.Time `json:"seen_at"`
}

// WatchlistState is a person's switches, last pass and newest entries.
type WatchlistState struct {
	WatchlistSwitches
	// Chosen: the person set their own switches (else they follow the owner).
	Chosen    bool            `json:"chosen"`
	CheckedAt *time.Time      `json:"checked_at,omitempty"`
	Error     string          `json:"error,omitempty"`
	Items     []WatchlistItem `json:"items"`
}

// ReadWatchlist reads one person's watchlist switches, last pass and the
// newest entries seen.
func ReadWatchlist(ctx context.Context, db *sql.DB, userID int64) (WatchlistState, error) {
	s := WatchlistState{Items: []WatchlistItem{}}
	var at sql.NullString
	err := db.QueryRowContext(ctx, readWatchlistSQL, userID).
		Scan(&s.Movies, &s.Shows, &s.Chosen, &at, &s.Error)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	if at.Valid {
		t, _ := time.Parse(database.TimeLayout, at.String)
		s.CheckedAt = &t
	}
	rows, err := db.QueryContext(ctx, readWatchlistSQL2, userID, WatchlistSize)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var it WatchlistItem
		var seen string
		if err := rows.Scan(&it.Name, &it.Year, &it.Kind, &it.State, &it.AskID, &it.Note, &seen); err != nil {
			return s, err
		}
		it.SeenAt, _ = time.Parse(database.TimeLayout, seen)
		s.Items = append(s.Items, it)
	}
	return s, rows.Err()
}

// SaveWatchlistSwitches stores which kinds a person has HolyArrs read.
func SaveWatchlistSwitches(ctx context.Context, st *database.Store, userID int64, sw WatchlistSwitches) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(saveWatchlistSwitchesSQL,
			userID, sw.Movies, sw.Shows)
		return err
	})
}

package discover

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func (s ExploreSource) List(ctx context.Context, q engine.Request) ([]engine.Item, error) {
	e := s.E
	key := q.String()
	ttl := ExploreListTTL[q.List]
	got, ok := s.cachedList(ctx, key)
	if ok && time.Since(got.at) < ttl {
		return got.items, nil
	}
	if s.CacheOnly || s.Lister == nil {
		if ok {
			return got.items, nil
		}
		return []engine.Item{}, nil
	}
	e.mu.Lock()
	if f, busy := e.flight[key]; busy {
		e.mu.Unlock()
		select {
		case <-f.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if f.err == nil {
			return f.items, nil
		}
		if ok {
			return got.items, nil
		}
		return nil, f.err
	}
	if e.flight == nil {
		e.flight = map[string]*listFlight{}
	}
	f := &listFlight{done: make(chan struct{})}
	e.flight[key] = f
	if e.Paced == nil {
		// A cold genre page asks up to 9 lists at once; a burst lets them
		// run side by side instead of 2 s apart (about 6 s cold before).
		e.Paced = &engine.Paced{Source: ListerSource{L: s.Lister}, Burst: exploreListBurst}
	}
	p := e.Paced
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.flight, key)
		e.mu.Unlock()
		close(f.done)
	}()
	s.Calls.Add(q)
	f.items, f.err = p.List(ctx, q)
	if f.err != nil {
		if ctx.Err() == nil && s.noteListDown(key, true) && (!errors.Is(f.err, metadata.ErrNoAccess) || listLogDue(f.err, time.Now())) {
			log.Printf("explore: list %s failed: %v", q.List, f.err)
		}
		if ok {
			return got.items, nil
		}
		return nil, f.err
	}
	s.noteListDown(key, false)
	now := time.Now()
	s.keepList(key, exploreList{at: now, items: f.items})
	if s.E.Store() != nil {
		if b, err := json.Marshal(storedList{At: now, Items: f.items}); err == nil {
			_ = metadata.LookupCache{Store: s.E.Store()}.Put(context.WithoutCancel(ctx), "explore|"+key, b, now, now.Add(ttl))
		}
	}
	return f.items, nil
}

// noteListDown records whether key's last outside ask failed and says
// whether that changed. Kept per key: one list's keys fail on their own (a
// removed title's "similar"), and some answer from the TMDB copy in an
// outage. Bounded like the list cache; starting over only logs again.
func (s ExploreSource) noteListDown(key string, down bool) bool {
	e := s.E
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failing[key] == down {
		return false
	}
	if !down {
		delete(e.failing, key)
		return true
	}
	if e.failing == nil || len(e.failing) >= maxExploreRows {
		e.failing = map[string]bool{}
	}
	e.failing[key] = true
	return true
}

// ExploreMember is what a pool build needs about the member.
type ExploreMember struct {
	ID               int64
	Kid              bool
	MaxRating        string
	Services, Region string
}

func (e *Explore) LoadMember(ctx context.Context, id int64) (ExploreMember, error) {
	m := ExploreMember{ID: id}
	var role string
	err := e.Store().Read().QueryRowContext(ctx, "SELECT role, max_rating FROM users WHERE id = ?", id).Scan(&role, &m.MaxRating)
	if err != nil {
		return m, err
	}
	m.Kid = role == "member" && m.MaxRating != ""
	_ = e.Store().Read().QueryRowContext(ctx, "SELECT services, region FROM explore_prefs WHERE member_id = ?", id).Scan(&m.Services, &m.Region)
	return m, nil
}

// exploreInput reads the member's signals from the Library and the asks.
func (e *Explore) Input(ctx context.Context, m ExploreMember, sw string, now time.Time) (engine.Input, error) {
	in := engine.Input{Member: m.ID, Kid: m.Kid, Switch: sw, Now: now, Owned: map[string]bool{},
		OwnedPeople: map[string]int{}, OwnedDecadeGenre: map[string]int{}, Services: m.Services, Region: m.Region}
	db := e.Store().Read()
	rows, err := db.QueryContext(ctx, `SELECT kind, json_extract(source_ids, '$.tmdb'), name, COALESCE(year, 0), genres, people, rating, created_at
		FROM titles WHERE kind IN ('movie', 'show') AND state != 'removed' AND json_extract(source_ids, '$.tmdb') IS NOT NULL
		ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return in, err
	}
	var lib []engine.LibTitle
	for rows.Next() {
		var t engine.LibTitle
		var genres, people, added string
		if err := rows.Scan(&t.Kind, &t.TMDBID, &t.Name, &t.Year, &genres, &people, &t.Rating, &added); err != nil {
			rows.Close()
			return in, err
		}
		_ = json.Unmarshal([]byte(genres), &t.Genres)
		_ = json.Unmarshal([]byte(people), &t.People)
		t.Added, _ = time.Parse(database.TimeLayout, added)
		key := t.Kind + ":" + t.TMDBID
		in.Owned[key] = true
		if m.Kid && !metadata.RatingAllows(m.MaxRating, t.Kind, t.Rating) {
			continue
		}
		lib = append(lib, t)
		for _, p := range t.People[:min(4, len(t.People))] {
			in.OwnedPeople[p]++
		}
		if t.Year > 0 {
			for _, g := range t.Genres {
				in.OwnedDecadeGenre[strconv.Itoa(t.Year/10*10)+"s|"+g]++
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return in, err
	}
	rows, err = db.QueryContext(ctx, `SELECT a.kind, a.tmdb_id, a.name, COALESCE(a.year, 0), a.poster, p.user_id, p.asked_at, u.role = 'member' AND u.max_rating != '',
		a.state IN ('asked', 'approved')
		FROM ask_people p JOIN asks a ON a.id = p.ask_id JOIN users u ON u.id = p.user_id
		WHERE a.kind IN ('movie', 'show') AND a.tmdb_id != '' AND a.state != 'turned_down' ORDER BY p.asked_at`)
	if err != nil {
		return in, err
	}
	var asks []engine.AskSeed
	type hh struct {
		item    engine.Item
		asks    int
		members map[int64]bool
	}
	house := map[string]*hh{}
	var order []string
	month := now.AddDate(0, 0, -30)
	for rows.Next() {
		var s engine.AskSeed
		var poster, at string
		var open bool
		if err := rows.Scan(&s.Kind, &s.TMDBID, &s.Name, &s.Year, &poster, &s.Member, &at, &s.Kid, &open); err != nil {
			rows.Close()
			return in, err
		}
		s.At, _ = time.Parse(database.TimeLayout, at)
		asks = append(asks, s)
		if s.Member == m.ID && open {
			in.MyAsks = append([]engine.Item{{TMDBID: s.TMDBID, Kind: s.Kind, Name: s.Name, Year: s.Year, Poster: poster, Genres: []string{}}}, in.MyAsks...)
		}
		if s.At.After(month) {
			key := s.Kind + ":" + s.TMDBID
			h, ok := house[key]
			if !ok {
				h = &hh{item: engine.Item{TMDBID: s.TMDBID, Kind: s.Kind, Name: s.Name, Year: s.Year, Poster: poster, Genres: []string{}}, members: map[int64]bool{}}
				house[key] = h
				order = append(order, key)
			}
			h.asks++
			h.members[s.Member] = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return in, err
	}
	for _, k := range order {
		h := house[k]
		in.Household = append(in.Household, engine.HouseholdTitle{Item: h.item, Asks: h.asks, Members: len(h.members)})
	}
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE state = 'active'").Scan(&in.Members)
	if sw == "shows" {
		// Returning soon: Library shows whose next episode airs in 7 to 60 days.
		if in.Returning, err = ExploreReturning(ctx, db, now); err != nil {
			return in, err
		}
	}
	if sw == "movies" {
		// Finish the collection: TMDB collections the Library has a movie of.
		if in.Collections, err = ExploreCollections(ctx, db); err != nil {
			return in, err
		}
	}
	in.Seeds = engine.Seeds(m.ID, m.Kid, lib, asks, now)
	return in, nil
}

// ExploreReturning are the Library shows whose next episode airs in 7 to
// 60 days, soonest first (local data only).
func ExploreReturning(ctx context.Context, db *sql.DB, now time.Time) ([]engine.Item, error) {
	rows, err := db.QueryContext(ctx, `SELECT json_extract(t.source_ids, '$.tmdb'), t.name, COALESCE(t.year, 0), t.genres, t.artwork, t.backdrop, MIN(i.air_date) AS next
		FROM titles t JOIN items i ON i.title_id = t.id AND i.kind = 'episode' AND i.dropped_at IS NULL AND i.air_date > ?
		WHERE t.kind = 'show' AND t.state != 'removed' AND json_extract(t.source_ids, '$.tmdb') IS NOT NULL
		GROUP BY t.id HAVING next >= ? AND next <= ? ORDER BY next, t.id LIMIT 40`,
		database.FormatTime(now), database.FormatTime(now.AddDate(0, 0, 7)), database.FormatTime(now.AddDate(0, 0, 60)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engine.Item
	for rows.Next() {
		var it engine.Item
		var genres, artwork, next string
		if err := rows.Scan(&it.TMDBID, &it.Name, &it.Year, &genres, &artwork, &it.Backdrop, &next); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(genres), &it.Genres)
		var art []string
		_ = json.Unmarshal([]byte(artwork), &art)
		if links := ArtworkLinks(art); len(links) > 0 {
			it.Poster = links[0]
		}
		if d, err := time.Parse(database.TimeLayout, next); err == nil {
			it.Released = d.Format("2006-01-02")
		}
		it.Kind, it.Owned = "show", true
		out = append(out, it)
	}
	return out, rows.Err()
}

// ExploreCollections are the TMDB collections with an owned movie, most
// owned first (local data only; the parts are looked up by collection id).
func ExploreCollections(ctx context.Context, db *sql.DB) ([]engine.LibCollection, error) {
	rows, err := db.QueryContext(ctx, `SELECT substr(c.source_id, 6), c.name, count(t.id) AS n
		FROM movie_collections c
		JOIN movie_collection_titles ct ON ct.collection_id = c.id
		JOIN titles t ON t.id = ct.title_id AND t.kind = 'movie' AND t.state != 'removed'
		WHERE substr(c.source_id, 1, 5) = 'tmdb:'
		GROUP BY c.id ORDER BY n DESC, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engine.LibCollection
	for rows.Next() {
		var c engine.LibCollection
		if err := rows.Scan(&c.TMDBID, &c.Name, &c.Owned); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// kidAllow is the Kid limit at candidate time: known ratings first, then a
// few lookups; an unknown rating hides the title.
func (e *Explore) KidAllow(ctx context.Context, max string) func(engine.Item) bool {
	left := exploreKidLookups
	return func(it engine.Item) bool {
		ref := mediafiles.RatingRef{Kind: it.Kind, TMDBID: it.TMDBID}
		r, ok := e.KnownRating(ctx, ref)
		if !ok && left > 0 {
			left--
			r, ok = e.LookupRating(ctx, ref)
		}
		return ok && metadata.RatingAllows(max, it.Kind, r)
	}
}

// readPool reads the member's pool for a day ("" takes the newest).
func (e *Explore) ReadPool(ctx context.Context, member int64, sw, day string, before bool) (*engine.Pool, bool, error) {
	q := "SELECT pool, stale FROM explore_pool WHERE member_id = ? AND type_switch = ? AND day = ?"
	if before {
		q = "SELECT pool, stale FROM explore_pool WHERE member_id = ? AND type_switch = ? AND day < ? ORDER BY day DESC LIMIT 1"
	}
	var raw string
	var stale bool
	err := e.Store().Read().QueryRowContext(ctx, q, member, sw, day).Scan(&raw, &stale)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var p engine.Pool
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, false, err
	}
	return &p, stale, nil
}

// buildPool builds and stores one member's pool for today. It goes out
// only through the job's own limiter, never the per-person limit.
func (e *Explore) BuildPool(ctx context.Context, member int64, sw string) error {
	now := e.Now()
	m, err := e.LoadMember(ctx, member)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := e.Input(ctx, m, sw, now)
	if err != nil {
		return err
	}
	day := engine.Day(now)
	if in.Today, _, err = e.ReadPool(ctx, member, sw, day, false); err != nil {
		return err
	}
	if in.Prev, _, err = e.ReadPool(ctx, member, sw, day, true); err != nil {
		return err
	}
	if m.Kid {
		in.Allow = e.KidAllow(ctx, m.MaxRating)
	}
	var lister metadata.ExploreLister
	if l, ok := metadata.ExploreListerFor(ctx, e.Metadata()); ok {
		lister = l
	}
	calls := &engine.Calls{}
	start := time.Now()
	pool, err := engine.Build(ctx, ExploreSource{E: e, Lister: lister, Calls: calls}, in)
	if err != nil {
		return err
	}
	log.Printf("explore: pool %d/%s built in %s, %d seeds, outside calls %s", member, sw, time.Since(start).Round(time.Millisecond), pool.Seeds, calls)
	raw, err := json.Marshal(pool)
	if err != nil {
		return err
	}
	err = e.Store().Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO explore_pool (member_id, type_switch, day, built_at, stale, due, pool) VALUES (?, ?, ?, ?, 0, NULL, ?)
			ON CONFLICT (member_id, type_switch, day) DO UPDATE SET built_at = excluded.built_at, stale = 0, due = NULL, pool = excluded.pool`,
			member, sw, day, database.FormatTime(now), string(raw))
		return err
	})
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.builds++
	if e.calls == nil {
		e.calls = map[PoolKey]*engine.Calls{}
	}
	e.calls[PoolKey{member, sw}] = calls
	for k := range e.pages {
		if strings.HasPrefix(k, strconv.FormatInt(member, 10)+"|"+sw+"|") {
			delete(e.pages, k)
		}
	}
	e.mu.Unlock()
	e.QueueCardArt(ctx, &pool)
	return nil
}

// listLogDue says whether a refused list should be logged: the same error
// is logged at most once every five minutes, whichever list hit it. A
// refused install fails every list at once, and each one used to log the
// same line. Other failures still log once per list per outage.
func listLogDue(err error, now time.Time) bool {
	msg := err.Error()
	if len(msg) > 120 {
		msg = msg[:120]
	}
	listLogMu.Lock()
	defer listLogMu.Unlock()
	if at, ok := listLogAt[msg]; ok && now.Sub(at) < 5*time.Minute {
		return false
	}
	if len(listLogAt) > 64 {
		clear(listLogAt)
	}
	listLogAt[msg] = now
	return true
}

var (
	listLogMu sync.Mutex
	listLogAt = map[string]time.Time{}
)

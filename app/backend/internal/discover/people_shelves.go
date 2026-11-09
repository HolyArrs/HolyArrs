package discover

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// People shelves are Search's People sections ahead of the popular list:
// the people most in the Library, and the leads of what is hot now. Each
// shelf is kept 12 hours per age-rating limit.
const peopleShelfTTL = 12 * time.Hour

// peopleShelfMin is the fewest people with a picture a shelf needs to show.
const peopleShelfMin = 8

const peopleShelfMax = 48

var peopleShelfTitles = map[string]string{"library": "From your Library", "hot": "In what's hot right now"}

type shelfPerson struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Department string `json:"department"`
	Profile    string `json:"profile"`
	Note       string `json:"note"`
	count      int
}

type peopleShelf struct {
	ID     string        `json:"id"`
	Title  string        `json:"title"`
	People []shelfPerson `json:"people"`
}

// shelfDepartment keeps actors, directors and composers ("Sound"); writers,
// creators and the rest are left out.
func shelfDepartment(d string) bool { return d == "Acting" || d == "Directing" || d == "Sound" }

func titlesNote(n int) string {
	if n == 1 {
		return "1 title"
	}
	return strconv.Itoa(n) + " titles"
}

// eachLimited runs f for 0..n-1, four at a time (the metadata server
// turns away bursts).
func eachLimited(n int, f func(i int)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer func() { <-sem; wg.Done() }()
			f(i)
		}(i)
	}
	wg.Wait()
}

func (h *Handlers) peopleShelf(ctx context.Context, id, max string, p metadata.PeopleProvider) (peopleShelf, error) {
	key := "peopleshelf|v2|" + id + "|" + max
	now := time.Now()
	c := metadata.LookupCache{Store: h.Store}
	if b, ok, _ := c.Get(ctx, key, now); ok {
		var s peopleShelf
		if json.Unmarshal(b, &s) == nil {
			return s, nil
		}
	}
	s := peopleShelf{ID: id, Title: peopleShelfTitles[id], People: []shelfPerson{}}
	var err error
	// missed counts lookups that failed; such a shelf is tried again soon.
	var missed atomic.Int32
	if id == "library" {
		s.People, err = h.libraryShelf(ctx, max, p, &missed)
	} else if max == "" {
		// A Kid gets no hot shelf: those titles are not held to their limit.
		s.People = h.hotShelf(ctx, &missed)
	}
	if err != nil {
		return s, err
	}
	ttl := peopleShelfTTL
	if missed.Load() > 0 {
		ttl = 15 * time.Minute
	}
	if b, err := json.Marshal(s); err == nil {
		_ = c.Put(context.WithoutCancel(ctx), key, b, now, now.Add(ttl))
	}
	return s, nil
}

// libraryShelf counts the names stored with the Library titles this member
// may see, looks the most frequent up, and adds the composers of the newest
// titles from their credits.
func (h *Handlers) libraryShelf(ctx context.Context, limit string, p metadata.PeopleProvider, missed *atomic.Int32) ([]shelfPerson, error) {
	where := "t.kind IN ('movie', 'show')"
	clause, args := mediafiles.KidSQL(limit)
	if clause != "" {
		where += " AND " + clause
	}
	rows, err := h.Store.Read().QueryContext(ctx, `SELECT t.kind, COALESCE(json_extract(t.source_ids, '$.tmdb'), ''), t.people
		FROM titles t WHERE `+where+` ORDER BY t.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	type titleRef struct{ kind, tmdb string }
	counts, order, recent := map[string]int{}, []string{}, []titleRef{}
	for rows.Next() {
		var kind, tmdb, raw string
		if err := rows.Scan(&kind, &tmdb, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		if tmdb != "" && len(recent) < 40 {
			recent = append(recent, titleRef{kind, tmdb})
		}
		var names []string
		_ = json.Unmarshal([]byte(raw), &names)
		for _, n := range names {
			if n = strings.TrimSpace(n); n != "" {
				if counts[n] == 0 {
					order = append(order, n)
				}
				counts[n]++
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	for len(order) > 0 && counts[order[len(order)-1]] < 2 {
		order = order[:len(order)-1]
	}
	order = order[:min(len(order), peopleShelfMax)]
	found := make([]shelfPerson, len(order))
	eachLimited(len(order), func(i int) {
		got, err := p.SearchPeople(ctx, order[i], 1)
		if err != nil {
			missed.Add(1)
			return
		}
		for _, m := range got.People {
			if strings.EqualFold(m.Name, order[i]) {
				found[i] = shelfPerson{ID: m.ID, Name: m.Name, Department: m.Department, Profile: m.Profile, count: counts[order[i]]}
				return
			}
		}
	})
	byID := map[string]*shelfPerson{}
	out := []shelfPerson{}
	add := func(sp shelfPerson) {
		if sp.ID == "" || sp.Profile == "" || !shelfDepartment(sp.Department) {
			return
		}
		if had := byID[sp.ID]; had != nil {
			had.count = max(had.count, sp.count)
			return
		}
		out = append(out, sp)
		byID[sp.ID] = &out[len(out)-1]
	}
	for _, sp := range found {
		add(sp)
	}
	if cr, ok := metadata.TitleCrewerFor(ctx, h.Metadata); ok {
		crews := make([]metadata.TitleCrew, len(recent))
		eachLimited(len(recent), func(i int) {
			var err error
			if crews[i], err = cr.TitleCrew(ctx, recent[i].kind, "tmdb:"+recent[i].tmdb); err != nil {
				missed.Add(1)
			}
		})
		sound, soundOrder := map[string]*shelfPerson{}, []string{}
		for _, crew := range crews {
			for _, m := range crew.Composers {
				if sound[m.ID] == nil {
					sound[m.ID] = &shelfPerson{ID: m.ID, Name: m.Name, Department: "Sound", Profile: m.Profile}
					soundOrder = append(soundOrder, m.ID)
				}
				sound[m.ID].count++
			}
		}
		for _, id := range soundOrder {
			if byID[id] == nil {
				add(*sound[id])
			}
		}
	}
	// add keeps pointers into out, so the order is set only now.
	sort.SliceStable(out, func(i, j int) bool { return out[i].count > out[j].count })
	for i := range out {
		out[i].Note = titlesNote(out[i].count)
	}
	return out, nil
}

// hotShelf takes the leads of this week's trending titles and the Top 10
// from the lists Explore already keeps: three billed actors, the director
// and the composer of each.
func (h *Handlers) hotShelf(ctx context.Context, missed *atomic.Int32) []shelfPerson {
	type hot struct{ kind, tmdb, name string }
	var lists [][]hot
	src := h.LiveSource(context.WithoutCancel(ctx))
	for _, kind := range []string{"movie", "show"} {
		var l []hot
		if items, err := src.List(ctx, engine.Request{List: "trending", Kind: kind, Scope: engine.ScopeShared}); err == nil {
			for _, it := range items {
				l = append(l, hot{it.Kind, it.TMDBID, it.Name})
			}
		}
		lists = append(lists, l)
	}
	// One title from each list in turn, each title once, up to 24.
	titles, seen := []hot{}, map[string]bool{}
	for i := 0; len(titles) < 24; i++ {
		more := false
		for _, l := range lists {
			if i < len(l) {
				more = true
				if k := l[i].kind + ":" + l[i].tmdb; l[i].tmdb != "" && !seen[k] && len(titles) < 24 {
					seen[k] = true
					titles = append(titles, l[i])
				}
			}
		}
		if !more {
			break
		}
	}
	cr, ok := metadata.TitleCrewerFor(ctx, h.Metadata)
	if !ok {
		return []shelfPerson{}
	}
	crews := make([]metadata.TitleCrew, len(titles))
	eachLimited(len(titles), func(i int) {
		var err error
		if crews[i], err = cr.TitleCrew(ctx, titles[i].kind, "tmdb:"+titles[i].tmdb); err != nil {
			missed.Add(1)
		}
	})
	out, have := []shelfPerson{}, map[string]bool{}
	pick := func(people []metadata.Person, n int, dept, note string) {
		for _, m := range people[:min(len(people), n)] {
			if m.ID != "" && m.Profile != "" && !have[m.ID] && len(out) < peopleShelfMax {
				have[m.ID] = true
				out = append(out, shelfPerson{ID: m.ID, Name: m.Name, Department: dept, Profile: m.Profile, Note: note})
			}
		}
	}
	// Title by title, so the cast of the first titles does not crowd out
	// every director and composer.
	for i, crew := range crews {
		pick(crew.Cast, 3, "Acting", "Starring in "+titles[i].name)
		pick(crew.Directors, 1, "Directing", "Directed "+titles[i].name)
		pick(crew.Composers, 1, "Sound", "Music for "+titles[i].name)
	}
	return out
}

// peopleShelves answers GET /api/v1/people/shelves: the People sections
// that have enough people to show, in order, with how many people each
// holds per department so Search can hide a chip with too few.
func (h *Handlers) peopleShelves(w http.ResponseWriter, r *http.Request) {
	p, ok := h.peopleProvider(r, w)
	if !ok {
		return
	}
	out := []map[string]any{}
	for _, id := range []string{"library", "hot"} {
		s, err := h.peopleShelf(r.Context(), id, h.KidMax(r), p)
		if err != nil {
			webserver.Internal(w, "discover", err)
			return
		}
		if len(s.People) < peopleShelfMin {
			continue
		}
		depts := map[string]int{"Acting": 0, "Directing": 0, "Sound": 0}
		for _, sp := range s.People {
			depts[sp.Department]++
		}
		out = append(out, map[string]any{"id": s.ID, "title": s.Title, "count": len(s.People), "departments": depts})
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"shelves": out, "attribution": metadata.Attribution})
}

// peopleShelfOne answers GET /api/v1/people/shelves/{id}: one section's
// people (library or hot), each with the line under their name.
func (h *Handlers) peopleShelfOne(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if peopleShelfTitles[id] == "" {
		webserver.Fail(w, http.StatusNotFound, "no such people section")
		return
	}
	p, ok := h.peopleProvider(r, w)
	if !ok {
		return
	}
	s, err := h.peopleShelf(r.Context(), id, h.KidMax(r), p)
	if err != nil {
		webserver.Internal(w, "discover", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"id": s.ID, "title": s.Title, "people": s.People, "attribution": metadata.Attribution})
}

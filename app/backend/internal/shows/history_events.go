package shows

import (
	"database/sql"

	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// EventView is one History row: an Event with its movie's name.
type EventView struct {
	domain.Event
	TitleName string `json:"title_name,omitempty"`
	// Episode names the episodes of a show Event: S01E02, or S01E01-E03
	// for a pack.
	Episode string `json:"episode,omitempty"`
	// Version names the Version (such as 4K) the Event is about, when its
	// title has more than one.
	Version string `json:"version,omitempty"`
	// ActorName is the member who did it, so History can say who without
	// the admin-only member list.
	ActorName string `json:"actor_name,omitempty"`
}

// parseDay reads a date (2 January 2006) or a full RFC 3339 time.
func parseDay(v string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, v)
}

// ListEvents is History: ?title=<id>&kind=a,b,import_*&from=&to=&limit=&offset=.
// to is a day that is included (or an exact time that is not).
func ListEvents(store *database.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { listEvents(store, w, r) }
}

func listEvents(store *database.Store, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := database.EventFilter{Limit: webserver.QueryLimit(r, 50)}
	if v := q.Get("title"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "title must be a movie or show id")
			return
		}
		f.TitleID = id
	}
	if v := q.Get("episode"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "episode must be an episode id")
			return
		}
		var titleID int64
		if err := store.Read().QueryRowContext(r.Context(), "SELECT title_id FROM items WHERE id = ? AND kind = 'episode'", id).Scan(&titleID); err != nil {
			webserver.Fail(w, http.StatusBadRequest, "no such episode")
			return
		}
		f.TitleID, f.ItemIDs, f.SlotIDs = titleID, []int64{id}, []int64{}
		rows, err := store.Read().QueryContext(r.Context(), "SELECT id FROM slots WHERE item_id = ?", id)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for rows.Next() {
			var sid int64
			if err := rows.Scan(&sid); err != nil {
				rows.Close()
				webserver.Internal(w, "api", err)
				return
			}
			f.SlotIDs = append(f.SlotIDs, sid)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	// season=<n> with title=<id>: the history of one season's episodes.
	if v := q.Get("season"); v != "" && f.TitleID > 0 {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			webserver.Fail(w, http.StatusBadRequest, "season must be a number")
			return
		}
		rows, err := store.Read().QueryContext(r.Context(), "SELECT i.id, s.id FROM items i LEFT JOIN slots s ON s.item_id = i.id WHERE i.title_id = ? AND i.kind = 'episode' AND json_extract(i.numbers, '$.season') = ?", f.TitleID, n)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		f.ItemIDs, f.SlotIDs = []int64{}, []int64{}
		seen := map[int64]bool{}
		for rows.Next() {
			var iid int64
			var sid *int64
			if err := rows.Scan(&iid, &sid); err != nil {
				rows.Close()
				webserver.Internal(w, "api", err)
				return
			}
			if !seen[iid] {
				seen[iid] = true
				f.ItemIDs = append(f.ItemIDs, iid)
			}
			if sid != nil {
				f.SlotIDs = append(f.SlotIDs, *sid)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		if len(f.ItemIDs) == 0 {
			f.ItemIDs = []int64{-1}
		}
	}
	if v := q.Get("album"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "album must be an album id")
			return
		}
		var titleID int64
		if err := store.Read().QueryRowContext(r.Context(), "SELECT title_id FROM parts WHERE id = ? AND kind = 'album'", id).Scan(&titleID); err != nil {
			webserver.Fail(w, http.StatusBadRequest, "no such album")
			return
		}
		f.TitleID, f.PartID, f.ItemIDs, f.SlotIDs = titleID, id, []int64{}, []int64{}
		rows, err := store.Read().QueryContext(r.Context(), "SELECT i.id, s.id FROM items i LEFT JOIN slots s ON s.item_id = i.id WHERE i.part_id = ? AND i.kind = 'track'", id)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		for rows.Next() {
			var iid int64
			var sid sql.NullInt64
			if err := rows.Scan(&iid, &sid); err != nil {
				rows.Close()
				webserver.Internal(w, "api", err)
				return
			}
			f.ItemIDs = append(f.ItemIDs, iid)
			if sid.Valid {
				f.SlotIDs = append(f.SlotIDs, sid.Int64)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
	}
	for _, k := range strings.Split(q.Get("kind"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			// History's default list (every kind but routine) is about 52 long.
			if len(k) > 60 || len(f.Kinds) >= 64 {
				webserver.Fail(w, http.StatusBadRequest, "kind is a comma list of at most 64 Event kinds")
				return
			}
			f.Kinds = append(f.Kinds, k)
		}
	}
	// source=<id>: one Source's grabs and pauses (owners and admins see the Source list).
	if v := q.Get("source"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			webserver.Fail(w, http.StatusBadRequest, "source must be a Source id")
			return
		}
		f.SourceID = id
	}
	// Feed checks are routine: History's today look hides them with feed=0,
	// unless the kinds ask for them.
	f.HideFeed = q.Get("feed") == "0"
	for _, k := range f.Kinds {
		if k == "feed_checked" || k == "feed_*" {
			f.HideFeed = false
		}
	}
	if v := q.Get("from"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "from must be a date like 2026-10-01")
			return
		}
		f.From = t
	}
	if v := q.Get("to"); v != "" {
		t, err := parseDay(v)
		if err != nil {
			webserver.Fail(w, http.StatusBadRequest, "to must be a date like 2026-10-31")
			return
		}
		if len(v) == len("2006-01-02") {
			t = t.AddDate(0, 0, 1)
		}
		f.To = t
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			webserver.Fail(w, http.StatusBadRequest, "offset must be 0 or more")
			return
		}
		f.Offset = n
	}
	// The audit trail (sign-ins, API keys, settings changes) is only for
	// the owner and people who manage users or settings.
	if who, signed := webserver.From(r.Context()); signed && !who.MaySeeAudit() {
		f.HideKinds = database.AuditEventKinds
	}
	// Asks' Events name who asked and who decided, with the decline note:
	// only for people who approve, as on the asks routes.
	if who, signed := webserver.From(r.Context()); signed && !who.Has(webserver.PermApprove) {
		f.HideKinds = append(slices.Clone(f.HideKinds), domain.AskEventKinds...)
	}
	if f.SourceID > 0 {
		ids, err := sources.ReleaseIDs(r.Context(), store.Read(), f.SourceID)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		f.SourceReleaseIDs = ids
	}
	es, total, err := store.Events(r.Context(), f)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// The filtered episode, named for the screen, is read with the page.
	var filtered *domain.Event
	if len(f.ItemIDs) == 1 {
		filtered = &domain.Event{TitleID: &f.TitleID, Data: map[string]any{"item_id": float64(f.ItemIDs[0])}}
	}
	page, err := LoadEventPage(r.Context(), store.Read(), AppendEvent(es, filtered))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	// Member names for the rows a person did (a missing name stays empty).
	var people []int64
	for _, e := range es {
		// An AI app acts with a member's key; its rows name that member too.
		if (e.Actor.Kind == "user" || e.Actor.Kind == "ai") && e.Actor.ID != 0 && !slices.Contains(people, e.Actor.ID) {
			people = append(people, e.Actor.ID)
		}
	}
	names := map[int64]string{}
	_ = eachIn(r.Context(), store.Read(), `SELECT id, name FROM users WHERE id IN (%s)`, people, func(rs *sql.Rows) error {
		var id int64
		var n string
		if err := rs.Scan(&id, &n); err != nil {
			return err
		}
		names[id] = n
		return nil
	})
	out := make([]EventView, 0, len(es))
	for _, e := range es {
		v := EventView{Event: e}
		if e.Actor.Kind == "user" || e.Actor.Kind == "ai" {
			v.ActorName = names[e.Actor.ID]
		}
		if e.TitleID != nil {
			v.TitleName = page.TitleName(e)
			v.Episode = page.Episode(e)
			v.Version = page.Version(e)
		}
		out = append(out, v)
	}
	resp := map[string]any{"events": out, "total": total, "limit": f.Limit, "offset": f.Offset}
	if filtered != nil {
		resp["episode"] = page.Episode(*filtered)
	}
	webserver.WriteJSON(w, http.StatusOK, resp)
}

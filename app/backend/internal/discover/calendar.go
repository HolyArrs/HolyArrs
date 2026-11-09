package discover

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// viewer is the signed-in person's id (1, the owner, when none: as api).
func viewer(ctx context.Context) int64 {
	if who, ok := webserver.From(ctx); ok {
		return who.UserID
	}
	return 1
}

// calendar lists cinema, digital and physical dates of Library movies in
// [from, to); dates are YYYY-MM-DD, default today to today + 30 days.
func (h *Handlers) calendar(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from := time.Now().UTC().Truncate(24 * time.Hour)
	to := from.AddDate(0, 0, 30)
	var err error
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse("2006-01-02", v); err != nil {
			webserver.Fail(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
			return
		}
		if q.Get("to") == "" {
			to = from.AddDate(0, 0, 30)
		}
	}
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse("2006-01-02", v); err != nil {
			webserver.Fail(w, http.StatusBadRequest, "to must be YYYY-MM-DD")
			return
		}
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		webserver.Fail(w, http.StatusBadRequest, "to must be after from, at most a year later")
		return
	}
	titles, ok := CalendarTitles(q.Get("titles"))
	if !ok {
		webserver.Fail(w, http.StatusBadRequest, "titles must be all, movies, shows or music")
		return
	}
	es, err := library.Calendar(r.Context(), h.Store.Read(), from, to, titles)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if max := h.KidMax(r); max != "" {
		ids := make([]int64, len(es))
		for i, e := range es {
			ids[i] = e.TitleID
		}
		ok, err := h.AllowedTitleIDs(r.Context(), max, ids)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		kept := []library.CalendarEntry{}
		for _, e := range es {
			if ok[e.TitleID] {
				kept = append(kept, e)
			}
		}
		es = kept
	}
	for i := range es {
		if es[i].Poster != "" {
			es[i].Poster = ArtworkLinks([]string{es[i].Poster})[0]
		}
		// An album carries its artist's genres when already known (read at
		// start for every Library artist); the calendar never looks them up.
		if es[i].Kind == "album" && es[i].ArtistMBID != "" {
			es[i].Genres = h.KnownGenres(es[i].ArtistMBID)
		}
	}
	out, mine, people, err := h.calendarFollows(r.Context(), es)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	h.calendarArt(r, out)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02"), "titles": titles, "dates": out,
		"my_follows": mine, "people": people})
}

// calendarRow is a calendar date with who follows its title: following
// says the viewer does; followers are everyone who follows it.
type calendarRow struct {
	library.CalendarEntry
	Following bool               `json:"following,omitempty"`
	Followers []calendarFollower `json:"followers,omitempty"`
	// Banner is the List view's wide picture (textless when a logo goes
	// over it); Logo is the title or artist logo. Both may be empty.
	Banner string `json:"banner,omitempty"`
	Logo   string `json:"logo,omitempty"`
}

// calendarArt adds the List view's banner pictures. Movies and shows get
// the stored card pick (a textless backdrop and the title logo, from
// card_art); albums get the artist's wide picture and logo through our
// metadata server's picture route. Only stored data is read: no outside
// call on the request, and a title without a pick keeps its backdrop.
func (h *Handlers) calendarArt(r *http.Request, rows []calendarRow) {
	lang := metadata.TrailerLanguage(r)
	ids := map[int64]string{}
	var args []any
	for _, e := range rows {
		if e.Kind != "album" {
			if _, ok := ids[e.TitleID]; !ok {
				ids[e.TitleID] = ""
				args = append(args, e.TitleID)
			}
		}
	}
	db := h.Store.Read()
	for len(args) > 0 {
		chunk := args[:min(len(args), 500)]
		args = args[len(chunk):]
		q, err := db.QueryContext(r.Context(), `SELECT id, kind, COALESCE(CAST(json_extract(source_ids, '$.tmdb') AS TEXT), '') FROM titles
			WHERE id IN (?`+strings.Repeat(", ?", len(chunk)-1)+`)`, chunk...)
		if err != nil {
			return
		}
		for q.Next() {
			var id int64
			var kind string
			var tmdb string
			if q.Scan(&id, &kind, &tmdb) == nil {
				ids[id] = kind + ":" + tmdb
			}
		}
		_ = q.Close()
	}
	var keys [][2]string
	for _, k := range ids {
		if kind, id, ok := strings.Cut(k, ":"); ok && id != "" && (kind == "movie" || kind == "show") {
			keys = append(keys, [2]string{kind, id})
		}
	}
	got := h.ReadCardArt(r.Context(), keys, lang)
	for i := range rows {
		e := &rows[i]
		if e.Kind == "album" {
			if e.ArtistMBID != "" {
				e.Banner = music.ArtPrefix + "wide/" + e.ArtistMBID
				e.Logo = music.ArtPrefix + "logo/" + e.ArtistMBID
			}
			continue
		}
		kind, id, _ := strings.Cut(ids[e.TitleID], ":")
		row, ok := got[CardKey(kind, id, lang)]
		if !ok {
			row, ok = got[CardKey(kind, id, "en")]
		}
		if !ok {
			continue
		}
		e.Logo = row.Logo
		// A backdrop with the title printed on it is never used under a logo.
		if row.Art != "" && !row.HasTitle {
			e.Banner = row.Art
		}
	}
}

type calendarFollower struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	Avatar *string `json:"avatar,omitempty"`
	Me     bool    `json:"me,omitempty"`
}

// calendarFollows adds who follows each date's title, and says how many
// titles the viewer follows in all and how many people use this server.
func (h *Handlers) calendarFollows(ctx context.Context, es []library.CalendarEntry) ([]calendarRow, int, int, error) {
	db := h.Store.Read()
	me := viewer(ctx)
	var mine, people int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM follows f JOIN titles t ON t.id = f.title_id
		WHERE f.user_id = ? AND t.state != 'removed'`, me).Scan(&mine); err != nil {
		return nil, 0, 0, err
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM users WHERE state = 'active'").Scan(&people); err != nil {
		return nil, 0, 0, err
	}
	type who struct {
		seen []calendarFollower
		me   bool
	}
	by := map[int64]*who{}
	var ids []any
	for _, e := range es {
		if by[e.TitleID] == nil {
			by[e.TitleID] = &who{}
			ids = append(ids, e.TitleID)
		}
	}
	for len(ids) > 0 {
		chunk := ids[:min(len(ids), 500)]
		ids = ids[len(chunk):]
		rows, err := db.QueryContext(ctx, `SELECT f.title_id, u.id, u.name, (SELECT etag FROM user_avatars WHERE user_id = u.id)
			FROM follows f JOIN users u ON u.id = f.user_id
			WHERE u.state != 'disabled' AND f.title_id IN (?`+strings.Repeat(", ?", len(chunk)-1)+`) ORDER BY f.created_at, u.id`, chunk...)
		if err != nil {
			return nil, 0, 0, err
		}
		for rows.Next() {
			var tid, uid int64
			var name string
			var etag sql.NullString
			if err := rows.Scan(&tid, &uid, &name, &etag); err != nil {
				rows.Close()
				return nil, 0, 0, err
			}
			w := by[tid]
			if uid == me {
				w.me = true
			}
			f := calendarFollower{ID: uid, Name: strings.TrimSpace(name), Me: uid == me}
			if etag.Valid && etag.String != "" {
				u := "/api/v1/users/" + strconv.FormatInt(uid, 10) + "/avatar?v=" + etag.String
				f.Avatar = &u
			}
			w.seen = append(w.seen, f)
		}
		if err := rows.Close(); err != nil {
			return nil, 0, 0, err
		}
	}
	out := make([]calendarRow, len(es))
	for i, e := range es {
		w := by[e.TitleID]
		out[i] = calendarRow{CalendarEntry: e, Following: w.me, Followers: w.seen}
	}
	return out, mine, people, nil
}

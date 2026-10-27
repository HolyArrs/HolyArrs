package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// Home rows, in the words the web app keys on.
const (
	homeWelcome   = "welcome"
	homeAttention = "attention"
	homeArrived   = "arrived"
	homeApproval  = "approval"
	homeAsks      = "asks"
	homeNewSeason = "new_season"
	homeFollowing = "following"
	homeSoon      = "soon"
	homeDiscover  = "discover"
)

const (
	homeArrivedDays = 14 // Just arrived reaches back two weeks
	homeSeasonDays  = 30 // New season: a season that started or starts within a month
	homeCardsMax    = 20
)

// homeViewer is who looks at Home: a Kid, an owner (runs the Library) or
// a member, and whether they decide other people's asks.
type homeViewer struct {
	Owner, Approver, Kid bool
}

// homeHas says which rows the server already knows to be empty. A row the
// server does not fill (asks, approval) is listed and the web
// app hides it when it has nothing.
type homeHas struct {
	Empty                                          bool // nothing in the Library and nobody asked yet
	Attention, Arrived, NewSeason, Following, Soon bool
}

// homeRows is Home's row order for one viewer. Members never get the
// owner rows; a Kid gets only Just arrived, their asks and new seasons;
// an empty household gets one welcome card.
func homeRows(v homeViewer, has homeHas) []string {
	if has.Empty {
		return []string{homeWelcome}
	}
	var out []string
	add := func(row string, ok bool) {
		if ok {
			out = append(out, row)
		}
	}
	if v.Kid {
		add(homeArrived, has.Arrived)
		add(homeAsks, true)
		add(homeNewSeason, has.NewSeason)
		return out
	}
	add(homeAttention, v.Owner && has.Attention)
	add(homeArrived, has.Arrived)
	add(homeApproval, v.Approver)
	add(homeAsks, true)
	add(homeNewSeason, has.NewSeason)
	add(homeFollowing, has.Following)
	add(homeSoon, has.Soon)
	add(homeDiscover, true)
	return out
}

// homeAlbum is one new album on an artist's Just arrived card.
type homeAlbum struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Cover string `json:"cover"`
}

// homeCard is one card on a Home row the server fills.
type homeCard struct {
	Key      string      `json:"key"`
	Kind     string      `json:"kind"`
	TitleID  int64       `json:"title_id"`
	Name     string      `json:"name"`
	Year     *int        `json:"year,omitempty"`
	Line     string      `json:"line,omitempty"`
	Href     string      `json:"href"`
	Backdrop string      `json:"backdrop,omitempty"`
	Poster   string      `json:"poster,omitempty"`
	Logo     string      `json:"logo,omitempty"`
	Season   int         `json:"season,omitempty"`
	Albums   []homeAlbum `json:"albums,omitempty"`
	At       string      `json:"at,omitempty"`
}

type homeArt struct {
	backdrop, poster, logo string
}

func homeArtOf(backdrop, logo string, artwork sql.NullString) homeArt {
	a := homeArt{backdrop: backdrop, logo: logo}
	var posters []string
	if artwork.Valid && json.Unmarshal([]byte(artwork.String), &posters) == nil {
		if links := artworkLinks(posters); len(links) > 0 {
			a.poster = links[0]
		}
	}
	return a
}

// home answers GET /api/v1/home: the row order for the signed-in person
// and the rows only the server can fill (Just arrived, New season,
// Following, Coming soon).
func (a *API) home(w http.ResponseWriter, r *http.Request) {
	who, signed := auth.From(r.Context())
	v := homeViewer{Owner: !signed || who.Has(auth.PermManageLibrary), Approver: !signed || who.Has(auth.PermApprove), Kid: a.kidMax(r) != ""}
	if v.Kid {
		v.Owner, v.Approver = false, false
	}
	ctx, db := r.Context(), a.s.Store.Read()
	where, args := a.homeWhere(r)
	var titles, asked int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM titles WHERE state != 'removed'").Scan(&titles); err != nil {
		internal(w, err)
		return
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM asks").Scan(&asked); err != nil {
		internal(w, err)
		return
	}
	now := time.Now()
	arrived, err := homeArrivals(ctx, db, where, args, now)
	if err != nil {
		internal(w, err)
		return
	}
	seasons, err := homeNewSeasons(ctx, db, where, args, now)
	if err != nil {
		internal(w, err)
		return
	}
	following := []homeCard{}
	if !v.Kid {
		if following, err = homeFollows(ctx, db, where, args, a.person(r).UserID); err != nil {
			internal(w, err)
			return
		}
	}
	soon := []homeCard{}
	if !v.Kid {
		if soon, err = homeComingSoon(ctx, db, where, args, now); err != nil {
			internal(w, err)
			return
		}
	}
	// A show in New season is not repeated in Coming soon.
	inSeasons := map[int64]bool{}
	for _, c := range seasons {
		inSeasons[c.TitleID] = true
	}
	kept := soon[:0]
	for _, c := range soon {
		if !inSeasons[c.TitleID] {
			kept = append(kept, c)
		}
	}
	soon = kept
	progress, err := homeAskProgress(ctx, db, a.person(r).UserID)
	if err != nil {
		internal(w, err)
		return
	}
	failed := 0
	if v.Owner {
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM slots WHERE state = 'failed'").Scan(&failed); err != nil {
			internal(w, err)
			return
		}
		if !signed || who.Has(auth.PermSettings) {
			var sick int
			if err := db.QueryRowContext(ctx, "SELECT count(*) FROM health_checks WHERE confirmed != 'ok'").Scan(&sick); err != nil {
				internal(w, err)
				return
			}
			failed += sick
		}
	}
	library, err := homeLibraryCounts(ctx, db, where, args, a.homeKinds())
	if err != nil {
		internal(w, err)
		return
	}
	has := homeHas{Empty: titles == 0 && asked == 0, Attention: failed > 0, Arrived: len(arrived) > 0, NewSeason: len(seasons) > 0, Following: len(following) > 0, Soon: len(soon) > 0}
	writeJSON(w, http.StatusOK, map[string]any{
		"rows": homeRows(v, has), "owner": v.Owner, "kid": v.Kid,
		"arrived": arrived, "new_seasons": seasons, "following": following, "soon": soon,
		"ask_progress": progress, "library": library,
	})
}

// homeWhere limits Home to the titles of the parts that are on and, for a
// Kid, to the titles their limit allows (columns on alias t).
func (a *API) homeWhere(r *http.Request) (string, []any) {
	var marks []string
	var args []any
	for _, kind := range a.homeKinds() {
		marks = append(marks, "?")
		args = append(args, kind)
	}
	if len(marks) == 0 {
		return "0", nil
	}
	where := "t.state != 'removed' AND t.kind IN (" + strings.Join(marks, ",") + ")"
	if kid, kidArgs := kidSQL(a.kidMax(r)); kid != "" {
		where += " AND " + kid
		args = append(args, kidArgs...)
	}
	return where, args
}

// homeKinds are the title kinds whose part is on (every kind in safe mode).
func (a *API) homeKinds() []string {
	var out []string
	for _, kind := range []string{"movie", "show", "artist"} {
		part, _ := parts.ForTitleKind(kind)
		if a.partIsOn(part) || (a.s.Parts != nil && a.s.Parts.SafeMode) {
			out = append(out, kind)
		}
	}
	return out
}

// homeLibraryKind is what the Library holds of one kind, for Your Library
// on Home. Titles are movies, shows or artists; Parts are numbered seasons
// or included albums; Items are movies, episodes or tracks. OnDisk,
// Missing and Downloading count items of the main copy: on disk has a
// file, missing is out (kept up to date) with no file and nothing on the
// way, downloading is on its way. Size is the bytes of those files.
type homeLibraryKind struct {
	Titles      int   `json:"titles"`
	Parts       int   `json:"parts"`
	Items       int   `json:"items"`
	OnDisk      int   `json:"on_disk"`
	Missing     int   `json:"missing"`
	Downloading int   `json:"downloading"`
	Size        int64 `json:"size"`
}

// homeLibraryCounts counts the Library per kind in one query, for the kinds
// that are on (a kind that is on but empty is all zeros). Dropped seasons,
// albums and episodes, and albums left out of an artist, are not counted.
func homeLibraryCounts(ctx context.Context, db *sql.DB, where string, args []any, kinds []string) (map[string]homeLibraryKind, error) {
	out := map[string]homeLibraryKind{}
	for _, k := range kinds {
		out[k] = homeLibraryKind{}
	}
	rows, err := db.QueryContext(ctx, `SELECT t.kind, count(DISTINCT t.id),
			count(DISTINCT CASE WHEN (p.kind = 'season' AND p.number > 0) OR p.kind = 'album' THEN p.id END),
			count(i.id),
			COALESCE(sum(s.state IN ('in_library', 'better_wanted')), 0),
			COALESCE(sum((s.state = 'wanted' AND i.keep_up_to_date = 1) OR s.state IN ('searching', 'held', 'failed', 'needs_a_decision', 'rejected')), 0),
			COALESCE(sum(s.state IN ('chosen', 'downloading', 'stalled', 'importing')), 0),
			COALESCE(sum(CASE WHEN f.state = 'present' AND s.state IN ('in_library', 'better_wanted') THEN f.size END), 0)
		FROM titles t
		LEFT JOIN items i ON i.title_id = t.id AND i.dropped_at IS NULL AND (i.part_id IS NULL OR i.part_id IN
			(SELECT x.id FROM parts x WHERE x.title_id = t.id AND x.dropped_at IS NULL AND (x.kind != 'album' OR x.included = 1)))
		LEFT JOIN parts p ON p.id = i.part_id
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT v.id FROM versions v WHERE v.title_id = t.id ORDER BY v.is_main DESC, v.id LIMIT 1)
		LEFT JOIN files f ON f.id = s.file_id
		WHERE `+where+`
		GROUP BY t.kind`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var c homeLibraryKind
		if err := rows.Scan(&kind, &c.Titles, &c.Parts, &c.Items, &c.OnDisk, &c.Missing, &c.Downloading, &c.Size); err != nil {
			return nil, err
		}
		out[kind] = c
	}
	return out, rows.Err()
}

// homeArrivals is Just arrived: the latest additions, newest first, never
// empty while the Library has files (the row's title says how fresh). A show is one card that says what is new, an
// artist is one card with its new albums, a movie is one card.
func homeArrivals(ctx context.Context, db *sql.DB, where string, args []any, now time.Time) ([]homeCard, error) {
	cut := store.FormatTime(now.AddDate(0, 0, -homeArrivedDays))
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT t.id, t.kind, t.name, t.year, t.backdrop, t.logo, t.artwork, t.created_at,
			i.id, COALESCE(p.id, 0), COALESCE(p.number, 0), COALESCE(p.name, ''), f.added_at
		FROM slots s JOIN files f ON f.id = s.file_id JOIN items i ON i.id = s.item_id
		JOIN titles t ON t.id = i.title_id LEFT JOIN parts p ON p.id = i.part_id
		WHERE f.state = 'present' AND f.role = 'main' AND `+where+`
		ORDER BY f.added_at DESC LIMIT 2000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type part struct {
		number int
		name   string
		items  map[int64]bool
	}
	type group struct {
		card    homeCard
		created string
		parts   map[int64]*part
		order   []int64
	}
	groups := map[int64]*group{}
	var order []int64
	for rows.Next() {
		var c homeCard
		var backdrop, logo, created, at, partName string
		var art sql.NullString
		var itemID, partID int64
		var number int
		if err := rows.Scan(&c.TitleID, &c.Kind, &c.Name, &c.Year, &backdrop, &logo, &art, &created, &itemID, &partID, &number, &partName, &at); err != nil {
			return nil, err
		}
		g := groups[c.TitleID]
		if g == nil {
			ha := homeArtOf(backdrop, logo, art)
			c.Backdrop, c.Poster, c.Logo, c.At = ha.backdrop, ha.poster, ha.logo, at
			g = &group{card: c, created: created, parts: map[int64]*part{}}
			groups[c.TitleID] = g
			order = append(order, c.TitleID)
		}
		p := g.parts[partID]
		if p == nil {
			p = &part{number: number, name: partName, items: map[int64]bool{}}
			g.parts[partID] = p
			g.order = append(g.order, partID)
		}
		p.items[itemID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	out := []homeCard{}
	for _, id := range order {
		if len(out) == homeCardsMax {
			break
		}
		g := groups[id]
		c := g.card
		c.Key = fmt.Sprintf("a%d", id)
		c.Href = fmt.Sprintf("/library/%d", id)
		switch c.Kind {
		case "show":
			// The newest season with new episodes is the one the card names.
			var top *part
			for _, pid := range g.order {
				if p := g.parts[pid]; top == nil || p.number > top.number {
					top = p
				}
			}
			var aired int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM items i JOIN parts p ON p.id = i.part_id
				WHERE i.title_id = ? AND p.kind = 'season' AND p.number = ? AND i.air_date IS NOT NULL AND i.air_date <= ?`,
				id, top.number, store.FormatTime(now)).Scan(&aired); err != nil {
				return nil, err
			}
			newCount := len(top.items)
			c.Season = top.number
			c.Href += fmt.Sprintf("?season=%d", top.number)
			c.Line = homeShowLine(g.created >= store.FormatTime(now.AddDate(0, 0, -homeArrivedDays)), top.number, newCount, aired)
		case "artist":
			var albums []homeAlbum
			for _, pid := range g.order {
				if pid != 0 {
					albums = append(albums, homeAlbum{ID: pid, Name: g.parts[pid].name, Cover: fmt.Sprintf("/api/v1/albums/%d/cover", pid)})
				}
			}
			c.Albums = albums
			c.Line = homeArtistLine(len(albums), albums)
		default:
			c.Line = "Movie"
			if c.At >= cut {
				c.Line = "New movie"
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// homeShowLine says what is new on a show: the whole show, a whole season
// or some episodes of the season.
func homeShowLine(newShow bool, season, fresh, inSeason int) string {
	switch {
	case newShow:
		return "New show"
	case fresh > 1 && fresh >= inSeason:
		return fmt.Sprintf("Season %d added", season)
	case fresh == 1:
		return fmt.Sprintf("S%d · 1 new episode", season)
	default:
		return fmt.Sprintf("S%d · %d new episodes", season, fresh)
	}
}

// homeArtistLine names an artist's new albums: the album, or how many.
func homeArtistLine(n int, albums []homeAlbum) string {
	switch {
	case n == 1:
		return albums[0].Name
	case n > 1:
		return fmt.Sprintf("%d new albums", n)
	}
	return "New music"
}

// homeNewSeasons is New season of a show you have: a later season of a
// Library show whose first episode aired in the last month or airs in the
// next one, newest first, one card per show.
func homeNewSeasons(ctx context.Context, db *sql.DB, where string, args []any, now time.Time) ([]homeCard, error) {
	from, until := store.FormatTime(now.AddDate(0, 0, -homeSeasonDays)), store.FormatTime(now.AddDate(0, 0, homeSeasonDays))
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.name, t.year, t.backdrop, t.logo, t.artwork, p.number, min(i.air_date) AS first
		FROM parts p JOIN titles t ON t.id = p.title_id JOIN items i ON i.part_id = p.id
		WHERE p.kind = 'season' AND p.number > 1 AND t.kind = 'show' AND i.air_date IS NOT NULL AND `+where+`
		GROUP BY p.id HAVING first >= ? AND first <= ?
		ORDER BY first DESC, t.id`, append(args, from, until)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	out := []homeCard{}
	for rows.Next() {
		var c homeCard
		var backdrop, logo, first string
		var art sql.NullString
		if err := rows.Scan(&c.TitleID, &c.Name, &c.Year, &backdrop, &logo, &art, &c.Season, &first); err != nil {
			return nil, err
		}
		if seen[c.TitleID] || len(out) == homeCardsMax {
			continue
		}
		seen[c.TitleID] = true
		ha := homeArtOf(backdrop, logo, art)
		c.Kind, c.Backdrop, c.Poster, c.Logo, c.At = "show", ha.backdrop, ha.poster, ha.logo, first
		c.Key = fmt.Sprintf("s%d", c.TitleID)
		c.Href = fmt.Sprintf("/library/%d?season=%d", c.TitleID, c.Season)
		c.Line = fmt.Sprintf("Season %d · out now", c.Season)
		if t, err := time.Parse(time.RFC3339, first); err == nil && t.After(now) {
			c.Line = fmt.Sprintf("Season %d · %s", c.Season, t.Format("Jan 2"))
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// homeFollows is Following: the shows and artists the person follows,
// most recently followed first.
func homeFollows(ctx context.Context, db *sql.DB, where string, args []any, userID int64) ([]homeCard, error) {
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.kind, t.name, t.year, t.backdrop, t.logo, t.artwork
		FROM follows f JOIN titles t ON t.id = f.title_id
		WHERE f.user_id = ? AND `+where+` ORDER BY f.created_at DESC, t.id LIMIT ?`, append(append([]any{userID}, args...), homeCardsMax)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []homeCard{}
	for rows.Next() {
		var c homeCard
		var backdrop, logo string
		var art sql.NullString
		if err := rows.Scan(&c.TitleID, &c.Kind, &c.Name, &c.Year, &backdrop, &logo, &art); err != nil {
			return nil, err
		}
		ha := homeArtOf(backdrop, logo, art)
		c.Backdrop, c.Poster, c.Logo = ha.backdrop, ha.poster, ha.logo
		c.Key = fmt.Sprintf("f%d", c.TitleID)
		c.Href = fmt.Sprintf("/library/%d", c.TitleID)
		out = append(out, c)
	}
	return out, rows.Err()
}

// homeProgress is the live download state of one asked title.
type homeProgress struct {
	Progress float64 `json:"progress"`
	State    string  `json:"state"`
	ETA      *int64  `json:"eta,omitempty"`
}

// homeAskProgress is the download progress of the titles the person asked
// for, by title id: the average over the title's active downloads, so
// Your asks shows live progress without the Downloads screen.
func homeAskProgress(ctx context.Context, db *sql.DB, userID int64) (map[string]homeProgress, error) {
	rows, err := db.QueryContext(ctx, `SELECT i.title_id, avg(d.progress), max(d.state = 'downloading'), max(d.eta)
		FROM slots s JOIN items i ON i.id = s.item_id JOIN downloads d ON d.id = s.active_download_id
		WHERE d.state IN ('sent', 'queued', 'downloading', 'stalled') AND i.title_id IN
			(SELECT k.title_id FROM asks k LEFT JOIN ask_people ap ON ap.ask_id = k.id
			 WHERE k.title_id IS NOT NULL AND (k.asked_by = ? OR ap.user_id = ?))
		GROUP BY i.title_id`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]homeProgress{}
	for rows.Next() {
		var id int64
		var p homeProgress
		var moving bool
		var eta sql.NullInt64
		if err := rows.Scan(&id, &p.Progress, &moving, &eta); err != nil {
			return nil, err
		}
		p.State = "queued"
		if moving {
			p.State = "downloading"
		}
		if eta.Valid {
			p.ETA = &eta.Int64
		}
		out[fmt.Sprint(id)] = p
	}
	return out, rows.Err()
}

// homeComingSoon is Coming soon: the next episode of each Library show and
// the next date of each Library movie in the coming month, soonest first.
func homeComingSoon(ctx context.Context, db *sql.DB, where string, args []any, now time.Time) ([]homeCard, error) {
	from, until := store.FormatTime(now), store.FormatTime(now.AddDate(0, 0, homeSeasonDays))
	day := func(v string) string { return v[:min(len(v), 10)] }
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.kind, t.name, t.year, t.backdrop, t.logo, t.artwork, min(x.at), x.season, x.episode FROM (
			SELECT i.title_id AS id, i.air_date AS at, json_extract(i.numbers, '$.season') AS season, json_extract(i.numbers, '$.episode') AS episode
				FROM items i WHERE i.kind = 'episode' AND i.air_date >= ? AND i.air_date <= ?
			UNION ALL SELECT id, d, NULL, NULL FROM (
				SELECT id, cinema_date AS d FROM titles UNION ALL SELECT id, digital_date FROM titles UNION ALL SELECT id, physical_date FROM titles)
				WHERE d >= ? AND d <= ?
		) x JOIN titles t ON t.id = x.id WHERE `+where+`
		GROUP BY t.id ORDER BY min(x.at), t.id LIMIT ?`, append(append([]any{from, until, day(from), until}, args...), homeCardsMax)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []homeCard{}
	for rows.Next() {
		var c homeCard
		var backdrop, logo, at string
		var art sql.NullString
		var season, episode sql.NullInt64
		if err := rows.Scan(&c.TitleID, &c.Kind, &c.Name, &c.Year, &backdrop, &logo, &art, &at, &season, &episode); err != nil {
			return nil, err
		}
		ha := homeArtOf(backdrop, logo, art)
		c.Backdrop, c.Poster, c.Logo, c.At = ha.backdrop, ha.poster, ha.logo, at
		c.Key = fmt.Sprintf("c%d", c.TitleID)
		c.Href = fmt.Sprintf("/library/%d", c.TitleID)
		when := day(at)
		if t, err := time.Parse("2006-01-02", when); err == nil {
			when = t.Format("Jan 2")
		}
		c.Line = "Out " + when
		if season.Valid && episode.Valid {
			c.Season = int(season.Int64)
			c.Line = fmt.Sprintf("S%dE%d · %s", season.Int64, episode.Int64, when)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

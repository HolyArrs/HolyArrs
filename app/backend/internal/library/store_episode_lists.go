package library

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The reads below serve the arr API's lists.

// idTuple writes ids as an SQL list: (1,2,3).
func idTuple(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return "(" + strings.Join(parts, ",") + ")"
}

// SeasonPartID finds the season Part of a show by its number.
func SeasonPartID(ctx context.Context, q database.Querier, titleID int64, number int) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT id FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?", titleID, number).Scan(&id)
	return id, err
}

// EpisodeSearchSlots lists the Slots of the given episodes of shows that are
// still in the Library, in id order.
func EpisodeSearchSlots(ctx context.Context, q database.Lister, episodeIDs []int64) ([]mediafiles.Slot, error) {
	return database.Select[mediafiles.Slot](ctx, q, "slots", `item_id IN (SELECT i.id FROM items i JOIN titles t ON t.id = i.title_id WHERE i.kind = 'episode' AND t.kind = 'show'
			AND t.state != 'removed' AND i.id IN `+idTuple(episodeIDs)+`) ORDER BY id`)
}

// MovieSearchSlots lists the Slots of the given movies that are still in the
// Library, in id order.
func MovieSearchSlots(ctx context.Context, q database.Lister, titleIDs []int64) ([]mediafiles.Slot, error) {
	return database.Select[mediafiles.Slot](ctx, q, "slots", `item_id IN (SELECT i.id FROM items i JOIN titles t ON t.id = i.title_id WHERE t.kind = 'movie'
			AND t.state != 'removed' AND t.id IN `+idTuple(titleIDs)+`) ORDER BY id`)
}

// EpisodeFilter narrows the episodes EpisodeRows reads.
type EpisodeFilter struct {
	// Where is a condition on the episode's own columns (an SQL fragment the
	// caller writes, with Args for its marks).
	Where string
	Args  []any
	// KeptShows leaves out the episodes that are not kept up to date and
	// those of shows that are not.
	KeptShows bool
}

// EpisodeRows reads the episodes of shows in the Library, in show, season and
// episode order.
func EpisodeRows(ctx context.Context, q database.Lister, f EpisodeFilter) ([]mediafiles.Item, error) {
	where := f.Where
	if f.KeptShows {
		where += " AND keep_up_to_date = 1 AND title_id IN (SELECT id FROM titles WHERE keep_up_to_date = 1)"
	}
	return database.Select[mediafiles.Item](ctx, q, "items", "kind = 'episode' AND dropped_at IS NULL AND title_id IN (SELECT id FROM titles WHERE kind = 'show' AND state != 'removed') AND "+
		where+" ORDER BY title_id, json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')", f.Args...)
}

// MainFileOfItems says which File each of the items holds in its title's main
// Version; an item without a File is left out.
func MainFileOfItems(ctx context.Context, q database.Lister, itemIDs []int64) (map[int64]int64, error) {
	files := map[int64]int64{}
	rows, err := q.QueryContext(ctx, `SELECT s.item_id, s.file_id FROM slots s JOIN versions v ON v.id = s.version_id AND v.is_main = 1
		WHERE s.file_id IS NOT NULL AND s.item_id IN `+idTuple(itemIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item, file int64
		if err := rows.Scan(&item, &file); err != nil {
			return nil, err
		}
		files[item] = file
	}
	return files, rows.Err()
}

// MovieMainFiles reads the Files held in the main Version of the given movies.
func MovieMainFiles(ctx context.Context, q database.Lister, titleIDs []int64) ([]mediafiles.File, error) {
	return database.Select[mediafiles.File](ctx, q, "files", `id IN (SELECT s.file_id FROM slots s JOIN items i ON i.id = s.item_id
		JOIN versions v ON v.id = s.version_id AND v.is_main = 1 WHERE i.kind = 'movie' AND s.file_id IS NOT NULL AND i.title_id IN `+idTuple(titleIDs)+`)`)
}

// SeasonCount is what the episodes of one season add up to.
type SeasonCount struct {
	TitleID       int64
	Season        int
	Total         int
	Wanted        int
	WithFile      int
	Size          int64
	NextAiring    *string
	PreviousAired *string
	ReleaseGroups string // comma separated
	LastAired     *string
}

// SeasonCounts adds up the episodes of every season of the shows in titleIDs
// in one grouped query. now tells aired from not yet aired.
func SeasonCounts(ctx context.Context, q database.Lister, titleIDs []int64, now time.Time) ([]SeasonCount, error) {
	at := database.FormatTime(now)
	rows, err := q.QueryContext(ctx, `SELECT i.title_id, COALESCE(json_extract(i.numbers, '$.season'), 0) AS sn, count(*),
		sum(s.file_id IS NOT NULL OR (i.keep_up_to_date = 1 AND i.air_date IS NOT NULL AND i.air_date <= ?)),
		sum(s.file_id IS NOT NULL), COALESCE(sum(f.size), 0),
		min(CASE WHEN i.keep_up_to_date = 1 AND i.air_date > ? THEN i.air_date END),
		max(CASE WHEN i.keep_up_to_date = 1 AND i.air_date <= ? THEN i.air_date END),
		COALESCE(group_concat(DISTINCT f.release_group), ''), max(CASE WHEN i.air_date <= ? THEN i.air_date END)
		FROM items i
		LEFT JOIN versions v ON v.title_id = i.title_id AND v.is_main = 1
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = v.id
		LEFT JOIN files f ON f.id = s.file_id
		WHERE i.kind = 'episode' AND i.dropped_at IS NULL AND i.title_id IN `+idTuple(titleIDs)+` GROUP BY i.title_id, sn`, at, at, at, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeasonCount
	for rows.Next() {
		var c SeasonCount
		if err := rows.Scan(&c.TitleID, &c.Season, &c.Total, &c.Wanted, &c.WithFile, &c.Size, &c.NextAiring, &c.PreviousAired, &c.ReleaseGroups, &c.LastAired); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SlotItem says which item and title a Slot belongs to.
type SlotItem struct {
	SlotID   int64
	State    string
	ItemID   int64
	TitleID  int64
	ItemKind string
	Season   *int
}

// SlotItems reads the items of the given Slots, leaving out those of removed
// titles.
func SlotItems(ctx context.Context, q database.Lister, slotIDs []int64) ([]SlotItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT s.id, s.state, i.id, i.title_id, i.kind, json_extract(i.numbers, '$.season') FROM slots s
		JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE t.state != 'removed' AND s.id IN `+idTuple(slotIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SlotItem
	for rows.Next() {
		var s SlotItem
		if err := rows.Scan(&s.SlotID, &s.State, &s.ItemID, &s.TitleID, &s.ItemKind, &s.Season); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// keptWhere is the "monitored" rule of the lists: the episode, its season
// and its show are all kept up to date.
const keptWhere = `(i.keep_up_to_date = 1 AND t.keep_up_to_date = 1
	AND (i.part_id IS NULL OR EXISTS (SELECT 1 FROM parts p WHERE p.id = i.part_id AND p.keep_up_to_date = 1)))`

// MissingSortColumns are the columns the missing list may sort on.
var MissingSortColumns = map[string]bool{"i.air_date": true, "i.last_searched_at": true, "t.sort_name": true}

// ErrMissingSort refuses a sort column that is not in MissingSortColumns.
var ErrMissingSort = errors.New("unknown sort column for the missing list")

// MissingPage is one page of the episodes that have aired and have no File in
// the main Version.
type MissingPage struct {
	IDs   []int64
	Total int
}

// MissingReader is a pool or a transaction that can read rows.
type MissingReader interface {
	database.Lister
	database.Querier
}

// MissingEpisodes pages the missing episodes. kept false gives the episodes
// that are not kept up to date instead. The sort column must be one of
// MissingSortColumns; desc sorts from the biggest value; a page past the end
// is empty.
func MissingEpisodes(ctx context.Context, q MissingReader, kept bool, sortColumn string, desc bool, now time.Time, page, size int) (MissingPage, error) {
	if !MissingSortColumns[sortColumn] {
		return MissingPage{}, ErrMissingSort
	}
	sqlDir := "ASC"
	if desc {
		sqlDir = "DESC"
	}
	filter := keptWhere
	if !kept {
		filter = "NOT " + keptWhere
	}
	from := `FROM items i JOIN titles t ON t.id = i.title_id JOIN versions v ON v.title_id = i.title_id AND v.is_main = 1
		JOIN slots s ON s.item_id = i.id AND s.version_id = v.id
		WHERE i.kind = 'episode' AND i.dropped_at IS NULL AND t.kind = 'show' AND t.state != 'removed' AND s.state != 'removed'
		AND s.file_id IS NULL AND i.air_date IS NOT NULL AND i.air_date <= ? AND ` + filter
	at := database.FormatTime(now)
	var total int
	if err := q.QueryRowContext(ctx, "SELECT count(DISTINCT i.id) "+from, at).Scan(&total); err != nil {
		return MissingPage{}, err
	}
	offset := total // a page past the end (checked first, so a huge page cannot overflow)
	if page-1 <= total/size {
		offset = (page - 1) * size
	}
	rows, err := q.QueryContext(ctx, "SELECT DISTINCT i.id, "+sortColumn+" "+from+" ORDER BY "+sortColumn+" "+sqlDir+", i.id "+sqlDir+" LIMIT ? OFFSET ?",
		at, size, offset)
	if err != nil {
		return MissingPage{}, err
	}
	defer rows.Close()
	out := MissingPage{Total: total}
	for rows.Next() {
		var id int64
		var key any
		if err := rows.Scan(&id, &key); err != nil {
			return MissingPage{}, err
		}
		out.IDs = append(out.IDs, id)
	}
	return out, rows.Err()
}

// EpisodeNamedNumbers maps each episode of a show to the season and number
// release names use for it: the release-name numbers when set, else TMDB's.
func EpisodeNamedNumbers(ctx context.Context, q database.Lister, titleID int64) (map[int64][2]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, COALESCE(json_extract(numbers, '$.alt_season'), json_extract(numbers, '$.season'), 0),
		COALESCE(json_extract(numbers, '$.alt_episode'), json_extract(numbers, '$.episode'), 0) FROM items WHERE title_id = ? AND kind = 'episode'`, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][2]int{}
	for rows.Next() {
		var id int64
		var n [2]int
		if err := rows.Scan(&id, &n[0], &n[1]); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// HasEpisodeGroup reports whether a show follows a chosen TMDB episode order.
func HasEpisodeGroup(ctx context.Context, q database.Lister, titleID int64) (bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT COALESCE(episode_group_id, '') != '' FROM titles WHERE id = ?`, titleID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	grouped := false
	for rows.Next() {
		if err := rows.Scan(&grouped); err != nil {
			return false, err
		}
	}
	return grouped, rows.Err()
}

// OrderEpisodeRow is one episode of a show as season counts by a chosen
// episode order read it. AltSeason is 0 when the episode has no
// release-name season.
type OrderEpisodeRow struct {
	PartID    int64
	AltSeason int
	AirDate   sql.NullString
	Keep      bool
	State     string // the main Version's Slot state, "" when none
}

// OrderEpisodeRows reads every episode of a show with its release-name
// season and the main Version's Slot state, in that season's order.
func OrderEpisodeRows(ctx context.Context, q database.Lister, titleID int64) ([]OrderEpisodeRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT COALESCE(i.part_id, 0), COALESCE(json_extract(i.numbers, '$.alt_season'), 0) AS sn, i.air_date, i.keep_up_to_date, COALESCE(s.state, '') FROM items i
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = (SELECT id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1)
		WHERE i.title_id = ? AND i.kind = 'episode' ORDER BY sn`, titleID, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrderEpisodeRow
	for rows.Next() {
		var r OrderEpisodeRow
		if err := rows.Scan(&r.PartID, &r.AltSeason, &r.AirDate, &r.Keep, &r.State); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetEpisodeGroup puts a show back on the episode order id; nil or ""
// is TMDB's own numbering.
func SetEpisodeGroup(ctx context.Context, st *database.Store, titleID int64, id *string, by domain.Actor) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		var t mediafiles.Title
		if err := database.Get(ctx, tx, "titles", titleID, &t); err != nil {
			return err
		}
		t.EpisodeGroupID = nil
		if id != nil && *id != "" {
			g := *id
			t.EpisodeGroupID = &g
		}
		return database.Update(tx, "titles", &t, by)
	})
}

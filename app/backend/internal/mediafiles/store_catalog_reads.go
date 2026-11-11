package mediafiles

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// SlotKind is the kind of the Title a Slot belongs to (movie, show, artist).
func SlotKind(ctx context.Context, q database.Querier, slotID int64) (string, error) {
	var kind string
	err := q.QueryRowContext(ctx, `SELECT t.kind FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE s.id = ?`, slotID).Scan(&kind)
	return kind, err
}

// SlotStateOf is the state a Slot is in.
func SlotStateOf(ctx context.Context, q database.Querier, slotID int64) (string, error) {
	var st string
	err := q.QueryRowContext(ctx, "SELECT state FROM slots WHERE id = ?", slotID).Scan(&st)
	return st, err
}

// SlotTitleID is the Title a Slot belongs to.
func SlotTitleID(ctx context.Context, q database.Querier, slotID int64) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT i.title_id FROM slots s JOIN items i ON i.id = s.item_id WHERE s.id = ?", slotID).Scan(&id)
	return id, err
}

// SlotNames is the id and name of a Slot's Title and the label of its Version.
func SlotNames(ctx context.Context, q database.Querier, slotID int64) (titleID int64, title, version string, err error) {
	err = q.QueryRowContext(ctx, `SELECT t.id, t.name, v.label FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		JOIN versions v ON v.id = s.version_id WHERE s.id = ?`, slotID).Scan(&titleID, &title, &version)
	return
}

// HeldSlot is a Slot that holds a Release, and whether it has a file.
type HeldSlot struct {
	ID      int64
	HasFile bool
}

// SlotsHeldBy lists the Slots in the held state that hold a Release.
func SlotsHeldBy(tx *sql.Tx, releaseID int64) ([]HeldSlot, error) {
	rows, err := tx.Query("SELECT id, file_id IS NOT NULL FROM slots WHERE held_release_id = ? AND state = 'held'", releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HeldSlot
	for rows.Next() {
		var h HeldSlot
		if err := rows.Scan(&h.ID, &h.HasFile); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

// HeldReleaseOf is the Release a Slot holds, if any.
func HeldReleaseOf(ctx context.Context, q database.Querier, slotID int64) (sql.NullInt64, error) {
	var held sql.NullInt64
	err := q.QueryRowContext(ctx, "SELECT held_release_id FROM slots WHERE id = ?", slotID).Scan(&held)
	return held, err
}

// ClearSlotHeldTx lets a Slot stop holding a Release.
func ClearSlotHeldTx(tx *sql.Tx, slotID int64) error {
	_, err := tx.Exec("UPDATE slots SET held_release_id = NULL, next_try_at = NULL WHERE id = ?", slotID)
	return err
}

// SetSlotHeldTx makes a Slot hold a Release until the given time.
func SetSlotHeldTx(tx *sql.Tx, slotID, releaseID int64, until time.Time) error {
	_, err := tx.Exec("UPDATE slots SET held_release_id = ?, next_try_at = ? WHERE id = ?", releaseID, database.FormatTime(until), slotID)
	return err
}

// SetSlotChosenTx records the Release a Slot picked.
func SetSlotChosenTx(tx *sql.Tx, slotID, releaseID int64) error {
	_, err := tx.Exec("UPDATE slots SET chosen_release_id = ? WHERE id = ?", releaseID, slotID)
	return err
}

// SetSlotReasonTx replaces the reason shown for a Slot's state, keeping the state.
func SetSlotReasonTx(tx *sql.Tx, slotID int64, reason string, by database.Actor) error {
	who, _ := json.Marshal(by)
	_, err := tx.Exec("UPDATE slots SET last_reason = ?, updated_at = ?, updated_by = ? WHERE id = ?", reason, database.FormatTime(time.Now()), string(who), slotID)
	return err
}

// TouchItemSearchedTx notes that an Item was searched for at the given time.
func TouchItemSearchedTx(tx *sql.Tx, itemID int64, at time.Time) error {
	_, err := tx.Exec("UPDATE items SET last_searched_at = ? WHERE id = ?", database.FormatTime(at), itemID)
	return err
}

// LibraryPathOfVersion is the path of the Library folder a Version lives in.
func LibraryPathOfVersion(ctx context.Context, q database.Querier, versionID int64) (string, error) {
	var path string
	err := q.QueryRowContext(ctx, "SELECT lf.path FROM versions v JOIN library_folders lf ON lf.id = v.library_folder_id WHERE v.id = ?", versionID).Scan(&path)
	return path, err
}

// SlotsSearchingBefore lists the Slots that have been searching since before
// the given time.
func SlotsSearchingBefore(ctx context.Context, q database.Lister, before time.Time) ([]Slot, error) {
	return database.Select[Slot](ctx, q, "slots", "state = 'searching' AND updated_at < ?", database.FormatTime(before))
}

// KindWanted says whether an active Title of the kind has a Slot looking for
// a Release (wanted, better wanted or holding one).
func KindWanted(ctx context.Context, q database.Querier, kind string) bool {
	var wanted bool
	_ = q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		WHERE t.kind = ? AND t.state = 'active' AND s.state IN ('wanted', 'better_wanted', 'held'))`, kind).Scan(&wanted)
	return wanted
}

// LookingSlotsOfTitle lists a Title's Slots in one of three states, by id.
func LookingSlotsOfTitle(ctx context.Context, q database.Lister, titleID int64, states [3]string) ([]Slot, error) {
	return database.Select[Slot](ctx, q, "slots", `state IN (?, ?, ?) AND item_id IN (
			SELECT id FROM items WHERE title_id = ?) ORDER BY id`, states[0], states[1], states[2], titleID)
}

// TitlesLooking lists the active Titles that have a Slot in one of three
// states, by sort name.
func TitlesLooking(ctx context.Context, q database.Lister, states [3]string) ([]Title, error) {
	return database.Select[Title](ctx, q, "titles", `state = 'active' AND EXISTS (
		SELECT 1 FROM items i JOIN slots sl ON sl.item_id = i.id
		WHERE i.title_id = titles.id AND sl.state IN (?, ?, ?)) ORDER BY sort_name, id`, states[0], states[1], states[2])
}

// HeldSlotDue says whether a Slot holds a Release whose wait is over.
func HeldSlotDue(ctx context.Context, q database.Querier, now time.Time) (bool, error) {
	var due bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM slots WHERE state = 'held' AND held_release_id IS NOT NULL AND next_try_at <= ?)`,
		database.FormatTime(now)).Scan(&due)
	return due, err
}

// HeldSlotsDue lists the Slots that hold a Release whose wait is over,
// longest waiting first.
func HeldSlotsDue(ctx context.Context, q database.Lister, now time.Time) ([]Slot, error) {
	return database.Select[Slot](ctx, q, "slots",
		"state = 'held' AND held_release_id IS NOT NULL AND next_try_at <= ? ORDER BY next_try_at, id", database.FormatTime(now))
}

// ItemTitleID is the Title an Item belongs to.
func ItemTitleID(ctx context.Context, q database.Querier, itemID int64) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT title_id FROM items WHERE id = ?", itemID).Scan(&id)
	return id, err
}

// VersionLabel is the label of a Version, "" when it is gone.
func VersionLabel(ctx context.Context, q database.Querier, versionID int64) string {
	var label string
	_ = q.QueryRowContext(ctx, "SELECT label FROM versions WHERE id = ?", versionID).Scan(&label)
	return label
}

// SlotsOfTitle lists every Slot of a Title, oldest first.
func SlotsOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]Slot, error) {
	return database.Select[Slot](ctx, q, "slots", "item_id IN (SELECT id FROM items WHERE title_id = ?) ORDER BY id", titleID)
}

// VersionSlotsOfTitle lists the Slots of one Version that belong to a Title.
func VersionSlotsOfTitle(ctx context.Context, q database.Lister, versionID, titleID int64) ([]Slot, error) {
	return database.Select[Slot](ctx, q, "slots", "version_id = ? AND item_id IN (SELECT id FROM items WHERE title_id = ?)", versionID, titleID)
}

// VersionSlotsOfPart lists the Slots of one Version that belong to a Part
// (a season or an album).
func VersionSlotsOfPart(ctx context.Context, q database.Lister, versionID, partID int64) ([]Slot, error) {
	return database.Select[Slot](ctx, q, "slots", "version_id = ? AND item_id IN (SELECT id FROM items WHERE part_id = ?)", versionID, partID)
}

// LoadSlot reads one Slot by id into dest.
func LoadSlot(ctx context.Context, q database.Querier, id int64, dest *Slot) error {
	return database.Get(ctx, q, "slots", id, dest)
}

// LoadVersion reads one Version by id into dest.
func LoadVersion(ctx context.Context, q database.Querier, id int64, dest *Version) error {
	return database.Get(ctx, q, "versions", id, dest)
}

// LoadItem reads one Item by id into dest.
func LoadItem(ctx context.Context, q database.Querier, id int64, dest *Item) error {
	return database.Get(ctx, q, "items", id, dest)
}

// LoadTitle reads one Title by id into dest.
func LoadTitle(ctx context.Context, q database.Querier, id int64, dest *Title) error {
	return database.Get(ctx, q, "titles", id, dest)
}

// LoadPickRules reads one set of pick rules by id into dest.
func LoadPickRules(ctx context.Context, q database.Querier, id int64, dest *PickRules) error {
	return database.Get(ctx, q, "pick_rules", id, dest)
}

// LoadPart reads one Part (a season or an album) by id into dest.
func LoadPart(ctx context.Context, q database.Querier, id int64, dest *Part) error {
	return database.Get(ctx, q, "parts", id, dest)
}

// LoadFile reads one File by id into dest.
func LoadFile(ctx context.Context, q database.Querier, id int64, dest *File) error {
	return database.Get(ctx, q, "files", id, dest)
}

// PartsOfTitle lists every Part of a Title.
func PartsOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]Part, error) {
	return database.Select[Part](ctx, q, "parts", "title_id = ?", titleID)
}

// AlbumPartsOfTitle lists the albums of an artist that are not dropped.
func AlbumPartsOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]Part, error) {
	return database.Select[Part](ctx, q, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL", titleID)
}

// EpisodesOfTitle lists the episodes of a show that are not dropped, by season and episode.
func EpisodesOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]Item, error) {
	return database.Select[Item](ctx, q, "items", `title_id = ? AND kind = 'episode' AND dropped_at IS NULL
		ORDER BY json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')`, titleID)
}

// TracksOfPart lists the tracks of an album that are not dropped, by disc and track.
func TracksOfPart(ctx context.Context, q database.Lister, partID int64) ([]Item, error) {
	return database.Select[Item](ctx, q, "items", `part_id = ? AND kind = 'track' AND dropped_at IS NULL
		ORDER BY json_extract(numbers, '$.disc'), json_extract(numbers, '$.track')`, partID)
}

// SlotRetryFacts reads what deciding the search after a failed Download
// needs about a Slot: its Version, Part, active Download and Title kind.
func SlotRetryFacts(ctx context.Context, q database.Querier, slotID int64) (versionID int64, partID, dlID sql.NullInt64, kind string, err error) {
	err = q.QueryRowContext(ctx, `SELECT s.version_id, i.part_id, s.active_download_id, t.kind FROM slots s JOIN items i ON i.id = s.item_id
		JOIN titles t ON t.id = i.title_id WHERE s.id = ?`, slotID).Scan(&versionID, &partID, &dlID, &kind)
	return
}

// SlotChoiceFacts reads a Slot's state, its Title's state and whether its
// copy is protected, in the caller's transaction.
func SlotChoiceFacts(tx *sql.Tx, slotID int64) (slotState, titleState string, protected bool, err error) {
	err = tx.QueryRow(`SELECT s.state, t.state, COALESCE(f.protected, 0)
		FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id
		LEFT JOIN files f ON f.id = s.file_id WHERE s.id = ?`, slotID).Scan(&slotState, &titleState, &protected)
	return
}

// SlotCopyProtected reports whether the Slot's current file is protected.
func SlotCopyProtected(ctx context.Context, q database.Querier, slotID int64) (bool, error) {
	var protected bool
	err := q.QueryRowContext(ctx, `SELECT COALESCE(f.protected, 0) FROM slots s
		LEFT JOIN files f ON f.id = s.file_id WHERE s.id = ?`, slotID).Scan(&protected)
	return protected, err
}

// VersionsOfTitle lists a Title's Versions, the main one first.
func VersionsOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]Version, error) {
	return database.Select[Version](ctx, q, "versions", "title_id = ? ORDER BY is_main DESC, id", titleID)
}

// SlotsOfItem lists an Item's Slots, oldest first.
func SlotsOfItem(ctx context.Context, q database.Lister, itemID int64) ([]Slot, error) {
	return database.Select[Slot](ctx, q, "slots", "item_id = ? ORDER BY id", itemID)
}

// SeasonPartID is the id of a show's season Part with that number.
func SeasonPartID(ctx context.Context, q database.Querier, titleID int64, number int) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, "SELECT id FROM parts WHERE title_id = ? AND kind = 'season' AND number = ?", titleID, number).Scan(&id)
	return id, err
}

// ItemOfTitle reports whether the Item belongs to the Title.
func ItemOfTitle(ctx context.Context, q database.Querier, itemID, titleID int64) bool {
	var n int
	_ = q.QueryRowContext(ctx, "SELECT count(*) FROM items WHERE id = ? AND title_id = ?", itemID, titleID).Scan(&n)
	return n == 1
}

// ShowEpisodes lists every episode of a show (dropped ones too) by season
// and episode; season limits it to one season when not nil.
func ShowEpisodes(ctx context.Context, q database.Lister, titleID int64, season *int) ([]Item, error) {
	where, args := "title_id = ? AND kind = 'episode'", []any{titleID}
	if season != nil {
		where += " AND json_extract(numbers, '$.season') = ?"
		args = append(args, *season)
	}
	return database.Select[Item](ctx, q, "items", where+" ORDER BY json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')", args...)
}

// ShowEpisodesByOrder lists a show's episodes by its chosen episode order:
// by the release-name numbers when set, else by TMDB's. With season, only
// that season of the order.
func ShowEpisodesByOrder(ctx context.Context, q database.Lister, titleID int64, season *int) ([]Item, error) {
	where, args := "title_id = ? AND kind = 'episode'", []any{titleID}
	if season != nil {
		where += " AND COALESCE(json_extract(numbers, '$.alt_season'), json_extract(numbers, '$.season')) = ?"
		args = append(args, *season)
	}
	return database.Select[Item](ctx, q, "items", where+` ORDER BY COALESCE(json_extract(numbers, '$.alt_season'), json_extract(numbers, '$.season')),
		COALESCE(json_extract(numbers, '$.alt_episode'), json_extract(numbers, '$.episode'))`, args...)
}

// EpisodeSlotsOfTitle lists the Slots of a show's episodes, oldest first.
func EpisodeSlotsOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]Slot, error) {
	return database.Select[Slot](ctx, q, "slots", "item_id IN (SELECT id FROM items WHERE title_id = ? AND kind = 'episode') ORDER BY id", titleID)
}

// EpisodeFilesOfTitle lists the files held by a show's episode Slots.
func EpisodeFilesOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]File, error) {
	return database.Select[File](ctx, q, "files", `id IN (SELECT file_id FROM slots WHERE file_id IS NOT NULL AND item_id IN (SELECT id FROM items WHERE title_id = ? AND kind = 'episode'))`, titleID)
}

// TitlesBySourceID maps "movie|tmdb:603" and "show|tvdb:100" (kind, then a
// tmdb, tvdb or imdb id) to the Library's movie and show titles.
func TitlesBySourceID(ctx context.Context, q database.Lister) (map[string]int64, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, kind, source_ids FROM titles WHERE kind IN ('movie', 'show') AND state != 'removed'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var kind, raw string
		if err := rows.Scan(&id, &kind, &raw); err != nil {
			return nil, err
		}
		var ids map[string]string
		if json.Unmarshal([]byte(raw), &ids) != nil {
			continue
		}
		for _, k := range []string{"tmdb", "tvdb", "imdb"} {
			if v := ids[k]; v != "" {
				out[kind+"|"+k+":"+v] = id
			}
		}
	}
	return out, rows.Err()
}

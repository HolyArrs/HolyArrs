package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
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
func SetSlotReasonTx(tx *sql.Tx, slotID int64, reason string, by domain.Actor) error {
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
func SlotsSearchingBefore(ctx context.Context, q database.Lister, before time.Time) ([]domain.Slot, error) {
	return database.Select[domain.Slot](ctx, q, "slots", "state = 'searching' AND updated_at < ?", database.FormatTime(before))
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
func LookingSlotsOfTitle(ctx context.Context, q database.Lister, titleID int64, states [3]string) ([]domain.Slot, error) {
	return database.Select[domain.Slot](ctx, q, "slots", `state IN (?, ?, ?) AND item_id IN (
			SELECT id FROM items WHERE title_id = ?) ORDER BY id`, states[0], states[1], states[2], titleID)
}

// TitlesLooking lists the active Titles that have a Slot in one of three
// states, by sort name.
func TitlesLooking(ctx context.Context, q database.Lister, states [3]string) ([]domain.Title, error) {
	return database.Select[domain.Title](ctx, q, "titles", `state = 'active' AND EXISTS (
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
func HeldSlotsDue(ctx context.Context, q database.Lister, now time.Time) ([]domain.Slot, error) {
	return database.Select[domain.Slot](ctx, q, "slots",
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

// VersionSlotsOfTitle lists the Slots of one Version that belong to a Title.
func VersionSlotsOfTitle(ctx context.Context, q database.Lister, versionID, titleID int64) ([]domain.Slot, error) {
	return database.Select[domain.Slot](ctx, q, "slots", "version_id = ? AND item_id IN (SELECT id FROM items WHERE title_id = ?)", versionID, titleID)
}

// VersionSlotsOfPart lists the Slots of one Version that belong to a Part
// (a season or an album).
func VersionSlotsOfPart(ctx context.Context, q database.Lister, versionID, partID int64) ([]domain.Slot, error) {
	return database.Select[domain.Slot](ctx, q, "slots", "version_id = ? AND item_id IN (SELECT id FROM items WHERE part_id = ?)", versionID, partID)
}

// LoadSlot reads one Slot by id into dest.
func LoadSlot(ctx context.Context, q database.Querier, id int64, dest *domain.Slot) error {
	return database.Get(ctx, q, "slots", id, dest)
}

// LoadVersion reads one Version by id into dest.
func LoadVersion(ctx context.Context, q database.Querier, id int64, dest *domain.Version) error {
	return database.Get(ctx, q, "versions", id, dest)
}

// LoadItem reads one Item by id into dest.
func LoadItem(ctx context.Context, q database.Querier, id int64, dest *domain.Item) error {
	return database.Get(ctx, q, "items", id, dest)
}

// LoadTitle reads one Title by id into dest.
func LoadTitle(ctx context.Context, q database.Querier, id int64, dest *domain.Title) error {
	return database.Get(ctx, q, "titles", id, dest)
}

// LoadPickRules reads one set of pick rules by id into dest.
func LoadPickRules(ctx context.Context, q database.Querier, id int64, dest *domain.PickRules) error {
	return database.Get(ctx, q, "pick_rules", id, dest)
}

// LoadPart reads one Part (a season or an album) by id into dest.
func LoadPart(ctx context.Context, q database.Querier, id int64, dest *domain.Part) error {
	return database.Get(ctx, q, "parts", id, dest)
}

// LoadFile reads one File by id into dest.
func LoadFile(ctx context.Context, q database.Querier, id int64, dest *domain.File) error {
	return database.Get(ctx, q, "files", id, dest)
}

// PartsOfTitle lists every Part of a Title.
func PartsOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]domain.Part, error) {
	return database.Select[domain.Part](ctx, q, "parts", "title_id = ?", titleID)
}

// AlbumPartsOfTitle lists the albums of an artist that are not dropped.
func AlbumPartsOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]domain.Part, error) {
	return database.Select[domain.Part](ctx, q, "parts", "title_id = ? AND kind = 'album' AND dropped_at IS NULL", titleID)
}

// EpisodesOfTitle lists the episodes of a show that are not dropped, by season and episode.
func EpisodesOfTitle(ctx context.Context, q database.Lister, titleID int64) ([]domain.Item, error) {
	return database.Select[domain.Item](ctx, q, "items", `title_id = ? AND kind = 'episode' AND dropped_at IS NULL
		ORDER BY json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')`, titleID)
}

// TracksOfPart lists the tracks of an album that are not dropped, by disc and track.
func TracksOfPart(ctx context.Context, q database.Lister, partID int64) ([]domain.Item, error) {
	return database.Select[domain.Item](ctx, q, "items", `part_id = ? AND kind = 'track' AND dropped_at IS NULL
		ORDER BY json_extract(numbers, '$.disc'), json_extract(numbers, '$.track')`, partID)
}

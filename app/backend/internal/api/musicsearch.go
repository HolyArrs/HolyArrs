package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/source"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// Album search (stage 3.3): an album, a track's album or every album with a
// track looking, by hand.

// searchArtist queues the album searches a person asked for on an artist
// page: one album (album_id), the album of a track (item_id) or of a track
// Slot (slot_id) or, with none of them, every album with a track kept up to
// date and looking. version_id narrows to one Version. choose false only
// lists the Releases, and needs an album.
func (a *API) searchArtist(w http.ResponseWriter, r *http.Request, t domain.Title, in showSearchIn) {
	ctx := r.Context()
	db := a.s.Store.Read()
	if in.Season != nil {
		fail(w, http.StatusBadRequest, "season is for shows; say album_id for an artist")
		return
	}
	listOnly := in.Choose != nil && !*in.Choose
	vs, err := store.Select[domain.Version](ctx, db, "versions", "title_id = ? ORDER BY is_main DESC, id", t.ID)
	if err != nil {
		internal(w, err)
		return
	}
	if in.VersionID != nil {
		vs = slices.DeleteFunc(vs, func(v domain.Version) bool { return v.ID != *in.VersionID })
		if len(vs) == 0 {
			fail(w, http.StatusNotFound, "no such Version on this artist")
			return
		}
	}
	album := int64(0)
	switch {
	case in.AlbumID != nil:
		album = *in.AlbumID
	case in.SlotID != nil:
		var sl domain.Slot
		if err := store.Get(ctx, db, "slots", *in.SlotID, &sl); err != nil || !a.slotOfTitle(ctx, sl.ItemID, t.ID) {
			fail(w, http.StatusNotFound, "no such Slot on this artist")
			return
		}
		vs = slices.DeleteFunc(vs, func(v domain.Version) bool { return v.ID != sl.VersionID })
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(part_id, 0) FROM items WHERE id = ?", sl.ItemID).Scan(&album)
	case in.ItemID != nil:
		if !a.slotOfTitle(ctx, *in.ItemID, t.ID) {
			fail(w, http.StatusNotFound, "no such track on this artist")
			return
		}
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(part_id, 0) FROM items WHERE id = ?", *in.ItemID).Scan(&album)
	}
	if album != 0 {
		var n int
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM parts WHERE id = ? AND title_id = ? AND kind = 'album'", album, t.ID).Scan(&n)
		if n == 0 {
			fail(w, http.StatusNotFound, "no such album on this artist")
			return
		}
	} else if listOnly {
		fail(w, http.StatusBadRequest, "say which album (album_id) to list Releases for")
		return
	}
	by := actor(r)
	ids := []int64{}
	var runAfter time.Time // zero: run now
	queue := func(partID, versionID int64, keptOnly bool) error {
		params := map[string]any{"by_hand": true, "version_id": versionID}
		key := fmt.Sprintf("hand_search_album:%d:%d", partID, versionID)
		if keptOnly {
			params["kept_only"] = true
		}
		if listOnly {
			params["choose"] = false
			key = fmt.Sprintf("show_album:%d:%d", partID, versionID)
		}
		id, created, err := a.s.Queue.EnqueueAt(ctx, source.KindSearchAlbum, &domain.Subject{Type: "part", ID: partID}, params, key, by, runAfter)
		if err == nil {
			ids = append(ids, id)
			if created && !runAfter.IsZero() {
				runAfter = runAfter.Add(source.BulkStagger)
			}
		}
		return err
	}
	if album != 0 {
		for _, v := range vs {
			if err := queue(album, v.ID, false); err != nil {
				internal(w, err)
				return
			}
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids})
		return
	}
	// The whole artist: every album with a track kept up to date that is
	// looking, in an artist and Version kept up to date (albums not out yet
	// are left out).
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT p.id, s.version_id, p.release_date FROM parts p JOIN items i ON i.part_id = p.id JOIN slots s ON s.item_id = i.id
		JOIN titles t ON t.id = p.title_id JOIN versions v ON v.id = s.version_id
		WHERE p.title_id = ? AND p.kind = 'album' AND t.keep_up_to_date = 1 AND v.keep_up_to_date = 1 AND p.keep_up_to_date = 1 AND p.included = 1
		AND p.dropped_at IS NULL AND i.keep_up_to_date = 1 AND i.dropped_at IS NULL
		AND s.state IN ('wanted', 'better_wanted', 'failed', 'rejected', 'stalled') ORDER BY p.release_date, p.id, s.version_id`, t.ID)
	if err != nil {
		internal(w, err)
		return
	}
	type pv struct{ part, ver int64 }
	var list []pv
	for rows.Next() {
		var x pv
		var d any
		if err := rows.Scan(&x.part, &x.ver, &d); err != nil {
			rows.Close()
			internal(w, err)
			return
		}
		list = append(list, x)
	}
	rows.Close()
	list = slices.DeleteFunc(list, func(x pv) bool {
		return !slices.ContainsFunc(vs, func(v domain.Version) bool { return v.ID == x.ver })
	})
	// Every album × Version at once would ask the Sources hundreds of times
	// in a burst: the searches are spread out like a bulk search, and one
	// click queues at most artist_search_cap of them.
	limit := artistSearchCap(ctx, db)
	left := max(len(list)-limit, 0)
	list = list[:len(list)-left]
	runAfter = time.Now().UTC()
	for _, x := range list {
		if err := queue(x.part, x.ver, true); err != nil {
			internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_ids": ids, "left_out": left})
}

// artistSearchCap is the most album searches one Search now on an artist
// queues.
func artistSearchCap(ctx context.Context, db *sql.DB) int {
	if n, err := strconv.Atoi(source.Setting(ctx, db, source.SettingArtistSearchCap)); err == nil && n > 0 {
		return n
	}
	return source.DefaultArtistSearchCap
}

// albumLabels names the albums of track Slots ("Low Tide Atlas"); "" when
// none is a track.
func (a *API) albumLabels(ctx context.Context, slotIDs []int64) string {
	var names []string
	for _, id := range slotIDs {
		var name string
		if a.s.Store.Read().QueryRowContext(ctx, `SELECT p.name FROM slots s JOIN items i ON i.id = s.item_id JOIN parts p ON p.id = i.part_id
			WHERE s.id = ? AND i.kind = 'track'`, id).Scan(&name) == nil && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// partLabel names a part Job's subject: the show and "S01", or the artist
// and the album.
func (a *API) partLabel(ctx context.Context, partID int64) (titleID int64, title, season, album string) {
	var kind string
	_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT kind, name FROM parts WHERE id = ?", partID).Scan(&kind, &album)
	if kind == "album" {
		_ = a.s.Store.Read().QueryRowContext(ctx, "SELECT t.id, t.name FROM parts p JOIN titles t ON t.id = p.title_id WHERE p.id = ?", partID).Scan(&titleID, &title)
		return titleID, title, "", album
	}
	titleID, title, season = a.seasonLabel(ctx, partID)
	return titleID, title, season, ""
}

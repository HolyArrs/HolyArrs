package music

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Pins the music SQL (library snapshot, track states, asks and follows
// reads, whole-artist search).

const t10Stamp = "2026-01-01T00:00:00Z"

// t10Clone copies one row of a table with new values for some columns
// (the id is always new), reading the column list from the schema.
func t10Clone(t *testing.T, tx *sql.Tx, table string, id int64, set map[string]any) int64 {
	t.Helper()
	rows, err := tx.Query("SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatal(err)
	}
	var cols, sels []string
	var args []any
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		if c == "id" {
			continue
		}
		cols = append(cols, c)
		if v, ok := set[c]; ok {
			sels = append(sels, "?")
			args = append(args, v)
		} else {
			sels = append(sels, c)
		}
	}
	rows.Close()
	args = append(args, id)
	res, err := tx.Exec(fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s WHERE id = ?", table, strings.Join(cols, ","), strings.Join(sels, ","), table), args...)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := res.LastInsertId()
	return n
}

// t10MusicWorld seeds a show next to the artist so every read has to
// keep the two catalogs apart, then refreshes the artist's albums.
func t10MusicWorld(t *testing.T) (*t10Env, int64) {
	t.Helper()
	e := t10Setup(t)
	_, _, _, _, artist, _ := t10Seed(t, e)
	if _, err := e.lib.RefreshArtist(context.Background(), artist, domain.System, nil); err != nil {
		t.Fatal(err)
	}
	return e, artist
}

func t10GiveFile(t *testing.T, e *t10Env, slotID int64) {
	t.Helper()
	err := e.store.Write(context.Background(), func(tx *sql.Tx) error {
		var version int64
		if err := tx.QueryRow("SELECT version_id FROM slots WHERE id = ?", slotID).Scan(&version); err != nil {
			return err
		}
		f := mediafiles.File{SlotIDs: []int64{slotID}, VersionID: version, RelativePath: fmt.Sprintf("x/%d.flac", slotID), Size: 10, AddedAt: time.Now().UTC(),
			Languages: []string{}, MediaInfo: map[string]any{}, Origin: "imported", PlacedBy: "none", Role: "main", State: "present"}
		fid, err := database.Insert(tx, "files", &f, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", fid, slotID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestT10PinMusicLibrarySnapshot(t *testing.T) {
	e, artist := t10MusicWorld(t)
	ctx := context.Background()
	var trackSlot, episodeSlot int64
	_ = e.store.Read().QueryRow("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.kind = 'track' ORDER BY s.id LIMIT 1").Scan(&trackSlot)
	_ = e.store.Read().QueryRow("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.kind = 'episode' ORDER BY s.id LIMIT 1").Scan(&episodeSlot)
	t10GiveFile(t, e, trackSlot)
	t10GiveFile(t, e, episodeSlot)
	// A second artist without a MusicBrainz id (NULL partner) and a removed one.
	err := e.store.Write(ctx, func(tx *sql.Tx) error {
		t10Clone(t, tx, "titles", artist, map[string]any{"name": "No Id Band", "sort_name": "no id band", "source_ids": "{}"})
		gone := t10Clone(t, tx, "titles", artist, map[string]any{"name": "Gone Band", "sort_name": "gone band", "source_ids": `{"musicbrainz":"gone"}`})
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", gone)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	lib, err := e.api.musicLibrary(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.hadParts) != 1 || !lib.hadParts[4] {
		t.Fatalf("parts with files %v (episodes must not count)", lib.hadParts)
	}
	if len(lib.albums) != 3 {
		t.Fatalf("albums %+v", lib.albums)
	}
	want := []struct {
		name, kind, date string
		secondary        bool
	}{{"First Light", "album", "2020-03-06", false}, {"Live Night", "album", "2021-12-31", true}, {"Soon", "ep", "2099-01-01", false}}
	for i, w := range want {
		al := lib.albums[i]
		if al.name != w.name || al.kind != w.kind || al.date != w.date || al.secondary != w.secondary || al.mbid != strings.ToLower(al.mbid) || al.mbid == "" {
			t.Fatalf("album %d %+v want %+v", i, al, w)
		}
	}
	if len(lib.artists) != 2 || lib.artists[0].name != "Lab Band" || lib.artists[0].had != 1 || !lib.artists[0].hadAny ||
		lib.artists[1].name != "No Id Band" || lib.artists[1].mbid != "" || lib.artists[1].hadAny {
		t.Fatalf("artists %+v", lib.artists)
	}
	if i, ok := lib.byMBID[apiArtist]; !ok || i != 0 || len(lib.byMBID) != 1 {
		t.Fatalf("byMBID %v", lib.byMBID)
	}
}

func TestT10PinTrackStates(t *testing.T) {
	e, _ := t10MusicWorld(t)
	ctx := context.Background()
	// Album 4 has rec-1 and rec-2: one gets a file, the other loses its slot.
	var s1, s2 int64
	_ = e.store.Read().QueryRow("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.recording_id = 'rec-1'").Scan(&s1)
	_ = e.store.Read().QueryRow("SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.recording_id = 'rec-2'").Scan(&s2)
	t10GiveFile(t, e, s1)
	if err := e.store.Write(ctx, func(tx *sql.Tx) error { _, err := tx.Exec("DELETE FROM slots WHERE id = ?", s2); return err }); err != nil {
		t.Fatal(err)
	}
	got := e.api.trackStates(ctx, 4)
	want := map[string]string{
		"rec-1":                           "in_library",
		"rec-2":                           "missing",
		"n:" + metadata.SongKey("Song 1"): "in_library",
		"n:" + metadata.SongKey("Song 2"): "missing",
	}
	if len(got) != len(want) {
		t.Fatalf("states %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("states %v want %v", got, want)
		}
	}
	// An album with no tracks, and the show's season part: empty.
	if got := e.api.trackStates(ctx, 5); len(got) != 0 {
		t.Fatalf("empty album %v", got)
	}
	if got := e.api.trackStates(ctx, 2); len(got) != 0 {
		t.Fatalf("season part %v", got)
	}
}

func TestT10PinMusicActsAndAlbumFollows(t *testing.T) {
	e, _ := t10MusicWorld(t)
	ctx := context.Background()
	err := e.store.Write(ctx, func(tx *sql.Tx) error {
		ask := func(mbid, name, state, albums string, by int64, joined ...int64) error {
			res, err := tx.Exec(`INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, name, state, asked_by, asked_at, source_ids, albums)
				VALUES (?, '{}', ?, '{}', 'artist', ?, ?, ?, ?, ?, ?, ?)`, t10Stamp, t10Stamp, "mbid:"+mbid, name, state, by, t10Stamp, `{"musicbrainz":"`+mbid+`"}`, albums)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			for _, u := range joined {
				if _, err := tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at) VALUES (?, ?, ?)", id, u, t10Stamp); err != nil {
					return err
				}
			}
			return nil
		}
		// Whole artist, asked by the owner, the member joined.
		if err := ask("AAAA0000-0000-0000-0000-000000000001", "Whole Band", "asked", "[]", 1, 2); err != nil {
			return err
		}
		// Albums of an artist, approved, asked by the member.
		if err := ask("BBBB0000-0000-0000-0000-000000000002", "Album Band", "approved", `[{"mbid":"CCCC0000-0000-0000-0000-000000000003","name":"One"},{"mbid":"DDDD0000-0000-0000-0000-000000000004","name":"Two"}]`, 2); err != nil {
			return err
		}
		// Closed asks are not read.
		if err := ask("EEEE0000-0000-0000-0000-000000000005", "Done Band", "turned_down", "[]", 2); err != nil {
			return err
		}
		// A large household: 150 album asks by the owner.
		for i := 0; i < 150; i++ {
			mbid := fmt.Sprintf("F%03d0000-0000-0000-0000-000000000006", i)
			if err := ask(mbid, fmt.Sprintf("Bulk %d", i), "asked", fmt.Sprintf(`[{"mbid":"%s","name":"Bulk Album %d"}]`, mbid, i), 1); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("INSERT INTO follows (user_id, title_id, created_at) VALUES (1, 1, ?), (2, 1, ?), (2, 2, ?)", t10Stamp, t10Stamp, t10Stamp); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO album_follows (user_id, mbid, name, artist, release_date, created_at) VALUES
			(2, 'ABCDEF00-0000-0000-0000-000000000001', 'Mixed', 'Band', '2030-01-01', ?), (1, 'other', 'Other', 'Band', '2030-01-01', ?)`, t10Stamp, t10Stamp)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	x, err := e.api.musicActsOf(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Adding a show and an artist as the owner follows them for the owner;
	// the rows above add the member on titles 1 and 2.
	if len(x.follows) != 3 || len(x.follows[1]) != 2 || len(x.follows[2]) != 2 || !x.follows[2][2] || len(x.follows[3]) != 1 || !x.follows[3][1] {
		t.Fatalf("follows %v", x.follows)
	}
	whole := "aaaa0000-0000-0000-0000-000000000001"
	if !x.asked[whole][1] || !x.asked[whole][2] || len(x.asked[whole]) != 2 {
		t.Fatalf("asked %v", x.asked[whole])
	}
	if x.artistState[whole] != "asked" || x.artistState["bbbb0000-0000-0000-0000-000000000002"] != "" {
		t.Fatalf("artistState %v", x.artistState)
	}
	if x.albumState["cccc0000-0000-0000-0000-000000000003"] != "approved" || x.albumState["dddd0000-0000-0000-0000-000000000004"] != "approved" {
		t.Fatalf("albumState has %d", len(x.albumState))
	}
	if _, ok := x.asked["eeee0000-0000-0000-0000-000000000005"]; ok {
		t.Fatal("a turned down ask was read")
	}
	// 150 bulk + the whole-artist and the albums asks.
	if len(x.asked) != 152 || len(x.albumState) != 152 {
		t.Fatalf("asked %d albumState %d", len(x.asked), len(x.albumState))
	}
	// The member's own list: the joined whole-artist ask and the two albums.
	if len(x.mine) != 3 {
		t.Fatalf("mine %+v", x.mine)
	}
	got := e.api.albumFollows(ctx, 2)
	if len(got) != 1 || !got["abcdef00-0000-0000-0000-000000000001"] {
		t.Fatalf("album follows %v", got)
	}
	if got := e.api.albumFollows(ctx, 99); got == nil || len(got) != 0 {
		t.Fatalf("no follows %v", got)
	}
}

// The whole-artist search over many albums: cap, order by release date
// (NULL first), skip dropped, excluded and complete albums.
func TestT10PinWholeArtistSearchLargeLibrary(t *testing.T) {
	e, artist := t10MusicWorld(t)
	ctx := context.Background()
	var cloned []int64
	err := e.store.Write(ctx, func(tx *sql.Tx) error {
		ids := func(q string, arg int64) []int64 {
			rows, err := tx.Query(q, arg)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var out []int64
			for rows.Next() {
				var id int64
				_ = rows.Scan(&id)
				out = append(out, id)
			}
			return out
		}
		for i := 0; i < 40; i++ {
			set := map[string]any{"name": fmt.Sprintf("Bulk %02d", i), "release_date": fmt.Sprintf("2010-01-%02dT00:00:00.000000000Z", 1+i%28), "source_ids": "{}"}
			if i == 0 {
				set["release_date"] = nil
			}
			p := t10Clone(t, tx, "parts", 4, set)
			cloned = append(cloned, p)
			for _, it := range ids("SELECT id FROM items WHERE part_id = ?", 4) {
				ni := t10Clone(t, tx, "items", it, map[string]any{"part_id": p, "recording_id": nil})
				for _, s := range ids("SELECT id FROM slots WHERE item_id = ?", it) {
					t10Clone(t, tx, "slots", s, map[string]any{"item_id": ni})
				}
			}
		}
		if _, err := tx.Exec("UPDATE parts SET dropped_at = ? WHERE id = ?", t10Stamp, cloned[1]); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE parts SET included = 0 WHERE id = ?", cloned[2]); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET state = 'in_library' WHERE item_id IN (SELECT id FROM items WHERE part_id = ?)", cloned[3]); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
			VALUES ('what_to_pick', 'artist_search_cap', '10', ?, '{}', ?, '{}')`, t10Stamp, t10Stamp)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	st, b := e.call(e.owner, "POST", fmt.Sprintf("/api/v1/titles/%d/search", artist), "")
	if st != 202 {
		t.Fatalf("search %d %s", st, b)
	}
	var out struct {
		JobIDs  []int64 `json:"job_ids"`
		LeftOut int     `json:"left_out"`
	}
	_ = json.Unmarshal(b, &out)
	// 41 albums look (First Light + 40 clones) minus 3 skipped clones = 38; cap 10.
	if len(out.JobIDs) != 10 || out.LeftOut != 28 {
		t.Fatalf("jobs %d left out %d", len(out.JobIDs), out.LeftOut)
	}
	var first domain.Job
	if err := database.Get(ctx, e.store.Read(), "jobs", out.JobIDs[0], &first); err != nil {
		t.Fatal(err)
	}
	// A NULL release date sorts first: clone 0.
	if first.Subject == nil || first.Subject.ID != cloned[0] {
		t.Fatalf("first job %+v want album %d", first.Subject, cloned[0])
	}
	for _, id := range out.JobIDs {
		var j domain.Job
		_ = database.Get(ctx, e.store.Read(), "jobs", id, &j)
		for _, bad := range cloned[1:4] {
			if j.Subject.ID == bad {
				t.Fatalf("album %d must not be searched", bad)
			}
		}
	}
}

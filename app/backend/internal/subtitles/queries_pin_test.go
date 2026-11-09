package subtitles

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// These tests pin the results of the subtitle queries that join files,
// slots, items, versions, titles and library_folders, tables owned by another
// module. They run on a large library with missing partners, such as a File
// with no subtitle rows, a File never looked at and a slot with no File, so a
// later rewrite of these queries can be checked against them.

type subPinBig struct {
	f         *fix
	show      int64
	showVer   int64
	movies    []int64 // title ids
	epItem    []int64 // item ids, indexed by episode number minus one
	epFile    []int64 // file id of each episode, or 0 when it has none
	epState   []string
	movieFile []int64
	future    string
}

const (
	subPinEps    = 1000
	subPinMovies = 200
)

// subPinSeed adds a show with subPinEps episodes and subPinMovies movies to the fixture.
func subPinSeed(t *testing.T) *subPinBig {
	t.Helper()
	f := newFix(t)
	b := &subPinBig{f: f}
	ts := database.FormatTime(now())
	b.future = database.FormatTime(now().Add(48 * time.Hour))
	past := database.FormatTime(now().Add(-48 * time.Hour))
	// Every file starts with the default wish of English.
	wishAll(t, f, Want{Language: "en", HearingImpaired: HINormal})
	set := ReadSettings(context.Background(), f.st.Read())
	wantJSON, _ := json.Marshal(set.Languages)
	err := f.st.Write(context.Background(), func(tx *sql.Tx) error {
		ins := func(q string, args ...any) int64 {
			t.Helper()
			res, err := tx.Exec(q, args...)
			if err != nil {
				t.Fatalf("%s: %v", q, err)
			}
			id, _ := res.LastInsertId()
			return id
		}
		var rules, lf int64
		_ = tx.QueryRow("SELECT min(id) FROM pick_rules").Scan(&rules)
		_ = tx.QueryRow("SELECT min(id) FROM library_folders").Scan(&lf)
		title := func(kind, name string) int64 {
			return ins(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state)
				VALUES (?, ?, ?, ?, ?, ?, ?, 2000, '{}', 'tmdb', 'by_hand', 'active')`, ts, sysActor, ts, sysActor, kind, name, name)
		}
		version := func(tid int64, folder string) int64 {
			return ins(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
				VALUES (?, ?, ?, ?, ?, 'HD', ?, ?, ?, 1)`, ts, sysActor, ts, sysActor, tid, rules, lf, folder)
		}
		item := func(tid int64, kind, numbers string) int64 {
			return ins(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind, numbers) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				ts, sysActor, ts, sysActor, tid, kind, numbers)
		}
		// slotFile makes a slot and, when state is not empty, a File in that state.
		slotFile := func(ver, it int64, rel, state string) int64 {
			slot := ins(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, ?, ?, ?, ?, ?, 'in_library')`,
				ts, sysActor, ts, sysActor, it, ver)
			if state == "" {
				return 0
			}
			id := ins(`INSERT INTO files (created_at, created_by, updated_at, updated_by, slot_ids, version_id, relative_path, added_at, origin, placed_by, state)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'imported', 'copy', ?)`, ts, sysActor, ts, sysActor, fmt.Sprintf("[%d]", slot), ver, rel, ts, state)
			if _, err := tx.Exec("UPDATE slots SET file_id = ? WHERE id = ?", id, slot); err != nil {
				t.Fatal(err)
			}
			return id
		}
		b.show = title("show", "Big Show")
		b.showVer = version(b.show, "Big Show (2000)")
		for i := 0; i < subPinEps; i++ {
			it := item(b.show, "episode", fmt.Sprintf(`{"season":%d,"episode":%d}`, i/100+1, i%100+1))
			state := "present"
			switch {
			case i%50 == 49:
				state = "missing" // a File that is absent from disk
			case i%40 == 39:
				state = "" // a slot that has no File
			}
			b.epItem = append(b.epItem, it)
			b.epState = append(b.epState, state)
			b.epFile = append(b.epFile, slotFile(b.showVer, it, fmt.Sprintf("Big Show (2000)/Season %02d/S%02dE%03d.mkv", i/100+1, i/100+1, i%100+1), state))
		}
		for i := 0; i < subPinMovies; i++ {
			name := fmt.Sprintf("Movie %03d", i)
			tid := title("movie", name)
			ver := version(tid, name)
			it := item(tid, "movie", "{}")
			b.movies = append(b.movies, tid)
			b.movieFile = append(b.movieFile, slotFile(ver, it, name+"/"+name+".mkv", "present"))
		}
		// Subtitle rows for the show's files: i%3==0 wants English now, i%3==1 gave
		// up and retries later or is due, and i%3==2 has no rows.
		sub := func(file int64, lang, state string, next any) {
			ins(`INSERT INTO subtitles (created_at, created_by, updated_at, updated_by, file_id, language, place, origin, state, next_try_at)
				VALUES (?, ?, ?, ?, ?, ?, 'wish', 'found', ?, ?)`, ts, sysActor, ts, sysActor, file, lang, state, next)
		}
		for i, id := range b.epFile {
			if id == 0 {
				continue
			}
			switch i % 3 {
			case 0:
				sub(id, "en", "wanted", nil)
			case 1:
				if i%2 == 1 {
					sub(id, "en", "none_good_enough", b.future)
				} else {
					sub(id, "en", "none_good_enough", past)
				}
			}
			if i%10 == 0 {
				sub(id, "fr", "wanted", past)
			}
			if i%4 == 0 { // looked at; i%8==0 under the current wish, i%8==4 under an old one
				w := string(wantJSON)
				if i%8 == 4 {
					w = "[]"
				}
				ins(`INSERT INTO subtitle_looks (file_id, looked_at, wish) VALUES (?, ?, ?)`, id, ts, w)
			}
		}
		// Even movies are missing English and odd ones have it.
		for i, id := range b.movieFile {
			if i%2 == 0 {
				sub(id, "en", "wanted", nil)
			} else {
				ins(`INSERT INTO subtitles (created_at, created_by, updated_at, updated_by, file_id, language, place, origin, state, relative_path)
					VALUES (?, ?, ?, ?, ?, 'en', 'beside', 'found', 'present', 'x.en.srt')`, ts, sysActor, ts, sysActor, id)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// missingEps returns the show's files that should be in the missing list:
// present files with a wanted or none_good_enough row for English.
func (b *subPinBig) missingEps() []int {
	var out []int
	for i, id := range b.epFile {
		if id == 0 || b.epState[i] != "present" {
			continue
		}
		if i%3 == 0 || i%3 == 1 {
			out = append(out, i)
		}
	}
	return out
}

func TestSubPinMissingListLargeLibrary(t *testing.T) {
	b := subPinSeed(t)
	ctx := context.Background()
	db := b.f.st.Read()
	eps := b.missingEps()
	// English rows come from the episodes and half the movies; French rows on some present files are counted below.
	wantEN := len(eps) + subPinMovies/2
	wantFR := 0
	for i := range b.epFile {
		if b.epFile[i] != 0 && b.epState[i] == "present" && i%10 == 0 {
			wantFR++
		}
	}
	rows, total, err := MissingList(ctx, db, MissingFilter{}, 5000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != wantEN+wantFR || len(rows) != total {
		t.Fatalf("total %d rows %d, want %d", total, len(rows), wantEN+wantFR)
	}
	// Rows sort by title sort name, season, episode, then language, so "Big Show" comes before "Movie ...".
	if !sort.SliceIsSorted(rows, func(i, j int) bool {
		a, c := rows[i], rows[j]
		if a.Title != c.Title {
			return a.Title < c.Title
		}
		as, cs := 0, 0
		if a.Season != nil {
			as = *a.Season
		}
		if c.Season != nil {
			cs = *c.Season
		}
		if as != cs {
			return as < cs
		}
		ae, ce := 0, 0
		if a.Episode != nil {
			ae = *a.Episode
		}
		if c.Episode != nil {
			ce = *c.Episode
		}
		if ae != ce {
			return ae < ce
		}
		return a.Language < c.Language
	}) {
		t.Fatal("missing list is not in title, season, episode, language order")
	}
	first := rows[0]
	if first.Title != "Big Show" || first.Kind != "episode" || first.Season == nil || *first.Season != 1 || first.Episode == nil || *first.Episode != 1 ||
		first.Language != "en" || first.State != "wanted" || first.FileID != b.epFile[0] || first.ItemID != b.epItem[0] || first.TitleID != b.show || first.NextTryAt != nil {
		t.Fatalf("first row %+v", first)
	}
	// A movie row has no season and no episode.
	var movie *MissingRow
	for i := range rows {
		if rows[i].Kind == "movie" {
			movie = &rows[i]
			break
		}
	}
	if movie == nil || movie.Season != nil || movie.Episode != nil || movie.Title != "Movie 000" || movie.State != "wanted" {
		t.Fatalf("movie row %+v", movie)
	}
	// Left out: missing Files, slots without Files, beside rows and files with no rows.
	for _, r := range rows {
		for i, id := range b.epFile {
			if id == r.FileID && b.epState[i] != "present" {
				t.Fatalf("row for a file that is %s", b.epState[i])
			}
		}
	}
	// The total stays the same whatever page is asked for.
	page, total2, err := MissingList(ctx, db, MissingFilter{}, 50, 100)
	if err != nil || total2 != total || len(page) != 50 || fmt.Sprint(*page[0].Episode, page[0].FileID, page[49].FileID) != fmt.Sprint(*rows[100].Episode, rows[100].FileID, rows[149].FileID) {
		t.Fatalf("page %v total %d err %v", len(page), total2, err)
	}
	// Filters narrow the list.
	fr, n, _ := MissingList(ctx, db, MissingFilter{Language: "fr"}, 5000, 0)
	if n != wantFR || len(fr) != wantFR {
		t.Fatalf("fr %d, want %d", n, wantFR)
	}
	mv, n, _ := MissingList(ctx, db, MissingFilter{Kind: "movie"}, 5000, 0)
	if n != subPinMovies/2 || len(mv) != n {
		t.Fatalf("movies %d", n)
	}
	one, n, _ := MissingList(ctx, db, MissingFilter{TitleID: b.movies[0]}, 5000, 0)
	if n != 1 || len(one) != 1 || one[0].Title != "Movie 000" {
		t.Fatalf("one title: %d %+v", n, one)
	}
	none, n, err := MissingList(ctx, db, MissingFilter{TitleID: b.movies[1]}, 5000, 0)
	if err != nil || n != 0 || none == nil || len(none) != 0 {
		t.Fatalf("a title with only present subtitles: %d %v %v", n, none, err)
	}
	// A retry time is shown when one is set.
	var timed int
	for _, r := range rows {
		if r.NextTryAt != nil {
			timed++
		}
	}
	if timed == 0 {
		t.Fatal("no row carries a next try time")
	}
}

func TestSubPinMissingTitlesLargeLibrary(t *testing.T) {
	b := subPinSeed(t)
	got, err := MissingTitles(context.Background(), b.f.st.Read())
	if err != nil {
		t.Fatal(err)
	}
	// The show comes first by sort name, then 100 movies with one missing each.
	if len(got) != 1+subPinMovies/2 {
		t.Fatalf("%d titles", len(got))
	}
	wantShow := len(b.missingEps())
	for i := range b.epFile {
		if b.epFile[i] != 0 && b.epState[i] == "present" && i%10 == 0 {
			wantShow++
		}
	}
	if got[0].Name != "Big Show" || got[0].Kind != "show" || got[0].Count != wantShow || got[0].ID != b.show {
		t.Fatalf("show %+v, want count %d", got[0], wantShow)
	}
	if got[1].Name != "Movie 000" || got[1].Count != 1 || got[1].Kind != "movie" {
		t.Fatalf("movie %+v", got[1])
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i].Name < got[j].Name }) {
		t.Fatal("titles not in name order")
	}
}

func TestSubPinFilesOf(t *testing.T) {
	b := subPinSeed(t)
	ctx := context.Background()
	db := b.f.st.Read()
	ids, err := FilesOf(ctx, db, b.show, 0)
	if err != nil {
		t.Fatal(err)
	}
	var want []int64
	for i, id := range b.epFile {
		if id != 0 && b.epState[i] == "present" {
			want = append(want, id)
		}
	}
	if fmt.Sprint(ids) != fmt.Sprint(want) {
		t.Fatalf("%d files, want %d (present main files in id order)", len(ids), len(want))
	}
	one, _ := FilesOf(ctx, db, b.show, b.epItem[1])
	if len(one) != 1 || one[0] != b.epFile[1] {
		t.Fatalf("one item: %v", one)
	}
	// An episode with no File, an episode whose File is missing, an artist and an
	// unknown title all return nil with no error.
	for _, x := range []struct{ title, item int64 }{{b.show, b.epItem[39]}, {b.show, b.epItem[49]}, {b.f.artist, 0}, {999999, 0}} {
		got, err := FilesOf(ctx, db, x.title, x.item)
		if err != nil || len(got) != 0 {
			t.Fatalf("title %d item %d: %v %v", x.title, x.item, got, err)
		}
	}
}

func TestSubPinLoadFileContext(t *testing.T) {
	f := newFix(t)
	s := &Service{Store: f.st}
	fc, err := s.load(context.Background(), f.ep1File)
	if err != nil {
		t.Fatal(err)
	}
	if fc.file.ID != f.ep1File || fc.item.ID != f.ep1 || fc.version.ID != f.showVer || fc.title.ID != f.show || fc.lib != f.lib {
		t.Fatalf("context %+v", fc)
	}
	if fc.video() != f.abs("Quiet Harbour (2019)/Season 01/Quiet Harbour - S01E02.mkv") {
		t.Fatalf("video %s", fc.video())
	}
	v := fc.videoFacts()
	if v.Kind != "episode" || v.Title != "Quiet Harbour" || v.Year != 2019 || v.Season != 1 || v.Episode != 2 || v.ReleaseName != "Quiet.Harbour.S01E02.1080p.WEB.h264-GRP" {
		t.Fatalf("facts %+v", v)
	}
	if _, err := s.load(context.Background(), 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown file: %v", err)
	}
	// A File that is not present, or has no slots, is not a main video.
	subPinExec(t, f.st, "UPDATE files SET state = 'missing' WHERE id = ?", f.ep2File)
	if _, err := s.load(context.Background(), f.ep2File); !errors.Is(err, ErrNotMain) {
		t.Fatalf("missing file: %v", err)
	}
	subPinExec(t, f.st, "UPDATE files SET role = 'artwork' WHERE id = ?", f.movieFile)
	if _, err := s.load(context.Background(), f.movieFile); !errors.Is(err, ErrNotMain) {
		t.Fatalf("artwork file: %v", err)
	}
}

func TestSubPinDueAndStaleFiles(t *testing.T) {
	b := subPinSeed(t)
	ctx := context.Background()
	s := &Service{Store: b.f.st}
	// Due rows are wanted or none_good_enough with no retry time or a past one.
	// Rows with no retry time come first.
	due, err := s.dueFiles(ctx, 40)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 40 {
		t.Fatalf("%d due files, want a full batch of 40", len(due))
	}
	byFile := map[int64]int{}
	for i, id := range b.epFile {
		if id != 0 {
			byFile[id] = i
		}
	}
	for _, id := range due {
		i, ok := byFile[id]
		if ok && b.epState[i] == "present" && i%3 == 1 && i%2 == 1 && i%10 != 0 {
			t.Fatalf("file %d (retry in the future) is due", id)
		}
	}
	all, err := s.dueFiles(ctx, 100000)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, id := range all {
		if seen[id] {
			t.Fatalf("file %d listed twice", id)
		}
		seen[id] = true
	}
	// Expect every present English or French wish that is due, plus the stale
	// ones that were never looked at or were looked at under another wish.
	// The due query ignores the File's state, so a missing File with a due row is
	// listed, while the stale query only takes present main Files.
	for i, id := range b.epFile {
		if id == 0 {
			continue
		}
		looked := i%4 == 0
		current := i%8 == 0
		due := i%3 == 0 || i%3 == 1 && i%2 == 0 || i%10 == 0
		stale := b.epState[i] == "present" && (!looked || !current)
		if want := due || stale; seen[id] != want {
			t.Fatalf("episode %d (%s): listed %v, want %v (due %v stale %v)", i, b.epState[i], seen[id], want, due, stale)
		}
	}
	// No movie was ever looked at, so all of them are stale and listed.
	for i, id := range b.movieFile {
		if !seen[id] {
			t.Fatalf("movie %d is not listed", i)
		}
	}
	// The stale query alone lists never-looked-at Files first, in file id order.
	stale, err := s.staleFiles(ctx, 100000, nil, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	var lastUnlooked int64
	inLooked := false
	for _, id := range stale {
		i, isEp := byFile[id]
		looked := isEp && i%4 == 0
		if looked {
			inLooked = true
			continue
		}
		if inLooked {
			t.Fatalf("never-looked file %d comes after a looked one", id)
		}
		if id < lastUnlooked {
			t.Fatalf("never-looked files not in id order at %d", id)
		}
		lastUnlooked = id
	}
	// A look under the current wish (i%8==0) is not stale.
	cur := map[int64]bool{}
	for _, id := range stale {
		cur[id] = true
	}
	if cur[b.epFile[0]] || !cur[b.epFile[4]] {
		t.Fatalf("episode 0 (current look) stale=%v, episode 4 (old wish) stale=%v", cur[b.epFile[0]], cur[b.epFile[4]])
	}
}

func TestSubPinWishResolution(t *testing.T) {
	b := subPinSeed(t)
	ctx := context.Background()
	db := b.f.st.Read()
	r, err := WishFor(ctx, db, b.show, b.showVer, b.epItem[0])
	if err != nil || r.From != FromDefault || len(r.Wants) != 1 || r.Wants[0].Language != "en" {
		t.Fatalf("default %+v %v", r, err)
	}
	by := domain.System
	if err := SetWish(ctx, b.f.st, b.show, 0, 0, []Want{{Language: "fr"}}, false, by); err != nil {
		t.Fatal(err)
	}
	if err := SetWish(ctx, b.f.st, b.show, b.showVer, 0, []Want{{Language: "de"}}, false, by); err != nil {
		t.Fatal(err)
	}
	if err := SetWish(ctx, b.f.st, b.show, 0, b.epItem[5], []Want{{Language: "es"}}, false, by); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		item int64
		from string
		lang string
	}{{b.epItem[0], FromVersion, "de"}, {b.epItem[5], FromItem, "es"}} {
		r, err := WishFor(ctx, db, b.show, b.showVer, c.item)
		if err != nil || r.From != c.from || len(r.Wants) != 1 || r.Wants[0].Language != c.lang {
			t.Fatalf("item %d: %+v %v", c.item, r, err)
		}
	}
	// A version with no wish of its own falls back to the title's wish.
	r, _ = WishFor(ctx, db, b.show, b.showVer+9999, b.epItem[0])
	if r.From != FromTitle || r.Wants[0].Language != "fr" {
		t.Fatalf("title wish %+v", r)
	}
	ws, err := Wishes(ctx, db, b.show)
	if err != nil || len(ws) != 3 || ws[0].VersionID != nil || ws[0].ItemID != nil || ws[1].VersionID == nil || ws[2].ItemID == nil {
		t.Fatalf("wishes %+v %v", ws, err)
	}
	// Music has no wish, an unknown title is not found and the artist is refused.
	if _, err := WishFor(ctx, db, b.f.artist, 0, 0); !errors.Is(err, ErrMusic) {
		t.Fatalf("artist: %v", err)
	}
	if _, err := Wishes(ctx, db, 999999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	// An episode that belongs to another title is refused.
	if err := SetWish(ctx, b.f.st, b.movies[0], 0, b.epItem[0], []Want{{Language: "en"}}, false, by); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign episode: %v", err)
	}
	// Inherit deletes the row.
	if err := SetWish(ctx, b.f.st, b.show, 0, b.epItem[5], nil, true, by); err != nil {
		t.Fatal(err)
	}
	if ws, _ := Wishes(ctx, db, b.show); len(ws) != 2 {
		t.Fatalf("after inherit %d wishes", len(ws))
	}
}

func TestSubPinListForOrder(t *testing.T) {
	b := subPinSeed(t)
	ctx := context.Background()
	// The File of episode 0 wants en and fr; add a beside row as well.
	ts := database.FormatTime(now())
	if err := b.f.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO subtitles (created_at, created_by, updated_at, updated_by, file_id, language, place, origin, state, relative_path)
			VALUES (?, ?, ?, ?, ?, 'de', 'beside', 'found', 'present', 'x.de.srt')`, ts, sysActor, ts, sysActor, b.epFile[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := ListFor(ctx, b.f.st.Read(), b.epFile[0])
	if err != nil || len(got) != 3 {
		t.Fatalf("%d rows %v", len(got), err)
	}
	// Rows sort by place, then language: beside/de, wish/en, wish/fr.
	order := []string{got[0].Place + "/" + got[0].Language, got[1].Place + "/" + got[1].Language, got[2].Place + "/" + got[2].Language}
	if fmt.Sprint(order) != "[beside/de wish/en wish/fr]" {
		t.Fatalf("order %v", order)
	}
	// A File with no rows gives an empty list rather than nil.
	none, err := ListFor(ctx, b.f.st.Read(), b.epFile[2])
	if err != nil || none == nil || len(none) != 0 {
		t.Fatalf("no rows: %#v %v", none, err)
	}
}

func subPinExec(t *testing.T, st *database.Store, q string, args ...any) {
	t.Helper()
	if err := st.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

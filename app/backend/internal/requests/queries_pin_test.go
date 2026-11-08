package requests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

const pinNow = "2026-10-13T12:00:00Z"

// pinExec runs raw SQL in one write, for seeding rows the way the Library
// would have left them.
func pinExec(t *testing.T, r *rig, q string, args ...any) {
	t.Helper()
	if err := r.st.Write(context.Background(), func(tx *sql.Tx) error { _, err := tx.Exec(q, args...); return err }); err != nil {
		t.Fatal(err)
	}
}

// pinBigLibrary adds n movie titles that each have a version, an item and an
// in_library slot, none of them asked for or followed: the crowd every
// cross-table query must not pick up.
func pinBigLibrary(t *testing.T, r *rig, n int) {
	t.Helper()
	err := r.st.Write(context.Background(), func(tx *sql.Tx) error {
		var folder, rules int64
		if err := tx.QueryRow("SELECT id FROM library_folders ORDER BY id LIMIT 1").Scan(&folder); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT id FROM pick_rules ORDER BY id LIMIT 1").Scan(&rules); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			res, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, year, source_ids, metadata_source, why_added, state)
				VALUES (?, '{}', ?, '{}', 'movie', ?, ?, ?, ?, 'tmdb', 'import', 'active')`,
				pinNow, pinNow, fmt.Sprintf("Crowd %04d", i), fmt.Sprintf("crowd %04d", i), 1990+i%30, fmt.Sprintf(`{"tmdb":"%d"}`, 800000+i))
			if err != nil {
				return err
			}
			tid, _ := res.LastInsertId()
			res, err = tx.Exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
				VALUES (?, '{}', ?, '{}', ?, 'Main', ?, ?, ?, 1)`, pinNow, pinNow, tid, rules, folder, fmt.Sprintf("Crowd %04d", i))
			if err != nil {
				return err
			}
			vid, _ := res.LastInsertId()
			res, err = tx.Exec(`INSERT INTO items (created_at, created_by, updated_at, updated_by, title_id, kind) VALUES (?, '{}', ?, '{}', ?, 'movie')`, pinNow, pinNow, tid)
			if err != nil {
				return err
			}
			iid, _ := res.LastInsertId()
			if _, err := tx.Exec(`INSERT INTO slots (created_at, created_by, updated_at, updated_by, item_id, version_id, state) VALUES (?, '{}', ?, '{}', ?, ?, 'in_library')`,
				pinNow, pinNow, iid, vid); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// CheckReady reads asks, slots, items and versions together. Today: a plain
// ask is ready once any slot of the title is in_library or better_wanted; a
// 4K ask only once a slot of its 4K version (label or Pick rules name) is.
func TestPinCheckReadyPlainAndFourK(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	pinBigLibrary(t, r, 300)
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	// Approved, nothing in yet: the 300 other in_library titles change nothing.
	if n, err := r.svc.CheckReady(ctx); err != nil || n != 0 {
		t.Fatalf("not in yet: %d %v", n, err)
	}
	// The 4K ask: its own ask, approved apart.
	res4, err := r.svc.Ask4K(ctx, r.bo, "693134")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res4.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	// Only the plain copy in: the plain ask is ready, the 4K ask is not.
	pinExec(t, r, `UPDATE slots SET state = 'in_library' WHERE version_id IN (SELECT id FROM versions WHERE title_id = ? AND label = 'Main')`, *got.TitleID)
	n, err := r.svc.CheckReady(ctx)
	if err != nil || n != 1 {
		t.Fatalf("plain copy in: %d %v", n, err)
	}
	state := func(id int64) string {
		var s string
		_ = r.st.Read().QueryRow("SELECT state FROM asks WHERE id = ?", id).Scan(&s)
		return s
	}
	if state(res.Ask.ID) != "available" || state(res4.Ask.ID) != "approved" {
		t.Fatalf("states %s / %s", state(res.Ask.ID), state(res4.Ask.ID))
	}
	// The 4K copy arrives (better_wanted counts too): the 4K ask is ready.
	pinExec(t, r, `UPDATE slots SET state = 'better_wanted' WHERE version_id IN (SELECT id FROM versions WHERE title_id = ? AND label = '4K' COLLATE NOCASE)`, *got.TitleID)
	if n, err := r.svc.CheckReady(ctx); err != nil || n != 1 {
		t.Fatalf("4K copy in: %d %v", n, err)
	}
	if state(res4.Ask.ID) != "available" {
		t.Fatalf("4K ask %s", state(res4.Ask.ID))
	}
	// Run again: nothing left to mark.
	if n, _ := r.svc.CheckReady(ctx); n != 0 {
		t.Fatalf("again: %d", n)
	}
}

// fourKVersion matches by label (any case) or by the 4K Pick rules name.
func TestPinHasFourKByLabelOrRulesName(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	pinBigLibrary(t, r, 50)
	var tid, folder, rules int64
	_ = r.st.Read().QueryRow("SELECT id FROM titles WHERE name = 'Crowd 0007'").Scan(&tid)
	_ = r.st.Read().QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&folder)
	if has, err := HasFourK(ctx, r.st.Read(), tid); err != nil || has {
		t.Fatalf("plain only: %v %v", has, err)
	}
	// A version labelled "4k" (lower case) counts.
	_ = r.st.Read().QueryRow("SELECT id FROM pick_rules WHERE name != '4K' ORDER BY id LIMIT 1").Scan(&rules)
	pinExec(t, r, `INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name)
		VALUES (?, '{}', ?, '{}', ?, '4k', ?, ?, 'x')`, pinNow, pinNow, tid, rules, folder)
	if has, _ := HasFourK(ctx, r.st.Read(), tid); !has {
		t.Fatal("label 4k not seen")
	}
	// A differently labelled version on the 4K Pick rules counts.
	var tid2, rules4 int64
	_ = r.st.Read().QueryRow("SELECT id FROM titles WHERE name = 'Crowd 0008'").Scan(&tid2)
	_ = r.st.Read().QueryRow("SELECT id FROM pick_rules WHERE name = '4K'").Scan(&rules4)
	pinExec(t, r, `INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name)
		VALUES (?, '{}', ?, '{}', ?, 'Ultra', ?, ?, 'y')`, pinNow, pinNow, tid2, rules4, folder)
	if has, _ := HasFourK(ctx, r.st.Read(), tid2); !has {
		t.Fatal("4K rules not seen")
	}
	// A title with no versions at all: no 4K, no error.
	if has, err := HasFourK(ctx, r.st.Read(), 987654); err != nil || has {
		t.Fatalf("unknown title %v %v", has, err)
	}
}

// Following joins follows to titles: removed titles are hidden, order is the
// title's sort name then id, a NULL year stays nil, other people's follows
// stay out.
func TestPinFollowingJoinAndOrder(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	pinBigLibrary(t, r, 400)
	pinExec(t, r, `UPDATE titles SET year = NULL WHERE name = 'Crowd 0003'`)
	pinExec(t, r, `UPDATE titles SET state = 'removed' WHERE name = 'Crowd 0002'`)
	pinExec(t, r, `UPDATE titles SET sort_name = 'crowd 0001' WHERE name = 'Crowd 0399'`) // ties with 0001: id breaks it
	for _, n := range []string{"Crowd 0399", "Crowd 0003", "Crowd 0002", "Crowd 0001", "Crowd 0200"} {
		pinExec(t, r, `INSERT INTO follows (user_id, title_id, created_at) SELECT ?, id, ? FROM titles WHERE name = ?`, r.ana.UserID, pinNow, n)
	}
	pinExec(t, r, `INSERT INTO follows (user_id, title_id, created_at) SELECT ?, id, ? FROM titles WHERE name = 'Crowd 0100'`, r.bo.UserID, pinNow)
	fs, err := r.svc.Following(ctx, r.ana.UserID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range fs {
		names = append(names, f.Name)
	}
	want := []string{"Crowd 0001", "Crowd 0399", "Crowd 0003", "Crowd 0200"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Fatalf("order %v, want %v", names, want)
	}
	if fs[2].Year != nil || fs[0].Year == nil {
		t.Fatalf("years %+v %+v", fs[2].Year, fs[0].Year)
	}
	if fs[0].Kind != "movie" || fs[0].Unfollow != fmt.Sprintf("/titles/%d/follow", fs[0].TitleID) {
		t.Fatalf("record %+v", fs[0])
	}
	if fs, _ := r.svc.Following(ctx, 424242); len(fs) != 0 {
		t.Fatalf("stranger follows %+v", fs)
	}
}

// FollowTitle on a movie marks it "still to tell" (tmdb_movie_follows) only
// when no file is in the Library yet; a title without a TMDB id is never
// marked, and a plain Follow never marks.
func TestPinFollowTitleMarksOnlyMoviesNotOnFile(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	pinBigLibrary(t, r, 20)
	id := func(name string) int64 {
		var x int64
		_ = r.st.Read().QueryRow("SELECT id FROM titles WHERE name = ?", name).Scan(&x)
		return x
	}
	marked := func(user int64) int {
		var n int
		_ = r.st.Read().QueryRow("SELECT count(*) FROM tmdb_movie_follows WHERE user_id = ?", user).Scan(&n)
		return n
	}
	// In the Library already: followed, not marked.
	if err := r.svc.FollowTitle(ctx, r.bo, id("Crowd 0001")); err != nil {
		t.Fatal(err)
	}
	if marked(r.bo.UserID) != 0 {
		t.Fatal("marked a title that has its file")
	}
	// No file yet: marked.
	pinExec(t, r, `UPDATE slots SET state = 'wanted' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)`, id("Crowd 0002"))
	if err := r.svc.FollowTitle(ctx, r.bo, id("Crowd 0002")); err != nil {
		t.Fatal(err)
	}
	if marked(r.bo.UserID) != 1 {
		t.Fatalf("marked %d, want 1", marked(r.bo.UserID))
	}
	// No TMDB id (NULL from json_extract): followed, not marked.
	pinExec(t, r, `UPDATE titles SET source_ids = '{}' WHERE name = 'Crowd 0003'`)
	pinExec(t, r, `UPDATE slots SET state = 'wanted' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)`, id("Crowd 0003"))
	if err := r.svc.FollowTitle(ctx, r.bo, id("Crowd 0003")); err != nil {
		t.Fatal(err)
	}
	if marked(r.bo.UserID) != 1 {
		t.Fatalf("no-TMDB title was marked: %d", marked(r.bo.UserID))
	}
	// Plain Follow (not from the bell) never marks.
	pinExec(t, r, `UPDATE slots SET state = 'wanted' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)`, id("Crowd 0004"))
	if err := r.svc.Follow(ctx, r.bo, id("Crowd 0004")); err != nil {
		t.Fatal(err)
	}
	if marked(r.bo.UserID) != 1 {
		t.Fatalf("Follow marked: %d", marked(r.bo.UserID))
	}
}

// stepOf reads slots, items, versions, titles and parts: a movie ask's step
// is the plain copy's, a 4K ask's step is the 4K copy's, and movie items
// with no part (NULL part_id) are included.
func TestPinAskStepSplitsPlainFromFourK(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	pinBigLibrary(t, r, 100)
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	res4, err := r.svc.Ask4K(ctx, r.bo, "693134")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res4.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	pinExec(t, r, "UPDATE jobs SET state = 'done'")
	pinExec(t, r, `UPDATE slots SET state = 'downloading' WHERE version_id IN (SELECT id FROM versions WHERE title_id = ? AND label = 'Main')`, *got.TitleID)
	pinExec(t, r, `UPDATE slots SET state = 'needs_a_decision' WHERE version_id IN (SELECT id FROM versions WHERE title_id = ? AND label = '4K')`, *got.TitleID)
	stepKind := func(id int64) string {
		vs, err := r.svc.Views(ctx, "a.id = ?", id)
		if err != nil || len(vs) != 1 {
			t.Fatalf("views %v %v", vs, err)
		}
		r.svc.AddSteps(ctx, vs, false)
		if vs[0].Step == nil {
			return ""
		}
		return vs[0].Step.Kind
	}
	if k := stepKind(res.Ask.ID); k != "downloading" {
		t.Fatalf("plain step %q", k)
	}
	if k := stepKind(res4.Ask.ID); k != "waiting_on_admin" {
		t.Fatalf("4K step %q", k)
	}
}

// Views joins asks to the people who asked and to users. Today an ask whose
// title_id is still NULL (asked, not approved) shows without a Library
// title, and two askers of one ask show together.
func TestPinViewsWithNullTitleAndPeople(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	pinBigLibrary(t, r, 120)
	a1, err := r.svc.Ask(ctx, r.ana, "900002")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Ask(ctx, r.bo, "900002"); err != nil { // joins the open ask
		t.Fatal(err)
	}
	a2, err := r.svc.Ask(ctx, r.bo, "900003")
	if err != nil {
		t.Fatal(err)
	}
	vs, err := r.svc.Views(ctx, "1 = 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 {
		t.Fatalf("%d views", len(vs))
	}
	by := map[int64]View{}
	for _, v := range vs {
		by[v.ID] = v
	}
	v1, v2 := by[a1.Ask.ID], by[a2.Ask.ID]
	if v1.TitleID != nil || v2.TitleID != nil {
		t.Fatalf("title ids %v %v on asks not approved", v1.TitleID, v2.TitleID)
	}
	if len(v1.People) != 2 || len(v2.People) != 1 || v1.State != "asked" {
		t.Fatalf("people %d/%d state %s", len(v1.People), len(v2.People), v1.State)
	}
}

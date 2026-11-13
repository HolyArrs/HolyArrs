package requests

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// copyRow is one Version as the 4K tests read it.
type copyRow struct {
	id, rules, folder int64
	label             string
	main, fourK       bool
}

func copies(t *testing.T, r *rig, titleID int64) []copyRow {
	t.Helper()
	rows, err := r.st.Read().Query("SELECT id, pick_rules_id, library_folder_id, label, is_main, four_k FROM versions WHERE title_id = ? ORDER BY id", titleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []copyRow
	for rows.Next() {
		var c copyRow
		if err := rows.Scan(&c.id, &c.rules, &c.folder, &c.label, &c.main, &c.fourK); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

func rulesID(t *testing.T, r *rig, name string) int64 {
	t.Helper()
	var id int64
	if err := r.st.Read().QueryRow("SELECT id FROM pick_rules WHERE name = ?", name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// fillCopy sets every Slot of one Version.
func fillCopy(t *testing.T, r *rig, versionID int64, state string) {
	t.Helper()
	pinExec(t, r, "UPDATE slots SET state = ? WHERE version_id = ?", state, versionID)
}

func fourKOf(cs []copyRow) (copyRow, bool) {
	for _, c := range cs {
		if c.fourK {
			return c, true
		}
	}
	return copyRow{}, false
}

func plainOf(cs []copyRow) (copyRow, bool) {
	for _, c := range cs {
		if !c.fourK {
			return c, true
		}
	}
	return copyRow{}, false
}

// A plain and a 4K ask for the same season are two asks; each joins only
// its own kind and each approves on its own.
func TestFourKShowAskIsItsOwnAsk(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	plain, err := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	if err != nil {
		t.Fatal(err)
	}
	uhd, err := r.svc.AskShowFourK(ctx, r.bo, "tmdb:100", []int{1}, nil, false)
	if err != nil || uhd.Joined || uhd.Ask.ID == plain.Ask.ID || !uhd.Ask.FourK {
		t.Fatalf("4K ask %+v %v", uhd, err)
	}
	// Another 4K ask for the season joins the 4K one, a plain one the plain one.
	if j, err := r.svc.AskShowFourK(ctx, r.ana, "tvdb:200", []int{1}, nil, false); err != nil || !j.Joined || j.Ask.ID != uhd.Ask.ID {
		t.Fatalf("4K join %+v %v", j, err)
	}
	if j, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{1}); err != nil || !j.Joined || j.Ask.ID != plain.Ask.ID {
		t.Fatalf("plain join %+v %v", j, err)
	}
	// Turning the 4K ask down leaves the plain one waiting.
	if _, err := r.svc.TurnDown(ctx, uhd.Ask.ID, r.luis, ""); err != nil {
		t.Fatal(err)
	}
	if a, _ := get(ctx, r.st.Read(), plain.Ask.ID); a.State != domain.AskAsked {
		t.Fatalf("plain after 4K turned down %+v", a)
	}
	got, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cs := copies(t, r, *got.TitleID); len(cs) != 1 || cs[0].fourK {
		t.Fatalf("plain approval copies %+v", cs)
	}
}

// A 4K ask on a show in the Library adds the 4K Version on the 4K Pick
// rules; the main copy keeps its own rules and folder.
func TestFourKShowAskAddsFourKVersion(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	plain, _ := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	got, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	before := copies(t, r, *got.TitleID)
	uhd, err := r.svc.AskShowFourK(ctx, r.bo, "tmdb:100", []int{1}, nil, false)
	if err != nil || uhd.Ask.State != domain.AskAsked {
		t.Fatalf("4K on owned show %+v %v", uhd, err)
	}
	if _, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	cs := copies(t, r, *got.TitleID)
	k, ok := fourKOf(cs)
	if len(cs) != 2 || !ok || k.rules != rulesID(t, r, mediafiles.FourKName) || k.main {
		t.Fatalf("copies %+v", cs)
	}
	if cs[0] != before[0] {
		t.Fatalf("main copy changed %+v -> %+v", before[0], cs[0])
	}
	// Plain copy full, 4K copy empty: the plain ask is done, the 4K one not.
	fillCopy(t, r, cs[0].id, "in_library")
	if _, err := r.svc.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	if a, _ := get(ctx, r.st.Read(), plain.Ask.ID); a.State != domain.AskAvailable {
		t.Fatalf("plain %s", a.State)
	}
	if a, _ := get(ctx, r.st.Read(), uhd.Ask.ID); a.State != domain.AskApproved {
		t.Fatalf("4K done on plain files: %s", a.State)
	}
	fillCopy(t, r, k.id, "in_library")
	if _, err := r.svc.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	if a, _ := get(ctx, r.st.Read(), uhd.Ask.ID); a.State != domain.AskAvailable {
		t.Fatalf("4K %s", a.State)
	}
	// The 4K copy holds season 1 now: another 4K ask for it is in the Library.
	var in *InLibraryError
	if _, err := r.svc.AskShowFourK(ctx, r.ana, "tmdb:100", []int{1}, nil, false); !errors.As(err, &in) || !in.Ready {
		t.Fatalf("4K again %v", err)
	}
}

// A 4K ask for a show not in the Library adds only what was asked: the show
// with its main copy as the 4K copy, on the 4K rules (as a 4K movie ask
// does). The ask is one ask, done on the 4K files.
func TestFourKShowAskOnNewShowAddsOnlyTheFourKCopy(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	uhd, err := r.svc.AskShowFourK(ctx, r.ana, "tmdb:100", []int{1}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var mine int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM asks WHERE asked_by = ?", r.ana.UserID).Scan(&mine)
	if mine != 1 {
		t.Fatalf("a 4K show ask counts as %d asks", mine)
	}
	got, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cs := copies(t, r, *got.TitleID)
	fourKRules := rulesID(t, r, mediafiles.FourKName)
	if len(cs) != 1 || !cs[0].fourK || !cs[0].main || cs[0].rules != fourKRules || cs[0].label != mediafiles.FourKName {
		t.Fatalf("copies %+v", cs)
	}
	if keeps := seasonKeeps(t, r.st, *got.TitleID); !keeps[1] || keeps[2] {
		t.Fatalf("kept seasons %v", keeps)
	}
	fillCopy(t, r, cs[0].id, "in_library")
	if _, err := r.svc.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	if a, _ := get(ctx, r.st.Read(), uhd.Ask.ID); a.State != domain.AskAvailable {
		t.Fatalf("4K %s", a.State)
	}
	// A plain ask for season 1 is not in the Library: there is no plain copy.
	plain, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{1})
	if err != nil || plain.Ask.FourK || plain.Ask.State != domain.AskAsked {
		t.Fatalf("plain ask after the 4K copy %+v %v", plain, err)
	}
	// A later 4K ask for season 2 keeps it up to date on the 4K copy and adds
	// no other copy.
	more, err := r.svc.AskShowFourK(ctx, r.ana, "tmdb:100", []int{2}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, more.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if cs := copies(t, r, *got.TitleID); len(cs) != 1 {
		t.Fatalf("copies after season 2 %+v", cs)
	}
	if keeps := seasonKeeps(t, r.st, *got.TitleID); !keeps[2] {
		t.Fatalf("season 2 not kept %v", keeps)
	}
}

// A 4K ask for the whole show or future seasons only adds the 4K copy only too.
func TestFourKWholeAndFutureShowAskAddOnlyTheFourKCopy(t *testing.T) {
	for _, future := range []bool{false, true} {
		r := showRig(t)
		ctx := context.Background()
		uhd, err := r.svc.AskShowFourK(ctx, r.ana, "tmdb:100", nil, nil, future)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if cs := copies(t, r, *got.TitleID); len(cs) != 1 || !cs[0].fourK || !cs[0].main {
			t.Fatalf("future %v copies %+v", future, cs)
		}
	}
}

// A show whose only copy is the 4K one (as a 4K ask adds a new show): a 4K
// ask adds no copy, a plain ask adds the plain one and is not done on the
// 4K files.
func TestPlainShowAskBesideFourKOnlyShow(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	tid, _, err := shows.New(r.svc.Library).AddShow(ctx, "tmdb:100", shows.ShowAddOptions{Seasons: "none", MainFourK: true, PickRulesID: rulesID(t, r, mediafiles.FourKName)}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	uhd, err := r.svc.AskShowFourK(ctx, r.ana, "tmdb:100", []int{1}, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	cs := copies(t, r, tid)
	if len(cs) != 1 || !cs[0].fourK || !cs[0].main || cs[0].label != mediafiles.FourKName || cs[0].rules != rulesID(t, r, mediafiles.FourKName) {
		t.Fatalf("4K-only copies %+v", cs)
	}
	got := domain.Ask{TitleID: &tid}
	fillCopy(t, r, cs[0].id, "in_library")
	plain, err := r.svc.AskShow(ctx, r.bo, "tmdb:100", []int{1})
	if err != nil || plain.Ask.FourK || plain.Ask.State != domain.AskAsked {
		t.Fatalf("plain on 4K-only %+v %v", plain, err)
	}
	if _, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	cs = copies(t, r, *got.TitleID)
	p, ok := plainOf(cs)
	if len(cs) != 2 || !ok || p.rules == rulesID(t, r, mediafiles.FourKName) {
		t.Fatalf("plain copy %+v", cs)
	}
	if a, _ := get(ctx, r.st.Read(), plain.Ask.ID); a.State != domain.AskApproved {
		t.Fatalf("plain ask done on 4K files: %s", a.State)
	}
	fillCopy(t, r, p.id, "in_library")
	if _, err := r.svc.CheckReady(ctx); err != nil {
		t.Fatal(err)
	}
	if a, _ := get(ctx, r.st.Read(), plain.Ask.ID); a.State != domain.AskAvailable {
		t.Fatalf("plain ask %s", a.State)
	}
}

// 4K asks offer the plain choices: single episodes and future seasons only.
func TestFourKShowAskEpisodesAndFutureOnly(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	eps, err := r.svc.AskShowFourK(ctx, r.ana, "tmdb:100", nil, []domain.AskEpisodes{{Season: 2, Episodes: []int{1}}}, false)
	if err != nil || !eps.Ask.FourK || len(eps.Ask.Episodes) != 1 {
		t.Fatalf("4K episodes %+v %v", eps, err)
	}
	fut, err := r.svc.AskShowFourK(ctx, r.bo, "tmdb:100", nil, nil, true)
	if err != nil || !fut.Ask.FourK || !fut.Ask.FutureOnly || fut.Ask.ID == eps.Ask.ID {
		t.Fatalf("4K future %+v %v", fut, err)
	}
	if _, err := r.svc.Approve(ctx, eps.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, fut.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	// The episodes ask added the show with its 4K copy only; the future one adds none.
	a, _ := get(ctx, r.st.Read(), fut.Ask.ID)
	if cs := copies(t, r, *a.TitleID); len(cs) != 1 || !cs[0].fourK {
		t.Fatalf("copies %+v", cs)
	}
}

// Plain and 4K approved together land on one copy of each.
func TestPlainAndFourKShowApprovedTogether(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	plain, _ := r.svc.AskShow(ctx, r.ana, "tmdb:100", nil)
	uhd, _ := r.svc.AskShowFourK(ctx, r.bo, "tmdb:100", nil, nil, false)
	if _, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cs := copies(t, r, *got.TitleID)
	if len(cs) != 2 || cs[0].fourK || !cs[1].fourK {
		t.Fatalf("copies %+v", cs)
	}
}

// The mark, not the name, makes the 4K copy: renaming it or its Pick
// rules keeps it the 4K copy.
func TestFourKMarkSurvivesRenames(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	plain, _ := r.svc.AskShow(ctx, r.ana, "tmdb:100", []int{1})
	got, _ := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{})
	uhd, _ := r.svc.AskShowFourK(ctx, r.bo, "tmdb:100", []int{1}, nil, false)
	if _, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	pinExec(t, r, "UPDATE versions SET label = 'Big screen' WHERE title_id = ? AND four_k = 1", *got.TitleID)
	pinExec(t, r, "UPDATE pick_rules SET name = 'Ultra' WHERE name = ?", mediafiles.FourKName)
	if vid, err := fourKVersion(ctx, r.st.Read(), *got.TitleID); err != nil || vid == 0 {
		t.Fatalf("4K after renames %d %v", vid, err)
	}
	// With the 4K rules renamed and no 4K setting, a new 4K ask cannot be approved.
	again, err := r.svc.Ask4K(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, again.Ask.ID, r.luis, ApproveOptions{}); !errors.Is(err, ErrNoFourKRules) {
		t.Fatalf("no 4K rules: %v", err)
	}
	if ok, err := FourKReady(ctx, r.st.Read(), "show"); err != nil || ok {
		t.Fatalf("4K ready %v %v", ok, err)
	}
	// The 4K show setting names the renamed rules: 4K works again.
	set, _ := ReadSettings(ctx, r.st.Read())
	set.ShowFourKPickRulesID = rulesID(t, r, "Ultra")
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	if ok, _ := FourKReady(ctx, r.st.Read(), "show"); !ok {
		t.Fatal("4K show not ready with the setting")
	}
	if ok, _ := FourKReady(ctx, r.st.Read(), "movie"); ok {
		t.Fatal("4K movie ready without rules")
	}
}

// The 4K settings check the Library folder's kind and the Pick rules.
func TestFourKSettingsChecks(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	var movies, showsF int64
	_ = r.st.Read().QueryRow("SELECT id FROM library_folders WHERE kind = 'movie'").Scan(&movies)
	_ = r.st.Read().QueryRow("SELECT id FROM library_folders WHERE kind = 'show'").Scan(&showsF)
	set, _ := ReadSettings(ctx, r.st.Read())
	bad := set
	bad.ShowFourKLibraryFolderID = movies
	if err := SaveSettings(ctx, r.st, bad, domain.System); !errors.Is(err, library.ErrNotShowFolder) {
		t.Fatalf("movie folder as 4K show folder: %v", err)
	}
	bad = set
	bad.FourKPickRulesID = 99999
	if err := SaveSettings(ctx, r.st, bad, domain.System); !errors.Is(err, library.ErrNoPickRules) {
		t.Fatalf("missing rules: %v", err)
	}
	set.ShowFourKLibraryFolderID = showsF
	set.FourKLibraryFolderID = movies
	if err := SaveSettings(ctx, r.st, set, domain.System); err != nil {
		t.Fatal(err)
	}
	back, _ := ReadSettings(ctx, r.st.Read())
	if back.ShowFourKLibraryFolderID != showsF || back.FourKLibraryFolderID != movies {
		t.Fatalf("read back %+v", back)
	}
}

// A 4K ask from someone whose asks are approved straight away waits for
// staff unless they may also auto-approve 4K.
func TestFourKAskAutoApproveNeedsItsOwnSwitch(t *testing.T) {
	r := showRig(t)
	ctx := context.Background()
	auto := r.ana
	auto.AutoApprove = true
	res, err := r.svc.AskShowFourK(ctx, auto, "tmdb:100", []int{1}, nil, false)
	if err != nil || res.Ask.State != domain.AskAsked {
		t.Fatalf("4K auto without switch %+v %v", res, err)
	}
	if res, err := r.svc.Ask4K(ctx, auto, "693134"); err != nil || res.Ask.State != domain.AskAsked {
		t.Fatalf("4K movie auto without switch %+v %v", res, err)
	}
	auto.UserID, auto.Name, auto.AutoApproveFourK = r.bo.UserID, r.bo.Name, true
	res, err = r.svc.AskShowFourK(ctx, auto, "tmdb:100", []int{1}, nil, false)
	if err != nil || res.Ask.State != domain.AskApproved {
		t.Fatalf("4K auto with switch %+v %v", res, err)
	}
}

// A plain movie ask on a movie whose only copy is the 4K one adds a plain
// copy and is not done on the 4K file.
func TestPlainMovieAskBesideFourKOnlyMovie(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	uhd, _ := r.svc.Ask4K(ctx, r.ana, "693134")
	got, err := r.svc.Approve(ctx, uhd.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cs := copies(t, r, *got.TitleID)
	if len(cs) != 1 || !cs[0].fourK {
		t.Fatalf("4K-only %+v", cs)
	}
	fillCopy(t, r, cs[0].id, "in_library")
	plain, err := r.svc.Ask(ctx, r.bo, "693134")
	if err != nil || plain.Ask.State != domain.AskAsked {
		t.Fatalf("plain on 4K-only %+v %v", plain, err)
	}
	if _, err := r.svc.Approve(ctx, plain.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	if a, _ := get(ctx, r.st.Read(), plain.Ask.ID); a.State != domain.AskApproved {
		t.Fatalf("plain done on 4K file: %s", a.State)
	}
	cs = copies(t, r, *got.TitleID)
	if p, ok := plainOf(cs); len(cs) != 2 || !ok || p.rules == rulesID(t, r, mediafiles.FourKName) {
		t.Fatalf("plain copy %+v", cs)
	}
}

// The migration marks old 4K copies, at most one per title, and a main
// copy only when a 4K ask added it.
func TestFourKBackfill(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE versions SET four_k = 0")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var folder int64
	_ = r.st.Read().QueryRow("SELECT id FROM library_folders LIMIT 1").Scan(&folder)
	fk, std := rulesID(t, r, mediafiles.FourKName), rulesID(t, r, mediafiles.GoodEverydayName)
	add := func(name string, tmdb string, vs ...[3]any) int64 {
		var tid int64
		_ = r.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, source_ids, metadata_source, why_added, state)
				VALUES (?, '{}', ?, '{}', 'movie', ?, ?, ?, 'tmdb', 'import', 'active')`, pinNow, pinNow, name, name, `{"tmdb":"`+tmdb+`"}`)
			if err != nil {
				return err
			}
			tid, _ = res.LastInsertId()
			for i, v := range vs {
				if _, err := tx.Exec(`INSERT INTO versions (created_at, created_by, updated_at, updated_by, title_id, label, pick_rules_id, library_folder_id, folder_name, is_main)
					VALUES (?, '{}', ?, '{}', ?, ?, ?, ?, ?, ?)`, pinNow, pinNow, tid, v[0], v[1], folder, name+v[0].(string), v[2]); err != nil {
					return err
				}
				_ = i
			}
			return nil
		})
		return tid
	}
	two := add("Two", "1", [3]any{"Main", std, 1}, [3]any{"4k", std, 0}, [3]any{"UHD", fk, 0})
	allFour := add("All four", "2", [3]any{"Main", fk, 1})
	asked := add("Asked", "3", [3]any{"Main", fk, 1})
	pinExec(t, r, `INSERT INTO asks (created_at, created_by, updated_at, updated_by, kind, tmdb_id, ref, source_ids, seasons, name, state, asked_by, asked_at, four_k, title_id)
		VALUES (?, '{}', ?, '{}', 'movie', '3', 'tmdb:3', '{}', '[]', 'Asked', 'available', ?, ?, 1, ?)`, pinNow, pinNow, r.ana.UserID, pinNow, asked)
	ms, err := database.EmbeddedMigrations("migrations/main")
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, m := range ms {
		if !strings.Contains(m.Name, "four_k_versions") {
			continue
		}
		for _, stmt := range strings.Split(m.SQL, ";") {
			var lines []string
			for _, l := range strings.Split(stmt, "\n") {
				if !strings.HasPrefix(strings.TrimSpace(l), "--") {
					lines = append(lines, l)
				}
			}
			if q := strings.TrimSpace(strings.Join(lines, "\n")); strings.HasPrefix(q, "UPDATE") {
				pinExec(t, r, q)
				ran++
			}
		}
	}
	if ran != 2 {
		t.Fatalf("ran %d backfill steps", ran)
	}
	if cs := copies(t, r, two); cs[0].fourK || !cs[1].fourK || cs[2].fourK {
		t.Fatalf("two %+v", cs)
	}
	if cs := copies(t, r, allFour); cs[0].fourK {
		t.Fatalf("a Library on 4K for all keeps plain copies %+v", cs)
	}
	if cs := copies(t, r, asked); !cs[0].fourK {
		t.Fatalf("4K-asked main %+v", cs)
	}
}

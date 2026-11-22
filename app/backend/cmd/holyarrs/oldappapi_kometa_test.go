package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Kometa's calls against /radarr and /sonarr, replayed as the arrapi
// library sends them: bulk imports of add objects, the movie and series
// editors with their tag choices, the bulk deletes with their exclusion
// flags, and the season pass. The metadata stand-ins keep every lookup
// local.

// kometaFix serves the app with two lab movies, because a bulk import
// needs more than the one lab movie to show a skip beside an add.
func kometaFix(t *testing.T) *oldAppFix {
	t.Helper()
	md := &metadata.Static{Movies: []metadata.Movie{metadata.Lab().Movies[0],
		{Name: "Paper Harbour", Year: 2023, IDs: map[string]string{"tmdb": "900002"}}}}
	return newOldAppFixMeta(t, md)
}

// kometaMovie is one movie add object, the shape POST /movie takes.
func kometaMovie(profile, tmdb int64, folder, tags string) string {
	return fmt.Sprintf(`{"tmdbId":%d,"title":"x","qualityProfileId":%d,"rootFolderPath":%q,"monitored":true,
		"minimumAvailability":"released","tags":%s,"addOptions":{"searchForMovie":false}}`, tmdb, profile, folder, tags)
}

// tagNums reads a title answer's tags as ids.
func tagNums(t *testing.T, m map[string]any) []int64 {
	t.Helper()
	var out []int64
	for _, x := range m["tags"].([]any) {
		out = append(out, num(x))
	}
	return out
}

// seasonsOn reads a series answer's season switches.
func seasonsOn(m map[string]any) map[int64]bool {
	out := map[int64]bool{}
	for _, x := range m["seasons"].([]any) {
		se := x.(map[string]any)
		out[num(se["seasonNumber"])] = se["monitored"].(bool)
	}
	return out
}

// The movie import: every folder and Pick rules id is checked before
// anything is added, a wrong one refuses the whole call, and movies
// already there, on the Never-add list or not found are skipped.
func TestOldAppKometaMovieImport(t *testing.T) {
	t.Parallel()
	f := kometaFix(t)
	f.turnOn()
	r := "/radarr/api/v3/"
	profile := f.profile()
	tag := num(f.arrOK(f.radarrKey, "POST", r+"tag", `{"label":"kometa"}`, 201).(map[string]any)["id"])
	f.arrOK(f.radarrKey, "POST", r+"movie", kometaMovie(profile, 693134, f.movieDir, "[]"), 201)

	// A wrong root folder refuses the call and adds nothing.
	st, _, b := f.arr(f.radarrKey, "POST", r+"movie/import", "["+kometaMovie(profile, 900002, "/nowhere", "[]")+"]")
	if st != 400 || !strings.Contains(b, "RootFolderPath") {
		t.Fatalf("bad folder: %d %s", st, b)
	}
	arrList(t, "movies after folder refusal", f.arrOK(f.radarrKey, "GET", r+"movie", "", 200), 1)
	// A Pick rules id this side does not list refuses the call too.
	bad := strings.Replace(kometaMovie(profile, 900002, f.movieDir, "[]"),
		fmt.Sprintf(`"qualityProfileId":%d`, profile), `"qualityProfileId":999999`, 1)
	st, _, b = f.arr(f.radarrKey, "POST", r+"movie/import", "["+bad+"]")
	if st != 400 || !strings.Contains(b, "QualityProfileId") {
		t.Fatalf("bad profile: %d %s", st, b)
	}
	arrList(t, "movies after profile refusal", f.arrOK(f.radarrKey, "GET", r+"movie", "", 200), 1)

	// The bulk add skips the movie already there and adds the new one
	// with its tags, in the shape GET /movie/{id} answers.
	body := "[" + kometaMovie(profile, 693134, f.movieDir, "[]") + "," + kometaMovie(profile, 900002, f.movieDir, fmt.Sprintf("[%d]", tag)) + "]"
	added := arrList(t, "import", f.arrOK(f.radarrKey, "POST", r+"movie/import", body, 200), 1)
	m := fields(t, "imported movie", added[0], []string{"id", "tmdbId", "title", "monitored", "hasFile", "statistics", "tags", "qualityProfileId", "path"})
	if num(m["tmdbId"]) != 900002 || num(m["qualityProfileId"]) != profile || m["monitored"] != true {
		t.Fatalf("imported: %v", m)
	}
	if tags := tagNums(t, m); !slices.Equal(tags, []int64{tag}) {
		t.Fatalf("imported tags: %v", tags)
	}
	arrList(t, "movies after import", f.arrOK(f.radarrKey, "GET", r+"movie", "", 200), 2)
	// A movie the metadata server does not know is skipped, not an error.
	arrList(t, "unknown movie import", f.arrOK(f.radarrKey, "POST", r+"movie/import", "["+kometaMovie(profile, 999999, f.movieDir, "[]")+"]", 200), 0)
	// A movie on the Never-add list is skipped too.
	id := num(arrList(t, "paper harbour", f.arrOK(f.radarrKey, "GET", r+"movie?tmdbId=900002", "", 200), 1)[0].(map[string]any)["id"])
	f.arrOK(f.radarrKey, "DELETE", r+"movie/"+arrItoa(id)+"?addImportExclusion=true", "", 200)
	arrList(t, "never-add import", f.arrOK(f.radarrKey, "POST", r+"movie/import", "["+kometaMovie(profile, 900002, f.movieDir, "[]")+"]", 200), 0)
	// More than 500 items refuse the call.
	many := make([]string, 501)
	for i := range many {
		many[i] = `{"tmdbId":1}`
	}
	st, _, b = f.arr(f.radarrKey, "POST", r+"movie/import", "["+strings.Join(many, ",")+"]")
	if st != 400 || !strings.Contains(b, "500") {
		t.Fatalf("too many: %d %s", st, b)
	}
}

// The movie editor: unknown ids are skipped, monitored is taken, and the
// tags follow add, remove and replace, with add as the default.
func TestOldAppKometaMovieEditor(t *testing.T) {
	t.Parallel()
	f := kometaFix(t)
	f.turnOn()
	r := "/radarr/api/v3/"
	profile := f.profile()
	t1 := num(f.arrOK(f.radarrKey, "POST", r+"tag", `{"label":"kometa-a"}`, 201).(map[string]any)["id"])
	t2 := num(f.arrOK(f.radarrKey, "POST", r+"tag", `{"label":"kometa-b"}`, 201).(map[string]any)["id"])
	body := "[" + kometaMovie(profile, 693134, f.movieDir, "[]") + "," + kometaMovie(profile, 900002, f.movieDir, "[]") + "]"
	added := arrList(t, "import", f.arrOK(f.radarrKey, "POST", r+"movie/import", body, 200), 2)
	id1, id2 := num(added[0].(map[string]any)["id"]), num(added[1].(map[string]any)["id"])
	editor := func(b string) []any {
		t.Helper()
		return arrList(t, "editor", f.arrOK(f.radarrKey, "PUT", r+"movie/editor", b, 202), -1)
	}
	wantTags := func(out []any, want []int64) {
		t.Helper()
		if len(out) != 2 {
			t.Fatalf("editor answered %d movies, want 2", len(out))
		}
		for _, x := range out {
			m := x.(map[string]any)
			if m["monitored"] != false {
				t.Errorf("editor monitored: %v", m)
			}
			if tags := tagNums(t, m); !slices.Equal(tags, want) {
				t.Errorf("editor tags %v, want %v", tags, want)
			}
		}
	}
	// Add puts the tags on and switches monitored off; a gone id is skipped.
	wantTags(editor(fmt.Sprintf(`{"movieIds":[%d,%d,999999],"monitored":false,"tags":[%d],"applyTags":"add"}`, id1, id2, t1)), []int64{t1})
	// Remove takes them off.
	wantTags(editor(fmt.Sprintf(`{"movieIds":[%d,%d],"monitored":false,"tags":[%d],"applyTags":"remove"}`, id1, id2, t1)), []int64{})
	// Replace sets exactly the tags sent.
	wantTags(editor(fmt.Sprintf(`{"movieIds":[%d,%d],"tags":[%d,%d],"applyTags":"add"}`, id1, id2, t1, t2)), []int64{t1, t2})
	wantTags(editor(fmt.Sprintf(`{"movieIds":[%d,%d],"tags":[%d],"applyTags":"replace"}`, id1, id2, t2)), []int64{t2})
	// A missing applyTags means add. A tag id no tag has is ignored.
	wantTags(editor(fmt.Sprintf(`{"movieIds":[%d,%d],"tags":[%d,999999]}`, id1, id2, t1)), []int64{t1, t2})
	// No tags field leaves the tags alone, even with replace: only an
	// empty list sent takes them all off.
	wantTags(editor(fmt.Sprintf(`{"movieIds":[%d,%d],"monitored":false,"applyTags":"replace"}`, id1, id2)), []int64{t1, t2})
	// The edits stayed, as a read shows them.
	m := fields(t, "edited movie", f.arrOK(f.radarrKey, "GET", r+"movie/"+arrItoa(id1), "", 200), []string{"monitored", "tags"})
	if m["monitored"] != false || !slices.Equal(tagNums(t, m), []int64{t1, t2}) {
		t.Fatalf("edited movie: %v", m)
	}
	// An unknown applyTags and more than 1000 ids refuse the call.
	st, _, b := f.arr(f.radarrKey, "PUT", r+"movie/editor", fmt.Sprintf(`{"movieIds":[%d],"applyTags":"sideways"}`, id1))
	if st != 400 || !strings.Contains(b, "ApplyTags") {
		t.Fatalf("unknown applyTags: %d %s", st, b)
	}
	many := make([]string, 1001)
	for i := range many {
		many[i] = arrItoa(id1)
	}
	st, _, b = f.arr(f.radarrKey, "PUT", r+"movie/editor", `{"movieIds":[`+strings.Join(many, ",")+`],"monitored":true}`)
	if st != 400 || !strings.Contains(b, "1000") {
		t.Fatalf("too many ids: %d %s", st, b)
	}
}

// The bulk delete: with the delete-files setting off the files stay, the
// exclusion flag puts the title on the Never-add list, a gone id is
// skipped, and a title that cannot be removed leaves the rest to go and
// answers 409 with the count and the first reason.
func TestOldAppKometaMovieDelete(t *testing.T) {
	t.Parallel()
	f := kometaFix(t)
	f.turnOn()
	r := "/radarr/api/v3/"
	profile := f.profile()
	id1 := num(f.arrOK(f.radarrKey, "POST", r+"movie", kometaMovie(profile, 693134, f.movieDir, "[]"), 201).(map[string]any)["id"])
	id2 := num(f.arrOK(f.radarrKey, "POST", r+"movie", kometaMovie(profile, 900002, f.movieDir, "[]"), 201).(map[string]any)["id"])
	// A file on disk and in the Library, as an import leaves it.
	const rel = "Dune Part Two (2024)/Dune.mkv"
	disk := filepath.Join(f.movieDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(disk), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disk, []byte("lab"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.putFile(f.slotsOf(id1)[[2]int{0, 0}], rel, 1080)
	// deleteFiles=true while the setting is off: the title goes, the file stays.
	f.arrOK(f.radarrKey, "DELETE", r+"movie/editor", fmt.Sprintf(`{"movieIds":[%d,999999],"deleteFiles":true,"addImportExclusion":true}`, id1), 200)
	if st, _, _ := f.arr(f.radarrKey, "GET", r+"movie/"+arrItoa(id1), ""); st != 404 {
		t.Fatalf("movie after bulk delete: %d", st)
	}
	var present int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'present' AND relative_path = ?", rel).Scan(&present)
	if present != 1 {
		t.Fatalf("files kept: %d, want 1", present)
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("file on disk: %v", err)
	}
	var never int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM never_add WHERE kind = 'movie'").Scan(&never)
	if never != 1 {
		t.Fatalf("import exclusion: never-add rows %d", never)
	}
	// A protected copy refuses its own title; the rest still go, and the
	// answer says how many failed and why. The gone id is skipped.
	f.putFile(f.slotsOf(id2)[[2]int{0, 0}], "Paper Harbour (2023)/Paper.mkv", 1080)
	f.exec("UPDATE files SET protected = 1 WHERE relative_path = ?", "Paper Harbour (2023)/Paper.mkv")
	st, _, b := f.arr(f.radarrKey, "DELETE", r+"movie/editor", fmt.Sprintf(`{"movieIds":[%d,%d],"deleteFiles":false,"addImportExclusion":false}`, id2, id1))
	if st != 409 || !strings.Contains(b, "1 of 2") || !strings.Contains(b, "protected copy") {
		t.Fatalf("protected delete: %d %s", st, b)
	}
	f.arrOK(f.radarrKey, "GET", r+"movie/"+arrItoa(id2), "", 200)
}

// Kometa sends a movie's own folder with path, and arrapi sends Radarr's
// monitor words. The folder is kept as it is; a path outside the root
// folder and an unknown monitor word refuse the call.
func TestOldAppKometaMoviePathAndMonitorWords(t *testing.T) {
	t.Parallel()
	f := kometaFix(t)
	f.turnOn()
	r := "/radarr/api/v3/"
	profile := f.profile()
	withPath := func(tmdb int64, path, monitored string) string {
		return fmt.Sprintf(`{"tmdbId":%d,"title":"x","qualityProfileId":%d,"rootFolderPath":%q,"path":%q,"monitored":%s,
			"minimumAvailability":"released","tags":[],"addOptions":{"searchForMovie":false}}`, tmdb, profile, f.movieDir, path, monitored)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(f.movieDir, "linked away")); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []struct{ body, field string }{
		{withPath(900002, "/elsewhere/Paper Harbour (2023)", "true"), "Path"},
		{withPath(900002, filepath.Join(f.movieDir, "a", "b"), "true"), "Path"},
		// HolyArrs' own holding folder, a NAS system folder and a link out
		// of the Library folder are never a title's folder.
		{withPath(900002, filepath.Join(f.movieDir, ".holyarrs-replaced"), "true"), "Path"},
		{withPath(900002, filepath.Join(f.movieDir, "@eaDir"), "true"), "Path"},
		{withPath(900002, filepath.Join(f.movieDir, "linked away"), "true"), "Path"},
		{withPath(900002, filepath.Join(f.movieDir, "Paper Harbour (2023)"), `"sideways"`), "Monitored"},
	} {
		if st, _, b := f.arr(f.radarrKey, "POST", r+"movie/import", "["+bad.body+"]"); st != 400 || !strings.Contains(b, bad.field) {
			t.Fatalf("%s: %d %s", bad.field, st, b)
		}
	}
	arrList(t, "movies after refusals", f.arrOK(f.radarrKey, "GET", r+"movie", "", 200), 0)
	kept := filepath.Join(f.movieDir, "Harbour, Paper - 2023 [kept]")
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	added := arrList(t, "import", f.arrOK(f.radarrKey, "POST", r+"movie/import", "["+withPath(900002, kept, `"movieOnly"`)+"]", 200), 1)
	m := added[0].(map[string]any)
	if m["path"] != kept || m["monitored"] != true {
		t.Fatalf("the movie's own folder or monitor word was not kept: path %v monitored %v", m["path"], m["monitored"])
	}
}

// arrapi reads a title's tags as ids and then each tag by id, as Kometa's
// remove-by-tag does; an editor's wrong Pick rules id changes nothing; and
// a bulk import of whole lookup answers may pass 1 MiB.
func TestOldAppKometaTagByIDAndEditorChecks(t *testing.T) {
	t.Parallel()
	f := kometaFix(t)
	f.turnOn()
	r := "/radarr/api/v3/"
	profile := f.profile()
	tg := num(f.arrOK(f.radarrKey, "POST", r+"tag", `{"label":"kometa-x"}`, 201).(map[string]any)["id"])
	added := arrList(t, "import", f.arrOK(f.radarrKey, "POST", r+"movie/import", "["+kometaMovie(profile, 900002, f.movieDir, fmt.Sprintf("[%d]", tg))+"]", 200), 1)
	id := num(added[0].(map[string]any)["id"])
	one := fields(t, "tag", f.arrOK(f.radarrKey, "GET", r+"tag/"+arrItoa(tg), "", 200), []string{"id", "label"})
	if one["label"] != "kometa-x" {
		t.Fatalf("tag by id: %v", one)
	}
	detail := f.arrOK(f.radarrKey, "GET", r+"tag/detail/"+arrItoa(tg), "", 200).(map[string]any)
	if ids, _ := detail["movieIds"].([]any); len(ids) != 1 || num(ids[0]) != id {
		t.Fatalf("tag detail by id: %v", detail)
	}
	if st, _, _ := f.arr(f.radarrKey, "GET", r+"tag/999999", ""); st != 404 {
		t.Fatalf("missing tag: %d", st)
	}
	if st, _, b := f.arr(f.radarrKey, "PUT", r+"movie/editor", `{"movieIds":[999999],"monitored":"sideways"}`); st != 400 || !strings.Contains(b, "Monitored") {
		t.Fatalf("editor with an unknown monitor word: %d %s", st, b)
	}
	if st, _, b := f.arr(f.radarrKey, "PUT", r+"movie/editor", fmt.Sprintf(`{"movieIds":[%d],"monitored":false,"qualityProfileId":999999}`, id)); st != 400 || !strings.Contains(b, "QualityProfileId") {
		t.Fatalf("editor with a wrong profile: %d %s", st, b)
	}
	if m := f.arrOK(f.radarrKey, "GET", r+"movie/"+arrItoa(id), "", 200).(map[string]any); m["monitored"] != true {
		t.Fatalf("a refused editor call changed the movie: %v", m)
	}
	// 1.5 MiB of items, each a whole lookup answer; all are already there,
	// so all are skipped, but the body is read.
	long := strings.Repeat("x", 6000)
	items := make([]string, 250)
	for i := range items {
		items[i] = fmt.Sprintf(`{"tmdbId":900002,"title":"x","overview":%q,"qualityProfileId":%d,"rootFolderPath":%q,"monitored":true}`, long, profile, f.movieDir)
	}
	if st, _, b := f.arr(f.radarrKey, "POST", r+"movie/import", "["+strings.Join(items, ",")+"]"); st != 200 {
		t.Fatalf("a large import body was refused: %d %.200s", st, b)
	}
}

// The Sonarr side: the series import, the series editor's tags, the
// season pass with its monitoring choices, and the bulk delete.
func TestOldAppKometaSeries(t *testing.T) {
	t.Parallel()
	f := newOldAppFix(t)
	f.turnOn()
	s := "/sonarr/api/v3/"
	profile := f.profile()
	t1 := num(f.arrOK(f.sonarrKey, "POST", s+"tag", `{"label":"kometa-a"}`, 201).(map[string]any)["id"])
	t2 := num(f.arrOK(f.sonarrKey, "POST", s+"tag", `{"label":"kometa-b"}`, 201).(map[string]any)["id"])
	show := func(folder, tags string) string {
		return fmt.Sprintf(`{"tvdbId":200,"title":"Lab Show","qualityProfileId":%d,"rootFolderPath":%q,"monitored":true,"monitorNewItems":"all",
			"seriesType":"standard","seasonFolder":true,"tags":%s,"addOptions":{"monitor":"all","searchForMissingEpisodes":false}}`, profile, folder, tags)
	}
	// A wrong root folder refuses the call and adds nothing.
	st, _, b := f.arr(f.sonarrKey, "POST", s+"series/import", "["+show("/nowhere", "[]")+"]")
	if st != 400 || !strings.Contains(b, "RootFolderPath") {
		t.Fatalf("bad folder: %d %s", st, b)
	}
	arrList(t, "series after refusal", f.arrOK(f.sonarrKey, "GET", s+"series", "", 200), 0)
	// The import adds the show with its tags, in the GET /series shape.
	added := arrList(t, "import", f.arrOK(f.sonarrKey, "POST", s+"series/import", "["+show(f.showDir, fmt.Sprintf("[%d]", t1))+"]", 200), 1)
	m := fields(t, "imported series", added[0], []string{"id", "tvdbId", "title", "monitored", "seasons", "statistics", "tags", "qualityProfileId", "path"})
	showID := num(m["id"])
	if num(m["tvdbId"]) != 200 || !slices.Equal(tagNums(t, m), []int64{t1}) {
		t.Fatalf("imported: %v", m)
	}
	// A second import skips the show already there and a show not found.
	st, _, b = f.arr(f.sonarrKey, "POST", s+"series/import", "["+show(f.showDir, "[]")+`,{"tvdbId":999,"title":"x","qualityProfileId":`+arrItoa(profile)+`,"rootFolderPath":`+fmt.Sprintf("%q", f.showDir)+`}]`)
	if st != 200 || strings.TrimSpace(b) != "[]" {
		t.Fatalf("skip import: %d %s", st, b)
	}
	// The editor takes monitored and the tag choices; a gone id is skipped.
	editor := func(body string) map[string]any {
		t.Helper()
		out := arrList(t, "editor", f.arrOK(f.sonarrKey, "PUT", s+"series/editor", body, 202), 1)
		return out[0].(map[string]any)
	}
	m = editor(fmt.Sprintf(`{"seriesIds":[%d,999999],"monitored":false,"tags":[%d],"applyTags":"add"}`, showID, t2))
	if m["monitored"] != false || !slices.Equal(tagNums(t, m), []int64{t1, t2}) {
		t.Fatalf("editor add: %v", m)
	}
	m = editor(fmt.Sprintf(`{"seriesIds":[%d],"tags":[%d],"applyTags":"remove"}`, showID, t1))
	if !slices.Equal(tagNums(t, m), []int64{t2}) {
		t.Fatalf("editor remove: %v", m)
	}
	m = editor(fmt.Sprintf(`{"seriesIds":[%d],"tags":[%d],"applyTags":"replace"}`, showID, t1))
	if !slices.Equal(tagNums(t, m), []int64{t1}) {
		t.Fatalf("editor replace: %v", m)
	}
	// The season pass: "none" switches the show and every season off.
	f.arrOK(f.sonarrKey, "POST", s+"seasonPass", fmt.Sprintf(`{"series":[{"id":%d,"monitored":false},{"id":999999,"monitored":true}],"monitoringOptions":{"monitor":"none"}}`, showID), 202)
	if got := f.keptEpisodes(showID); len(got) != 0 {
		t.Fatalf("season pass none kept %v", got)
	}
	m = fields(t, "series after none", f.arrOK(f.sonarrKey, "GET", s+"series/"+arrItoa(showID), "", 200), []string{"monitored", "seasons"})
	if m["monitored"] != false {
		t.Fatalf("show switch after none: %v", m)
	}
	for n, on := range seasonsOn(m) {
		if on {
			t.Errorf("season %d still on after none", n)
		}
	}
	// "all" switches every season but the specials on.
	f.arrOK(f.sonarrKey, "POST", s+"seasonPass", fmt.Sprintf(`{"series":[{"id":%d,"monitored":true}],"monitoringOptions":{"monitor":"all"}}`, showID), 202)
	if got := f.keptEpisodes(showID); !slices.Equal(got, []string{"S1E1", "S1E2", "S2E1", "S2E2"}) {
		t.Fatalf("season pass all kept %v", got)
	}
	m = fields(t, "series after all", f.arrOK(f.sonarrKey, "GET", s+"series/"+arrItoa(showID), "", 200), []string{"monitored", "seasons"})
	if on := seasonsOn(m); m["monitored"] != true || on[0] || !on[1] || !on[2] {
		t.Fatalf("seasons after all: %v", m)
	}
	// latestSeason keeps only the newest season, firstSeason only the first.
	pass := func(monitor string) {
		t.Helper()
		f.arrOK(f.sonarrKey, "POST", s+"seasonPass", fmt.Sprintf(`{"series":[{"id":%d,"monitored":true}],"monitoringOptions":{"monitor":%q}}`, showID, monitor), 202)
	}
	pass("latestSeason")
	if got := f.keptEpisodes(showID); !slices.Equal(got, []string{"S2E1", "S2E2"}) {
		t.Fatalf("season pass latestSeason kept %v", got)
	}
	pass("firstSeason")
	if got := f.keptEpisodes(showID); !slices.Equal(got, []string{"S1E1", "S1E2"}) {
		t.Fatalf("season pass firstSeason kept %v", got)
	}
	// A choice that picks single episodes is refused before anything
	// changes: HolyArrs keeps whole seasons.
	st, _, b = f.arr(f.sonarrKey, "POST", s+"seasonPass", fmt.Sprintf(`{"series":[{"id":%d,"monitored":false}],"monitoringOptions":{"monitor":"future"}}`, showID))
	if st != 400 || !strings.Contains(b, "Monitor") {
		t.Fatalf("future: %d %s", st, b)
	}
	if m := f.arrOK(f.sonarrKey, "GET", s+"series/"+arrItoa(showID), "", 200).(map[string]any); m["monitored"] != true {
		t.Fatalf("a refused season pass changed the show: %v", m)
	}
	// A season list switches just those seasons; skip leaves the rest.
	f.arrOK(f.sonarrKey, "POST", s+"seasonPass", fmt.Sprintf(`{"series":[{"id":%d,"monitored":false,"seasons":[{"seasonNumber":1,"monitored":false},{"seasonNumber":2,"monitored":true}]}],"monitoringOptions":{"monitor":"skip"}}`, showID), 202)
	if got := f.keptEpisodes(showID); !slices.Equal(got, []string{"S2E1", "S2E2"}) {
		t.Fatalf("season list kept %v", got)
	}
	if m := f.arrOK(f.sonarrKey, "GET", s+"series/"+arrItoa(showID), "", 200).(map[string]any); m["monitored"] != false {
		t.Fatalf("show switch after season list: %v", m)
	}
	// Leaving monitored out changes only the seasons, as in Sonarr.
	f.arrOK(f.sonarrKey, "POST", s+"seasonPass", fmt.Sprintf(`{"series":[{"id":%d,"seasons":[{"seasonNumber":1,"monitored":true}]}]}`, showID), 202)
	if m := f.arrOK(f.sonarrKey, "GET", s+"series/"+arrItoa(showID), "", 200).(map[string]any); m["monitored"] != false {
		t.Fatalf("a season pass without monitored changed the show switch: %v", m)
	}
	if got := f.keptEpisodes(showID); !slices.Equal(got, []string{"S1E1", "S1E2", "S2E1", "S2E2"}) {
		t.Fatalf("season pass without monitored kept %v", got)
	}
	// The sent seasons switch first, then the choice reads them again:
	// all turns back on a season the list just turned off.
	f.arrOK(f.sonarrKey, "POST", s+"seasonPass", fmt.Sprintf(`{"series":[{"id":%d,"monitored":true,"seasons":[{"seasonNumber":1,"monitored":false}]}],"monitoringOptions":{"monitor":"all"}}`, showID), 202)
	if got := f.keptEpisodes(showID); !slices.Equal(got, []string{"S1E1", "S1E2", "S2E1", "S2E2"}) {
		t.Fatalf("season list then all kept %v", got)
	}
	// More than 1000 shows in one pass is refused before any change.
	many := make([]string, 1001)
	for i := range many {
		many[i] = fmt.Sprintf(`{"id":%d,"monitored":true}`, showID)
	}
	if st, _, b := f.arr(f.sonarrKey, "POST", s+"seasonPass", `{"series":[`+strings.Join(many, ",")+`],"monitoringOptions":{"monitor":"all"}}`); st != 400 {
		t.Fatalf("1001 shows: %d %s", st, b)
	}
	// The bulk delete takes the show out and puts it on the Never-add list.
	f.arrOK(f.sonarrKey, "DELETE", s+"series/editor", fmt.Sprintf(`{"seriesIds":[%d],"deleteFiles":false,"addImportListExclusion":true}`, showID), 200)
	arrList(t, "series after delete", f.arrOK(f.sonarrKey, "GET", s+"series", "", 200), 0)
	var never int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM never_add WHERE kind = 'show'").Scan(&never)
	if never != 1 {
		t.Fatalf("import list exclusion: never-add rows %d", never)
	}
}

// Two adds sent at once that name the same folder for two movies: one
// gets the folder, the other does not, so one folder never holds two
// titles.
func TestOldAppKometaOneFolderOneTitle(t *testing.T) {
	t.Parallel()
	f := kometaFix(t)
	f.turnOn()
	r := "/radarr/api/v3/"
	profile := f.profile()
	kept := filepath.Join(f.movieDir, "Shared Folder")
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	first := num(atoiAny(metadata.Lab().Movies[0].IDs["tmdb"]))
	body := func(tmdb int64) string {
		return fmt.Sprintf(`[{"tmdbId":%d,"title":"x","qualityProfileId":%d,"rootFolderPath":%q,"path":%q,"monitored":true}]`, tmdb, profile, f.movieDir, kept)
	}
	done := make(chan int, 2)
	for _, tmdb := range []int64{first, 900002} {
		go func(tmdb int64) {
			st, _, _ := f.arr(f.radarrKey, "POST", r+"movie/import", body(tmdb))
			done <- st
		}(tmdb)
	}
	<-done
	<-done
	var n int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM versions WHERE folder_name = 'Shared Folder'").Scan(&n)
	if n != 1 {
		t.Fatalf("%d titles hold the one folder", n)
	}
}

// atoiAny reads a number from text, for the lab's ids.
func atoiAny(s string) any {
	var n float64
	_, _ = fmt.Sscan(s, &n)
	return n
}

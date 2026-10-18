package api

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

type titleDefaultsAnswer struct {
	Movie library.KindDefaults `json:"movie"`
	Show  library.KindDefaults `json:"show"`
}

// thenOf reads the chain steps of an add's first job.
func thenOf(t *testing.T, st *store.Store, jobID int64) []string {
	t.Helper()
	var j domain.Job
	if err := store.Get(context.Background(), st.Read(), "jobs", jobID, &j); err != nil {
		t.Fatal(err)
	}
	var out []string
	raw, _ := j.Params["then"].([]any)
	for _, s := range raw {
		out = append(out, fmt.Sprint(s))
	}
	return out
}

type addAnswer struct {
	Title TitleView `json:"title"`
	JobID int64     `json:"job_id"`
}

func addMovie(t *testing.T, base, body string) addAnswer {
	t.Helper()
	var a addAnswer
	_ = json.Unmarshal(do(t, "POST", base+"/api/v1/titles", body, 201), &a)
	return a
}

func folder(t *testing.T, base, kind string) FolderView {
	t.Helper()
	var lf FolderView
	_ = json.Unmarshal(do(t, "POST", base+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":%q,"label":%q}`, t.TempDir(), kind, kind+t.Name()), 201), &lf)
	return lf
}

// route: GET /api/v1/settings/title-defaults
// route: PUT /api/v1/settings/title-defaults
func TestTitleDefaultsBuiltIn(t *testing.T) {
	srv, _, _ := libSetup(t)
	var d titleDefaultsAnswer
	_ = json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/settings/title-defaults", "", 200), &d)
	want := titleDefaultsAnswer{Movie: library.KindDefaults{KeepUpToDate: true, SearchOnAdd: true}, Show: library.KindDefaults{KeepUpToDate: true}}
	if d != want {
		t.Fatalf("built in %+v", d)
	}
}

func TestTitleDefaultsSaveAndValidate(t *testing.T) {
	srv, _, _ := libSetup(t)
	movies := folder(t, srv.URL, "movie")
	shows := folder(t, srv.URL, "show")
	music := ruleByName(t, srv.URL, domain.LosslessFirstName)
	path := srv.URL + "/api/v1/settings/title-defaults"
	do(t, "PUT", path, fmt.Sprintf(`{"movie":{"library_folder_id":%d}}`, shows.ID), 400)
	do(t, "PUT", path, fmt.Sprintf(`{"show":{"library_folder_id":%d}}`, movies.ID), 400)
	do(t, "PUT", path, fmt.Sprintf(`{"movie":{"pick_rules_id":%d}}`, music.ID), 400)
	do(t, "PUT", path, `{"movie":{"pick_rules_id":99999}}`, 400)
	do(t, "PUT", path, `{"movie":`, 400)
	var d titleDefaultsAnswer
	_ = json.Unmarshal(do(t, "PUT", path, fmt.Sprintf(`{"movie":{"library_folder_id":%d,"search_on_add":false}}`, movies.ID), 200), &d)
	// Left-out fields keep their value.
	if d.Movie.LibraryFolderID != movies.ID || d.Movie.SearchOnAdd || !d.Movie.KeepUpToDate || !d.Show.KeepUpToDate || d.Show.SearchOnAdd {
		t.Fatalf("saved %+v", d)
	}
	_ = json.Unmarshal(do(t, "GET", path, "", 200), &d)
	if d.Movie.LibraryFolderID != movies.ID || d.Movie.SearchOnAdd {
		t.Fatalf("read back %+v", d)
	}
}

func TestAddWithoutSavedDefaultsKeepsOldBehaviour(t *testing.T) {
	srv, st, _ := libSetup(t)
	first := folder(t, srv.URL, "movie")
	folder(t, srv.URL, "movie")
	a := addMovie(t, srv.URL, `{"id":"tmdb:693134"}`)
	if a.Title.LibraryFolderID != first.ID || !a.Title.KeepUpToDate || slices.Contains(thenOf(t, st, a.JobID), library.KindSearchSlot) {
		t.Fatalf("old behaviour: %+v then %v", a.Title, thenOf(t, st, a.JobID))
	}
}

func TestAddUsesSavedMovieDefaults(t *testing.T) {
	srv, st, _ := libSetup(t)
	folder(t, srv.URL, "movie")
	second := folder(t, srv.URL, "movie")
	var p pickRulesDetail
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, "Mine", []string{"movie"}, domain.GoodEveryday()), 201), &p)
	do(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", fmt.Sprintf(
		`{"movie":{"library_folder_id":%d,"pick_rules_id":%d,"keep_up_to_date":false,"search_on_add":true}}`, second.ID, p.ID), 200)
	a := addMovie(t, srv.URL, `{"id":"tmdb:693134"}`)
	if a.Title.LibraryFolderID != second.ID || a.Title.PickRulesID != p.ID || a.Title.KeepUpToDate || !slices.Contains(thenOf(t, st, a.JobID), library.KindSearchSlot) {
		t.Fatalf("saved defaults not used: %+v then %v", a.Title, thenOf(t, st, a.JobID))
	}
}

func TestRequestFieldsBeatDefaults(t *testing.T) {
	srv, st, _ := libSetup(t)
	first := folder(t, srv.URL, "movie")
	second := folder(t, srv.URL, "movie")
	ge := ruleByName(t, srv.URL, domain.GoodEverydayName)
	var p pickRulesDetail
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, "Mine", []string{"movie"}, domain.GoodEveryday()), 201), &p)
	do(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", fmt.Sprintf(
		`{"movie":{"library_folder_id":%d,"pick_rules_id":%d,"keep_up_to_date":false,"search_on_add":true}}`, second.ID, p.ID), 200)
	a := addMovie(t, srv.URL, fmt.Sprintf(`{"id":"tmdb:693134","library_folder_id":%d,"pick_rules_id":%d,"keep_up_to_date":true,"search_on_add":false}`, first.ID, ge.ID))
	if a.Title.LibraryFolderID != first.ID || a.Title.PickRulesID != ge.ID || !a.Title.KeepUpToDate || slices.Contains(thenOf(t, st, a.JobID), library.KindSearchSlot) {
		t.Fatalf("request lost to defaults: %+v then %v", a.Title, thenOf(t, st, a.JobID))
	}
}

func TestTitleDefaultsIgnoreAGoneFolder(t *testing.T) {
	srv, _, _ := libSetup(t)
	first := folder(t, srv.URL, "movie")
	second := folder(t, srv.URL, "movie")
	var p pickRulesDetail
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, "Mine", []string{"movie"}, domain.GoodEveryday()), 201), &p)
	do(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", fmt.Sprintf(`{"movie":{"library_folder_id":%d}}`, second.ID), 200)
	do(t, "DELETE", srv.URL+"/api/v1/library-folders/"+itoa(second.ID), "", 204)
	a := addMovie(t, srv.URL, `{"id":"tmdb:693134"}`)
	if a.Title.LibraryFolderID != first.ID {
		t.Fatalf("gone folder: %+v", a.Title)
	}
}

func TestAddShowUsesSavedShowDefaults(t *testing.T) {
	// Asked for in the request, without saved defaults.
	srv1, st1, _, _ := showSetup(t)
	var a addAnswer
	_ = json.Unmarshal(do(t, "POST", srv1.URL+"/api/v1/shows", `{"id":"tmdb:100","search_on_add":true}`, 201), &a)
	if then := thenOf(t, st1, a.JobID); !slices.Equal(then, []string{"search_show"}) {
		t.Fatalf("asked search_on_add: then %v", then)
	}
	srv, st, _, _ := showSetup(t)
	second := folder(t, srv.URL, "show")
	var p pickRulesDetail
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/pick-rules", ruleJSON(t, "Mine", []string{"show"}, domain.GoodEveryday()), 201), &p)
	do(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", fmt.Sprintf(
		`{"show":{"library_folder_id":%d,"pick_rules_id":%d,"keep_up_to_date":false,"search_on_add":true}}`, second.ID, p.ID), 200)
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/shows", `{"id":"tmdb:100"}`, 201), &a)
	if a.Title.LibraryFolderID != second.ID || a.Title.PickRulesID != p.ID || a.Title.KeepUpToDate {
		t.Fatalf("show defaults not used: %+v", a.Title)
	}
	if then := thenOf(t, st, a.JobID); !slices.Equal(then, []string{"search_show"}) {
		t.Fatalf("saved search_on_add: then %v", then)
	}
	if b := string(do(t, "GET", srv.URL+"/api/v1/settings/title-defaults", "", 200)); !strings.Contains(b, `"show":{`) {
		t.Fatalf("answer %s", b)
	}
}

func TestShowAddWithoutSearchOnAdd(t *testing.T) {
	srv, st, _, _ := showSetup(t)
	var a addAnswer
	_ = json.Unmarshal(do(t, "POST", srv.URL+"/api/v1/shows", `{"id":"tmdb:100"}`, 201), &a)
	if then := thenOf(t, st, a.JobID); len(then) != 0 {
		t.Fatalf("searched without asking: %v", then)
	}
}

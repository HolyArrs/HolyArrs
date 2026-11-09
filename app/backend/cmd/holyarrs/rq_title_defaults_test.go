package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

type rqTitleDefaults struct {
	Movie library.KindDefaults `json:"movie"`
	Show  library.KindDefaults `json:"show"`
}

// thenOf reads the chain steps of an add's first job.
func rqThenOf(t *testing.T, st *database.Store, jobID int64) []string {
	t.Helper()
	var j domain.Job
	if err := database.Get(context.Background(), st.Read(), "jobs", jobID, &j); err != nil {
		t.Fatal(err)
	}
	var out []string
	raw, _ := j.Params["then"].([]any)
	for _, s := range raw {
		out = append(out, fmt.Sprint(s))
	}
	return out
}

type rqAddAnswer struct {
	Title TitleView `json:"title"`
	JobID int64     `json:"job_id"`
}

func rqAddMovie(t *testing.T, base, body string) rqAddAnswer {
	t.Helper()
	var a rqAddAnswer
	_ = json.Unmarshal(rqDo(t, "POST", base+"/api/v1/titles", body, 201), &a)
	return a
}

func rqFolder(t *testing.T, base, kind string) library.FolderView {
	t.Helper()
	var lf library.FolderView
	_ = json.Unmarshal(rqDo(t, "POST", base+"/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":%q,"label":%q}`, t.TempDir(), kind, kind+t.Name()), 201), &lf)
	return lf
}

// route: GET /api/v1/settings/title-defaults
// route: PUT /api/v1/settings/title-defaults
func TestTitleDefaultsBuiltIn(t *testing.T) {
	t.Parallel()
	srv, _ := rqLibSetup(t)
	var d rqTitleDefaults
	_ = json.Unmarshal(rqDo(t, "GET", srv.URL+"/api/v1/settings/title-defaults", "", 200), &d)
	want := rqTitleDefaults{Movie: library.KindDefaults{KeepUpToDate: true, SearchOnAdd: true}, Show: library.KindDefaults{KeepUpToDate: true}}
	if d != want {
		t.Fatalf("built in %+v", d)
	}
}

func TestTitleDefaultsSaveAndValidate(t *testing.T) {
	t.Parallel()
	srv, _ := rqLibSetup(t)
	movies := rqFolder(t, srv.URL, "movie")
	shows := rqFolder(t, srv.URL, "show")
	music := rqRuleByName(t, srv.URL, mediafiles.LosslessFirstName)
	path := srv.URL + "/api/v1/settings/title-defaults"
	rqDo(t, "PUT", path, fmt.Sprintf(`{"movie":{"library_folder_id":%d}}`, shows.ID), 400)
	rqDo(t, "PUT", path, fmt.Sprintf(`{"show":{"library_folder_id":%d}}`, movies.ID), 400)
	rqDo(t, "PUT", path, fmt.Sprintf(`{"movie":{"pick_rules_id":%d}}`, music.ID), 400)
	rqDo(t, "PUT", path, `{"movie":{"pick_rules_id":99999}}`, 400)
	rqDo(t, "PUT", path, `{"movie":`, 400)
	var d rqTitleDefaults
	_ = json.Unmarshal(rqDo(t, "PUT", path, fmt.Sprintf(`{"movie":{"library_folder_id":%d,"search_on_add":false}}`, movies.ID), 200), &d)
	// Left-out fields keep their value.
	if d.Movie.LibraryFolderID != movies.ID || d.Movie.SearchOnAdd || !d.Movie.KeepUpToDate || !d.Show.KeepUpToDate || d.Show.SearchOnAdd {
		t.Fatalf("saved %+v", d)
	}
	_ = json.Unmarshal(rqDo(t, "GET", path, "", 200), &d)
	if d.Movie.LibraryFolderID != movies.ID || d.Movie.SearchOnAdd {
		t.Fatalf("read back %+v", d)
	}
}

func TestAddWithoutSavedDefaultsKeepsOldBehaviour(t *testing.T) {
	t.Parallel()
	srv, st := rqLibSetup(t)
	first := rqFolder(t, srv.URL, "movie")
	rqFolder(t, srv.URL, "movie")
	a := rqAddMovie(t, srv.URL, `{"id":"tmdb:693134"}`)
	if a.Title.LibraryFolderID != first.ID || !a.Title.KeepUpToDate || slices.Contains(rqThenOf(t, st, a.JobID), library.KindSearchSlot) {
		t.Fatalf("old behaviour: %+v then %v", a.Title, rqThenOf(t, st, a.JobID))
	}
}

func TestAddUsesSavedMovieDefaults(t *testing.T) {
	t.Parallel()
	srv, st := rqLibSetup(t)
	rqFolder(t, srv.URL, "movie")
	second := rqFolder(t, srv.URL, "movie")
	var p rqPickRules
	_ = json.Unmarshal(rqDo(t, "POST", srv.URL+"/api/v1/pick-rules", rqRuleJSON(t, "Mine", []string{"movie"}, mediafiles.GoodEveryday()), 201), &p)
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", fmt.Sprintf(
		`{"movie":{"library_folder_id":%d,"pick_rules_id":%d,"keep_up_to_date":false,"search_on_add":true}}`, second.ID, p.ID), 200)
	a := rqAddMovie(t, srv.URL, `{"id":"tmdb:693134"}`)
	if a.Title.LibraryFolderID != second.ID || a.Title.PickRulesID != p.ID || a.Title.KeepUpToDate || !slices.Contains(rqThenOf(t, st, a.JobID), library.KindSearchSlot) {
		t.Fatalf("saved defaults not used: %+v then %v", a.Title, rqThenOf(t, st, a.JobID))
	}
}

func TestRequestFieldsBeatDefaults(t *testing.T) {
	t.Parallel()
	srv, st := rqLibSetup(t)
	first := rqFolder(t, srv.URL, "movie")
	second := rqFolder(t, srv.URL, "movie")
	ge := rqRuleByName(t, srv.URL, mediafiles.GoodEverydayName)
	var p rqPickRules
	_ = json.Unmarshal(rqDo(t, "POST", srv.URL+"/api/v1/pick-rules", rqRuleJSON(t, "Mine", []string{"movie"}, mediafiles.GoodEveryday()), 201), &p)
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", fmt.Sprintf(
		`{"movie":{"library_folder_id":%d,"pick_rules_id":%d,"keep_up_to_date":false,"search_on_add":true}}`, second.ID, p.ID), 200)
	a := rqAddMovie(t, srv.URL, fmt.Sprintf(`{"id":"tmdb:693134","library_folder_id":%d,"pick_rules_id":%d,"keep_up_to_date":true,"search_on_add":false}`, first.ID, ge.ID))
	if a.Title.LibraryFolderID != first.ID || a.Title.PickRulesID != ge.ID || !a.Title.KeepUpToDate || slices.Contains(rqThenOf(t, st, a.JobID), library.KindSearchSlot) {
		t.Fatalf("request lost to defaults: %+v then %v", a.Title, rqThenOf(t, st, a.JobID))
	}
}

func TestTitleDefaultsIgnoreAGoneFolder(t *testing.T) {
	t.Parallel()
	srv, _ := rqLibSetup(t)
	first := rqFolder(t, srv.URL, "movie")
	second := rqFolder(t, srv.URL, "movie")
	var p rqPickRules
	_ = json.Unmarshal(rqDo(t, "POST", srv.URL+"/api/v1/pick-rules", rqRuleJSON(t, "Mine", []string{"movie"}, mediafiles.GoodEveryday()), 201), &p)
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", fmt.Sprintf(`{"movie":{"library_folder_id":%d}}`, second.ID), 200)
	rqDo(t, "DELETE", srv.URL+"/api/v1/library-folders/"+rqItoa(second.ID), "", 204)
	a := rqAddMovie(t, srv.URL, `{"id":"tmdb:693134"}`)
	if a.Title.LibraryFolderID != first.ID {
		t.Fatalf("gone folder: %+v", a.Title)
	}
}

func TestAddShowUsesSavedShowDefaults(t *testing.T) {
	t.Parallel()
	// Asked for in the request, without saved defaults.
	srv1, st1 := rqShowSetup(t)
	var a rqAddAnswer
	_ = json.Unmarshal(rqDo(t, "POST", srv1.URL+"/api/v1/shows", `{"id":"tmdb:100","search_on_add":true}`, 201), &a)
	if then := rqThenOf(t, st1, a.JobID); !slices.Equal(then, []string{"search_show"}) {
		t.Fatalf("asked search_on_add: then %v", then)
	}
	srv, st := rqShowSetup(t)
	second := rqFolder(t, srv.URL, "show")
	var p rqPickRules
	_ = json.Unmarshal(rqDo(t, "POST", srv.URL+"/api/v1/pick-rules", rqRuleJSON(t, "Mine", []string{"show"}, mediafiles.GoodEveryday()), 201), &p)
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", fmt.Sprintf(
		`{"show":{"library_folder_id":%d,"pick_rules_id":%d,"keep_up_to_date":false,"search_on_add":true}}`, second.ID, p.ID), 200)
	_ = json.Unmarshal(rqDo(t, "POST", srv.URL+"/api/v1/shows", `{"id":"tmdb:100"}`, 201), &a)
	if a.Title.LibraryFolderID != second.ID || a.Title.PickRulesID != p.ID || a.Title.KeepUpToDate {
		t.Fatalf("show defaults not used: %+v", a.Title)
	}
	if then := rqThenOf(t, st, a.JobID); !slices.Equal(then, []string{"search_show"}) {
		t.Fatalf("saved search_on_add: then %v", then)
	}
	if b := string(rqDo(t, "GET", srv.URL+"/api/v1/settings/title-defaults", "", 200)); !strings.Contains(b, `"show":{`) {
		t.Fatalf("answer %s", b)
	}
}

func TestShowAddWithoutSearchOnAdd(t *testing.T) {
	t.Parallel()
	srv, st := rqShowSetup(t)
	var a rqAddAnswer
	_ = json.Unmarshal(rqDo(t, "POST", srv.URL+"/api/v1/shows", `{"id":"tmdb:100"}`, 201), &a)
	if then := rqThenOf(t, st, a.JobID); len(then) != 0 {
		t.Fatalf("searched without asking: %v", then)
	}
}

// TestSavingShowDefaultsLeavesMovieUnsaved: saving only the show defaults
// marks no movie key saved, so a movie added without search_on_add still
// gets no search, as before anything was saved.
func TestSavingShowDefaultsLeavesMovieUnsaved(t *testing.T) {
	t.Parallel()
	srv, st := rqLibSetup(t)
	rqFolder(t, srv.URL, "movie")
	rqDo(t, "PUT", srv.URL+"/api/v1/settings/title-defaults", `{"show":{"keep_up_to_date":false}}`, 200)
	_, saved, err := library.ReadTitleDefaults(context.Background(), st.Read())
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 || !saved["show_keep_up_to_date"] {
		t.Fatalf("saved %v, want only the sent show_keep_up_to_date", saved)
	}
	a := rqAddMovie(t, srv.URL, `{"id":"tmdb:693134"}`)
	if then := rqThenOf(t, st, a.JobID); slices.Contains(then, library.KindSearchSlot) {
		t.Fatalf("a show-only save made movie adds search: then %v", then)
	}
}

type rqPickRules struct {
	ID int64 `json:"id"`
}

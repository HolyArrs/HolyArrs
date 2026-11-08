package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/movies"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func collectionAPI(t *testing.T) (*handlers, *database.Store, int64, int64) {
	t.Helper()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var collectionID, titleID int64
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		lf := mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}
		if _, err := database.Insert(tx, "library_folders", &lf, domain.System); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	titleID, err = library.AddMovie(context.Background(), s, metadata.Movie{Name: "The Fellowship of the Ring", Year: 2001, IDs: map[string]string{"tmdb": "120"}}, library.AddOptions{}, nil, time.Now().UTC(), domain.System)
	if err != nil {
		t.Fatal(err)
	}
	err = s.Write(context.Background(), func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO movie_collections(source_id, name) VALUES ('tmdb:119', 'The Lord of the Rings Collection')")
		if err != nil {
			return err
		}
		collectionID, _ = res.LastInsertId()
		_, err = tx.Exec("INSERT INTO movie_collection_titles(collection_id, title_id) VALUES (?, ?)", collectionID, titleID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	lib := &library.Service{Store: s, Metadata: metadata.Lab()}
	return buildHandlers(services{Store: s, Library: lib, Metadata: metadata.Lab()}), s, collectionID, titleID
}

func callCollection(t *testing.T, fn http.HandlerFunc, method, target, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.SetPathValue("id", id)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	fn(w, r)
	return w
}

func TestCollectionHandlersPreviewAndApply(t *testing.T) {
	t.Parallel()
	a, s, cid, titleID := collectionAPI(t)
	id := itoa(cid)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	base := srv.URL + "/api/v1/collections"
	listed := do(t, http.MethodGet, base, "", http.StatusOK)
	if !strings.Contains(string(listed), "The Lord of the Rings Collection") {
		t.Fatalf("list %s", listed)
	}
	detail := do(t, http.MethodGet, base+"/"+id, "", http.StatusOK)
	if !strings.Contains(string(detail), "The Lord of the Rings Collection") {
		t.Fatalf("detail %s", detail)
	}
	body := `{"title_ids":[` + itoa(titleID) + `],"delete_files":false,"never_add":false,"acknowledge_protection":false}`
	preview := do(t, http.MethodPost, base+"/"+id+"/remove/preview", body, http.StatusOK)
	var review movies.CollectionReview
	if err := json.Unmarshal(preview, &review); err != nil || review.ReviewToken == "" {
		t.Fatalf("review %+v, err %v", review, err)
	}
	apply := `{"title_ids":[` + itoa(titleID) + `],"review_token":"` + review.ReviewToken + `"}`
	do(t, http.MethodPost, base+"/"+id+"/remove", apply, http.StatusOK)
	var state string
	if err := s.Read().QueryRow("SELECT state FROM titles WHERE id = ?", titleID).Scan(&state); err != nil || state != "removed" {
		t.Fatalf("state %q err %v", state, err)
	}
}

func TestCollectionHandlerRejectsStaleReview(t *testing.T) {
	t.Parallel()
	a, _, cid, titleID := collectionAPI(t)
	body := `{"title_ids":[` + itoa(titleID) + `],"review_token":"old"}`
	mux := http.NewServeMux()
	(&movies.Handlers{Lib: a.s.Library, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs}).Routes(mux)
	w := callCollection(t, mux.ServeHTTP, http.MethodPost, "/api/v1/collections/"+itoa(cid)+"/remove", itoa(cid), body)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "review") {
		t.Fatalf("stale %d %s", w.Code, w.Body.String())
	}
}

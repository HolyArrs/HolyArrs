package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/movies"
)

func collectionMux(h *movies.Handlers) *http.ServeMux {
	mux := http.NewServeMux()
	h.Routes(mux)
	return mux
}

// applyBody is the remove request body for one title and a review.
func applyBody(titleID int64, review string) *strings.Reader {
	b, _ := json.Marshal(map[string]any{"title_ids": []int64{titleID}, "review_token": review})
	return strings.NewReader(string(b))
}

func serveCollection(mux *http.ServeMux, r *http.Request) *httptest.ResponseRecorder {
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestCollectionPreviewRejectsForeignTitle400(t *testing.T) {
	t.Parallel()
	a, _, cid, _ := collectionAPI(t)
	mux := collectionMux(&movies.Handlers{Lib: a.s.Library, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs})
	w := serveCollection(mux, httptest.NewRequest(http.MethodPost, "/api/v1/collections/"+itoa(cid)+"/remove/preview", strings.NewReader(`{"title_ids":[999999]}`)))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("foreign title: %d %s", w.Code, w.Body.String())
	}
}

func TestCollectionBadIDIs404(t *testing.T) {
	t.Parallel()
	a, _, _, _ := collectionAPI(t)
	mux := collectionMux(&movies.Handlers{Lib: a.s.Library, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs})
	for _, id := range []string{"abc", "0", "-3"} {
		for _, r := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "/api/v1/collections/"+id, nil),
			httptest.NewRequest(http.MethodPost, "/api/v1/collections/"+id+"/remove/preview", strings.NewReader(`{}`)),
		} {
			w := serveCollection(mux, r)
			if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "not found") {
				t.Fatalf("%s %s: %d %s", r.Method, r.URL.Path, w.Code, w.Body.String())
			}
		}
	}
}

func TestCollectionRemoveRecordsSignedInActor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ctx  func(context.Context) context.Context
		want int64
	}{
		{"session", func(c context.Context) context.Context {
			return webserver.ContextWith(c, webserver.Caller{UserID: 7, Via: "session"})
		}, 7},
		{"no session", func(c context.Context) context.Context { return c }, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, s, cid, titleID := collectionAPI(t)
			mux := collectionMux(&movies.Handlers{Lib: a.s.Library, KidMax: func(*http.Request) string { return "" }, AllowedTitleIDs: a.allowedTitleIDs})
			base := "/api/v1/collections/" + itoa(cid)
			w := serveCollection(mux, httptest.NewRequest(http.MethodPost, base+"/remove/preview", strings.NewReader(`{"title_ids":[`+itoa(titleID)+`]}`)))
			var review movies.CollectionReview
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &review) != nil {
				t.Fatalf("preview %d %s", w.Code, w.Body.String())
			}
			r := httptest.NewRequest(http.MethodPost, base+"/remove", applyBody(titleID, review.ReviewToken))
			r = r.WithContext(tc.ctx(r.Context()))
			if w := serveCollection(mux, r); w.Code != 200 {
				t.Fatalf("apply %d %s", w.Code, w.Body.String())
			}
			var raw string
			if err := s.Read().QueryRow("SELECT updated_by FROM titles WHERE id = ?", titleID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var by struct {
				Kind string `json:"kind"`
				ID   int64  `json:"id"`
			}
			if err := json.Unmarshal([]byte(raw), &by); err != nil || by.Kind != "user" || by.ID != tc.want {
				t.Fatalf("updated_by %s, err %v, want user %d", raw, err, tc.want)
			}
		})
	}
}

func TestKidCollectionsFailClosedOnFilterError(t *testing.T) {
	t.Parallel()
	a, _, cid, _ := collectionAPI(t)
	mux := collectionMux(&movies.Handlers{
		Lib:    a.s.Library,
		KidMax: func(*http.Request) string { return "PG" },
		AllowedTitleIDs: func(context.Context, string, []int64) (map[int64]bool, error) {
			return nil, errors.New("boom")
		},
	})
	for _, p := range []string{"/api/v1/collections", "/api/v1/collections/" + itoa(cid)} {
		w := serveCollection(mux, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "Fellowship") {
			t.Fatalf("%s: %d %s", p, w.Code, w.Body.String())
		}
	}
}

func TestKidCollectionListTitleIDsFiltered(t *testing.T) {
	t.Parallel()
	k := newKidRig(t)
	code, body := k.kai.get("/api/v1/collections")
	if code != 200 {
		t.Fatalf("kid collections: %d %s", code, body)
	}
	var out struct {
		Collections []struct {
			Name     string  `json:"name"`
			TitleIDs []int64 `json:"title_ids"`
		} `json:"collections"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Collections) != 1 || out.Collections[0].Name != "Shelf One" {
		t.Fatalf("collections %+v", out.Collections)
	}
	ids := out.Collections[0].TitleIDs
	if len(ids) != 1 || ids[0] != k.ids["Gentle Meadow"] {
		t.Fatalf("title_ids %v, want only %d (9102 hidden)", ids, k.ids["Gentle Meadow"])
	}
}

func TestCollectionStaleReviewReturnsFreshToken(t *testing.T) {
	t.Parallel()
	a, s, cid, titleID := collectionAPI(t)
	mux := collectionMux(&movies.Handlers{Lib: a.s.Library, KidMax: a.kidMax, AllowedTitleIDs: a.allowedTitleIDs})
	target := "/api/v1/collections/" + itoa(cid) + "/remove"
	w := serveCollection(mux, httptest.NewRequest(http.MethodPost, target, applyBody(titleID, "old")))
	if w.Code != http.StatusConflict {
		t.Fatalf("stale %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Review movies.CollectionReview `json:"review"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || out.Review.ReviewToken == "" || out.Review.ReviewToken == "old" {
		t.Fatalf("review in 409: %+v err %v", out.Review, err)
	}
	w = serveCollection(mux, httptest.NewRequest(http.MethodPost, target, applyBody(titleID, out.Review.ReviewToken)))
	if w.Code != http.StatusOK {
		t.Fatalf("retry %d %s", w.Code, w.Body.String())
	}
	var state string
	if err := s.Read().QueryRow("SELECT state FROM titles WHERE id = ?", titleID).Scan(&state); err != nil || state != "removed" {
		t.Fatalf("state %q err %v", state, err)
	}
}

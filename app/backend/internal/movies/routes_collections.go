package movies

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// Handlers serves the movie collection routes.
type Handlers struct {
	Lib *library.Service
	// KidMax is the age limit of the signed-in Kid, or "" for everyone else.
	KidMax func(r *http.Request) string
	// AllowedTitleIDs says which of ids a Kid with the age limit max may see.
	AllowedTitleIDs func(ctx context.Context, max string, ids []int64) (map[int64]bool, error)
}

// Routes adds the movie collection routes.
func (h *Handlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/collections", h.listCollections)
	m.HandleFunc("GET /api/v1/collections/{id}", h.getCollection)
	m.HandleFunc("POST /api/v1/collections/{id}/remove/preview", h.previewCollectionRemoval)
	m.HandleFunc("POST /api/v1/collections/{id}/remove", h.removeCollection)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

type collectionRemoveBody struct {
	library.RemoveChoice
	TitleIDs    []int64 `json:"title_ids"`
	ReviewToken string  `json:"review_token,omitempty"`
}

func (h *Handlers) listCollections(w http.ResponseWriter, r *http.Request) {
	cs, err := ListCollections(r.Context(), h.Lib.Store.Read())
	if err != nil {
		webserver.Internal(w, "movies", err)
		return
	}
	if max := h.KidMax(r); max != "" {
		// A Kid sees a collection only for the titles their limit allows,
		// and counts only those.
		kept := []MovieCollection{}
		for _, c := range cs {
			d, err := h.kidCollection(r, max, c.ID)
			if err != nil {
				webserver.Internal(w, "movies", err)
				return
			}
			if len(d.Titles) > 0 {
				c.OwnedTitles = len(d.Titles)
				c.TitleIDs = c.TitleIDs[:0]
				for _, t := range d.Titles {
					c.TitleIDs = append(c.TitleIDs, t.ID)
				}
				kept = append(kept, c)
			}
		}
		cs = kept
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"collections": cs})
}

// kidCollection is the collection with only the titles max allows, and
// counts only those.
func (h *Handlers) kidCollection(r *http.Request, max string, id int64) (CollectionDetail, error) {
	c, err := CollectionDetails(r.Context(), h.Lib.Store.Read(), id)
	if err != nil || max == "" {
		return c, err
	}
	ids := make([]int64, len(c.Titles))
	for i, t := range c.Titles {
		ids[i] = t.ID
	}
	ok, err := h.AllowedTitleIDs(r.Context(), max, ids)
	if err != nil {
		return c, err
	}
	kept := []CollectionTitle{}
	for _, t := range c.Titles {
		if ok[t.ID] {
			kept = append(kept, t)
		}
	}
	c.Titles = kept
	c.OwnedTitles = len(kept)
	return c, nil
}

func (h *Handlers) getCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	max := h.KidMax(r)
	c, err := h.kidCollection(r, max, id)
	if err == nil && max != "" && len(c.Titles) == 0 {
		err = ErrCollectionNotFound // nothing in it a Kid may see
	}
	if err != nil {
		collectionError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, c)
}

func (h *Handlers) previewCollectionRemoval(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body collectionRemoveBody
	if !webserver.Decode(w, r, &body, `send {"title_ids": [<owned title id>, ...], "delete_files": false, "never_add": false, "acknowledge_protection": false}`) {
		return
	}
	p, err := New(h.Lib).PreviewCollectionRemoval(r.Context(), id, body.TitleIDs, body.RemoveChoice)
	if err != nil {
		collectionError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, p)
}

func (h *Handlers) removeCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body collectionRemoveBody
	if !webserver.Decode(w, r, &body, `send the unchanged review_token, title_ids and choices returned by the preview`) {
		return
	}
	p, err := New(h.Lib).RemoveCollection(r.Context(), id, body.TitleIDs, body.RemoveChoice, body.ReviewToken, actor(r))
	if err != nil {
		if errors.Is(err, ErrCollectionReviewChanged) {
			webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "review": p})
			return
		}
		collectionError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, p)
}

func collectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCollectionNotFound), errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "movie collection not found")
	case errors.Is(err, ErrCollectionSelection):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	default:
		webserver.Internal(w, "movies", err)
	}
}

// actor is the signed-in person, or the owner when no session is attached.
func actor(r *http.Request) domain.Actor {
	if _, ok := webserver.From(r.Context()); ok {
		return webserver.Actor(r.Context())
	}
	return domain.Actor{Kind: "user", ID: 1}
}

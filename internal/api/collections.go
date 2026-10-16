package api

import (
	"errors"
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/store"
)

type collectionRemoveBody struct {
	library.RemoveChoice
	TitleIDs    []int64 `json:"title_ids"`
	ReviewToken string  `json:"review_token,omitempty"`
}

func (a *API) listCollections(w http.ResponseWriter, r *http.Request) {
	cs, err := library.ListCollections(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"collections": cs})
}

func (a *API) getCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := library.CollectionDetails(r.Context(), a.s.Store.Read(), id)
	if err != nil {
		collectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (a *API) previewCollectionRemoval(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body collectionRemoveBody
	if !decode(w, r, &body, `send {"title_ids": [<owned title id>, ...], "delete_files": false, "never_add": false, "acknowledge_protection": false}`) {
		return
	}
	p, err := a.s.Library.PreviewCollectionRemoval(r.Context(), id, body.TitleIDs, body.RemoveChoice)
	if err != nil {
		collectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) removeCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body collectionRemoveBody
	if !decode(w, r, &body, `send the unchanged review_token, title_ids and choices returned by the preview`) {
		return
	}
	p, err := a.s.Library.RemoveCollection(r.Context(), id, body.TitleIDs, body.RemoveChoice, body.ReviewToken, actor(r))
	if err != nil {
		if errors.Is(err, library.ErrCollectionReviewChanged) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "review": p})
			return
		}
		collectionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func collectionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, library.ErrCollectionNotFound), errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, "movie collection not found")
	case errors.Is(err, library.ErrCollectionSelection):
		fail(w, http.StatusBadRequest, err.Error())
	default:
		internal(w, err)
	}
}

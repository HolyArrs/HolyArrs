package api

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
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
	if max := a.kidMax(r); max != "" {
		// A Kid sees a collection only for the titles their limit allows,
		// and counts only those.
		kept := []library.MovieCollection{}
		for _, c := range cs {
			d, err := a.kidCollection(r, max, c.ID)
			if err != nil {
				internal(w, err)
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
	writeJSON(w, http.StatusOK, map[string]any{"collections": cs})
}

// kidCollection is the collection with only the titles max allows, and
// counts only those.
func (a *API) kidCollection(r *http.Request, max string, id int64) (library.CollectionDetail, error) {
	c, err := library.CollectionDetails(r.Context(), a.s.Store.Read(), id)
	if err != nil || max == "" {
		return c, err
	}
	ids := make([]int64, len(c.Titles))
	for i, t := range c.Titles {
		ids[i] = t.ID
	}
	ok, err := a.allowedTitleIDs(r.Context(), max, ids)
	if err != nil {
		return c, err
	}
	kept := []library.CollectionTitle{}
	for _, t := range c.Titles {
		if ok[t.ID] {
			kept = append(kept, t)
		}
	}
	c.Titles = kept
	c.OwnedTitles = len(kept)
	return c, nil
}

func (a *API) getCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	max := a.kidMax(r)
	c, err := a.kidCollection(r, max, id)
	if err == nil && max != "" && len(c.Titles) == 0 {
		err = library.ErrCollectionNotFound // nothing in it a Kid may see
	}
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

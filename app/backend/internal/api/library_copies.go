package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

type libraryCopySearchIn struct {
	Selections  []mediafiles.LibraryCopySelection `json:"selections"`
	ReviewToken string                            `json:"review_token,omitempty"`
}

type libraryCopySearchPreview struct {
	Copies      []mediafiles.LibraryCopy `json:"copies"`
	SlotCount   int                      `json:"slot_count"`
	ReviewToken string                   `json:"review_token"`
}

type libraryCopySearchItem struct {
	FileID    int64   `json:"file_id"`
	VersionID int64   `json:"version_id"`
	SlotIDs   []int64 `json:"slot_ids"`
	JobIDs    []int64 `json:"job_ids"`
}

type libraryCopySearchResult struct {
	Items  []libraryCopySearchItem `json:"items"`
	JobIDs []int64                 `json:"job_ids"`
}

func (a *API) listLibraryCopies(w http.ResponseWriter, r *http.Request) {
	limit, err := libraryCopyInt(r, "limit", 50)
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := libraryCopyInt(r, "offset", 0)
	if err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := mediafiles.LibraryCopies(r.Context(), a.s.Store, mediafiles.LibraryCopyFilter{Query: r.URL.Query().Get("q"), Codec: r.URL.Query().Get("codec"), Sort: r.URL.Query().Get("sort"), Limit: limit, Offset: offset})
	if errors.Is(err, mediafiles.ErrLibraryCopyFilter) || errors.Is(err, mediafiles.ErrTitleSearchQuery) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, page)
}

func libraryCopyInt(r *http.Request, name string, def int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number", name)
	}
	return n, nil
}

func (a *API) previewLibraryCopySearch(w http.ResponseWriter, r *http.Request) {
	var in libraryCopySearchIn
	if !decode(w, r, &in, `body is {"selections":[{"file_id":1,"version_id":2}]}`) {
		return
	}
	preview, err := a.libraryCopyPreview(r, in.Selections)
	if errors.Is(err, mediafiles.ErrLibraryCopyFilter) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, mediafiles.ErrLibraryCopyStale) || errors.Is(err, errProtectedLibraryCopy) {
		webserver.Fail(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, preview)
}

var errProtectedLibraryCopy = errors.New("a selected Library copy is protected")

func (a *API) libraryCopyPreview(r *http.Request, selections []mediafiles.LibraryCopySelection) (libraryCopySearchPreview, error) {
	copies, err := mediafiles.SelectedLibraryCopies(r.Context(), a.s.Store, selections)
	if err != nil {
		return libraryCopySearchPreview{}, err
	}
	count := 0
	for _, copy := range copies {
		if copy.Protected {
			return libraryCopySearchPreview{}, errProtectedLibraryCopy
		}
		slots, err := mediafiles.LibraryCopySlots(r.Context(), a.s.Store, mediafiles.LibraryCopySelection{FileID: copy.FileID, VersionID: copy.VersionID})
		if err != nil {
			return libraryCopySearchPreview{}, err
		}
		count += len(slots)
	}
	return libraryCopySearchPreview{Copies: copies, SlotCount: count, ReviewToken: libraryCopyReviewToken(copies)}, nil
}

func libraryCopyReviewToken(copies []mediafiles.LibraryCopy) string {
	h := sha256.New()
	for _, c := range copies {
		_, _ = fmt.Fprintf(h, "%d\x00%d\x00%d\x00%d\x00%d\x00%s\x00%s\x00%s\x00%t\n", c.FileID, c.VersionID, c.TitleID, c.PickRulesID, c.AudioTracks, c.FileUpdatedAt.UTC().Format(database.TimeLayout), c.VersionUpdatedAt.UTC().Format(database.TimeLayout), c.Codec, c.Protected)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (a *API) searchLibraryCopies(w http.ResponseWriter, r *http.Request) {
	var in libraryCopySearchIn
	if !decode(w, r, &in, `body is {"selections":[{"file_id":1,"version_id":2}],"review_token":"..."}`) {
		return
	}
	if len(in.ReviewToken) != sha256.Size*2 {
		webserver.Fail(w, http.StatusBadRequest, "review_token is required")
		return
	}
	preview, err := a.libraryCopyPreview(r, in.Selections)
	if errors.Is(err, mediafiles.ErrLibraryCopyFilter) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, mediafiles.ErrLibraryCopyStale) || errors.Is(err, errProtectedLibraryCopy) {
		webserver.Fail(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(in.ReviewToken)), []byte(preview.ReviewToken)) != 1 {
		webserver.Fail(w, http.StatusConflict, "the reviewed Library copies changed; review them again")
		return
	}
	if a.s.Queue == nil {
		webserver.Internal(w, "api", errors.New("Library copy search queue is unavailable"))
		return
	}
	result := libraryCopySearchResult{Items: make([]libraryCopySearchItem, 0, len(preview.Copies)), JobIDs: []int64{}}
	for _, copy := range preview.Copies {
		sel := mediafiles.LibraryCopySelection{FileID: copy.FileID, VersionID: copy.VersionID}
		slots, err := mediafiles.LibraryCopySlots(r.Context(), a.s.Store, sel)
		if err != nil {
			webserver.Fail(w, http.StatusConflict, mediafiles.ErrLibraryCopyStale.Error())
			return
		}
		item := libraryCopySearchItem{FileID: copy.FileID, VersionID: copy.VersionID, SlotIDs: slots, JobIDs: []int64{}}
		for _, slotID := range slots {
			jobID, _, err := a.s.Queue.Enqueue(r.Context(), jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: slotID}, map[string]any{"by_hand": true, "choose": false}, "show_slot:"+itoa(slotID), actor(r))
			if err != nil {
				webserver.Internal(w, "api", err)
				return
			}
			item.JobIDs = append(item.JobIDs, jobID)
			result.JobIDs = append(result.JobIDs, jobID)
		}
		result.Items = append(result.Items, item)
	}
	webserver.WriteJSON(w, http.StatusAccepted, result)
}

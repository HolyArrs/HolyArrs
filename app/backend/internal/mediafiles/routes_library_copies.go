package mediafiles

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

type libraryCopySearchIn struct {
	Selections  []LibraryCopySelection `json:"selections"`
	ReviewToken string                 `json:"review_token,omitempty"`
}

// LibraryCopySearchPreview is the reviewed copies a copy search would cover.
type LibraryCopySearchPreview struct {
	Copies      []LibraryCopy `json:"copies"`
	SlotCount   int           `json:"slot_count"`
	ReviewToken string        `json:"review_token"`
}

// LibraryCopySearchItem is one copy's queued slot searches.
type LibraryCopySearchItem struct {
	FileID    int64   `json:"file_id"`
	VersionID int64   `json:"version_id"`
	SlotIDs   []int64 `json:"slot_ids"`
	JobIDs    []int64 `json:"job_ids"`
}

// LibraryCopySearchResult is the slots and jobs a copy search queued.
type LibraryCopySearchResult struct {
	Items  []LibraryCopySearchItem `json:"items"`
	JobIDs []int64                 `json:"job_ids"`
}

func (h *Handlers) listLibraryCopies(w http.ResponseWriter, r *http.Request) {
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
	page, err := LibraryCopies(r.Context(), h.Store, LibraryCopyFilter{Query: r.URL.Query().Get("q"), Codec: r.URL.Query().Get("codec"), Sort: r.URL.Query().Get("sort"), Limit: limit, Offset: offset})
	if errors.Is(err, ErrLibraryCopyFilter) || errors.Is(err, ErrTitleSearchQuery) {
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

func (h *Handlers) previewLibraryCopySearch(w http.ResponseWriter, r *http.Request) {
	var in libraryCopySearchIn
	if !webserver.Decode(w, r, &in, `body is {"selections":[{"file_id":1,"version_id":2}]}`) {
		return
	}
	preview, err := h.libraryCopyPreview(r, in.Selections)
	if errors.Is(err, ErrLibraryCopyFilter) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, ErrLibraryCopyStale) || errors.Is(err, errProtectedLibraryCopy) {
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

func (h *Handlers) libraryCopyPreview(r *http.Request, selections []LibraryCopySelection) (LibraryCopySearchPreview, error) {
	copies, err := SelectedLibraryCopies(r.Context(), h.Store, selections)
	if err != nil {
		return LibraryCopySearchPreview{}, err
	}
	count := 0
	for _, copy := range copies {
		if copy.Protected {
			return LibraryCopySearchPreview{}, errProtectedLibraryCopy
		}
		slots, err := LibraryCopySlots(r.Context(), h.Store, LibraryCopySelection{FileID: copy.FileID, VersionID: copy.VersionID})
		if err != nil {
			return LibraryCopySearchPreview{}, err
		}
		count += len(slots)
	}
	return LibraryCopySearchPreview{Copies: copies, SlotCount: count, ReviewToken: libraryCopyReviewToken(copies)}, nil
}

func libraryCopyReviewToken(copies []LibraryCopy) string {
	h := sha256.New()
	for _, c := range copies {
		_, _ = fmt.Fprintf(h, "%d\x00%d\x00%d\x00%d\x00%d\x00%s\x00%s\x00%s\x00%t\n", c.FileID, c.VersionID, c.TitleID, c.PickRulesID, c.AudioTracks, c.FileUpdatedAt.UTC().Format(database.TimeLayout), c.VersionUpdatedAt.UTC().Format(database.TimeLayout), c.Codec, c.Protected)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (h *Handlers) searchLibraryCopies(w http.ResponseWriter, r *http.Request) {
	var in libraryCopySearchIn
	if !webserver.Decode(w, r, &in, `body is {"selections":[{"file_id":1,"version_id":2}],"review_token":"..."}`) {
		return
	}
	if len(in.ReviewToken) != sha256.Size*2 {
		webserver.Fail(w, http.StatusBadRequest, "review_token is required")
		return
	}
	preview, err := h.libraryCopyPreview(r, in.Selections)
	if errors.Is(err, ErrLibraryCopyFilter) {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, ErrLibraryCopyStale) || errors.Is(err, errProtectedLibraryCopy) {
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
	if h.Queue == nil {
		webserver.Internal(w, "api", errors.New("Library copy search queue is unavailable"))
		return
	}
	result := LibraryCopySearchResult{Items: make([]LibraryCopySearchItem, 0, len(preview.Copies)), JobIDs: []int64{}}
	for _, copy := range preview.Copies {
		sel := LibraryCopySelection{FileID: copy.FileID, VersionID: copy.VersionID}
		slots, err := LibraryCopySlots(r.Context(), h.Store, sel)
		if err != nil {
			webserver.Fail(w, http.StatusConflict, ErrLibraryCopyStale.Error())
			return
		}
		item := LibraryCopySearchItem{FileID: copy.FileID, VersionID: copy.VersionID, SlotIDs: slots, JobIDs: []int64{}}
		for _, slotID := range slots {
			jobID, _, err := h.Queue.Enqueue(r.Context(), jobs.KindSearchSlot, &database.Subject{Type: "slot", ID: slotID}, map[string]any{"by_hand": true, "choose": false}, "show_slot:"+strconv.FormatInt(slotID, 10), webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
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

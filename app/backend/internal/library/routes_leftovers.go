package library

import (
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// listLeftovers answers a title's leftover files: videos in its folder that
// fill no listed episode.
func (h *OrganiseHandlers) listLeftovers(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	ls, err := h.Lib.Leftovers(r.Context(), t.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"leftovers": ls})
}

func (h *OrganiseHandlers) matchLeftovers(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	var body struct {
		Items   []LeftoverMatch `json:"items"`
		Replace bool            `json:"replace"`
	}
	if !webserver.Decode(w, r, &body, `send {"items": [{"file_id": <id>, "episode_ids": [<id>, ...]}], "replace": false}; a movie sends no episode_ids`) {
		return
	}
	if len(body.Items) == 0 || len(body.Items) > 2000 {
		webserver.Fail(w, http.StatusBadRequest, "send 1 to 2000 files")
		return
	}
	out, err := h.Lib.MatchLeftovers(r.Context(), t.ID, body.Items, body.Replace, h.actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"results": out})
}

func (h *OrganiseHandlers) unmatchLeftovers(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	var body struct {
		FileIDs []int64 `json:"file_ids"`
	}
	if !webserver.Decode(w, r, &body, `send {"file_ids": [<id>, ...]}`) {
		return
	}
	if len(body.FileIDs) == 0 || len(body.FileIDs) > 2000 {
		webserver.Fail(w, http.StatusBadRequest, "send 1 to 2000 files")
		return
	}
	n, err := h.Lib.UnmatchLeftovers(r.Context(), t.ID, body.FileIDs, h.actor(r))
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"unmatched": n})
}

func (h *OrganiseHandlers) dismissLeftover(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	fid, err := strconv.ParseInt(r.PathValue("fid"), 10, 64)
	if err != nil || fid <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	var body struct {
		Dismissed bool `json:"dismissed"`
	}
	if !webserver.Decode(w, r, &body, `send {"dismissed": true} to leave the file alone, false to list it again`) {
		return
	}
	if err := h.Lib.DismissLeftover(r.Context(), t.ID, fid, body.Dismissed); err != nil {
		webserver.Fail(w, http.StatusNotFound, "no such file in this title")
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"file_id": fid, "dismissed": body.Dismissed})
}

// rescanTitle queues one Job that refreshes the title's episode list and
// looks at its folder again; GET /api/v1/jobs/{id} gives the counts.
func (h *OrganiseHandlers) rescanTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	id, _, err := h.Queue.Enqueue(r.Context(), KindRescanTitle, &domain.Subject{Type: "title", ID: t.ID}, nil,
		KindRescanTitle+":"+strconv.FormatInt(t.ID, 10), h.actor(r))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (h *OrganiseHandlers) actor(r *http.Request) domain.Actor {
	return webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1})
}

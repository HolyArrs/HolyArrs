package sources

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// CheckIn is the body of POST /releases/check: the movie, as the title page
// shows it, and the Pick rules to judge its releases with.
type CheckIn struct {
	TMDb             int64  `json:"tmdb"`
	IMDb             string `json:"imdb"`
	Name             string `json:"name"`
	OriginalName     string `json:"original_name"`
	OriginalLanguage string `json:"original_language"`
	Year             int    `json:"year"`
	PickRulesID      int64  `json:"pick_rules_id"`
}

// startCheck queues a release check for a movie not in the Library yet.
// Checks for the same movie and Pick rules share one Job while it runs.
func (h *Handlers) startCheck(w http.ResponseWriter, r *http.Request) {
	var in CheckIn
	if !webserver.Decode(w, r, &in, `body is {"tmdb": int, "name": text, "pick_rules_id": int}, with "imdb", "original_name", "original_language" and "year" optional`) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.TMDb <= 0 || in.Name == "" || len(in.Name) > 300 || len(in.OriginalName) > 300 || len(in.IMDb) > 20 || len(in.OriginalLanguage) > 10 || in.Year < 0 || in.Year > 3000 {
		webserver.Fail(w, http.StatusBadRequest, "the movie's name and TMDB id are needed")
		return
	}
	var pr mediafiles.PickRules
	if in.PickRulesID <= 0 || mediafiles.LoadPickRules(r.Context(), h.Store.Read(), in.PickRulesID, &pr) != nil {
		webserver.Fail(w, http.StatusBadRequest, "no such Pick rules")
		return
	}
	if ok, err := handSearchable(r, h.Store); err != nil {
		webserver.Internal(w, "sources", err)
		return
	} else if !ok {
		webserver.Fail(w, http.StatusConflict, "No Source to search. Add one or turn one on in Settings > Sources.")
		return
	}
	params := map[string]any{"by_hand": true, "tmdb": in.TMDb, "imdb": in.IMDb, "name": in.Name, "original_name": in.OriginalName,
		"original_language": in.OriginalLanguage, "year": in.Year, "pick_rules_id": in.PickRulesID}
	key := "release_check:" + strconv.FormatInt(in.TMDb, 10) + ":" + strconv.FormatInt(in.PickRulesID, 10)
	id, _, err := h.Queue.Enqueue(r.Context(), jobs.KindReleaseCheck, nil, params, key, actorOf(r))
	if err != nil {
		webserver.Internal(w, "sources", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_ids": []int64{id}})
}

// getCheck answers like the title's release list: the releases a check
// found, the Job while it runs, and what came of a check that found none.
func (h *Handlers) getCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var j domain.Job
	if err := database.Get(r.Context(), h.Store.Read(), "jobs", id, &j); err != nil || j.Kind != jobs.KindReleaseCheck {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return
	}
	views, _ := h.search().CheckResult(id)
	if views == nil {
		views = []ReleaseView{}
	}
	body := map[string]any{"releases": views, "total": len(views), "limit": len(views), "offset": 0, "running_job_ids": []int64{}}
	if j.State == "queued" || j.State == "running" {
		body["running_job_ids"] = []int64{id}
	} else if len(views) == 0 {
		ls := &LastSearch{At: j.EndedAt, Failed: j.State == "failed"}
		if n, ok := j.Result["dropped"].(float64); ok {
			ls.Dropped = int(n)
		}
		if xs, ok := j.Result["dropped_sources"].([]any); ok {
			for _, x := range xs {
				if s, ok := x.(string); ok {
					ls.DroppedBy = append(ls.DroppedBy, s)
				}
			}
		}
		body["last_search"] = ls
	}
	webserver.WriteJSON(w, http.StatusOK, body)
}

// pickCheck sends a checked release for the movie once it is added. Body:
// {"release_id": int, "title_id": int, "force": bool}.
func (h *Handlers) pickCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		ReleaseID int64 `json:"release_id"`
		TitleID   int64 `json:"title_id"`
		Force     bool  `json:"force"`
	}
	if !webserver.Decode(w, r, &in, `body is {"release_id": int, "title_id": int, "force": bool}`) {
		return
	}
	ch, err := h.search().CheckPick(r.Context(), id, in.ReleaseID, in.TitleID, in.Force, actorOf(r))
	if errors.Is(err, ErrCheckGone) {
		webserver.Fail(w, http.StatusGone, err.Error())
		return
	}
	if err != nil {
		SearchError(w, err)
		return
	}
	h.search().waitSend(r.Context(), &ch)
	webserver.WriteJSON(w, http.StatusOK, ch)
}

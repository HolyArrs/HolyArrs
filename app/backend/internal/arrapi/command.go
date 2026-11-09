package arrapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// doneID is the id of a command that had nothing to queue. The main case
// is RefreshMonitoredDownloads. Seerr sends it every minute, and the
// background reader already keeps downloads fresh, so it is answered at
// once with no Job row and no disk write while idle. Polling doneID
// always says completed. It sits far above any Job id HolyArrs will reach.
const doneID = 2147483647

// commandIn is a command: its name, plus its own fields at the top level.
type commandIn struct {
	Name         string  `json:"name"`
	SeriesID     int64   `json:"seriesId"`
	SeasonNumber *int    `json:"seasonNumber"`
	EpisodeIDs   []int64 `json:"episodeIds"`
	MovieIDs     []int64 `json:"movieIds"`
	MovieID      int64   `json:"movieId"`
}

// commandNames maps the lower-case names each side answers to their
// canonical spelling.
var commandNames = map[string]map[string]string{
	"sonarr": {"seriessearch": "SeriesSearch", "seasonsearch": "SeasonSearch", "episodesearch": "EpisodeSearch",
		"missingepisodesearch": "MissingEpisodeSearch", "refreshmonitoreddownloads": "RefreshMonitoredDownloads",
		"refreshseries": "RefreshSeries", "rescanseries": "RescanSeries"},
	"radarr": {"moviessearch": "MoviesSearch", "refreshmonitoreddownloads": "RefreshMonitoredDownloads",
		"refreshmovie": "RefreshMovie", "rescanmovie": "RescanMovie"},
}

// command queues the Jobs a command stands for and answers with its
// record. Searches run as hand searches, because the old apps treat API
// commands that way. They look even for titles switched off or not out
// yet.
func (h *handler) command(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, bodyMax))
	var in commandIn
	if err == nil {
		err = json.Unmarshal(raw, &in)
	}
	if err != nil {
		message(w, http.StatusBadRequest, "the body is not valid JSON for this route")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		refuse(w, failure{PropertyName: "Name", ErrorMessage: "send the command's name", AttemptedValue: in.Name})
		return
	}
	name, ok := commandNames[h.side][strings.ToLower(strings.TrimSpace(strings.TrimSuffix(in.Name, "Command")))]
	if !ok {
		// The old apps fail on an unknown name instead of a clean 400. No
		// client depends on that, so a 400 is safe.
		message(w, http.StatusInternalServerError, "Unknown command: "+in.Name+". This server answers "+strings.Join(names(h.side), ", "))
		return
	}
	ids, err := h.runCommand(r.Context(), name, in, actor(r))
	if errors.Is(err, database.ErrNotFound) {
		notFound(w)
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	body := map[string]any{}
	_ = json.Unmarshal(raw, &body)
	body["name"] = name
	rec := h.commandRecord(r.Context(), ids, name, body)
	w.Header().Set("Location", "/"+h.side+"/api/v3/command/"+itoa(rec.ID))
	writeJSON(w, http.StatusCreated, rec)
}

func names(side string) []string {
	var out []string
	for _, n := range commandNames[side] {
		out = append(out, n)
	}
	return out
}

// runCommand queues the Jobs and returns their ids. No ids means nothing
// was left to do.
func (h *handler) runCommand(ctx context.Context, name string, in commandIn, by domain.Actor) ([]int64, error) {
	var ids []int64
	enqueue := func(kind string, subject *domain.Subject, params map[string]any, key string) error {
		id, _, err := h.Queue.Enqueue(ctx, kind, subject, params, key, by)
		if err == nil {
			ids = append(ids, id)
		}
		return err
	}
	// searchSlots hand-searches each Slot of the items, one Job per Slot.
	searchSlots := func(slots []mediafiles.Slot, err error) error {
		if err != nil {
			return err
		}
		for _, s := range slots {
			if err := enqueue(jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: s.ID}, map[string]any{"by_hand": true}, "hand_search_slot:"+itoa(s.ID)); err != nil {
				return err
			}
		}
		return nil
	}
	searchSeason := func(partID, versionID int64, keptOnly bool) error {
		params := map[string]any{"by_hand": true, "version_id": versionID}
		if keptOnly {
			params["kept_only"] = true
		}
		return enqueue(jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: partID}, params, fmt.Sprintf("hand_search_season:%d:%d", partID, versionID))
	}
	switch name {
	case "RefreshMonitoredDownloads":
		return nil, nil
	case "SeriesSearch", "MissingEpisodeSearch":
		if in.SeriesID == 0 && name == "MissingEpisodeSearch" {
			// The whole missing list, the same way the daily missing search
			// runs.
			return ids, enqueue(jobs.KindSearchMissing, nil, map[string]any{"which": "missing"}, "search_missing:missing")
		}
		if _, err := h.title(ctx, in.SeriesID, "show"); err != nil {
			return nil, err
		}
		// Every season with a kept episode that still needs a copy.
		list, err := shows.ShowSeasonsToSearch(ctx, h.Store.Read(), in.SeriesID)
		if err != nil {
			return nil, err
		}
		for _, x := range list {
			if err := searchSeason(x.PartID, x.VersionID, true); err != nil {
				return nil, err
			}
		}
	case "SeasonSearch":
		if _, err := h.title(ctx, in.SeriesID, "show"); err != nil {
			return nil, err
		}
		if in.SeasonNumber == nil {
			return nil, database.ErrNotFound
		}
		partID, err := library.SeasonPartID(ctx, h.Store.Read(), in.SeriesID, *in.SeasonNumber)
		if err != nil {
			return nil, database.ErrNotFound
		}
		vs, err := database.Select[mediafiles.Version](ctx, h.Store.Read(), "versions", "title_id = ? ORDER BY is_main DESC, id", in.SeriesID)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			if err := searchSeason(partID, v.ID, false); err != nil {
				return nil, err
			}
		}
	case "EpisodeSearch":
		if len(in.EpisodeIDs) == 0 {
			return nil, nil
		}
		return ids, searchSlots(library.EpisodeSearchSlots(ctx, h.Store.Read(), in.EpisodeIDs))
	case "MoviesSearch":
		if len(in.MovieIDs) == 0 {
			return nil, nil
		}
		return ids, searchSlots(library.MovieSearchSlots(ctx, h.Store.Read(), in.MovieIDs))
	case "RefreshSeries", "RefreshMovie", "RescanSeries", "RescanMovie":
		kind, word := "show", library.KindRefreshTitle
		if strings.HasSuffix(name, "Movie") {
			kind = "movie"
		}
		if strings.HasPrefix(name, "Rescan") {
			word = library.KindScanTitle
		}
		targets := in.MovieIDs
		for _, id := range []int64{in.SeriesID, in.MovieID} {
			if id != 0 {
				targets = append(targets, id)
			}
		}
		for _, id := range targets {
			if _, err := h.title(ctx, id, kind); err != nil {
				return nil, err
			}
			params := map[string]any{"by_hand": true}
			if word == library.KindScanTitle {
				params = nil
			}
			if err := enqueue(word, &domain.Subject{Type: "title", ID: id}, params, word+":"+itoa(id)); err != nil {
				return nil, err
			}
		}
	}
	return ids, nil
}

// commandRecord is a command as the old apps answer it.
type commandRecord struct {
	ID                  int64          `json:"id"`
	Name                string         `json:"name"`
	CommandName         string         `json:"commandName"`
	Message             string         `json:"message,omitempty"`
	Body                map[string]any `json:"body"`
	Priority            string         `json:"priority"`
	Status              string         `json:"status"`
	Result              string         `json:"result"`
	Queued              utc            `json:"queued"`
	Started             *utc           `json:"started,omitempty"`
	Ended               *utc           `json:"ended,omitempty"`
	Duration            string         `json:"duration,omitempty"`
	Exception           string         `json:"exception,omitempty"`
	Trigger             string         `json:"trigger"`
	StateChangeTime     *utc           `json:"stateChangeTime,omitempty"`
	SendUpdatesToClient bool           `json:"sendUpdatesToClient"`
	UpdateScheduledTask bool           `json:"updateScheduledTask"`
}

// words splits a command name into words, so SeriesSearch becomes
// "Series Search".
func words(name string) string {
	var b strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// commandRecord answers for the first Job of a command, whose state
// stands for the whole command, or for doneID when nothing was queued.
func (h *handler) commandRecord(ctx context.Context, ids []int64, name string, body map[string]any) commandRecord {
	if body == nil {
		body = map[string]any{}
	}
	body["trigger"], body["sendUpdatesToClient"], body["updateScheduledTask"] = "manual", false, false
	now := h.now()
	rec := commandRecord{ID: doneID, Name: name, CommandName: words(name), Body: body, Priority: "normal", Status: "completed",
		Result: "successful", Queued: utc(now), Trigger: "manual", Started: utcPtr(&now), Ended: utcPtr(&now), Duration: "00:00:00"}
	rec.StateChangeTime = rec.Ended
	if len(ids) == 0 {
		return rec
	}
	var j domain.Job
	if err := database.Get(ctx, h.Store.Read(), "jobs", ids[0], &j); err != nil {
		rec.ID, rec.Status, rec.Result, rec.Started, rec.Ended, rec.StateChangeTime, rec.Duration = ids[0], "queued", "unknown", nil, nil, nil, ""
		return rec
	}
	return jobRecord(j, name, body)
}

// jobRecord turns a Job into a command record. The states run queued,
// started, then completed, failed or cancelled.
func jobRecord(j domain.Job, name string, body map[string]any) commandRecord {
	rec := commandRecord{ID: j.ID, Name: name, CommandName: words(name), Body: body, Priority: "normal", Result: "unknown",
		Queued: utc(j.CreatedAt), Trigger: "manual", Started: utcPtr(j.StartedAt), Ended: utcPtr(j.EndedAt)}
	if j.ProgressNote != nil {
		rec.Message = *j.ProgressNote
	}
	switch j.State {
	case "running":
		rec.Status = "started"
	case "done":
		rec.Status, rec.Result = "completed", "successful"
	case "failed":
		rec.Status = "failed"
		if j.Error != nil {
			rec.Exception = j.Error.Message
		}
	case "cancelled":
		rec.Status = "cancelled"
	default:
		rec.Status = "queued"
	}
	rec.StateChangeTime = rec.Started
	if rec.StateChangeTime == nil {
		rec.StateChangeTime = rec.Ended
	}
	if j.StartedAt != nil && j.EndedAt != nil {
		rec.Duration = timeSpan(int64(j.EndedAt.Sub(*j.StartedAt) / time.Second))
	}
	return rec
}

// getCommand polls a command by id. doneID always reads completed.
func (h *handler) getCommand(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if id == doneID {
		writeJSON(w, http.StatusOK, h.commandRecord(r.Context(), nil, "RefreshMonitoredDownloads", nil))
		return
	}
	var j domain.Job
	if err := database.Get(r.Context(), h.Store.Read(), "jobs", id, &j); err != nil {
		internalError(w, err)
		return
	}
	// Only Job kinds a command queues can be polled, so other Jobs stay
	// private.
	name := jobCommandName(j.Kind, h.side)
	if name == "" {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, jobRecord(j, name, map[string]any{"name": name}))
}

// jobCommandName names the command a Job kind stands for when polled. It
// is "" for a kind no command queues.
func jobCommandName(kind, side string) string {
	switch kind {
	case jobs.KindSearchSlot, jobs.KindSearchSeason:
		if side == "radarr" {
			return "MoviesSearch"
		}
		return "SeriesSearch"
	case jobs.KindSearchMissing:
		return "MissingEpisodeSearch"
	case library.KindRefreshTitle:
		if side == "radarr" {
			return "RefreshMovie"
		}
		return "RefreshSeries"
	case library.KindScanTitle:
		if side == "radarr" {
			return "RescanMovie"
		}
		return "RescanSeries"
	}
	return ""
}

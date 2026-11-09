package health

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// ScheduleHandlers serves the Schedules page: every scheduled Job kind and
// supervised loop, run now, and the interval and on/off switch.
type ScheduleHandlers struct {
	Store *database.Store
	// Queue runs and reschedules Jobs (nil: nothing can be run).
	Queue *jobs.Queue
	// States is the supervisor's loop registry (may be nil).
	States *jobs.Registry
	// Parts switches the parts and stages (nil: every part on).
	Parts *parts.Control
	// SafeMode says the app started with only the core.
	SafeMode bool
}

// ScheduleInfo says what a scheduled Job kind does in plain words and how
// far its interval may move (seconds; Step is the unit the page uses).
// Kinds that also have a settings page keep that page's bounds.
type ScheduleInfo struct {
	Label, About   string
	Min, Max, Step int64
	// Settings is the page that also changes it, if any.
	Settings string
}

const (
	schMinute = int64(60)
	schHour   = 60 * schMinute
	schDay    = 24 * schHour
)

var ScheduleInfos = map[string]ScheduleInfo{
	"refresh_titles":        {"Refresh movie details", "Reads names, dates and artwork of every movie again from TMDB.", 6 * schHour, 7 * schDay, schHour, ""},
	"refresh_shows":         {"Refresh show details", "Reads seasons, episodes and air dates of every show again.", 6 * schHour, 7 * schDay, schHour, ""},
	"refresh_artists":       {"Refresh artist details", "Reads albums and Pressings of every artist again from MusicBrainz.", 6 * schHour, 7 * schDay, schHour, ""},
	"artist_pictures":       {"Find artist pictures", "Fills missing artist photos, wide backgrounds and logos from the HolyArrs metadata server.", schHour, 7 * schDay, schHour, ""},
	"check_library_folders": {"Check Library folders", "Checks that every Library folder is there and has free space. The write and hardlink test runs at start-up, on Run now and when folders change.", schHour, schDay, schHour, ""},
	"sync_outside_lists":    {"Read Outside lists", "Adds what TMDB and Trakt lists hold now.", 15 * schMinute, schDay, 5 * schMinute, ""},
	"check_bridges":         {"Check Jackett", "Keeps the Sources from a connected Jackett in step with its sites.", 6 * schHour, 7 * schDay, schHour, "/settings/sources"},
	"sync_plex_watchlists":  {"Read Plex watchlists", "Asks for what is new on the Plex watchlists of people who turned it on.", 3 * schMinute, schHour, schMinute, "/settings/requests"},
	"feed_check":            {"Feed check", "Reads the newest Releases of every Source with a feed.", 10 * schMinute, 120 * schMinute, schMinute, "/settings/search"},
	"grab_held":             {"Try Held releases again", "Sends Held releases whose wait is over.", schMinute, schHour, schMinute, ""},
	"search_missing":        {"Search for missing", "Searches the Sources for missing titles, a few at a time.", 6 * schHour, 7 * schDay, schHour, "/settings/search"},
	"rescan_library":        {"Rescan the Library", "Looks for files added, changed or removed by hand.", 6 * schHour, 7 * schDay, schHour, ""},
	"clean_holding":         {"Clean the holding folder", "Removes deleted files whose keep time is over.", 6 * schHour, 7 * schDay, schHour, ""},
	"health_check":          {"Health checks", "Library folders, free space, Sources, Downloaders, backups and the update check.", schMinute, schDay, schMinute, "/settings/this-app"},
	"clean_history":         {"Clean up History", "Removes History Events older than their keep time.", 6 * schHour, 7 * schDay, schHour, "/settings/this-app"},
	"subtitle_missing":      {"Look for missing subtitles", "Searches the subtitle sites for every file missing a wished language.", schHour, 7 * schDay, schHour, "/settings/subtitles"},
	"subtitle_better":       {"Look for better subtitles", "Searches again for subtitles under their Pass mark.", 6 * schHour, 7 * schDay, schHour, "/settings/subtitles"},
}

// ScheduleView is one scheduled Job kind on the Schedules page.
type ScheduleView struct {
	Kind         string     `json:"kind"`
	Label        string     `json:"label"`
	About        string     `json:"about"`
	EverySeconds int64      `json:"every_s"`
	MinSeconds   int64      `json:"min_s"`
	MaxSeconds   int64      `json:"max_s"`
	StepSeconds  int64      `json:"step_s"`
	Enabled      bool       `json:"enabled"`
	LastRun      *time.Time `json:"last_run,omitempty"`
	NextRun      *time.Time `json:"next_run,omitempty"`
	// State: waiting, queued, running, off (the schedule is turned off),
	// part_off (its part is off) or safe_mode.
	State string `json:"state"`
	// LastJob is how the newest Job of the kind ended (done, failed,
	// cancelled), with when and why.
	LastJob      string     `json:"last_job,omitempty"`
	LastJobEnded *time.Time `json:"last_job_ended,omitempty"`
	LastJobError string     `json:"last_job_error,omitempty"`
	Part         string     `json:"part"`
	Settings     string     `json:"settings,omitempty"`
}

// Routes adds the Schedules page routes.
func (h *ScheduleHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/schedules", h.listSchedules)
	m.HandleFunc("GET /api/v1/jobs/active", h.activeJobs)
	m.HandleFunc("POST /api/v1/schedules/{kind}/run", h.runSchedule)
	m.HandleFunc("PUT /api/v1/schedules/{kind}", h.putSchedule)
}

// kindOn says whether a kind's part is on and, if not, the state to show.
func (h *ScheduleHandlers) kindOn(kind string) (string, bool) {
	o := parts.JobOwner(kind)
	if h.SafeMode && o != parts.Core {
		return "safe_mode", false
	}
	if h.Parts != nil && o != "" && o != parts.ByTitle && !h.Parts.OwnerOn(o) {
		return "part_off", false
	}
	return "", true
}

func (h *ScheduleHandlers) scheduleView(ctx context.Context, db *sql.DB, sc domain.Schedule) (ScheduleView, error) {
	info, ok := ScheduleInfos[sc.Kind]
	if !ok {
		info = ScheduleInfo{Label: strings.ReplaceAll(sc.Kind, "_", " "), Min: schMinute, Max: 7 * schDay, Step: schMinute}
	}
	v := ScheduleView{Kind: sc.Kind, Label: info.Label, About: info.About, EverySeconds: sc.EverySeconds, MinSeconds: info.Min, MaxSeconds: info.Max,
		StepSeconds: info.Step, Enabled: sc.Enabled, LastRun: sc.LastRun, NextRun: sc.NextRun, Part: string(parts.JobOwner(sc.Kind)), Settings: info.Settings, State: "waiting"}
	if h.Queue != nil {
		v.NextRun = h.Queue.EffectiveScheduleNext(sc)
	}
	var live int
	var running int
	if err := db.QueryRowContext(ctx, "SELECT count(*), coalesce(sum(state = 'running'), 0) FROM jobs WHERE kind = ? AND state IN ('queued', 'running')", sc.Kind).Scan(&live, &running); err != nil {
		return v, err
	}
	switch {
	case running > 0:
		v.State = "running"
	case live > 0:
		v.State = "queued"
	}
	if st, on := h.kindOn(sc.Kind); !on {
		v.State = st
	} else if !sc.Enabled && v.State == "waiting" {
		v.State = "off"
	}
	jobs, err := database.Select[domain.Job](ctx, db, "jobs", "kind = ? AND state IN ('done', 'failed', 'cancelled') ORDER BY id DESC LIMIT 1", sc.Kind)
	if err != nil {
		return v, err
	}
	if len(jobs) == 1 {
		j := jobs[0]
		v.LastJob, v.LastJobEnded = j.State, j.EndedAt
		if j.Error != nil {
			v.LastJobError = logs.RedactPatterns(j.Error.Message)
		}
	}
	return v, nil
}

// LoopView is one supervised background loop on the Schedules page.
type LoopView struct {
	jobs.Status
	Part string `json:"part"`
}

// listSchedules answers every scheduled Job kind and every background
// loop the supervisor runs, with last and next run and state.
func (h *ScheduleHandlers) listSchedules(w http.ResponseWriter, r *http.Request) {
	db := h.Store.Read()
	scs, err := database.Select[domain.Schedule](r.Context(), db, "schedules", "1=1 ORDER BY id")
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []ScheduleView{}
	for _, sc := range scs {
		v, err := h.scheduleView(r.Context(), db, sc)
		if err != nil {
			webserver.Internal(w, "api", err)
			return
		}
		out = append(out, v)
	}
	loops := []LoopView{}
	if h.States != nil {
		for _, st := range h.States.List() {
			name, ok := strings.CutPrefix(st.Key, "stage:")
			if !ok || st.Kind != "stage" {
				continue
			}
			st.Reason = logs.RedactPatterns(st.Reason)
			loops = append(loops, LoopView{Status: st, Part: string(parts.LoopOwner(name))})
		}
	}
	sort.SliceStable(loops, func(i, j int) bool { return loops[i].Name < loops[j].Name })
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"schedules": out, "loops": loops, "safe_mode": h.SafeMode})
}

// ActiveJob is one Job running or waiting right now, for the Health page's
// live board.
type ActiveJob struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	// State: running or queued.
	State string `json:"state"`
	// Progress is 0 to 1; 0 while a Job has not said how far it is.
	Progress  float64    `json:"progress"`
	Note      string     `json:"note,omitempty"`
	QueuedAt  time.Time  `json:"queued_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	// ByPerson is true when a person asked for the run (Run now), not the clock.
	ByPerson bool `json:"by_person"`
}

// activeJobs lists the Jobs running or waiting right now, running first,
// then by their place in line.
func (h *ScheduleHandlers) activeJobs(w http.ResponseWriter, r *http.Request) {
	js, err := database.Select[domain.Job](r.Context(), h.Store.Read(), "jobs",
		"state IN ('running', 'queued') ORDER BY CASE state WHEN 'running' THEN 0 ELSE 1 END, run_after, id LIMIT 50")
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := []ActiveJob{}
	for _, j := range js {
		label := strings.ReplaceAll(j.Kind, "_", " ")
		if info, ok := ScheduleInfos[j.Kind]; ok {
			label = info.Label
		}
		v := ActiveJob{ID: j.ID, Kind: j.Kind, Label: label, State: j.State, Progress: j.Progress, QueuedAt: j.CreatedAt, StartedAt: j.StartedAt, ByPerson: j.AskedBy.Kind == "user"}
		if j.ProgressNote != nil {
			v.Note = logs.RedactPatterns(*j.ProgressNote)
		}
		out = append(out, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"jobs": out})
}

func (h *ScheduleHandlers) scheduleOf(w http.ResponseWriter, r *http.Request) (domain.Schedule, bool) {
	scs, err := database.Select[domain.Schedule](r.Context(), h.Store.Read(), "schedules", "kind = ?", r.PathValue("kind"))
	if err != nil {
		webserver.Internal(w, "api", err)
		return domain.Schedule{}, false
	}
	if len(scs) == 0 {
		webserver.Fail(w, http.StatusNotFound, "no such schedule")
		return domain.Schedule{}, false
	}
	return scs[0], true
}

// runSchedule queues the schedule's Job now, without moving its clock. A
// Job of the kind already waiting or running is answered instead. A
// schedule turned off can still be run by hand with the usual permission.
func (h *ScheduleHandlers) runSchedule(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.scheduleOf(w, r)
	if !ok {
		return
	}
	if st, on := h.kindOn(sc.Kind); !on {
		msg := "its part is off; turn it on in Settings > Parts and controls first"
		if st == "safe_mode" {
			msg = "HolyArrs runs in safe mode: only the core runs; restart without safe mode first"
		}
		webserver.Fail(w, http.StatusConflict, msg)
		return
	}
	if h.Queue == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "the Job queue is not running")
		return
	}
	params := map[string]any{"scheduled": true, "by_hand": true}
	if sc.LastRun != nil {
		params["last_start"] = sc.LastRun.UTC().Format(time.RFC3339Nano)
	}
	id, created, err := h.Queue.Enqueue(r.Context(), sc.Kind, nil, params, "schedule:"+sc.Kind, webserver.RequestActor(r))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": id, "already": !created})
}

// putSchedule changes a schedule's interval within its bounds; the next
// run moves to now plus the new interval.
func (h *ScheduleHandlers) putSchedule(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.scheduleOf(w, r)
	if !ok {
		return
	}
	var in struct {
		EverySeconds *int64 `json:"every_s"`
		Enabled      *bool  `json:"enabled"`
	}
	if !webserver.Decode(w, r, &in, `send {"every_s": <seconds>} or {"enabled": false}`) {
		return
	}
	if in.EverySeconds == nil && in.Enabled == nil {
		webserver.Fail(w, http.StatusBadRequest, "send every_s, enabled, or both")
		return
	}
	info, known := ScheduleInfos[sc.Kind]
	if !known {
		webserver.Fail(w, http.StatusConflict, "this schedule's interval cannot be changed here")
		return
	}
	if in.Enabled != nil && sc.Kind != "refresh_titles" && sc.Kind != "rescan_library" {
		webserver.Fail(w, http.StatusConflict, "only scheduled movie details refresh and the Library rescan can be turned off here")
		return
	}
	if in.EverySeconds != nil && (*in.EverySeconds < info.Min || *in.EverySeconds > info.Max || *in.EverySeconds%info.Step != 0) {
		webserver.Fail(w, http.StatusBadRequest, "every_s must be "+Span(info.Min)+" to "+Span(info.Max)+" in steps of "+Span(info.Step))
		return
	}
	now := time.Now().UTC()
	if h.Queue == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "the Job queue is not running")
		return
	}
	err := h.Queue.UpdateSchedule(r.Context(), sc.Kind, in.EverySeconds, in.Enabled, webserver.RequestActor(r), now)
	if errors.Is(err, sql.ErrNoRows) {
		webserver.Fail(w, http.StatusNotFound, "no such schedule")
		return
	}
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	sc, ok = h.scheduleOf(w, r)
	if !ok {
		return
	}
	v, err := h.scheduleView(r.Context(), h.Store.Read(), sc)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

// span says a number of seconds in minutes, hours or days.
func Span(s int64) string {
	switch {
	case s%schDay == 0:
		return strconv.FormatInt(s/schDay, 10) + plural(s/schDay, " day", " days")
	case s%schHour == 0:
		return strconv.FormatInt(s/schHour, 10) + plural(s/schHour, " hour", " hours")
	case s%schMinute == 0:
		return strconv.FormatInt(s/schMinute, 10) + plural(s/schMinute, " minute", " minutes")
	}
	return strconv.FormatInt(s, 10) + " seconds"
}

func plural(n int64, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// RunNow is the "run now" route's handler, for callers that serve it
// outside Routes.
func (h *ScheduleHandlers) RunNow(w http.ResponseWriter, r *http.Request) { h.runSchedule(w, r) }

package api

import (
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// ScheduleView and LoopView are the Schedules page rows as health shows them.
type (
	ScheduleView = health.ScheduleView
	LoopView     = health.LoopView
	ActiveJob    = health.ActiveJob
)

// scheduleInfos are the scheduled Job kinds' words and bounds.
var scheduleInfos = health.ScheduleInfos

// scheduleRoutes mounts the health module's Schedules routes.
func (a *API) scheduleRoutes(m *http.ServeMux) { a.scheduleHandlers().Routes(m) }

func (a *API) scheduleHandlers() *health.ScheduleHandlers {
	return &health.ScheduleHandlers{Store: a.s.Store, Queue: a.s.Queue, States: a.s.States, Parts: a.s.Parts, SafeMode: a.s.SafeMode}
}

// runSchedule runs a scheduled Job kind now.
func (a *API) runSchedule(w http.ResponseWriter, r *http.Request) { a.scheduleHandlers().RunNow(w, r) }

// span says a number of seconds in minutes, hours or days.
func span(s int64) string { return health.Span(s) }

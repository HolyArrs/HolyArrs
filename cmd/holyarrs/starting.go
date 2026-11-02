package main

import (
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/CoderLuii/holyarrs/internal/store"
)

// swapHandler lets the listener answer before the app is ready: a small
// page while the data is opened (and updated), then the real API.
type swapHandler struct{ h atomic.Pointer[http.Handler] }

func (s *swapHandler) set(h http.Handler) { s.h.Store(&h) }

func (s *swapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	(*s.h.Load()).ServeHTTP(w, r)
}

// startingHandler answers while the data is opened: healthz says the app
// is alive ("updating" once updating is set: an update of the data may
// take a while, "starting" before), readyz and every page say not yet.
func startingHandler(updating *atomic.Bool) http.Handler {
	starting := holdingPage(http.StatusOK, `{"state":"starting"}`, "HolyArrs is starting, back soon.\n")
	busy := holdingPage(http.StatusOK, `{"state":"updating"}`, "HolyArrs is updating its data, back soon.\n")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if updating != nil && updating.Load() {
			busy.ServeHTTP(w, r)
			return
		}
		starting.ServeHTTP(w, r)
	})
}

// goBack is how to return to the data from before the update.
const goBack = " To go back on Docker, run `docker exec <container> /holyarrs restore --pre-update`, then `docker restart <container>`. Without Docker: stop HolyArrs, run holyarrs restore --pre-update, start it again."

// failedHandler answers when the data could not be opened for a reason a
// person has to fix; healthz reports it too.
func failedHandler(msg string) http.Handler {
	return holdingPage(http.StatusServiceUnavailable, `{"state":"failed"}`, "HolyArrs did not start: "+msg+"\n")
}

// publicReason says what went wrong without paths or other details, since
// the page is open to anyone; the log keeps the full error.
func publicReason(err error) string {
	var nd *store.NewerDataError
	switch {
	case errors.As(err, &nd):
		return "this data is from a newer HolyArrs. Run that version again, or restore a backup." + goBack
	case errors.Is(err, store.ErrNoSpace):
		return "not enough free disk space to update safely. Free some space and restart (or set " + store.SkipSpaceCheckEnv + "=yes to skip this check). Nothing was changed."
	case errors.Is(err, store.ErrBackup):
		return "the backup before updating failed, so nothing was changed. Check that the backups folder can be written, see the log, and restart."
	case errors.Is(err, store.ErrMigration):
		return "the update of the data failed and was undone." + goBack
	case errors.Is(err, store.ErrCheckFailed):
		return "the data check after updating failed." + goBack
	}
	return "see the log for details." + goBack
}

func holdingPage(healthCode int, health, page string) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(healthCode)
		_, _ = w.Write([]byte(health + "\n"))
	})
	m.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(page))
	})
	return m
}

// needsPerson reports whether a store.Open error is one the app should
// keep showing (newer data, no space, no backup, failed check) instead of
// exiting into a restart loop.
func needsPerson(err error) bool {
	var nd *store.NewerDataError
	return errors.As(err, &nd) || errors.Is(err, store.ErrNoSpace) || errors.Is(err, store.ErrBackup) || errors.Is(err, store.ErrCheckFailed) || errors.Is(err, store.ErrMigration)
}

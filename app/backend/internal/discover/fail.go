package discover

import (
	"context"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Fail answers a provider error: 200 source_off when lookups are off on
// this install, 501 not set up, 503 rate limited, 404 unknown, 502
// anything else.
func Fail(w http.ResponseWriter, err error) {
	switch {
	case metadata.SourceOff(err):
		metadata.WriteSourceOff(w)
	case errors.Is(err, metadata.ErrNoAccess):
		webserver.Fail(w, http.StatusNotImplemented, "Explore is not available here")
	case errors.Is(err, metadata.ErrRateLimited):
		w.Header().Set("Retry-After", "60")
		webserver.Fail(w, http.StatusServiceUnavailable, "The movie and show details are busy; try again in a minute")
	case errors.Is(err, metadata.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "That title could not be found")
	case DetailsBusy(err):
		// Held back, retrying or not answering for now: the page asks again
		// on its own after Retry-After.
		w.Header().Set("Retry-After", "5")
		webserver.Fail(w, http.StatusServiceUnavailable, "The movie and show details are busy; try again in a moment")
	default:
		webserver.Fail(w, http.StatusBadGateway, "The movie and show details did not answer; try again shortly")
	}
}

// DetailsBusy is an error that passes on its own: the service is held,
// its breaker is waiting, or it did not answer in time.
func DetailsBusy(err error) bool {
	return errors.Is(err, outbound.ErrSuppressed) || errors.Is(err, metadata.ErrServiceUnavailable) ||
		errors.Is(err, metadata.ErrServiceBusy) || errors.Is(err, metadata.ErrRateLimited) ||
		errors.Is(err, context.DeadlineExceeded)
}

// StaleOK says whether a kept copy may answer err: anything but a refusal
// or an unknown title.
func StaleOK(err error) bool {
	return !errors.Is(err, metadata.ErrNotFound) && !errors.Is(err, metadata.ErrNoAccess)
}

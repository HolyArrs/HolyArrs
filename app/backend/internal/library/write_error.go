package library

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// WriteError answers the library's refusals with a status and a reason.
func WriteError(w http.ResponseWriter, err error) {
	if ServiceError(w, err) {
		return
	}
	var dup *AlreadyAddedError
	var fe *FolderError
	switch {
	case errors.As(err, &dup):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.As(err, &fe):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "health": fe.Health.State, "reason": fe.Health.Reason})
	case errors.Is(err, ErrNeverAdd), errors.Is(err, ErrNoFolder), errors.Is(err, ErrFolderInUse), errors.Is(err, ErrFolderHeld), errors.Is(err, ErrFolderListed),
		errors.Is(err, ErrFolderTaken), errors.Is(err, ErrFolderName), errors.Is(err, ErrMainVersion), errors.Is(err, ErrVersionHasFiles), errors.Is(err, ErrVersionBusy),
		errors.Is(err, ErrVersionFolder), errors.Is(err, ErrProtectedCopy):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrBadGate), errors.Is(err, ErrBadDelay), errors.Is(err, ErrNotMovieFolder), errors.Is(err, ErrNotShowFolder), errors.Is(err, ErrNotMusicFolder),
		errors.Is(err, ErrNoPickRules), errors.Is(err, ErrNotMusicRules), errors.Is(err, ErrNotVideoRules), errors.Is(err, ErrFolderPath), errors.Is(err, ErrFolderKind),
		errors.Is(err, ErrFolderAbsent), errors.Is(err, ErrNoMetadataIDs), errors.Is(err, ErrVersionLabel),
		errors.Is(err, ErrTooManyVersions), errors.Is(err, ErrCopyChoice):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, database.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "not found")
	case errors.Is(err, metadata.ErrNotFound):
		webserver.Fail(w, http.StatusNotFound, "no movie with that id")
	case errors.Is(err, metadata.ErrNoAccess):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	default:
		webserver.Internal(w, "api", err)
	}
}

// ServiceError keeps service failures useful without exposing transport
// or credential details. Direct provider errors retain their existing answers.
func ServiceError(w http.ResponseWriter, err error) bool {
	var message string
	status := http.StatusServiceUnavailable
	switch {
	case errors.Is(err, metadata.ErrServiceAccess):
		message = metadata.ErrServiceAccess.Error()
	case errors.Is(err, metadata.ErrServiceUnavailable):
		message = metadata.ErrServiceUnavailable.Error()
	case errors.Is(err, metadata.ErrServiceBusy):
		message = metadata.ErrServiceBusy.Error()
	case errors.Is(err, metadata.ErrServiceIncomplete):
		message = metadata.ErrServiceIncomplete.Error()
	case errors.Is(err, metadata.ErrServiceInvalid):
		status = http.StatusBadGateway
		message = metadata.ErrServiceInvalid.Error()
	default:
		return false
	}
	webserver.Fail(w, status, message)
	return true
}

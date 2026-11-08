package music

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TrackView is one track with its Slots.
type TrackView[S any] struct {
	ID           int64             `json:"id"`
	Disc         int               `json:"disc"`
	Number       int               `json:"number"`
	Name         string            `json:"name,omitempty"`
	LengthMS     *int              `json:"length_ms,omitempty"`
	RecordingID  string            `json:"recording_id,omitempty"`
	ReleaseDate  *time.Time        `json:"release_date,omitempty"`
	KeepUpToDate bool              `json:"keep_up_to_date"`
	Dropped      bool              `json:"dropped,omitempty"`
	IDs          map[string]string `json:"ids"`
	Slots        []S               `json:"slots"`
}

// TrackHandlers serves an album's track list.
type TrackHandlers[S any] struct {
	Store *database.Store
	// Slots builds each track's Slot views, keyed by item id.
	Slots func(ctx context.Context, db *sql.DB, p mediafiles.Part) (map[int64][]S, error)
}

// Routes registers the track list route.
func (h TrackHandlers[S]) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/albums/{id}/tracks", h.ListTracks)
}

func strOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ListTracks lists an album's tracks in disc and track order, each with its
// Slot per Version.
func (h TrackHandlers[S]) ListTracks(w http.ResponseWriter, r *http.Request) {
	db := h.Store.Read()
	p, ok := AlbumID(w, r, db)
	if !ok {
		return
	}
	its, err := database.Select[mediafiles.Item](r.Context(), db, "items", `part_id = ? AND kind = 'track'
		ORDER BY json_extract(numbers, '$.disc'), json_extract(numbers, '$.track'), id`, p.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	byItem, err := h.Slots(r.Context(), db, p)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := make([]TrackView[S], 0, len(its))
	for _, it := range its {
		v := TrackView[S]{ID: it.ID, Name: strOr(it.Name), LengthMS: it.LengthMS, RecordingID: strOr(it.RecordingID), ReleaseDate: it.AirDate,
			KeepUpToDate: it.KeepUpToDate, Dropped: it.DroppedAt != nil, IDs: it.SourceIDs, Slots: []S{}}
		if it.Numbers.Disc != nil {
			v.Disc = *it.Numbers.Disc
		}
		if it.Numbers.Track != nil {
			v.Number = *it.Numbers.Track
		}
		if v.IDs == nil {
			v.IDs = map[string]string{}
		}
		if sl := byItem[it.ID]; sl != nil {
			v.Slots = sl
		}
		out = append(out, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"tracks": out, "album_id": p.ID})
}

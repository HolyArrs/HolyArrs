package music

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// LibraryHandlers serves an artist's albums, their Pressings and the music
// settings.
type LibraryHandlers struct {
	Store   *database.Store
	Library *library.Service
	// ArtistID reads an artist Title from the path; false means an answer was sent.
	ArtistID func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// Actor is who made the request.
	Actor func(r *http.Request) domain.Actor
	// Fallback answers errors that are not music refusals.
	Fallback func(w http.ResponseWriter, err error)
}

// Routes registers the album and music settings routes.
func (h LibraryHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/albums", h.ListAlbums)
	m.HandleFunc("GET /api/v1/albums/{id}", h.GetAlbum)
	m.HandleFunc("PUT /api/v1/albums/{id}", h.SetAlbum)
	m.HandleFunc("GET /api/v1/settings/music", h.GetSettings)
	m.HandleFunc("PUT /api/v1/settings/music", h.PutSettings)
}

// Error answers music refusals with a status and a reason; anything else
// goes to fallback.
func Error(w http.ResponseWriter, err error, fallback func(http.ResponseWriter, error)) {
	var dup *library.ArtistAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "title_id": dup.TitleID})
	case errors.Is(err, library.ErrNoMusicFolder), errors.Is(err, library.ErrArtistNeverAdd), errors.Is(err, library.ErrAlbumNotIncluded):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, library.ErrBadAlbumChoice), errors.Is(err, library.ErrBadNewAlbums), errors.Is(err, library.ErrBadAlbumKinds),
		errors.Is(err, library.ErrBadExtraKinds), errors.Is(err, library.ErrBadStatuses), errors.Is(err, library.ErrBadPressingRule),
		errors.Is(err, library.ErrBadMusicURL), errors.Is(err, library.ErrBadCacheHours), errors.Is(err, library.ErrNotMusicFolder),
		errors.Is(err, library.ErrBadArtistID):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, library.ErrNotArtist), errors.Is(err, library.ErrNotAlbum), errors.Is(err, library.ErrNoSuchPressing),
		errors.Is(err, metadata.ErrArtistNotFound):
		webserver.Fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, library.ErrNoMusicSource), errors.Is(err, metadata.ErrMusicBusy):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, metadata.ErrMusicUnavailable):
		// Plain words only: no status code or connection detail.
		webserver.Fail(w, http.StatusBadGateway, metadata.ErrMusicUnavailable.Error())
	default:
		fallback(w, err)
	}
}

func (h LibraryHandlers) fail(w http.ResponseWriter, err error) { Error(w, err, h.Fallback) }

// ListOr is v, or an empty list for nil.
func ListOr(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// ListAlbums lists an artist's albums, newest first; ?included=true leaves
// out the albums the artist's kinds do not include.
func (h LibraryHandlers) ListAlbums(w http.ResponseWriter, r *http.Request) {
	t, ok := h.ArtistID(w, r)
	if !ok {
		return
	}
	als, err := library.Albums(r.Context(), h.Store.Read(), t.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if r.URL.Query().Get("included") == "true" {
		kept := als[:0]
		for _, al := range als {
			if al.Included {
				kept = append(kept, al)
			}
		}
		als = kept
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"albums": als})
}

// AlbumID reads an album from the path.
func AlbumID(w http.ResponseWriter, r *http.Request, db database.Querier) (mediafiles.Part, bool) {
	var p mediafiles.Part
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return p, false
	}
	if err := database.Get(r.Context(), db, "parts", id, &p); err != nil || p.Kind != "album" {
		webserver.Fail(w, http.StatusNotFound, "no such album")
		return p, false
	}
	return p, true
}

// PressingView is one Pressing of an album with its tracks.
type PressingView struct {
	ID             int64                  `json:"id"`
	MBID           string                 `json:"mbid"`
	Name           string                 `json:"name"`
	Status         string                 `json:"status"`
	Country        string                 `json:"country,omitempty"`
	Date           string                 `json:"date,omitempty"`
	Disambiguation string                 `json:"disambiguation,omitempty"`
	Formats        []string               `json:"formats"`
	TrackCount     int                    `json:"track_count"`
	Chosen         bool                   `json:"chosen"`
	ChosenBy       string                 `json:"chosen_by,omitempty"` // auto, person (the chosen one only)
	Tracks         []domain.PressingTrack `json:"tracks"`
}

// GetAlbum answers GET /api/v1/albums/{id}.
func (h LibraryHandlers) GetAlbum(w http.ResponseWriter, r *http.Request) {
	p, ok := AlbumID(w, r, h.Store.Read())
	if !ok {
		return
	}
	sum, err := library.AlbumOf(r.Context(), h.Store.Read(), p)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	ps, err := database.Select[domain.AlbumPressing](r.Context(), h.Store.Read(), "pressings", "part_id = ? ORDER BY chosen DESC, date, id", p.ID)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	views := make([]PressingView, 0, len(ps))
	for _, x := range ps {
		v := PressingView{ID: x.ID, MBID: x.MBID, Name: x.Name, Status: x.Status, Country: x.Country, Date: x.Date, Disambiguation: x.Disambiguation,
			Formats: ListOr(x.Formats), TrackCount: x.TrackCount, Chosen: x.Chosen, Tracks: x.Tracks}
		if x.Chosen {
			v.ChosenBy = x.ChosenBy
		}
		if v.Tracks == nil {
			v.Tracks = []domain.PressingTrack{}
		}
		views = append(views, v)
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"album": sum, "title_id": p.TitleID, "pressings": views})
}

const albumHelp = `send {"keep_up_to_date": true or false} and/or {"pressing_id": <id of one of the album's Pressings>}`

// SetAlbum answers PUT /api/v1/albums/{id}.
func (h LibraryHandlers) SetAlbum(w http.ResponseWriter, r *http.Request) {
	p, ok := AlbumID(w, r, h.Store.Read())
	if !ok {
		return
	}
	var body struct {
		KeepUpToDate *bool `json:"keep_up_to_date"`
		PressingID   int64 `json:"pressing_id"`
	}
	if !webserver.Decode(w, r, &body, albumHelp) {
		return
	}
	if body.KeepUpToDate == nil && body.PressingID == 0 {
		webserver.Fail(w, http.StatusBadRequest, albumHelp)
		return
	}
	p, err := h.Library.SetAlbum(r.Context(), p.ID, body.KeepUpToDate, body.PressingID, h.Actor(r))
	if err != nil {
		h.fail(w, err)
		return
	}
	sum, err := library.AlbumOf(r.Context(), h.Store.Read(), p)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, sum)
}

// GetSettings answers GET /api/v1/settings/music.
func (h LibraryHandlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadMusicDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}

// PutSettings answers PUT /api/v1/settings/music.
func (h LibraryHandlers) PutSettings(w http.ResponseWriter, r *http.Request) {
	d, err := library.ReadMusicDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !webserver.Decode(w, r, &d, "send any of album_choice, new_albums, album_types, secondary_types, release_statuses, pressing_rule, musicbrainz_url, musicbrainz_allow_local_network, cache_hours") {
		return
	}
	if err := library.SetMusicDefaults(r.Context(), h.Store, d, h.Actor(r)); err != nil {
		h.fail(w, err)
		return
	}
	d, err = library.ReadMusicDefaults(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, d)
}

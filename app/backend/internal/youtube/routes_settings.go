package youtube

import (
	"context"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// settingsView is the YouTube settings card: the settings, the library
// total over everyone (counts only, no titles), where each program was
// found and yt-dlp's version.
type settingsView struct {
	Settings
	Totals     Totals `json:"totals"`
	YTDLPPath  string `json:"ytdlp_path"`  // "" when yt-dlp is not found
	FFmpegPath string `json:"ffmpeg_path"` // "" when ffmpeg is not found
	Version    string `json:"version"`     // "" when yt-dlp is missing or does not answer
}

func (h *Handlers) view(ctx context.Context) (settingsView, error) {
	v := settingsView{Settings: LoadSettings(ctx, h.Svc.Store.Read())}
	t, err := ReadTotals(ctx, h.Svc.Store.Read())
	if err != nil {
		return v, err
	}
	v.Totals = t
	r := h.Svc.runner()
	v.YTDLPPath, v.FFmpegPath = r.Tools.YTDLP, r.Tools.FFmpeg
	if r.Tools.YTDLP != "" {
		v.Version, _ = r.Version(ctx)
	}
	return v, nil
}

func (h *Handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.caller(w, r); !ok {
		return
	}
	v, err := h.view(r.Context())
	if err != nil {
		webserver.Internal(w, "youtube", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, v)
}

func (h *Handlers) putSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.caller(w, r); !ok {
		return
	}
	// Fields left out keep their saved values.
	in := LoadSettings(r.Context(), h.Svc.Store.Read())
	if !webserver.Decode(w, r, &in, `send {"folder": "/data/youtube", "min_free_gb": 10}`) {
		return
	}
	if _, err := h.Svc.SaveSettings(r.Context(), in, webserver.RequestActor(r)); err != nil {
		switch {
		case errors.Is(err, ErrFolderHasVideos):
			webserver.Fail(w, http.StatusConflict, err.Error())
		case errors.Is(err, ErrMinFree), folderError(err):
			webserver.Fail(w, http.StatusBadRequest, err.Error())
		default:
			webserver.Internal(w, "youtube", err)
		}
		return
	}
	h.getSettings(w, r)
}

// folderError reports whether err is a refused folder rather than a fault.
func folderError(err error) bool {
	for _, e := range []error{library.ErrFolderPath, library.ErrFolderAbsent, library.ErrFolderReadOnly, library.ErrFolderOverlap} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// adopt plans adopting a folder of videos (dry_run) or queues the job that
// does it. Owner only.
func (h *Handlers) adopt(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.caller(w, r); !ok {
		return
	}
	var in AdoptRequest
	if !webserver.Decode(w, r, &in, `send {"user_id": 1, "path": "/data/old-videos", "playlists": {"Folder": "https://www.youtube.com/playlist?list=..."}, "dry_run": true}`) {
		return
	}
	if in.DryRun {
		items, err := h.Svc.PlanAdopt(r.Context(), in)
		if err != nil {
			h.adoptFail(w, err)
			return
		}
		webserver.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	id, err := h.Svc.StartAdopt(r.Context(), in, webserver.RequestActor(r))
	if err != nil {
		h.adoptFail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (h *Handlers) adoptFail(w http.ResponseWriter, err error) {
	switch {
	case IsAdoptRefusal(err):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrNeedsTools):
		webserver.Fail(w, http.StatusServiceUnavailable, err.Error())
	default:
		webserver.Internal(w, "youtube", err)
	}
}

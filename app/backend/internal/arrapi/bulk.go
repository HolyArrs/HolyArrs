package arrapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Kometa drives the old apps through the arrapi library: it adds movies
// and shows in bulk, edits and removes them by id lists, and sets a
// show's seasons in one season pass. These handlers answer those bodies,
// reusing the single add, edit and delete paths.

// The caps keep one bulk call from holding the Library for long. Kometa
// sends up to 100 adds per call, far below both.
const (
	importMax = 500
	editorMax = 1000
)

// movieEditorIn is the movie editor body: the ids to change, the same
// fields a single PUT takes, and how the tags are applied.
type movieEditorIn struct {
	MovieIDs []int64 `json:"movieIds"`
	movieIn
	ApplyTags string `json:"applyTags"`
}

// seriesEditorIn is the show editor body, the same shape as the movie
// one under the Sonarr names.
type seriesEditorIn struct {
	SeriesIDs []int64 `json:"seriesIds"`
	seriesIn
	ApplyTags string `json:"applyTags"`
}

// editorRemoveIn is the bulk delete body. Each side reads its own id
// list; the exclusion flags mean the same thing under either name.
type editorRemoveIn struct {
	MovieIDs               []int64 `json:"movieIds"`
	SeriesIDs              []int64 `json:"seriesIds"`
	DeleteFiles            bool    `json:"deleteFiles"`
	AddImportExclusion     bool    `json:"addImportExclusion"`
	AddImportListExclusion bool    `json:"addImportListExclusion"`
}

// seasonPassIn is the season pass body: per show the keep-up-to-date
// switch and season switches, plus one monitoring choice for all of them.
type seasonPassIn struct {
	Series []struct {
		ID int64 `json:"id"`
		// Monitored may be left out to change only seasons, as in Sonarr.
		Monitored *bool          `json:"monitored"`
		Seasons   []seasonSwitch `json:"seasons"`
	} `json:"series"`
	MonitoringOptions struct {
		Monitor string `json:"monitor"`
	} `json:"monitoringOptions"`
}

// profileIDs lists the Pick rules ids this side answers as quality
// profiles, for checking a bulk add before anything is written.
func (h *handler) profileIDs(ctx context.Context) (map[int64]bool, error) {
	ps, _, err := h.listedProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(ps))
	for _, p := range ps {
		out[p.ID] = true
	}
	return out, nil
}

// checkProfile refuses a Pick rules id this side does not list. Zero
// means the folder's default, which the add itself picks.
func checkProfile(id looseID, known map[int64]bool) []failure {
	if id < 0 {
		return []failure{{PropertyName: "QualityProfileId", ErrorMessage: "send the Pick rules id as a number"}}
	}
	if id > 0 && !known[int64(id)] {
		return []failure{{PropertyName: "QualityProfileId", ErrorMessage: library.ErrNoPickRules.Error(), AttemptedValue: int64(id)}}
	}
	return nil
}

// importMovies adds movies from a JSON array, as Kometa sends them. Every
// item's id, Pick rules and root folder are checked first, so one wrong
// item refuses the whole call before anything is added. Items already in
// the Library, on the Never-add list or not found at the metadata server
// are skipped, as Radarr's import does; the answer lists only the adds.
func (h *handler) importMovies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var ins []movieIn
	if !decodeUpTo(w, r, &ins, importBodyMax) {
		return
	}
	if len(ins) > importMax {
		message(w, http.StatusBadRequest, fmt.Sprintf("send at most %d movies at once", importMax))
		return
	}
	known, err := h.profileIDs(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	var fs []failure
	for _, in := range ins {
		if in.TmdbID <= 0 {
			fs = append(fs, failure{PropertyName: "TmdbId", ErrorMessage: "send the movie's TMDB id", AttemptedValue: in.TmdbID})
		}
		fs = append(fs, checkProfile(in.QualityProfileID, known)...)
		if folder, ok := h.folderByPath(ctx, in.RootFolderPath); !ok {
			fs = append(fs, failure{PropertyName: "RootFolderPath", ErrorMessage: "no movie Library folder has this path", AttemptedValue: in.RootFolderPath})
		} else if _, _, f, err := h.keptFolder(ctx, folder, in.RootFolderPath, in.Path); err != nil {
			internalError(w, err)
			return
		} else if f != nil {
			fs = append(fs, *f)
		}
		if f := badKeep(in.Monitored); f != nil {
			fs = append(fs, *f)
		}
	}
	if len(fs) > 0 {
		refuse(w, fs...)
		return
	}
	out := []movie{}
	for _, in := range ins {
		id, f, err := h.addMovieOne(ctx, in, actor(r))
		if err != nil {
			internalError(w, err)
			return
		}
		if f != nil {
			// Past the checks above, a refusal skips that one item, as
			// Radarr's and Sonarr's import do; the adds already made stay
			// and are answered.
			if !f.skip {
				log.Printf("arrapi: bulk add skipped %s: %s", f.PropertyName, f.ErrorMessage)
			}
			continue
		}
		m, _, err := h.oneMovie(ctx, id, false)
		if err != nil {
			internalError(w, err)
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

// importSeries adds shows from a JSON array, as Kometa sends them. The
// checks and skips are the movie import's, under the Sonarr names.
func (h *handler) importSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var ins []seriesIn
	if !decodeUpTo(w, r, &ins, importBodyMax) {
		return
	}
	if len(ins) > importMax {
		message(w, http.StatusBadRequest, fmt.Sprintf("send at most %d shows at once", importMax))
		return
	}
	known, err := h.profileIDs(ctx)
	if err != nil {
		internalError(w, err)
		return
	}
	var fs []failure
	for _, in := range ins {
		if in.TvdbID <= 0 {
			fs = append(fs, failure{PropertyName: "TvdbId", ErrorMessage: "send the show's TVDB id", AttemptedValue: in.TvdbID})
		}
		fs = append(fs, checkProfile(in.QualityProfileID, known)...)
		if folder, ok := h.folderByPath(ctx, in.RootFolderPath); !ok {
			fs = append(fs, failure{PropertyName: "RootFolderPath", ErrorMessage: "no TV show Library folder has this path", AttemptedValue: in.RootFolderPath})
		} else if _, _, f, err := h.keptFolder(ctx, folder, in.RootFolderPath, in.Path); err != nil {
			internalError(w, err)
			return
		} else if f != nil {
			fs = append(fs, *f)
		}
	}
	if len(fs) > 0 {
		refuse(w, fs...)
		return
	}
	out := []series{}
	for _, in := range ins {
		id, f, err := h.addSeriesOne(ctx, in, actor(r))
		if err != nil {
			internalError(w, err)
			return
		}
		if f != nil {
			// Past the checks above, a refusal skips that one item, as
			// Radarr's and Sonarr's import do; the adds already made stay
			// and are answered.
			if !f.skip {
				log.Printf("arrapi: bulk add skipped %s: %s", f.PropertyName, f.ErrorMessage)
			}
			continue
		}
		s, _, err := h.oneSeries(ctx, id)
		if err != nil {
			internalError(w, err)
			return
		}
		out = append(out, s)
	}
	writeJSON(w, http.StatusOK, out)
}

// editorProfileOK checks an editor's Pick rules id once, before any title
// changes, so a wrong one never leaves part of the list changed. No id
// leaves the Pick rules as they are.
func (h *handler) editorProfileOK(w http.ResponseWriter, r *http.Request, id looseID) bool {
	if id == 0 {
		return true
	}
	known, err := h.profileIDs(r.Context())
	if err != nil {
		internalError(w, err)
		return false
	}
	if fs := checkProfile(id, known); len(fs) > 0 {
		refuse(w, fs...)
		return false
	}
	return true
}

// tagApply reads the editor's applyTags. A missing one means add, as in
// the old apps; a word it does not know refuses the call.
func tagApply(w http.ResponseWriter, apply string) (string, bool) {
	switch strings.ToLower(apply) {
	case "", "add":
		return "add", true
	case "remove":
		return "remove", true
	case "replace":
		return "replace", true
	}
	refuse(w, failure{PropertyName: "ApplyTags", ErrorMessage: "applyTags must be add, remove or replace", AttemptedValue: apply})
	return "", false
}

// applyTagsTo puts the editor's tags on a title the way applyTags says.
func applyTagsTo(ctx context.Context, st *database.Store, titleID int64, ids []int64, apply string) error {
	switch apply {
	case "remove":
		return unlinkTags(ctx, st, titleID, ids)
	case "replace":
		return setTags(ctx, st, titleID, ids)
	}
	return linkTags(ctx, st, titleID, ids)
}

// editMoviesBulk changes movies by id list, as Kometa sends them. Unknown
// or removed ids are skipped. The folder and moveFiles stay ignored, as
// the single PUT ignores them. Tags follow applyTags: add (the default),
// remove or replace.
func (h *handler) editMoviesBulk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in movieEditorIn
	if !decode(w, r, &in) {
		return
	}
	if len(in.MovieIDs) > editorMax {
		message(w, http.StatusBadRequest, fmt.Sprintf("send at most %d ids at once", editorMax))
		return
	}
	if f := badKeep(in.Monitored); f != nil {
		refuse(w, *f)
		return
	}
	if !h.editorProfileOK(w, r, in.QualityProfileID) {
		return
	}
	apply, ok := tagApply(w, in.ApplyTags)
	if !ok {
		return
	}
	out := []movie{}
	for _, id := range in.MovieIDs {
		t, err := h.title(ctx, id, "movie")
		if errors.Is(err, database.ErrNotFound) {
			continue
		}
		if err != nil {
			internalError(w, err)
			return
		}
		if f, err := h.editMovie(ctx, t, in.movieIn, actor(r)); f != nil {
			refuse(w, *f)
			return
		} else if err != nil {
			internalError(w, err)
			return
		}
		// No tags field leaves the tags alone: replace with nothing sent
		// would otherwise take every tag off.
		if in.Tags != nil {
			if err := applyTagsTo(ctx, h.Store, t.ID, tagIDs(in.Tags), apply); err != nil {
				internalError(w, err)
				return
			}
		}
		m, _, err := h.oneMovie(ctx, id, false)
		if err != nil {
			internalError(w, err)
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusAccepted, out)
}

// editSeriesBulk changes shows by id list, as Kometa sends them. The
// skips and ignored fields are the movie editor's, under the Sonarr
// names; seasonFolder and seriesType stay ignored too.
func (h *handler) editSeriesBulk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in seriesEditorIn
	if !decode(w, r, &in) {
		return
	}
	if len(in.SeriesIDs) > editorMax {
		message(w, http.StatusBadRequest, fmt.Sprintf("send at most %d ids at once", editorMax))
		return
	}
	if !h.editorProfileOK(w, r, in.QualityProfileID) {
		return
	}
	apply, ok := tagApply(w, in.ApplyTags)
	if !ok {
		return
	}
	out := []series{}
	for _, id := range in.SeriesIDs {
		t, err := h.title(ctx, id, "show")
		if errors.Is(err, database.ErrNotFound) {
			continue
		}
		if err != nil {
			internalError(w, err)
			return
		}
		if f, err := h.editSeries(ctx, t, in.seriesIn, actor(r)); f != nil {
			refuse(w, *f)
			return
		} else if err != nil {
			internalError(w, err)
			return
		}
		// No tags field leaves the tags alone: replace with nothing sent
		// would otherwise take every tag off.
		if in.Tags != nil {
			if err := applyTagsTo(ctx, h.Store, t.ID, tagIDs(in.Tags), apply); err != nil {
				internalError(w, err)
				return
			}
		}
		s, _, err := h.oneSeries(ctx, id)
		if err != nil {
			internalError(w, err)
			return
		}
		out = append(out, s)
	}
	writeJSON(w, http.StatusAccepted, out)
}

// deleteEditor takes titles out of the Library by id list, as Kometa
// sends it. Files go only when the delete-files setting allows it, and
// an exclusion flag puts the title on the Never-add list, exactly as the
// single DELETE does. Gone ids are skipped. When any title could not be
// removed the rest still go, then the answer says how many failed and
// the first reason.
func (h *handler) deleteEditor(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in editorRemoveIn
	if !decode(w, r, &in) {
		return
	}
	ids := in.MovieIDs
	if h.side == "sonarr" {
		ids = in.SeriesIDs
	}
	if len(ids) > editorMax {
		message(w, http.StatusBadRequest, fmt.Sprintf("send at most %d ids at once", editorMax))
		return
	}
	c := library.RemoveChoice{NeverAdd: in.AddImportExclusion || in.AddImportListExclusion}
	failed, first := 0, ""
	for _, id := range ids {
		// The setting is read again for each title, so turning it off
		// while a long list runs keeps the files of the titles left.
		c.DeleteFiles = in.DeleteFiles && h.DeleteFiles != nil && h.DeleteFiles(ctx)
		err := h.removeTitle(ctx, id, c, actor(r))
		switch {
		case errors.Is(err, database.ErrNotFound), errors.Is(err, library.ErrAlreadyRemoved):
			// A gone id is already removed, so it is skipped.
		case err != nil:
			log.Printf("arrapi: remove title %d: %v", id, err)
			failed++
			if first == "" {
				first = removeReason(err)
			}
		}
	}
	if failed > 0 {
		message(w, http.StatusConflict, fmt.Sprintf("%d of %d titles could not be removed: %s", failed, len(ids), first))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// passSeasons turns a season pass's monitoring choice into season switches
// for a show's seasons. ok is false for a choice that works episode by
// episode (future, missing, existing, recent, pilot): HolyArrs keeps whole
// seasons, so that choice is refused rather than half done. "skip" and no
// choice switch no season.
func passSeasons(monitor string, parts []mediafiles.Part) ([]seasonSwitch, bool) {
	var nums []int
	for _, p := range parts {
		if p.Number != nil {
			nums = append(nums, *p.Number)
		}
	}
	slices.Sort(nums)
	first, last := -1, -1
	for _, n := range nums {
		if n > 0 && first < 0 {
			first = n
		}
		if n > 0 {
			last = n
		}
	}
	var out []seasonSwitch
	set := func(want func(n int) (on, touch bool)) {
		for _, n := range nums {
			if on, touch := want(n); touch {
				out = append(out, seasonSwitch{SeasonNumber: n, Monitored: on})
			}
		}
	}
	switch strings.ToLower(strings.TrimSpace(monitor)) {
	case "", "skip":
	case "all":
		set(func(n int) (bool, bool) { return true, n != 0 }) // the specials stay as they are
	case "none":
		set(func(int) (bool, bool) { return false, true })
	case "firstseason":
		set(func(n int) (bool, bool) { return n == first, true })
	case "lastseason", "latestseason":
		set(func(n int) (bool, bool) { return n == last, true })
	case "monitorspecials":
		set(func(n int) (bool, bool) { return true, n == 0 })
	case "unmonitorspecials":
		set(func(n int) (bool, bool) { return false, n == 0 })
	default:
		return nil, false
	}
	return out, true
}

// seasonPass sets shows' keep-up-to-date switches and seasons in one
// call, as Kometa sends it. The monitoring choices that pick whole seasons
// are kept (all, none, firstSeason, latestSeason, the specials ones); one
// that picks single episodes is refused before anything changes. Unknown
// ids are skipped.
func (h *handler) seasonPass(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in seasonPassIn
	if !decode(w, r, &in) {
		return
	}
	if len(in.Series) > editorMax {
		message(w, http.StatusBadRequest, fmt.Sprintf("send at most %d shows at once", editorMax))
		return
	}
	if _, ok := passSeasons(in.MonitoringOptions.Monitor, nil); !ok {
		refuse(w, failure{PropertyName: "Monitor", ErrorMessage: "HolyArrs keeps whole seasons: use all, none, firstSeason, latestSeason, monitorSpecials or unmonitorSpecials", AttemptedValue: in.MonitoringOptions.Monitor})
		return
	}
	by := actor(r)
	for _, s := range in.Series {
		t, err := h.title(ctx, s.ID, "show")
		if errors.Is(err, database.ErrNotFound) {
			continue
		}
		if err != nil {
			internalError(w, err)
			return
		}
		if s.Monitored != nil && *s.Monitored != t.KeepUpToDate {
			if _, err := h.Library.Edit(ctx, t.ID, library.TitleChange{KeepUpToDate: s.Monitored}, false, by); err != nil {
				internalError(w, err)
				return
			}
		}
		parts, err := h.seasonParts(ctx, t.ID)
		if err != nil {
			internalError(w, err)
			return
		}
		if err := h.switchSeasons(ctx, t.ID, parts, s.Seasons, by); err != nil {
			internalError(w, err)
			return
		}
		// Read again: the sent seasons may have just switched some.
		if parts, err = h.seasonParts(ctx, t.ID); err != nil {
			internalError(w, err)
			return
		}
		seasons, _ := passSeasons(in.MonitoringOptions.Monitor, parts)
		if err := h.switchSeasons(ctx, t.ID, parts, seasons, by); err != nil {
			internalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]any{})
}

package library

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// OrganiseDeps is what the Organise routes need.
type OrganiseDeps struct {
	Store *database.Store
	Lib   *Service
	Queue *jobs.Queue
	// TitleID reads the {id} Title, answering 404 itself.
	TitleID func(w http.ResponseWriter, r *http.Request) (mediafiles.Title, bool)
	// Fail answers a library error.
	Fail func(w http.ResponseWriter, err error)
}

// OrganiseHandlers serve the Organise routes (stage 5.1): naming and rename,
// adopting an existing library, deleting a movie, and where files go.
type OrganiseHandlers struct {
	OrganiseDeps
}

// Routes registers the Organise routes.
func (h *OrganiseHandlers) Routes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/rename", h.previewTitleRename)
	m.HandleFunc("POST /api/v1/titles/{id}/rename", h.renameTitle)
	m.HandleFunc("GET /api/v1/rename", h.previewRename)
	m.HandleFunc("POST /api/v1/rename", h.renameAll)
	m.HandleFunc("GET /api/v1/jobs/{id}", h.getJob)
	m.HandleFunc("GET /api/v1/settings/naming", h.getNaming)
	m.HandleFunc("PUT /api/v1/settings/naming", h.putNaming)
	m.HandleFunc("POST /api/v1/settings/naming/example", h.namingExample)
	m.HandleFunc("GET /api/v1/settings/files", h.getFileRules)
	m.HandleFunc("PUT /api/v1/settings/files", h.putFileRules)
	m.HandleFunc("POST /api/v1/titles/{id}/delete", h.deleteTitle)
	m.HandleFunc("POST /api/v1/holding/clean", h.cleanHolding)
	m.HandleFunc("POST /api/v1/library-folders/{id}/scan", h.scanFolder)
	m.HandleFunc("POST /api/v1/library-folders/{id}/adopt", h.adoptFolders)
	m.HandleFunc("GET /api/v1/titles/{id}/leftovers", h.listLeftovers)
	m.HandleFunc("POST /api/v1/titles/{id}/leftovers/match", h.matchLeftovers)
	m.HandleFunc("POST /api/v1/titles/{id}/leftovers/unmatch", h.unmatchLeftovers)
	m.HandleFunc("PUT /api/v1/titles/{id}/leftovers/{fid}", h.dismissLeftover)
	m.HandleFunc("POST /api/v1/titles/{id}/rescan", h.rescanTitle)
}

// scanFolder queues the dry-run scan of a Library folder; GET
// /api/v1/jobs/{id} gives its plan when done.
func (h *OrganiseHandlers) scanFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		After string `json:"after"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &body, `send nothing to scan from the start, or {"after": "<the plan's next>"} to scan the rest`) {
		return
	}
	jobID, err := h.Lib.ScanFolder(r.Context(), id, body.After, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "dry_run": true})
}

func (h *OrganiseHandlers) adoptFolders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Folders []AdoptChoice `json:"folders"`
		Follow  *bool         `json:"follow"` // the importer follows what is brought in (default true)
	}
	if !webserver.Decode(w, r, &body, `send {"folders": [{"folder": "<folder name>", "tmdb_id": "<id>"}, ...]}, or "id": "<source>:<id>" for a show folder`) {
		return
	}
	if len(body.Folders) == 0 || len(body.Folders) > 5000 {
		webserver.Fail(w, http.StatusBadRequest, "send 1 to 5000 folders to adopt")
		return
	}
	for _, c := range body.Folders {
		if c.Folder == "" || (c.TMDB == "" && c.ID == "") {
			webserver.Fail(w, http.StatusBadRequest, "every folder needs its folder name and tmdb_id (a movie) or id (a show, such as tvdb:81189, or an artist, such as mbid:<id>)")
			return
		}
	}
	jobID, err := h.Lib.AdoptLater(r.Context(), id, body.Folders, body.Follow == nil || *body.Follow, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		h.Fail(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID})
}

// deleteTitle answers with the plan of a delete (apply false, the default)
// or carries it out (apply true).
func (h *OrganiseHandlers) deleteTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	var body struct {
		RemoveChoice
		RemoveScope
		Apply bool `json:"apply"`
	}
	if r.ContentLength != 0 && !webserver.Decode(w, r, &body, `send {"delete_files": false, "never_add": false, "apply": false} to see the plan, then "apply": true; for part of a show add "season": <n> or "episode_ids": [<id>, ...]; for part of an artist "album_id": <id> or "track_ids": [<id>, ...]`) {
		return
	}
	if err := body.RemoveScope.Check(); err != nil {
		webserver.Fail(w, http.StatusBadRequest, "send one of season, episode_ids, album_id or track_ids, not several")
		return
	}
	if len(body.EpisodeIDs) > 5000 || len(body.TrackIDs) > 5000 {
		webserver.Fail(w, http.StatusBadRequest, "send at most 5000 episode_ids or track_ids")
		return
	}
	var p RemovePlan
	var err error
	if body.RemoveScope.Whole() {
		p, err = h.Lib.Remove(r.Context(), t.ID, body.RemoveChoice, body.Apply, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	} else {
		p, err = h.Lib.RemoveEpisodes(r.Context(), t.ID, body.RemoveScope, body.RemoveChoice, body.Apply, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	}
	switch {
	case errors.Is(err, ErrNotShow), errors.Is(err, ErrNeverAddWholeShow), errors.Is(err, ErrNoEpisodesChosen),
		errors.Is(err, ErrNoSuchEpisode), errors.Is(err, ErrNoSuchSeason), errors.Is(err, ErrNotArtist),
		errors.Is(err, ErrNeverAddWholeArt), errors.Is(err, ErrNoSuchAlbumHere), errors.Is(err, ErrNoSuchTrack):
		webserver.Fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrAlreadyRemoved):
		webserver.Fail(w, http.StatusConflict, err.Error())
	case err != nil && p.TitleID != 0:
		webserver.WriteJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "plan": p})
	case err != nil:
		webserver.Internal(w, "api", err)
	default:
		webserver.WriteJSON(w, http.StatusOK, p)
	}
}

func (h *OrganiseHandlers) cleanHolding(w http.ResponseWriter, r *http.Request) {
	id, _, err := h.Queue.Enqueue(r.Context(), KindCleanHolding, nil, nil, KindCleanHolding, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (h *OrganiseHandlers) previewTitleRename(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	steps, hash, err := PlanRenameHashed(r.Context(), h.Store.Read(), []int64{t.ID})
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"steps": steps, "plan_hash": hash})
}

func (h *OrganiseHandlers) renameTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := h.TitleID(w, r)
	if !ok {
		return
	}
	var body struct {
		FileIDs  []int64 `json:"file_ids"`
		PlanHash string  `json:"plan_hash"`
	}
	const want = `send {"plan_hash": "<from the preview>"} to rename every file shown in the preview, with "file_ids": [<id>, ...] for some`
	if !webserver.Decode(w, r, &body, want) {
		return
	}
	if body.PlanHash == "" {
		webserver.Fail(w, http.StatusBadRequest, want)
		return
	}
	id, err := h.Lib.Rename(r.Context(), []int64{t.ID}, body.FileIDs, body.PlanHash, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (h *OrganiseHandlers) previewRename(w http.ResponseWriter, r *http.Request) {
	steps, hash, err := PlanRenameHashed(r.Context(), h.Store.Read(), nil)
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"steps": steps, "plan_hash": hash})
}

func (h *OrganiseHandlers) renameAll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TitleIDs []int64 `json:"title_ids"`
		PlanHash string  `json:"plan_hash"`
	}
	const want = `send {"plan_hash": "<from the preview>"} to rename what the whole-Library preview showed, or {"title_ids": [<id>, ...], "plan_hash": "<from the preview>"}`
	if !webserver.Decode(w, r, &body, want) {
		return
	}
	if body.PlanHash == "" {
		webserver.Fail(w, http.StatusBadRequest, want)
		return
	}
	id, err := h.Lib.Rename(r.Context(), body.TitleIDs, nil, body.PlanHash, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1}))
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (h *OrganiseHandlers) getJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var j jobs.Job
	if err := database.Get(r.Context(), h.Store.Read(), "jobs", id, &j); errors.Is(err, database.ErrNotFound) {
		webserver.Fail(w, http.StatusNotFound, "no such job")
		return
	} else if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, j)
}

func (h *OrganiseHandlers) getNaming(w http.ResponseWriter, r *http.Request) {
	ns, err := mediafiles.ReadNaming(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	file, folder := namingExamples(ns)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"naming": ns, "words": mediafiles.Words, "example": file, "example_folder": folder,
		"example_episodes": episodeExamples(ns), "example_music": musicExamples(ns)})
}

// musicExamples names sample tracks by the music patterns: a track, a track
// of a two-disc album, and a track of a various-artists compilation. Its
// {track_artist} is the album's artist, as on every real import until
// track credits are read.
func musicExamples(ns mediafiles.NamingSettings) map[string]string {
	one := mediafiles.TrackFacts{Artist: "The Lab Lanterns", Album: "Small Hours", AlbumType: "album", Year: 2025, Disc: 1, Discs: 1, Track: 3,
		Title: "Harbour Light", Ext: ".flac"}
	two := one
	two.Album, two.Edition, two.Disc, two.Discs, two.Track, two.Title = "Long Night", "deluxe", 2, 2, 4, "Second Wind"
	va := mediafiles.TrackFacts{Artist: "Various Artists", Album: "Lab Mix: Volume 1", AlbumType: "album", Year: 2024, Disc: 1, Discs: 1, Track: 5,
		Title: "Night", Ext: ".mp3"}
	artist := mediafiles.ArtistFolder(ns, one)
	return map[string]string{
		"artist_folder": artist,
		"album_folder":  mediafiles.AlbumFolder(ns, one),
		"track":         mediafiles.TrackPath(ns, artist, one),
		"multi_disc":    mediafiles.TrackPath(ns, artist, two),
		"various":       mediafiles.TrackPath(ns, mediafiles.ArtistFolder(ns, va), va),
	}
}

// episodeExamples names sample episodes by the show patterns: one episode,
// a multi-episode file, a daily show and an anime running number.
func episodeExamples(ns mediafiles.NamingSettings) map[string]string {
	show := mediafiles.NameFacts{Title: "Copper Valley", Year: 2025, Step: "1080p_web", Group: "HOLYARRS", Ext: ".mkv"}
	folder := mediafiles.ShowFolder(ns, show)
	return map[string]string{
		"show_folder": folder,
		"episode":     mediafiles.EpisodePath(ns, folder, mediafiles.EpisodeFacts{NameFacts: show, Order: mediafiles.OrderAired, Season: 1, Episodes: []int{2}, Titles: []string{"The Mill"}}),
		"multi":       mediafiles.EpisodePath(ns, folder, mediafiles.EpisodeFacts{NameFacts: show, Order: mediafiles.OrderAired, Season: 1, Episodes: []int{2, 3}, Titles: []string{"The Mill", "Flood"}}),
		"daily":       mediafiles.EpisodePath(ns, "Night Talk", mediafiles.EpisodeFacts{NameFacts: mediafiles.NameFacts{Title: "Night Talk", Step: "1080p_web", Ext: ".mkv"}, Order: mediafiles.OrderDaily, Season: 2026, Episodes: []int{31}, AirDate: "2026-03-14", Titles: []string{"Guests"}}),
		"anime":       mediafiles.EpisodePath(ns, "Blade Song", mediafiles.EpisodeFacts{NameFacts: mediafiles.NameFacts{Title: "Blade Song", Step: "1080p_web", Ext: ".mkv"}, Order: mediafiles.OrderAbsolute, Season: 5, Episodes: []int{12}, Absolute: []int{112}, Titles: []string{"Rain"}}),
		"special":     mediafiles.EpisodePath(ns, folder, mediafiles.EpisodeFacts{NameFacts: show, Order: mediafiles.OrderAired, Season: 0, Episodes: []int{1}, Titles: []string{"Behind the scenes"}}),
	}
}

// namingExamples names one sample movie by the settings, so the screen can
// show what a pattern gives.
func namingExamples(ns mediafiles.NamingSettings) (file, folder string) {
	file = mediafiles.FileName(ns, mediafiles.NameFacts{Title: "Dune: Part Two", Year: 2024, Step: "1080p_bluray", Group: "HOLYARRS", TMDB: "693134",
		IMDB: "tt15239678", Release: "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS", Ext: ".mkv"})
	folder = mediafiles.Folder(ns, mediafiles.NameFacts{Title: "Dune: Part Two", Year: 2024, TMDB: "693134", IMDB: "tt15239678"})
	return file, folder
}

// namingExample shows what unsaved settings would name the sample movie;
// nothing is saved.
func (h *OrganiseHandlers) namingExample(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their saved value.
	ns, err := mediafiles.ReadNaming(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !webserver.Decode(w, r, &ns, `send {"file_pattern","folder_pattern","colon","illegal"}`) {
		return
	}
	if err := ns.Validate(); err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	file, folder := namingExamples(ns)
	webserver.WriteJSON(w, http.StatusOK, map[string]any{"example": file, "example_folder": folder, "example_episodes": episodeExamples(ns), "example_music": musicExamples(ns)})
}

func (h *OrganiseHandlers) putNaming(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their saved value.
	ns, err := mediafiles.ReadNaming(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	if !webserver.Decode(w, r, &ns, `send {"file_pattern","folder_pattern","colon","illegal","rename_on_import"}`) {
		return
	}
	if err := mediafiles.PutNamingSettings(r.Context(), h.Store, ns, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1})); err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	h.getNaming(w, r)
}

func (h *OrganiseHandlers) getFileRules(w http.ResponseWriter, r *http.Request) {
	fr, err := importer.ReadFileRules(r.Context(), h.Store.Read())
	if err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, fr)
}

func (h *OrganiseHandlers) putFileRules(w http.ResponseWriter, r *http.Request) {
	var fr importer.FileRules
	if !webserver.Decode(w, r, &fr, `send {"holding_days","min_free_mb","skip_free_space_check"}`) {
		return
	}
	if err := importer.SaveFileRules(r.Context(), h.Store, fr, webserver.ActorOr(r, database.Actor{Kind: "user", ID: 1})); err != nil {
		webserver.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	webserver.WriteJSON(w, http.StatusOK, fr)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		webserver.Fail(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

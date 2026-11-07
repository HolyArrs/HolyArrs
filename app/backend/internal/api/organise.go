package api

import (
	"errors"
	"net/http"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/importer"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/naming"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// organiseRoutes are the Organise routes (stage 5.1): naming and rename,
// adopting an existing library, deleting a movie, and where files go.
func (a *API) organiseRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/titles/{id}/rename", a.previewTitleRename)
	m.HandleFunc("POST /api/v1/titles/{id}/rename", a.renameTitle)
	m.HandleFunc("GET /api/v1/rename", a.previewRename)
	m.HandleFunc("POST /api/v1/rename", a.renameAll)
	m.HandleFunc("GET /api/v1/jobs/{id}", a.getJob)
	m.HandleFunc("GET /api/v1/settings/naming", a.getNaming)
	m.HandleFunc("PUT /api/v1/settings/naming", a.putNaming)
	m.HandleFunc("POST /api/v1/settings/naming/example", a.namingExample)
	m.HandleFunc("GET /api/v1/settings/files", a.getFileRules)
	m.HandleFunc("PUT /api/v1/settings/files", a.putFileRules)
	m.HandleFunc("POST /api/v1/titles/{id}/delete", a.deleteTitle)
	m.HandleFunc("POST /api/v1/holding/clean", a.cleanHolding)
	m.HandleFunc("POST /api/v1/library-folders/{id}/scan", a.scanFolder)
	m.HandleFunc("POST /api/v1/library-folders/{id}/adopt", a.adoptFolders)
	m.HandleFunc("POST /api/v1/library/rescan", a.rescanLibrary)
}

// scanFolder queues the dry-run scan of a Library folder; GET
// /api/v1/jobs/{id} gives its plan when done.
func (a *API) scanFolder(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		After string `json:"after"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body, `send nothing to scan from the start, or {"after": "<the plan's next>"} to scan the rest`) {
		return
	}
	jobID, err := a.s.Library.ScanFolder(r.Context(), id, body.After, actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID, "dry_run": true})
}

func (a *API) adoptFolders(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Folders []library.AdoptChoice `json:"folders"`
		Follow  *bool                 `json:"follow"` // the importer follows what is brought in (default true)
	}
	if !decode(w, r, &body, `send {"folders": [{"folder": "<folder name>", "tmdb_id": "<id>"}, ...]}, or "id": "<source>:<id>" for a show folder`) {
		return
	}
	if len(body.Folders) == 0 || len(body.Folders) > 5000 {
		fail(w, http.StatusBadRequest, "send 1 to 5000 folders to adopt")
		return
	}
	for _, c := range body.Folders {
		if c.Folder == "" || (c.TMDB == "" && c.ID == "") {
			fail(w, http.StatusBadRequest, "every folder needs its folder name and tmdb_id (a movie) or id (a show, such as tvdb:81189, or an artist, such as mbid:<id>)")
			return
		}
	}
	jobID, err := a.s.Library.AdoptLater(r.Context(), id, body.Folders, body.Follow == nil || *body.Follow, actor(r))
	if err != nil {
		libraryError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": jobID})
}

func (a *API) rescanLibrary(w http.ResponseWriter, r *http.Request) {
	id, _, err := a.s.Queue.Enqueue(r.Context(), library.KindRescan, nil, nil, library.KindRescan, actor(r))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

// deleteTitle answers with the plan of a delete (apply false, the default)
// or carries it out (apply true).
func (a *API) deleteTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var body struct {
		library.RemoveChoice
		library.RemoveScope
		Apply bool `json:"apply"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body, `send {"delete_files": false, "never_add": false, "apply": false} to see the plan, then "apply": true; for part of a show add "season": <n> or "episode_ids": [<id>, ...]; for part of an artist "album_id": <id> or "track_ids": [<id>, ...]`) {
		return
	}
	if err := body.RemoveScope.Check(); err != nil {
		fail(w, http.StatusBadRequest, "send one of season, episode_ids, album_id or track_ids, not several")
		return
	}
	if len(body.EpisodeIDs) > 5000 || len(body.TrackIDs) > 5000 {
		fail(w, http.StatusBadRequest, "send at most 5000 episode_ids or track_ids")
		return
	}
	var p library.RemovePlan
	var err error
	if body.RemoveScope.Whole() {
		p, err = a.s.Library.Remove(r.Context(), t.ID, body.RemoveChoice, body.Apply, actor(r))
	} else {
		p, err = a.s.Library.RemoveEpisodes(r.Context(), t.ID, body.RemoveScope, body.RemoveChoice, body.Apply, actor(r))
	}
	switch {
	case errors.Is(err, library.ErrNotShow), errors.Is(err, library.ErrNeverAddWholeShow), errors.Is(err, library.ErrNoEpisodesChosen),
		errors.Is(err, library.ErrNoSuchEpisode), errors.Is(err, library.ErrNoSuchSeason), errors.Is(err, library.ErrNotArtist),
		errors.Is(err, library.ErrNeverAddWholeArt), errors.Is(err, library.ErrNoSuchAlbumHere), errors.Is(err, library.ErrNoSuchTrack):
		fail(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, library.ErrAlreadyRemoved):
		fail(w, http.StatusConflict, err.Error())
	case err != nil && p.TitleID != 0:
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "plan": p})
	case err != nil:
		internal(w, err)
	default:
		writeJSON(w, http.StatusOK, p)
	}
}

func (a *API) cleanHolding(w http.ResponseWriter, r *http.Request) {
	id, _, err := a.s.Queue.Enqueue(r.Context(), library.KindCleanHolding, nil, nil, library.KindCleanHolding, actor(r))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (a *API) previewTitleRename(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	steps, hash, err := library.PlanRenameHashed(r.Context(), a.s.Store.Read(), []int64{t.ID})
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"steps": steps, "plan_hash": hash})
}

func (a *API) renameTitle(w http.ResponseWriter, r *http.Request) {
	t, ok := a.titleID(w, r)
	if !ok {
		return
	}
	var body struct {
		FileIDs  []int64 `json:"file_ids"`
		PlanHash string  `json:"plan_hash"`
	}
	const want = `send {"plan_hash": "<from the preview>"} to rename every file shown in the preview, with "file_ids": [<id>, ...] for some`
	if !decode(w, r, &body, want) {
		return
	}
	if body.PlanHash == "" {
		fail(w, http.StatusBadRequest, want)
		return
	}
	id, err := a.s.Library.Rename(r.Context(), []int64{t.ID}, body.FileIDs, body.PlanHash, actor(r))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (a *API) previewRename(w http.ResponseWriter, r *http.Request) {
	steps, hash, err := library.PlanRenameHashed(r.Context(), a.s.Store.Read(), nil)
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"steps": steps, "plan_hash": hash})
}

func (a *API) renameAll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TitleIDs []int64 `json:"title_ids"`
		PlanHash string  `json:"plan_hash"`
	}
	const want = `send {"plan_hash": "<from the preview>"} to rename what the whole-Library preview showed, or {"title_ids": [<id>, ...], "plan_hash": "<from the preview>"}`
	if !decode(w, r, &body, want) {
		return
	}
	if body.PlanHash == "" {
		fail(w, http.StatusBadRequest, want)
		return
	}
	id, err := a.s.Library.Rename(r.Context(), body.TitleIDs, nil, body.PlanHash, actor(r))
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job_id": id})
}

func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var j domain.Job
	if err := store.Get(r.Context(), a.s.Store.Read(), "jobs", id, &j); errors.Is(err, store.ErrNotFound) {
		fail(w, http.StatusNotFound, "no such job")
		return
	} else if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (a *API) getNaming(w http.ResponseWriter, r *http.Request) {
	ns, err := naming.Read(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	file, folder := namingExamples(ns)
	writeJSON(w, http.StatusOK, map[string]any{"naming": ns, "words": naming.Words, "example": file, "example_folder": folder,
		"example_episodes": episodeExamples(ns), "example_music": musicExamples(ns)})
}

// musicExamples names sample tracks by the music patterns: a track, a track
// of a two-disc album, and a track of a various-artists compilation. Its
// {track_artist} is the album's artist, as on every real import until
// track credits are read.
func musicExamples(ns naming.Settings) map[string]string {
	one := naming.TrackFacts{Artist: "The Lab Lanterns", Album: "Small Hours", AlbumType: "album", Year: 2025, Disc: 1, Discs: 1, Track: 3,
		Title: "Harbour Light", Ext: ".flac"}
	two := one
	two.Album, two.Edition, two.Disc, two.Discs, two.Track, two.Title = "Long Night", "deluxe", 2, 2, 4, "Second Wind"
	va := naming.TrackFacts{Artist: "Various Artists", Album: "Lab Mix: Volume 1", AlbumType: "album", Year: 2024, Disc: 1, Discs: 1, Track: 5,
		Title: "Night", Ext: ".mp3"}
	artist := naming.ArtistFolder(ns, one)
	return map[string]string{
		"artist_folder": artist,
		"album_folder":  naming.AlbumFolder(ns, one),
		"track":         naming.TrackPath(ns, artist, one),
		"multi_disc":    naming.TrackPath(ns, artist, two),
		"various":       naming.TrackPath(ns, naming.ArtistFolder(ns, va), va),
	}
}

// episodeExamples names sample episodes by the show patterns: one episode,
// a multi-episode file, a daily show and an anime running number.
func episodeExamples(ns naming.Settings) map[string]string {
	show := naming.Facts{Title: "Copper Valley", Year: 2025, Step: "1080p_web", Group: "HOLYARRS", Ext: ".mkv"}
	folder := naming.ShowFolder(ns, show)
	return map[string]string{
		"show_folder": folder,
		"episode":     naming.EpisodePath(ns, folder, naming.EpisodeFacts{Facts: show, Order: naming.OrderAired, Season: 1, Episodes: []int{2}, Titles: []string{"The Mill"}}),
		"multi":       naming.EpisodePath(ns, folder, naming.EpisodeFacts{Facts: show, Order: naming.OrderAired, Season: 1, Episodes: []int{2, 3}, Titles: []string{"The Mill", "Flood"}}),
		"daily":       naming.EpisodePath(ns, "Night Talk", naming.EpisodeFacts{Facts: naming.Facts{Title: "Night Talk", Step: "1080p_web", Ext: ".mkv"}, Order: naming.OrderDaily, Season: 2026, Episodes: []int{31}, AirDate: "2026-03-14", Titles: []string{"Guests"}}),
		"anime":       naming.EpisodePath(ns, "Blade Song", naming.EpisodeFacts{Facts: naming.Facts{Title: "Blade Song", Step: "1080p_web", Ext: ".mkv"}, Order: naming.OrderAbsolute, Season: 5, Episodes: []int{12}, Absolute: []int{112}, Titles: []string{"Rain"}}),
		"special":     naming.EpisodePath(ns, folder, naming.EpisodeFacts{Facts: show, Order: naming.OrderAired, Season: 0, Episodes: []int{1}, Titles: []string{"Behind the scenes"}}),
	}
}

// namingExamples names one sample movie by the settings, so the screen can
// show what a pattern gives.
func namingExamples(ns naming.Settings) (file, folder string) {
	file = naming.File(ns, naming.Facts{Title: "Dune: Part Two", Year: 2024, Step: "1080p_bluray", Group: "HOLYARRS", TMDB: "693134",
		IMDB: "tt15239678", Release: "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS", Ext: ".mkv"})
	folder = naming.Folder(ns, naming.Facts{Title: "Dune: Part Two", Year: 2024, TMDB: "693134", IMDB: "tt15239678"})
	return file, folder
}

// namingExample shows what unsaved settings would name the sample movie;
// nothing is saved.
func (a *API) namingExample(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their saved value.
	ns, err := naming.Read(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	if !decode(w, r, &ns, `send {"file_pattern","folder_pattern","colon","illegal"}`) {
		return
	}
	if err := ns.Validate(); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	file, folder := namingExamples(ns)
	writeJSON(w, http.StatusOK, map[string]any{"example": file, "example_folder": folder, "example_episodes": episodeExamples(ns), "example_music": musicExamples(ns)})
}

func (a *API) putNaming(w http.ResponseWriter, r *http.Request) {
	// Fields left out keep their saved value.
	ns, err := naming.Read(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	if !decode(w, r, &ns, `send {"file_pattern","folder_pattern","colon","illegal","rename_on_import"}`) {
		return
	}
	if err := naming.Save(r.Context(), a.s.Store, ns, actor(r)); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	a.getNaming(w, r)
}

func (a *API) getFileRules(w http.ResponseWriter, r *http.Request) {
	fr, err := importer.ReadFileRules(r.Context(), a.s.Store.Read())
	if err != nil {
		internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, fr)
}

func (a *API) putFileRules(w http.ResponseWriter, r *http.Request) {
	var fr importer.FileRules
	if !decode(w, r, &fr, `send {"holding_days","min_free_mb","skip_free_space_check"}`) {
		return
	}
	if err := importer.SaveFileRules(r.Context(), a.s.Store, fr, actor(r)); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, fr)
}

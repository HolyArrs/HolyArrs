package arrapi

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// movie is a movie as the Radarr side answers it. hasFile, statistics and
// sizeOnDisk appear only on Library reads (GET /movie, /movie/{id}). A
// lookup leaves them out, as Radarr does, because Seerr acts on that
// difference.
type movie struct {
	ID                  int64             `json:"id,omitempty"`
	Title               string            `json:"title"`
	SortTitle           string            `json:"sortTitle,omitempty"`
	AlternateTitles     *[]alternateTitle `json:"alternateTitles,omitempty"`
	OriginalLanguage    *language         `json:"originalLanguage,omitempty"`
	SizeOnDisk          *int64            `json:"sizeOnDisk,omitempty"`
	Status              string            `json:"status"`
	Overview            string            `json:"overview,omitempty"`
	InCinemas           *utc              `json:"inCinemas,omitempty"`
	PhysicalRelease     *utc              `json:"physicalRelease,omitempty"`
	DigitalRelease      *utc              `json:"digitalRelease,omitempty"`
	Images              []image           `json:"images"`
	RemotePoster        string            `json:"remotePoster,omitempty"`
	Year                int               `json:"year"`
	HasFile             *bool             `json:"hasFile,omitempty"`
	Path                string            `json:"path,omitempty"`
	QualityProfileID    int64             `json:"qualityProfileId"`
	Monitored           bool              `json:"monitored"`
	MinimumAvailability string            `json:"minimumAvailability"`
	IsAvailable         bool              `json:"isAvailable"`
	Runtime             int               `json:"runtime"`
	TmdbID              int64             `json:"tmdbId"`
	ImdbID              string            `json:"imdbId,omitempty"`
	TitleSlug           string            `json:"titleSlug"`
	RootFolderPath      string            `json:"rootFolderPath,omitempty"`
	Genres              []string          `json:"genres"`
	Tags                []int64           `json:"tags"`
	Added               utc               `json:"added"`
	MovieFileID         int64             `json:"movieFileId"`
	MovieFile           *mediaFile        `json:"movieFile,omitempty"`
	Statistics          *movieStats       `json:"statistics,omitempty"`
}

type movieStats struct {
	MovieFileCount int      `json:"movieFileCount"`
	SizeOnDisk     int64    `json:"sizeOnDisk"`
	ReleaseGroups  []string `json:"releaseGroups"`
}

// mediaFile is the main Version's file of a movie or an episode.
type mediaFile struct {
	ID           int64          `json:"id"`
	MovieID      int64          `json:"movieId,omitempty"`
	SeriesID     int64          `json:"seriesId,omitempty"`
	SeasonNumber *int           `json:"seasonNumber,omitempty"`
	RelativePath string         `json:"relativePath"`
	Path         string         `json:"path"`
	Size         int64          `json:"size"`
	DateAdded    utc            `json:"dateAdded"`
	SceneName    string         `json:"sceneName,omitempty"`
	ReleaseGroup string         `json:"releaseGroup,omitempty"`
	Languages    []language     `json:"languages"`
	Quality      map[string]any `json:"quality"`
	MediaInfo    map[string]any `json:"mediaInfo,omitempty"`
}

// The release stages map HolyArrs' Release gate to the old apps' names
// and back. The lowest stage, tba, has no gate of its own, so it maps to
// announced, which waits the least.
func availability(gate string) string {
	switch gate {
	case library.GateAnnounced:
		return "announced"
	case library.GateInCinemas:
		return "inCinemas"
	}
	return "released"
}

func gateOf(stage string) string {
	switch strings.ToLower(stage) {
	case "tba", "announced":
		return library.GateAnnounced
	case "incinemas":
		return library.GateInCinemas
	case "released":
		return library.GateReleased
	}
	return ""
}

// movieStatus says how far the movie has come out, by its dates.
func movieStatus(cinema, digital, physical *time.Time, now time.Time) string {
	past := func(t *time.Time) bool { return t != nil && !t.After(now) }
	switch {
	case past(digital) || past(physical):
		return "released"
	case past(cinema):
		return "inCinemas"
	case cinema != nil || digital != nil || physical != nil:
		return "announced"
	}
	return "tba"
}

// fileWidths turn a stored vertical resolution into width-by-height text.
// Callers split it on "x", and Seerr reads a width of 2000 or more as 4K.
var fileWidths = map[int]int{2160: 3840, 1440: 2560, 1080: 1920, 720: 1280, 576: 720, 480: 640}

// sourceNames turn the parser's source keys into the old apps' quality
// names. An unlisted key passes through as it is.
var sourceNames = map[string]string{"web_dl": "WEBDL", "web_rip": "WEBRip", "tv": "HDTV", "bluray": "Bluray", "remux": "Bluray",
	"dvd": "DVD", "cam": "CAM", "telesync": "TELESYNC", "disc_image": "BR-DISK"}

// mediaFileOf builds a main Version's file. The stored path is relative
// to the Library folder root, while relativePath in the answer is
// relative to the title folder. The quality name is source-resolution,
// such as WEBDL-1080p, because Bazarr splits it on the dash.
func mediaFileOf(f mediafiles.File, root, folder string) *mediaFile {
	mf := &mediaFile{ID: f.ID, RelativePath: strings.TrimPrefix(f.RelativePath, folder+"/"),
		Path: filepath.Join(root, filepath.FromSlash(f.RelativePath)), Size: f.Size, DateAdded: utc(f.AddedAt), Languages: languagesOf(f.Languages)}
	if f.ReleaseName != nil {
		mf.SceneName = *f.ReleaseName
	}
	if f.Group != nil {
		mf.ReleaseGroup = *f.Group
	}
	info := map[string]any{}
	name := f.Quality.Source
	if n, ok := sourceNames[name]; ok {
		name = n
	}
	if f.Quality.Resolution > 0 {
		if name != "" {
			name += "-"
		}
		name += strconv.Itoa(f.Quality.Resolution) + "p"
		if w := fileWidths[f.Quality.Resolution]; w > 0 {
			info["resolution"] = strconv.Itoa(w) + "x" + strconv.Itoa(f.Quality.Resolution)
		}
	}
	if f.Quality.Source == "remux" {
		name += " Remux"
	}
	video := f.Quality.Codec
	if video == "" {
		video, _ = f.MediaInfo["video_codec"].(string)
	}
	if video != "" {
		info["videoCodec"] = video
	}
	if f.Quality.Audio != "" {
		info["audioCodec"] = f.Quality.Audio
	}
	if len(info) > 0 {
		mf.MediaInfo = info
	}
	mf.Quality = map[string]any{"quality": map[string]any{"name": name, "resolution": f.Quality.Resolution}}
	return mf
}

// loadMovies reads the Library movies that match where, each with the
// file of its main Version, in a fixed number of queries. With lookup set
// it leaves out what a lookup answer leaves out.
func (h *handler) loadMovies(ctx context.Context, lookup bool, where string, args ...any) ([]movie, error) {
	db := h.Store.Read()
	ts, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind = 'movie' AND state != 'removed' AND "+where+" ORDER BY sort_name, id", args...)
	if err != nil || len(ts) == 0 {
		return []movie{}, err
	}
	ids := make([]int64, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	mains, err := h.mainVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	folders, err := h.folders(ctx)
	if err != nil {
		return nil, err
	}
	fs, err := library.MovieMainFiles(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	fileOf := map[int64]mediafiles.File{}
	for _, f := range fs {
		fileOf[f.VersionID] = f
	}
	tags, err := tagsOf(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	now := h.now()
	out := make([]movie, 0, len(ts))
	for _, t := range ts {
		v := mains[t.ID]
		m := movie{ID: t.ID, Title: t.Name, SortTitle: t.SortName, Status: movieStatus(t.CinemaDate, t.DigitalDate, t.PhysicalDate, now),
			InCinemas: utcPtr(t.CinemaDate), PhysicalRelease: utcPtr(t.PhysicalDate), DigitalRelease: utcPtr(t.DigitalDate),
			QualityProfileID: v.PickRulesID, Monitored: t.KeepUpToDate, MinimumAvailability: availability(t.ReleaseGate),
			IsAvailable: t.GateOpensAt != nil && !t.GateOpensAt.After(now), Runtime: t.RuntimeMinutes, TmdbID: sourceID(t.SourceIDs, "tmdb"),
			ImdbID: t.SourceIDs["imdb"], TitleSlug: t.SourceIDs["tmdb"], Genres: nonNil(t.Genres), Tags: []int64{}, Added: utc(t.CreatedAt),
			Images: h.libraryImages(ctx, t), AlternateTitles: ptr(alternateTitles(t.Aliases)), OriginalLanguage: ptr(originalLanguage(t.OriginalLanguage))}
		if l := tags[t.ID]; l != nil {
			m.Tags = l
		}
		if t.Overview != nil {
			m.Overview = *t.Overview
		}
		if t.Year != nil {
			m.Year = *t.Year
		}
		lf, hasFolder := folders[v.LibraryFolderID]
		if hasFolder {
			m.RootFolderPath = lf.Path
			m.Path = filepath.Join(lf.Path, v.FolderName)
		}
		st := movieStats{ReleaseGroups: []string{}}
		if f, ok := fileOf[v.ID]; ok {
			m.MovieFileID, m.MovieFile = f.ID, mediaFileOf(f, lf.Path, v.FolderName)
			m.MovieFile.MovieID = t.ID
			st.MovieFileCount, st.SizeOnDisk = 1, f.Size
			if f.Group != nil {
				st.ReleaseGroups = []string{*f.Group}
			}
		}
		if !lookup {
			has := st.MovieFileCount > 0
			size := st.SizeOnDisk
			m.HasFile, m.SizeOnDisk, m.Statistics = &has, &size, &st
		}
		out = append(out, m)
	}
	return out, nil
}

// lookupMovieOf builds a movie not in the Library as a lookup answers it:
// no id, not monitored, the lowest stage and no profile.
func (h *handler) lookupMovieOf(m metadata.Movie) movie {
	out := movie{Title: m.Name, Status: movieStatus(m.CinemaDate, m.DigitalDate, m.PhysicalDate, h.now()), Overview: m.Overview,
		InCinemas: utcPtr(m.CinemaDate), PhysicalRelease: utcPtr(m.PhysicalDate), DigitalRelease: utcPtr(m.DigitalDate), Images: []image{},
		RemotePoster: m.Poster, Year: m.Year, MinimumAvailability: "tba", IsAvailable: true, Runtime: m.Runtime, TmdbID: sourceID(m.IDs, "tmdb"),
		ImdbID: m.IDs["imdb"], TitleSlug: m.IDs["tmdb"], Genres: nonNil(m.Genres), Tags: []int64{}, Added: zeroDate}
	if m.Poster != "" {
		out.Images = append(out.Images, image{CoverType: "poster", RemoteURL: m.Poster, URL: m.Poster})
	}
	if m.Backdrop != "" {
		out.Images = append(out.Images, image{CoverType: "fanart", RemoteURL: m.Backdrop, URL: m.Backdrop})
	}
	return out
}

// listMovies lists every movie, or the one with ?tmdbId=. That gives a
// list of zero or one, which callers use as the reliable in-the-Library
// test.
func (h *handler) listMovies(w http.ResponseWriter, r *http.Request) {
	where, args := "1=1", []any{}
	if v := query(r, "tmdbId"); v != "" {
		where, args = "json_extract(source_ids, '$.tmdb') = ?", []any{strconv.FormatInt(atoi64(v), 10)}
	}
	out, err := h.loadMovies(r.Context(), false, where, args...)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) oneMovie(ctx context.Context, id int64, lookup bool) (movie, bool, error) {
	out, err := h.loadMovies(ctx, lookup, "id = ?", id)
	if err != nil || len(out) == 0 {
		return movie{}, false, err
	}
	return out[0], true, nil
}

func (h *handler) getMovie(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	m, found, err := h.oneMovie(r.Context(), id, false)
	switch {
	case err != nil:
		internalError(w, err)
	case !found:
		notFound(w)
	default:
		writeJSON(w, http.StatusOK, m)
	}
}

// lookupMovies finds movies by tmdb: or imdb: id, or by name. A movie in
// the Library comes back as stored, with its id. Any other has no id.
func (h *handler) lookupMovies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	term := strings.TrimSpace(query(r, "term"))
	out := []movie{}
	var ms []metadata.Movie
	low := strings.ToLower(term)
	switch {
	case term == "":
	case strings.HasPrefix(low, "tmdb:") || strings.HasPrefix(low, "imdb:"):
		v := strings.TrimSpace(term[5:])
		if low[:4] == "tmdb" {
			if atoi64(v) <= 0 {
				break // a malformed id finds nothing
			}
			v = strconv.FormatInt(atoi64(v), 10)
			if id, err := library.TitleByTMDB(ctx, h.Store.Read(), v); err != nil {
				internalError(w, err)
				return
			} else if id != 0 {
				if m, found, err := h.oneMovie(ctx, id, true); err != nil {
					internalError(w, err)
					return
				} else if found {
					writeJSON(w, http.StatusOK, []movie{m})
					return
				}
			}
		}
		m, err := h.Metadata.Movie(ctx, low[:5]+v)
		if err != nil && !errors.Is(err, metadata.ErrNotFound) {
			internalError(w, err)
			return
		}
		if err == nil {
			ms = []metadata.Movie{m}
		}
	default:
		var err error
		if ms, err = h.Metadata.Search(ctx, term, 0); err != nil {
			internalError(w, err)
			return
		}
	}
	for _, m := range ms {
		if tmdb := m.IDs["tmdb"]; tmdb != "" {
			id, err := library.TitleByTMDB(ctx, h.Store.Read(), tmdb)
			if err != nil {
				internalError(w, err)
				return
			}
			if id != 0 {
				if lm, found, err := h.oneMovie(ctx, id, true); err != nil {
					internalError(w, err)
					return
				} else if found {
					out = append(out, lm)
					continue
				}
			}
		}
		out = append(out, h.lookupMovieOf(m))
	}
	writeJSON(w, http.StatusOK, out)
}

// movieIn is what an app sends to add or change a movie. Ids may come as
// numbers or as number text.
type movieIn struct {
	ID                  int64     `json:"id"`
	TmdbID              int64     `json:"tmdbId"`
	Title               string    `json:"title"`
	QualityProfileID    looseID   `json:"qualityProfileId"`
	MinimumAvailability string    `json:"minimumAvailability"`
	RootFolderPath      string    `json:"rootFolderPath"`
	Monitored           *bool     `json:"monitored"`
	Tags                []looseID `json:"tags"`
	AddOptions          struct {
		SearchForMovie bool `json:"searchForMovie"`
	} `json:"addOptions"`
}

// addMovie adds a movie and puts the tags sent on it. With searchForMovie
// the search waits for the folder scan and stays within the Release gate,
// because the old apps treat that search as automatic too.
func (h *handler) addMovie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in movieIn
	if !decode(w, r, &in) {
		return
	}
	if in.TmdbID <= 0 {
		refuse(w, failure{PropertyName: "TmdbId", ErrorMessage: "send the movie's TMDB id", AttemptedValue: in.TmdbID})
		return
	}
	if in.QualityProfileID < 0 {
		refuse(w, failure{PropertyName: "QualityProfileId", ErrorMessage: "send the Pick rules id as a number"})
		return
	}
	exists := failure{PropertyName: "TmdbId", ErrorMessage: movieAlreadyAdded, AttemptedValue: in.TmdbID, ErrorCode: "MovieExistsValidator"}
	if id, err := library.TitleByTMDB(ctx, h.Store.Read(), itoa(in.TmdbID)); err != nil {
		internalError(w, err)
		return
	} else if id != 0 {
		refuse(w, exists)
		return
	}
	folder, ok := h.folderByPath(ctx, in.RootFolderPath)
	if !ok {
		refuse(w, failure{PropertyName: "RootFolderPath", ErrorMessage: "no movie Library folder has this path", AttemptedValue: in.RootFolderPath})
		return
	}
	m, err := h.Metadata.Movie(ctx, "tmdb:"+itoa(in.TmdbID))
	if errors.Is(err, metadata.ErrNotFound) {
		refuse(w, failure{PropertyName: "TmdbId", ErrorMessage: "no movie with this TMDB id was found", AttemptedValue: in.TmdbID})
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	// A missing monitored counts as true, as in Radarr.
	keep := in.Monitored == nil || *in.Monitored
	// A missing or unknown gate counts as released, as in Radarr.
	gate := gateOf(in.MinimumAvailability)
	if gate == "" {
		gate = library.GateReleased
	}
	opt := library.AddOptions{LibraryFolderID: folder, PickRulesID: int64(in.QualityProfileID), KeepUpToDate: &keep,
		ReleaseGate: gate, SearchOnAdd: in.AddOptions.SearchForMovie}
	id, _, err := h.Library.Add(ctx, m, opt, actor(r))
	var dup *library.AlreadyAddedError
	switch {
	case errors.As(err, &dup):
		refuse(w, exists)
		return
	case errors.Is(err, library.ErrNeverAdd):
		refuse(w, failure{PropertyName: "TmdbId", ErrorMessage: "this movie is on HolyArrs' Never-add list", AttemptedValue: in.TmdbID})
		return
	case err != nil:
		if f, ok := addFailure(err, int64(in.QualityProfileID), in.RootFolderPath); ok {
			refuse(w, f)
			return
		}
		internalError(w, err)
		return
	}
	if err := linkTags(ctx, h.Store, id, tagIDs(in.Tags)); err != nil {
		internalError(w, err)
		return
	}
	out, _, err := h.oneMovie(ctx, id, false)
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Location", "/"+h.side+"/api/v3/movie/"+itoa(id))
	writeJSON(w, http.StatusCreated, out)
}

// putMovie changes a movie already in the Library. The id comes from the
// path or the body. Only monitored, the quality profile and the minimum
// stage are taken. The folder stays as it is, because moves are done in
// HolyArrs. An app sends a search as its own command.
func (h *handler) putMovie(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in movieIn
	if !decode(w, r, &in) {
		return
	}
	if v := r.PathValue("id"); v != "" {
		in.ID = atoi64(v)
	}
	t, err := h.title(ctx, in.ID, "movie")
	if err != nil {
		internalError(w, err)
		return
	}
	var c library.TitleChange
	if in.Monitored != nil && *in.Monitored != t.KeepUpToDate {
		c.KeepUpToDate = in.Monitored
	}
	if g := gateOf(in.MinimumAvailability); g != "" && g != t.ReleaseGate {
		c.ReleaseGate = &g
	}
	mains, err := h.mainVersions(ctx, []int64{t.ID})
	if err != nil {
		internalError(w, err)
		return
	}
	if p := int64(in.QualityProfileID); p > 0 && p != mains[t.ID].PickRulesID {
		c.PickRulesID = &p
	}
	if c != (library.TitleChange{}) {
		if _, err := h.Library.Edit(ctx, t.ID, c, false, actor(r)); err != nil {
			if f, ok := addFailure(err, int64(in.QualityProfileID), in.RootFolderPath); ok {
				refuse(w, f)
				return
			}
			internalError(w, err)
			return
		}
	}
	out, _, err := h.oneMovie(ctx, t.ID, false)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

package compat

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// series is a show as the Sonarr side answers it. Empty values are left
// out, as the old apps do, except the ones callers expect to be there.
type series struct {
	ID                int64             `json:"id,omitempty"`
	Title             string            `json:"title"`
	SortTitle         string            `json:"sortTitle,omitempty"`
	AlternateTitles   *[]alternateTitle `json:"alternateTitles,omitempty"`
	OriginalLanguage  *language         `json:"originalLanguage,omitempty"`
	Status            string            `json:"status"`
	Ended             bool              `json:"ended"`
	Overview          string            `json:"overview,omitempty"`
	Network           string            `json:"network,omitempty"`
	AirTime           string            `json:"airTime,omitempty"`
	Images            []image           `json:"images"`
	RemotePoster      string            `json:"remotePoster,omitempty"`
	Seasons           []season          `json:"seasons"`
	Year              int               `json:"year"`
	Path              string            `json:"path,omitempty"`
	QualityProfileID  int64             `json:"qualityProfileId"`
	LanguageProfileID int64             `json:"languageProfileId"`
	SeasonFolder      bool              `json:"seasonFolder"`
	Monitored         bool              `json:"monitored"`
	MonitorNewItems   string            `json:"monitorNewItems"`
	UseSceneNumbering bool              `json:"useSceneNumbering"`
	Runtime           int               `json:"runtime"`
	TvdbID            int64             `json:"tvdbId"`
	TvMazeID          int64             `json:"tvMazeId"`
	TmdbID            int64             `json:"tmdbId"`
	ImdbID            string            `json:"imdbId,omitempty"`
	SeriesType        string            `json:"seriesType"`
	TitleSlug         string            `json:"titleSlug"`
	RootFolderPath    string            `json:"rootFolderPath,omitempty"`
	Genres            []string          `json:"genres"`
	Tags              []int64           `json:"tags"`
	Added             utc               `json:"added"`
	NextAiring        *utc              `json:"nextAiring,omitempty"`
	PreviousAiring    *utc              `json:"previousAiring,omitempty"`
	LastAired         *utc              `json:"lastAired,omitempty"`
	Statistics        *seriesCounts     `json:"statistics,omitempty"`
}

type image struct {
	CoverType string `json:"coverType"`
	RemoteURL string `json:"remoteUrl"`
	URL       string `json:"url"`
}

type season struct {
	SeasonNumber int           `json:"seasonNumber"`
	Monitored    bool          `json:"monitored"`
	Statistics   *seasonCounts `json:"statistics,omitempty"`
}

// seasonCounts are one season's numbers, as the Sonarr manual defines
// them: total counts every episode; count those kept up to date that have
// aired, plus every one with a file; files those with a file in the main
// Version. Seerr calls a season available when files reaches total.
type seasonCounts struct {
	NextAiring        *utc     `json:"nextAiring,omitempty"`
	PreviousAiring    *utc     `json:"previousAiring,omitempty"`
	EpisodeFileCount  int      `json:"episodeFileCount"`
	EpisodeCount      int      `json:"episodeCount"`
	TotalEpisodeCount int      `json:"totalEpisodeCount"`
	SizeOnDisk        int64    `json:"sizeOnDisk"`
	ReleaseGroups     []string `json:"releaseGroups"`
	PercentOfEpisodes float64  `json:"percentOfEpisodes"`
	// lastAired is the newest air date gone by, kept up to date or not.
	lastAired *time.Time
}

type seriesCounts struct {
	SeasonCount       int      `json:"seasonCount"`
	EpisodeFileCount  int      `json:"episodeFileCount"`
	EpisodeCount      int      `json:"episodeCount"`
	TotalEpisodeCount int      `json:"totalEpisodeCount"`
	SizeOnDisk        int64    `json:"sizeOnDisk"`
	ReleaseGroups     []string `json:"releaseGroups"`
	PercentOfEpisodes float64  `json:"percentOfEpisodes"`
}

func percent(files, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(files) * 100 / float64(count)
}

// seriesType maps HolyArrs' episode order to the old apps' show type.
func seriesType(order *string) string {
	if order == nil {
		return "standard"
	}
	switch *order {
	case library.OrderAbsolute:
		return "anime"
	case library.OrderDaily:
		return "daily"
	}
	return "standard"
}

// episodeOrder is the other way round: anime shows go by absolute numbers.
func episodeOrder(t string) string {
	switch strings.ToLower(t) {
	case "anime":
		return library.OrderAbsolute
	case "daily":
		return library.OrderDaily
	}
	return library.OrderAired
}

func showStatus(s string) string {
	switch s {
	case "ended":
		return "ended"
	case "announced":
		return "upcoming"
	}
	return "continuing"
}

// idList writes ids as an SQL list; they are numbers, never text.
func idList(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = itoa(id)
	}
	return "(" + strings.Join(parts, ",") + ")"
}

func parseTime(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, err := time.Parse(store.TimeLayout, *s)
	if err != nil {
		return nil
	}
	return &t
}

// seasonCountsOf reads the counts of every season of the shows in ids in
// one grouped query (no query per show), keyed by show and season.
func (h *handler) seasonCountsOf(ctx context.Context, ids []int64) (map[int64]map[int]*seasonCounts, error) {
	now := store.FormatTime(h.now())
	rows, err := h.Store.Read().QueryContext(ctx, `SELECT i.title_id, COALESCE(json_extract(i.numbers, '$.season'), 0) AS sn, count(*),
		sum(s.file_id IS NOT NULL OR (i.keep_up_to_date = 1 AND i.air_date IS NOT NULL AND i.air_date <= ?)),
		sum(s.file_id IS NOT NULL), COALESCE(sum(f.size), 0),
		min(CASE WHEN i.keep_up_to_date = 1 AND i.air_date > ? THEN i.air_date END),
		max(CASE WHEN i.keep_up_to_date = 1 AND i.air_date <= ? THEN i.air_date END),
		COALESCE(group_concat(DISTINCT f.release_group), ''), max(CASE WHEN i.air_date <= ? THEN i.air_date END)
		FROM items i
		LEFT JOIN versions v ON v.title_id = i.title_id AND v.is_main = 1
		LEFT JOIN slots s ON s.item_id = i.id AND s.version_id = v.id
		LEFT JOIN files f ON f.id = s.file_id
		WHERE i.kind = 'episode' AND i.dropped_at IS NULL AND i.title_id IN `+idList(ids)+` GROUP BY i.title_id, sn`, now, now, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[int]*seasonCounts{}
	for rows.Next() {
		var id int64
		var n int
		var c seasonCounts
		var next, prev, last *string
		var groups string
		if err := rows.Scan(&id, &n, &c.TotalEpisodeCount, &c.EpisodeCount, &c.EpisodeFileCount, &c.SizeOnDisk, &next, &prev, &groups, &last); err != nil {
			return nil, err
		}
		c.NextAiring, c.PreviousAiring, c.lastAired = utcPtr(parseTime(next)), utcPtr(parseTime(prev)), parseTime(last)
		c.ReleaseGroups = []string{}
		for _, g := range strings.Split(groups, ",") {
			if g != "" {
				c.ReleaseGroups = append(c.ReleaseGroups, g)
			}
		}
		slices.Sort(c.ReleaseGroups)
		c.PercentOfEpisodes = percent(c.EpisodeFileCount, c.EpisodeCount)
		if out[id] == nil {
			out[id] = map[int]*seasonCounts{}
		}
		out[id][n] = &c
	}
	return out, rows.Err()
}

// loadSeries reads the shows in the Library matching where, with their
// seasons and counts, in a fixed number of queries.
func (h *handler) loadSeries(ctx context.Context, where string, args ...any) ([]series, error) {
	db := h.Store.Read()
	ts, err := store.Select[domain.Title](ctx, db, "titles", "kind = 'show' AND state != 'removed' AND "+where+" ORDER BY sort_name, id", args...)
	if err != nil || len(ts) == 0 {
		return []series{}, err
	}
	ids := make([]int64, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	parts, err := store.Select[domain.Part](ctx, db, "parts", "kind = 'season' AND dropped_at IS NULL AND title_id IN "+idList(ids)+" ORDER BY number")
	if err != nil {
		return nil, err
	}
	seasonsOf := map[int64][]domain.Part{}
	for _, p := range parts {
		seasonsOf[p.TitleID] = append(seasonsOf[p.TitleID], p)
	}
	mains, err := h.mainVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	folders, err := h.folders(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := h.seasonCountsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]series, 0, len(ts))
	for _, t := range ts {
		out = append(out, h.librarySeries(ctx, t, seasonsOf[t.ID], mains[t.ID], folders, counts[t.ID]))
	}
	return out, nil
}

// mainVersions reads the main Version of each title in ids.
func (h *handler) mainVersions(ctx context.Context, ids []int64) (map[int64]domain.Version, error) {
	vs, err := store.Select[domain.Version](ctx, h.Store.Read(), "versions", "is_main = 1 AND title_id IN "+idList(ids))
	out := map[int64]domain.Version{}
	for _, v := range vs {
		out[v.TitleID] = v
	}
	return out, err
}

func (h *handler) folders(ctx context.Context) (map[int64]domain.LibraryFolder, error) {
	fs, err := store.Select[domain.LibraryFolder](ctx, h.Store.Read(), "library_folders", "1=1")
	out := map[int64]domain.LibraryFolder{}
	for _, f := range fs {
		out[f.ID] = f
	}
	return out, err
}

// folderByPath finds this side's Library folder for a rootFolderPath; an
// empty path is the default (0).
func (h *handler) folderByPath(ctx context.Context, p string) (int64, bool) {
	p = strings.TrimRight(strings.TrimSpace(p), `/\`)
	if p == "" {
		return 0, true
	}
	fs, err := store.Select[domain.LibraryFolder](ctx, h.Store.Read(), "library_folders", "kind = ? ORDER BY id", h.folderKind())
	if err != nil {
		return 0, false
	}
	for _, f := range fs {
		if strings.EqualFold(strings.TrimRight(f.Path, `/\`), p) {
			return f.ID, true
		}
	}
	return 0, false
}

// sourceID reads one outside id as a number (0 when unknown).
func sourceID(ids map[string]string, k string) int64 { return atoi64(ids[k]) }

func (h *handler) librarySeries(ctx context.Context, t domain.Title, parts []domain.Part, v domain.Version, folders map[int64]domain.LibraryFolder, counts map[int]*seasonCounts) series {
	s := series{ID: t.ID, Title: t.Name, SortTitle: t.SortName, Status: showStatus(t.Status), Ended: t.Status == "ended",
		AlternateTitles: ptr(alternateTitles(t.Aliases)), OriginalLanguage: ptr(originalLanguage(t.OriginalLanguage)),
		Images: h.libraryImages(ctx, t), Seasons: []season{}, QualityProfileID: v.PickRulesID, LanguageProfileID: 1, SeasonFolder: true,
		Monitored: t.KeepUpToDate, MonitorNewItems: "all", Runtime: t.RuntimeMinutes, TvdbID: sourceID(t.SourceIDs, "tvdb"),
		TvMazeID: sourceID(t.SourceIDs, "tvmaze"), TmdbID: sourceID(t.SourceIDs, "tmdb"), ImdbID: t.SourceIDs["imdb"],
		SeriesType: seriesType(t.EpisodeOrder), TitleSlug: itoa(t.ID), Genres: nonNil(t.Genres), Tags: []int64{}, Added: utc(t.CreatedAt)}
	if t.NewPartsPolicy == "none" {
		s.MonitorNewItems = "none"
	}
	if t.Overview != nil {
		s.Overview = *t.Overview
	}
	if t.Network != nil {
		s.Network = *t.Network
	}
	if t.AirTime != nil {
		s.AirTime = *t.AirTime
	}
	if t.Year != nil {
		s.Year = *t.Year
	}
	if f, ok := folders[v.LibraryFolderID]; ok {
		s.RootFolderPath = f.Path
		s.Path = filepath.Join(f.Path, v.FolderName)
	}
	all := seriesCounts{ReleaseGroups: []string{}}
	var next, prev, last *time.Time
	for _, p := range parts {
		n := 0
		if p.Number != nil {
			n = *p.Number
		}
		se := season{SeasonNumber: n, Monitored: p.KeepUpToDate}
		if n > 0 {
			all.SeasonCount++
		}
		if c := counts[n]; c != nil {
			se.Statistics = c
			all.EpisodeFileCount += c.EpisodeFileCount
			all.EpisodeCount += c.EpisodeCount
			all.TotalEpisodeCount += c.TotalEpisodeCount
			all.SizeOnDisk += c.SizeOnDisk
			for _, g := range c.ReleaseGroups {
				if !slices.Contains(all.ReleaseGroups, g) {
					all.ReleaseGroups = append(all.ReleaseGroups, g)
				}
			}
			if c.NextAiring != nil && (next == nil || time.Time(*c.NextAiring).Before(*next)) {
				x := time.Time(*c.NextAiring)
				next = &x
			}
			if c.PreviousAiring != nil && (prev == nil || time.Time(*c.PreviousAiring).After(*prev)) {
				x := time.Time(*c.PreviousAiring)
				prev = &x
			}
			// The show's last air date leaves the specials out.
			if n > 0 && c.lastAired != nil && (last == nil || c.lastAired.After(*last)) {
				last = c.lastAired
			}
		}
		s.Seasons = append(s.Seasons, se)
	}
	slices.Sort(all.ReleaseGroups)
	all.PercentOfEpisodes = percent(all.EpisodeFileCount, all.EpisodeCount)
	// A single show always carries its counts: Seerr reads them unguarded.
	s.Statistics = &all
	s.NextAiring, s.PreviousAiring, s.LastAired = utcPtr(next), utcPtr(prev), utcPtr(last)
	return s
}

// lookupSeriesOf is a show not in the Library, as a lookup answers it: no
// id, nothing chosen yet, every season but the specials switched on.
func lookupSeriesOf(sh metadata.Show) series {
	s := series{Title: sh.Name, Status: showStatus(sh.Status), Ended: sh.Status == "ended", Overview: sh.Overview, Network: sh.Network,
		AirTime: sh.AirTime, Images: []image{}, Seasons: []season{}, Year: sh.Year, LanguageProfileID: 1, Monitored: true,
		MonitorNewItems: "all", Runtime: sh.Runtime, TvdbID: sourceID(sh.IDs, "tvdb"), TvMazeID: sourceID(sh.IDs, "tvmaze"),
		TmdbID: sourceID(sh.IDs, "tmdb"), ImdbID: sh.IDs["imdb"], SeriesType: "standard", Genres: nonNil(sh.Genres), Tags: []int64{},
		Added: zeroDate, RemotePoster: sh.Poster}
	s.TitleSlug = sh.IDs["tvdb"]
	if s.TitleSlug == "" {
		s.TitleSlug = sh.Source + "-" + sh.IDs[sh.Source]
	}
	if sh.Poster != "" {
		s.Images = append(s.Images, image{CoverType: "poster", RemoteURL: sh.Poster, URL: sh.Poster})
	}
	if sh.Backdrop != "" {
		s.Images = append(s.Images, image{CoverType: "fanart", RemoteURL: sh.Backdrop, URL: sh.Backdrop})
	}
	nums := []int{}
	for _, se := range sh.Seasons {
		nums = append(nums, se.Number)
	}
	for _, e := range sh.Episodes {
		nums = append(nums, e.Season)
	}
	slices.Sort(nums)
	nums = slices.Compact(nums)
	c := seriesCounts{ReleaseGroups: []string{}}
	for _, n := range nums {
		s.Seasons = append(s.Seasons, season{SeasonNumber: n, Monitored: n > 0})
		if n > 0 {
			c.SeasonCount++
		}
	}
	s.Statistics = &c
	return s
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// listSeries is every show, or the one with ?tvdbId= (a list of zero or
// one, never a 404).
func (h *handler) listSeries(w http.ResponseWriter, r *http.Request) {
	where, args := "1=1", []any{}
	if v := query(r, "tvdbId"); v != "" {
		where, args = "json_extract(source_ids, '$.tvdb') = ?", []any{strconv.FormatInt(atoi64(v), 10)}
	}
	out, err := h.loadSeries(r.Context(), where, args...)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// oneSeries reads one show of the Library; false when it is not there.
func (h *handler) oneSeries(ctx context.Context, id int64) (series, bool, error) {
	out, err := h.loadSeries(ctx, "id = ?", id)
	if err != nil || len(out) == 0 {
		return series{}, false, err
	}
	return out[0], true, nil
}

func (h *handler) getSeries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s, found, err := h.oneSeries(r.Context(), id)
	switch {
	case err != nil:
		internalError(w, err)
	case !found:
		notFound(w)
	default:
		writeJSON(w, http.StatusOK, s)
	}
}

// lookupSeries finds shows by tvdb:, tmdb:, imdb: or tvmaze: id, or by
// name. A show already in the Library comes back as the Library has it,
// with its id; any other without one.
func (h *handler) lookupSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	term := strings.TrimSpace(query(r, "term"))
	out := []series{}
	if term == "" {
		writeJSON(w, http.StatusOK, out)
		return
	}
	defs, err := library.ReadShowDefaults(ctx, h.Store.Read())
	if err != nil {
		internalError(w, err)
		return
	}
	var shows []metadata.Show
	kind, v, hasKind := strings.Cut(term, ":")
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "tvdbid" {
		kind = "tvdb"
	}
	if hasKind && (kind == "tvdb" || kind == "tmdb" || kind == "imdb" || kind == "tvmaze") {
		v = strings.TrimSpace(v)
		if kind != "imdb" && atoi64(v) <= 0 {
			writeJSON(w, http.StatusOK, out) // a malformed id finds nothing
			return
		}
		if id, err := library.ShowByIDs(ctx, h.Store.Read(), map[string]string{kind: v}); err != nil {
			internalError(w, err)
			return
		} else if id != 0 {
			if s, found, err := h.oneSeries(ctx, id); err != nil {
				internalError(w, err)
				return
			} else if found {
				writeJSON(w, http.StatusOK, []series{s})
				return
			}
		}
		sh, err := h.Library.LookupShow(ctx, kind+":"+v, defs.Source)
		if err != nil && !errors.Is(err, metadata.ErrShowNotFound) && !errors.Is(err, metadata.ErrNoTVDBKey) && !errors.Is(err, library.ErrNoShowSources) {
			message(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err == nil {
			shows = []metadata.Show{sh}
		}
	} else if h.Library.Shows != nil {
		p, err := h.Library.Shows.Get(defs.Source)
		if err == nil {
			shows, err = p.SearchShows(ctx, term)
		}
		if err != nil {
			message(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	for _, sh := range shows {
		id, err := library.ShowByIDs(ctx, h.Store.Read(), sh.IDs)
		if err != nil {
			internalError(w, err)
			return
		}
		if id != 0 {
			if s, found, err := h.oneSeries(ctx, id); err != nil {
				internalError(w, err)
				return
			} else if found {
				out = append(out, s)
				continue
			}
		}
		out = append(out, lookupSeriesOf(sh))
	}
	writeJSON(w, http.StatusOK, out)
}

// seriesIn is what an app sends to add or change a show; other fields are
// read from the Library or the details source, as the old apps do.
type seriesIn struct {
	ID               int64  `json:"id"`
	TvdbID           int64  `json:"tvdbId"`
	Title            string `json:"title"`
	QualityProfileID int64  `json:"qualityProfileId"`
	Seasons          []struct {
		SeasonNumber int  `json:"seasonNumber"`
		Monitored    bool `json:"monitored"`
	} `json:"seasons"`
	Monitored       *bool  `json:"monitored"`
	MonitorNewItems string `json:"monitorNewItems"`
	RootFolderPath  string `json:"rootFolderPath"`
	SeriesType      string `json:"seriesType"`
	AddOptions      struct {
		SearchForMissingEpisodes bool `json:"searchForMissingEpisodes"`
	} `json:"addOptions"`
}

// addSeries adds a show: the seasons sent as monitored are kept up to
// date, and with searchForMissingEpisodes the show is searched once its
// folder scan is done (so episodes already on disk are not grabbed again).
func (h *handler) addSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in seriesIn
	if !decode(w, r, &in) {
		return
	}
	if in.TvdbID <= 0 {
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "send the show's TVDB id", AttemptedValue: in.TvdbID})
		return
	}
	if id, err := library.ShowByIDs(ctx, h.Store.Read(), map[string]string{"tvdb": itoa(in.TvdbID)}); err != nil {
		internalError(w, err)
		return
	} else if id != 0 {
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "this show is already in the HolyArrs Library", AttemptedValue: in.TvdbID, ErrorCode: "AlreadyInLibrary"})
		return
	}
	folder, ok := h.folderByPath(ctx, in.RootFolderPath)
	if !ok {
		refuse(w, failure{PropertyName: "RootFolderPath", ErrorMessage: "no TV show Library folder has this path", AttemptedValue: in.RootFolderPath})
		return
	}
	// Left out, monitored counts as false, as in the old apps.
	keep := in.Monitored != nil && *in.Monitored
	opt := library.ShowAddOptions{LibraryFolderID: folder, PickRulesID: in.QualityProfileID, KeepUpToDate: &keep,
		NewSeasons: "all", EpisodeOrder: episodeOrder(in.SeriesType), SearchOnAdd: in.AddOptions.SearchForMissingEpisodes}
	if strings.EqualFold(in.MonitorNewItems, "none") {
		opt.NewSeasons = "none"
	}
	if len(in.Seasons) == 0 {
		opt.Seasons = library.SeasonsAll // no list: every season but the specials
	} else {
		opt.SeasonNumbers = []int{}
		for _, se := range in.Seasons {
			if se.Monitored {
				opt.SeasonNumbers = append(opt.SeasonNumbers, se.SeasonNumber)
			}
		}
	}
	id, _, err := h.Library.AddShow(ctx, "tvdb:"+itoa(in.TvdbID), opt, actor(r))
	var dup *library.ShowAlreadyAddedError
	switch {
	case errors.As(err, &dup):
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "this show is already in the HolyArrs Library", AttemptedValue: in.TvdbID, ErrorCode: "AlreadyInLibrary"})
		return
	case errors.Is(err, library.ErrShowNeverAdd):
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "this show is on HolyArrs' Never-add list", AttemptedValue: in.TvdbID})
		return
	case errors.Is(err, metadata.ErrShowNotFound), errors.Is(err, metadata.ErrNoTVDBKey):
		refuse(w, failure{PropertyName: "TvdbId", ErrorMessage: "no show with this TVDB id was found", AttemptedValue: in.TvdbID})
		return
	case err != nil:
		if f, ok := addFailure(err, in.QualityProfileID, in.RootFolderPath); ok {
			refuse(w, f)
			return
		}
		message(w, http.StatusInternalServerError, err.Error())
		return
	}
	s, _, err := h.oneSeries(ctx, id)
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Location", "/"+h.side+"/api/v3/series/"+itoa(id))
	writeJSON(w, http.StatusCreated, s)
}

// addFailure turns the library's refusals of a folder or Pick rules into
// a failed check.
func addFailure(err error, profile int64, folder string) (failure, bool) {
	var fe *library.FolderError
	switch {
	case errors.Is(err, library.ErrNoPickRules), errors.Is(err, library.ErrNotVideoRules):
		return failure{PropertyName: "QualityProfileId", ErrorMessage: err.Error(), AttemptedValue: profile}, true
	case errors.As(err, &fe), errors.Is(err, library.ErrNoShowFolder), errors.Is(err, library.ErrNotShowFolder),
		errors.Is(err, library.ErrNoFolder), errors.Is(err, library.ErrNotMovieFolder):
		return failure{PropertyName: "RootFolderPath", ErrorMessage: err.Error(), AttemptedValue: folder}, true
	}
	return failure{}, false
}

// putSeries changes a show already in the Library. The id comes from the
// path or, as Seerr sends it, from the body. Only the show and season
// switches, the new-season choice and the quality profile are taken;
// switching a season switches all its episodes, as in the old apps.
// Everything else in the body (folder, show type, tags) is left as it is.
func (h *handler) putSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in seriesIn
	if !decode(w, r, &in) {
		return
	}
	if v := r.PathValue("id"); v != "" {
		in.ID = atoi64(v)
	}
	t, err := h.title(ctx, in.ID, "show")
	if err != nil {
		internalError(w, err)
		return
	}
	by := actor(r)
	var c library.TitleChange
	if in.Monitored != nil && *in.Monitored != t.KeepUpToDate {
		c.KeepUpToDate = in.Monitored
	}
	if p := strings.ToLower(in.MonitorNewItems); (p == "all" || p == "none") && p != t.NewPartsPolicy {
		c.NewSeasons = &p
	}
	mains, err := h.mainVersions(ctx, []int64{t.ID})
	if err != nil {
		internalError(w, err)
		return
	}
	if in.QualityProfileID > 0 && in.QualityProfileID != mains[t.ID].PickRulesID {
		c.PickRulesID = &in.QualityProfileID
	}
	if c != (library.TitleChange{}) {
		if _, err := h.Library.Edit(ctx, t.ID, c, false, by); err != nil {
			if f, ok := addFailure(err, in.QualityProfileID, in.RootFolderPath); ok {
				refuse(w, f)
				return
			}
			internalError(w, err)
			return
		}
	}
	parts, err := store.Select[domain.Part](ctx, h.Store.Read(), "parts", "title_id = ? AND kind = 'season'", t.ID)
	if err != nil {
		internalError(w, err)
		return
	}
	for _, se := range in.Seasons {
		for _, p := range parts {
			if p.Number != nil && *p.Number == se.SeasonNumber && p.KeepUpToDate != se.Monitored {
				if err := h.Library.SetSeason(ctx, t.ID, se.SeasonNumber, se.Monitored, by); err != nil {
					internalError(w, err)
					return
				}
			}
		}
	}
	s, _, err := h.oneSeries(ctx, t.ID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, s)
}

// deleteTitle takes a show or movie out of the Library (it stays in the
// database as removed, so its id is never reused). deleteFiles takes its
// files too, only when the setting allows it; they go to the holding
// folder first, like every delete. An import exclusion puts the title on
// the Never-add list. An id that is gone answers 404, which Seerr reads as
// "already removed".
func (h *handler) deleteTitle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	kind := "movie"
	if h.side == "sonarr" {
		kind = "show"
	}
	if _, err := h.title(ctx, id, kind); err != nil {
		internalError(w, err)
		return
	}
	c := library.RemoveChoice{NeverAdd: queryBool(r, "addImportExclusion") || queryBool(r, "addImportListExclusion")}
	c.DeleteFiles = queryBool(r, "deleteFiles") && h.DeleteFiles != nil && h.DeleteFiles(ctx)
	_, err := h.Library.Remove(ctx, id, c, true, actor(r))
	switch {
	case errors.Is(err, library.ErrAlreadyRemoved):
		notFound(w)
	case err != nil:
		message(w, http.StatusConflict, err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]any{})
	}
}

// episode is one episode as the Sonarr side answers it.
type episode struct {
	ID                    int64  `json:"id"`
	SeriesID              int64  `json:"seriesId"`
	TvdbID                int64  `json:"tvdbId"`
	EpisodeFileID         int64  `json:"episodeFileId"`
	SeasonNumber          int    `json:"seasonNumber"`
	EpisodeNumber         int    `json:"episodeNumber"`
	Title                 string `json:"title,omitempty"`
	AirDate               string `json:"airDate,omitempty"`
	AirDateUtc            *utc   `json:"airDateUtc,omitempty"`
	Runtime               int    `json:"runtime"`
	HasFile               bool   `json:"hasFile"`
	Monitored             bool   `json:"monitored"`
	AbsoluteEpisodeNumber *int   `json:"absoluteEpisodeNumber,omitempty"`
	UnverifiedScene       bool   `json:"unverifiedSceneNumbering"`
	// Only when asked for (includeEpisodeFile, includeSeries).
	EpisodeFile *mediaFile `json:"episodeFile,omitempty"`
	Series      *series    `json:"series,omitempty"`
}

// episodesWhere reads the episodes of shows in the Library matching where,
// each with the file of its main Version.
func (h *handler) episodesWhere(ctx context.Context, where string, args ...any) ([]episode, error) {
	its, err := store.Select[domain.Item](ctx, h.Store.Read(), "items", "kind = 'episode' AND dropped_at IS NULL AND title_id IN (SELECT id FROM titles WHERE kind = 'show' AND state != 'removed') AND "+
		where+" ORDER BY title_id, json_extract(numbers, '$.season'), json_extract(numbers, '$.episode')", args...)
	if err != nil || len(its) == 0 {
		return []episode{}, err
	}
	ids := make([]int64, len(its))
	for i, it := range its {
		ids[i] = it.ID
	}
	files := map[int64]int64{}
	rows, err := h.Store.Read().QueryContext(ctx, `SELECT s.item_id, s.file_id FROM slots s JOIN versions v ON v.id = s.version_id AND v.is_main = 1
		WHERE s.file_id IS NOT NULL AND s.item_id IN `+idList(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item, file int64
		if err := rows.Scan(&item, &file); err != nil {
			return nil, err
		}
		files[item] = file
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]episode, 0, len(its))
	for _, it := range its {
		e := episode{ID: it.ID, SeriesID: it.TitleID, TvdbID: sourceID(it.SourceIDs, "tvdb"), EpisodeFileID: files[it.ID],
			AirDateUtc: utcPtr(it.AirDate), Monitored: it.KeepUpToDate, AbsoluteEpisodeNumber: it.Numbers.Absolute}
		e.HasFile = e.EpisodeFileID > 0
		if it.Numbers.Season != nil {
			e.SeasonNumber = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			e.EpisodeNumber = *it.Numbers.Episode
		}
		if it.Name != nil {
			e.Title = *it.Name
		}
		if it.AirDay != nil {
			e.AirDate = *it.AirDay
		}
		if it.Runtime != nil {
			e.Runtime = *it.Runtime
		}
		out = append(out, e)
	}
	return out, nil
}

// idsParam reads a list of ids sent as one comma list or repeated.
func idsParam(r *http.Request, name string) []int64 {
	var out []int64
	for k, vs := range r.URL.Query() {
		if !strings.EqualFold(k, name) {
			continue
		}
		for _, v := range vs {
			for _, x := range strings.Split(v, ",") {
				if n := atoi64(x); n > 0 {
					out = append(out, n)
				}
			}
		}
	}
	return out
}

// listEpisodes needs seriesId (with an optional seasonNumber) or
// episodeIds; with neither it answers 400.
func (h *handler) listEpisodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sid := query(r, "seriesId"); sid != "" {
		t, err := h.title(ctx, atoi64(sid), "show")
		if err != nil {
			internalError(w, err)
			return
		}
		where, args := "title_id = ?", []any{t.ID}
		if sn := query(r, "seasonNumber"); sn != "" {
			n, err := strconv.Atoi(sn)
			if err != nil {
				message(w, http.StatusBadRequest, "seasonNumber must be a number")
				return
			}
			where += " AND json_extract(numbers, '$.season') = ?"
			args = append(args, n)
		}
		out, err := h.episodesWhere(ctx, where, args...)
		if err == nil {
			err = h.include(ctx, r, out)
		}
		if err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if ids := idsParam(r, "episodeIds"); len(ids) > 0 {
		out, err := h.episodesWhere(ctx, "id IN "+idList(ids))
		if err == nil {
			err = h.include(ctx, r, out)
		}
		if err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	message(w, http.StatusBadRequest, "send seriesId or episodeIds")
}

// monitorEpisodes switches episodes on or off. Every id must be an episode
// of a show in the Library; otherwise nothing changes and it answers 404.
func (h *handler) monitorEpisodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		EpisodeIDs []int64 `json:"episodeIds"`
		Monitored  bool    `json:"monitored"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.EpisodeIDs == nil {
		message(w, http.StatusBadRequest, "send episodeIds and monitored")
		return
	}
	if len(in.EpisodeIDs) == 0 {
		writeJSON(w, http.StatusAccepted, []episode{})
		return
	}
	slices.Sort(in.EpisodeIDs)
	in.EpisodeIDs = slices.Compact(in.EpisodeIDs)
	found, err := h.episodesWhere(ctx, "id IN "+idList(in.EpisodeIDs))
	if err != nil {
		internalError(w, err)
		return
	}
	if len(found) != len(in.EpisodeIDs) {
		notFound(w)
		return
	}
	by := actor(r)
	for _, e := range found {
		if e.Monitored == in.Monitored {
			continue
		}
		if _, err := h.Library.SetEpisode(ctx, e.ID, in.Monitored, by); err != nil {
			internalError(w, err)
			return
		}
	}
	out, err := h.episodesWhere(ctx, "id IN "+idList(in.EpisodeIDs))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

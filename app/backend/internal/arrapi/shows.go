package arrapi

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// series is a show as the Sonarr side answers it. Empty values are left
// out, as in the old apps, except the ones callers expect to find.
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

// seasonCounts are one season's numbers as the old apps define them.
// total counts every episode. count covers kept episodes that have aired
// plus every one with a file. files counts those with a file in the main
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
	// lastAired is the newest past air date, kept or not.
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

// episodeOrder maps the other way. Anime shows go by absolute numbers.
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

// idList writes ids as an SQL list. They are numbers, never text, so
// writing them inline is safe.
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
	t, err := time.Parse(database.TimeLayout, *s)
	if err != nil {
		return nil
	}
	return &t
}

// seasonCountsOf reads the counts of every season of the shows in ids,
// keyed by show and season. One grouped query avoids a query per show.
func (h *handler) seasonCountsOf(ctx context.Context, ids []int64) (map[int64]map[int]*seasonCounts, error) {
	counts, err := library.SeasonCounts(ctx, h.Store.Read(), ids, h.now())
	if err != nil {
		return nil, err
	}
	out := map[int64]map[int]*seasonCounts{}
	for _, row := range counts {
		id, n := row.TitleID, row.Season
		var c seasonCounts
		c.TotalEpisodeCount, c.EpisodeCount, c.EpisodeFileCount, c.SizeOnDisk = row.Total, row.Wanted, row.WithFile, row.Size
		c.NextAiring, c.PreviousAiring, c.lastAired = utcPtr(parseTime(row.NextAiring)), utcPtr(parseTime(row.PreviousAired)), parseTime(row.LastAired)
		c.ReleaseGroups = []string{}
		for _, g := range strings.Split(row.ReleaseGroups, ",") {
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
	return out, nil
}

// loadSeries reads the Library shows that match where, with their
// seasons and counts, in a fixed number of queries.
func (h *handler) loadSeries(ctx context.Context, where string, args ...any) ([]series, error) {
	db := h.Store.Read()
	ts, err := database.Select[mediafiles.Title](ctx, db, "titles", "kind = 'show' AND state != 'removed' AND "+where+" ORDER BY sort_name, id", args...)
	if err != nil || len(ts) == 0 {
		return []series{}, err
	}
	ids := make([]int64, len(ts))
	for i, t := range ts {
		ids[i] = t.ID
	}
	parts, err := database.Select[mediafiles.Part](ctx, db, "parts", "kind = 'season' AND dropped_at IS NULL AND title_id IN "+idList(ids)+" ORDER BY number")
	if err != nil {
		return nil, err
	}
	seasonsOf := map[int64][]mediafiles.Part{}
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
func (h *handler) mainVersions(ctx context.Context, ids []int64) (map[int64]mediafiles.Version, error) {
	vs, err := database.Select[mediafiles.Version](ctx, h.Store.Read(), "versions", "is_main = 1 AND title_id IN "+idList(ids))
	out := map[int64]mediafiles.Version{}
	for _, v := range vs {
		out[v.TitleID] = v
	}
	return out, err
}

func (h *handler) folders(ctx context.Context) (map[int64]mediafiles.LibraryFolder, error) {
	fs, err := database.Select[mediafiles.LibraryFolder](ctx, h.Store.Read(), "library_folders", "1=1")
	out := map[int64]mediafiles.LibraryFolder{}
	for _, f := range fs {
		out[f.ID] = f
	}
	return out, err
}

// folderByPath finds this side's Library folder for a rootFolderPath. An
// empty path means the default, 0.
func (h *handler) folderByPath(ctx context.Context, p string) (int64, bool) {
	p = strings.TrimRight(strings.TrimSpace(p), `/\`)
	if p == "" {
		return 0, true
	}
	fs, err := database.Select[mediafiles.LibraryFolder](ctx, h.Store.Read(), "library_folders", "kind = ? ORDER BY id", h.folderKind())
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

// sourceID reads one outside id as a number. It is 0 when unknown.
func sourceID(ids map[string]string, k string) int64 { return atoi64(ids[k]) }

func (h *handler) librarySeries(ctx context.Context, t mediafiles.Title, parts []mediafiles.Part, v mediafiles.Version, folders map[int64]mediafiles.LibraryFolder, counts map[int]*seasonCounts) series {
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
			// Specials do not count toward the show's last air date.
			if n > 0 && c.lastAired != nil && (last == nil || c.lastAired.After(*last)) {
				last = c.lastAired
			}
		}
		s.Seasons = append(s.Seasons, se)
	}
	slices.Sort(all.ReleaseGroups)
	all.PercentOfEpisodes = percent(all.EpisodeFileCount, all.EpisodeCount)
	// A single show always carries its counts, because Seerr reads them
	// without a nil check.
	s.Statistics = &all
	s.NextAiring, s.PreviousAiring, s.LastAired = utcPtr(next), utcPtr(prev), utcPtr(last)
	return s
}

// lookupSeriesOf builds a show not in the Library as a lookup answers it:
// no id, nothing chosen yet and every season but the specials on.
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

// listSeries lists every show, or the one with ?tvdbId=. That gives a
// list of zero or one, never a 404.
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

// oneSeries reads one Library show. It returns false when it is not there.
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
// name. A show already in the Library comes back as stored, with its id.
// Any other has no id.
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
	var list []metadata.Show
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
		sh, err := shows.New(h.Library).LookupShow(ctx, kind+":"+v, defs.Source)
		if err != nil && !errors.Is(err, metadata.ErrShowNotFound) && !errors.Is(err, metadata.ErrNoTVDBKey) && !errors.Is(err, library.ErrNoShowSources) {
			internalError(w, err)
			return
		}
		if err == nil {
			list = []metadata.Show{sh}
		}
	} else if h.Library.Shows != nil {
		p, err := h.Library.Shows.Get(defs.Source)
		if err == nil {
			list, err = p.SearchShows(ctx, term)
		}
		if err != nil {
			internalError(w, err)
			return
		}
	}
	for _, sh := range list {
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

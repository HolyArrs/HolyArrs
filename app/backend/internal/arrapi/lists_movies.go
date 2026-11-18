package arrapi

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// The Radarr side's read-only lists that dashboards and phone apps poll:
// the missing movies and the calendar. The health list below answers on
// both sides. None of them writes.

// movieSorts maps each lower-case sort key of the movie missing list to
// the name answered and the column sorted on.
var movieSorts = map[string][2]string{
	"moviemetadata.digitalrelease":  {"movieMetadata.digitalRelease", "t.digital_date"},
	"moviemetadata.incinemas":       {"movieMetadata.inCinemas", "t.cinema_date"},
	"moviemetadata.physicalrelease": {"movieMetadata.physicalRelease", "t.physical_date"},
	"moviemetadata.sorttitle":       {"movieMetadata.sortTitle", "t.sort_name"},
	"moviemetadata.year":            {"movieMetadata.year", "t.year"},
	"movies.lastsearchtime":         {"movies.lastSearchTime", "i.last_searched_at"},
}

// moviesInOrder puts ms back in the order of ids, which is the sorted order.
func moviesInOrder(ms []movie, ids []int64) []movie {
	at := map[int64]int{}
	for i, id := range ids {
		if _, ok := at[id]; !ok {
			at[id] = i
		}
	}
	slices.SortStableFunc(ms, func(a, b movie) int { return at[a.ID] - at[b.ID] })
	return ms
}

// missingMovies pages the movies with no file in the main Version. As in
// Radarr, a movie that is not out yet is listed too. monitored defaults to
// true and keeps the movies kept up to date; false gives the others. Paging
// and sorting follow the same rules as the show list: no direction means
// descending, only "default" gives the sort title's own ascending order,
// an unknown key falls back to the sort title and a page past the end is
// empty.
func (h *handler) missingMovies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sort, ok := movieSorts[strings.ToLower(query(r, "sortKey"))]
	if !ok {
		sort = movieSorts["moviemetadata.sorttitle"]
	}
	pg := pagingOf(r)
	found, err := library.MissingMovies(ctx, h.Store.Read(), pg.kept, sort[1], !pg.asc, pg.page, pg.size)
	if err != nil {
		internalError(w, err)
		return
	}
	records := []movie{}
	if len(found.IDs) > 0 {
		if records, err = h.loadMovies(ctx, false, "id IN "+idList(found.IDs)); err != nil {
			internalError(w, err)
			return
		}
		records = moviesInOrder(records, found.IDs)
	}
	writeJSON(w, http.StatusOK, map[string]any{"page": pg.page, "pageSize": pg.size, "sortKey": sort[0], "sortDirection": pg.word,
		"totalRecords": found.Total, "records": records})
}

// calendarMovies lists the movies with a cinema, digital or physical
// release date from start to end, both ends included, each movie once, in
// the order of those dates. The default window is today to two days on.
// Without unmonitored=true only kept movies are listed.
func (h *handler) calendarMovies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := h.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from, ok := queryTime(r, "start")
	if !ok {
		from = today
	}
	to, ok := queryTime(r, "end")
	if !ok {
		to = today.AddDate(0, 0, 2)
	}
	out := []movie{}
	if to.Before(from) {
		writeJSON(w, http.StatusOK, out)
		return
	}
	// The Library's calendar leaves the end out; a second more keeps it.
	es, err := library.Calendar(ctx, h.Store.Read(), from, to.Add(time.Second), library.CalendarMovies)
	if err != nil {
		internalError(w, err)
		return
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, e := range es {
		if !seen[e.TitleID] {
			seen[e.TitleID] = true
			ids = append(ids, e.TitleID)
		}
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	where := "id IN " + idList(ids)
	if !queryBool(r, "unmonitored") {
		where += " AND keep_up_to_date = 1"
	}
	if out, err = h.loadMovies(ctx, false, where); err != nil {
		internalError(w, err)
		return
	}
	// Radarr orders by cinema, then digital, then physical date.
	slices.SortStableFunc(out, func(a, b movie) int {
		for _, d := range [][2]*utc{{a.InCinemas, b.InCinemas}, {a.DigitalRelease, b.DigitalRelease}, {a.PhysicalRelease, b.PhysicalRelease}} {
			if c := compareDates(d[0], d[1]); c != 0 {
				return c
			}
		}
		return strings.Compare(a.SortTitle, b.SortTitle)
	})
	writeJSON(w, http.StatusOK, out)
}

// compareDates orders two optional dates, a missing one last.
func compareDates(a, b *utc) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return time.Time(*a).Compare(time.Time(*b))
}

// healthItem is one problem as the old apps list it. wikiUrl is empty:
// HolyArrs has no page for it.
type healthItem struct {
	Source  string `json:"source"`
	Type    string `json:"type"`
	Message string `json:"message"`
	WikiURL string `json:"wikiUrl"`
}

// healthSources name each check's kind the old apps' way, so a dashboard can
// tell the problems apart.
var healthSources = map[string]string{"library_folder": "LibraryFolderCheck", "disk_space": "DiskSpaceCheck", "source": "SourceCheck",
	"downloader": "DownloaderCheck", "backups": "BackupCheck", "update": "UpdateCheck"}

// systemHealth lists what is wrong the way Home counts it: each check
// whose shown state (confirmed, else the latest) is not ok, the worst first,
// as type warning or error, then the setup gaps Home adds itself once setup
// is over: no Source or Downloader, all of them off, or one whose state
// already fails before its check does. A healthy app gives
// an empty list, as the old apps do.
func (h *handler) systemHealth(w http.ResponseWriter, r *http.Request) {
	cs, err := health.List(r.Context(), h.Store)
	if err != nil {
		internalError(w, err)
		return
	}
	out := []healthItem{}
	failingKind := map[string]bool{}
	for _, c := range cs {
		state := c.Confirmed
		if state == "" {
			state = c.State
		}
		if state != health.Warning && state != health.Error {
			continue
		}
		failingKind[c.Kind] = true
		src := healthSources[c.Kind]
		if src == "" {
			src = "HolyArrsCheck"
		}
		out = append(out, healthItem{Source: src, Type: state, Message: c.Message})
	}
	p, err := readSetupParts(r.Context(), h.Store)
	if err != nil {
		internalError(w, err)
		return
	}
	if p.Done {
		gap := func(src, msg string) { out = append(out, healthItem{Source: src, Type: health.Warning, Message: msg}) }
		switch {
		case p.SourcesOff:
		case p.Sources == 0:
			gap("IndexerCheck", "No Sources added yet")
		case p.SourcesOn == 0:
			gap("IndexerCheck", "All Sources are turned off")
		}
		switch {
		case p.Dls == 0:
			gap("DownloadClientCheck", "No Downloaders added yet")
		case p.DlsOn == 0:
			gap("DownloadClientCheck", "All Downloaders are turned off")
		}
		// The checks run every few minutes; a part whose state already
		// fails gets its row at once, as on Home.
		if !p.SourcesOff && p.SourcesFailing > 0 && !failingKind["source"] {
			gap("IndexerCheck", plural(p.SourcesFailing, "Source has", "Sources have")+" a problem")
		}
		if p.DlsFailing > 0 && !failingKind["downloader"] {
			gap("DownloadClientCheck", plural(p.DlsFailing, "Downloader has", "Downloaders have")+" a problem")
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// plural is "1 Source has" or "2 Sources have".
func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

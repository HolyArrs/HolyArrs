package arrapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// Hand search for phone apps: GET /release searches one episode, one
// season or one movie and answers every Release found with its verdict,
// and POST /release sends one of them the way the app's Choose button
// does, with the same refusals.

// searchWaitDefault is how long GET /release waits for its search when
// Deps sets no wait. The old apps answer once every Source has replied.
const searchWaitDefault = 90 * time.Second

// guidPrefix starts the guid HolyArrs gives each Release it answers. The
// grab reads the Release back from it, so nothing else is kept between
// the search and the grab.
const guidPrefix = "holyarrs-release-"

// Each waiting hand search holds a slot: at most perKeySearches for one
// caller and serverSearches in all, so waits cannot pile up.
const (
	perKeySearches = 2
	serverSearches = 8
)

// searchLimit hands out the slots of waiting hand searches.
type searchLimit struct {
	mu     sync.Mutex
	perKey map[domain.Actor]chan struct{}
	all    chan struct{}
}

var searchSlots = &searchLimit{perKey: map[domain.Actor]chan struct{}{}, all: make(chan struct{}, serverSearches)}

// take holds a slot for who, or reports false when none is free. done
// gives the slot back.
func (l *searchLimit) take(who domain.Actor) (done func(), ok bool) {
	l.mu.Lock()
	mine := l.perKey[who]
	if mine == nil {
		mine = make(chan struct{}, perKeySearches)
		l.perKey[who] = mine
	}
	l.mu.Unlock()
	select {
	case mine <- struct{}{}:
	default:
		return nil, false
	}
	select {
	case l.all <- struct{}{}:
	default:
		<-mine
		return nil, false
	}
	return func() { <-l.all; <-mine }, true
}

// releaseRow is one hand search result as the old apps answer it.
type releaseRow struct {
	GUID                string         `json:"guid"`
	Quality             map[string]any `json:"quality"`
	QualityWeight       int            `json:"qualityWeight"`
	Age                 int            `json:"age"`
	AgeHours            float64        `json:"ageHours"`
	AgeMinutes          float64        `json:"ageMinutes"`
	Size                int64          `json:"size"`
	IndexerID           int64          `json:"indexerId"`
	Indexer             string         `json:"indexer"`
	ReleaseGroup        string         `json:"releaseGroup,omitempty"`
	Title               string         `json:"title"`
	FullSeason          bool           `json:"fullSeason"`
	SeasonNumber        *int           `json:"seasonNumber,omitempty"`
	Languages           []language     `json:"languages"`
	Approved            bool           `json:"approved"`
	TemporarilyRejected bool           `json:"temporarilyRejected"`
	Rejected            bool           `json:"rejected"`
	Rejections          []string       `json:"rejections"`
	PublishDate         *utc           `json:"publishDate,omitempty"`
	InfoURL             string         `json:"infoUrl,omitempty"`
	DownloadAllowed     bool           `json:"downloadAllowed"`
	ReleaseWeight       int            `json:"releaseWeight"`
	CustomFormats       []any          `json:"customFormats"`
	CustomFormatScore   int            `json:"customFormatScore"`
	Protocol            string         `json:"protocol"`
	Seeders             *int           `json:"seeders,omitempty"`
	Leechers            *int           `json:"leechers,omitempty"`
	InfoHash            string         `json:"infoHash,omitempty"`
	// Sonarr
	SeriesID         int64   `json:"seriesId,omitempty"`
	MappedSeriesID   int64   `json:"mappedSeriesId,omitempty"`
	EpisodeIDs       []int64 `json:"episodeIds,omitempty"`
	EpisodeRequested *bool   `json:"episodeRequested,omitempty"`
	// Radarr
	MovieID        int64 `json:"movieId,omitempty"`
	MappedMovieID  int64 `json:"mappedMovieId,omitempty"`
	MovieRequested *bool `json:"movieRequested,omitempty"`
}

// releaseSearch is what one GET /release asks for: the Jobs it started
// and, for one episode, the Slots its results are kept to.
type releaseSearch struct {
	titleID int64
	jobs    []int64
	only    []int64
}

// releases answers GET /release. With no title it answers an empty list:
// HolyArrs reads its feeds on its own schedule.
func (h *handler) releases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.Queue == nil {
		writeJSON(w, http.StatusOK, []releaseRow{})
		return
	}
	done, ok := searchSlots.take(actor(r))
	if !ok {
		message(w, http.StatusTooManyRequests, "Too many searches are waiting. Try again when one has answered.")
		return
	}
	defer done()
	s, code, msg := h.startReleaseSearch(ctx, r)
	if code != 0 {
		if code == http.StatusNotFound {
			notFound(w)
		} else if code == http.StatusInternalServerError {
			internalError(w, errors.New(msg))
		} else {
			message(w, code, msg)
		}
		return
	}
	if s == nil {
		writeJSON(w, http.StatusOK, []releaseRow{})
		return
	}
	h.waitJobs(ctx, s.jobs)
	db := h.Store.Read()
	rels, err := sources.ReleasesOfJobs(ctx, db, s.jobs, s.only)
	if err != nil {
		internalError(w, err)
		return
	}
	out, err := h.releaseRows(ctx, rels, s.titleID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// startReleaseSearch queues the hand searches GET /release asks for. They
// only judge and store: nothing is sent until POST /release. A nil search
// and code 0 means nothing was asked for.
func (h *handler) startReleaseSearch(ctx context.Context, r *http.Request) (*releaseSearch, int, string) {
	db := h.Store.Read()
	by := actor(r)
	s := &releaseSearch{}
	enqueue := func(kind string, subject *domain.Subject, params map[string]any, key string) error {
		params["by_hand"], params["choose"] = true, false
		id, _, err := h.Queue.Enqueue(ctx, kind, subject, params, key, by)
		if err == nil {
			s.jobs = append(s.jobs, id)
		}
		return err
	}
	slotSearch := func(slots []mediafiles.Slot) (*releaseSearch, int, string) {
		for _, sl := range slots {
			if err := enqueue(jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: sl.ID}, map[string]any{}, "show_slot:"+itoa(sl.ID)); err != nil {
				return nil, http.StatusInternalServerError, err.Error()
			}
		}
		return s, 0, ""
	}
	failed := func(err error) (*releaseSearch, int, string) {
		if errors.Is(err, database.ErrNotFound) {
			return nil, http.StatusNotFound, ""
		}
		return nil, http.StatusInternalServerError, err.Error()
	}
	if h.side == "radarr" {
		if query(r, "movieId") == "" {
			return nil, 0, ""
		}
		t, err := h.title(ctx, atoi64(query(r, "movieId")), "movie")
		if err != nil {
			return failed(err)
		}
		s.titleID = t.ID
		slots, err := library.MovieSearchSlots(ctx, db, []int64{t.ID})
		if err != nil {
			return failed(err)
		}
		return slotSearch(slots)
	}
	if v := query(r, "episodeId"); v != "" {
		eps, err := h.episodesWhere(ctx, "id = ?", atoi64(v))
		if err != nil {
			return failed(err)
		}
		if len(eps) == 0 {
			return nil, http.StatusNotFound, ""
		}
		s.titleID = eps[0].SeriesID
		slots, err := library.EpisodeSearchSlots(ctx, db, []int64{eps[0].ID})
		if err != nil {
			return failed(err)
		}
		for _, sl := range slots {
			s.only = append(s.only, sl.ID)
		}
		return slotSearch(slots)
	}
	sid, sn := query(r, "seriesId"), query(r, "seasonNumber")
	if sid == "" && sn == "" {
		return nil, 0, ""
	}
	season, err := strconv.Atoi(sn)
	if sid == "" || err != nil {
		return nil, http.StatusBadRequest, "send seriesId and seasonNumber together, or episodeId"
	}
	t, err := h.title(ctx, atoi64(sid), "show")
	if err != nil {
		return failed(err)
	}
	s.titleID = t.ID
	partID, err := library.SeasonPartID(ctx, db, t.ID, season)
	if err != nil {
		return nil, http.StatusNotFound, ""
	}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? ORDER BY is_main DESC, id", t.ID)
	if err != nil {
		return failed(err)
	}
	for _, v := range vs {
		params := map[string]any{"version_id": v.ID}
		if err := enqueue(jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: partID}, params, fmt.Sprintf("show_season:%d:%d", partID, v.ID)); err != nil {
			return failed(err)
		}
	}
	return s, 0, ""
}

// waitJobs waits until the Jobs end, the wait runs out or the caller goes.
// What was found by then is answered either way.
func (h *handler) waitJobs(ctx context.Context, ids []int64) {
	wait := h.SearchWait
	if wait <= 0 {
		wait = searchWaitDefault
	}
	deadline := time.Now().Add(wait)
	for {
		n, err := jobs.CountLive(ctx, h.Store.Read(), ids)
		if err != nil || n == 0 || !time.Now().Before(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// releaseRows turns stored Releases into the old apps' results. titleID
// is the title searched, for results that matched no Slot.
func (h *handler) releaseRows(ctx context.Context, rels []domain.Release, titleID int64) ([]releaseRow, error) {
	out := []releaseRow{}
	if len(rels) == 0 {
		return out, nil
	}
	db := h.Store.Read()
	var srcIDs, slots []int64
	for _, x := range rels {
		srcIDs = append(srcIDs, x.SourceID)
		slots = append(slots, x.MatchedSlotIDs...)
	}
	names, err := sources.SourceNames(ctx, db, srcIDs)
	if err != nil {
		return nil, err
	}
	items, err := mediafiles.SlotItems(ctx, db, slots)
	if err != nil {
		return nil, err
	}
	now := h.now()
	for _, x := range rels {
		row := h.releaseRow(x, names[x.SourceID], now)
		if h.side == "radarr" {
			row.MovieID, row.MappedMovieID, row.MovieRequested = titleID, titleID, ptr(true)
		} else {
			row.SeriesID, row.MappedSeriesID, row.EpisodeRequested = titleID, titleID, ptr(true)
			seen := map[int64]bool{}
			for _, sl := range x.MatchedSlotIDs {
				if it := items[sl]; it > 0 && !seen[it] {
					seen[it] = true
					row.EpisodeIDs = append(row.EpisodeIDs, it)
				}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// releaseRow is one Release with its verdict in the old apps' words. A
// Release held for its wait is turned down for now; one that failed the
// Pick rules or was turned down by hand is turned down with its reasons.
func (h *handler) releaseRow(x domain.Release, indexer string, now time.Time) releaseRow {
	p := releasename.Parse(x.Name, releasename.Hints{})
	row := releaseRow{GUID: guidPrefix + itoa(x.ID), Quality: qualityOfRelease(h.side, x.Name), Size: x.Size, IndexerID: x.SourceID, Indexer: indexer,
		ReleaseGroup: p.Group.Value, Title: x.Name, Languages: []language{}, Rejections: []string{}, CustomFormats: []any{},
		Protocol: arrProtocol(x.Protocol), Seeders: x.Seeders, Leechers: x.Peers, DownloadAllowed: true}
	if x.InfoHash != nil {
		row.InfoHash = *x.InfoHash
	}
	if u, _ := x.Parsed["info_url"].(string); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
		row.InfoURL = u
	}
	if h.side == "sonarr" && p.Show != nil && len(p.Show.Seasons) > 0 {
		row.SeasonNumber = ptr(p.Show.Seasons[0])
		row.FullSeason = len(p.Show.Episodes) == 0 && p.Show.AirDate == ""
	}
	if x.PublishedAt != nil {
		row.PublishDate = utcPtr(x.PublishedAt)
		age := now.Sub(*x.PublishedAt)
		row.Age, row.AgeHours, row.AgeMinutes = int(age.Hours()/24), age.Hours(), age.Minutes()
	}
	if n, ok := x.Verdict["points"].(float64); ok {
		row.CustomFormatScore = int(n)
	}
	if n, ok := x.Verdict["rank"].(float64); ok {
		row.ReleaseWeight = int(n)
	}
	reasons, _ := x.Verdict["reasons"].([]any)
	for _, y := range reasons {
		m, _ := y.(map[string]any)
		if eff, _ := m["effect"].(string); eff != "fail" {
			continue
		}
		if text, _ := m["detail"].(string); text != "" {
			row.Rejections = append(row.Rejections, text)
		}
	}
	passed, _ := x.Verdict["passed"].(bool)
	switch {
	case x.State == "held":
		row.TemporarilyRejected = true
		if x.HoldUntil != nil {
			row.Rejections = append(row.Rejections, "held until "+x.HoldUntil.UTC().Format(time.RFC3339))
		}
	case x.State == "turned_down":
		row.Rejected = true
		why, _ := x.Verdict["turned_down_reason"].(string)
		row.Rejections = append(row.Rejections, strings.TrimSpace("turned down "+why))
	case x.State == "refused" || !passed:
		row.Rejected = true
	default:
		row.Approved = true
	}
	if row.Rejected && len(row.Rejections) == 0 {
		row.Rejections = append(row.Rejections, "it failed the Pick rules")
	}
	return row
}

// grabIn is the part of a result that POST /release reads back.
type grabIn struct {
	GUID      string `json:"guid"`
	IndexerID int64  `json:"indexerId"`
}

// grab answers POST /release: the Release goes through the same Choose
// as the app's button, with its refusals. A Release the Pick rules
// refused is not sent: the app asks a person to force it.
func (h *handler) grab(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in grabIn
	if !decode(w, r, &in) {
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(in.GUID, guidPrefix), 10, 64)
	var rel domain.Release
	if err == nil && strings.HasPrefix(in.GUID, guidPrefix) {
		err = database.Get(ctx, h.Store.Read(), "releases", id, &rel)
	} else {
		err = database.ErrNotFound
	}
	if err == nil && (in.IndexerID != 0 && in.IndexerID != rel.SourceID || !h.releaseOnSide(ctx, rel)) {
		err = database.ErrNotFound
	}
	if err != nil {
		message(w, http.StatusNotFound, "HolyArrs no longer knows this release. Search again.")
		return
	}
	if h.Search == nil {
		message(w, http.StatusServiceUnavailable, "Searching is not running in HolyArrs right now.")
		return
	}
	ch, err := h.Search.Choose(ctx, rel.ID, 0, false, actor(r))
	var nf *sources.NeedsForceError
	switch {
	case errors.As(err, &nf):
		message(w, http.StatusConflict, "HolyArrs did not send it: "+strings.Join(nf.Reasons, "; ")+". To send it anyway, choose it in HolyArrs.")
		return
	case errors.Is(err, sources.ErrNoRelease):
		message(w, http.StatusNotFound, "HolyArrs no longer knows this release. Search again.")
		return
	case errors.Is(err, sources.ErrNoSlot):
		message(w, http.StatusBadRequest, "This release matches nothing HolyArrs is looking for. Pick it from the episode or movie it is for in HolyArrs.")
		return
	case errors.Is(err, sources.ErrBusy), errors.Is(err, sources.ErrState):
		message(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		internalError(w, err)
		return
	}
	h.Search.WaitSend(ctx, &ch)
	if ch.NotSent != "" {
		message(w, http.StatusConflict, "The Downloader did not take it: "+ch.NotSent)
		return
	}
	if err := database.Get(ctx, h.Store.Read(), "releases", rel.ID, &rel); err != nil {
		internalError(w, err)
		return
	}
	rows, err := h.releaseRows(ctx, []domain.Release{rel}, h.releaseTitle(ctx, rel))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows[0])
}

// releaseOnSide reports whether a Release belongs to this side: it
// matched, or was searched for, a title of this side's kind.
func (h *handler) releaseOnSide(ctx context.Context, rel domain.Release) bool {
	return h.releaseTitle(ctx, rel) > 0
}

// releaseTitle is the title of this side's kind a Release matched or was
// searched for, or 0.
func (h *handler) releaseTitle(ctx context.Context, rel domain.Release) int64 {
	kind := map[string]string{"sonarr": "show", "radarr": "movie"}[h.side]
	slots := append([]int64{}, rel.MatchedSlotIDs...)
	if rel.JobID != nil {
		var j domain.Job
		if database.Get(ctx, h.Store.Read(), "jobs", *rel.JobID, &j) == nil && j.Subject != nil {
			if j.Subject.Type == "slot" {
				slots = append(slots, j.Subject.ID)
			} else if j.Subject.Type == "part" {
				var p mediafiles.Part
				if database.Get(ctx, h.Store.Read(), "parts", j.Subject.ID, &p) == nil {
					if t, err := h.title(ctx, p.TitleID, kind); err == nil {
						return t.ID
					}
				}
			}
		}
	}
	rows, err := library.SlotItems(ctx, h.Store.Read(), slots)
	if err != nil {
		return 0
	}
	for _, x := range rows {
		if t, err := h.title(ctx, x.TitleID, kind); err == nil {
			return t.ID
		}
	}
	return 0
}

// arrProtocol is a protocol the old apps know: a plugin's own download type
// shows as "unknown", so their clients do not break on it.
func arrProtocol(p string) string {
	if p != "usenet" && p != "torrent" {
		return "unknown"
	}
	return p
}

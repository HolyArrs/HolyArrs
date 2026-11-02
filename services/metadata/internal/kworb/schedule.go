package kworb

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sort"
	"time"
)

// The schedule: each list page and chart is a job with its next time
// (kept, so a restart keeps the rhythm). A first time is drawn within the
// first hour; after a good read the next is a period later with up to 30
// minutes of jitter; after a failure an hour later. The crawl reads one
// songs page every 45 to 135 s (about 90 s, so 1,000 a day). No time ever
// falls in the first minute of an hour.

// Periods and waits.
const (
	daily       = 24 * time.Hour
	weekly      = 7 * daily
	afterFail   = time.Hour
	pageFor     = weekly // an artist's songs page is read again a week later
	openedFor   = 30 * daily
	crawlMin    = 45 * time.Second
	crawlSpread = 90 * time.Second
	crawlQueue  = 2000 // songs pages lined up per rebuild
	resolveMax  = 500  // artists asked of Wikidata per rebuild
	rebuildFor  = time.Hour
)

// risingSize is how many top daily gainers the rising row looks at.
const risingSize = 20

// offHour moves a time out of the first minute of an hour, just past it
// (runJob already spreads the daily and weekly jobs).
func (s *Service) offHour(t time.Time) time.Time {
	if t.Minute() == 0 {
		t = t.Truncate(time.Minute).Add(time.Minute + time.Duration(s.o.rnd()*float64(59*time.Second)))
	}
	return t
}

func (s *Service) dueAt(job string) (time.Time, bool) {
	var at int64
	if s.db.QueryRow(`SELECT at FROM due WHERE job = ?`, job).Scan(&at) != nil {
		return time.Time{}, false
	}
	return time.Unix(at, 0), true
}

func (s *Service) setDue(job string, t time.Time) {
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO due (job, at) VALUES (?, ?)`, job, s.offHour(t).Unix())
}

// jobs are the scheduled pages and their periods.
func (s *Service) jobs() map[string]time.Duration {
	out := map[string]time.Duration{"listeners": daily, "artists": daily}
	for _, r := range s.o.Regions {
		out[chartKey(r, false)] = daily
		out[chartKey(r, true)] = weekly
	}
	return out
}

// next answers the job due first and when ("crawl" for the crawl).
func (s *Service) nextJob() (string, time.Time) {
	now := s.o.Now()
	best, at := "", time.Time{}
	for job := range s.jobs() {
		t, ok := s.dueAt(job)
		if !ok {
			t = now.Add(time.Duration(s.o.rnd() * float64(time.Hour)))
			s.setDue(job, t)
			t, _ = s.dueAt(job)
		}
		if best == "" || t.Before(at) || t.Equal(at) && job < best {
			best, at = job, t
		}
	}
	t, ok := s.dueAt("crawl")
	if !ok {
		t = now.Add(crawlMin + time.Duration(s.o.rnd()*float64(crawlSpread)))
		s.setDue("crawl", t)
		t, _ = s.dueAt("crawl")
	}
	if best == "" || t.Before(at) {
		best, at = "crawl", t
	}
	return best, at
}

// runJob runs one job and sets its next time.
func (s *Service) runJob(ctx context.Context, job string) error {
	now := s.o.Now()
	var err error
	switch {
	case job == "crawl":
		err = s.crawlOne(ctx)
		next := now.Add(crawlMin + time.Duration(s.o.rnd()*float64(crawlSpread)))
		if errors.Is(err, errCap) {
			// The day's pages are spent: start again early in the next day.
			y, m, d := now.UTC().Date()
			next = time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC).Add(time.Duration(s.o.rnd() * float64(time.Hour)))
		}
		s.setDue("crawl", next)
		return err
	case job == "listeners":
		err = s.RefreshListeners(ctx)
	case job == "artists":
		err = s.RefreshArtists(ctx)
	default:
		region, weekly := chartRegion(job)
		err = s.RefreshChart(ctx, region, weekly)
	}
	period := s.jobs()[job]
	switch {
	case errors.Is(err, errCap):
		y, m, d := now.UTC().Date()
		s.setDue(job, time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC).Add(time.Duration(s.o.rnd()*float64(time.Hour))))
	case err != nil:
		s.setDue(job, now.Add(afterFail+time.Duration(s.o.rnd()*float64(10*time.Minute))))
	default:
		s.setDue(job, now.Add(period+time.Duration((s.o.rnd()-0.5)*float64(time.Hour))))
	}
	return err
}

// chartRegion reads "chart:<region>:daily|weekly".
func chartRegion(job string) (string, bool) {
	rest := job[len("chart:"):]
	for i := len(rest) - 1; i >= 0; i-- {
		if rest[i] == ':' {
			return rest[:i], rest[i+1:] == "weekly"
		}
	}
	return rest, false
}

// RefreshListeners reads the listeners page (asked only when it changed),
// keeps it when it reads, then matches its new ids. A failed read, a
// changed layout or a short page keeps the kept copy.
func (s *Service) RefreshListeners(ctx context.Context) error {
	s.mu.RLock()
	old := s.top
	s.mu.RUnlock()
	body, tag, lastMod, ok, notModified, err := s.page(ctx, "/listeners.html", old.ETag, false)
	switch {
	case err != nil:
		if !errors.Is(err, errOff) && !errors.Is(err, errCap) {
			s.fail(err)
		}
		return err
	case !ok:
		err := &statusErr{"kworb listeners page", 404}
		s.fail(err)
		return err
	case notModified:
		return s.matchLists(ctx)
	}
	rows, err := ParseListeners(body)
	if err == nil && len(rows) < s.o.MinRows {
		err = ErrLayout
	}
	if err != nil {
		s.layout("listeners")
		return err
	}
	s.readOK("listeners")
	t := topList{Rows: rows, ETag: tag, AsOf: dayOf(lastMod)}
	if t.AsOf == "" {
		t.AsOf = s.today()
	}
	s.keep("listeners", t)
	s.setTop(t)
	return s.matchLists(ctx)
}

// RefreshArtists reads the most streamed artists page, like the listeners
// page.
func (s *Service) RefreshArtists(ctx context.Context) error {
	body, _, lastMod, ok, _, err := s.page(ctx, "/artists.html", "", false)
	switch {
	case err != nil:
		if !errors.Is(err, errOff) && !errors.Is(err, errCap) {
			s.fail(err)
		}
		return err
	case !ok:
		err := &statusErr{"kworb artists page", 404}
		s.fail(err)
		return err
	}
	rows, err := ParseArtists(body)
	if err == nil && len(rows) < s.o.MinRows {
		err = ErrLayout
	}
	if err != nil {
		s.layout("artists")
		return err
	}
	s.readOK("artists")
	a := streamedList{Rows: rows, AsOf: dayOf(lastMod)}
	if a.AsOf == "" {
		a.AsOf = s.today()
	}
	s.keep("artists", a)
	s.setStreamed(a)
	return s.matchLists(ctx)
}

// RefreshChart reads one region's daily or weekly song chart. A failed
// read, a missing page or a changed layout keeps the last good chart and
// marks the region failed (its backup source reads it then).
func (s *Service) RefreshChart(ctx context.Context, region string, weekly bool) error {
	kind := "daily"
	if weekly {
		kind = "weekly"
	}
	key := chartKey(region, weekly)
	var st ChartState
	_ = s.loadKept(key, &st)
	body, _, _, ok, _, err := s.page(ctx, "/country/"+region+"_"+kind+".html", "", false)
	if errors.Is(err, errOff) || errors.Is(err, errCap) {
		return err
	}
	if err == nil && !ok {
		err = &statusErr{"kworb " + region + " " + kind + " chart", 404}
	}
	var c Chart
	if err == nil {
		if c, err = ParseChart(body, weekly); err != nil {
			s.layout(key)
		}
	} else {
		s.fail(err)
	}
	if err != nil {
		st.Failed = true
		s.keep(key, st)
		return err
	}
	s.readOK(key)
	s.keep(key, ChartState{Chart: c, At: s.o.Now(), Failed: false})
	return nil
}

// matchLists matches both list pages' ids not paired yet: the day's
// biggest gainers first (the rising row), then by rank.
func (s *Service) matchLists(ctx context.Context) error {
	s.mu.RLock()
	rows := append([]Listener{}, s.top.Rows...)
	streamed := append([]Streamed{}, s.streamed.Rows...)
	s.mu.RUnlock()
	gainers := append([]Listener{}, rows...)
	sort.SliceStable(gainers, func(i, j int) bool { return gainers[i].Change > gainers[j].Change })
	var ids []string
	seen := map[string]bool{}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, l := range gainers[:min(risingSize, len(gainers))] {
		add(l.Spotify)
	}
	for _, l := range rows {
		add(l.Spotify)
	}
	for _, a := range streamed {
		add(a.Spotify)
	}
	_, err := s.matchSpotify(ctx, ids)
	matched := 0
	for _, id := range ids {
		if s.pairBySpotify(id) != "" {
			matched++
		}
	}
	s.count(func(st *Status) { st.Matched = matched })
	s.buildRising()
	return err
}

// buildRising makes the rising row: of the day's 20 biggest gainers, the
// matched ones, biggest gain first.
func (s *Service) buildRising() {
	s.mu.RLock()
	t := s.top
	s.mu.RUnlock()
	if len(t.Rows) == 0 {
		return
	}
	gainers := append([]Listener{}, t.Rows...)
	sort.SliceStable(gainers, func(i, j int) bool { return gainers[i].Change > gainers[j].Change })
	items := []Riser{}
	for _, l := range gainers[:min(risingSize, len(gainers))] {
		if l.Change <= 0 {
			break
		}
		if mb := s.pairBySpotify(l.Spotify); mb != "" {
			items = append(items, Riser{MBID: mb, Name: l.Name, Listeners: l.Listeners, Gain: l.Change, Rank: l.Rank})
		}
	}
	b, err := json.Marshal(map[string]any{"schema_version": 1, "as_of": t.AsOf, "items": items})
	if err != nil {
		return
	}
	s.mu.Lock()
	s.rising = b
	s.mu.Unlock()
}

// crawlTargets lines up the songs pages due: artists opened in the last
// 30 days (latest first), then the catalog's (most popular first), each
// with a Spotify id, not on either list page, and without a page read in
// the last week. Artists with no id yet are asked of Wikidata (batched).
func (s *Service) crawlTargets(ctx context.Context) []string {
	var order []string
	seen := map[string]bool{}
	rows, err := s.db.Query(`SELECT mbid FROM opened WHERE at >= ? ORDER BY at DESC`, s.o.Now().Add(-openedFor).Unix())
	if err == nil {
		for rows.Next() {
			var m string
			if rows.Scan(&m) == nil && !seen[m] {
				seen[m] = true
				order = append(order, m)
			}
		}
		rows.Close()
	}
	if s.o.Catalog != nil {
		for _, m := range s.o.Catalog() {
			if !seen[m] && mbidRe.MatchString(m) {
				seen[m] = true
				order = append(order, m)
			}
		}
	}
	var out, ask []string
	for _, m := range order {
		if len(out) >= crawlQueue || ctx.Err() != nil {
			break
		}
		sp := s.localSpotify(ctx, m)
		if sp == "" {
			if len(ask) < resolveMax && !s.triedRecently("mb:"+m) {
				ask = append(ask, m)
			}
			continue
		}
		if !s.covered(sp) && s.pageDue(sp) {
			out = append(out, sp)
		}
	}
	if len(ask) > 0 && s.wikidataSpotify(ctx, ask) == nil {
		for _, m := range ask {
			if sp := s.pairByMBID(m); sp != "" && len(out) < crawlQueue && !s.covered(sp) && s.pageDue(sp) {
				out = append(out, sp)
			}
		}
	}
	return out
}

// pageDue reports a songs page not read in the last week (or failed an
// hour ago).
func (s *Service) pageDue(sp string) bool {
	if s.triedRecently("page:" + sp) {
		return false
	}
	var at int64
	if s.db.QueryRow(`SELECT at FROM pages WHERE spotify = ?`, sp).Scan(&at) != nil {
		return true
	}
	return s.o.Now().Sub(time.Unix(at, 0)) >= pageFor
}

// crawlOne reads the next due songs page (the line-up is made again every
// hour, so an artist opened goes to the front of the next one).
func (s *Service) crawlOne(ctx context.Context) error {
	if !s.on() {
		return errOff
	}
	if pages, crawled := s.spentToday(); pages >= dayCap || crawled >= crawlCap {
		return errCap
	}
	s.mu.Lock()
	if s.queue == nil || s.o.Now().Sub(s.queueAt) >= rebuildFor {
		s.mu.Unlock()
		q := s.crawlTargets(ctx)
		s.mu.Lock()
		s.queue, s.queueAt = q, s.o.Now()
	}
	var sp string
	for len(s.queue) > 0 && sp == "" {
		sp, s.queue = s.queue[0], s.queue[1:]
		if !s.pageDue(sp) {
			sp = ""
		}
	}
	s.mu.Unlock()
	if sp == "" {
		return nil
	}
	return s.readPage(ctx, sp)
}

// readPage reads one artist's songs page: its total plays, or no page (a
// 404, looked for again a week later). A failure or a changed layout keeps
// what was kept and tries again an hour later.
func (s *Service) readPage(ctx context.Context, sp string) error {
	body, _, _, ok, _, err := s.page(ctx, "/artist/"+sp+"_songs.html", "", true)
	switch {
	case err != nil:
		if !errors.Is(err, errOff) && !errors.Is(err, errCap) {
			s.fail(err)
			_, _ = s.db.Exec(`INSERT OR REPLACE INTO tried (k, until) VALUES (?, ?)`, "page:"+sp, s.o.Now().Add(afterFail).Unix())
		}
		return err
	case !ok:
		s.noPage(sp)
		return nil
	}
	p, err := ParseArtist(body)
	if errors.Is(err, ErrNoTotals) {
		// No totals to keep: like no page, looked at again a week later.
		s.noPage(sp)
		return nil
	}
	if err != nil {
		s.layout("songs page")
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO tried (k, until) VALUES (?, ?)`, "page:"+sp, s.o.Now().Add(afterFail).Unix())
		return err
	}
	s.readOK("songs page")
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO pages (spotify, found, total, as_of, at) VALUES (?, 1, ?, ?, ?)`, sp, p.Total, p.AsOf, s.o.Now().Unix())
	return nil
}

// Run runs the schedule until ctx ends: the job due first, then the next.
func (s *Service) Run(ctx context.Context) {
	for ctx.Err() == nil {
		job, at := s.nextJob()
		if wait := at.Sub(s.o.Now()); wait > 0 {
			// Look again at least every 5 minutes (the kill switch, new jobs).
			if s.o.sleep(ctx, min(wait, 5*time.Minute)) != nil {
				return
			}
			continue
		}
		c, cancel := context.WithTimeout(ctx, 30*time.Minute)
		err := s.runJob(c, job)
		cancel()
		if err != nil && !errors.Is(err, errOff) && !errors.Is(err, errCap) && ctx.Err() == nil {
			log.Printf("kworb: %s: %v", job, err)
		}
	}
}

// noPage keeps that the artist has no songs page (or one without totals),
// looked at again a week later.
func (s *Service) noPage(sp string) {
	s.count(func(st *Status) { st.NoPage++ })
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO pages (spotify, found, total, as_of, at) VALUES (?, 0, 0, '', ?)`, sp, s.o.Now().Unix())
}

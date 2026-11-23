package sources

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A release check searches the Sources for a movie that is not in the
// Library yet and judges what they have against the Pick rules a person
// chose, so they can see the releases before adding it. Nothing is stored
// and no Slot moves; the results are kept in memory for a while. Picking one
// after the movie is added stores it for the new Slot and chooses it.

// ErrCheckGone is a pick from a check that is no longer kept.
var ErrCheckGone = errors.New("these results are no longer kept; check releases again")

// checkKeep is how long a check's results are kept, and checkMax how many.
const (
	checkKeep = time.Hour
	checkMax  = 20
)

type checkResult struct {
	at    time.Time
	rules int64
	all   []found
	views []ReleaseView
}

type checkStore struct {
	mu sync.Mutex
	m  map[int64]*checkResult
}

func (c *checkStore) put(id int64, r *checkResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[int64]*checkResult{}
	}
	for k, v := range c.m {
		if time.Since(v.at) > checkKeep {
			delete(c.m, k)
		}
	}
	for len(c.m) >= checkMax {
		var oldest int64
		for k, v := range c.m {
			if oldest == 0 || v.at.Before(c.m[oldest].at) {
				oldest = k
			}
		}
		delete(c.m, oldest)
	}
	c.m[id] = r
}

func (c *checkStore) get(id int64) (*checkResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.m[id]
	if !ok || time.Since(r.at) > checkKeep {
		return nil, false
	}
	return r, true
}

// checkTitle is the movie a check is for, read from the Job's params.
func checkTitle(p map[string]any) (mediafiles.Title, int64, error) {
	num := func(k string) int64 {
		if n, ok := p[k].(float64); ok {
			return int64(n)
		}
		if n, ok := p[k].(int); ok {
			return int64(n)
		}
		return 0
	}
	name, _ := p["name"].(string)
	tmdb, rules := num("tmdb"), num("pick_rules_id")
	if name == "" || tmdb <= 0 || rules <= 0 {
		return mediafiles.Title{}, 0, errors.New("a release check needs a name, a TMDB id and Pick rules")
	}
	ti := mediafiles.Title{Kind: "movie", Name: name, Aliases: []string{}, SourceIDs: map[string]string{"tmdb": strconv.FormatInt(tmdb, 10)}}
	if imdb, _ := p["imdb"].(string); imdb != "" {
		ti.SourceIDs["imdb"] = imdb
	}
	if y := int(num("year")); y > 0 {
		ti.Year = &y
	}
	if l, _ := p["original_language"].(string); l != "" {
		ti.OriginalLanguage = &l
	}
	if o, _ := p["original_name"].(string); o != "" && o != name {
		ti.Aliases = append(ti.Aliases, o)
	}
	return ti, rules, nil
}

// HandleCheck runs a release check Job.
func (s *Searcher) HandleCheck(ctx context.Context, r *jobs.Run) error {
	ti, rulesID, err := checkTitle(r.Job.Params)
	if err != nil {
		return err
	}
	var c slotCtx
	if err := mediafiles.LoadPickRules(ctx, s.Store.Read(), rulesID, &c.pr); err != nil {
		return fmt.Errorf("pick rules %d: %w", rulesID, err)
	}
	c.ti, c.ver.PickRulesID, c.listing = ti, rulesID, true
	q := Query{Title: ti.Name, IMDb: ti.SourceIDs["imdb"], Aliases: ti.Aliases}
	if ti.Year != nil {
		q.Year = *ti.Year
	}
	q.TMDb, _ = strconv.Atoi(ti.SourceIDs["tmdb"])
	q.other = s.nameAsk(ctx, ti, true)
	withWanted(&q, wantedNames(ti, nil, s.titleLanguage(ctx)))
	q.drops = &dropTally{}
	all, failures, answered, skipped, note, err := s.askAll(ctx, r, q, true, false, ti, 0, c.pr.Body.Tags)
	if err != nil {
		return err
	}
	if keptApart(note) {
		return errors.New(note)
	}
	if reason, _, ok, failed := noAnswer(failures, answered, skipped); ok {
		s.checks.put(r.Job.ID, &checkResult{at: time.Now(), rules: rulesID})
		if failed {
			return errors.New(reason)
		}
		r.SetResult(map[string]any{"releases": 0, "note": reason})
		return nil
	}
	all = dedupe(all)
	rels, vs, err := s.judgeAll(ctx, c, all, true, true, false)
	if err != nil {
		return err
	}
	views := make([]ReleaseView, len(all))
	refused := 0
	for i, f := range all {
		v := vs[i]
		if v.Outcome == "refused" {
			refused++
		}
		x := domain.Release{ID: int64(i + 1), SourceID: f.src.ID, Name: f.item.Title, Protocol: f.item.Protocol, Size: f.item.Size,
			Seeders: f.item.Seeders, Peers: f.item.Peers, State: "judged", FoundBy: "hand_search", Parsed: toMap(rels[i].Parse), Verdict: toMap(v)}
		if !f.item.Published.IsZero() {
			p := f.item.Published
			x.PublishedAt = &p
		}
		views[i] = releaseView(x)
		views[i].SourceName = f.src.Name
	}
	s.checks.put(r.Job.ID, &checkResult{at: time.Now(), rules: rulesID, all: all, views: views})
	result := map[string]any{"releases": len(all), "refused": refused, "sources_answered": answered}
	if dropped, by := q.drops.total(); dropped > 0 {
		result["dropped"], result["dropped_sources"] = dropped, by
	}
	r.SetResult(result)
	return nil
}

// CheckResult gives the releases a check found, judged; false when the
// check is not kept (still running, never ran, or too old).
func (s *Searcher) CheckResult(jobID int64) ([]ReleaseView, bool) {
	r, ok := s.checks.get(jobID)
	if !ok {
		return nil, false
	}
	return r.views, true
}

// CheckPick stores one checked release for the added movie's Slot whose
// Version uses the Pick rules the check judged with, and chooses it.
func (s *Searcher) CheckPick(ctx context.Context, jobID, releaseID, titleID int64, force bool, by domain.Actor) (Choice, error) {
	r, ok := s.checks.get(jobID)
	if !ok || releaseID < 1 || releaseID > int64(len(r.all)) {
		return Choice{}, ErrCheckGone
	}
	slotID, err := s.checkSlot(ctx, titleID, r)
	if err != nil {
		return Choice{}, err
	}
	c, err := s.loadSlot(ctx, slotID)
	if err != nil {
		return Choice{}, ErrNoSlot
	}
	c.listing = true
	one := []found{r.all[releaseID-1]}
	rels, vs, err := s.judgeAll(ctx, c, one, true, true, false)
	if err != nil {
		return Choice{}, err
	}
	// Judged against the real Slot, so a refusal names it and the pick goes there.
	vs[0].SlotIDs = []int64{slotID}
	ids, err := s.storeAll(ctx, by, nil, "hand_search", slotID, one, rels, vs, true)
	if err != nil {
		return Choice{}, err
	}
	return s.Choose(ctx, ids[0], slotID, force, by)
}

// checkSlot is the movie's Slot whose Version has the Pick rules the check
// used, else its main one.
func (s *Searcher) checkSlot(ctx context.Context, titleID int64, r *checkResult) (int64, error) {
	db := s.Store.Read()
	var ti mediafiles.Title
	if err := mediafiles.LoadTitle(ctx, db, titleID, &ti); err != nil || ti.Kind != "movie" {
		return 0, ErrNoSlot
	}
	slots, err := mediafiles.SlotsOfTitle(ctx, db, titleID)
	if err != nil {
		return 0, err
	}
	if len(slots) == 0 {
		return 0, ErrNoSlot
	}
	for _, sl := range slots {
		var v mediafiles.Version
		if mediafiles.LoadVersion(ctx, db, sl.VersionID, &v) == nil && v.PickRulesID == r.rules {
			return sl.ID, nil
		}
	}
	return slots[0].ID, nil
}

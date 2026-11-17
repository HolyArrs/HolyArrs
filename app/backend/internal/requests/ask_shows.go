package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// errAskedMeanwhile says someone asked for the same show while this ask was
// being written; asking again joins theirs.
var errAskedMeanwhile = errors.New("asked for at the same moment")

// AskShow asks for a show for p by "tmdb:", "tvdb:", "tvmaze:" or "imdb:"
// id: the whole show when seasons is empty, else the chosen seasons. An
// open ask for the same show joins it (a waiting one takes the new seasons
// too); a show already in the Library is asked only for the seasons it
// does not keep up to date yet.
func (s *Service) AskShow(ctx context.Context, p Person, id string, seasons []int) (Result, error) {
	return s.askShowLine(ctx, p, id, seasons, nil, false, false)
}

// AskShowParts asks for whole seasons and single episodes of others (for
// example season 1 whole and episodes 1, 2 and 5 of season 2). Both empty
// is the whole show.
func (s *Service) AskShowParts(ctx context.Context, p Person, id string, seasons []int, eps []domain.AskEpisodes) (Result, error) {
	return s.askShowLine(ctx, p, id, seasons, eps, false, false)
}

// AskShowFutureOnly asks for a show's future seasons only: the seasons and
// episodes that air from now on, not the ones already out.
func (s *Service) AskShowFutureOnly(ctx context.Context, p Person, id string) (Result, error) {
	return s.askShowLine(ctx, p, id, nil, nil, true, false)
}

// AskShowFourK asks for a 4K copy of a show, with the same choices as a
// plain ask (seasons, single episodes, or future seasons only). It is its
// own ask, approved apart from a plain one, and joins only other 4K asks.
func (s *Service) AskShowFourK(ctx context.Context, p Person, id string, seasons []int, eps []domain.AskEpisodes, futureOnly bool) (Result, error) {
	return s.askShowLine(ctx, p, id, seasons, eps, futureOnly, true)
}

// AskShowQuality asks for a show in one line of what people can ask for
// (empty: p's first line), with the same choices as AskShowFourK. A line p
// may not use is ErrQualityNotOffered.
func (s *Service) AskShowQuality(ctx context.Context, p Person, id string, seasons []int, eps []domain.AskEpisodes, futureOnly bool, quality string) (Result, error) {
	l, err := pickQuality(ctx, s.Store.Read(), p, "show", quality)
	if err != nil {
		return Result{}, err
	}
	return s.askShow(ctx, p, strings.TrimSpace(id), seasons, eps, futureOnly, l, false)
}

// askShowLine asks on the Standard line, or the 4K line for fourK.
func (s *Service) askShowLine(ctx context.Context, p Person, id string, seasons []int, eps []domain.AskEpisodes, futureOnly, fourK bool) (Result, error) {
	l, err := builtInLine(ctx, s.Store.Read(), "show", fourK)
	if err != nil {
		return Result{}, err
	}
	return s.askShow(ctx, p, strings.TrimSpace(id), seasons, eps, futureOnly, l, false)
}

// fits says whether an open ask o gives what a new ask wants: its seasons
// cover them, and a future-only ask only fits another future-only ask.
func fits(o domain.Ask, want []int, wantEps []domain.AskEpisodes, futureOnly bool) bool {
	if futureOnly || o.FutureOnly {
		return o.FutureOnly && futureOnly || (futureOnly && wholeShow(o))
	}
	return coversParts(o, want, wantEps)
}

func (s *Service) askShow(ctx context.Context, p Person, id string, seasons []int, asked []domain.AskEpisodes, futureOnly bool, q QualityLine, retried bool) (Result, error) {
	db := s.Store.Read()
	fourK := q.ID == QualityFourK
	defs, err := library.ReadShowDefaults(ctx, db)
	if err != nil {
		return Result{}, err
	}
	sh, err := shows.New(s.Library).LookupShow(ctx, id, defs.Source)
	if err != nil {
		return Result{}, err
	}
	if p.MaxRating != "" {
		tid, err := library.ShowByIDs(ctx, db, sh.IDs)
		if err != nil {
			return Result{}, err
		}
		if err := libraryOverLimit(ctx, db, p, "show", tid); err != nil {
			return Result{}, err
		}
		if err := overLimit(p, "show", sh.Rating); err != nil {
			return Result{}, err
		}
	}
	want, err := pickSeasons(seasons, sh.Seasons)
	if err != nil {
		return Result{}, err
	}
	eps, err := pickEpisodes(asked, want, sh.Seasons)
	if err != nil {
		return Result{}, err
	}
	if futureOnly {
		want, eps = []int{}, []domain.AskEpisodes{}
	}
	open, err := openShowAsks(ctx, db, sh.IDs, q.ID)
	if err != nil {
		return Result{}, err
	}
	for _, o := range open {
		if fits(o, want, eps, futureOnly) {
			return s.join(ctx, p, o.ID)
		}
	}
	for _, o := range open {
		if o.State == domain.AskAsked && !futureOnly {
			res, err := s.joinWider(ctx, p, o, want, eps)
			if errors.Is(err, ErrNotOpen) && !retried {
				// Decided between the look and the write: look again.
				return s.askShow(ctx, p, id, seasons, asked, futureOnly, q, true)
			}
			return res, err
		}
	}
	tid, err := library.ShowByIDs(ctx, db, sh.IDs)
	if err != nil {
		return Result{}, err
	}
	// The Library holds the ask's own copy (the 4K one for a 4K ask, else
	// a plain one); without it everything asked is still to get.
	var vid int64
	if tid != 0 {
		if vid, err = copyOf(ctx, db, tid, fourK); err != nil {
			return Result{}, err
		}
	}
	if vid != 0 && futureOnly {
		var keep bool
		var policy string
		var choice sql.NullString
		if err := db.QueryRowContext(ctx, askShowSQL, tid).Scan(&keep, &policy, &choice); err != nil {
			return Result{}, err
		}
		if keep && policy == "all" && (choice.String == library.SeasonsFuture || choice.String == library.SeasonsAll) {
			return Result{}, &InLibraryError{TitleID: tid, Show: true}
		}
	} else if vid != 0 {
		missing := []int{}
		if len(want) > 0 || len(eps) == 0 {
			if missing, err = notKept(ctx, db, tid, want); err != nil {
				return Result{}, err
			}
		}
		missingEps, err := episodesNotKept(ctx, db, tid, eps)
		if err != nil {
			return Result{}, err
		}
		if len(missing) == 0 && len(missingEps) == 0 {
			ready, err := showReady(ctx, db, tid, vid, want, eps, s.now())
			if err != nil {
				return Result{}, err
			}
			return Result{}, &InLibraryError{TitleID: tid, Ready: ready, Show: true}
		}
		want, eps = missing, missingEps
	}
	if never, err := library.ShowOnNeverAdd(ctx, db, sh.IDs); err != nil {
		return Result{}, err
	} else if never {
		return Result{}, shows.ErrShowNeverAdd
	}
	if err := s.checkLimit(ctx, db, p, KindShows); err != nil {
		return Result{}, err
	}
	now := s.now()
	ids := map[string]string{}
	for k, v := range sh.IDs {
		if v != "" {
			ids[k] = v
		}
	}
	a := domain.Ask{Kind: "show", TMDBID: ids["tmdb"], Ref: id, SourceIDs: ids, Seasons: want, Episodes: eps, FutureOnly: futureOnly, Name: sh.Name, Poster: sh.Poster,
		State: domain.AskAsked, AskedBy: p.UserID, AskedAt: now}
	withQuality(&a, q)
	if sh.Year > 0 {
		y := sh.Year
		a.Year = &y
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		// Look again inside the write, so two people asking at once end
		// up on one ask, and asks sent at once cannot all pass the limit.
		// Only an open ask this one would join or widen counts; an
		// approved ask for other seasons does not stop a new ask.
		open, err := openShowAsks(ctx, tx, ids, q.ID)
		if err != nil {
			return err
		}
		for _, o := range open {
			if fits(o, want, eps, futureOnly) || (o.State == domain.AskAsked && !futureOnly) {
				return errAskedMeanwhile
			}
		}
		if err := s.checkLimit(ctx, tx, p, KindShows); err != nil {
			return err
		}
		if _, err := database.Insert(tx, "asks", &a, p.actor()); err != nil {
			return err
		}
		_, err = tx.Exec(askShowSQL2, a.ID, p.UserID, database.FormatTime(now))
		return err
	})
	if errors.Is(err, errAskedMeanwhile) && !retried {
		return s.askShow(ctx, p, id, seasons, asked, futureOnly, q, true)
	}
	if err != nil {
		return Result{}, err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskMade, Summary: p.Name + " asked for " + label(a),
		Data: map[string]any{"ask_id": a.ID, "show": id, "seasons": askSeasonWords(a), "asked_by": p.Name}, GroupKey: groupKey(a.ID)})
	if p.autoApproves(fourK) {
		if got, _, err := s.decide(ctx, a.ID, p, ApproveOptions{}, true); err == nil {
			return Result{Ask: got}, nil
		} else {
			log.Printf("asks: auto-approve %s: %v", a.Name, err)
		}
	}
	s.tellStaff(ctx, a, p)
	s.changed(ctx, a.ID)
	return Result{Ask: a}, nil
}

// ErrNoSuchSeason says an asked-for season is not in the show.
var ErrNoSuchSeason = errors.New("the show has no such season")

// pickSeasons checks the asked-for seasons against the show's and sorts
// them; empty stays empty (the whole show).
func pickSeasons(asked []int, known []metadata.Season) ([]int, error) {
	out := []int{}
	for _, n := range asked {
		if !slices.ContainsFunc(known, func(k metadata.Season) bool { return k.Number == n }) {
			return nil, ErrNoSuchSeason
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out, nil
}

// covers says whether an ask for have already asks for want.
func covers(have, want []int) bool {
	if len(have) == 0 {
		return true
	}
	if len(want) == 0 {
		return false
	}
	for _, n := range want {
		if !slices.Contains(have, n) {
			return false
		}
	}
	return true
}

// widen joins two season lists; either being the whole show gives the
// whole show.
func widen(a, b []int) []int {
	if len(a) == 0 || len(b) == 0 {
		return []int{}
	}
	out := slices.Clone(a)
	for _, n := range b {
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return out
}

// openShowAsks finds the open asks (waiting or approved) for one line of
// what people can ask for, for a show with any of ids, oldest first.
func openShowAsks(ctx context.Context, q database.Lister, ids map[string]string, quality string) ([]domain.Ask, error) {
	var or []string
	args := []any{quality}
	for _, k := range []string{"tmdb", "tvdb", "tvmaze", "imdb"} {
		if ids[k] != "" {
			or = append(or, "json_extract(source_ids, '$."+k+"') = ?")
			args = append(args, ids[k])
		}
	}
	if len(or) == 0 {
		return nil, nil
	}
	return database.Select[domain.Ask](ctx, q, "asks", "kind = 'show' AND quality = ? AND state IN ('asked', 'approved') AND ("+strings.Join(or, " OR ")+") ORDER BY id", args...)
}

// joinWider adds p to a waiting show ask that asks for fewer seasons, and
// widens it to the seasons p asked for too. Asking for more seasons is a
// new ask for p: it counts as one against p's show limit, checked inside
// the write before anything changes.
func (s *Service) joinWider(ctx context.Context, p Person, o domain.Ask, want []int, wantEps []domain.AskEpisodes) (Result, error) {
	return s.joinWiderWith(ctx, p, o, KindShows, func(a *domain.Ask) { widenParts(a, want, wantEps); a.FutureOnly = false },
		func(a domain.Ask) map[string]any { return map[string]any{"seasons": askSeasonWords(a)} })
}

// joinWiderWith adds p to a waiting ask and grows it; it counts as one ask
// against p's limit for kind, and data adds to the Event.
func (s *Service) joinWiderWith(ctx context.Context, p Person, o domain.Ask, kind string, grow func(*domain.Ask), data func(domain.Ask) map[string]any) (Result, error) {
	var cur domain.Ask
	already := false
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		var err error
		if cur, err = get(ctx, tx, o.ID); err != nil {
			return err
		}
		if cur.State != domain.AskAsked {
			return ErrNotOpen
		}
		var n int
		if err := tx.QueryRow(joinWiderWithSQL, o.ID, p.UserID).Scan(&n); err != nil {
			return err
		}
		already = n > 0
		if !already {
			if err := s.checkLimit(ctx, tx, p, kind); err != nil {
				return err
			}
		}
		grow(&cur)
		if err := database.Update(tx, "asks", &cur, p.actor()); err != nil {
			return err
		}
		if already {
			return nil
		}
		_, err = tx.Exec(joinWiderWithSQL2, o.ID, p.UserID, database.FormatTime(s.now()))
		return err
	})
	if err != nil {
		return Result{}, err
	}
	if already {
		s.changed(ctx, cur.ID)
		return Result{Ask: cur, Already: true}, nil
	}
	d := map[string]any{"ask_id": cur.ID, "asked_by": p.Name}
	for k, v := range data(cur) {
		d[k] = v
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskJoined, TitleID: cur.TitleID, Summary: p.Name + " also asked for " + label(cur),
		Data: d, GroupKey: groupKey(cur.ID)})
	if p.autoApproves(cur.FourK) {
		if got, _, err := s.decide(ctx, cur.ID, p, ApproveOptions{}, true); err == nil {
			cur = got
		}
	}
	s.changed(ctx, cur.ID)
	return Result{Ask: cur, Joined: true}, nil
}

// regularSeasons lists a show's seasons in the Library except specials.
func regularSeasons(ctx context.Context, db querier, titleID int64) ([]int, error) {
	rows, err := db.QueryContext(ctx, regularSeasonsSQL, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// notKept lists which of the wanted seasons (empty: every season but
// specials) a show in the Library does not keep up to date. A show whose
// own switch is off keeps none.
func notKept(ctx context.Context, db querier, titleID int64, want []int) ([]int, error) {
	if len(want) == 0 {
		var err error
		if want, err = regularSeasons(ctx, db, titleID); err != nil {
			return nil, err
		}
	}
	var showKeep bool
	if err := db.QueryRowContext(ctx, notKeptSQL, titleID).Scan(&showKeep); err != nil {
		return nil, err
	}
	out := []int{}
	for _, n := range want {
		var keep bool
		err := db.QueryRowContext(ctx, notKeptSQL2, titleID, n).Scan(&keep)
		if errors.Is(err, sql.ErrNoRows) {
			// A season the Library does not know yet (the next refresh adds
			// it) is asked for.
			out = append(out, n)
			continue
		}
		if err != nil {
			return nil, err
		}
		if !keep || !showKeep {
			out = append(out, n)
		}
	}
	return out, nil
}

// showReady says whether every aired episode of the seasons (empty: every
// season but specials) has a file in the Version versionID (the ask's own
// copy), with at least one aired. No such copy is not ready.
func showReady(ctx context.Context, db querier, titleID, versionID int64, seasons []int, eps []domain.AskEpisodes, now time.Time) (bool, error) {
	if versionID == 0 {
		return false, nil
	}
	if len(seasons) == 0 && len(eps) == 0 {
		var err error
		if seasons, err = regularSeasons(ctx, db, titleID); err != nil {
			return false, err
		}
	}
	if len(seasons) == 0 && len(eps) == 0 {
		return false, nil
	}
	args := []any{versionID, titleID}
	var or []string
	if len(seasons) > 0 {
		or = append(or, "p.number IN ("+strings.TrimSuffix(strings.Repeat("?,", len(seasons)), ",")+")")
		for _, n := range seasons {
			args = append(args, n)
		}
	}
	for _, e := range eps {
		or = append(or, "(p.number = ? AND json_extract(i.numbers, '$.episode') IN ("+strings.TrimSuffix(strings.Repeat("?,", len(e.Episodes)), ",")+"))")
		args = append(args, e.Season)
		for _, n := range e.Episodes {
			args = append(args, n)
		}
	}
	args = append(args, database.FormatTime(now))
	var aired, ready int
	err := db.QueryRowContext(ctx, showReadySQL+strings.Join(or, " OR ")+`) AND i.air_date IS NOT NULL AND i.air_date <= ?`,
		args...).Scan(&aired, &ready)
	return aired > 0 && aired == ready, err
}

// addShowForAsk adds the asked-for show (or finds it in the Library) and
// keeps the asked-for seasons up to date. Only the asked copy is added: a
// new show asked for in 4K gets its main copy as the 4K copy, as a movie
// does. It says true when the approver's pick was not used because that
// copy was there.
func (s *Service) addShowForAsk(ctx context.Context, a domain.Ask, set Settings, opt ApproveOptions, by Person) (int64, bool, error) {
	tid, err := library.ShowByIDs(ctx, s.Store.Read(), a.SourceIDs)
	if err != nil {
		return 0, false, err
	}
	if err := askedRules(ctx, s.Store.Read(), a, &opt, set.ShowFourKLibraryFolderID, set.ShowFourKPickRulesID, set.ShowLibraryFolderID); err != nil {
		return 0, false, err
	}
	kept := false
	if tid != 0 {
		// In the Library: the ask's own copy (4K, or plain beside a 4K-only
		// show) is added in one write; the main copy stays as it is.
		if kept, err = s.addCopy(ctx, tid, a.FourK, opt, set.ShowLibraryFolderID, set.ShowPickRulesID, by); err != nil {
			return 0, false, err
		}
	}
	if tid == 0 {
		o := shows.ShowAddOptions{LibraryFolderID: opt.LibraryFolderID, PickRulesID: opt.PickRulesID, Seasons: "all", WhyAdded: "ask", MainFourK: a.FourK}
		if !wholeShow(a) {
			o.Seasons = "none"
		}
		if a.FutureOnly {
			o.Seasons = library.SeasonsFuture
		}
		if o.LibraryFolderID == 0 {
			o.LibraryFolderID = set.ShowLibraryFolderID
		}
		if o.PickRulesID == 0 {
			o.PickRulesID = set.ShowPickRulesID
		}
		var dup *shows.ShowAlreadyAddedError
		tid, _, err = shows.New(s.Library).AddShow(ctx, a.Ref, o, by.actor())
		switch {
		case errors.As(err, &dup):
			tid = dup.TitleID
			if kept, err = s.addCopy(ctx, tid, a.FourK, opt, set.ShowLibraryFolderID, set.ShowPickRulesID, by); err != nil {
				return 0, false, err
			}
		case err != nil:
			return 0, false, err
		case wholeShow(a):
			return tid, kept, nil
		}
	}
	if a.FutureOnly {
		return tid, kept, shows.New(s.Library).KeepFuture(ctx, tid, by.actor())
	}
	db := s.Store.Read()
	var showKeep bool
	if err := db.QueryRowContext(ctx, addShowForAskSQL, tid).Scan(&showKeep); err != nil {
		return 0, false, err
	}
	if !showKeep {
		on := true
		if _, err := s.Library.Edit(ctx, tid, library.TitleChange{KeepUpToDate: &on}, false, by.actor()); err != nil {
			return 0, false, err
		}
	}
	seasons := a.Seasons
	if wholeShow(a) {
		if seasons, err = regularSeasons(ctx, db, tid); err != nil {
			return 0, false, err
		}
	}
	for _, n := range seasons {
		var keep bool
		err := db.QueryRowContext(ctx, addShowForAskSQL2, tid, n).Scan(&keep)
		if errors.Is(err, sql.ErrNoRows) || keep {
			continue
		}
		if err != nil {
			return 0, false, err
		}
		if err := shows.New(s.Library).SetSeason(ctx, tid, n, true, by.actor()); err != nil && !errors.Is(err, library.ErrNoSuchSeason) {
			return 0, false, err
		}
	}
	for _, e := range a.Episodes {
		if err := shows.New(s.Library).KeepEpisodes(ctx, tid, e.Season, e.Episodes, by.actor()); err != nil && !errors.Is(err, library.ErrNoSuchSeason) {
			return 0, false, err
		}
	}
	return tid, kept, nil
}

// readyShows finds approved show asks whose asked-for seasons are all in
// the Library (every aired episode has a file).
func (s *Service) readyShows(ctx context.Context) ([]domain.Ask, error) {
	as, err := database.Select[domain.Ask](ctx, s.Store.Read(), "asks", "kind = 'show' AND state = 'approved' AND title_id IS NOT NULL ORDER BY id")
	if err != nil {
		return nil, err
	}
	var out []domain.Ask
	for _, a := range as {
		if a.FutureOnly {
			continue // keeps up with what airs; there is no "all in" moment
		}
		vid, err := copyOf(ctx, s.Store.Read(), *a.TitleID, a.FourK)
		if err != nil {
			return nil, err
		}
		ok, err := showReady(ctx, s.Store.Read(), *a.TitleID, vid, a.Seasons, a.Episodes, s.now())
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, a)
		}
	}
	return out, nil
}

// seasonWords says which seasons an ask is for: "the whole show",
// "season 2" or "seasons 1, 2 and 4".
func seasonWords(seasons []int) string {
	switch len(seasons) {
	case 0:
		return "the whole show"
	case 1:
		return "season " + strconv.Itoa(seasons[0])
	}
	parts := make([]string, len(seasons))
	for i, n := range seasons {
		parts[i] = strconv.Itoa(n)
	}
	return fmt.Sprintf("seasons %s and %s", strings.Join(parts[:len(parts)-1], ", "), parts[len(parts)-1])
}

// askSeasonWords says which seasons a show ask is for, "future seasons
// only" for a future-only ask.
func askSeasonWords(a domain.Ask) string {
	if a.FutureOnly {
		return "future seasons only"
	}
	if len(a.Episodes) > 0 {
		return partsWords(a.Seasons, a.Episodes)
	}
	return seasonWords(a.Seasons)
}

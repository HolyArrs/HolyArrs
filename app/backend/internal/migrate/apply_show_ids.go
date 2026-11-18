package migrate

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

var errShowNotFound = errors.New("the show could not be found by its ids or its name")

// withShowIDs runs add with the show's own id; only when the metadata
// source does not know that show it tries the IMDb id, then the folder's
// name and year, and last the TMDB id the old app gave. A different TMDB
// id from the old app is never used to replace an id that found the show.
// When only that TMDB id finds it and the show has another name, add hears
// that name as other: the two may number their seasons differently. The
// dry run and the import both decide through here.
func (r *run) withShowIDs(t *TitlePlan, add func(id, other string) error) error {
	err := add(t.ID, "")
	if t.Kind != "show" || !isNoSuchShow(err) {
		return err
	}
	if r.dry && r.pastBudget() {
		return errNotChecked
	}
	ids, last := r.otherShowIDs(t)
	for _, id := range ids {
		if err = add(id, ""); !isNoSuchShow(err) {
			return err
		}
	}
	if last != "" {
		sh, err := r.lookupShow(last)
		if err == nil {
			other := ""
			if !sameName(sh, t.Folder, t.Year) {
				other = sh.Name
			}
			return add(last, other)
		}
		if !isNoSuchShow(err) {
			return err
		}
	}
	return errShowNotFound
}

// lookupShow finds a show by id the way adding it does. A dry run keeps
// each answer a while (Mover.dryShows) and, once its time is up (dryLookupBudget),
// asks for no more: those shows are errNotChecked.
func (r *run) lookupShow(id string) (metadata.Show, error) {
	defs, err := library.ReadShowDefaults(r.ctx, r.m.Store.Read())
	if err != nil {
		return metadata.Show{}, err
	}
	if !r.dry {
		return shows.New(r.m.Library).LookupShow(r.ctx, id, defs.Source)
	}
	key := defs.Source + " " + id
	r.m.dryMu.Lock()
	a, ok := r.m.dryShows[key]
	r.m.dryMu.Unlock()
	if ok && time.Since(a.at) < dryShowKeep {
		return a.sh, a.err
	}
	if r.pastBudget() {
		return metadata.Show{}, errNotChecked
	}
	sh, err := shows.New(r.m.Library).LookupShow(r.ctx, id, defs.Source)
	if err == nil || isNoSuchShow(err) {
		r.m.dryMu.Lock()
		if r.m.dryShows == nil {
			r.m.dryShows = map[string]dryShow{}
		}
		r.m.dryShows[key] = dryShow{sh: sh, err: err, at: time.Now()}
		r.m.dryMu.Unlock()
	}
	return sh, err
}

// errNotChecked: the dry run's time for show lookups ran out first.
var errNotChecked = errors.New("still being checked")

// dryLookupBudget is how long one dry run may spend looking shows up; the
// page asks again after each choice and before the import, and the shows
// not checked in time are looked up after it answers (warm).
var dryLookupBudget = 15 * time.Second

// dryShowKeep is how long a dry run's answer about one show is reused.
const dryShowKeep = time.Hour

type dryShow struct {
	sh  metadata.Show
	err error
	at  time.Time
}

func (r *run) pastBudget() bool { return !r.lookupBy.IsZero() && time.Now().After(r.lookupBy) }

// dryWarming says whether shows a dry run did not check in time are still
// being looked up.
func (m *Mover) dryWarming() bool {
	m.dryMu.Lock()
	defer m.dryMu.Unlock()
	return m.dryWarm
}

// warm looks up, after the dry run answered, the shows it had no time for,
// so the next dry run finds their answers kept. One runs at a time; it
// stops after 30 minutes.
func (r *run) warm(left []TitlePlan) {
	r.m.dryMu.Lock()
	if r.m.dryWarm || len(left) == 0 {
		r.m.dryMu.Unlock()
		return
	}
	r.m.dryWarm = true
	r.m.dryMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		defer func() {
			r.m.dryMu.Lock()
			r.m.dryWarm = false
			r.m.dryMu.Unlock()
		}()
		w := &run{m: r.m, ctx: ctx, dry: true}
		for i := range left {
			if ctx.Err() != nil {
				return
			}
			_ = w.withShowIDs(&left[i], func(sid, _ string) error {
				_, e := w.lookupShow(sid)
				return e
			})
		}
	}()
}

// sameName says whether a show's own name is the folder's, compared the
// way release names are matched, with a country tag like "(US)" set aside;
// when both know a year, it must agree. Its other names do not count: TMDB
// lists "Cunk on Earth" as another name of "Cunk on...", whose season 1 is
// a different show.
func sameName(sh metadata.Show, folder string, year int) bool {
	if fy := folderYearOf(folder); year == 0 {
		year = fy
	}
	if d := year - sh.Year; year != 0 && sh.Year != 0 && (d > 1 || d < -1) {
		return false
	}
	bare := func(s string) string { return strings.TrimSpace(countryTag.ReplaceAllString(folderYear.ReplaceAllString(s, ""), "")) }
	return releasename.SameName(bare(sh.Name), releasename.CleanName(bare(folder)))
}

var countryTag = regexp.MustCompile(`\s*\((?:US|UK|GB|AU|CA|NZ|IE|DE|FR|NL|SE|NO|DK|JP|KR|IN|BR|MX|ES|IT)\)\s*$`)

var yearIn = regexp.MustCompile(`[\(\[](\d{4})[\)\]]`)

// folderYearOf is the year in a folder name like "Doctor Who (2005)", or 0.
func folderYearOf(folder string) int {
	m := yearIn.FindStringSubmatch(folder)
	if m == nil {
		return 0
	}
	y, _ := strconv.Atoi(m[1])
	return y
}

// seasonsNote ends the line of a show found only by the old app's TMDB id
// under another name; files says its folder's files wait to be placed.
func seasonsNote(d, other string, files bool) string {
	if !files {
		d = fmt.Sprintf("%s; found only by the old app's TMDB number, as %s: check that it is the same show, and remove it if it is not. Its seasons may be numbered differently here, so no file is placed on an episode by its numbers. Keep up to date is off and the old app's season choices were not used, so nothing is downloaded until you turn it on", d, other)
		return d
	}
	d = fmt.Sprintf("%s; found only by the old app's TMDB number, as %s: check that it is the same show. If it is, its seasons are numbered differently here, so its files wait in Leftover files for you to place; if it is not, remove it. Keep up to date is off and the old app's season choices were not used, so nothing is downloaded until you turn it on", d, other)
	return d
}

// isNoSuchShow tells a show the source does not know from any other
// failure; an adopted folder's error arrives as text.
func isNoSuchShow(err error) bool {
	return err != nil && (errors.Is(err, metadata.ErrShowNotFound) || strings.Contains(err.Error(), metadata.ErrShowNotFound.Error()))
}

var folderYear = regexp.MustCompile(`\s*[\(\[]\d{4}[\)\]]|\s*[\{\[][^}\]]*[\}\]]`)

func plainName(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(folderYear.ReplaceAllString(s, "")) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c > 127 {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// otherShowIDs are the ids to try after t.ID found nothing, and last the
// old app's TMDB id when none of them is it.
func (r *run) otherShowIDs(t *TitlePlan) (out []string, last string) {
	seen := map[string]bool{t.ID: true}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	defer func() {
		if v := t.IDs["tmdb"]; v != "" && !seen["tmdb:"+v] {
			last = "tmdb:" + v
		}
	}()
	if v := t.IDs["imdb"]; v != "" {
		add("imdb:" + v)
	}
	if r.m.Library == nil || r.m.Library.Shows == nil {
		return out, ""
	}
	// By name last: only a result with the same name (and year, when both
	// know one) counts.
	for _, src := range []string{"tmdb", "tvmaze", "tvdb"} {
		p, err := r.m.Library.Shows.Get(src)
		if err != nil {
			continue
		}
		list, err := p.SearchShows(r.ctx, strings.TrimSpace(folderYear.ReplaceAllString(t.Folder, "")))
		if err != nil {
			continue
		}
		want, found := plainName(t.Folder), false
		for _, sh := range list {
			if plainName(sh.Name) != want || (t.Year != 0 && sh.Year != 0 && sh.Year != t.Year) {
				continue
			}
			for _, k := range []string{"tvdb", "tmdb", "imdb"} {
				if v := sh.IDs[k]; v != "" {
					add(k + ":" + v)
					found = true
				}
			}
		}
		if found {
			break
		}
	}
	return out, ""
}

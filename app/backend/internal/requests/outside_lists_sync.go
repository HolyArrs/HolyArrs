package requests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// maxPages caps how many pages of one list a sync reads, so a huge list
// cannot run forever.
const maxPages = 20

// OutsideLists reads Outside lists and adds their movies or shows to the Library.
type OutsideLists struct {
	Store    *database.Store
	Queue    *jobs.Queue
	Library  *library.Service
	Metadata metadata.Provider
	// TMDBBase and TraktBase default to the real services. Tests point them
	// at stand-ins.
	TMDBBase  string
	TraktBase string
	// TMDBToken and TraktClient read the keys from settings on each sync.
	// The keys go only in request headers and are never logged.
	TMDBToken   func(ctx context.Context) (string, error)
	TraktClient func(ctx context.Context) (string, error)
	// TMDBRoute, when set, says where TMDB lists are read now (the metadata
	// mode): the address, the credential and the client, in place of
	// TMDBBase, TMDBToken and HTTP. metadata.ErrSourceOff: lookups are off.
	TMDBRoute func(ctx context.Context) (string, func(context.Context) (string, error), *http.Client, error)
	HTTP      *http.Client
	Now       func() time.Time
}

// ErrListsOff means TMDB lists are not read: lookups are off on this install.
var ErrListsOff = errors.New("TMDB lists are off on this install")

// OutsideListResult counts what one sync of one list did.
type OutsideListResult struct {
	Found    int      `json:"found"`
	Added    int      `json:"added"`
	Already  int      `json:"already"`
	NeverAdd int      `json:"never_add"`
	Failed   int      `json:"failed"`
	Skipped  int      `json:"skipped"`
	Names    []string `json:"added_names"`
}

// entry is one movie or show on a list, with the ids the list gave.
type entry struct{ tmdb, imdb, tvdb, name string }

// Register adds the list Job handler to the queue.
func (s *OutsideLists) Register(q *jobs.Queue) { q.Handle(OutsideListSync, s.handle) }

// ProbeSync reports whether any enabled list is due. It uses the same test
// as the Job handler and writes nothing.
func (s *OutsideLists) ProbeSync(ctx context.Context, now time.Time) (bool, any, error) {
	ls, err := AllOutsideLists(ctx, s.Store.Read())
	if err != nil {
		return true, nil, err
	}
	for _, l := range ls {
		if due(l, now.UTC()) {
			return true, nil, nil
		}
	}
	return false, nil, nil
}

// Sync reads one list now and adds its new movies or shows. A failed read is
// returned and saved on the list; a title that fails to add only counts in
// the result.
func (s *OutsideLists) Sync(ctx context.Context, id int64, by domain.Actor) (OutsideListResult, error) {
	res := OutsideListResult{Names: []string{}}
	l, err := GetOutsideList(ctx, s.Store.Read(), id)
	if err != nil {
		return res, err
	}
	var es []entry
	switch l.Kind {
	case ListTMDB:
		es, err = s.fetchTMDB(ctx, l.Ref, l.TitleKind)
	case ListTrakt:
		es, err = s.fetchTrakt(ctx, l.Ref, l.TitleKind)
	default:
		err = ErrKind
	}
	if err != nil {
		s.record(ctx, l, res, err.Error(), by)
		return res, err
	}
	res.Found = len(es)
	var firstFail string
	add := s.addOne
	if l.TitleKind == "show" {
		add = s.addShow
	}
	for _, e := range es {
		switch outcome, name, err := add(ctx, l, e, by); outcome {
		case "added":
			res.Added++
			res.Names = append(res.Names, name)
		case "already":
			res.Already++
		case "never_add":
			res.NeverAdd++
		case "skipped":
			res.Skipped++
		default:
			res.Failed++
			if firstFail == "" {
				firstFail = fmt.Sprintf("%s: %v", e.name, err)
			}
		}
	}
	s.record(ctx, l, res, firstFail, by)
	return res, nil
}

// now returns the current time in UTC, from the test clock when one is set.
func (s *OutsideLists) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// client returns the configured HTTP client, or one with a 15 second timeout.
func (s *OutsideLists) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// handle reads one list when the Job has a list_id, or every due list on the
// schedule. One failing list does not stop the others.
func (s *OutsideLists) handle(ctx context.Context, r *jobs.Run) error {
	by := domain.Actor{Kind: "job", ID: r.Job.ID}
	if v, ok := r.Job.Params["list_id"].(float64); ok && v > 0 {
		res, err := s.Sync(ctx, int64(v), by)
		out := map[string]any{"list_id": int64(v), "result": res}
		if err != nil && !errors.Is(err, database.ErrNotFound) {
			// The problem is shown on the list. A retry with the same key or
			// link would fail the same way, so the Job still succeeds.
			out["problem"] = err.Error()
		}
		r.SetResult(out)
		return nil
	}
	ls, err := AllOutsideLists(ctx, s.Store.Read())
	if err != nil {
		return err
	}
	now := s.now()
	read, failed := 0, 0
	for _, l := range ls {
		if !due(l, now) {
			continue
		}
		read++
		if _, err := s.Sync(ctx, l.ID, by); err != nil {
			failed++
			log.Printf("lists: %s: %v", l.Name, err)
		}
	}
	r.SetResult(map[string]any{"lists_read": read, "lists_failed": failed})
	return nil
}

// addOne adds one list movie unless it is already in the Library or on the
// Never-add list. It returns the outcome word, the title name and any error.
func (s *OutsideLists) addOne(ctx context.Context, l domain.OutsideList, e entry, by domain.Actor) (string, string, error) {
	db := s.Store.Read()
	lookup := ""
	switch {
	case e.tmdb != "":
		lookup = "tmdb:" + e.tmdb
		if id, err := library.TitleByTMDB(ctx, db, e.tmdb); err != nil {
			return "failed", "", err
		} else if id != 0 {
			return "already", "", nil
		}
		if on, err := library.OnNeverAdd(ctx, db, e.tmdb); err != nil {
			return "failed", "", err
		} else if on {
			return "never_add", "", nil
		}
	case e.imdb != "":
		lookup = "imdb:" + e.imdb
	default:
		return "skipped", "", nil
	}
	m, err := s.Metadata.Movie(ctx, lookup)
	if err != nil {
		return "failed", "", err
	}
	opt := library.AddOptions{LibraryFolderID: l.LibraryFolderID, KeepUpToDate: &l.KeepUpToDate, ReleaseGate: l.ReleaseGate,
		SearchOnAdd: l.SearchOnAdd, WhyAdded: "list"}
	if l.PickRulesID != nil {
		opt.PickRulesID = *l.PickRulesID
	}
	_, _, err = s.Library.Add(ctx, m, opt, by)
	var dup *library.AlreadyAddedError
	switch {
	case err == nil:
		return "added", m.Name, nil
	case errors.As(err, &dup):
		return "already", "", nil
	case errors.Is(err, library.ErrNeverAdd):
		return "never_add", "", nil
	}
	return "failed", "", err
}

// addShow adds one list show unless it is already in the Library or on the
// Never-add list, using the list's folder, rules, seasons and search choice.
// It returns the outcome word, the title name and any error.
func (s *OutsideLists) addShow(ctx context.Context, l domain.OutsideList, e entry, by domain.Actor) (string, string, error) {
	ids := map[string]string{}
	for k, v := range map[string]string{"tmdb": e.tmdb, "tvdb": e.tvdb, "imdb": e.imdb} {
		if v != "" {
			ids[k] = v
		}
	}
	lookup := ""
	for _, k := range []string{"tmdb", "tvdb", "imdb"} {
		if ids[k] != "" {
			lookup = k + ":" + ids[k]
			break
		}
	}
	if lookup == "" {
		return "skipped", "", nil
	}
	db := s.Store.Read()
	if id, err := library.ShowByIDs(ctx, db, ids); err != nil {
		return "failed", "", err
	} else if id != 0 {
		return "already", "", nil
	}
	if on, err := library.ShowOnNeverAdd(ctx, db, ids); err != nil {
		return "failed", "", err
	} else if on {
		return "never_add", "", nil
	}
	opt := shows.ShowAddOptions{LibraryFolderID: l.LibraryFolderID, KeepUpToDate: &l.KeepUpToDate, Seasons: l.Seasons,
		NewSeasons: l.NewSeasons, SearchOnAdd: l.SearchOnAdd, WhyAdded: "list"}
	if l.Seasons == SeasonsFirst {
		opt.Seasons, opt.FirstSeason = "", true
	}
	if l.PickRulesID != nil {
		opt.PickRulesID = *l.PickRulesID
	}
	id, _, err := shows.New(s.Library).AddShow(ctx, lookup, opt, by)
	var dup *shows.ShowAlreadyAddedError
	switch {
	case err == nil:
		name := e.name
		_ = db.QueryRowContext(ctx, addShowSQL, id).Scan(&name)
		return "added", name, nil
	case errors.As(err, &dup):
		return "already", "", nil
	case errors.Is(err, shows.ErrShowNeverAdd):
		return "never_add", "", nil
	}
	return "failed", "", err
}

// record saves the outcome on the list and in History, then tells open pages.
func (s *OutsideLists) record(ctx context.Context, l domain.OutsideList, res OutsideListResult, problem string, by domain.Actor) {
	now := s.now()
	text := describe(res)
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(recordSQL,
			database.FormatTime(now), text, problem, database.FormatTime(now), l.ID)
		return err
	})
	if err != nil {
		log.Printf("lists: saving the result of %s: %v", l.Name, err)
	}
	summary := "Read the list " + l.Name + ": " + text
	if problem != "" {
		summary = "Could not fully read the list " + l.Name + ": " + problem
	}
	e := &domain.Event{Actor: by, Kind: domain.EventListSynced, Summary: summary,
		Data: map[string]any{"list_id": l.ID, "kind": l.Kind, "result": res, "problem": problem}}
	if err := s.Store.AddEvent(ctx, e); err != nil {
		log.Printf("lists: event: %v", err)
	}
	if s.Library != nil && s.Library.Live != nil {
		s.Library.Live.Publish("outside_list", map[string]any{"id": l.ID})
	}
}

// get fetches one page and decodes it into into. The identity keeps retry
// state per service and key. The body is capped at 4 MiB.
func (s *OutsideLists) get(ctx context.Context, what, endpoint, u string, head map[string]string, into any) (http.Header, error) {
	return s.getWith(ctx, s.client(), what, endpoint, u, head, into)
}

// getWith is get through the client cl.
func (s *OutsideLists) getWith(ctx context.Context, cl *http.Client, what, endpoint, u string, head map[string]string, into any) (http.Header, error) {
	identity := []string{what, endpoint}
	for _, k := range []string{"Authorization", "trakt-api-key", "X-Trakt-Client-Id"} {
		identity = append(identity, head[k])
	}
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "outside_list", Identity: identity})
	if err != nil {
		return nil, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	rctx, cancel := op.AttemptContext(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, u, nil)
	if err != nil {
		outcome = outbound.TerminalFailure
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range head {
		req.Header.Set(k, v)
	}
	resp, err := cl.Do(req)
	if err != nil {
		if errors.Is(err, metadata.ErrSourceOff) {
			return nil, ErrListsOff
		}
		outcome = outbound.HTTPOutcome(0, err, ctx)
		return nil, fmt.Errorf("%s did not answer", what)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if closeErr := resp.Body.Close(); readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		outcome = outbound.HTTPOutcome(resp.StatusCode, readErr, ctx)
		return nil, fmt.Errorf("%s sent an answer that could not be read", what)
	}
	outcome = outbound.HTTPOutcome(resp.StatusCode, nil, ctx)
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%s refused the key; check it in Settings", what)
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s has no such list, or it is private", what)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s answered %d", what, resp.StatusCode)
	}
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(into); err != nil {
		if rctx.Err() != nil && ctx.Err() == nil {
			outcome = outbound.TransientFailure
		} else {
			outcome = outbound.TerminalFailure
		}
		return nil, fmt.Errorf("%s sent something that is not a list", what)
	}
	outcome = outbound.Success
	return resp.Header, nil
}

// fetchTMDB reads a TMDB list. A list can mix movies and shows, so only the
// ones of the list's kind are kept.
func (s *OutsideLists) fetchTMDB(ctx context.Context, ref, titleKind string) ([]entry, error) {
	base, token, cl := s.TMDBBase, s.TMDBToken, s.client()
	if s.TMDBRoute != nil {
		var err error
		if base, token, cl, err = s.TMDBRoute(ctx); err != nil {
			if errors.Is(err, metadata.ErrSourceOff) {
				return nil, ErrListsOff
			}
			return nil, err
		}
		if cl == nil {
			cl = s.client()
		}
	}
	if token == nil {
		return nil, metadata.ErrNoAccess
	}
	tok, err := token(ctx)
	if err != nil || tok == "" {
		return nil, metadata.ErrNoAccess
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		base = "https://api.themoviedb.org/3"
	}
	var out []entry
	for page := 1; page <= maxPages; page++ {
		var body struct {
			TotalPages int `json:"total_pages"`
			Items      []struct {
				ID        int64  `json:"id"`
				MediaType string `json:"media_type"`
				Title     string `json:"title"`
				Name      string `json:"name"`
			} `json:"items"`
		}
		u := base + "/list/" + url.PathEscape(ref) + "?page=" + strconv.Itoa(page)
		if _, err := s.getWith(ctx, cl, "TMDB", base, u, map[string]string{"Authorization": "Bearer " + tok}, &body); err != nil {
			return nil, err
		}
		for _, it := range body.Items {
			switch {
			case titleKind == "show" && it.MediaType == "tv":
				out = append(out, entry{tmdb: strconv.FormatInt(it.ID, 10), name: it.Name})
			case titleKind != "show" && (it.MediaType == "" || it.MediaType == "movie"):
				out = append(out, entry{tmdb: strconv.FormatInt(it.ID, 10), name: it.Title})
			}
		}
		if page >= body.TotalPages {
			break
		}
	}
	return out, nil
}

// fetchTrakt reads a Trakt list or watchlist: its movies, or its shows for a
// show list. Trakt gives the page count in a response header.
func (s *OutsideLists) fetchTrakt(ctx context.Context, ref, titleKind string) ([]entry, error) {
	if s.TraktClient == nil {
		return nil, ErrNoTraktKey
	}
	key, err := s.TraktClient(ctx)
	if err != nil || key == "" {
		return nil, ErrNoTraktKey
	}
	user, slug, _ := strings.Cut(ref, "/")
	base := strings.TrimRight(s.TraktBase, "/")
	if base == "" {
		base = "https://api.trakt.tv"
	}
	one := "movie"
	if titleKind == "show" {
		one = "show"
	}
	path := "/users/" + url.PathEscape(user) + "/lists/" + url.PathEscape(slug) + "/items/" + one
	if slug == "watchlist" {
		path = "/users/" + url.PathEscape(user) + "/watchlist/" + one + "s"
	}
	head := map[string]string{"trakt-api-version": "2", "trakt-api-key": key, "Content-Type": "application/json"}
	var out []entry
	for page := 1; page <= maxPages; page++ {
		type item struct {
			Title string `json:"title"`
			IDs   struct {
				TMDB int64  `json:"tmdb"`
				TVDB int64  `json:"tvdb"`
				IMDB string `json:"imdb"`
			} `json:"ids"`
		}
		var body []struct {
			Type  string `json:"type"`
			Movie *item  `json:"movie"`
			Show  *item  `json:"show"`
		}
		h, err := s.get(ctx, "Trakt", base, base+path+"?limit=100&page="+strconv.Itoa(page), head, &body)
		if err != nil {
			return nil, err
		}
		for _, it := range body {
			x := it.Movie
			if titleKind == "show" {
				x = it.Show
			}
			if x == nil {
				continue
			}
			e := entry{imdb: x.IDs.IMDB, name: x.Title}
			if x.IDs.TMDB > 0 {
				e.tmdb = strconv.FormatInt(x.IDs.TMDB, 10)
			}
			if titleKind == "show" && x.IDs.TVDB > 0 {
				e.tvdb = strconv.FormatInt(x.IDs.TVDB, 10)
			}
			out = append(out, e)
		}
		pages, _ := strconv.Atoi(h.Get("X-Pagination-Page-Count"))
		if page >= pages {
			break
		}
	}
	return out, nil
}

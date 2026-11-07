package lists

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

	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// maxPages caps how much of one list is read in one sync.
const maxPages = 20

// Service reads Outside lists and adds their movies or shows.
type Service struct {
	Store    *store.Store
	Queue    *jobs.Queue
	Library  *library.Service
	Metadata metadata.Provider
	// TMDBBase and TraktBase default to the real services; tests and the
	// browser tests point them at stand-ins.
	TMDBBase  string
	TraktBase string
	// TMDBToken and TraktClient read the keys from the encrypted settings on
	// each sync. They are sent only in headers and never logged.
	TMDBToken   func(ctx context.Context) (string, error)
	TraktClient func(ctx context.Context) (string, error)
	HTTP        *http.Client
	Now         func() time.Time
}

// Result is what one sync of one list did.
type Result struct {
	Found    int      `json:"found"`
	Added    int      `json:"added"`
	Already  int      `json:"already"`
	NeverAdd int      `json:"never_add"`
	Failed   int      `json:"failed"`
	Skipped  int      `json:"skipped"`
	Names    []string `json:"added_names"`
}

// entry is one movie or show on a list.
type entry struct{ tmdb, imdb, tvdb, name string }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// Register adds the list Job handler.
func (s *Service) Register(q *jobs.Queue) { q.Handle(KindSync, s.handle) }

// ProbeSync reports whether the scheduled list Job has a due enabled list.
// It uses the same due predicate as handle and performs no writes.
func (s *Service) ProbeSync(ctx context.Context, now time.Time) (bool, any, error) {
	ls, err := All(ctx, s.Store.Read())
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

// handle reads one list (params list_id, from a person or a save) or every
// enabled list that is due (the schedule). One failing list does not stop
// the others.
func (s *Service) handle(ctx context.Context, r *jobs.Run) error {
	by := domain.Actor{Kind: "job", ID: r.Job.ID}
	if v, ok := r.Job.Params["list_id"].(float64); ok && v > 0 {
		res, err := s.Sync(ctx, int64(v), by)
		out := map[string]any{"list_id": int64(v), "result": res}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			// The problem is on the list for people to see; trying again
			// with the same key or link would not help.
			out["problem"] = err.Error()
		}
		r.SetResult(out)
		return nil
	}
	ls, err := All(ctx, s.Store.Read())
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

// Sync reads one list now and adds its new movies or shows.
func (s *Service) Sync(ctx context.Context, id int64, by domain.Actor) (Result, error) {
	res := Result{Names: []string{}}
	l, err := Get(ctx, s.Store.Read(), id)
	if err != nil {
		return res, err
	}
	var es []entry
	switch l.Kind {
	case KindTMDB:
		es, err = s.fetchTMDB(ctx, l.Ref, l.TitleKind)
	case KindTrakt:
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

// addOne adds one list movie unless it is already there or on the Never-add list.
func (s *Service) addOne(ctx context.Context, l domain.OutsideList, e entry, by domain.Actor) (string, string, error) {
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

// addShow adds one list show unless it is already there or on the
// Never-add list, with the list's season choice (Sonarr's list add: the
// list's folder, rules, monitoring and search-on-add).
func (s *Service) addShow(ctx context.Context, l domain.OutsideList, e entry, by domain.Actor) (string, string, error) {
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
	opt := library.ShowAddOptions{LibraryFolderID: l.LibraryFolderID, KeepUpToDate: &l.KeepUpToDate, Seasons: l.Seasons,
		NewSeasons: l.NewSeasons, SearchOnAdd: l.SearchOnAdd, WhyAdded: "list"}
	if l.Seasons == SeasonsFirst {
		opt.Seasons, opt.FirstSeason = "", true
	}
	if l.PickRulesID != nil {
		opt.PickRulesID = *l.PickRulesID
	}
	id, _, err := s.Library.AddShow(ctx, lookup, opt, by)
	var dup *library.ShowAlreadyAddedError
	switch {
	case err == nil:
		name := e.name
		_ = db.QueryRowContext(ctx, "SELECT name FROM titles WHERE id = ?", id).Scan(&name)
		return "added", name, nil
	case errors.As(err, &dup):
		return "already", "", nil
	case errors.Is(err, library.ErrShowNeverAdd):
		return "never_add", "", nil
	}
	return "failed", "", err
}

// record saves the outcome on the list and in History.
func (s *Service) record(ctx context.Context, l domain.OutsideList, res Result, problem string, by domain.Actor) {
	now := s.now()
	text := describe(res)
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE outside_lists SET last_synced_at = ?, last_result = ?, last_error = ?, updated_at = ? WHERE id = ?",
			store.FormatTime(now), text, problem, store.FormatTime(now), l.ID)
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

// get fetches one page and decodes it, with the key only in headers.
func (s *Service) get(ctx context.Context, what, endpoint, u string, head map[string]string, into any) (http.Header, error) {
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
	resp, err := s.client().Do(req)
	if err != nil {
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

// fetchTMDB reads a TMDB list. A list may hold movies and shows together;
// only the ones of the list's kind are kept.
func (s *Service) fetchTMDB(ctx context.Context, ref, titleKind string) ([]entry, error) {
	if s.TMDBToken == nil {
		return nil, metadata.ErrNoAccess
	}
	tok, err := s.TMDBToken(ctx)
	if err != nil || tok == "" {
		return nil, metadata.ErrNoAccess
	}
	base := strings.TrimRight(s.TMDBBase, "/")
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
		if _, err := s.get(ctx, "TMDB", base, u, map[string]string{"Authorization": "Bearer " + tok}, &body); err != nil {
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

// fetchTrakt reads a Trakt list or watchlist: its movies, or its shows for
// a show list.
func (s *Service) fetchTrakt(ctx context.Context, ref, titleKind string) ([]entry, error) {
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

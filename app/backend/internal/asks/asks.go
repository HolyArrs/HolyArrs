// Package asks runs Requests for movies, shows and music: people ask (a
// show whole or by seasons, an artist whole or by albums), a second ask for the same thing joins the first, an owner or admin approves or turns it
// down (or auto-approve does), approving adds the Title and starts the
// search, and the people who asked are told when it is approved, turned
// down and ready. Each person may ask for a number of movies per week.
package asks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/ratings"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
	"github.com/HolyArrs/holyarrs/app/backend/internal/supervisor"
)

// Window is how far back the weekly limit looks (a rolling week).
const Window = 7 * 24 * time.Hour

// Errors from asking and deciding.
var (
	ErrNotOpen    = errors.New("this ask was already decided")
	ErrAskedAgain = errors.New("someone asked for this movie again; decide the open ask instead")
	ErrNotYours   = errors.New("only the person who asked can take it back")
	ErrNoteLength = errors.New("a note can be at most 500 characters")
	ErrPerWeek    = errors.New("the weekly limit must be between 0 (no limit) and 1000")
)

// InLibraryError says the movie (or the asked-for seasons of a show) is
// already in the Library.
type InLibraryError struct {
	TitleID int64
	Ready   bool // a file is there (for a show: every aired episode asked for)
	Show    bool
	Music   bool // an artist (every track of the albums asked for)
}

func (e *InLibraryError) Error() string {
	what := "this movie is"
	if e.Show {
		what = "this show (the seasons you asked for) is"
	}
	if e.Music {
		what = "this artist (the albums you asked for) is"
		if e.Ready {
			return what + " already in the Library, ready to listen to"
		}
	}
	if e.Ready {
		return what + " already in the Library, ready to watch"
	}
	return what + " already in the Library; HolyArrs is looking for it"
}

// The kinds a weekly limit counts. Each person has one limit per kind.
const (
	KindMovies = "movies" // movie asks
	KindShows  = "shows"  // show asks (one per ask, whatever its seasons)
	KindMusic  = "music"  // artist and album asks (one per ask, whatever its albums)
)

// Kinds lists the limit kinds in the order screens show them.
var Kinds = []string{KindMovies, KindShows, KindMusic}

// kindWord names asks of a kind: "movie", "shows", "music asks".
func kindWord(kind string, n int) string {
	w := map[string]string{KindMovies: "movie", KindShows: "show", KindMusic: "music ask"}[kind]
	if w == "" {
		w = "movie"
	}
	if n != 1 {
		w += "s"
	}
	return w
}

// LimitError says the person has used up their asks of one kind for the week.
type LimitError struct {
	Kind    string // KindMovies, KindShows or KindMusic
	PerWeek int
	RetryAt time.Time
	now     time.Time
}

func (e *LimitError) Error() string {
	limit := strings.TrimSuffix(kindWord(e.Kind, 1), " ask")
	return fmt.Sprintf("you have asked for %d %s in the last 7 days, which is your %s limit; try again in %s",
		e.PerWeek, kindWord(e.Kind, e.PerWeek), limit, Wait(e.RetryAt.Sub(e.now)))
}

// Wait says a duration in words, such as "2 days and 3 hours" or "40 minutes".
func Wait(d time.Duration) string {
	if d <= time.Minute {
		return "a minute"
	}
	// Round up to the smallest unit shown, so the wait is never said
	// shorter than it is (6 days and 23 hours 59 minutes is 7 days).
	if d > 24*time.Hour {
		d = (d + time.Hour - 1).Truncate(time.Hour)
	} else {
		d = (d + time.Minute - 1).Truncate(time.Minute)
	}
	plural := func(n int, w string) string {
		if n == 1 {
			return "1 " + w
		}
		return strconv.Itoa(n) + " " + w + "s"
	}
	days, hours, mins := int(d/(24*time.Hour)), int(d%(24*time.Hour)/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case days > 0 && hours > 0:
		return plural(days, "day") + " and " + plural(hours, "hour")
	case days > 0:
		return plural(days, "day")
	case hours > 0 && mins > 0:
		return plural(hours, "hour") + " and " + plural(mins, "minute")
	case hours > 0:
		return plural(hours, "hour")
	}
	return plural(mins, "minute")
}

// Notifier sends Alerts; *alerts.Sender is one.
type Notifier interface {
	Send(ctx context.Context, m alerts.Message) error
}

// Service runs asks.
type Service struct {
	Store    *store.Store
	Library  *library.Service
	Metadata metadata.Provider
	// Alerts sends Discord and email Alerts (nil sends none).
	Alerts Notifier
	// Live tells staff pages about changes (may be nil).
	Live *webserver.LiveHub
	// Now is the clock (time.Now when nil).
	Now func() time.Time
	// PrefetchParts stores an ask's episode and song lists in the
	// background when it is asked for (off in tests).
	PrefetchParts bool

	sendMu   sync.Mutex
	sendTail <-chan struct{}
	sending  sync.WaitGroup
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Person is who acts: their user id, name and what they may do.
type Person struct {
	UserID      int64
	Name        string
	AutoApprove bool // their asks are approved straight away
	NoLimit     bool // the weekly limit does not apply (people who approve)
	// MaxRating is a Kid's age-rating limit ("" for none): they may ask
	// only for movies and shows rated up to it, and never for music.
	MaxRating string
}

// ErrAboveLimit is behind every ask refused for the asker's age-rating
// limit; the error itself says why in plain words.
var ErrAboveLimit = errors.New("above the age-rating limit")

type limitRefusal string

func (e limitRefusal) Error() string { return string(e) }
func (e limitRefusal) Unwrap() error { return ErrAboveLimit }

// overLimit refuses a movie or show rated rating when p's limit does not
// allow it; nil when it does.
func overLimit(p Person, kind, rating string) error {
	if ratings.Allows(p.MaxRating, kind, rating) {
		return nil
	}
	if kind == "artist" {
		return limitRefusal("This account cannot ask for music, because music has no age ratings.")
	}
	return limitRefusal("This account can ask only for titles rated " + ratings.Up(p.MaxRating) + ".")
}

// libraryOverLimit refuses when the Library title id (0: none) is rated
// above p's limit.
func libraryOverLimit(ctx context.Context, db store.Querier, p Person, kind string, id int64) error {
	if id == 0 {
		return nil
	}
	var rating string
	if err := db.QueryRowContext(ctx, "SELECT rating FROM titles WHERE id = ?", id).Scan(&rating); err != nil {
		return err
	}
	return overLimit(p, kind, rating)
}

func (p Person) actor() domain.Actor { return domain.Actor{Kind: "user", ID: p.UserID} }

// Result is what asking did.
type Result struct {
	Ask     domain.Ask
	Joined  bool // someone had asked already; this person was added
	Already bool // this person had asked already
}

// KindLimit is a person's weekly limit for one kind and what is used of it.
type KindLimit struct {
	PerWeek int        `json:"per_week"` // 0: no limit
	Used    int        `json:"used"`
	Left    int        `json:"left"`
	RetryAt *time.Time `json:"retry_at,omitempty"` // when the next ask frees up, if none are left
	NoLimit bool       `json:"no_limit"`
}

// Limit is a person's weekly limits, one per kind.
type Limit struct {
	NoLimit bool      `json:"no_limit"` // no limit applies at all (people who approve)
	Movies  KindLimit `json:"movies"`
	Shows   KindLimit `json:"shows"`
	Music   KindLimit `json:"music"`
}

// Of gives the limit for one kind.
func (l Limit) Of(kind string) KindLimit {
	switch kind {
	case KindShows:
		return l.Shows
	case KindMusic:
		return l.Music
	}
	return l.Movies
}

// LimitOf reads a person's limits and use.
func (s *Service) LimitOf(ctx context.Context, p Person) (Limit, error) {
	if p.NoLimit {
		no := KindLimit{NoLimit: true}
		return Limit{NoLimit: true, Movies: no, Shows: no, Music: no}, nil
	}
	db := s.Store.Read()
	var l Limit
	var err error
	if l.Movies, err = s.kindLimit(ctx, db, p, KindMovies); err != nil {
		return l, err
	}
	if l.Shows, err = s.kindLimit(ctx, db, p, KindShows); err != nil {
		return l, err
	}
	l.Music, err = s.kindLimit(ctx, db, p, KindMusic)
	return l, err
}

// checkLimit says whether p may make one more ask of this kind now: nil, or
// a *LimitError. Inside a write it counts what that write sees, so asks
// sent at the same moment cannot all pass.
func (s *Service) checkLimit(ctx context.Context, db querier, p Person, kind string) error {
	if p.NoLimit {
		return nil
	}
	l, err := s.kindLimit(ctx, db, p, kind)
	if err != nil {
		return err
	}
	if l.NoLimit || l.Left > 0 {
		return nil
	}
	return &LimitError{Kind: kind, PerWeek: l.PerWeek, RetryAt: *l.RetryAt, now: s.now()}
}

// querier reads from the database or from inside a write.
type querier interface {
	store.Lister
	store.Querier
}

// PersonLimits reads one person's own weekly limit per kind; nil uses the
// default. The older all-kinds value (per_week) counts for each kind that
// has no value of its own.
func PersonLimits(ctx context.Context, db store.Querier, userID int64) (movies, shows, music *int, err error) {
	var all, m, sh, mu sql.NullInt64
	err = db.QueryRowContext(ctx, "SELECT per_week, movies_per_week, shows_per_week, music_per_week FROM user_ask_settings WHERE user_id = ?", userID).Scan(&all, &m, &sh, &mu)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	pick := func(own sql.NullInt64) *int {
		if !own.Valid {
			own = all
		}
		if !own.Valid {
			return nil
		}
		n := int(own.Int64)
		return &n
	}
	return pick(m), pick(sh), pick(mu), nil
}

// askKinds is the ask kind each limit kind counts.
var askKinds = map[string]string{KindMovies: "movie", KindShows: "show", KindMusic: "artist"}

func (s *Service) kindLimit(ctx context.Context, db querier, p Person, kind string) (KindLimit, error) {
	if p.NoLimit {
		return KindLimit{NoLimit: true}, nil
	}
	set, err := ReadSettings(ctx, db)
	if err != nil {
		return KindLimit{}, err
	}
	l := KindLimit{PerWeek: set.PerWeekOf(kind)}
	movies, shows, music, err := PersonLimits(ctx, db, p.UserID)
	if err != nil {
		return l, err
	}
	if own := map[string]*int{KindMovies: movies, KindShows: shows, KindMusic: music}[kind]; own != nil {
		l.PerWeek = *own
	}
	now := s.now()
	// Only the person's own asks count (counted = 1): joining someone
	// else's ask is free, and a turned down ask frees its place.
	rows, err := db.QueryContext(ctx, `SELECT p.asked_at FROM ask_people p JOIN asks a ON a.id = p.ask_id
		WHERE p.user_id = ? AND p.counted = 1 AND p.asked_at > ? AND a.state != 'turned_down' AND a.kind = ? ORDER BY p.asked_at`,
		p.UserID, store.FormatTime(now.Add(-Window)), askKinds[kind])
	if err != nil {
		return l, err
	}
	defer rows.Close()
	var times []time.Time
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return l, err
		}
		at, _ := time.Parse(store.TimeLayout, t)
		times = append(times, at)
	}
	if err := rows.Err(); err != nil {
		return l, err
	}
	l.Used = len(times)
	if l.PerWeek == 0 {
		l.NoLimit = true
		return l, nil
	}
	l.Left = max(0, l.PerWeek-l.Used)
	if l.Left == 0 && l.Used > 0 {
		at := times[l.Used-l.PerWeek].Add(Window)
		l.RetryAt = &at
	}
	return l, nil
}

func (s *Service) event(ctx context.Context, e *domain.Event) {
	if err := s.Store.AddEvent(ctx, e); err != nil {
		log.Printf("asks: event %s: %v", e.Kind, err)
		return
	}
	if s.Live == nil {
		return
	}
	// An ask's Event names who asked and who decided, with the decline
	// note: only people who approve read those, as on the asks routes.
	if strings.HasPrefix(e.Kind, "ask_") {
		s.Live.PublishApprovers("event", e)
		return
	}
	s.Live.Publish("event", e)
}

func (s *Service) changed(ctx context.Context, id int64) {
	s.storePartsLater(id)
	if s.Live == nil {
		return
	}
	vs, err := s.Views(ctx, "a.id = ?", id)
	if err != nil || len(vs) != 1 {
		return
	}
	s.AddSteps(ctx, vs, true)
	s.AddAlsoDeclined(ctx, vs)
	v := vs[0]
	// Everyone's asks, decline notes and names go to the people who
	// approve, as the asks routes show them (#139), who else declined too
	// (#136), so a live copy never drops what the list read showed.
	s.Live.PublishApprovers("ask", v)
	// The people who asked follow their own ask live too (#139), told who
	// decided by display name only, as their Requests list reads it.
	mine := v
	mine.Mine, mine.AlsoDeclined = true, nil
	if v.Step != nil {
		st := *v.Step
		st.Text = ""
		mine.Step = &st
	}
	if v.DecidedBy != nil {
		d := *v.DecidedBy
		d.Name = ""
		mine.DecidedBy = &d
	}
	for _, p := range v.People {
		s.Live.PublishUser("ask", mine, p.ID)
	}
}

// shownAs is how the people who asked read who decided: the display name
// the decider chose, else "an admin" (their sign-in name is never told).
func (s *Service) shownAs(ctx context.Context, by Person) string {
	var n string
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT display_name FROM user_profiles WHERE user_id = ?", by.UserID).Scan(&n)
	if n = strings.TrimSpace(n); n != "" {
		return n
	}
	return "an admin"
}

func get(ctx context.Context, q store.Querier, id int64) (domain.Ask, error) {
	var a domain.Ask
	err := store.Get(ctx, q, "asks", id, &a)
	return a, err
}

// Ask asks for a movie by TMDB id for p.
func (s *Service) Ask(ctx context.Context, p Person, tmdbID string) (Result, error) {
	return s.ask(ctx, p, tmdbID, false, false)
}

// Ask4K asks for a 4K copy of a movie by TMDB id for p. It is its own ask,
// approved apart from a plain ask, and may be made for a movie already in
// the Library that has no 4K Version yet.
func (s *Service) Ask4K(ctx context.Context, p Person, tmdbID string) (Result, error) {
	return s.ask(ctx, p, tmdbID, true, false)
}

// fourKVersion finds the title's 4K Version (labelled 4K or on the 4K
// Pick rules), 0 when it has none.
func fourKVersion(ctx context.Context, db store.Querier, titleID int64) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, `SELECT v.id FROM versions v LEFT JOIN pick_rules p ON p.id = v.pick_rules_id
		WHERE v.title_id = ? AND (v.label = ? COLLATE NOCASE OR p.name = ?) ORDER BY v.id LIMIT 1`, titleID, domain.FourKName, domain.FourKName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// HasFourK says the title already has a 4K Version (a 4K ask is refused then).
func HasFourK(ctx context.Context, db store.Querier, titleID int64) (bool, error) {
	id, err := fourKVersion(ctx, db, titleID)
	return id != 0, err
}

// fourKRules is the 4K Pick rules preset's id (0 when it was removed).
func fourKRules(ctx context.Context, db store.Querier) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, "SELECT id FROM pick_rules WHERE name = ? ORDER BY id LIMIT 1", domain.FourKName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// ErrNoFourKRules: a 4K ask cannot be approved without the 4K Pick rules.
var ErrNoFourKRules = errors.New("no 4K Pick rules: add Pick rules named 4K first")

// openAskConflict says err is the one-open-ask-per-movie index refusing a
// second open ask (asks_one_open on kind and tmdb_id).
func openAskConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: asks.kind, asks.tmdb_id")
}

func (s *Service) ask(ctx context.Context, p Person, tmdbID string, fourK, retried bool) (Result, error) {
	db := s.Store.Read()
	tmdbID = strings.TrimSpace(tmdbID)
	if p.MaxRating != "" {
		// Checked first, so a Kid neither joins an open ask nor learns that
		// the movie is in the Library. The Library's rating is checked
		// first, then the provider's; either above the limit refuses.
		tid, err := library.TitleByTMDB(ctx, db, tmdbID)
		if err != nil {
			return Result{}, err
		}
		if err := libraryOverLimit(ctx, db, p, "movie", tid); err != nil {
			return Result{}, err
		}
		m, err := s.Metadata.Movie(ctx, "tmdb:"+tmdbID)
		if err != nil {
			return Result{}, err
		}
		if err := overLimit(p, "movie", m.Rating); err != nil {
			return Result{}, err
		}
	}
	// An open ask for the same movie: join it.
	var openID int64
	err := db.QueryRowContext(ctx, "SELECT id FROM asks WHERE kind = 'movie' AND tmdb_id = ? AND four_k = ? AND state IN ('asked', 'approved')", tmdbID, fourK).Scan(&openID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Result{}, err
	}
	if openID != 0 {
		return s.join(ctx, p, openID)
	}
	if tid, err := library.TitleByTMDB(ctx, db, tmdbID); err != nil {
		return Result{}, err
	} else if tid != 0 {
		// A 4K ask is fine for a movie in the Library without a 4K Version.
		vid := int64(0)
		if fourK {
			if vid, err = fourKVersion(ctx, db, tid); err != nil {
				return Result{}, err
			}
		}
		if !fourK || vid != 0 {
			q, args := "SELECT count(*) FROM slots WHERE state = 'in_library' AND item_id IN (SELECT id FROM items WHERE title_id = ?)", []any{tid}
			if fourK {
				q, args = q+" AND version_id = ?", append(args, vid)
			}
			var n int
			_ = db.QueryRowContext(ctx, q, args...).Scan(&n)
			return Result{}, &InLibraryError{TitleID: tid, Ready: n > 0}
		}
	}
	if never, err := library.OnNeverAdd(ctx, db, tmdbID); err != nil {
		return Result{}, err
	} else if never {
		return Result{}, library.ErrNeverAdd
	}
	if err := s.checkLimit(ctx, db, p, KindMovies); err != nil {
		return Result{}, err
	}
	m, err := s.Metadata.Movie(ctx, "tmdb:"+tmdbID)
	if err != nil {
		return Result{}, err
	}
	now := s.now()
	a := domain.Ask{Kind: "movie", TMDBID: tmdbID, Ref: "tmdb:" + tmdbID, SourceIDs: map[string]string{"tmdb": tmdbID}, Seasons: []int{},
		Name: m.Name, Poster: m.Poster, State: domain.AskAsked, AskedBy: p.UserID, AskedAt: now, FourK: fourK}
	if m.Year > 0 {
		y := m.Year
		a.Year = &y
	}
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		// Count again inside the write, so asks sent at the same moment
		// cannot all pass the limit.
		if err := s.checkLimit(ctx, tx, p, KindMovies); err != nil {
			return err
		}
		if _, err := store.Insert(tx, "asks", &a, p.actor()); err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 1)", a.ID, p.UserID, store.FormatTime(now))
		return err
	})
	if err != nil {
		// Two people asking at the same moment: the second joins (once).
		if openAskConflict(err) && !retried {
			return s.ask(ctx, p, tmdbID, fourK, true)
		}
		return Result{}, err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskMade, Summary: p.Name + " asked for " + label(a),
		Data: map[string]any{"ask_id": a.ID, "tmdb_id": tmdbID, "asked_by": p.Name}, GroupKey: groupKey(a.ID)})
	if p.AutoApprove {
		if got, err := s.decide(ctx, a.ID, p, ApproveOptions{}, true); err == nil {
			return Result{Ask: got}, nil
		} else {
			log.Printf("asks: auto-approve %s: %v", a.Name, err)
		}
	}
	s.tellStaff(ctx, a, p)
	s.changed(ctx, a.ID)
	return Result{Ask: a}, nil
}

func (s *Service) join(ctx context.Context, p Person, id int64) (Result, error) {
	a, err := get(ctx, s.Store.Read(), id)
	if err != nil {
		return Result{}, err
	}
	var n int
	if err := s.Store.Read().QueryRowContext(ctx, "SELECT count(*) FROM ask_people WHERE ask_id = ? AND user_id = ?", id, p.UserID).Scan(&n); err != nil {
		return Result{}, err
	}
	if n > 0 {
		return Result{Ask: a, Already: true}, nil
	}
	// Joining costs nothing from the weekly limit: nothing new is fetched.
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT OR IGNORE INTO ask_people (ask_id, user_id, asked_at, counted) VALUES (?, ?, ?, 0)", id, p.UserID, store.FormatTime(s.now()))
		return err
	})
	if err != nil {
		return Result{}, err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskJoined, TitleID: a.TitleID, Summary: p.Name + " also asked for " + label(a),
		Data: map[string]any{"ask_id": a.ID, "asked_by": p.Name}, GroupKey: groupKey(a.ID)})
	if a.State == domain.AskAsked && p.AutoApprove {
		if got, err := s.decide(ctx, a.ID, p, ApproveOptions{}, true); err == nil {
			a = got
		}
	}
	s.changed(ctx, a.ID)
	return Result{Ask: a, Joined: true}, nil
}

// ApproveOptions are the approver's choices; zero values take Requests
// settings (then the add-a-movie defaults).
type ApproveOptions struct {
	LibraryFolderID int64  `json:"library_folder_id"`
	PickRulesID     int64  `json:"pick_rules_id"`
	Note            string `json:"note"`
	// AudioLanguages are the audio languages the approver wants on the
	// movie or show (base codes such as en or ja); empty leaves the
	// Version as it is. Music asks ignore them.
	AudioLanguages []string `json:"audio_languages"`
	// Seasons (show asks) and Albums (artist asks, MusicBrainz ids) approve
	// only part of the ask; empty approves all of it.
	Seasons []int    `json:"seasons"`
	Albums  []string `json:"albums"`
	// Episodes (show asks) keeps single episodes of seasons the ask is
	// for, besides the whole Seasons; with Seasons it is the final pick.
	Episodes []domain.AskEpisodes `json:"episodes"`
	// Songs (artist asks) keeps only some songs of albums: the album's
	// MusicBrainz id and the recording ids of the songs to keep. Once the
	// album's tracks are in the Library the other songs are switched off.
	Songs []AlbumSongs `json:"songs"`
}

// AlbumSongs are the songs of one album an approver keeps.
type AlbumSongs struct {
	Album      string   `json:"album"`
	Recordings []string `json:"recordings"`
}

// Approve says yes to an ask: the movie is added with the chosen Library
// folder and Pick rules and, unless the setting is off, searched for.
func (s *Service) Approve(ctx context.Context, id int64, by Person, opt ApproveOptions) (domain.Ask, error) {
	return s.decide(ctx, id, by, opt, false)
}

func (s *Service) decide(ctx context.Context, id int64, by Person, opt ApproveOptions, auto bool) (domain.Ask, error) {
	if len(opt.Note) > 500 {
		return domain.Ask{}, ErrNoteLength
	}
	db := s.Store.Read()
	a, err := get(ctx, db, id)
	if err != nil {
		return a, err
	}
	if a.State != domain.AskAsked && a.State != domain.AskTurnedDown {
		return a, ErrNotOpen
	}
	if a.Kind == "artist" {
		opt.AudioLanguages = nil
	} else if err := library.CheckAudioLanguages(a.Kind, opt.AudioLanguages); err != nil {
		return a, err
	}
	narrowed, err := s.narrow(ctx, a, opt)
	if err != nil {
		return a, err
	}
	set, err := ReadSettings(ctx, db)
	if err != nil {
		return a, err
	}
	// Claim the ask first: only one decision wins, and only the winner adds
	// the movie and tells anyone.
	now := s.now()
	var before domain.Ask
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		cur, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if cur.State == domain.AskTurnedDown {
			// Another open ask for the same thing means it was asked again.
			// Artist asks have no TMDB id: they match by MusicBrainz id.
			q, args := "SELECT count(*) FROM asks WHERE kind = ? AND tmdb_id = ? AND four_k = ? AND state IN ('asked', 'approved') AND id != ?", []any{cur.Kind, cur.TMDBID, cur.FourK, id}
			if cur.Kind == "artist" {
				q = "SELECT count(*) FROM asks WHERE kind = 'artist' AND json_extract(source_ids, '$.musicbrainz') = ? AND state IN ('asked', 'approved') AND id != ?"
				args = []any{cur.SourceIDs["musicbrainz"], id}
			}
			var n int
			if err := tx.QueryRow(q, args...).Scan(&n); err != nil {
				return err
			}
			if n > 0 {
				return ErrAskedAgain
			}
		}
		res, err := tx.Exec("UPDATE asks SET state = 'approved' WHERE id = ? AND state IN ('asked', 'turned_down')", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotOpen
		}
		before = cur
		a = cur
		a.State, a.TitleID, a.DecidedBy, a.DecidedAt, a.Note, a.AutoApproved = domain.AskApproved, nil, &by.UserID, &now, strings.TrimSpace(opt.Note), auto
		if len(opt.Seasons) > 0 || len(opt.Episodes) > 0 {
			a.Seasons, a.Episodes = narrowed.Seasons, narrowed.Episodes
		}
		if len(opt.Albums) > 0 || len(opt.Songs) > 0 {
			a.Albums = narrowed.Albums
		}
		return store.Update(tx, "asks", &a, by.actor())
	})
	if errors.Is(err, store.ErrNotFound) {
		err = ErrNotOpen
	}
	if err != nil {
		return a, err
	}
	// The audio languages only go on a title this approval adds: changing a
	// title already in the Library is a Library edit, not an approval.
	if len(opt.AudioLanguages) > 0 {
		var had int64
		var herr error
		if a.Kind == "show" {
			had, herr = library.ShowByIDs(ctx, db, a.SourceIDs)
		} else {
			had, herr = library.TitleByTMDB(ctx, db, a.TMDBID)
		}
		if herr != nil || had != 0 {
			opt.AudioLanguages = nil
		}
	}
	var tid int64
	switch a.Kind {
	case "show":
		tid, err = s.addShowForAsk(ctx, a, set, opt, by)
	case "artist":
		tid, err = s.addArtistForAsk(ctx, a, set, opt, by)
	default:
		tid, err = s.addForAsk(ctx, a, set, opt, by)
	}
	if err != nil {
		// Put the ask back as it was, so it can be decided again.
		if rerr := s.Store.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
			return store.Update(tx, "asks", &before, by.actor())
		}); rerr != nil {
			log.Printf("asks: put back ask %d: %v", id, rerr)
		}
		return before, err
	}
	// The title is in the Library now: the rest of the bookkeeping must finish even if the
	// approver's request goes away, or the ask stays approved with no title for good.
	ctx = context.WithoutCancel(ctx)
	if len(opt.AudioLanguages) > 0 { // only for a title this approval added (see above)
		if err := s.setAudio(ctx, tid, opt.AudioLanguages, by); err != nil {
			log.Printf("asks: audio languages of ask %d: %v", id, err)
		}
	}
	a.TitleID = &tid
	if err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE asks SET title_id = ? WHERE id = ?", tid, a.ID)
		return err
	}); err != nil {
		return a, err
	}
	if err := s.followAskers(ctx, a.ID, tid); err != nil {
		log.Printf("asks: follows of ask %d: %v", a.ID, err)
	}
	// A movie is searched as it is added; a show's episodes are kept up to
	// date above, then looked for at once too, not at the next missing
	// search (hours away). Artists are searched in addArtistForAsk, after
	// the MusicBrainz refresh when it brings the asked albums' tracks.
	if set.SearchOnApprove && a.Kind == "show" && s.Library != nil && s.Library.Queue != nil {
		if err := s.Library.ScanThenSearch(ctx, tid, by.actor()); err != nil {
			log.Printf("asks: search for ask %d: %v", a.ID, err)
		}
	}
	people, _ := s.people(ctx, a.ID)
	how := "approved by " + by.Name
	if auto {
		how = "approved straight away (" + by.Name + " may auto-approve)"
	}
	s.event(ctx, &domain.Event{Actor: by.actor(), Kind: domain.EventAskApproved, TitleID: &tid, Summary: label(a) + ": " + how,
		Data: map[string]any{"ask_id": a.ID, "asked_by": names(people), "approved_by": by.Name, "auto": auto, "note": a.Note}, GroupKey: groupKey(a.ID)})
	text := label(a) + " was approved"
	if !auto {
		text += " by " + s.shownAs(ctx, by)
	}
	switch a.Kind {
	case "show":
		text += ". HolyArrs keeps it up to date; you will hear when the episodes you asked for are ready."
	case "artist":
		text += ". HolyArrs keeps the albums you asked for up to date; you will hear when every track is ready to listen to."
	default:
		text += ". HolyArrs is looking for it; you will hear when it is ready."
	}
	if a.Note != "" {
		text += " Note: " + a.Note
	}
	s.tell(ctx, a, people, alerts.MomentApproved, "Approved: "+label(a), text)
	s.changed(ctx, a.ID)
	if _, err := s.CheckReady(ctx); err != nil {
		log.Printf("asks: check ready: %v", err)
	}
	return get(ctx, s.Store.Read(), a.ID)
}

// setAudio puts the approver's audio languages on the title's main Version.
func (s *Service) setAudio(ctx context.Context, titleID int64, langs []string, by Person) error {
	var vid int64
	if err := s.Store.Read().QueryRowContext(ctx, "SELECT id FROM versions WHERE title_id = ? ORDER BY is_main DESC, id LIMIT 1", titleID).Scan(&vid); err != nil {
		return err
	}
	_, err := library.EditVersion(ctx, s.Store, titleID, vid, library.VersionChange{AudioLanguages: &langs}, by.actor())
	return err
}

// addForAsk adds the asked-for movie to the Library, or finds it there. A
// 4K ask always uses the 4K Pick rules: on a movie already in the Library
// it adds a 4K Version (unless one is there), otherwise the movie is added
// with its main copy on the 4K Pick rules.
func (s *Service) addForAsk(ctx context.Context, a domain.Ask, set Settings, opt ApproveOptions, by Person) (int64, error) {
	db := s.Store.Read()
	tid, err := library.TitleByTMDB(ctx, db, a.TMDBID)
	if err != nil {
		return 0, err
	}
	if a.FourK {
		rules, err := fourKRules(ctx, db)
		if err != nil {
			return 0, err
		}
		if rules == 0 {
			return 0, ErrNoFourKRules
		}
		opt.PickRulesID = rules
		if tid != 0 {
			if vid, err := fourKVersion(ctx, db, tid); err != nil || vid != 0 {
				return tid, err
			}
			v, err := library.AddVersion(ctx, s.Store, tid, library.VersionInput{Label: domain.FourKName, PickRulesID: rules}, nil, s.Library.Clock(), by.actor())
			if err != nil {
				return 0, err
			}
			var t domain.Title
			if store.Get(ctx, db, "titles", tid, &t) == nil {
				s.Library.VersionEvent(ctx, t, "Added the "+v.Label+" Version", v, by.actor())
			}
			return tid, nil
		}
	}
	if tid != 0 {
		return tid, nil
	}
	m, err := s.Metadata.Movie(ctx, "tmdb:"+a.TMDBID)
	if err != nil {
		return 0, err
	}
	o := library.AddOptions{LibraryFolderID: opt.LibraryFolderID, PickRulesID: opt.PickRulesID, SearchOnAdd: set.SearchOnApprove, WhyAdded: "ask"}
	if o.LibraryFolderID == 0 {
		o.LibraryFolderID = set.LibraryFolderID
	}
	if o.PickRulesID == 0 {
		o.PickRulesID = set.PickRulesID
	}
	tid, _, err = s.Library.Add(ctx, m, o, by.actor())
	var dup *library.AlreadyAddedError
	if errors.As(err, &dup) {
		return dup.TitleID, nil
	}
	return tid, err
}

// TurnDown says no to an ask, with a note for the people who asked.
func (s *Service) TurnDown(ctx context.Context, id int64, by Person, note string) (domain.Ask, error) {
	note = strings.TrimSpace(note)
	if len(note) > 500 {
		return domain.Ask{}, ErrNoteLength
	}
	now := s.now()
	var a domain.Ask
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		// An approved ask that never got its title (adding it failed) can still be declined.
		res, err := tx.Exec("UPDATE asks SET state = 'turned_down' WHERE id = ? AND (state = 'asked' OR (state = 'approved' AND title_id IS NULL))", id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			var n int
			if err := tx.QueryRow("SELECT count(*) FROM asks WHERE id = ?", id).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return store.ErrNotFound
			}
			return ErrNotOpen
		}
		if a, err = get(ctx, tx, id); err != nil {
			return err
		}
		a.State, a.DecidedBy, a.DecidedAt, a.Note = domain.AskTurnedDown, &by.UserID, &now, note
		return store.Update(tx, "asks", &a, by.actor())
	})
	if err != nil {
		return a, err
	}
	people, _ := s.people(ctx, a.ID)
	s.event(ctx, &domain.Event{Actor: by.actor(), Kind: domain.EventAskTurnedDown, Summary: label(a) + ": turned down by " + by.Name,
		Data: map[string]any{"ask_id": a.ID, "asked_by": names(people), "turned_down_by": by.Name, "note": note}, GroupKey: groupKey(a.ID)})
	text := label(a) + " was turned down by " + s.shownAs(ctx, by) + "."
	if note != "" {
		text += " Note: " + note
	}
	s.tell(ctx, a, people, alerts.MomentTurnedDown, "Turned down: "+label(a), text)
	s.changed(ctx, a.ID)
	return a, nil
}

// Withdraw takes p's ask back while it waits. When others asked too, only
// p leaves it. staff may take back anyone's waiting ask.
func (s *Service) Withdraw(ctx context.Context, id int64, p Person, staff bool) error {
	a, err := get(ctx, s.Store.Read(), id)
	if err != nil {
		return err
	}
	if a.State != domain.AskAsked {
		return ErrNotOpen
	}
	// Check and change inside one write, so a decision made at the same
	// moment cannot slip in between.
	var ids []int64 // everyone on the ask, read inside the write
	left := false   // only p left it; the others keep it
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		var state string
		var owner int64
		if err := tx.QueryRow("SELECT state, asked_by FROM asks WHERE id = ?", id).Scan(&state, &owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotOpen
			}
			return err
		}
		if state != domain.AskAsked {
			return ErrNotOpen
		}
		rows, err := tx.Query("SELECT user_id FROM ask_people WHERE ask_id = ? ORDER BY asked_at, user_id", id)
		if err != nil {
			return err
		}
		ids = ids[:0]
		for rows.Next() {
			var u int64
			if err := rows.Scan(&u); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, u)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		mine := slices.Contains(ids, p.UserID)
		if !mine && !staff {
			return ErrNotYours
		}
		if mine && len(ids) > 1 {
			left = true
			if _, err := tx.Exec("DELETE FROM ask_people WHERE ask_id = ? AND user_id = ?", id, p.UserID); err != nil {
				return err
			}
			if owner != p.UserID {
				return nil
			}
			// The next person who asked takes the ask over, and it now
			// counts against their weekly limit.
			next := ids[0]
			if next == p.UserID {
				next = ids[1]
			}
			if _, err := tx.Exec("UPDATE asks SET asked_by = ? WHERE id = ?", next, id); err != nil {
				return err
			}
			_, err := tx.Exec("UPDATE ask_people SET counted = 1 WHERE ask_id = ? AND user_id = ?", id, next)
			return err
		}
		_, err = tx.Exec("DELETE FROM asks WHERE id = ? AND state = 'asked'", id)
		return err
	})
	if err != nil {
		return err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskWithdrawn, Summary: p.Name + " took back the ask for " + label(a),
		Data: map[string]any{"ask_id": a.ID}, GroupKey: groupKey(a.ID)})
	if s.Live != nil {
		s.Live.PublishApprovers("ask_removed", map[string]any{"id": id})
		// The people who asked see it go from their own open pages too (#139):
		// only p when the others keep the ask, else everyone on it.
		gone := ids
		if left {
			gone = []int64{p.UserID}
			s.changed(ctx, id)
		}
		for _, u := range gone {
			s.Live.PublishUser("ask_removed", map[string]any{"id": id}, u)
		}
	}
	return nil
}

// CheckReady marks approved asks whose movie has a file in the Library as
// available and tells the people who asked. It returns how many it marked.
func (s *Service) CheckReady(ctx context.Context) (int, error) {
	// Followed movies whose file is now in are told here too, right after their import.
	if _, err := s.TellMovieFollows(ctx); err != nil {
		log.Printf("asks: followed movies ready: %v", err)
	}
	// A 4K ask is ready once its 4K copy is in (labelled 4K or on the 4K Pick rules).
	as, err := store.Select[domain.Ask](ctx, s.Store.Read(), "asks", `kind = 'movie' AND state = 'approved' AND title_id IS NOT NULL AND EXISTS (
		SELECT 1 FROM slots sl JOIN items i ON i.id = sl.item_id WHERE i.title_id = asks.title_id AND sl.state IN ('in_library', 'better_wanted')
		AND (asks.four_k = 0 OR sl.version_id IN (SELECT v.id FROM versions v LEFT JOIN pick_rules p ON p.id = v.pick_rules_id
			WHERE v.title_id = asks.title_id AND (v.label = '4K' COLLATE NOCASE OR p.name = '4K')))) ORDER BY id`)
	if err != nil {
		return 0, err
	}
	if err := s.applySongPicks(ctx); err != nil {
		return 0, err
	}
	shows, err := s.readyShows(ctx)
	if err != nil {
		return 0, err
	}
	as = append(as, shows...)
	music, err := s.readyArtists(ctx)
	if err != nil {
		return 0, err
	}
	as = append(as, music...)
	if len(as) == 0 {
		return 0, nil
	}
	n := 0
	for _, a := range as {
		now := s.now()
		var moved int64
		err := s.Store.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("UPDATE asks SET state = 'available', available_at = ?, updated_at = ? WHERE id = ? AND state = 'approved'",
				store.FormatTime(now), store.FormatTime(now), a.ID)
			if err == nil {
				moved, _ = res.RowsAffected()
			}
			return err
		})
		if err != nil {
			return n, err
		}
		if moved == 0 {
			continue
		}
		n++
		people, _ := s.people(ctx, a.ID)
		s.event(ctx, &domain.Event{Kind: domain.EventAskAvailable, TitleID: a.TitleID, Summary: label(a) + " is ready for " + strings.Join(names(people), ", "),
			Data: map[string]any{"ask_id": a.ID, "asked_by": names(people)}, GroupKey: groupKey(a.ID)})
		if a.Kind == "artist" {
			s.tell(ctx, a, people, alerts.MomentReady, "Ready to listen: "+label(a), label(a)+" is in the Library and ready to listen to.")
		} else {
			s.tell(ctx, a, people, alerts.MomentReady, "Ready to watch: "+label(a), label(a)+" is in the Library and ready to watch.")
		}
		s.changed(ctx, a.ID)
	}
	return n, nil
}

// Wait waits for Alerts still being sent (tests and shutdown).
func (s *Service) Wait() {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	s.sending.Wait()
}

func label(a domain.Ask) string {
	l := a.Name
	if a.Year != nil {
		l += " (" + strconv.Itoa(*a.Year) + ")"
	}
	if a.Kind == "show" && !wholeShow(a) && !a.FutureOnly {
		l += ", " + askSeasonWords(a)
	}
	if a.Kind == "artist" {
		l += ", " + albumWords(a.Albums)
	}
	return l
}

func groupKey(id int64) string { return "ask:" + strconv.FormatInt(id, 10) }

// person is someone who asked, with where Alerts reach them.
type person struct {
	ID        int64
	Name      string
	Email     string
	DiscordID string
	AskedAt   time.Time
	// FromWatchlist: they asked (or joined) from their Plex watchlist.
	FromWatchlist bool
}

func names(ps []person) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

func (s *Service) people(ctx context.Context, askID int64) ([]person, error) {
	rows, err := s.Store.Read().QueryContext(ctx, `SELECT u.id, u.name, coalesce(x.email, ''), coalesce(x.discord_id, ''), p.asked_at,
			EXISTS (SELECT 1 FROM plex_watchlist_items w WHERE w.user_id = p.user_id AND w.ask_id = p.ask_id AND w.state IN ('asked', 'joined'))
		FROM ask_people p JOIN users u ON u.id = p.user_id LEFT JOIN user_ask_settings x ON x.user_id = u.id
		WHERE p.ask_id = ? ORDER BY p.asked_at, u.id`, askID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []person
	for rows.Next() {
		var p person
		var at string
		if err := rows.Scan(&p.ID, &p.Name, &p.Email, &p.DiscordID, &at, &p.FromWatchlist); err != nil {
			return nil, err
		}
		p.AskedAt, _ = time.Parse(store.TimeLayout, at)
		out = append(out, p)
	}
	return out, rows.Err()
}

// tell leaves an in-app message for each person and sends the Alerts.
func (s *Service) tell(ctx context.Context, a domain.Ask, people []person, moment, subject, text string) {
	kind := strings.TrimPrefix(moment, "ask_")
	now := store.FormatTime(s.now())
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, p := range people {
			if _, err := tx.Exec("INSERT INTO messages (created_at, user_id, kind, text, ask_id, title_id) VALUES (?, ?, ?, ?, ?, ?)",
				now, p.ID, kind, text, a.ID, a.TitleID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("asks: messages: %v", err)
	}
	m := alerts.Message{Moment: moment, Subject: subject, Text: text}
	for _, p := range people {
		m.People = append(m.People, alerts.Person{Name: p.Name, Email: p.Email, DiscordID: p.DiscordID})
	}
	s.send(ctx, m)
}

func (s *Service) tellStaff(ctx context.Context, a domain.Ask, by Person) {
	s.send(ctx, alerts.Message{Moment: alerts.MomentWaiting, Subject: by.Name + " asked for " + label(a),
		Text: by.Name + " asked for " + label(a) + ". Approve or turn it down on the Requests page."})
}

// send hands the message to the Alerts in the background, so a slow mail
// server never holds up the page.
func (s *Service) send(ctx context.Context, m alerts.Message) {
	if s.Alerts == nil {
		return
	}
	done := make(chan struct{})
	s.sendMu.Lock()
	previous := s.sendTail
	s.sendTail = done
	s.sending.Add(1)
	s.sendMu.Unlock()
	go func() {
		defer s.sending.Done()
		defer close(done)
		if previous != nil {
			<-previous
		}
		supervisor.Guard("alerts", func() {
			sc, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			_ = s.Alerts.Send(sc, m) // failures are recorded on the Alert and as Events
		})
	}()
}

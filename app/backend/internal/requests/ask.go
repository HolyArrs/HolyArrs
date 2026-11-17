package requests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
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

// Error says in plain words what is already in the Library.
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

// Error says which ask limit was hit and when the next ask is allowed.
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
	Store    *database.Store
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

	fill     askFill
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
	// AutoApproveFourK lets their 4K asks be approved straight away too
	// (with AutoApprove); without it a 4K ask waits for staff.
	AutoApproveFourK bool
	NoLimit     bool // the weekly limit does not apply (people who approve)
	// MaxRating is a Kid's age-rating limit ("" for none): they may ask
	// only for movies and shows rated up to it, and never for music.
	MaxRating string
}

// ErrAboveLimit is behind every ask refused for the asker's age-rating
// limit; the error itself says why in plain words.
var ErrAboveLimit = errors.New("above the age-rating limit")

// limitRefusal carries the plain reason while still matching ErrAboveLimit.
type limitRefusal string

func (e limitRefusal) Error() string { return string(e) }
func (e limitRefusal) Unwrap() error { return ErrAboveLimit }

// overLimit refuses a movie or show rated rating when p's limit does not
// allow it; nil when it does.
func overLimit(p Person, kind, rating string) error {
	if metadata.RatingAllows(p.MaxRating, kind, rating) {
		return nil
	}
	if kind == "artist" {
		return limitRefusal("This account cannot ask for music, because music has no age ratings.")
	}
	return limitRefusal("This account can ask only for titles rated " + metadata.RatingUp(p.MaxRating) + ".")
}

// libraryOverLimit refuses when the Library title id (0: none) is rated
// above p's limit.
func libraryOverLimit(ctx context.Context, db database.Querier, p Person, kind string, id int64) error {
	if id == 0 {
		return nil
	}
	var rating string
	if err := db.QueryRowContext(ctx, libraryOverLimitSQL, id).Scan(&rating); err != nil {
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
	database.Lister
	database.Querier
}

// PersonLimits reads one person's own weekly limit per kind; nil uses the
// default. The older all-kinds value (per_week) counts for each kind that
// has no value of its own.
func PersonLimits(ctx context.Context, db database.Querier, userID int64) (movies, shows, music *int, err error) {
	var all, m, sh, mu sql.NullInt64
	err = db.QueryRowContext(ctx, personLimitsSQL, userID).Scan(&all, &m, &sh, &mu)
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
	rows, err := db.QueryContext(ctx, kindLimitSQL,
		p.UserID, database.FormatTime(now.Add(-Window)), askKinds[kind])
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
		at, _ := time.Parse(database.TimeLayout, t)
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

func get(ctx context.Context, q database.Querier, id int64) (domain.Ask, error) {
	var a domain.Ask
	err := database.Get(ctx, q, "asks", id, &a)
	return a, err
}

// Ask asks for a movie by TMDB id for p.
func (s *Service) Ask(ctx context.Context, p Person, tmdbID string) (Result, error) {
	l, err := builtInLine(ctx, s.Store.Read(), "movie", false)
	if err != nil {
		return Result{}, err
	}
	return s.ask(ctx, p, tmdbID, l, false)
}

// Ask4K asks for a 4K copy of a movie by TMDB id for p. It is its own ask,
// approved apart from a plain ask, and may be made for a movie already in
// the Library that has no 4K Version yet.
func (s *Service) Ask4K(ctx context.Context, p Person, tmdbID string) (Result, error) {
	l, err := builtInLine(ctx, s.Store.Read(), "movie", true)
	if err != nil {
		return Result{}, err
	}
	return s.ask(ctx, p, tmdbID, l, false)
}

// AskQuality asks for a movie by TMDB id for p in one line of what people can
// ask for (empty: p's first line). A line p may not use (hidden, not set up,
// or not the first one for a Kid) is ErrQualityNotOffered.
func (s *Service) AskQuality(ctx context.Context, p Person, tmdbID, quality string) (Result, error) {
	l, err := pickQuality(ctx, s.Store.Read(), p, "movie", quality)
	if err != nil {
		return Result{}, err
	}
	return s.ask(ctx, p, tmdbID, l, false)
}

// autoApproves says p's ask of this kind is approved straight away: a 4K
// ask needs the 4K switch too.
func (p Person) autoApproves(fourK bool) bool {
	return p.AutoApprove && (!fourK || p.AutoApproveFourK)
}

// copyOf finds the title's 4K Version (fourK) or its plain copy (the main
// one first among the Versions without the 4K mark), 0 when it has none.
func copyOf(ctx context.Context, db database.Querier, titleID int64, fourK bool) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, copyOfSQL, titleID, fourK).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// fourKVersion finds the title's 4K Version, 0 when it has none.
func fourKVersion(ctx context.Context, db database.Querier, titleID int64) (int64, error) {
	return copyOf(ctx, db, titleID, true)
}

// HasFourK says the title already has a 4K Version (a 4K ask is refused then).
func HasFourK(ctx context.Context, db database.Querier, titleID int64) (bool, error) {
	id, err := fourKVersion(ctx, db, titleID)
	return id != 0, err
}

// fourKRules is the 4K Pick rules preset's id (0 when it was removed).
func fourKRules(ctx context.Context, db database.Querier) (int64, error) {
	var id int64
	err := db.QueryRowContext(ctx, fourKRulesSQL, mediafiles.FourKName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// ErrNoFourKRules: a 4K ask cannot be approved without the 4K Pick rules.
var ErrNoFourKRules = errors.New("no 4K Pick rules: add Pick rules named 4K first")

// openAskConflict says err is the one-open-ask-per-movie index refusing a
// second open ask (asks_one_open on kind, tmdb_id and quality).
func openAskConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: asks.kind, asks.tmdb_id")
}

func (s *Service) ask(ctx context.Context, p Person, tmdbID string, q QualityLine, retried bool) (Result, error) {
	db := s.Store.Read()
	fourK := q.ID == QualityFourK
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
	err := db.QueryRowContext(ctx, askSQL, tmdbID, q.ID).Scan(&openID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Result{}, err
	}
	if openID != 0 {
		return s.join(ctx, p, openID)
	}
	if tid, err := library.TitleByTMDB(ctx, db, tmdbID); err != nil {
		return Result{}, err
	} else if tid != 0 {
		// A 4K ask is fine for a movie in the Library without a 4K
		// Version, a plain ask for one whose only copy is the 4K one.
		vid, err := copyOf(ctx, db, tid, fourK)
		if err != nil {
			return Result{}, err
		}
		if vid != 0 {
			var n int
			_ = db.QueryRowContext(ctx, askSQL2, tid, vid).Scan(&n)
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
		Name: m.Name, Poster: m.Poster, State: domain.AskAsked, AskedBy: p.UserID, AskedAt: now}
	withQuality(&a, q)
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
		if _, err := database.Insert(tx, "asks", &a, p.actor()); err != nil {
			return err
		}
		_, err = tx.Exec(askSQL3, a.ID, p.UserID, database.FormatTime(now))
		return err
	})
	if err != nil {
		// Two people asking at the same moment: the second joins (once).
		if openAskConflict(err) && !retried {
			return s.ask(ctx, p, tmdbID, q, true)
		}
		return Result{}, err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskMade, Summary: p.Name + " asked for " + label(a),
		Data: map[string]any{"ask_id": a.ID, "tmdb_id": tmdbID, "asked_by": p.Name}, GroupKey: groupKey(a.ID)})
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

func (s *Service) join(ctx context.Context, p Person, id int64) (Result, error) {
	a, err := get(ctx, s.Store.Read(), id)
	if err != nil {
		return Result{}, err
	}
	var n int
	if err := s.Store.Read().QueryRowContext(ctx, joinSQL, id, p.UserID).Scan(&n); err != nil {
		return Result{}, err
	}
	if n > 0 {
		return Result{Ask: a, Already: true}, nil
	}
	// Joining costs nothing from the weekly limit: nothing new is fetched.
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(joinSQL2, id, p.UserID, database.FormatTime(s.now()))
		return err
	})
	if err != nil {
		return Result{}, err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskJoined, TitleID: a.TitleID, Summary: p.Name + " also asked for " + label(a),
		Data: map[string]any{"ask_id": a.ID, "asked_by": p.Name}, GroupKey: groupKey(a.ID)})
	if a.State == domain.AskAsked && p.autoApproves(a.FourK) {
		if got, _, err := s.decide(ctx, a.ID, p, ApproveOptions{}, true); err == nil {
			a = got
		}
	}
	s.changed(ctx, a.ID)
	return Result{Ask: a, Joined: true}, nil
}

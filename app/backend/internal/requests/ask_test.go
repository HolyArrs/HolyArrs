package requests

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeAlerts keeps what would have been sent.
type fakeAlerts struct {
	mu   sync.Mutex
	sent []alerts.Message
}

type blockingAlerts struct {
	entered chan string
	release chan struct{}
}

func (b *blockingAlerts) Send(_ context.Context, m alerts.Message) error {
	b.entered <- m.Subject
	<-b.release
	return nil
}

type guardedAlerts struct {
	calls chan string
}

func (g *guardedAlerts) Send(_ context.Context, m alerts.Message) error {
	g.calls <- m.Subject
	switch m.Subject {
	case "panic":
		panic("test notifier panic")
	case "error":
		return errors.New("test notifier error")
	default:
		return nil
	}
}

func (f *fakeAlerts) Send(_ context.Context, m alerts.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeAlerts) moments() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, m := range f.sent {
		out = append(out, m.Moment)
	}
	return out
}

func (f *fakeAlerts) last() alerts.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sent[len(f.sent)-1]
}

func TestAlertsAreSentInEnqueueOrder(t *testing.T) {
	n := &blockingAlerts{entered: make(chan string, 2), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(n.release) }) }
	t.Cleanup(release)
	s := &Service{Alerts: n}
	s.send(context.Background(), alerts.Message{Subject: "first"})
	if got := <-n.entered; got != "first" {
		t.Fatalf("first send = %q", got)
	}

	s.send(context.Background(), alerts.Message{Subject: "second"})
	select {
	case got := <-n.entered:
		t.Fatalf("%q entered while the first send was blocked", got)
	case <-time.After(500 * time.Millisecond):
	}

	release()
	if got := <-n.entered; got != "second" {
		t.Fatalf("second send = %q", got)
	}
	s.Wait()
}

func TestAlertFailureDoesNotStrandQueue(t *testing.T) {
	n := &guardedAlerts{calls: make(chan string, 3)}
	s := &Service{Alerts: n}
	for _, subject := range []string{"panic", "error", "after"} {
		s.send(context.Background(), alerts.Message{Subject: subject})
	}
	done := make(chan struct{})
	go func() {
		s.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("alert queue did not continue after a notifier failure")
	}
	for _, want := range []string{"panic", "error", "after"} {
		if got := <-n.calls; got != want {
			t.Fatalf("send order = %q, want %q", got, want)
		}
	}
}

type rig struct {
	svc   *Service
	st    *database.Store
	fake  *fakeAlerts
	clock time.Time
	ana   Person
	bo    Person
	luis  Person
}

func movie(name string, year int, tmdb string) metadata.Movie {
	d := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	return metadata.Movie{Name: name, Year: year, IDs: map[string]string{"tmdb": tmdb}, CinemaDate: &d, DigitalDate: &d}
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	r := &rig{st: st, fake: &fakeAlerts{}, clock: time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)}
	ids := map[string]int64{}
	err = st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: t.TempDir(), Kind: "movie", Label: "Movies", Health: "ok", SameDiskAs: []int64{}}, domain.System); err != nil {
			return err
		}
		for _, u := range []struct{ name, role string }{{"luis", "owner"}, {"ana", "member"}, {"bo", "member"}} {
			id, err := database.Insert(tx, "users", &domain.User{Name: u.name, Role: u.role, State: "active"}, domain.System)
			if err != nil {
				return err
			}
			ids[u.name] = id
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r.luis = Person{UserID: ids["luis"], Name: "luis", AutoApprove: true, NoLimit: true}
	r.ana = Person{UserID: ids["ana"], Name: "ana"}
	r.bo = Person{UserID: ids["bo"], Name: "bo"}
	md := &metadata.Static{Movies: []metadata.Movie{movie("Dune: Part Two", 2024, "693134"), movie("Paper Harbour", 2023, "900002"),
		movie("Quiet Orchard", 2022, "900003"), movie("Salt Choir", 2021, "900005")}}
	q := jobs.New(st, jobs.Options{Workers: 1})
	lib := &library.Service{Store: st, Queue: q, Metadata: md}
	r.svc = &Service{Store: st, Library: lib, Metadata: md, Alerts: r.fake, Now: func() time.Time { return r.clock }}
	return r
}

func (r *rig) events(t *testing.T) []domain.Event {
	es, err := r.st.RecentEvents(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func hasEvent(es []domain.Event, kind string) *domain.Event {
	for i := range es {
		if es[i].Kind == kind {
			return &es[i]
		}
	}
	return nil
}

func TestAskJoinApproveAndAlreadyInLibrary(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	a := res.Ask
	if a.State != domain.AskAsked || a.Name != "Dune: Part Two" || res.Joined || a.TitleID != nil {
		t.Fatalf("ask %+v", res)
	}
	r.svc.Wait()
	if m := r.fake.moments(); len(m) != 1 || m[0] != alerts.MomentWaiting {
		t.Fatalf("alerts %v", m)
	}
	if e := hasEvent(r.events(t), domain.EventAskMade); e == nil || e.Summary != "ana asked for Dune: Part Two (2024)" {
		t.Fatalf("event %+v", e)
	}

	// bo asks for the same movie: he joins ana's ask, free of his limit.
	res, err = r.svc.Ask(ctx, r.bo, "693134")
	if err != nil || !res.Joined || res.Ask.ID != a.ID {
		t.Fatalf("join %+v %v", res, err)
	}
	if l, _ := r.svc.LimitOf(ctx, r.bo); l.Movies.Used != 0 {
		t.Fatalf("bo limit %+v", l)
	}
	res, _ = r.svc.Ask(ctx, r.ana, "693134")
	if !res.Already {
		t.Fatalf("again %+v", res)
	}
	vs, _ := r.svc.Views(ctx, "1=1")
	if len(vs) != 1 || len(vs[0].People) != 2 || vs[0].People[0].Name != "ana" || vs[0].People[1].Name != "bo" || vs[0].StateLabel != "Waiting for approval" {
		t.Fatalf("views %+v", vs)
	}

	// luis approves: the movie is added (why added: ask) and both are told.
	got, err := r.svc.Approve(ctx, a.ID, r.luis, ApproveOptions{Note: "enjoy"})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.AskApproved || got.TitleID == nil || got.DecidedBy == nil || *got.DecidedBy != r.luis.UserID || got.AutoApproved {
		t.Fatalf("approved %+v", got)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, r.st.Read(), "titles", *got.TitleID, &ti)
	if ti.WhyAdded != "ask" {
		t.Fatalf("why added %q", ti.WhyAdded)
	}
	r.svc.Wait()
	last := r.fake.last()
	if last.Moment != alerts.MomentApproved || len(last.People) != 2 || !strings.Contains(last.Text, "approved by an admin") || !strings.Contains(last.Text, "Note: enjoy") {
		t.Fatalf("approved alert %+v", last)
	}
	for _, p := range []Person{r.ana, r.bo} {
		ms, unread, _ := Messages(ctx, r.st.Read(), p.UserID, 10)
		if unread != 1 || ms[0].Kind != "approved" {
			t.Fatalf("%s messages %+v", p.Name, ms)
		}
	}
	e := hasEvent(r.events(t), domain.EventAskApproved)
	if e == nil || e.Data["approved_by"] != "luis" || e.TitleID == nil {
		t.Fatalf("approved event %+v", e)
	}
	if _, err := r.svc.Approve(ctx, a.ID, r.luis, ApproveOptions{}); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("approve twice: %v", err)
	}

	// A movie already in the Library says so, with its Title.
	if _, _, err := r.svc.Library.Add(ctx, movie("Paper Harbour", 2023, "900002"), library.AddOptions{}, domain.System); err != nil {
		t.Fatal(err)
	}
	_, err = r.svc.Ask(ctx, r.ana, "900002")
	var in *InLibraryError
	if !errors.As(err, &in) || in.TitleID == 0 || in.Ready {
		t.Fatalf("in library: %v", err)
	}
	// Never-add wins.
	_ = r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO never_add (created_at, created_by, updated_at, updated_by, kind, source_ids, name, reason)
			VALUES ('x', '{}', 'x', '{}', 'movie', '{"tmdb":"900005"}', 'Salt Choir', '')`)
		return err
	})
	if _, err := r.svc.Ask(ctx, r.ana, "900005"); !errors.Is(err, library.ErrNeverAdd) {
		t.Fatalf("never add: %v", err)
	}
}

func TestLimitWindowSaysWhenToTryAgain(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := SaveSettings(ctx, r.st, Settings{MoviesPerWeek: 2, SearchOnApprove: true}, domain.System); err != nil {
		t.Fatal(err)
	}
	first := r.clock
	if _, err := r.svc.Ask(ctx, r.ana, "693134"); err != nil {
		t.Fatal(err)
	}
	r.clock = r.clock.Add(time.Hour)
	if _, err := r.svc.Ask(ctx, r.ana, "900002"); err != nil {
		t.Fatal(err)
	}
	r.clock = r.clock.Add(time.Hour)
	_, err := r.svc.Ask(ctx, r.ana, "900003")
	var le *LimitError
	if !errors.As(err, &le) || !le.RetryAt.Equal(first.Add(Window)) {
		t.Fatalf("limit: %v", err)
	}
	if !strings.Contains(err.Error(), "try again in 6 days and 22 hours") {
		t.Fatalf("message %q", err)
	}
	l, _ := r.svc.LimitOf(ctx, r.ana)
	if m := l.Movies; m.PerWeek != 2 || m.Used != 2 || m.Left != 0 || m.RetryAt == nil {
		t.Fatalf("limit %+v", l)
	}
	// Others are not held back by ana's use; people who approve have no limit.
	if _, err := r.svc.Ask(ctx, r.bo, "900003"); err != nil {
		t.Fatal(err)
	}
	// A turned down ask gives the slot back.
	vs, _ := r.svc.Views(ctx, "a.tmdb_id = '693134'")
	if _, err := r.svc.TurnDown(ctx, vs[0].ID, r.luis, "not this one"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Ask(ctx, r.ana, "900005"); err != nil {
		t.Fatalf("after turn down: %v", err)
	}
	// Her own limit wins over the default; 0 is no limit.
	zero := 0
	if err := SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{Movies: &zero}); err != nil {
		t.Fatal(err)
	}
	if l, _ := r.svc.LimitOf(ctx, r.ana); !l.Movies.NoLimit || l.NoLimit {
		t.Fatalf("own limit %+v", l)
	}
	one := 1
	_ = SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{Movies: &one})
	if l, _ := r.svc.LimitOf(ctx, r.ana); l.Movies.PerWeek != 1 || l.Movies.Left != 0 {
		t.Fatalf("own limit %+v", l)
	}
	// A week after the first ask, the window has moved on.
	r.clock = first.Add(Window + 90*time.Minute)
	_ = SetPersonLimit(ctx, r.st, r.ana.UserID, PersonLimit{})
	if l, _ := r.svc.LimitOf(ctx, r.ana); l.Movies.Used != 1 || l.Movies.Left != 1 {
		t.Fatalf("after a week %+v", l)
	}
	if err := SetPersonLimit(ctx, r.st, 999, PersonLimit{Movies: &one}); !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("no such user: %v", err)
	}
	if err := SaveSettings(ctx, r.st, Settings{MoviesPerWeek: -1}, domain.System); !errors.Is(err, ErrPerWeek) {
		t.Fatalf("bad limit: %v", err)
	}
}

func TestAutoApproveAddsAndSearches(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.ana.AutoApprove = true
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	if res.Ask.State != domain.AskApproved || !res.Ask.AutoApproved || res.Ask.TitleID == nil {
		t.Fatalf("auto %+v", res.Ask)
	}
	js, err := database.Select[domain.Job](ctx, r.st.Read(), "jobs", "kind = 'refresh_title'")
	if err != nil {
		t.Fatal(err)
	}
	if len(js) != 1 || !strings.Contains(strings.Join(anyStrings(js[0].Params["then"]), ","), "search_slot") {
		t.Fatalf("jobs %+v", js)
	}
	r.svc.Wait()
	if m := r.fake.moments(); len(m) != 1 || m[0] != alerts.MomentApproved {
		t.Fatalf("alerts %v (no waiting alert for auto-approved asks)", m)
	}
	// With search on approve off, the movie is added but not searched.
	_ = SaveSettings(ctx, r.st, Settings{MoviesPerWeek: 10, SearchOnApprove: false}, domain.System)
	res, _ = r.svc.Ask(ctx, r.ana, "900002")
	js, _ = database.Select[domain.Job](ctx, r.st.Read(), "jobs", "kind = 'refresh_title' AND json_extract(subject, '$.id') = ?", *res.Ask.TitleID)
	if len(js) != 1 || strings.Contains(strings.Join(anyStrings(js[0].Params["then"]), ","), "search_slot") {
		t.Fatalf("jobs %+v", js)
	}
}

func anyStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestTurnDownAndTakeBack(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	_, _ = r.svc.Ask(ctx, r.bo, "693134")
	if _, err := r.svc.TurnDown(ctx, res.Ask.ID, r.luis, strings.Repeat("x", 501)); !errors.Is(err, ErrNoteLength) {
		t.Fatalf("long note: %v", err)
	}
	got, err := r.svc.TurnDown(ctx, res.Ask.ID, r.luis, "we have it on disc")
	if err != nil || got.State != domain.AskTurnedDown || got.Note != "we have it on disc" {
		t.Fatalf("turned down %+v %v", got, err)
	}
	r.svc.Wait()
	last := r.fake.last()
	if last.Moment != alerts.MomentTurnedDown || len(last.People) != 2 || !strings.Contains(last.Text, "we have it on disc") {
		t.Fatalf("alert %+v", last)
	}
	if e := hasEvent(r.events(t), domain.EventAskTurnedDown); e == nil || e.Data["turned_down_by"] != "luis" {
		t.Fatalf("event %+v", e)
	}
	// After a no, asking again starts a fresh ask.
	again, err := r.svc.Ask(ctx, r.bo, "693134")
	if err != nil || again.Ask.ID == res.Ask.ID || again.Joined {
		t.Fatalf("again %+v %v", again, err)
	}
	_, _ = r.svc.Ask(ctx, r.ana, "693134")
	// bo takes his back: ana keeps it.
	if err := r.svc.Withdraw(ctx, again.Ask.ID, r.bo, false); err != nil {
		t.Fatal(err)
	}
	vs, _ := r.svc.Views(ctx, "a.id = ?", again.Ask.ID)
	if len(vs) != 1 || vs[0].AskedBy.Name != "ana" || len(vs[0].People) != 1 {
		t.Fatalf("after bo left %+v", vs)
	}
	if err := r.svc.Withdraw(ctx, again.Ask.ID, r.bo, false); !errors.Is(err, ErrNotYours) {
		t.Fatalf("not his: %v", err)
	}
	if err := r.svc.Withdraw(ctx, again.Ask.ID, r.ana, false); err != nil {
		t.Fatal(err)
	}
	if vs, _ := r.svc.Views(ctx, "a.id = ?", again.Ask.ID); len(vs) != 0 {
		t.Fatalf("still there %+v", vs)
	}
}

func TestReadyTellsEveryoneWhoAsked(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	_, _ = r.svc.Ask(ctx, r.bo, "693134")
	a, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := r.svc.CheckReady(ctx); n != 0 {
		t.Fatalf("ready too soon: %d", n)
	}
	// The import placed the file.
	_ = r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET state = 'in_library' WHERE item_id IN (SELECT id FROM items WHERE title_id = ?)", *a.TitleID)
		return err
	})
	r.clock = r.clock.Add(time.Hour)
	if n, err := r.svc.CheckReady(ctx); n != 1 || err != nil {
		t.Fatalf("ready %d %v", n, err)
	}
	if n, _ := r.svc.CheckReady(ctx); n != 0 {
		t.Fatalf("told twice")
	}
	vs, _ := r.svc.Views(ctx, "a.id = ?", a.ID)
	if vs[0].State != domain.AskAvailable || vs[0].AvailableAt == nil || vs[0].StateLabel != "Ready to watch" {
		t.Fatalf("view %+v", vs[0])
	}
	r.svc.Wait()
	last := r.fake.last()
	if last.Moment != alerts.MomentReady || len(last.People) != 2 || last.Subject != "Ready to watch: Dune: Part Two (2024)" {
		t.Fatalf("alert %+v", last)
	}
	e := hasEvent(r.events(t), domain.EventAskAvailable)
	if e == nil || e.Summary != "Dune: Part Two (2024) is ready for ana, bo" {
		t.Fatalf("event %+v", e)
	}
	ms, unread, _ := Messages(ctx, r.st.Read(), r.bo.UserID, 10)
	if unread != 2 || ms[0].Kind != "ready" {
		t.Fatalf("messages %+v", ms)
	}
	_ = MarkRead(ctx, r.st, r.bo.UserID)
	if _, unread, _ := Messages(ctx, r.st.Read(), r.bo.UserID, 10); unread != 0 {
		t.Fatalf("unread %d", unread)
	}
	// Asking for it now says it is ready.
	_, err = r.svc.Ask(ctx, r.luis, "693134")
	var in *InLibraryError
	if !errors.As(err, &in) || !in.Ready {
		t.Fatalf("ask ready movie: %v", err)
	}
}

func TestWaitWords(t *testing.T) {
	for d, want := range map[time.Duration]string{
		30 * time.Second: "a minute", 40 * time.Minute: "40 minutes", time.Hour: "1 hour", 90 * time.Minute: "1 hour and 30 minutes",
		26 * time.Hour: "1 day and 2 hours", 48 * time.Hour: "2 days",
		// Rounded up, never said shorter than it is.
		90*time.Second + time.Millisecond: "2 minutes", 7*24*time.Hour - time.Millisecond: "7 days", 7 * 24 * time.Hour: "7 days",
		6*24*time.Hour + 22*time.Hour + time.Minute: "6 days and 23 hours",
	} {
		if got := Wait(d); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}

// race runs f and g at the same moment and returns their errors.
func race(f, g func() error) (error, error) {
	var wg sync.WaitGroup
	var e1, e2 error
	start := make(chan struct{})
	wg.Add(2)
	go func() { defer wg.Done(); <-start; e1 = f() }()
	go func() { defer wg.Done(); <-start; e2 = g() }()
	close(start)
	wg.Wait()
	return e1, e2
}

func (f *fakeAlerts) count(moment string) int {
	n := 0
	for _, m := range f.moments() {
		if m == moment {
			n++
		}
	}
	return n
}

func TestDecisionsAtTheSameMomentOnlyOneWins(t *testing.T) {
	ctx := context.Background()
	for i := 0; i < 8; i++ {
		// approve and approve
		r := newRig(t)
		res, _ := r.svc.Ask(ctx, r.ana, "693134")
		e1, e2 := race(func() error { _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); return err },
			func() error { _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); return err })
		if (e1 == nil) == (e2 == nil) || !(errors.Is(e1, ErrNotOpen) || errors.Is(e2, ErrNotOpen)) {
			t.Fatalf("approve/approve: %v, %v", e1, e2)
		}
		r.svc.Wait()
		if n := r.fake.count(alerts.MomentApproved); n != 1 {
			t.Fatalf("approve/approve sent %d approvals", n)
		}

		// approve and turn down
		r = newRig(t)
		res, _ = r.svc.Ask(ctx, r.ana, "693134")
		e1, e2 = race(func() error { _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); return err },
			func() error { _, err := r.svc.TurnDown(ctx, res.Ask.ID, r.luis, "no"); return err })
		r.svc.Wait()
		a, _ := get(ctx, r.st.Read(), res.Ask.ID)
		tid, _ := library.TitleByTMDB(ctx, r.st.Read(), "693134")
		switch {
		case e1 == nil && e2 == nil:
			// The no came first and the yes then overrode it (allowed).
			if a.State != domain.AskApproved || tid == 0 {
				t.Fatalf("both won but %+v title %d", a, tid)
			}
		case e1 != nil && e2 != nil:
			t.Fatalf("approve/turn down: %v, %v", e1, e2)
		}
		if e1 == nil && e2 != nil && (a.State != domain.AskApproved || tid == 0 || r.fake.count(alerts.MomentTurnedDown) != 0) {
			t.Fatalf("approve won but %+v title %d", a, tid)
		}
		if e2 == nil && e1 != nil && (a.State != domain.AskTurnedDown || tid != 0 || r.fake.count(alerts.MomentApproved) != 0) {
			t.Fatalf("turn down won but %+v title %d", a, tid)
		}

		// approve and take back
		r = newRig(t)
		res, _ = r.svc.Ask(ctx, r.ana, "693134")
		e1, e2 = race(func() error { _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); return err },
			func() error { return r.svc.Withdraw(ctx, res.Ask.ID, r.ana, false) })
		if (e1 == nil) == (e2 == nil) {
			t.Fatalf("approve/withdraw: %v, %v", e1, e2)
		}
		r.svc.Wait()
		tid, _ = library.TitleByTMDB(ctx, r.st.Read(), "693134")
		if e2 == nil && (tid != 0 || r.fake.count(alerts.MomentApproved) != 0) {
			t.Fatalf("withdraw won but title %d was added", tid)
		}
	}
}

func TestParallelAsksKeepTheWeeklyLimit(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := SaveSettings(ctx, r.st, Settings{MoviesPerWeek: 1}, domain.System); err != nil {
		t.Fatal(err)
	}
	ids := []string{"693134", "900002", "900003", "900005"}
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, id := range ids {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, errs[i] = r.svc.Ask(ctx, r.ana, id) }()
	}
	close(start)
	wg.Wait()
	ok := 0
	for _, err := range errs {
		var le *LimitError
		switch {
		case err == nil:
			ok++
		case !errors.As(err, &le):
			t.Fatalf("unexpected %v", err)
		}
	}
	if ok != 1 {
		t.Fatalf("%d asks got through a limit of 1: %v", ok, errs)
	}
}

func TestTakingBackHandsTheAskOverAndItCounts(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	_, _ = r.svc.Ask(ctx, r.bo, "693134")
	if l, _ := r.svc.LimitOf(ctx, r.bo); l.Movies.Used != 0 {
		t.Fatalf("joining counted %+v", l)
	}
	if err := r.svc.Withdraw(ctx, res.Ask.ID, r.ana, false); err != nil {
		t.Fatal(err)
	}
	a, _ := get(ctx, r.st.Read(), res.Ask.ID)
	if a.AskedBy != r.bo.UserID {
		t.Fatalf("owner %d", a.AskedBy)
	}
	if l, _ := r.svc.LimitOf(ctx, r.bo); l.Movies.Used != 1 {
		t.Fatalf("bo's ask is not counted: %+v", l)
	}
}

func TestApprovingAnOldNoWhileSomeoneAskedAgain(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	old, _ := r.svc.Ask(ctx, r.ana, "693134")
	if _, err := r.svc.TurnDown(ctx, old.Ask.ID, r.luis, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Ask(ctx, r.bo, "693134"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, old.Ask.ID, r.luis, ApproveOptions{}); !errors.Is(err, ErrAskedAgain) {
		t.Fatalf("approve old: %v", err)
	}
	if tid, _ := library.TitleByTMDB(ctx, r.st.Read(), "693134"); tid != 0 {
		t.Fatal("the movie was added anyway")
	}
	if a, _ := get(ctx, r.st.Read(), old.Ask.ID); a.State != domain.AskTurnedDown {
		t.Fatalf("old ask %s", a.State)
	}
}

func TestOpenAskConflict(t *testing.T) {
	if !openAskConflict(errors.New("constraint failed: UNIQUE constraint failed: asks.kind, asks.tmdb_id (2067)")) {
		t.Fatal("the open-ask index was not recognised")
	}
	if openAskConflict(errors.New("UNIQUE constraint failed: ask_people.ask_id, ask_people.user_id")) || openAskConflict(nil) {
		t.Fatal("another UNIQUE was taken for the open-ask index")
	}
}

func TestApproveWithAudioLanguages(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	// A code that is no language is refused before anything is added.
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{AudioLanguages: []string{"zz9"}}); !errors.Is(err, library.ErrCopyChoice) {
		t.Fatalf("bad language: %v", err)
	}
	vs, _ := r.svc.Views(ctx, "1=1")
	if len(vs) != 1 || vs[0].State != domain.AskAsked {
		t.Fatalf("still waiting %+v", vs)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{AudioLanguages: []string{"ja", "EN", "ja"}})
	if err != nil {
		t.Fatal(err)
	}
	var v mediafiles.Version
	if err := r.st.Read().QueryRowContext(ctx, "SELECT id FROM versions WHERE title_id = ? AND is_main = 1", *got.TitleID).Scan(&v.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(ctx, r.st.Read(), "versions", v.ID, &v); err != nil {
		t.Fatal(err)
	}
	if strings.Join(v.AudioLanguages, ",") != "ja,en" {
		t.Fatalf("audio %v", v.AudioLanguages)
	}
}

func TestApproveLeavesAnExistingTitlesAudioAlone(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	// Someone adds the movie by hand while the ask waits.
	tid, _, err := r.svc.Library.Add(ctx, movie("Dune: Part Two", 2024, "693134"), library.AddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{AudioLanguages: []string{"ja"}})
	if err != nil || got.TitleID == nil || *got.TitleID != tid {
		t.Fatalf("approve %+v %v", got, err)
	}
	var v mediafiles.Version
	if err := r.st.Read().QueryRowContext(ctx, "SELECT id FROM versions WHERE title_id = ? AND is_main = 1", tid).Scan(&v.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(ctx, r.st.Read(), "versions", v.ID, &v); err != nil {
		t.Fatal(err)
	}
	// Changing a title already in the Library is a Library edit: the approval leaves it as it was.
	if len(v.AudioLanguages) != 0 {
		t.Fatalf("audio %v", v.AudioLanguages)
	}
}

// The decider's display name rides along with the account name, so every
// viewer sees who declined by the name they chose.
func TestDecidedByDisplayName(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	if _, err := r.svc.TurnDown(ctx, res.Ask.ID, r.luis, ""); err != nil {
		t.Fatal(err)
	}
	vs, _ := r.svc.Views(ctx, "a.id = ?", res.Ask.ID)
	if len(vs) != 1 || vs[0].DecidedBy == nil || vs[0].DecidedBy.Name != "luis" || vs[0].DecidedBy.DisplayName != "" {
		t.Fatalf("no profile %+v", vs)
	}
	if err := r.svc.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO user_profiles (user_id, display_name) VALUES (?, ?)", r.luis.UserID, " Luis R ")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	vs, _ = r.svc.Views(ctx, "a.id = ?", res.Ask.ID)
	if vs[0].DecidedBy.DisplayName != "Luis R" {
		t.Fatalf("display name %+v", vs[0].DecidedBy)
	}
}

// Asked again after a decline: each ask names the others who declined the
// same title before, newest first, without its own decider and each once.
func TestAlsoDeclinedNamesEarlierDecliners(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	first, _ := r.svc.Ask(ctx, r.ana, "693134")
	if _, err := r.svc.TurnDown(ctx, first.Ask.ID, r.luis, ""); err != nil {
		t.Fatal(err)
	}
	r.clock = r.clock.Add(time.Minute)
	second, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil || second.Ask.ID == first.Ask.ID {
		t.Fatalf("asked again %+v %v", second, err)
	}
	if _, err := r.svc.TurnDown(ctx, second.Ask.ID, r.bo, ""); err != nil {
		t.Fatal(err)
	}
	r.clock = r.clock.Add(time.Minute)
	third, _ := r.svc.Ask(ctx, r.ana, "693134")
	// A second "no" to the same ask is refused: one decision wins.
	if _, err := r.svc.TurnDown(ctx, second.Ask.ID, r.luis, ""); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("second decline %v", err)
	}
	names := func(id int64) []string {
		vs, _ := r.svc.Views(ctx, "a.id = ?", id)
		if vs[0].AlsoDeclined != nil {
			t.Fatalf("Views fills also_declined by itself: %+v", vs[0].AlsoDeclined)
		}
		r.svc.AddAlsoDeclined(ctx, vs)
		out := []string{}
		for _, w := range *vs[0].AlsoDeclined {
			out = append(out, w.Name)
		}
		return out
	}
	if got := names(first.Ask.ID); len(got) != 1 || got[0] != "bo" {
		t.Fatalf("first %v", got)
	}
	if got := names(second.Ask.ID); len(got) != 1 || got[0] != "luis" {
		t.Fatalf("second %v", got)
	}
	if got := names(third.Ask.ID); len(got) != 2 || got[0] != "bo" || got[1] != "luis" {
		t.Fatalf("third %v", got)
	}
	// Another movie's declines are not this one's.
	other, _ := r.svc.Ask(ctx, r.ana, "900002")
	if got := names(other.Ask.ID); len(got) != 0 {
		t.Fatalf("other %v", got)
	}
}

// Shows found through TVmaze have no TMDB id: they match by the ids they
// share, so two different such shows never name each other's decliners.
func TestAlsoDeclinedShowsWithoutTMDBID(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	at := r.clock
	add := func(ids map[string]string, by *int64) int64 {
		var id int64
		at = at.Add(time.Minute)
		if err := r.st.Write(ctx, func(tx *sql.Tx) error {
			a := &domain.Ask{Kind: "show", SourceIDs: ids, Seasons: []int{}, Episodes: []domain.AskEpisodes{}, Albums: []domain.AskAlbum{}, Name: "Show", State: domain.AskAsked, AskedBy: r.ana.UserID, AskedAt: at}
			if by != nil {
				a.State, a.DecidedBy, a.DecidedAt = domain.AskTurnedDown, by, &at
			}
			var err error
			id, err = database.Insert(tx, "asks", a, domain.System)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	luis, bo := r.luis.UserID, r.bo.UserID
	add(map[string]string{"tvmaze": "82"}, &luis)
	add(map[string]string{"tvmaze": "999"}, &bo)
	again := add(map[string]string{"tvmaze": "82", "tvdb": "121361"}, nil)
	other := add(map[string]string{"tvmaze": "5"}, nil)
	names := func(id int64) []string {
		vs, err := r.svc.Views(ctx, "a.id = ?", id)
		if err != nil || len(vs) != 1 {
			t.Fatalf("views %v %v", vs, err)
		}
		r.svc.AddAlsoDeclined(ctx, vs)
		out := []string{}
		for _, w := range *vs[0].AlsoDeclined {
			out = append(out, w.Name)
		}
		return out
	}
	if got := names(again); len(got) != 1 || got[0] != "luis" {
		t.Fatalf("same show %v", got)
	}
	if got := names(other); len(got) != 0 {
		t.Fatalf("other show %v", got)
	}
}

// An approved ask whose title was deleted from the Library later says so.
func TestApprovedAskWithItsTitleDeletedSaysWhy(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, _ := r.svc.Ask(ctx, r.ana, "693134")
	got, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{})
	if err != nil || got.TitleID == nil {
		t.Fatalf("approve %+v %v", got, err)
	}
	vs, _ := r.svc.Views(ctx, "a.id = ?", res.Ask.ID)
	if vs[0].Problem != "" {
		t.Fatalf("problem before delete %q", vs[0].Problem)
	}
	if _, err := r.svc.Library.Remove(ctx, *got.TitleID, library.RemoveChoice{}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	vs, _ = r.svc.Views(ctx, "a.id = ?", res.Ask.ID)
	if vs[0].State != domain.AskApproved || !strings.Contains(vs[0].Problem, "deleted from the Library") {
		t.Fatalf("after delete %+v", vs[0])
	}
}

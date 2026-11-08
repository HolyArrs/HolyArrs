package requests

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
)

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
	// approve, as the asks routes show them, with who else declined too,
	// so a live copy never drops what the list read showed.
	s.Live.PublishApprovers("ask", v)
	// The people who asked follow their own ask live too, told who
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
	_ = s.Store.Read().QueryRowContext(ctx, shownAsSQL, by.UserID).Scan(&n)
	if n = strings.TrimSpace(n); n != "" {
		return n
	}
	return "an admin"
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
		if err := tx.QueryRow(withdrawSQL, id).Scan(&state, &owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotOpen
			}
			return err
		}
		if state != domain.AskAsked {
			return ErrNotOpen
		}
		rows, err := tx.Query(withdrawSQL2, id)
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
			if _, err := tx.Exec(withdrawSQL3, id, p.UserID); err != nil {
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
			if _, err := tx.Exec(withdrawSQL4, next, id); err != nil {
				return err
			}
			_, err := tx.Exec(withdrawSQL5, id, next)
			return err
		}
		_, err = tx.Exec(withdrawSQL6, id)
		return err
	})
	if err != nil {
		return err
	}
	s.event(ctx, &domain.Event{Actor: p.actor(), Kind: domain.EventAskWithdrawn, Summary: p.Name + " took back the ask for " + label(a),
		Data: map[string]any{"ask_id": a.ID}, GroupKey: groupKey(a.ID)})
	if s.Live != nil {
		s.Live.PublishApprovers("ask_removed", map[string]any{"id": id})
		// The people who asked see it go from their own open pages too:
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
	as, err := database.Select[domain.Ask](ctx, s.Store.Read(), "asks", checkReadySQL)
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
			res, err := tx.Exec(checkReadySQL2,
				database.FormatTime(now), database.FormatTime(now), a.ID)
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
	rows, err := s.Store.Read().QueryContext(ctx, peopleSQL, askID)
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
		p.AskedAt, _ = time.Parse(database.TimeLayout, at)
		out = append(out, p)
	}
	return out, rows.Err()
}

// tell leaves an in-app message for each person and sends the Alerts.
func (s *Service) tell(ctx context.Context, a domain.Ask, people []person, moment, subject, text string) {
	kind := strings.TrimPrefix(moment, "ask_")
	now := database.FormatTime(s.now())
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, p := range people {
			if _, err := tx.Exec(tellSQL,
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
		jobs.Guard("alerts", func() {
			sc, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
			defer cancel()
			_ = s.Alerts.Send(sc, m) // failures are recorded on the Alert and as Events
		})
	}()
}

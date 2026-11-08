package requests

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Staff hear about an ask first, then the asker hears the decision, in the
// order they happened.
func TestAskTellsStaffThroughNotifier(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Approve(ctx, res.Ask.ID, r.luis, ApproveOptions{}); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	r.fake.mu.Lock()
	got := append([]alerts.Message(nil), r.fake.sent...)
	r.fake.mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("alerts %+v", got)
	}
	if got[0].Moment != alerts.MomentWaiting || !strings.Contains(got[0].Subject, "ana asked for Dune: Part Two") {
		t.Fatalf("first alert %+v", got[0])
	}
	if got[1].Moment != alerts.MomentApproved || len(got[1].People) != 1 || got[1].People[0].Name != "ana" {
		t.Fatalf("second alert %+v", got[1])
	}
}

func TestPersonLimitsFallback(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO user_ask_settings (user_id, per_week, movies_per_week, shows_per_week, music_per_week) VALUES (?, 5, 2, NULL, 0)`, r.ana.UserID)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	m, sh, mu, err := PersonLimits(ctx, r.st.Read(), r.ana.UserID)
	if err != nil || m == nil || sh == nil || mu == nil {
		t.Fatalf("limits %v %v %v err %v", m, sh, mu, err)
	}
	if *m != 2 || *sh != 5 || *mu != 0 {
		t.Fatalf("movies %d shows %d music %d", *m, *sh, *mu)
	}
	m, sh, mu, err = PersonLimits(ctx, r.st.Read(), r.bo.UserID)
	if err != nil || m != nil || sh != nil || mu != nil {
		t.Fatalf("no row: %v %v %v %v", m, sh, mu, err)
	}
}

// The audio choice lands on the main Version, whatever order the Versions
// were made in, and leaves the others alone.
func TestApproveSetsAudioOnMainVersion(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	tid, _, err := r.svc.Library.Add(ctx, movie("Dune: Part Two", 2024, "693134"), library.AddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	var first mediafiles.Version
	var id int64
	if err := r.st.Read().QueryRowContext(ctx, "SELECT id FROM versions WHERE title_id = ?", tid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(ctx, r.st.Read(), "versions", id, &first); err != nil {
		t.Fatal(err)
	}
	var other int64
	err = r.st.Write(ctx, func(tx *sql.Tx) error {
		// A later Version that is not the main one, and the first made non-main
		// after it: the main Version is the later one.
		v := first
		v.ID, v.IsMain, v.Label, v.FolderName, v.AudioLanguages = 0, false, "extra", "extra copy", []string{}
		var err error
		if other, err = database.Insert(tx, "versions", &v, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE versions SET is_main = 0 WHERE id = ?", first.ID); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE versions SET is_main = 1 WHERE id = ?", other)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.setAudio(ctx, tid, []string{"de", "en"}, r.luis); err != nil {
		t.Fatal(err)
	}
	audio := func(id int64) string {
		var v mediafiles.Version
		if err := database.Get(ctx, r.st.Read(), "versions", id, &v); err != nil {
			t.Fatal(err)
		}
		return strings.Join(v.AudioLanguages, ",")
	}
	if got := audio(other); got != "de,en" {
		t.Fatalf("main version audio %q", got)
	}
	if got := audio(first.ID); got != "" {
		t.Fatalf("other version changed: %q", got)
	}
}

// people lists everyone on an ask in the order they asked, with where Alerts
// reach them and whether they came from their Plex watchlist.
func TestPeopleListsJoinersInOrder(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	res, err := r.svc.Ask(ctx, r.ana, "693134")
	if err != nil {
		t.Fatal(err)
	}
	r.clock = r.clock.Add(time.Minute)
	if _, err := r.svc.Ask(ctx, r.bo, "693134"); err != nil {
		t.Fatal(err)
	}
	now := database.FormatTime(r.clock)
	err = r.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO user_ask_settings (user_id, email, discord_id) VALUES (?, 'ana@example.test', '42')`, r.ana.UserID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO plex_watchlist_items (user_id, plex_key, kind, state, ask_id, seen_at, updated_at) VALUES (?, 'k1', 'movie', 'joined', ?, ?, ?)`,
			r.bo.UserID, res.Ask.ID, now, now)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	ps, err := r.svc.people(ctx, res.Ask.ID)
	if err != nil || len(ps) != 2 {
		t.Fatalf("people %+v %v", ps, err)
	}
	if ps[0].Name != "ana" || ps[0].Email != "ana@example.test" || ps[0].DiscordID != "42" || ps[0].FromWatchlist {
		t.Fatalf("first %+v", ps[0])
	}
	if ps[1].Name != "bo" || ps[1].Email != "" || !ps[1].FromWatchlist || !ps[1].AskedAt.After(ps[0].AskedAt) {
		t.Fatalf("second %+v", ps[1])
	}
}

package accounts

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func TestCleanupSkipsWriteWhenNothingCanExpire(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		f := setup(t, Options{})
		before := f.s.WriteCount()

		if err := f.a.Cleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := f.s.WriteCount(); got != before {
			t.Fatalf("cleanup committed a write with no sessions: before %d, after %d", before, got)
		}
	})

	t.Run("active session", func(t *testing.T) {
		f := setup(t, Options{})
		setupOwner(t, f, client(t))
		before := f.s.WriteCount()

		if err := f.a.Cleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := f.s.WriteCount(); got != before {
			t.Fatalf("cleanup committed a write with only an active session: before %d, after %d", before, got)
		}
	})
}

func TestCleanupDropsOnlyExpiredOrIdleSessions(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	now := f.clock.now()
	idleBoundary := now.Add(-IdleTimeout)

	err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		for _, session := range []struct {
			hash, seen, expires string
		}{
			{hash: "expired-at-boundary", seen: database.FormatTime(now), expires: database.FormatTime(now)},
			{hash: "idle-before-boundary", seen: database.FormatTime(idleBoundary.Add(-time.Second)), expires: database.FormatTime(now.Add(time.Hour))},
			{hash: "seen-at-boundary", seen: database.FormatTime(idleBoundary), expires: database.FormatTime(now.Add(time.Hour))},
			{hash: "fresh", seen: database.FormatTime(now), expires: database.FormatTime(now.Add(time.Hour))},
		} {
			if _, err := tx.Exec(`INSERT INTO sessions
				(hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at, user_agent, family)
				VALUES (?, 1, 'csrf', ?, ?, ?, ?, '', ?)`, session.hash, database.FormatTime(now), database.FormatTime(now),
				session.seen, session.expires, session.hash); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := f.a.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}

	rows, err := f.s.Read().Query("SELECT hash FROM sessions WHERE hash IN (?, ?, ?, ?) ORDER BY hash",
		"expired-at-boundary", "idle-before-boundary", "seen-at-boundary", "fresh")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			t.Fatal(err)
		}
		got = append(got, hash)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "fresh" || got[1] != "seen-at-boundary" {
		t.Fatalf("sessions left = %v, want fresh and seen-at-boundary", got)
	}
}

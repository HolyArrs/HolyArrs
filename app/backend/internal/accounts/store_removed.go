package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// SignInServer is the address of the kind (jellyfin or emby) sign-in way's
// server, or "" when that way is off.
func SignInServer(ctx context.Context, q database.Querier, kind string) string {
	if database.SettingOr(ctx, q, settingsGroup, signInPrefix+kind+"_on", "") != "true" {
		return ""
	}
	return strings.TrimRight(database.SettingOr(ctx, q, settingsGroup, signInPrefix+kind+"_url", ""), "/")
}

// TurnOffRemoved turns off every person, never the owner, linked to a kind
// (jellyfin or emby) account whose id is not in present: the account was
// removed on the sign-in server. They are signed out, their API keys stop,
// the reason is kept for their account card and History gets an event.
//
// It changes nothing when present is empty or when more than half of the
// linked accounts would be missing: that is a wrong answer (a proxy page, a
// key that sees nobody), not people leaving. Someone the owner turned back
// on ("kept:<kind>") is skipped until their account is on the server again.
func TurnOffRemoved(ctx context.Context, s *database.Store, kind string, present map[string]bool) error {
	type link struct {
		id                  int64
		name, ext, role, st string
		why                 string
	}
	rows, err := s.Read().QueryContext(ctx, `SELECT u.id, u.name, l.external_id, u.role, u.state, u.off_reason
		FROM user_links l JOIN users u ON u.id = l.user_id WHERE l.provider = ?`, kind)
	if err != nil {
		return err
	}
	var all []link
	for rows.Next() {
		var k link
		if err := rows.Scan(&k.id, &k.name, &k.ext, &k.role, &k.st, &k.why); err != nil {
			rows.Close()
			return err
		}
		all = append(all, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	kept := "kept:" + kind
	// Marks for accounts back on the server go first, whatever happens next.
	missing, total := 0, 0
	for i, k := range all {
		if present[k.ext] && k.why == kept {
			if err := s.WriteCore(ctx, func(tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, "UPDATE users SET off_reason = '' WHERE id = ? AND off_reason = ?", k.id, kept)
				return err
			}); err != nil {
				return err
			}
			all[i].why = ""
			k.why = ""
		}
		if k.role == "owner" || k.st != "active" || k.why == kept {
			continue
		}
		total++
		if !present[k.ext] {
			missing++
		}
	}
	if len(present) == 0 {
		return nil
	}
	if missing == 0 {
		return setRemovedStop(ctx, s, kind, 0)
	}
	if total >= 3 && missing*2 > total {
		log.Printf("auth: %s removed people check: %d of %d linked accounts missing; changing nothing", kind, missing, total)
		return setRemovedStop(ctx, s, kind, missing)
	}
	if err := setRemovedStop(ctx, s, kind, 0); err != nil {
		return err
	}
	word := kindWord[kind]
	by, _ := json.Marshal(domain.System)
	for _, k := range all {
		if present[k.ext] || k.role == "owner" || k.st != "active" || k.why == kept {
			continue
		}
		turned := false
		err := s.WriteCore(ctx, func(tx *sql.Tx) error {
			res, err := tx.ExecContext(ctx, "UPDATE users SET state = 'disabled', off_reason = ?, updated_at = ?, updated_by = ? WHERE id = ? AND role != 'owner' AND state = 'active'",
				kind, database.FormatTime(time.Now()), string(by), k.id)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return nil
			}
			turned = true
			if err := deleteSessionsOf(ctx, tx, k.id); err != nil {
				return err
			}
			return deleteKeysOf(ctx, tx, k.id)
		})
		if err != nil {
			return err
		}
		if turned {
			_ = s.AddEvent(ctx, &domain.Event{Actor: domain.System, Kind: EventAccountChanged,
				Summary: k.name + " was removed on the " + word + " server, so turned off here",
				Data:    map[string]any{"user_id": k.id, "action": "edited", "state": "disabled", "reason": kind}})
		}
	}
	return nil
}

// setRemovedStop records whether the removed-people check for kind is held
// back (missing > 0) and writes one warning, with an Alert, each time it
// starts holding back. A later run that is not held back clears it and
// writes one recovered event.
func setRemovedStop(ctx context.Context, s *database.Store, kind string, missing int) error {
	key := "removed_check_stop_" + kind
	was := database.SettingOr(ctx, s.Read(), settingsGroup, key, "") == "1"
	if was == (missing > 0) {
		return nil
	}
	val := ""
	if missing > 0 {
		val = "1"
	}
	if err := s.WriteCore(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, key, val, domain.System, time.Now())
	}); err != nil {
		return err
	}
	word := kindWord[kind]
	if missing == 0 {
		// Shown as the health check's "Working again" line, like any other check.
		e := &domain.Event{Kind: domain.EventHealthRecovered, GroupKey: "removed_people:" + kind,
			Data: map[string]any{"check": "removed_people_" + kind, "kind": "accounts", "subject": word, "state": "ok"}}
		e.Summary = "Fixed: " + word
		return s.AddEvent(ctx, e)
	}
	n := strconv.Itoa(missing)
	e := &domain.Event{Kind: domain.EventHealthBroken, GroupKey: "removed_people:" + kind,
		Summary: "Not turning off " + n + " people removed on the " + word + " server: more than half at once. Check the server, then turn them off by hand if that is right."}
	e.Data = map[string]any{"check": "removed_people_" + kind, "kind": "accounts", "subject": word, "state": "warning", "message": e.Summary}
	return s.AddEvent(ctx, e)
}

// NoteUserListRead counts the runs in a row in which the kind sign-in
// server's user list could not be read; a good read (or nothing left to
// read) starts again at 0. UnreadUserLists turns the count into a health
// check, so the warning shows where the other health warnings show.
func NoteUserListRead(ctx context.Context, s *database.Store, kind string, ok bool) error {
	key := "removed_check_fails_" + kind
	was := database.SettingOr(ctx, s.Read(), settingsGroup, key, "")
	n, _ := strconv.Atoi(was)
	next := n + 1
	if ok {
		next = 0
	}
	if (ok && was == "") || next > unreadAfter && n >= unreadAfter {
		return nil
	}
	return s.WriteCore(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, key, strconv.Itoa(next), domain.System, time.Now())
	})
}

// ForgetUserListRead drops the kind's count, so its people check goes when
// there is no connected server left to read.
func ForgetUserListRead(ctx context.Context, s *database.Store, kind string) error {
	key := "removed_check_fails_" + kind
	if database.SettingOr(ctx, s.Read(), settingsGroup, key, "") == "" {
		return nil
	}
	return s.WriteCore(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, key, "", domain.System, time.Now())
	})
}

// unreadAfter is how many failed reads in a row make a warning.
const unreadAfter = 3

// UnreadUserLists is, for each kind whose sign-in way is on and whose user
// list was ever read for the removed-people check, the warning line while it could not be read
// unreadAfter times in a row, or "" while it reads.
func UnreadUserLists(ctx context.Context, q database.Querier) map[string]string {
	out := map[string]string{}
	for _, kind := range []string{ProviderJellyfin, ProviderEmby} {
		v := database.SettingOr(ctx, q, settingsGroup, "removed_check_fails_"+kind, "")
		if v == "" || SignInServer(ctx, q, kind) == "" {
			continue
		}
		out[kind] = ""
		if n, _ := strconv.Atoi(v); n >= unreadAfter {
			word := kindWord[kind]
			summary := "Can't check for people removed on the " + word + " server: its user list could not be read 3 times in a row."
			out[kind] = summary
		}
	}
	return out
}

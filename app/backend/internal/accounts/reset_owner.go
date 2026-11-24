package accounts

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// ErrNoOwnerInput means nothing came in on stdin, which is what docker exec
// without -i gives. The command then says to add -it.
var ErrNoOwnerInput = errors.New("no name or password came in")

// ReadNewOwner reads a name and a password for ResetOwner from two lines:
// the name first, then the password. Either line may carry a label
// ("name: ...", "password: ..." or "=" instead of ":"), in any order. A line
// after a name is the password as typed, so a password such as "pass:word"
// stays whole. The values are never written anywhere but the database.
func ReadNewOwner(r io.Reader) (name, password string, err error) {
	sc := bufio.NewScanner(io.LimitReader(r, 64<<10))
	first, any := true, false
	var haveName, havePw bool
	// Stop once both are in, so a terminal that never sends end of input works.
	for !(haveName && havePw) && sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if first { // a byte order mark some Windows editors add
			line, first = strings.TrimPrefix(line, string(rune(0xfeff))), false
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		any = true
		k, v, labelled := cutLabel(line)
		switch {
		case labelled && !haveName && (k == "name" || k == "username" || k == "user"):
			name, haveName = v, true
		case labelled && !havePw && (k == "password" || (k == "pass" && !haveName)):
			password, havePw = v, true
		case !haveName:
			name, haveName = strings.TrimSpace(line), true
		default:
			password, havePw = strings.TrimSpace(line), true
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if !any {
		return "", "", ErrNoOwnerInput
	}
	if err := CheckNewOwner(name, password, MinPassword); err != nil {
		return "", "", err
	}
	return name, password, nil
}

// CheckNewOwner says what is wrong with a new owner name and password, or nil.
func CheckNewOwner(name, password string, minLen int) error {
	if msg := typedNameProblem(name); msg != "" {
		return errors.New(msg)
	}
	if msg := passwordProblem(password, minLen); msg != "" {
		return errors.New(msg)
	}
	return nil
}

// ResetOwner sets the owner's name and password, or makes the owner when there
// is none yet. It ends every session, removes every API key and turns off
// two-factor sign-in, so anything
// signed in with the old login has to sign in again.
func ResetOwner(ctx context.Context, s *database.Store, name, password string) error {
	return resetOwnerVia(ctx, s, name, password, resetByCommand)
}

// How the owner was reset, for the audit trail.
const (
	resetByCommand = "command"
	resetByCode    = "log_code"
)

func resetOwnerVia(ctx context.Context, s *database.Store, name, password, how string) error {
	if msg := passwordProblem(password, MinPasswordLength(ctx, s.Read())); msg != "" {
		return errors.New(msg)
	}
	h, err := HashPassword(password)
	if err != nil {
		return err
	}
	by, err := json.Marshal(domain.System)
	if err != nil {
		return err
	}
	now := database.FormatTime(time.Now())
	var id, invites, arrKeys int64
	err = s.Write(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = firstOwnerID(ctx, tx)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = insertUserTx(tx, &domain.User{Name: name, Role: "owner", PasswordHash: h, State: "active", Perms: "[]"}, domain.System)
			return err
		}
		if err != nil {
			return err
		}
		if err := resetOwnerRow(ctx, tx, name, h, now, string(by), id); err != nil {
			return err
		}
		if err := deleteSessionsOf(ctx, tx, id); err != nil {
			return err
		}
		// Resetting is also the way back in for an owner who lost their
		// authenticator, so two-factor sign-in is turned off.
		if err := clearTwoFactor(ctx, tx, nil, id); err != nil {
			return err
		}
		// No vault is open here; deleting a secret row needs no key.
		if err := endOutsideLinks(ctx, tx, nil, id); err != nil {
			return err
		}
		// Invites and arr keys made with the old login could let someone back in.
		if invites, err = deleteOpenInvites(ctx, tx); err != nil {
			return err
		}
		if arrKeys, err = deleteArrKeysOf(ctx, tx, id); err != nil {
			return err
		}
		return deleteKeysOf(ctx, tx, id)
	})
	if err == nil && id != 0 {
		// The reset is on the audit trail: someone with the server's shell took the owner account back.
		summary := "Owner account reset from the server's command line: name and password set, two-factor off, signed out, API keys, open invites, arr keys and outside sign-in links removed"
		if how == resetByCode {
			summary = "Owner account reset with a one-time code from the log: name and password set, two-factor off, signed out, API keys, open invites, arr keys and outside sign-in links removed"
		}
		_ = s.AddEvent(ctx, &domain.Event{Actor: domain.System, Kind: EventAccountChanged,
			Summary: summary,
			Data: map[string]any{"user_id": id, "action": "owner_reset", "how": how, "name": name,
				"invites_removed": invites, "old_app_keys_removed": arrKeys}})
	}
	return err
}

func cutLabel(line string) (string, string, bool) {
	i := strings.IndexAny(line, ":=")
	if i <= 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(line[:i])), strings.TrimSpace(line[i+1:]), true
}

// endOutsideLinks drops the Plex, Jellyfin, Emby and single sign-on
// identities linked to a person, so whoever linked one while knowing the old
// password cannot keep signing in with it after a password change or reset.
// The person links them again from My account.
func endOutsideLinks(ctx context.Context, tx *sql.Tx, vault *secrets.Vault, id int64) error {
	ids, err := tokenIDsOfUser(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := vault.DeleteTx(ctx, tx, ids...); err != nil {
		return err
	}
	if err := deleteLinksOf(ctx, tx, id); err != nil {
		return err
	}
	return deleteSSOLinksOf(ctx, tx, id)
}

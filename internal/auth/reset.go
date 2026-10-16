package auth

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// ReadNewOwner reads a name and a password for ResetOwner. It accepts
// "name: ..." and "password: ..." lines (or "=" instead of ":"), or two bare
// lines, name first. The values are never written anywhere but the database.
func ReadNewOwner(r io.Reader) (name, password string, err error) {
	var bare []string
	sc := bufio.NewScanner(io.LimitReader(r, 64<<10))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if k, v, ok := cutLabel(line); ok {
			switch k {
			case "name", "username", "user":
				name = v
				continue
			case "password", "pass":
				password = v
				continue
			}
		}
		bare = append(bare, strings.TrimSpace(line))
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	if name == "" && password == "" && len(bare) == 2 {
		name, password = bare[0], bare[1]
	}
	if name == "" || len(name) > 64 {
		return "", "", errors.New("pick a name of 1 to 64 characters")
	}
	if msg := passwordProblem(password, MinPassword); msg != "" {
		return "", "", errors.New(msg)
	}
	return name, password, nil
}

func cutLabel(line string) (string, string, bool) {
	i := strings.IndexAny(line, ":=")
	if i <= 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(line[:i])), strings.TrimSpace(line[i+1:]), true
}

// ResetOwner sets the owner's name and password, or makes the owner when there
// is none yet. It ends every session, removes every API key and turns off
// two-factor sign-in, so anything
// signed in with the old login has to sign in again.
func ResetOwner(ctx context.Context, s *store.Store, name, password string) error {
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
	now := store.FormatTime(time.Now())
	return s.Write(ctx, func(tx *sql.Tx) error {
		var id int64
		err := tx.QueryRow("SELECT id FROM users WHERE role = 'owner' ORDER BY id LIMIT 1").Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = store.Insert(tx, "users", &domain.User{Name: name, Role: "owner", PasswordHash: h, State: "active", Perms: "[]"}, domain.System)
			return err
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET name = ?, password_hash = ?, state = 'active', updated_at = ?, updated_by = ? WHERE id = ?`,
			name, h, now, string(by), id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
			return err
		}
		// Resetting is also the way back in for an owner who lost their
		// authenticator, so two-factor sign-in is turned off.
		if err := clearTwoFactor(tx, id); err != nil {
			return err
		}
		_, err = tx.Exec("DELETE FROM api_keys WHERE owner_user_id = ?", id)
		return err
	})
}

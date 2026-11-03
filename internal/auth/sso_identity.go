package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

var errIdentityLinked = errors.New("this identity is already linked to another account")

type ssoProofKey struct{}
type ssoLinkProof struct {
	hash, family string
	userID       int64
}

func (a *Auth) ssoLinkIdentity(r *http.Request, kind, issuer, subject, display string, userID int64, family string) (int64, string, error) {
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" || family == "" {
		return 0, "", errNotAllowed
	}
	proof := ssoLinkProof{hash: sum(cookie.Value), family: family, userID: userID}
	ctx := context.WithValue(r.Context(), ssoProofKey{}, proof)
	id, name, err := a.ssoIdentity(ctx, kind, issuer, subject, display, false, userID)
	if err == nil {
		// A linked identity signs in without the password, so the link is on the audit trail.
		a.accountEvent(r, userID, "sign_in_linked", byName(r)+" linked a "+kind+" sign-in ("+outsideName(display)+") to "+a.userName(ctx, userID),
			map[string]any{"provider": kind})
	}
	return id, name, err
}

// ssoIdentity makes the identity check and link/create one writer transaction.
func (a *Auth) ssoIdentity(ctx context.Context, kind, issuer, subject, display string, join bool, linkID int64) (int64, string, error) {
	if (kind != "oidc" && kind != "forward_auth") || issuer == "" || len(issuer) > 1000 || !ssoText(subject, 255) {
		return 0, "", errNotAllowed
	}
	var id int64
	var name string
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		if linkID != 0 {
			proof, ok := ctx.Value(ssoProofKey{}).(ssoLinkProof)
			if !ok || proof.userID != linkID {
				return errNotAllowed
			}
			var local, method, created, seen, expires string
			if err := tx.QueryRow(`SELECT s.local_auth_at, s.auth_method, s.created_at, s.last_seen_at, s.expires_at FROM sessions s JOIN users u ON u.id = s.user_id
				WHERE s.hash = ? AND s.family = ? AND s.user_id = ? AND u.state = 'active'`,
				proof.hash, proof.family, linkID).Scan(&local, &method, &created, &seen, &expires); err != nil {
				return errNotAllowed
			}
			now, authenticated := a.now(), parseTime(local)
			if (method != AuthMethodLocalPassword && method != AuthMethodLocalSetup) ||
				authenticated.IsZero() || authenticated.After(now) || now.Sub(authenticated) >= 10*time.Minute ||
				!now.Before(parseTime(expires)) || now.Sub(parseTime(seen)) >= IdleTimeout || now.Sub(parseTime(created)) >= AbsoluteTimeout {
				return errNotAllowed
			}
		}
		var owner int
		if err := tx.QueryRow("SELECT count(*) FROM users WHERE role = 'owner' AND state = 'active'").Scan(&owner); err != nil {
			return err
		}
		if owner != 1 {
			return errNotAllowed
		}
		var state string
		err := tx.QueryRow(`SELECT u.id, u.name, u.state FROM sso_links l JOIN users u ON u.id = l.user_id
			WHERE l.kind = ? AND l.issuer = ? AND l.subject = ?`, kind, issuer, subject).Scan(&id, &name, &state)
		if err == nil {
			if linkID != 0 && linkID != id {
				return errIdentityLinked
			}
			if state != "active" {
				return errDisabled
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if linkID != 0 {
			id = linkID
			if err := tx.QueryRow("SELECT name, state FROM users WHERE id = ?", id).Scan(&name, &state); err != nil {
				return err
			}
			if state != "active" {
				return errDisabled
			}
		} else {
			if !join {
				return errNotAllowed
			}
			base := strings.TrimSpace(display)
			if !ssoText(base, 200) {
				base = kind + "-user"
			}
			base = truncate(base, 50)
			for n := 1; ; n++ {
				name = base
				if n > 1 {
					name = fmt.Sprintf("%s-%d", base, n)
				}
				var count int
				if err := tx.QueryRow("SELECT count(*) FROM users WHERE name = ?", name).Scan(&count); err != nil {
					return err
				}
				if count == 0 {
					break
				}
			}
			id, err = store.Insert(tx, "users", &domain.User{Name: name, Role: RoleMember, State: "active", Perms: permsJSON(DefaultPerms(RoleMember))}, domain.System)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(`INSERT INTO sso_links(kind, issuer, subject, user_id, display_name, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			kind, issuer, subject, id, truncate(display, 200), store.FormatTime(a.now()))
		return err
	})
	return id, name, err
}

func ssoText(s string, limit int) bool {
	if s == "" || len(s) > limit || strings.TrimSpace(s) != s {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}

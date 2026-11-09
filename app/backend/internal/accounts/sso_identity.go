package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
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
			if err := a.linkSessionFreshTx(tx, proof); err != nil {
				return err
			}
		}
		owner, err := countActiveOwners(ctx, tx)
		if err != nil {
			return err
		}
		if owner != 1 {
			return errNotAllowed
		}
		var state string
		id, name, state, err = ssoLinkedUser(ctx, tx, kind, issuer, subject)
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
			if name, state, err = nameAndState(ctx, tx, id); err != nil {
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
				count, err := countUsersNamed(ctx, tx, name)
				if err != nil {
					return err
				}
				if count == 0 {
					break
				}
			}
			id, err = insertUserTx(tx, &domain.User{Name: name, Role: webserver.RoleMember, State: "active", Perms: permsJSON(webserver.DefaultPerms(webserver.RoleMember))}, domain.System)
			if err != nil {
				return err
			}
		}
		return insertSSOLink(ctx, tx, kind, issuer, subject, id, truncate(display, 200), database.FormatTime(a.now()))
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

// linkSessionFreshTx re-reads, inside the write that makes a link, the session
// that asked for it: still there (a password change or reset deletes it), the
// person active, signed in with the local password under 10 minutes ago, and
// within the idle and absolute limits. Otherwise errNotAllowed.
func (a *Auth) linkSessionFreshTx(tx *sql.Tx, proof ssoLinkProof) error {
	local, method, created, seen, expires, err := sessionProof(context.Background(), tx, proof.hash, proof.family, proof.userID)
	if err != nil {
		return errNotAllowed
	}
	now, authenticated := a.now(), parseTime(local)
	if (method != AuthMethodLocalPassword && method != AuthMethodLocalSetup) ||
		authenticated.IsZero() || authenticated.After(now) || now.Sub(authenticated) >= 10*time.Minute ||
		!now.Before(parseTime(expires)) || now.Sub(parseTime(seen)) >= IdleTimeout || now.Sub(parseTime(created)) >= AbsoluteTimeout {
		return errNotAllowed
	}
	return nil
}

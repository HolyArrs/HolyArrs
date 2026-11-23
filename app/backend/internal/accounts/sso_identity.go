package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
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
	cookie, err := sessionCookie(r)
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
	return a.ssoIdentityWith(ctx, kind, issuer, subject, display, join, linkID, "")
}

// ssoIdentityRole is a sign-in that also sets the role from the person's
// groups; groupRole "" leaves it alone (see sso_groups.go).
func (a *Auth) ssoIdentityRole(ctx context.Context, kind, issuer, subject, display string, join bool, groupRole string) (int64, string, error) {
	return a.ssoIdentityWith(ctx, kind, issuer, subject, display, join, 0, groupRole)
}

func (a *Auth) ssoIdentityWith(ctx context.Context, kind, issuer, subject, display string, join bool, linkID int64, groupRole string) (int64, string, error) {
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
			if groupRole != "" {
				return a.applyGroupRoleTx(ctx, tx, id, groupRole)
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
			base := NewPersonName(display, kind+"-user", 50)
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
			role := webserver.RoleMember
			if groupRole == webserver.RoleAdmin {
				role = webserver.RoleAdmin
			}
			id, err = insertUserTx(tx, &domain.User{Name: name, Role: role, State: "active", Perms: permsJSON(webserver.DefaultPerms(role))}, domain.System)
			if err != nil {
				return err
			}
		}
		return insertSSOLink(ctx, tx, kind, issuer, subject, id, truncate(display, 200), database.FormatTime(a.now()))
	})
	return id, name, err
}

// applyGroupRoleTx changes the role only when the groups ask for another
// one, logs the change, and ends the person's sessions when it goes down.
func (a *Auth) applyGroupRoleTx(ctx context.Context, tx *sql.Tx, id int64, role string) error {
	if role != webserver.RoleAdmin && role != webserver.RoleMember {
		return nil
	}
	changed, err := setGroupRole(ctx, tx, id, role, permsJSON(webserver.DefaultPerms(role)), database.FormatTime(a.now()))
	if err != nil || !changed {
		return err
	}
	log.Printf("auth: user %d role set to %s by sign-in groups", id, role)
	if role == webserver.RoleMember {
		// Lowered: other devices keep no admin session. The session for
		// this sign-in is made after this write.
		return deleteSessionsOf(ctx, tx, id)
	}
	return nil
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
// person active, signed in with the local password under 10 minutes ago (or,
// for a person with no password, with Plex, Jellyfin or Emby under 10 minutes
// ago, as RecentLocal allows), and within the idle and absolute limits.
// Otherwise errNotAllowed.
func (a *Auth) linkSessionFreshTx(tx *sql.Tx, proof ssoLinkProof) error {
	return a.linkSessionCheckTx(tx, proof, false)
}

// mediaLinkSessionFreshTx is linkSessionFreshTx for linking a Plex, Jellyfin
// or Emby account: during first-run setup the setup session counts as checked.
func (a *Auth) mediaLinkSessionFreshTx(tx *sql.Tx, proof ssoLinkProof) error {
	return a.linkSessionCheckTx(tx, proof, true)
}

func (a *Auth) linkSessionCheckTx(tx *sql.Tx, proof ssoLinkProof, setupCounts bool) error {
	ctx := context.Background()
	local, method, created, seen, expires, err := sessionProof(ctx, tx, proof.hash, proof.family, proof.userID)
	if err != nil {
		return errNotAllowed
	}
	now, authenticated := a.now(), parseTime(local)
	switch method {
	case AuthMethodLocalPassword, AuthMethodLocalSetup:
	case ProviderPlex, ProviderJellyfin, ProviderEmby:
		if has, err := hasPassword(ctx, tx, proof.userID); err != nil || has {
			return errNotAllowed
		}
		authenticated = parseTime(created)
	default:
		return errNotAllowed
	}
	// During first-run setup the setup session counts as checked (no re-check
	// during setup), capped like SetupRecent: same rule as the setup routes.
	setupWindow := setupCounts && strings.HasPrefix(proof.family, setupFamily) && database.SettingOr(ctx, tx, settingsGroup, settingFirstRunDone, "") != "1" &&
		!parseTime(created).IsZero() && now.Sub(parseTime(created)) < setupRecentFor
	if !setupWindow && (authenticated.IsZero() || authenticated.After(now) || now.Sub(authenticated) >= localRecentFor) ||
		!now.Before(parseTime(expires)) || now.Sub(parseTime(seen)) >= IdleTimeout || now.Sub(parseTime(created)) >= AbsoluteTimeout {
		return errNotAllowed
	}
	return nil
}

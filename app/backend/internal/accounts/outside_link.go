package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// linkedUser finds or makes the user for an outside account. A new person
// the server limits (kid) joins as a Kid, as an import would make them.
func (a *Auth) linkedUser(ctx context.Context, provider, extID, extName string, mayJoin, kid bool) (int64, string, error) {
	id, name, state, err := linkedUserRow(ctx, a.s.Read(), provider, extID)
	if err == nil {
		if state != "active" {
			// The member's name goes with the refusal, for the audit row only.
			return 0, name, errDisabled
		}
		return id, name, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, "", err
	}
	if !mayJoin {
		// The outside account's name (from the server) goes with the refusal.
		return 0, strings.TrimSpace(extName), errNotAllowed
	}
	u := domain.User{Role: webserver.RoleMember, State: "active", Perms: permsJSON(webserver.DefaultPerms(webserver.RoleMember))}
	if kid {
		if a.skipBlocksKids(ctx) {
			// A Kid could not be told apart from the person local visitors become.
			return 0, "", errKidSkipLocal
		}
		u.Perms, u.MaxRating = permsJSON([]webserver.Perm{webserver.PermAsk}), "PG"
	}
	base := outsideBase(provider, extName)
	err = a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		var err error
		if name, err = freeName(tx, base); err != nil {
			return err
		}
		u.Name = name
		id, err = insertUserTx(tx, &u, domain.System)
		if err != nil {
			return err
		}
		return insertUserLink(ctx, tx, id, provider, extID, truncate(extName, 200), database.FormatTime(a.now()))
	})
	if err == nil {
		log.Printf("auth: %s account joined as member %q (%d), kid %t", provider, name, id, kid)
	}
	return id, name, err
}

// outsideBase is the name a new user from an outside account starts from.
func outsideBase(provider, extName string) string {
	base := strings.TrimSpace(extName)
	if base == "" {
		base = provider + "-user"
	}
	return truncate(base, 56)
}

// freeName is base, or base-2, base-3... when that name is taken.
func freeName(tx *sql.Tx, base string) (string, error) {
	name := base
	for i := 2; ; i++ {
		n, err := countUsersNamed(context.Background(), tx, name)
		if err != nil {
			return "", err
		}
		if n == 0 {
			return name, nil
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}

// linkedName is the member an outside account belongs to, found through its
// link (by id, else by its name on that server), else the member with that
// name, else unknownName. The typed name is only matched, never stored.
func (a *Auth) linkedName(ctx context.Context, provider, extID, extName string) string {
	if extID != "" {
		if n, err := linkedUserName(ctx, a.s.Read(), provider, extID); err == nil {
			return n
		}
	}
	if n, err := linkedUserNameByExternalName(ctx, a.s.Read(), provider, extName); err == nil {
		return n
	}
	return a.knownName(ctx, extName)
}

// outsideName is an outside account's name for a failed sign-in row (it
// comes from the Plex, Jellyfin or Emby server, never typed by hand).
func outsideName(name string) string {
	if name == "" {
		return unknownName
	}
	return name
}

// outsideDone answers after an outside account checked out.
func (a *Auth) outsideDone(w http.ResponseWriter, r *http.Request, provider string, id int64, name string, err error, pl place) {
	switch {
	case errors.Is(err, errNotAllowed):
		log.Printf("auth: %s account not allowed to join, from %s", provider, r.RemoteAddr)
		a.failedEvent(r, outsideName(name), provider+" not allowed")
		fail(w, http.StatusForbidden, "this account is not allowed in yet; ask the owner")
	case errors.Is(err, errDisabled):
		a.failedEvent(r, outsideName(name), "turned off")
		fail(w, http.StatusForbidden, "this account is turned off; ask the owner")
	case errors.Is(err, errKidSkipLocal):
		log.Printf("auth: %s account has limits on the server and sign-in is skipped at home; not joined", provider)
		fail(w, http.StatusForbidden, "this account has an age limit, and it can't join while sign-in is skipped at home; ask the owner")
	case err != nil:
		serverError(w, provider+" sign-in", err)
	default:
		a.passedPlace(w, r, id, name, provider, pl)
	}
}

// fetch makes one call to an outside service and decodes a JSON answer.
func (a *Auth) fetch(ctx context.Context, method, u string, hdr map[string]string, body any, out any) (int, error) {
	return a.call(ctx, a.http, method, u, hdr, body, out)
}

// serverFetch is fetch for a Jellyfin or Emby server the admin typed in.
// It never follows a redirect, so a password is only sent to that address.
func (a *Auth) serverFetch(ctx context.Context, method, u string, hdr map[string]string, body any, out any) (int, error) {
	return a.call(ctx, a.server, method, u, hdr, body, out)
}

func (a *Auth) call(ctx context.Context, c *http.Client, method, u string, hdr map[string]string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, outsideMax))
	if err != nil {
		return resp.StatusCode, err
	}
	if out != nil && resp.StatusCode/100 == 2 && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, fmt.Errorf("unreadable answer: %w", err)
		}
	}
	return resp.StatusCode, nil
}

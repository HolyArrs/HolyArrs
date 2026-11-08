package accounts

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// unknownName stands for a typed sign-in name that is no member.
const unknownName = "unknown name"

// EventAccountChanged is a change to someone's account: role, permissions,
// turned off or on, password set or changed, two-factor reset or turned on
// or off, sessions ended, an invite made. Never a password, code or key.
const EventAccountChanged = "account_changed"

// event writes a sign-in or API key Event with the caller's address. It
// never holds a password or a key.
func (a *Auth) event(r *http.Request, by domain.Actor, kind, summary string, data map[string]any) {
	data["ip"] = a.eventAddr(r)
	_ = a.s.AddEvent(context.WithoutCancel(r.Context()), &domain.Event{Actor: by, Kind: kind, Summary: summary, Data: data})
}

// knownName is the member (in any state) a typed sign-in name belongs to,
// or unknownName.
func (a *Auth) knownName(ctx context.Context, typed string) string {
	n, err := nameOfTyped(ctx, a.s.Read(), typed)
	if err != nil {
		return unknownName
	}
	return n
}

// failedSummary is a failed sign-in row's line: how it failed and for whom.
func failedSummary(name, how string) string {
	switch how {
	case "", "password":
		return "Sign-in failed for " + name
	case "two-factor":
		return "Wrong two-factor code for " + name
	case "turned off":
		return "Sign-in by turned-off account " + name
	}
	return "Sign-in failed for " + name + " (" + how + ")"
}

// failedEvent logs a failed sign-in and how it failed (password,
// two-factor, turned off, jellyfin, emby, not allowed). Repeats from the
// same address for the same name and way within 10 minutes of the first add
// to that row's count and last_at instead of making new rows.
func (a *Auth) failedEvent(r *http.Request, name, how string) {
	ip := a.eventAddr(r)
	now := a.now().UTC()
	ctx := context.WithoutCancel(r.Context())
	var n int64
	_ = a.s.WriteEvents(ctx, func(tx *sql.Tx) error {
		var err error
		n, err = database.BumpFailedSignIn(ctx, tx, now, ip, name, how)
		return err
	})
	if n > 0 {
		return
	}
	_ = a.s.AddEvent(ctx, &domain.Event{At: now, Kind: "sign_in_failed", Summary: failedSummary(name, how),
		Data: map[string]any{"name": name, "how": how, "ip": ip, "count": 1, "last_at": database.FormatTime(now)}})
}

// ssoRefusal is how an OIDC or forward-auth sign-in was refused, for its row.
func ssoRefusal(kind string, err error) string {
	switch {
	case errors.Is(err, errDisabled):
		return "turned off"
	case errors.Is(err, errNotAllowed):
		return kind + " not allowed"
	}
	return kind + " refused"
}

// accountEvent writes an account_changed Event about the member userID.
func (a *Auth) accountEvent(r *http.Request, userID int64, action, summary string, extra map[string]any) {
	data := map[string]any{"user_id": userID, "action": action}
	for k, v := range extra {
		data[k] = v
	}
	a.event(r, webserver.Actor(r.Context()), EventAccountChanged, summary, data)
}

// userName is a member's name ("member <id>" when it cannot be read).
func (a *Auth) userName(ctx context.Context, id int64) string {
	n, err := nameOfUser(ctx, a.s.Read(), id)
	if err != nil || n == "" {
		return "member " + itoa(id)
	}
	return n
}

// byName is who made a change, for an Event line.
func byName(r *http.Request) string {
	if who, ok := webserver.From(r.Context()); ok && who.Name != "" {
		return who.Name
	}
	return "someone"
}

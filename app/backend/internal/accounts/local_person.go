package accounts

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strconv"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// keyLocalPerson is the sign-in setting with the id of the person local
// visitors become; missing or 0 means the owner.
const keyLocalPerson = "local_person"

// Why the skip is not applied right now, for the Settings card.
const (
	skipOffKid    = "kid"         // a Kid or an age-limited invite exists and the person is not a Kid
	skipOffPerson = "person_gone" // the picked person was deleted or turned off
)

// loadLocalPerson reads the saved person; anything unreadable is the owner.
func (a *Auth) loadLocalPerson(ctx context.Context) {
	id, _ := strconv.ParseInt(a.signInSetting(ctx, keyLocalPerson), 10, 64)
	if id < 0 {
		id = 0
	}
	a.localPerson.Store(id)
}

// localPersonID is the picked person's id, the active owner's when none is
// picked; 0 when there is nobody to be.
func (a *Auth) localPersonID(ctx context.Context) int64 {
	if id := a.localPerson.Load(); id != 0 {
		return id
	}
	id, _, ok := a.owner(ctx)
	if !ok {
		return 0
	}
	return id
}

// localCaller is who a local visitor becomes: the picked person with their
// own role, permissions and age limit. reason says why there is nobody: the
// person is gone or turned off (never the owner instead), or the Kid guard
// holds.
func (a *Auth) localCaller(ctx context.Context) (who webserver.Caller, reason string, ok bool) {
	id := a.localPersonID(ctx)
	if id == 0 {
		return who, skipOffPerson, false
	}
	perms, err := activeCaller(ctx, a.s.Read(), id, &who)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("auth: local visitor: %v", err)
		}
		return webserver.Caller{}, skipOffPerson, false
	}
	kids, invites, err := localSkipFacts(ctx, a.s.Read(), database.FormatTime(a.now()))
	if err != nil {
		log.Printf("auth: local visitor: %v", err)
		return webserver.Caller{}, skipOffPerson, false
	}
	// Anyone at home becomes this person, so with a Kid around only a Kid will do.
	if who.MaxRating == "" && (kids > 0 || invites > 0) {
		return webserver.Caller{}, skipOffKid, false
	}
	a.fill(&who, perms)
	who.Via = "local_network"
	return who, "", true
}

// skipBlocksKids says whether a new age limit must be refused: sign-in is
// skipped at home and the person local visitors become is not a Kid.
func (a *Auth) skipBlocksKids(ctx context.Context) bool {
	if !a.skipLocal.Load() {
		return false
	}
	var who webserver.Caller
	if _, err := activeCaller(ctx, a.s.Read(), a.localPersonID(ctx), &who); err != nil {
		return !errors.Is(err, sql.ErrNoRows) // nobody to be: the skip does not apply
	}
	return who.MaxRating == ""
}

// checkLocalPerson checks a picked person: 0 (the owner) or an active person.
func (a *Auth) checkLocalPerson(ctx context.Context, id int64) (webserver.Caller, error) {
	var who webserver.Caller
	if id < 0 {
		return who, errors.New("pick a person for local visitors")
	}
	if id == 0 {
		var ok bool
		if id, _, ok = a.owner(ctx); !ok {
			return who, errors.New("there is no owner yet")
		}
	}
	if _, err := activeCaller(ctx, a.s.Read(), id, &who); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return who, errors.New("pick a person who exists and is turned on")
		}
		return who, err
	}
	return who, nil
}

// kidGuardMsg is the refusal when skipping would let a non-Kid in at home
// while a Kid or an age-limited invite exists; "" when it is fine.
func (a *Auth) kidGuardMsg(ctx context.Context, person webserver.Caller) (string, error) {
	if person.MaxRating != "" {
		return "", nil
	}
	kids, invites, err := localSkipFacts(ctx, a.s.Read(), database.FormatTime(a.now()))
	if err != nil {
		return "", err
	}
	switch {
	case kids > 0:
		return "Skipping sign-in on the local network cannot be on while a Kid account has an age limit: anyone at home would sign in as the owner. Remove the limits first.", nil
	case invites > 0:
		return "Skipping sign-in on the local network cannot be on while an open invite has an age limit: the Kid it makes would sign in as the owner at home. Withdraw the invite or let it run out first.", nil
	}
	return "", nil
}

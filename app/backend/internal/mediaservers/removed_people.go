package mediaservers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// CheckPeopleAndHistory is the scheduled media-server read: removed people
// first, then the watch history.
func (l *Linker) CheckPeopleAndHistory(ctx context.Context) error {
	return errors.Join(l.CheckRemovedPeople(ctx), l.ReadWatchHistory(ctx))
}

// CheckRemovedPeople turns off the people here linked to a Jellyfin or Emby
// account that is no longer on the sign-in server. That server's user list is
// read with the key of the library server connected at the same address;
// with no such server, or when the list cannot be read, nothing changes.
func (l *Linker) CheckRemovedPeople(ctx context.Context) error {
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil {
		return err
	}
	for _, kind := range []string{"jellyfin", "emby"} {
		addr := accounts.SignInServer(ctx, l.Store.Read(), kind)
		read := false
		for _, m := range ms {
			if addr == "" || m.Kind != kind || m.TokenSecretID == nil || !strings.EqualFold(strings.TrimRight(m.URL, "/"), addr) {
				continue
			}
			read = true
			present, err := l.userIDs(ctx, m)
			if nerr := accounts.NoteUserListRead(ctx, l.Store, kind, err == nil); nerr != nil {
				return nerr
			}
			if err != nil {
				log.Printf("media servers: %s: removed people check: %v", m.Name, err)
				break
			}
			if err := accounts.TurnOffRemoved(ctx, l.Store, kind, present); err != nil {
				return err
			}
			break
		}
		if !read {
			// Nothing to read any more: the people check and its warning go.
			if err := accounts.ForgetUserListRead(ctx, l.Store, kind); err != nil {
				return err
			}
		}
	}
	return nil
}

// The user list is read as it arrives, keeping only ids, so big servers
// fit; past these bounds the read counts as failed.
const (
	maxUsers     = 50000
	maxUserBytes = 64 << 20
)

// userIDs are the account ids on a Jellyfin or Emby server. An empty list is
// an error: a server always has at least its admin.
func (l *Linker) userIDs(ctx context.Context, m domain.MediaServer) (map[string]bool, error) {
	out := map[string]bool{}
	notList := errors.New("the server's answer was not a user list")
	var bad error
	err := l.doStream(ctx, m, http.MethodGet, "/Users", nil, maxUserBytes, func(r io.Reader) error {
		dec := json.NewDecoder(r)
		if t, err := dec.Token(); err != nil || t != json.Delim('[') {
			bad = notList
			return nil
		}
		for dec.More() {
			var u struct {
				ID string `json:"Id"`
			}
			if err := dec.Decode(&u); err != nil {
				return err
			}
			if u.ID != "" {
				out[u.ID] = true
			}
			if len(out) > maxUsers {
				bad = errors.New("the server listed too many people to check")
				return nil
			}
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		err = bad
	}
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("the server listed nobody")
	}
	return out, nil
}

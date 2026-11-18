package mediaservers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
)

// LinkServers lists the switched-on Jellyfin and Emby servers with a kept
// key, the ones a person may link their account on.
func (l *Linker) LinkServers(ctx context.Context) ([]accounts.LinkServer, error) {
	ms, err := enabledServers(ctx, l.Store.Read())
	if err != nil {
		return nil, err
	}
	out := []accounts.LinkServer{}
	for _, m := range ms {
		if (m.Kind == "jellyfin" || m.Kind == "emby") && m.TokenSecretID != nil {
			out = append(out, accounts.LinkServer{ID: m.ID, Name: m.Name, Kind: m.Kind, URL: m.URL})
		}
	}
	return out, nil
}

// ServerAccounts reads the accounts of one Jellyfin or Emby server.
func (l *Linker) ServerAccounts(ctx context.Context, id int64) ([]accounts.ServerAccount, error) {
	m, err := getServer(ctx, l.Store.Read(), id)
	if err != nil {
		return nil, err
	}
	b, err := l.do(ctx, m, http.MethodGet, "/Users", nil)
	if err != nil {
		return nil, err
	}
	var us []struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	if err := json.Unmarshal(b, &us); err != nil {
		return nil, err
	}
	out := []accounts.ServerAccount{}
	for _, u := range us {
		if u.ID != "" {
			out = append(out, accounts.ServerAccount{ID: u.ID, Name: u.Name})
		}
	}
	return out, nil
}

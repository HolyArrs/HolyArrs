package metadata

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// The owner's own keys, kept encrypted in the vault (links group). TMDB's
// is the same one the secrets-file import writes.
const (
	KeyGroup   = "links"
	KeyTMDB    = "tmdb_token"
	KeyTVDB    = "tvdb_key"
	KeyTVDBPIN = "tvdb_pin"
	KeyMDBList = "mdblist_key"
	KeyFanart  = "fanart_key"
)

// OwnKeyNames are the own-key settings, in the order Settings lists them.
var OwnKeyNames = []string{KeyTMDB, KeyTVDB, KeyTVDBPIN, KeyMDBList, KeyFanart}

// KeyReader reads one of the owner's own keys ("" when not set).
func (h *Hosted) KeyReader(name string) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		if h.Vault == nil {
			return "", nil
		}
		return h.Vault.Setting(ctx, KeyGroup, name)
	}
}

// directClient reaches outside sources itself, only while Your own keys is
// picked: public addresses only and no proxy (the policy must see the real
// address); a stand-in (tests) may be on this machine.
func directClient(m *Modes, standIn bool) *http.Client { return m.Client(standIn, OwnOnly) }

// Client is a client to an outside source that sends only while allow says
// so for the current mode (ErrSourceOff otherwise); public addresses only
// unless standIn (tests).
func (m *Modes) Client(standIn bool, allow func(Mode) bool) *http.Client {
	return &http.Client{Timeout: 20 * time.Second, Transport: m.Guard(publicTransport(standIn), allow)}
}

// publicTransport reaches public addresses only, with no proxy (the policy
// must see the real address); a stand-in (tests) may be on this machine.
func publicTransport(standIn bool) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	if !standIn {
		t.DialContext = outbound.PolicyDialer(outbound.PublicOnly).DialContext
	}
	return t
}

// Stand-ins for the own-key sources (tests). Each is asked directly when set.
const (
	EnvTVDBURL    = "HOLYARRS_TVDB_URL"
	EnvMDBListURL = "HOLYARRS_MDBLIST_URL"
	EnvFanartURL  = "HOLYARRS_FANART_URL"
)

// TestKey checks one own key before it is used: a TMDB token, a TheTVDB
// key with its PIN, an MDBList or a fanart.tv key. The owner asked for it,
// so it goes out in any mode, over public addresses only.
func (h *Hosted) TestKey(ctx context.Context, key, value, pin string) error {
	stand := func(env string) bool { return os.Getenv(env) != "" }
	cl := &http.Client{Timeout: 15 * time.Second, Transport: publicTransport(h.stand != "" || stand(EnvTVDBURL) || stand(EnvMDBListURL) || stand(EnvFanartURL))}
	fixed := func(context.Context) (string, error) { return value, nil }
	switch key {
	case KeyTMDB:
		t := &TMDB{Route: func(context.Context) TMDBRoute { return TMDBRoute{Base: h.stand, Access: fixed, HTTP: cl} }}
		_, err := t.get(NoCache(ctx), "/configuration", nil)
		return err
	case KeyTVDB:
		d := &TVDB{BaseURL: os.Getenv(EnvTVDBURL), Key: fixed, PIN: func(context.Context) (string, error) { return pin, nil }, HTTP: cl}
		_, err := d.session(ctx, true)
		return err
	case KeyMDBList, KeyFanart:
		o := &OwnExtras{MDBListBase: os.Getenv(EnvMDBListURL), FanartBase: os.Getenv(EnvFanartURL), HTTP: cl}
		var err error
		if key == KeyMDBList {
			o.MDBListKey = fixed
			_, err = o.Ratings(ctx, "movie", "603")
		} else {
			o.FanartKey = fixed
			_, err = o.Art(ctx, "movie", "603")
		}
		if errors.Is(err, ErrNotFound) {
			return nil // the key was taken; that title just has nothing
		}
		return err
	}
	return errors.New("unknown key")
}

// TMDBRoute is where a TMDB request goes now:
//   - a key from the environment (HOLYARRS_TMDB_KEY or the token file):
//     TMDB directly, in every mode but off;
//   - the metadata server: through it, with this install's credential;
//   - Your own keys: TMDB directly with the key from Settings;
//   - off: nowhere; answers kept from the server still answer.
func (h *Hosted) TMDBRoute(ctx context.Context) TMDBRoute {
	mode := h.Modes.Current()
	server := TMDBRoute{Base: h.Service.Base() + TMDBPath, Access: h.TMDBAccess, HTTP: h.HTTP}
	switch {
	case h.OwnKey:
		return TMDBRoute{Base: h.TMDBBase, Access: h.TMDBAccess, Off: mode == ModeOff}
	case mode == ModeServer:
		return server
	case mode == ModeOwn:
		return TMDBRoute{Base: h.stand, Access: h.KeyReader(KeyTMDB), HTTP: h.Direct}
	}
	server.Off = true
	return server
}

// ListRoute is TMDBRoute for the Outside lists sync: the address, the
// credential and the client, or ErrSourceOff.
func (h *Hosted) ListRoute(ctx context.Context) (string, func(context.Context) (string, error), *http.Client, error) {
	r := h.TMDBRoute(ctx)
	if r.Off {
		return "", nil, nil, ErrSourceOff
	}
	return r.Base, r.Access, r.HTTP, nil
}

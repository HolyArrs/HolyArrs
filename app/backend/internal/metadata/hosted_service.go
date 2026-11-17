package metadata

import (
	"context"
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// installationSetting is where the install's own metadata server credential
// is kept (encrypted). It is never shown, logged or exported.
const installationGroup, installationKey = "links", "metadata_service_installation_token"

// Hosted is the metadata server every install uses (ServiceURL):
// TMDB reads go through its /v1/tmdb with this install's credential, music
// lookups through its schema-1 API. A person's own TMDB key
// (HOLYARRS_TMDB_KEY, or the lab's HOLYARRS_TMDB_TOKEN_FILE) sends TMDB reads
// straight to TMDB instead. There is no fallback from the server to TMDB.
type Hosted struct {
	Service    *Service
	Health     *Health
	TMDBBase   string
	TMDBAccess func(context.Context) (string, error)
	Images     string
	OwnKey     bool
	HTTP       *http.Client
	// Transport is the pooled connection to the server, shared with the
	// artwork cache for the server's picture addresses.
	Transport http.RoundTripper
	// Gate is that transport; it carries "Count this install".
	Gate *Gate
	// Disk keeps TMDB answers across restarts (lookup_cache).
	Disk Cache
	// Modes is where details come from (server, own keys or off).
	Modes *Modes
	// Vault holds the owner's own keys.
	Vault *secrets.Vault
	// Direct reaches outside sources over public addresses while Your own
	// keys is picked (a stand-in may be on this machine).
	Direct *http.Client
	// stand is the TMDB stand-in (tests), "" for TMDB itself.
	stand string
}

func NewHosted(v *secrets.Vault) (*Hosted, error) {
	local := false
	if raw := os.Getenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL"); raw != "" {
		var err error
		local, err = strconv.ParseBool(raw)
		if err != nil {
			return nil, errors.New("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL must be true or false")
		}
	}
	allow := outbound.PublicOnly
	if local {
		allow = outbound.MetadataServiceLocal
	}
	d := outbound.PolicyDialer(allow)
	// One pooled HTTP/2 transport for every call to the server: text
	// answers here, and the server's pictures through the artwork client.
	modes := EnvModes()
	tr := &Gate{Next: NewServerTransport(d.DialContext), Modes: modes}
	cl := &http.Client{Transport: keyCheck{next: tr, refused: dropUnknownKey(v)}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	h := &Hosted{HTTP: cl, Transport: tr, Gate: tr, Modes: modes, Vault: v}
	access := installationAccess(v, func(ctx context.Context) (string, error) { return h.Service.Register(ctx) })
	service, err := NewService(ServiceOptions{URL: ServiceURL(), HTTP: cl, Access: access, AllowLocal: local})
	if err != nil {
		return nil, err
	}
	h.Service, h.Health = service, &Health{Service: service}
	modes.OnSet = service.ResetRegisterWait
	// HOLYARRS_TMDB_URL points at a stand-in TMDB (browser tests); it
	// serves pictures too.
	stand := strings.TrimRight(os.Getenv("HOLYARRS_TMDB_URL"), "/")
	if stand != "" {
		h.Images = stand + "/images"
		webserver.ImageHosts += " " + stand
	}
	h.stand = stand
	// Title art from fanart.tv with the owner's own key loads from its host.
	if fan := strings.TrimSuffix(FanartPictureHost, "/"); !strings.Contains(webserver.ImageHosts, fan) {
		webserver.ImageHosts += " " + fan
	}
	h.Direct = directClient(modes, stand != "")
	switch key, file := strings.TrimSpace(os.Getenv(EnvTMDBKey)), os.Getenv(EnvTokenFile); {
	case key != "":
		h.OwnKey, h.TMDBBase = true, stand
		h.TMDBAccess = func(context.Context) (string, error) { return key, nil }
	case file != "":
		h.OwnKey, h.TMDBBase = true, stand
		h.TMDBAccess = TokenFile(file)
	default:
		h.TMDBBase, h.TMDBAccess = service.Base()+TMDBPath, access
	}
	switch {
	case modes.BadValue() != "":
		log.Printf("metadata: %s is not full, own or off, so lookups are off", EnvMode)
	case modes.Locked():
		log.Printf("metadata: details come from %s (set by the Docker settings)", modes.Current())
	case h.OwnKey:
		log.Printf("metadata: TMDB directly with the key from the environment")
	default:
		log.Printf("metadata: the HolyArrs metadata server at %s", service.Base())
	}
	return h, nil
}

// TMDB is a TMDB client for movies, shows, Explore, people and ratings. Each
// request goes where the mode says (TMDBRoute).
func (h *Hosted) TMDB() *TMDB {
	return &TMDB{Route: h.TMDBRoute, Images: h.Images, Disk: h.Disk}
}

// installationAccess reads the install's credential: from the file named by
// HOLYARRS_METADATA_SERVICE_TOKEN_FILE (read on each call), else from the
// vault. A fresh install has none yet: it registers once with the server and
// keeps the answer. A credential the server no longer knows is dropped by
// dropUnknownKey, and the next call registers again.
func installationAccess(v *secrets.Vault, register func(context.Context) (string, error)) func(context.Context) (string, error) {
	if path := os.Getenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE"); path != "" {
		return func(context.Context) (string, error) {
			f, err := os.Open(path)
			if err != nil {
				return "", ErrServiceAccess
			}
			defer f.Close()
			b, err := io.ReadAll(io.LimitReader(f, 4097))
			if err != nil || len(b) > 4096 {
				return "", ErrServiceAccess
			}
			return strings.TrimSpace(string(b)), nil
		}
	}
	var mu sync.Mutex
	return func(ctx context.Context) (string, error) {
		if v == nil {
			return "", ErrServiceAccess
		}
		if tok, err := v.Setting(ctx, installationGroup, installationKey); err != nil || tok != "" {
			return tok, err
		}
		mu.Lock()
		defer mu.Unlock()
		if tok, err := v.Setting(ctx, installationGroup, installationKey); err != nil || tok != "" {
			return tok, err
		}
		tok, err := register(ctx)
		if err != nil {
			return "", err
		}
		if err := v.SetSetting(ctx, installationGroup, installationKey, tok, database.System); err != nil {
			return "", err
		}
		log.Printf("metadata: this install registered with the metadata server")
		return tok, nil
	}
}

// keyCheck watches the server's answers: a 401 to a call that carried the
// install's credential means the server does not know it (any more).
type keyCheck struct {
	next    http.RoundTripper
	refused func(tok string)
}

func (k keyCheck) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := k.next.RoundTrip(req)
	if err == nil && resp.StatusCode == http.StatusUnauthorized && k.refused != nil {
		if tok, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer "); ok && tok != "" {
			k.refused(tok)
		}
	}
	return resp, err
}

// unknownKeyWait is the least time between two dropped credentials. The
// server gives an address only a few registrations an hour.
const unknownKeyWait = 30 * time.Minute

// dropUnknownKey forgets the stored credential when the server refused
// exactly that one, so the next call registers a new one. The server drops
// credentials on its own (idle ones, or to make room); without this the
// install would stay refused for good. A credential from the token file is
// the owner's to change and is left alone.
func dropUnknownKey(v *secrets.Vault) func(string) {
	if v == nil || os.Getenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE") != "" {
		return nil
	}
	var mu sync.Mutex
	var last time.Time
	return func(tok string) {
		mu.Lock()
		defer mu.Unlock()
		if !last.IsZero() && time.Since(last) < unknownKeyWait {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if cur, err := v.Setting(ctx, installationGroup, installationKey); err != nil || cur != tok {
			return
		}
		if err := v.SetSetting(ctx, installationGroup, installationKey, "", database.System); err != nil {
			return
		}
		last = time.Now()
		log.Printf("metadata: the server no longer knows this install; registering again")
	}
}

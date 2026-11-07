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
	tr := &Gate{Next: NewServerTransport(d.DialContext)}
	cl := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	h := &Hosted{HTTP: cl, Transport: tr, Gate: tr}
	access := installationAccess(v, func(ctx context.Context) (string, error) { return h.Service.Register(ctx) })
	service, err := NewService(ServiceOptions{URL: ServiceURL(), HTTP: cl, Access: access, AllowLocal: local})
	if err != nil {
		return nil, err
	}
	h.Service, h.Health = service, &Health{Service: service}
	// HOLYARRS_TMDB_URL points at a stand-in TMDB (browser tests); it
	// serves pictures too.
	stand := strings.TrimRight(os.Getenv("HOLYARRS_TMDB_URL"), "/")
	if stand != "" {
		h.Images = stand + "/images"
		webserver.ImageHosts += " " + stand
	}
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
	if h.OwnKey {
		log.Printf("metadata: TMDB directly with the key from the environment")
	} else {
		log.Printf("metadata: the HolyArrs metadata server at %s", service.Base())
	}
	return h, nil
}

// TMDB is a TMDB client for movies, shows, Explore, people and ratings.
func (h *Hosted) TMDB() *TMDB {
	t := &TMDB{Access: h.TMDBAccess, BaseURL: h.TMDBBase, Images: h.Images, Disk: h.Disk}
	if !h.OwnKey {
		t.HTTP = h.HTTP
	}
	return t
}

// installationAccess reads the install's credential: from the file named by
// HOLYARRS_METADATA_SERVICE_TOKEN_FILE (read on each call), else from the
// vault. A fresh install has none yet: it registers once with the server and
// keeps the answer. A refused credential is never replaced by registering
// again, so a revocation sticks.
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

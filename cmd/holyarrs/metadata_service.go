package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/source"
)

// installationSetting is where the install's own metadata server credential
// is kept (encrypted). It is never shown, logged or exported.
const installationGroup, installationKey = "links", "metadata_service_installation_token"

// hostedMetadata is the metadata server every install uses (metadata.ServiceURL):
// TMDB reads go through its /v1/tmdb with this install's credential, music
// lookups through its schema-1 API. A person's own TMDB key
// (HOLYARRS_TMDB_KEY, or the lab's HOLYARRS_TMDB_TOKEN_FILE) sends TMDB reads
// straight to TMDB instead. There is no fallback from the server to TMDB.
type hostedMetadata struct {
	Service    *metadata.Service
	Health     *metadata.Health
	TMDBBase   string
	TMDBAccess func(context.Context) (string, error)
	Images     string
	OwnKey     bool
	HTTP       *http.Client
}

func newHostedMetadata(v *secrets.Vault) (*hostedMetadata, error) {
	local := false
	if raw := os.Getenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL"); raw != "" {
		var err error
		local, err = strconv.ParseBool(raw)
		if err != nil {
			return nil, errors.New("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL must be true or false")
		}
	}
	allow := source.PublicOnly
	if local {
		allow = metadataServiceLocal
	}
	d := source.PolicyDialer(allow)
	cl := &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: d.DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 16 * time.Second, MaxResponseHeaderBytes: 32 << 10, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	h := &hostedMetadata{HTTP: cl}
	access := installationAccess(v, func(ctx context.Context) (string, error) { return h.Service.Register(ctx) })
	service, err := metadata.NewService(metadata.ServiceOptions{URL: metadata.ServiceURL(), HTTP: cl, Access: access, AllowLocal: local})
	if err != nil {
		return nil, err
	}
	h.Service, h.Health = service, &metadata.Health{Service: service}
	// HOLYARRS_TMDB_URL points at a stand-in TMDB (browser tests); it
	// serves pictures too.
	stand := strings.TrimRight(os.Getenv("HOLYARRS_TMDB_URL"), "/")
	if stand != "" {
		h.Images = stand + "/images"
	}
	switch key, file := strings.TrimSpace(os.Getenv(metadata.EnvTMDBKey)), os.Getenv(metadata.EnvTokenFile); {
	case key != "":
		h.OwnKey, h.TMDBBase = true, stand
		h.TMDBAccess = func(context.Context) (string, error) { return key, nil }
	case file != "":
		h.OwnKey, h.TMDBBase = true, stand
		h.TMDBAccess = metadata.TokenFile(file)
	default:
		h.TMDBBase, h.TMDBAccess = service.Base()+metadata.TMDBPath, access
	}
	if h.OwnKey {
		log.Printf("metadata: TMDB directly with the key from the environment")
	} else {
		log.Printf("metadata: the HolyArrs metadata server at %s", service.Base())
	}
	return h, nil
}

// TMDB is a TMDB client for movies, shows, Explore, people and ratings.
func (h *hostedMetadata) TMDB() *metadata.TMDB {
	t := &metadata.TMDB{Access: h.TMDBAccess, BaseURL: h.TMDBBase, Images: h.Images}
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
				return "", metadata.ErrServiceAccess
			}
			defer f.Close()
			b, err := io.ReadAll(io.LimitReader(f, 4097))
			if err != nil || len(b) > 4096 {
				return "", metadata.ErrServiceAccess
			}
			return strings.TrimSpace(string(b)), nil
		}
	}
	var mu sync.Mutex
	return func(ctx context.Context) (string, error) {
		if v == nil {
			return "", metadata.ErrServiceAccess
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
		if err := v.SetSetting(ctx, installationGroup, installationKey, tok, domain.System); err != nil {
			return "", err
		}
		log.Printf("metadata: this install registered with the metadata server")
		return tok, nil
	}
}

func metadataServiceLocal(ip net.IP) bool {
	return ip != nil && source.NotMetadata(ip) && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsInterfaceLocalMulticast() && !ip.Equal(net.IPv4bcast)
}

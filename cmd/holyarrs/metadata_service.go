package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/source"
)

// metadataServiceProviders keeps the current providers unless the operator
// explicitly selects the distinct service API. Provider keys stay upstream.
func metadataServiceProviders(v *secrets.Vault, movies metadata.Provider, shows metadata.ShowSources, artists metadata.ArtistProvider) (metadata.Provider, metadata.ShowSources, metadata.ArtistProvider, error) {
	base := os.Getenv("HOLYARRS_METADATA_SERVICE_URL")
	if base == "" {
		return movies, shows, artists, nil
	}
	local := false
	if raw := os.Getenv("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL"); raw != "" {
		var err error
		local, err = strconv.ParseBool(raw)
		if err != nil {
			return nil, nil, nil, errors.New("HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL must be true or false")
		}
	}
	access := func(ctx context.Context) (string, error) {
		if v == nil {
			return "", metadata.ErrServiceAccess
		}
		return v.Setting(ctx, "links", "metadata_service_installation_token")
	}
	if path := os.Getenv("HOLYARRS_METADATA_SERVICE_TOKEN_FILE"); path != "" {
		access = func(context.Context) (string, error) {
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
	allow := source.PublicOnly
	if local {
		allow = metadataServiceLocal
	}
	d := source.PolicyDialer(allow)
	cl := &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: d.DialContext, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 16 * time.Second, MaxResponseHeaderBytes: 32 << 10, MaxConnsPerHost: 4, MaxIdleConnsPerHost: 2, IdleConnTimeout: 30 * time.Second}}
	service, err := metadata.NewService(metadata.ServiceOptions{URL: base, HTTP: cl, Access: access, AllowLocal: local})
	if err != nil {
		return nil, nil, nil, err
	}
	wrapped := metadata.ShowSources{}
	for name, p := range shows {
		if name == "tmdb" || name == "tvmaze" {
			wrapped[name] = service.Shows(name, p)
		} else {
			wrapped[name] = p
		}
	}
	return service.Movies(movies), wrapped, service.Artists(artists), nil
}

func metadataServiceLocal(ip net.IP) bool {
	return ip != nil && source.NotMetadata(ip) && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsInterfaceLocalMulticast() && !ip.Equal(net.IPv4bcast)
}

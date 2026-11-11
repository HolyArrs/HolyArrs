package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// showSources are the show details sources: TMDB (with anime release
// numbers) and TVmaze through the metadata server. TVDB is off; TVDB ids are still found through these two.
// HOLYARRS_TVMAZE_URL points at a stand-in (tests); a
// TVmaze stand-in is asked directly, and HOLYARRS_DIRECT_SOURCES=1
// (developers only) keeps the real TVmaze behind the server for TVDB and
// IMDb ids.
func showSources(v *secrets.Vault, hosted *metadata.Hosted) metadata.ShowSources {
	var maze metadata.ShowProvider = &metadata.TVMaze{BaseURL: os.Getenv("HOLYARRS_TVMAZE_URL")}
	if !standIn("HOLYARRS_TVMAZE_URL") {
		var direct metadata.ShowProvider
		if directSources() {
			direct = maze
		}
		maze = hosted.Service.Shows("tvmaze", direct)
	}
	return metadata.ShowSources{
		"tmdb":   hosted.Service.AnimeShows(hosted.TMDB()),
		"tvmaze": maze,
	}
}

// directSources reports HOLYARRS_DIRECT_SOURCES=1: a developer stand-in
// that lets this install ask Deezer, Wikipedia, MusicBrainz and TVmaze
// itself. Off by default and never for installs: they ask the metadata
// server only.
func directSources() bool { return os.Getenv("HOLYARRS_DIRECT_SOURCES") == "1" }

// standIn reports a stand-in address (tests) named by the variable.
func standIn(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	return v != "" && v != "off"
}

// artistImages is the direct Deezer and Wikipedia client, a stand-in
// only: HOLYARRS_DEEZER_URL names a stand-in (tests), and
// HOLYARRS_DIRECT_SOURCES=1 (developers) reaches the real Deezer over
// public addresses. Off otherwise ("off", or nothing named): the metadata
// server answers instead.
func artistImages() *metadata.ArtistImages {
	env := strings.TrimRight(os.Getenv("HOLYARRS_DEEZER_URL"), "/")
	switch {
	case env == "off", env == "" && (os.Getenv("HOLYARRS_MUSICBRAINZ_URL") != "" || !directSources()):
		return nil
	case env != "":
		return &metadata.ArtistImages{BaseURL: env}
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil // the policy must see the real address
	t.DialContext = outbound.PolicyDialer(outbound.PublicOnly).DialContext
	return &metadata.ArtistImages{BaseURL: metadata.DeezerBase, HTTP: &http.Client{Transport: t}}
}

// musicSource is MusicBrainz at no more than one request a second (one
// shared limiter), with answers kept in the lookup_cache table. The base
// URL and cache hours are settings (Settings > Music), read on each
// request; HOLYARRS_MUSICBRAINZ_URL points at a stand-in (tests) and wins.
// Its own clients reach public addresses only, or the local network too
// (never cloud metadata addresses) when the music settings allow it or
// the stand-in is used.
func musicSource(s *database.Store, version string) *metadata.MusicBrainz {
	env := os.Getenv("HOLYARRS_MUSICBRAINZ_URL")
	client := func(allow func(net.IP) bool) *http.Client {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil // the policy must see the real address
		t.DialContext = outbound.PolicyDialer(allow).DialContext
		return &http.Client{Transport: t}
	}
	public, local := client(outbound.PublicOnly), client(outbound.NotMetadata)
	return &metadata.MusicBrainz{
		Client: func(ctx context.Context) *http.Client {
			if env != "" {
				return local
			}
			if d, _ := library.ReadMusicDefaults(ctx, s.Read()); d.AllowLocal {
				return local
			}
			return public
		},
		BaseURL: func(ctx context.Context) string {
			if env != "" {
				return env
			}
			d, _ := library.ReadMusicDefaults(ctx, s.Read())
			return d.MusicBrainzURL
		},
		TTL: func(ctx context.Context) time.Duration {
			d, _ := library.ReadMusicDefaults(ctx, s.Read())
			return time.Duration(d.CacheHours) * time.Hour
		},
		UserAgent: metadata.UserAgentFor(version),
		Cache:     metadata.LookupCache{Store: s},
	}
}

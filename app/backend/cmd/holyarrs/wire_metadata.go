package main

import (
	"context"
	"os"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// Where details come from (Settings > Metadata): every call below goes the
// way the mode says, and a change applies at once.

// ownMusic is MusicBrainz asked directly. It answers only in Your own keys
// mode (the metadata server answers in its own mode, and off asks nothing);
// HOLYARRS_DIRECT_SOURCES=1 (developers only) keeps it on in every mode.
func ownMusic(s *database.Store, version string, hosted *metadata.Hosted) metadata.ArtistProvider {
	mb := musicSource(s, version)
	if !directSources() {
		mb.Allowed = hosted.Modes.Direct
	}
	return mb
}

// ownExtras reads ratings from MDBList and title art from fanart.tv with
// the owner's own keys, in Your own keys mode only. HOLYARRS_MDBLIST_URL
// and HOLYARRS_FANART_URL name stand-ins (tests).
func ownExtras(hosted *metadata.Hosted) *metadata.OwnExtras {
	mdb, fan := os.Getenv(metadata.EnvMDBListURL), os.Getenv(metadata.EnvFanartURL)
	return &metadata.OwnExtras{MDBListBase: mdb, FanartBase: fan,
		MDBListKey: hosted.KeyReader(metadata.KeyMDBList), FanartKey: hosted.KeyReader(metadata.KeyFanart),
		HTTP: hosted.Modes.Client(mdb != "" || fan != "", metadata.OwnOnly)}
}

// ownPictures adds Deezer and Wikipedia asked directly to the music calls,
// used in Your own keys mode in place of the server's kept answers.
func ownPictures(d music.Deps, hosted *metadata.Hosted, version string) music.Deps {
	env := os.Getenv("HOLYARRS_DEEZER_URL")
	base := metadata.DeezerBase
	if env != "" && env != "off" {
		base = env
	}
	d.OwnPictures = &metadata.ArtistImages{BaseURL: base, HTTP: hosted.Modes.Client(base != metadata.DeezerBase, metadata.OwnOnly), UserAgent: metadata.UserAgentFor(version)}
	d.OwnMode = hosted.Modes.Direct
	return d
}

// tvdbSource is TheTVDB with the owner's own key and PIN, in every mode but
// off. HOLYARRS_TVDB_URL names a stand-in (tests).
func tvdbSource(v *secrets.Vault, hosted *metadata.Hosted) metadata.ShowProvider {
	read := func(name string) func(ctx context.Context) (string, error) {
		return func(ctx context.Context) (string, error) {
			if v == nil {
				return "", nil
			}
			return v.Setting(ctx, metadata.KeyGroup, name)
		}
	}
	env := os.Getenv(metadata.EnvTVDBURL)
	return &metadata.TVDB{BaseURL: env, Key: read(metadata.KeyTVDB), PIN: read(metadata.KeyTVDBPIN), HTTP: hosted.Modes.Client(env != "", metadata.NotOff)}
}

// metadataUp says whether the metadata server answers; outside its own
// mode it is not asked, so nothing reads as down.
func metadataUp(hosted *metadata.Hosted) func(context.Context) bool {
	return func(ctx context.Context) bool {
		if !hosted.Modes.Server() {
			return true
		}
		return hosted.Health.Up(ctx)
	}
}

// metadataModeHandlers serves Settings > Metadata. Tests without a Hosted
// get a picker that starts on the metadata server.
func (a *handlers) metadataModeHandlers() *metadata.ModeHandlers {
	m := a.s.MetadataModes
	if m == nil {
		m = metadata.NewModes(func(string) string { return "" })
		a.s.MetadataModes = m
	}
	return &metadata.ModeHandlers{Modes: m, Store: a.s.Store, Vault: a.s.Secrets, EnvTMDB: a.s.MetadataEnvKey, Test: a.s.MetadataTest}
}

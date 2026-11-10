package main

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"

	"log"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/music"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// services are the shared services main.go makes; buildHandlers hands
// them to every module.
type services struct {
	Store *database.Store
	// DLSync is the background download reader (downloads.sync = on); nil
	// keeps the Downloads view's own client reads.
	DLSync   *downloads.Sync
	Queue    *jobs.Queue
	Live     *webserver.LiveHub
	Metadata metadata.Provider
	Version  string
	Started  time.Time
	// Auth guards every route but the health check and the sign-in page.
	// Nil serves without sign-in (tests only).
	Auth *accounts.Auth
	// Backups serves the backup routes when set; Restart is called after a
	// restore is staged.
	Backups *backups.Manager
	Restart func()
	// Secrets stores the encrypted settings.
	Secrets *secrets.Vault
	// Bridges keeps the connected Jackett and Prowlarr; nil makes one.
	Bridges *sources.BridgeService
	// TraktBase is the Trakt API address for reviews; empty is the real one.
	TraktBase string
	// MetadataUp reports whether the metadata server answers (cached
	// about a minute); nil counts as up.
	MetadataUp func(context.Context) bool
	// ServiceTop reads every streaming service's top list for a region
	// from the metadata server; nil leaves the row empty.
	ServiceTop func(ctx context.Context, region string) (metadata.ServiceTop, error)
	// TrailerPick reads the metadata server's trailer key for a title
	// (kind movie or show); nil uses only TMDB's videos.
	TrailerPick func(ctx context.Context, kind, id, lang string) (string, error)
	// AnimeMap reads the metadata server's anime ID mapping (by tmdb_tv,
	// tmdb_movie, tvdb, anilist or mal); nil maps nothing.
	AnimeMap func(ctx context.Context, by, id string) (metadata.AnimeMap, error)
	// Deps are the metadata server calls and flags the music code reads
	// (internal/music).
	music.Deps
	// ExtraArt and ExtraRatings read the server's stored title art and outside ratings
	// from the metadata server; nil (or a source off there) shows neither.
	ExtraArt func(ctx context.Context, kind, id string) (metadata.ServerArt, error)
	// ImageBase is the metadata server's picture origin ("<server>/v1/img");
	// its sharp artist photo is the first choice for every artist picture.
	// Empty skips it.
	ImageBase    string
	ExtraRatings func(ctx context.Context, kind, id string) (metadata.ServerRatings, error)
	// WikiTitle and WikiPerson read awards and the Wikipedia bio from the
	// metadata server; AwardWinners its award-winning TMDB IDs (the Award
	// winners collection). nil hides them.
	WikiTitle    func(ctx context.Context, kind, id string) (metadata.WikiEntity, error)
	WikiPerson   func(ctx context.Context, id, name string) (metadata.WikiEntity, error)
	AwardWinners func(ctx context.Context, kind string) ([]metadata.AwardWinner, error)
	// MusicControl reads the metadata server's switch for fetch-ahead
	// (on, slow, off); ArtistVideos an artist's official music videos.
	// nil, or an older server without them, keeps fetch-ahead on and the
	// artist hero a photo.
	MusicControl func(ctx context.Context) (metadata.Control, error)
	ArtistVideos func(ctx context.Context, mbid string) ([]metadata.ArtistVideo, error)
	// CountryCount passes "Count this install" (Settings, This app) on to
	// the requests to the metadata server (metadata.Gate.SetCountryCount).
	// nil: nothing to tell.
	CountryCount func(on bool)
	// Packages is the local copy of the metadata server's artist packages
	// (internal/music): music pages answer from it first. nil: none.
	Packages *music.PackageSync
	// Library runs adding, editing and refreshing; New makes one when nil.
	Library *library.Service
	// Lists reads Outside lists; New makes one without keys when nil.
	Lists *requests.OutsideLists
	// Alerts sends Discord and email Alerts; New makes one when Secrets is set.
	Alerts *alerts.Sender
	// Asks runs Requests; New makes one when nil.
	Asks *requests.Service
	// Watchlists reads Plex watchlists (nil: no person can be read).
	Watchlists *requests.Watchlists
	// Downloads follows Downloaders and Downloads; made on first use when nil.
	Downloads *downloads.Manager
	// Search runs searches, hand choices and the Held and Turned down
	// lists; New makes one (choices queue a grab Job) when nil.
	Search *sources.Searcher
	// Health runs the health checks; New makes one when nil.
	Health *health.Engine
	// Updates is the update check; New makes one when nil and hands it to
	// Health when Health has no update check yet.
	Updates *updates.Checker
	// Media tells media servers about changed folders; New makes one when
	// Secrets is set.
	Media *mediaservers.Linker
	// Subtitles runs subtitle work; New makes one when nil.
	Subtitles *subtitles.Service
	// States holds the state of every supervised stage (may be nil).
	States *jobs.Registry
	// SafeMode says the app started with only the core.
	SafeMode bool
	// Parts switches the five parts and the stages (nil: every part on,
	// no controls).
	Parts *parts.Control
	// Mover runs moves from the old apps; made on first use when nil.
	Mover *migrate.Mover
	// ExitApp stops the process for a whole-app restart ("restart"), a
	// restart in safe mode ("safe_mode") or a shut down ("shutdown"); nil
	// refuses those controls. It returns an error, and stops nothing,
	// when it cannot arrange the exit. Tests record the call instead.
	ExitApp func(how string) error
	// Log is the app log for the log viewer and download (nil: none kept).
	Log *logs.Log
	// ConfigDir and SavePath are shown on This app (ConfigDir defaults to
	// the database folder).
	ConfigDir string
	SavePath  string
	// URLBase is the path HolyArrs runs under behind a proxy ("" for none),
	// shown on This app.
	URLBase string
}

// fill makes a default for every service main.go (or a test) left nil, so
// no module Deps gets a nil service. It runs before any module is built.
func (s *services) fill() {
	if s.Started.IsZero() {
		s.Started = time.Now()
	}
	if s.Library == nil {
		s.Library = &library.Service{Store: s.Store, Queue: s.Queue, Metadata: s.Metadata, Live: s.Live}
	}
	if s.Lists == nil {
		s.Lists = &requests.OutsideLists{Store: s.Store, Queue: s.Queue, Library: s.Library, Metadata: s.Metadata}
	}
	if s.Alerts == nil && s.Secrets != nil {
		s.Alerts = &alerts.Sender{Store: s.Store, Vault: s.Secrets, Lang: func(ctx context.Context) string { return accounts.OwnerLanguage(ctx, s.Store.Read()) }}
	}
	if s.Asks == nil {
		s.Asks = &requests.Service{Store: s.Store, Library: s.Library, Metadata: s.Metadata, Live: s.Live}
		if s.Alerts != nil {
			s.Asks.Alerts = s.Alerts
		}
	}
	if s.Health == nil {
		s.Health = &health.Engine{Store: s.Store, Version: s.Version}
	}
	if s.Updates == nil {
		s.Updates = &updates.Checker{Store: s.Store, Version: s.Version}
	}
	if s.Health.UpdateCheck == nil {
		WireUpdateChecks(s.Health, s.Updates)
	}
	if s.Media == nil && s.Secrets != nil {
		s.Media = &mediaservers.Linker{Store: s.Store, Vault: s.Secrets}
	}
	if s.Subtitles == nil {
		s.Subtitles = subtitles.NewService(s.Store, s.Secrets, s.Live, s.Queue)
	}
	if s.Search == nil {
		q := s.Queue
		var pub sources.Publisher
		if s.Live != nil {
			pub = s.Live // a nil hub must stay a nil Publisher
		}
		s.Search = &sources.Searcher{Store: s.Store, Live: pub, Secrets: s.Secrets, Queue: q,
			OnChosen: func(ctx context.Context, slotID, releaseID int64) {
				if q == nil {
					return
				}
				if _, _, err := q.Enqueue(ctx, downloads.KindGrab, &domain.Subject{Type: "slot", ID: slotID}, map[string]any{"release_id": releaseID},
					"grab:"+itoa(slotID), domain.System); err != nil {
					log.Printf("queue grab: %v", err)
				}
			}}
	}
}

// WireUpdateChecks hands the update check to the health checks: health
// shows its row and its state, and never imports updates.
func WireUpdateChecks(he *health.Engine, uc *updates.Checker) {
	he.UpdateState = func(ctx context.Context) health.UpdateState {
		i := uc.Update(ctx)
		return health.UpdateState{On: i.On, Feed: i.Feed, CheckedAt: i.CheckedAt, Every: uc.Every(ctx), Latest: i.Latest, Newer: i.Newer}
	}
	he.UpdateCheck = func(ctx context.Context) (string, bool, bool) {
		i, ran := uc.Scheduled(ctx)
		return i.Latest, i.Newer, ran
	}
}

// Command holyarrs runs the HolyArrs server.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"

	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/music"

	"github.com/HolyArrs/holyarrs/app/backend/internal/alerts"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// errRestart asks main to exit so the container starts HolyArrs again,
// which puts a staged restore in place.
var errRestart = errors.New("restart requested")

// exitRestart is the exit status after a staged restore and after a
// restart asked for from the controls (not zero, so a restart policy of
// on-failure starts the app again too).
const exitRestart = 75

// errAppExit ends serve because a person asked for a restart or a shut
// down from the controls; main exits with code.
type errAppExit struct{ code int }

func (e errAppExit) Error() string { return fmt.Sprintf("exit %d asked for from the controls", e.code) }

// safeNextFile in the config folder makes the next start a safe-mode start
// (restart in safe mode); it is removed as that start reads it.
const safeNextFile = "safe-mode-next"

// runOpts are test hooks for serve.
type runOpts struct {
	// wrap changes each supervised loop before it starts (tests inject crashes).
	wrap func(jobs.Loop) jobs.Loop
	// reg replaces the Registry (tests set short restart delays).
	reg *jobs.Registry
	// ready is told the Registry and the address once the server listens.
	ready func(reg *jobs.Registry, addr string)
	// kinds is told every Job kind with a Handler.
	kinds func([]string)
	// parts is told the parts Control once it is made.
	parts func(*parts.Control)
	// store is told the main store once it is open (tests emit Events).
	store func(*database.Store)
}

func run(c config) error {
	startup.UseMemoryMode()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, c, runOpts{})
}

// serve runs HolyArrs until ctx ends or a restart is asked for.
func serve(ctx context.Context, c config, o runOpts) error {
	registerHooks()
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	reg := o.reg
	if reg == nil {
		reg = jobs.NewRegistry()
	}
	jobs.SetDefault(reg)
	database.SetEventGuard(jobs.Guard)
	const notStarted = "Safe mode: not started. Restart without safe mode to turn it on."
	if err := os.MkdirAll(c.ConfigDir, 0o750); err != nil {
		return err
	}
	// The app log for the log viewer: every line also goes to a capped
	// file in the config folder, secrets masked; the program output (docker
	// logs) is masked the same way.
	applogs, err := logs.Open(filepath.Join(c.ConfigDir, "logs"))
	if err != nil {
		if nw := database.NotWritable(c.ConfigDir, err); nw != nil {
			return refuseEarly(ctx, c, nw)
		}
		return fmt.Errorf("log folder: %w", err)
	}
	defer applogs.Close()
	prevOut := log.Writer()
	log.SetOutput(io.MultiWriter(applogs.Mask(prevOut), applogs))
	defer log.SetOutput(prevOut)
	if err := os.Remove(filepath.Join(c.ConfigDir, safeNextFile)); err == nil {
		log.Printf("holyarrs: this start is in safe mode, as asked for with restart in safe mode; the next start is normal")
		c.SafeMode = true
	}
	// Every background loop is added to the parts Control, which starts it
	// under the supervisor once its part is on (and not in safe mode when
	// optional), and stops and starts it when a part or the stage is
	// switched.
	pc := &parts.Control{Reg: reg, SafeMode: c.SafeMode}
	loop := func(l jobs.Loop) {
		if o.wrap != nil {
			l = o.wrap(l)
		}
		pc.Add(l, false)
	}
	optional := func(l jobs.Loop) {
		if o.wrap != nil {
			l = o.wrap(l)
		}
		pc.Add(l, true)
	}
	if c.SafeMode {
		log.Printf("holyarrs: SAFE MODE: only the core runs (sign-in, the Library, settings, backups, health); schedules, searches, downloads, imports, subtitles and Requests check are off")
	}
	// The lock tells tools that change a stopped config folder (holyarrs-demo)
	// that HolyArrs runs here; the system drops it when the process ends.
	unlock, err := files.LockDir(c.ConfigDir)
	if err != nil {
		if nw := database.NotWritable(c.ConfigDir, err); nw != nil {
			return refuseEarly(ctx, c, nw)
		}
		return fmt.Errorf("config folder %s: %w", c.ConfigDir, err)
	}
	defer unlock()
	// Listen first, so a long update of the data still answers healthz
	// (alive) and shows a short page; the real API takes over once ready.
	swap := &swapHandler{}
	var updating atomic.Bool
	swap.set(webserver.UnderURLBase(c.URLBase, startingHandler(&updating)))
	srv := newServer(c.Addr, swap)
	ln, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return err
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	defer srv.Close()
	go func() {
		<-ctx.Done()
		sh, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sh)
	}()
	// A backup or restore upload cut off by a stop or kill left copies of
	// the databases behind; none is valid after a restart.
	if n, err := backups.CleanLeftovers(c.ConfigDir); err != nil {
		log.Printf("backup: could not remove all of an unfinished backup: %v", err)
	} else if n > 0 {
		log.Printf("backup: removed what an unfinished backup or restore left behind, %d MB freed", (n+1<<20-1)>>20)
	}
	if _, err := backups.ApplyStaged(c.ConfigDir); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	s, err := database.Open(c.ConfigDir, database.Options{OnUpdating: func() { updating.Store(true) }, CacheKiB: startup.LowMemoryPick(0, 4000), ReaderCacheKiB: startup.LowMemoryPick(0, 2000)})
	if err != nil {
		if !needsPerson(err) {
			return err
		}
		// Let holyarrs restore work while this page shows; nothing here
		// touches the data any more.
		unlock()
		// Exiting would only restart into the same refusal: keep saying
		// what is wrong until a person stops the app.
		log.Printf("holyarrs: not started: %v", err)
		// Older than the data: the page may offer to put back the data
		// from before the update; once it has, stop so the app starts on it.
		putBackDone := make(chan struct{})
		swap.set(webserver.UnderURLBase(c.URLBase, refusedHandler(c.ConfigDir, err, putBackDone)))
		select {
		case <-ctx.Done():
			return err
		case <-putBackDone:
			return errRestart
		}
	}
	defer s.Close()
	pc.Store = s
	unmapped := health.UnmappedConfig(c.ConfigDir)
	if w := unmappedConfigWarning(c.ConfigDir, unmapped); w != "" {
		log.Printf("holyarrs: %s", w)
	}
	if err := pc.Load(ctx); err != nil {
		return fmt.Errorf("part switches: %w", err)
	}
	// A base saved in Settings is used when the start setting is left
	// unset; HOLYARRS_URL_BASE=/ forces none.
	if _, set := os.LookupEnv(webserver.URLBaseEnv); !set && !c.urlBaseFlag && c.URLBase == "" {
		if saved := webserver.SavedURLBase(ctx, s.Read()); saved != "" {
			c.URLBase = saved
			log.Printf("holyarrs: URL base %s (saved in Settings)", saved)
		}
	}
	if off := pc.Off(); len(off) > 0 && !c.SafeMode {
		log.Printf("holyarrs: parts turned off: %v", off)
	}
	if o.parts != nil {
		o.parts(pc)
	}
	if o.store != nil {
		o.store(s)
	}
	// The loops end before the store closes; one stuck in code that ignores
	// its context is left behind after a while so the app still stops.
	defer func() {
		stop()
		done := make(chan struct{})
		go func() { reg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			log.Printf("supervisor: a background loop did not stop in time")
		}
	}()
	// A master key that does not match needs a person, like the refusals
	// above: keep a plain page up instead of exiting into a restart loop.
	hold := func(err error) error {
		log.Printf("holyarrs: not started: %v", err)
		swap.set(webserver.UnderURLBase(c.URLBase, failedHandler(publicReason(err))))
		<-ctx.Done()
		return err
	}
	keyRefused := func(err error) error {
		if !keyNeedsPerson(err) {
			return err
		}
		return hold(err)
	}
	named := os.Getenv(secrets.EnvKeyFile)
	key, err := secrets.LoadKey(c.ConfigDir, named)
	if err != nil {
		// Other errors (the database check, making a new key) may pass:
		// exit as before, so the restart policy tries again.
		if !keyNeedsPerson(err) {
			return err
		}
		// Hold the page until the key file can be read (a late share
		// mount, an owner fixed on the host), then exit so the restart
		// policy starts the app with it.
		log.Printf("holyarrs: not started: %v", err)
		swap.set(webserver.UnderURLBase(c.URLBase, failedHandler(publicReason(err))))
		p := named
		if p == "" {
			p = filepath.Join(c.ConfigDir, secrets.KeyFile)
		}
		t := time.NewTicker(keyRecheck)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return err
			case <-t.C:
				if _, rerr := secrets.LoadKey("", p); rerr == nil {
					log.Printf("holyarrs: the master key file %s can be read now; exiting so the app starts again with it", p)
					return err
				}
			}
		}
	}
	vault, err := secrets.Open(ctx, s, key)
	if err != nil {
		return keyRefused(err)
	}
	if n, err := vault.MigratePlaintext(ctx); err != nil {
		return fmt.Errorf("encrypting stored secrets: %w", err)
	} else if n > 0 {
		log.Printf("secrets: encrypted %d secrets that were stored in the clear", n)
	}
	// Not fatal: a Source left as it was still works.
	if n, err := sources.HidePathKeys(ctx, s, vault); err != nil {
		log.Printf("secrets: moving rss passkeys into the vault: %v", err)
	} else if n > 0 {
		log.Printf("secrets: moved the passkeys of %d rss Sources into the vault", n)
	}
	// Every value the vault stores is masked before the save returns; the
	// minute loop is a safety net that also masks, in the files, values
	// changed or removed by any code path before the vault forgets them.
	vault.OnPut = applogs.AddSecret
	refreshLogSecrets := func(ctx context.Context) error { return applogs.Sync(ctx, vault) }
	if err := refreshLogSecrets(ctx); err != nil {
		return fmt.Errorf("log masking: %w", err)
	}
	loop(jobs.Loop{Name: "log_secrets", Label: "Log masking", Every: time.Minute, Run: refreshLogSecrets})
	bm := &backups.Manager{Store: s, Dir: c.ConfigDir, Version: version, CheckKey: func(p string) error { return secrets.CheckFile(p, key) },
		Key: key, KeyFromEnv: os.Getenv(secrets.EnvKeyFile) != "", Stop: ctx}
	if _, err := bm.AdoptMigrationCopies(); err != nil {
		log.Printf("backup: %v", err)
	}
	if err := bm.AfterStart(ctx); err != nil {
		return err
	}
	loop(jobs.Loop{Name: "backups", Label: "Backups", Every: 10 * time.Minute, Deadline: 2 * time.Hour, Run: bm.Pass})
	if err := ensure(ctx, s, c); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	// After ensure, so folders added from the environment on a first start
	// are checked too.
	logFolderCheck(ctx, s.Read(), c.SavePath, c.ConfigDir)
	hub := webserver.NewLiveHub(webserver.LiveOptions{Merge: downloads.MergePatch})
	pools := jobs.DefaultPools
	if c.SafeMode {
		pools = map[jobs.Pool]int{jobs.PoolCore: jobs.DefaultPools[jobs.PoolCore]}
	}
	q := jobs.New(s, jobs.Options{Pools: pools, OnChange: func(j domain.Job) {
		hub.Publish("job", map[string]any{"id": j.ID, "kind": j.Kind, "state": j.State, "progress": j.Progress})
	}})
	// Jobs of a part that is off wait in the queue.
	q.SetGate(pc)
	pc.Jobs = q
	enqueue := func(ctx context.Context, kind string, subj domain.Subject, params map[string]any, key string) {
		if _, _, err := q.Enqueue(ctx, kind, &subj, params, key, domain.System); err != nil {
			log.Printf("queue %s: %v", kind, err)
		}
	}
	// Your own traits go to the judge before anything is judged.
	if err := library.LoadOwnTraits(ctx, s); err != nil {
		return fmt.Errorf("own traits: %w", err)
	}
	dm := &downloads.Manager{Store: s, Live: hub, SavePath: c.SavePath,
		Link: func(ctx context.Context, rel domain.Release) (string, error) {
			return sources.GrabLink(ctx, s, vault, rel)
		},
		SourceTransport: func(ctx context.Context, src domain.Source) (http.RoundTripper, error) {
			cfg, err := sources.ConfigFor(ctx, src, vault)
			if err != nil {
				return nil, err
			}
			return sources.Transport(cfg), nil
		},
		FromVault: vault.Get}
	// Background download reader: one reader per Downloader inside the
	// "downloads" loop; the Manager and the Downloads view read its snapshots.
	dls := &downloads.Sync{Store: &downloads.Store{}, DB: s, Manager: dm, Lines: downloads.TorrentLines}
	dm.Sync = dls
	im := &importer.Importer{Store: s, Live: hub}
	sr := &sources.Searcher{Store: s, Live: hub, Secrets: vault, Queue: q, OnChosen: func(ctx context.Context, slotID, releaseID int64) {
		enqueue(ctx, downloads.KindGrab, domain.Subject{Type: "slot", ID: slotID}, map[string]any{"release_id": releaseID}, fmt.Sprintf("grab:%d", slotID))
	}}
	// A rejected Import turned its Release down: remove the Download from
	// its Downloader and search that Slot again.
	im.OnRejected = func(ctx context.Context, slotID int64) {
		var dlID sql.NullInt64
		if s.Read().QueryRowContext(ctx, "SELECT active_download_id FROM slots WHERE id = ?", slotID).Scan(&dlID) == nil && dlID.Valid {
			if err := dm.Discard(ctx, dlID.Int64); err != nil {
				log.Printf("remove rejected download %d: %v", dlID.Int64, err)
			}
		}
		enqueue(ctx, jobs.KindSearchSlot, domain.Subject{Type: "slot", ID: slotID}, nil, fmt.Sprintf("search_slot:%d", slotID))
	}
	// A finished Import: clear a Usenet download out of its Downloader.
	im.OnImported = dm.ClearFinished
	// Finished Usenet files move into the Library unless HolyArrs may not
	// change that Downloader.
	im.MayMove = func(d domain.Downloader) bool { return !downloads.ReadOnlyFor(d) }
	// A failed Download: search its Slot for the next Release (a failed
	// season pack: one season search for its episodes).
	dm.OnFailed = func(ctx context.Context, slotID int64) {
		if err := sr.QueueAfterFailure(ctx, slotID, domain.System); err != nil {
			log.Printf("search after a failed download: %v", err)
		}
	}
	dm.OnFinished = func(ctx context.Context, downloadID int64) {
		enqueue(ctx, importer.KindImport, domain.Subject{Type: "download", ID: downloadID}, nil, fmt.Sprintf("import:%d", downloadID))
	}
	sr.Register(q)
	q.ProbeSchedule(jobs.KindFeedCheck, sr.ProbeFeed)
	q.ProbeSchedule(jobs.KindGrabHeld, sr.ProbeGrabHeld)
	q.Handle(downloads.KindGrab, dm.HandleGrab)
	// An ask turns Ready right after its import, not on the next 30 s ready
	// check: the Requests card would drop from Importing back to Processing.
	var readyCheck atomic.Pointer[requests.Service]
	q.Handle(importer.KindImport, func(ctx context.Context, r *jobs.Run) error {
		err := im.HandleJob(ctx, r)
		if w := readyCheck.Load(); err == nil && w != nil {
			if _, err := w.CheckReady(ctx); err != nil {
				log.Printf("requests: ready check after an import: %v", err)
			}
		}
		return err
	})
	hosted, err := metadata.NewHosted(vault)
	if err != nil {
		return fmt.Errorf("metadata server: %w", err)
	}
	// A stand-in YouTube and a quicker trailer pass (browser tests only).
	if err := metadata.TrailerStandIns(); err != nil {
		return err
	}
	hosted.Disk = metadata.LookupCache{Store: s}
	// "Count this install" is set before the Job queue starts: a Job's
	// request must not go out counted after the owner switched it off.
	hosted.Gate.SetCountryCount(health.CountInstall(ctx, s.Read()))
	var md metadata.Provider = hosted.TMDB()
	showMD := showSources(vault, hosted)
	var artistMD metadata.ArtistProvider = musicSource(s, version)
	// Music lookups go through the metadata server only: an install never
	// asks MusicBrainz itself. A MusicBrainz stand-in (tests) is asked
	// directly; HOLYARRS_DIRECT_SOURCES=1 (developers only) keeps the real
	// MusicBrainz behind the server for its fallbacks.
	if os.Getenv("HOLYARRS_MUSICBRAINZ_URL") == "" {
		var direct metadata.ArtistProvider
		if directSources() {
			direct = artistMD
		}
		artistMD = hosted.Service.Artists(direct)
	}
	pictures := artistImages()
	// Pictures come from public addresses only; a picture stand-in named
	// by HOLYARRS_DEEZER_URL (tests) may be on this machine.
	artworkHTTP := library.ArtworkClient()
	if env := os.Getenv("HOLYARRS_DEEZER_URL"); env != "" && env != "off" {
		artworkHTTP = &http.Client{Timeout: 20 * time.Second, Transport: http.DefaultTransport}
	}
	// The metadata server's picture addresses ride its pooled connection.
	if u, err := url.Parse(hosted.Service.Base()); err == nil {
		artworkHTTP.Transport = metadata.HostRoute{Host: u.Host, Via: hosted.Transport, Rest: artworkHTTP.Transport}
	}
	if pictures != nil {
		pictures.UserAgent = metadata.UserAgentFor(version)
	}
	sources.UserAgent = "HolyArrs/" + version
	library.UseSubtitleNames(subtitles.Names{})
	lib := &library.Service{Store: s, Queue: q, Metadata: md, Live: hub, SavePath: c.SavePath, ArtistImages: pictures,
		ImageServer: hosted.Service.Base() + "/v1/img",
		Artwork: &library.ArtworkCache{Dir: filepath.Join(c.ConfigDir, "artwork"), Allowed: []string{metadata.ImageBase + "/", hosted.Service.Base() + "/v1/img/", metadata.OldPictures(hosted.Service.Base())},
			HTTP: artworkHTTP, Fallback: library.TMDBPassThrough(hosted.Service.Base())}}
	// TMDB copies are kept at most six months (TMDB's rule).
	go lib.Artwork.KeepTMDBFresh(ctx)
	lib.Shows = showMD
	lib.AnimeNames = func(ctx context.Context, kind, tmdbID, name string) []string {
		by := "tmdb_tv"
		if kind == "movie" {
			by = "tmdb_movie"
		}
		m, err := hosted.Service.AnimeMap(ctx, by, tmdbID)
		if err != nil {
			return nil
		}
		return m.AltNames(name)
	}
	lib.Artists = artistMD
	lib.Register(q)
	// Once per install: grey stand-ins stored before they were refused are
	// removed, so the picture run below fetches the real ones.
	if n, err := lib.HealServerPlaceholders(ctx); err != nil || n > 0 {
		log.Printf("artwork: %d placeholder pictures removed, error %v", n, err)
	}
	// Once per install, in the background: files moved over from the old apps
	// get their quality read again, so good copies stop asking for a better one.
	go func() {
		if _, err := lib.RejudgeAdopted(ctx); err != nil {
			log.Printf("library: reading moved-over quality again: %v", err)
		}
	}()
	// Artists still without a picture get one soon after start, apart from
	// the slower MusicBrainz refresh.
	if _, _, err := q.Enqueue(ctx, library.KindArtistPictures, nil, map[string]any{"startup": true}, library.KindArtistPictures, domain.System); err != nil {
		log.Printf("queue %s: %v", library.KindArtistPictures, err)
	}
	// Moves from the old apps: each part that is off is left out, and safe
	// mode refuses moves at the API.
	mover := &migrate.Mover{Store: s, Library: lib, Queue: q, Secrets: vault, PartOn: func(p string) bool { return !c.SafeMode && pc.On(parts.Part(p)) }}
	mover.Register(q)
	// A move or undo whose Job a crash left behind is failed, so it never
	// blocks the next one; one whose Job is still there resumes.
	if err := mover.Recover(ctx); err != nil {
		log.Printf("moves cut off by a stop: %v", err)
	}
	if c.SafeMode {
		log.Printf("safe mode: renames cut off by a stop are finished on the next normal start")
	} else if n, err := lib.RecoverRenames(ctx, domain.System); err != nil || n > 0 {
		log.Printf("renames cut off by a stop: %d finished, error %v", n, err)
	}
	ol := &requests.OutsideLists{Store: s, Queue: q, Library: lib, Metadata: md, TMDBBase: hosted.TMDBBase,
		TraktBase: os.Getenv("HOLYARRS_TRAKT_URL"), TMDBToken: hosted.TMDBAccess,
		TraktClient: func(ctx context.Context) (string, error) { return vault.Setting(ctx, "links", "trakt_client_id") }}
	ol.Register(q)
	q.ProbeSchedule(requests.OutsideListSync, ol.ProbeSync)
	he := &health.Engine{Store: s, Version: version, ConfigUnmapped: unmapped, ConfigDir: c.ConfigDir, Publish: func(topic string, data any) { hub.Publish(topic, data) },
		Backups: backupsHealth(bm)}
	// The update check: once a day from the release feed, through the
	// network policy (public only unless the setting allows the local
	// network; never cloud metadata addresses). HOLYARRS_RELEASE_FEED
	// points it elsewhere (tests).
	uc := &updates.Checker{Store: s, Version: version}
	uc.ReleaseFeed = env("HOLYARRS_RELEASE_FEED", updates.DefaultReleaseFeed)
	uc.ClientFor = func(allowLocal bool) *http.Client {
		allow := outbound.PublicOnly
		if allowLocal {
			allow = outbound.NotMetadata
		}
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil
		t.DialContext = outbound.PolicyDialer(allow).DialContext
		return &http.Client{Transport: t, Timeout: 15 * time.Second}
	}
	WireUpdateChecks(he, uc)
	he.Register(q)
	// History clean-up: once a day, old Events past their keep time.
	q.Handle(database.KindCleanHistory, func(ctx context.Context, r *jobs.Run) error {
		n, err := s.PruneEvents(ctx, database.ReadHistoryKeep(ctx, s.Read()), time.Now().UTC())
		r.SetResult(map[string]any{"removed": n})
		return err
	})
	q.ProbeSchedule(health.KindCheck, he.Probe)
	// Connected Jackett: its Sources follow its sites; no bridge, no run.
	bridges := &sources.BridgeService{Store: s, Vault: vault}
	q.Handle(sources.BridgeKindCheck, bridges.HandleCheck)
	q.ProbeSchedule(sources.BridgeKindCheck, bridges.Probe)
	// Plex watchlists (Requests): registered here with the other kinds; the
	// reader is set once Requests and sign-in are up, and a pass before
	// that finds nothing to do.
	var watchlists atomic.Pointer[requests.Watchlists]
	q.Handle(requests.KindPlexWatchlists, func(ctx context.Context, r *jobs.Run) error {
		if w := watchlists.Load(); w != nil {
			return w.Run(ctx, r)
		}
		r.SetResult(map[string]any{"skipped": "still starting"})
		return nil
	})
	q.ProbeSchedule(requests.KindPlexWatchlists, func(ctx context.Context, now time.Time) (bool, any, error) {
		if w := watchlists.Load(); w != nil {
			return w.Probe(ctx, now)
		}
		return false, nil, nil
	})
	// Subtitles: searches after imports and better copies, two schedules.
	subs := &subtitles.Service{Store: s, Live: hub, Vault: vault}
	subs.Register(q)
	if !c.SafeMode {
		s.OnEvent(func(e domain.Event) {
			if pc.On(parts.Subtitles) && startup.FeatureOn(startup.SubtitleSites) {
				subs.OnEvent(e)
			}
		})
	}
	im.OnLowSpace = he.LowSpace
	// Slots a crash left searching go back to looking before any Job runs
	// (in safe mode nothing searches; the next normal start does it).
	if !c.SafeMode {
		if n, err := sr.SweepSearching(ctx, 30*time.Minute); err != nil || n > 0 {
			log.Printf("slots left searching: %d put back, error %v", n, err)
		}
	}
	if o.kinds != nil {
		o.kinds(q.Kinds())
	}
	if err := q.Start(ctx); err != nil {
		return err
	}
	defer q.Stop()
	for p, label := range map[jobs.Pool]string{jobs.PoolCore: "Core workers", jobs.PoolIO: "Search and download workers", jobs.PoolHeavy: "Import and scan workers", jobs.PoolMigrate: "Import from other apps worker"} {
		if pools[p] == 0 {
			reg.Set(jobs.StageKey("jobs_"+string(p)), "stage", label, jobs.Off, notStarted)
			continue
		}
		reg.Set(jobs.StageKey("jobs_"+string(p)), "stage", label, jobs.Running, "")
	}
	optional(jobs.Loop{Name: "downloads", Label: "Download sync", Run: func(ctx context.Context) error {
		// The reader lives and dies with this loop: it stops after dm.Run
		// returns (or panics), then flushes with its own deadline.
		dls.Around(ctx, dm.Run)
		return nil
	}})
	optional(jobs.Loop{Name: "schedules", Label: "Schedules", Every: time.Minute, Run: func(ctx context.Context) error {
		_, err := q.RunSchedules(ctx, time.Now().UTC())
		return err
	}})
	optional(jobs.Loop{Name: "stranded_imports", Label: "Stranded imports", Every: 10 * time.Minute, Run: func(ctx context.Context) error {
		n, err := im.RequeueStranded(ctx, func(ctx context.Context, id int64) {
			enqueue(ctx, importer.KindImport, domain.Subject{Type: "download", ID: id}, nil, fmt.Sprintf("import:%d", id))
		})
		if n > 0 {
			log.Printf("stranded show downloads: queued %d imports again", n)
		}
		return err
	}})
	optional(jobs.Loop{Name: "gates", Label: "Release gates", Every: time.Minute, Run: lib.GatesPass})

	sso, err := accounts.SSOFromEnv(c.URLBase)
	if err != nil {
		return err
	}
	au := accounts.New(s, accounts.Options{Vault: vault, SafeMode: c.SafeMode, SSO: sso, ConfigUnmapped: unmapped != ""})
	defer au.Close()
	loop(jobs.Loop{Name: "sessions", Label: "Session clean-up", Every: time.Hour, Run: au.Cleanup})
	var restarting atomic.Bool
	restart := func() {
		time.Sleep(500 * time.Millisecond) // let the answer reach the page
		restarting.Store(true)
		stop()
	}
	// exitApp is the controls' whole-app restart, restart in safe mode and
	// shut down: the server stops cleanly and main exits, so Docker's
	// restart policy starts the app again (unless the policy is "no").
	var exitCode atomic.Int32
	exitCode.Store(-1)
	exitApp := func(how string) error {
		code := int32(exitRestart)
		switch how {
		case "safe_mode":
			if err := os.WriteFile(filepath.Join(c.ConfigDir, safeNextFile), []byte("restart in safe mode asked for from the controls\n"), 0o600); err != nil {
				log.Printf("holyarrs: could not mark the next start as safe mode, so nothing restarts: %v", err)
				return err
			}
		case "shutdown":
			code = 0
		}
		go func() {
			time.Sleep(500 * time.Millisecond) // let the answer reach the page
			exitCode.Store(code)
			stop()
		}()
		return nil
	}
	sender := &alerts.Sender{Store: s, Vault: vault, Lang: func(ctx context.Context) string { return accounts.OwnerLanguage(ctx, s.Read()) }}
	sender.Watch(ctx)
	media := &mediaservers.Linker{Store: s, Vault: vault}
	reg.AlsoWait(func() { media.Stop(); media.Wait() })
	reg.AlsoWait(he.Wait)
	if !c.SafeMode {
		media.Watch(ctx)
	}
	defer sender.Wait()
	requestsSvc := &requests.Service{Store: s, Library: lib, Metadata: md, Alerts: sender, Live: hub}
	readyCheck.Store(requestsSvc)
	optional(jobs.Loop{Name: "wishlist", Label: "Requests ready check", Every: 30 * time.Second, Run: func(ctx context.Context) error {
		if _, err := requestsSvc.CheckReady(ctx); err != nil {
			return err
		}
		if _, err := requestsSvc.SendEpisodeNotices(ctx); err != nil {
			return err
		}
		_, err := requestsSvc.SendAlbumNotices(ctx)
		return err
	}})
	requestsSvc.PrefetchParts = true
	// Plex watchlists are read with each person's own kept Plex sign-in.
	plexWatch := &requests.Watchlists{Asks: requestsSvc, Base: env("HOLYARRS_PLEX_DISCOVER_URL", requests.DefaultPlexDiscover),
		Token: au.PlexToken, ClientID: au.PlexClientID}
	watchlists.Store(plexWatch)
	go requestsSvc.BackfillParts(context.Background())
	requestsSvc.WatchEpisodes()
	requestsSvc.WatchTMDBFollows()
	reg.Set(jobs.StageKey("alerts"), "stage", "Alerts", jobs.Running, "")
	if c.SafeMode {
		reg.Set(jobs.StageKey("media_servers"), "stage", "Media-server refresh", jobs.Off, notStarted)
	} else {
		reg.Set(jobs.StageKey("media_servers"), "stage", "Media-server refresh", jobs.Running, "")
	}
	reg.Set(jobs.StageKey("events"), "stage", "Event hooks", jobs.Running, "")
	defer requestsSvc.Wait()
	// Deezer marks and artist bios come from the metadata server; only the
	// direct stand-in (a Deezer stand-in in tests, or
	// HOLYARRS_DIRECT_SOURCES=1) is asked directly.
	serverDeezer, serverDeezerAlbum, serverBio := hosted.Service.MusicDeezerArtist, hosted.Service.MusicDeezerAlbum, hosted.Service.MusicArtistBio
	if pictures != nil {
		serverDeezer, serverDeezerAlbum, serverBio = nil, nil, nil
	}
	// Artist packages are copied to disk on a schedule (no credential sent);
	// the hosted music source answers from them first.
	pkgs := &music.PackageSync{Store: s, Source: hosted.Service}
	hosted.Service.SetLocalMusic(pkgs)
	a := buildHandlers(services{Bridges: bridges, Packages: pkgs, Library: lib, Lists: ol, Store: s, Queue: q, Live: hub, Metadata: md, Version: version, Auth: au, Backups: bm, Restart: restart,
		Secrets: vault, TraktBase: os.Getenv("HOLYARRS_TRAKT_URL"), MetadataUp: hosted.Health.Up, ServiceTop: hosted.Service.ServiceTop, TrailerPick: func(ctx context.Context, kind, id, lang string) (string, error) {
			t, err := hosted.Service.TrailerPick(ctx, kind, id, lang)
			return t.Key, err
		}, AnimeMap: hosted.Service.AnimeMap, ExtraArt: hosted.Service.Art, ImageBase: hosted.Service.Base() + "/v1/img", MusicControl: hosted.Service.Control, ArtistVideos: hosted.Service.ArtistVideos, ExtraRatings: hosted.Service.Ratings, CountryCount: hosted.Gate.SetCountryCount, WikiTitle: hosted.Service.WikiTitle, WikiPerson: hosted.Service.WikiPerson, AwardWinners: hosted.Service.AwardWinners, Alerts: sender, Asks: requestsSvc, Watchlists: plexWatch, Search: sr, Downloads: dm, DLSync: dls, Health: he, Updates: uc, Media: media, Subtitles: subs, States: reg, SafeMode: c.SafeMode, Parts: pc, ExitApp: exitApp, Mover: mover,
		Log: applogs, ConfigDir: c.ConfigDir, SavePath: c.SavePath, URLBase: c.URLBase, Deps: musicDeps(hosted.Service, serverDeezer, serverDeezerAlbum, serverBio)})
	// "Count this install" rides on every request to the metadata server.
	a.ApplyCountryCount(ctx)
	// Matching in the reader's queue is link-only: it never adds a title.
	dls.Match = a.DownloadsWeb().MatchLinkOnly
	// Push updates of the snapshots to the Downloads page (only while on).
	dls.Pub = &downloads.SnapshotPublisher{Store: dls.Store, Build: a.DownloadsWeb().PublishDL, Health: a.DownloadsWeb().PublishDLHealth}
	if !c.SafeMode {
		a.StartExplore(ctx)
	}
	for _, l := range a.MusicLoops() {
		optional(l)
	}
	for _, l := range a.Trailers().Loops() {
		optional(l)
	}
	startTrailers(a.Trailers(), he)
	// Health starts once every check is wired (the trailers check comes
	// from the API), so its first run sees them all.
	reg.Set(jobs.StageKey("health"), "stage", "Health checks", jobs.Running, "")
	reg.GoGuard("health", func() {
		if _, err := he.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("health: %v", err)
		}
	})
	pc.Start(ctx)
	swap.set(routes(a, c))
	log.Printf("holyarrs %s: config %s, listening on %s%s", version, c.ConfigDir, ln.Addr(), c.URLBase)
	if o.ready != nil {
		o.ready(reg, ln.Addr().String())
	}
	if err := <-served; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if restarting.Load() {
		return errRestart
	}
	if code := exitCode.Load(); code >= 0 {
		return errAppExit{int(code)}
	}
	return nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		os.Exit(healthcheckCmd())
	}
	// FILE__, UMASK and PUID/PGID come first, for the server and for every
	// command that opens the database, so a root docker exec never leaves
	// root-owned files. Started as root on Linux this restarts as PUID:PGID.
	if err := startup.StartRunAs(configDirArg(os.Args[1:])); err != nil {
		log.Printf("holyarrs: %v", err)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "reset-owner" {
		os.Exit(resetOwner(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == "restore" {
		os.Exit(restoreCmd(os.Args[2:], os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && os.Args[1] == "import-secrets" {
		os.Exit(importSecrets(os.Args[2:], os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && os.Args[1] == "doctor" {
		os.Exit(doctorCmd(os.Args[2:], os.Stdout, os.Stderr))
	}
	c, err := parseFlags(os.Args[1:], os.Stderr)
	if err != nil {
		os.Exit(2)
	}
	var ex errAppExit
	if err := run(c); errors.Is(err, errRestart) {
		log.Printf("holyarrs: stopping so the restore is put in place on the next start")
		os.Exit(exitRestart)
	} else if errors.As(err, &ex) {
		log.Printf("holyarrs: stopped as asked for from the controls (exit %d)", ex.code)
		os.Exit(ex.code)
	} else if err != nil {
		log.Fatalf("holyarrs: %v", err)
	}
}

// Command holyarrs runs the HolyArrs server.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"github.com/CoderLuii/holyarrs/internal/lowmem"
	"github.com/CoderLuii/holyarrs/internal/music"
	"github.com/CoderLuii/holyarrs/internal/musicsync"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/CoderLuii/holyarrs/internal/alerts"
	"github.com/CoderLuii/holyarrs/internal/api"
	"github.com/CoderLuii/holyarrs/internal/applog"
	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/backup"
	"github.com/CoderLuii/holyarrs/internal/compress"
	"github.com/CoderLuii/holyarrs/internal/dlsync"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/health"
	"github.com/CoderLuii/holyarrs/internal/importer"
	"github.com/CoderLuii/holyarrs/internal/jobs"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/lists"
	"github.com/CoderLuii/holyarrs/internal/live"
	"github.com/CoderLuii/holyarrs/internal/mediaserver"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/migrate"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/subtitles"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
	"github.com/CoderLuii/holyarrs/internal/urlbase"
)

var version = "0.0.0-skeleton"

type config struct {
	ConfigDir string
	Addr      string
	// Skeleton setup: one Source, one Downloader, one movie Library folder.
	SourceURL string
	QbitURL   string
	Movies    string
	SavePath  string
	// SafeMode starts only the core (sign-in, the Library, settings,
	// backups, health): no schedules, searches, downloads or imports.
	SafeMode bool
	// URLBase runs the app under a path such as /holyarrs (reverse proxy).
	URLBase string
	// urlBaseFlag is true when -url-base was given on the command line.
	urlBaseFlag bool
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnvSafeMode set to 1 (or true) starts HolyArrs in safe mode.
const EnvSafeMode = "HOLYARRS_SAFE_MODE"

func envOn(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func parseFlags(args []string, errOut io.Writer) (config, error) {
	fs := flag.NewFlagSet("holyarrs", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var c config
	fs.StringVar(&c.ConfigDir, "config", "/config", "folder for the database and settings (must be on local disk)")
	fs.StringVar(&c.Addr, "addr", ":18730", "address to listen on")
	fs.StringVar(&c.SourceURL, "source-url", env("HOLYARRS_SOURCE_URL", ""), "Torznab Source to add on start (no key)")
	fs.StringVar(&c.QbitURL, "qbittorrent-url", env("HOLYARRS_QBITTORRENT_URL", ""), "qBittorrent Web UI to add on start")
	fs.StringVar(&c.Movies, "movies", env("HOLYARRS_MOVIES", ""), "movie Library folder to add on start")
	fs.StringVar(&c.SavePath, "save-path", env("HOLYARRS_SAVE_PATH", ""), "where the Downloader saves (same path in both containers)")
	fs.BoolVar(&c.SafeMode, "safe-mode", envOn(EnvSafeMode), "start only the core: sign-in, the Library, settings, backups and health ("+EnvSafeMode+"=1)")
	fs.StringVar(&c.URLBase, "url-base", env(urlbase.Env, ""), "serve under this path behind a reverse proxy, such as /holyarrs ("+urlbase.Env+")")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	fs.Visit(func(f *flag.Flag) { c.urlBaseFlag = c.urlBaseFlag || f.Name == "url-base" })
	base, err := urlbase.Clean(c.URLBase)
	if err != nil {
		fmt.Fprintln(errOut, "-url-base:", err)
		return c, err
	}
	c.URLBase = base
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return c, nil
}

// ensure adds the configured Source, Downloader and Library folder once.
func ensure(ctx context.Context, s *store.Store, c config) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		exists := func(q string, arg any) (bool, error) {
			var n int
			err := tx.QueryRow(q, arg).Scan(&n)
			return n > 0, err
		}
		if c.Movies != "" && !library.FullPath(c.Movies) {
			// Git Bash turns /data/media/movies into "C:/Program Files/Git/data/media/movies"
			// when a container is made without MSYS_NO_PATHCONV=1: a folder that can never be reached.
			log.Printf("setup: movie Library folder %q is not a full path here; not added", c.Movies)
		} else if c.Movies != "" {
			ok, err := exists("SELECT count(*) FROM library_folders WHERE path = ?", c.Movies)
			if err != nil {
				return err
			}
			if !ok {
				if _, err := store.Insert(tx, "library_folders", &domain.LibraryFolder{Path: c.Movies, Kind: "movie", Label: "Movies",
					Health: "unknown", SameDiskAs: []int64{}}, domain.System); err != nil {
					return err
				}
			}
		}
		if c.SourceURL != "" {
			ok, err := exists("SELECT count(*) FROM sources WHERE url = ?", c.SourceURL)
			if err != nil {
				return err
			}
			if !ok {
				if _, err := store.Insert(tx, "sources", &domain.Source{Name: "Lab fake indexer", Protocol: "torrent", Format: "torznab",
					URL: c.SourceURL, UseFor: domain.SourceUse{Feed: false, AutoSearch: true, HandSearch: true}, Kinds: []string{"movie"},
					Categories: map[string][]int{"movie": {2000}}, Limits: domain.SourceLimits{GapMS: 2000, SearchesPerWindow: 100, WindowSeconds: 3600},
					Priority: 25, State: "enabled", Extra: map[string]string{}, AllowLocal: true}, domain.System); err != nil {
					return err
				}
			}
			// The Source named on the command line is the person's own (the lab's
			// fake indexer on a private network), so it may reach the local network.
			if _, err := tx.Exec("UPDATE sources SET allow_local_network = 1 WHERE url = ?", c.SourceURL); err != nil {
				return err
			}
		}
		if c.QbitURL != "" {
			ok, err := exists("SELECT count(*) FROM downloaders WHERE url = ?", c.QbitURL)
			if err != nil {
				return err
			}
			if !ok {
				if _, err := store.Insert(tx, "downloaders", &domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent",
					URL: c.QbitURL, Category: "holyarrs", Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}, domain.System); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// maxBody is the largest API request body accepted.
const maxBody = 1 << 20

// limitBodies refuses API request bodies over maxBody with 413.
func limitBodies(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Body != nil {
			if r.URL.Path == backup.RestorePath {
				h.ServeHTTP(w, r) // the restore zip has its own, larger limit
				return
			}
			if r.ContentLength > maxBody {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		h.ServeHTTP(w, r)
	})
}

// newServer has no WriteTimeout: the live stream stays open. Instead each
// write gets its own deadline (stallGuard), so a client that stops reading
// frees its handler instead of holding it forever.
func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: stallGuard(h), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second}
}

// writeStall is how long one write may wait on a client that does not read.
var writeStall = 60 * time.Second

// stallGuard moves the connection's write deadline forward before every
// write, so long streams that keep writing stay open while a stalled one
// fails after writeStall.
func stallGuard(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := &stallWriter{ResponseWriter: w, rc: http.NewResponseController(w)}
		h.ServeHTTP(g, r)
		if g.wrote && !g.hijacked {
			g.extend() // the final flush after the handler returns
		}
	})
}

type stallWriter struct {
	http.ResponseWriter
	rc       *http.ResponseController
	wrote    bool
	hijacked bool
}

func (w *stallWriter) extend() {
	w.wrote = true
	_ = w.rc.SetWriteDeadline(time.Now().Add(writeStall))
}

func (w *stallWriter) Write(b []byte) (int, error) {
	w.extend()
	return w.ResponseWriter.Write(b)
}

func (w *stallWriter) Flush() {
	w.extend()
	_ = w.rc.Flush()
}

// Hijack hands over the connection with no deadline left on it.
func (w *stallWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c, rw, err := w.rc.Hijack()
	if err == nil {
		w.hijacked = true
		_ = c.SetDeadline(time.Time{})
	}
	return c, rw, err
}

// Unwrap gives http.ResponseController the underlying writer.
func (w *stallWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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
	wrap func(supervisor.Loop) supervisor.Loop
	// reg replaces the Registry (tests set short restart delays).
	reg *supervisor.Registry
	// ready is told the Registry and the address once the server listens.
	ready func(reg *supervisor.Registry, addr string)
	// kinds is told every Job kind with a Handler.
	kinds func([]string)
	// parts is told the parts Control once it is made.
	parts func(*parts.Control)
}

func run(c config) error {
	useMemoryMode()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx, c, runOpts{})
}

// serve runs HolyArrs until ctx ends or a restart is asked for.
func serve(ctx context.Context, c config, o runOpts) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	reg := o.reg
	if reg == nil {
		reg = supervisor.NewRegistry()
	}
	supervisor.SetDefault(reg)
	const notStarted = "Safe mode: not started. Restart without safe mode to turn it on."
	if err := os.MkdirAll(c.ConfigDir, 0o750); err != nil {
		return err
	}
	// The app log for the log viewer: every line also goes to a capped
	// file in the config folder, secrets masked; the program output (docker
	// logs) is masked the same way.
	applogs, err := applog.Open(filepath.Join(c.ConfigDir, "logs"))
	if err != nil {
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
	loop := func(l supervisor.Loop) {
		if o.wrap != nil {
			l = o.wrap(l)
		}
		pc.Add(l, false)
	}
	optional := func(l supervisor.Loop) {
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
	unlock, err := store.LockDir(c.ConfigDir)
	if err != nil {
		return fmt.Errorf("config folder %s: %w", c.ConfigDir, err)
	}
	defer unlock()
	// Listen first, so a long update of the data still answers healthz
	// (alive) and shows a short page; the real API takes over once ready.
	swap := &swapHandler{}
	var updating atomic.Bool
	swap.set(urlbase.Handler(c.URLBase, startingHandler(&updating)))
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
	if _, err := backup.ApplyStaged(c.ConfigDir); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	s, err := store.Open(c.ConfigDir, store.Options{OnUpdating: func() { updating.Store(true) }, CacheKiB: lowmem.Pick(0, 4000), ReaderCacheKiB: lowmem.Pick(0, 2000)})
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
		swap.set(urlbase.Handler(c.URLBase, refusedHandler(c.ConfigDir, err, putBackDone)))
		select {
		case <-ctx.Done():
			return err
		case <-putBackDone:
			return errRestart
		}
	}
	defer s.Close()
	pc.Store = s
	if err := pc.Load(ctx); err != nil {
		return fmt.Errorf("part switches: %w", err)
	}
	// A base saved in Settings is used when the start setting is left
	// unset; HOLYARRS_URL_BASE=/ forces none.
	if _, set := os.LookupEnv(urlbase.Env); !set && !c.urlBaseFlag && c.URLBase == "" {
		if saved := api.SavedURLBase(ctx, s.Read()); saved != "" {
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
	key, err := secrets.LoadKey(c.ConfigDir, os.Getenv(secrets.EnvKeyFile))
	if err != nil {
		return err
	}
	vault, err := secrets.Open(ctx, s, key)
	if err != nil {
		return err
	}
	if n, err := vault.MigratePlaintext(ctx); err != nil {
		return fmt.Errorf("encrypting stored secrets: %w", err)
	} else if n > 0 {
		log.Printf("secrets: encrypted %d secrets that were stored in the clear", n)
	}
	// Not fatal: a Source left as it was still works.
	if n, err := source.HidePathKeys(ctx, s, vault); err != nil {
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
	loop(supervisor.Loop{Name: "log_secrets", Label: "Log masking", Every: time.Minute, Run: refreshLogSecrets})
	bm := &backup.Manager{Store: s, Dir: c.ConfigDir, Version: version, CheckKey: func(p string) error { return secrets.CheckFile(p, key) }}
	if _, err := bm.AdoptMigrationCopies(); err != nil {
		log.Printf("backup: %v", err)
	}
	if err := bm.AfterStart(ctx); err != nil {
		return err
	}
	loop(supervisor.Loop{Name: "backups", Label: "Backups", Every: 10 * time.Minute, Deadline: 2 * time.Hour, Run: bm.Pass})
	if err := ensure(ctx, s, c); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	hub := live.NewHub(live.Options{Merge: dlsync.MergePatch})
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
	// Your own traits (#97) go to the judge before anything is judged.
	if err := library.LoadOwnTraits(ctx, s); err != nil {
		return fmt.Errorf("own traits: %w", err)
	}
	dm := &downloader.Manager{Store: s, Live: hub, SavePath: c.SavePath,
		Link: func(ctx context.Context, rel domain.Release) (string, error) {
			return source.GrabLink(ctx, s, vault, rel)
		},
		FromVault: vault.Get}
	// Background download reader: one reader per Downloader inside the
	// "downloads" loop; the Manager and the Downloads view read its snapshots.
	dls := &dlsync.Sync{Store: &dlsync.Store{}, DB: s, Manager: dm, Lines: api.TorrentLines}
	dm.Sync = dls
	im := &importer.Importer{Store: s, Live: hub}
	sr := &source.Searcher{Store: s, Live: hub, Secrets: vault, Queue: q, OnChosen: func(ctx context.Context, slotID, releaseID int64) {
		enqueue(ctx, downloader.KindGrab, domain.Subject{Type: "slot", ID: slotID}, map[string]any{"release_id": releaseID}, fmt.Sprintf("grab:%d", slotID))
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
		enqueue(ctx, source.KindSearchSlot, domain.Subject{Type: "slot", ID: slotID}, nil, fmt.Sprintf("search_slot:%d", slotID))
	}
	// A finished Import: clear a Usenet download out of its Downloader.
	im.OnImported = dm.ClearFinished
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
	q.ProbeSchedule(source.KindFeedCheck, sr.ProbeFeed)
	q.ProbeSchedule(source.KindGrabHeld, sr.ProbeGrabHeld)
	q.Handle(downloader.KindGrab, dm.HandleGrab)
	// An ask turns Ready right after its import, not on the next 30 s ready
	// check: the Requests card would drop from Importing back to Processing.
	var readyCheck atomic.Pointer[asks.Service]
	q.Handle(importer.KindImport, func(ctx context.Context, r *jobs.Run) error {
		err := im.HandleJob(ctx, r)
		if w := readyCheck.Load(); err == nil && w != nil {
			if _, err := w.CheckReady(ctx); err != nil {
				log.Printf("requests: ready check after an import: %v", err)
			}
		}
		return err
	})
	hosted, err := newHostedMetadata(vault)
	if err != nil {
		return fmt.Errorf("metadata server: %w", err)
	}
	hosted.Disk = store.LookupCache{Store: s}
	// "Count this install" is set before the Job queue starts: a Job's
	// request must not go out counted after the owner switched it off.
	hosted.Gate.SetCountryCount(api.CountInstall(ctx, s.Read()))
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
	lib := &library.Service{Store: s, Queue: q, Metadata: md, Live: hub, SavePath: c.SavePath, ArtistImages: pictures,
		ImageServer: hosted.Service.Base() + "/v1/img",
		Artwork: &library.ArtworkCache{Dir: filepath.Join(c.ConfigDir, "artwork"), Allowed: []string{metadata.ImageBase + "/", hosted.Service.Base() + "/v1/img/"},
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
	// Artists still without a picture get one soon after start, apart from
	// the slower MusicBrainz refresh.
	if _, _, err := q.Enqueue(ctx, library.KindArtistPictures, nil, map[string]any{"startup": true}, library.KindArtistPictures, domain.System); err != nil {
		log.Printf("queue %s: %v", library.KindArtistPictures, err)
	}
	// Moves from the old apps: each part that is off is left out, and safe
	// mode refuses moves at the API.
	mover := &migrate.Mover{Store: s, Library: lib, Queue: q, PartOn: func(p string) bool { return !c.SafeMode && pc.On(parts.Part(p)) }}
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
	ol := &lists.Service{Store: s, Queue: q, Library: lib, Metadata: md, TMDBBase: hosted.TMDBBase,
		TraktBase: os.Getenv("HOLYARRS_TRAKT_URL"), TMDBToken: hosted.TMDBAccess,
		TraktClient: func(ctx context.Context) (string, error) { return vault.Setting(ctx, "links", "trakt_client_id") }}
	ol.Register(q)
	q.ProbeSchedule(lists.KindSync, ol.ProbeSync)
	he := &health.Engine{Store: s, Version: version, Publish: func(topic string, data any) { hub.Publish(topic, data) },
		Backups: func(ctx context.Context) (health.Backups, error) {
			newest, every, lastErr, err := bm.State(ctx)
			return health.Backups{Newest: newest, Every: every, LastErr: lastErr}, err
		}}
	// The update check: once a day from the release feed, through the
	// network policy (public only unless the setting allows the local
	// network; never cloud metadata addresses). HOLYARRS_RELEASE_FEED
	// points it elsewhere (tests).
	he.ReleaseFeed = env("HOLYARRS_RELEASE_FEED", health.DefaultReleaseFeed)
	he.ClientFor = func(allowLocal bool) *http.Client {
		allow := source.PublicOnly
		if allowLocal {
			allow = source.NotMetadata
		}
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil
		t.DialContext = source.PolicyDialer(allow).DialContext
		return &http.Client{Transport: t, Timeout: 15 * time.Second}
	}
	he.Register(q)
	// History clean-up: once a day, old Events past their keep time.
	q.Handle(store.KindCleanHistory, func(ctx context.Context, r *jobs.Run) error {
		n, err := s.PruneEvents(ctx, store.ReadHistoryKeep(ctx, s.Read()), time.Now().UTC())
		r.SetResult(map[string]any{"removed": n})
		return err
	})
	q.ProbeSchedule(health.KindCheck, he.Probe)
	// Plex watchlists (Requests): registered here with the other kinds; the
	// reader is set once Requests and sign-in are up, and a pass before
	// that finds nothing to do.
	var watchlists atomic.Pointer[asks.Watchlists]
	q.Handle(asks.KindPlexWatchlists, func(ctx context.Context, r *jobs.Run) error {
		if w := watchlists.Load(); w != nil {
			return w.Run(ctx, r)
		}
		r.SetResult(map[string]any{"skipped": "still starting"})
		return nil
	})
	q.ProbeSchedule(asks.KindPlexWatchlists, func(ctx context.Context, now time.Time) (bool, any, error) {
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
			if pc.On(parts.Subtitles) {
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
	for p, label := range map[jobs.Pool]string{jobs.PoolCore: "Core workers", jobs.PoolIO: "Search and download workers", jobs.PoolHeavy: "Import and scan workers"} {
		if pools[p] == 0 {
			reg.Set(supervisor.StageKey("jobs_"+string(p)), "stage", label, supervisor.Off, notStarted)
			continue
		}
		reg.Set(supervisor.StageKey("jobs_"+string(p)), "stage", label, supervisor.Running, "")
	}
	optional(supervisor.Loop{Name: "downloads", Label: "Download sync", Run: func(ctx context.Context) error {
		// The reader lives and dies with this loop: it stops after dm.Run
		// returns (or panics), then flushes with its own deadline.
		dls.Around(ctx, dm.Run)
		return nil
	}})
	optional(supervisor.Loop{Name: "schedules", Label: "Schedules", Every: time.Minute, Run: func(ctx context.Context) error {
		_, err := q.RunSchedules(ctx, time.Now().UTC())
		return err
	}})
	optional(supervisor.Loop{Name: "stranded_imports", Label: "Stranded imports", Every: 10 * time.Minute, Run: func(ctx context.Context) error {
		n, err := im.RequeueStranded(ctx, func(ctx context.Context, id int64) {
			enqueue(ctx, importer.KindImport, domain.Subject{Type: "download", ID: id}, nil, fmt.Sprintf("import:%d", id))
		})
		if n > 0 {
			log.Printf("stranded show downloads: queued %d imports again", n)
		}
		return err
	}})
	optional(supervisor.Loop{Name: "gates", Label: "Release gates", Every: time.Minute, Run: lib.GatesPass})
	reg.Set(supervisor.StageKey("health"), "stage", "Health checks", supervisor.Running, "")
	reg.GoGuard("health", func() {
		if _, err := he.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("health: %v", err)
		}
	})

	sso, err := auth.SSOFromEnv(c.URLBase)
	if err != nil {
		return err
	}
	au := auth.New(s, auth.Options{Vault: vault, SafeMode: c.SafeMode, SSO: sso})
	defer au.Close()
	loop(supervisor.Loop{Name: "sessions", Label: "Session clean-up", Every: time.Hour, Run: au.Cleanup})
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
	sender := &alerts.Sender{Store: s, Vault: vault}
	sender.Watch(ctx)
	media := &mediaserver.Linker{Store: s, Vault: vault}
	reg.AlsoWait(func() { media.Stop(); media.Wait() })
	reg.AlsoWait(he.Wait)
	if !c.SafeMode {
		media.Watch(ctx)
	}
	defer sender.Wait()
	wish := &asks.Service{Store: s, Library: lib, Metadata: md, Alerts: sender, Live: hub}
	readyCheck.Store(wish)
	optional(supervisor.Loop{Name: "wishlist", Label: "Requests ready check", Every: 30 * time.Second, Run: func(ctx context.Context) error {
		if _, err := wish.CheckReady(ctx); err != nil {
			return err
		}
		if _, err := wish.SendEpisodeNotices(ctx); err != nil {
			return err
		}
		_, err := wish.SendAlbumNotices(ctx)
		return err
	}})
	wish.PrefetchParts = true
	// Plex watchlists are read with each person's own kept Plex sign-in.
	plexWatch := &asks.Watchlists{Asks: wish, Base: env("HOLYARRS_PLEX_DISCOVER_URL", asks.DefaultPlexDiscover),
		Token: au.PlexToken, ClientID: au.PlexClientID}
	watchlists.Store(plexWatch)
	go wish.BackfillParts(context.Background())
	wish.WatchEpisodes()
	wish.WatchTMDBFollows()
	reg.Set(supervisor.StageKey("alerts"), "stage", "Alerts", supervisor.Running, "")
	if c.SafeMode {
		reg.Set(supervisor.StageKey("media_servers"), "stage", "Media-server refresh", supervisor.Off, notStarted)
	} else {
		reg.Set(supervisor.StageKey("media_servers"), "stage", "Media-server refresh", supervisor.Running, "")
	}
	reg.Set(supervisor.StageKey("events"), "stage", "Event hooks", supervisor.Running, "")
	defer wish.Wait()
	// Deezer marks and artist bios come from the metadata server; only the
	// direct stand-in (a Deezer stand-in in tests, or
	// HOLYARRS_DIRECT_SOURCES=1) is asked directly.
	serverDeezer, serverDeezerAlbum, serverBio := hosted.Service.MusicDeezerArtist, hosted.Service.MusicDeezerAlbum, hosted.Service.MusicArtistBio
	if pictures != nil {
		serverDeezer, serverDeezerAlbum, serverBio = nil, nil, nil
	}
	// Artist packages are copied to disk on a schedule (no credential sent);
	// the hosted music source answers from them first.
	pkgs := &musicsync.Sync{Store: s, Source: hosted.Service}
	hosted.Service.SetLocalMusic(pkgs)
	a := api.New(api.Server{Packages: pkgs, Library: lib, Lists: ol, Store: s, Queue: q, Live: hub, Metadata: md, Version: version, Auth: au, Backups: bm, Restart: restart,
		Secrets: vault, TraktBase: os.Getenv("HOLYARRS_TRAKT_URL"), MetadataUp: hosted.Health.Up, ServiceTop: hosted.Service.ServiceTop, TrailerPick: func(ctx context.Context, kind, id, lang string) (string, error) {
			t, err := hosted.Service.TrailerPick(ctx, kind, id, lang)
			return t.Key, err
		}, AnimeMap: hosted.Service.AnimeMap, LiveSearch: flagOn("live_search"), ExtraArt: hosted.Service.Art, ImageBase: hosted.Service.Base() + "/v1/img", MusicControl: hosted.Service.Control, ArtistVideos: hosted.Service.ArtistVideos, ExtraRatings: hosted.Service.Ratings, CountryCount: hosted.Gate.SetCountryCount, WikiTitle: hosted.Service.WikiTitle, WikiPerson: hosted.Service.WikiPerson, AwardWinners: hosted.Service.AwardWinners, Alerts: sender, Asks: wish, Watchlists: plexWatch, Search: sr, Downloads: dm, DLSync: dls, Health: he, Media: media, Subtitles: subs, States: reg, SafeMode: c.SafeMode, Parts: pc, ExitApp: exitApp, Mover: mover,
		Log: applogs, ConfigDir: c.ConfigDir, SavePath: c.SavePath, URLBase: c.URLBase, Deps: music.Deps{MusicTrending: hosted.Service.MusicTrending, MusicPictures: hosted.Service.MusicPictures, MusicNew: hosted.Service.MusicNewWindow, MusicSimilar: hosted.Service.MusicSimilar, MusicCharts: hosted.Service.MusicCharts, MusicGenres: hosted.Service.MusicGenreList, MusicGenreTile: hosted.Service.MusicGenreTile, MusicRowsV2: flagOn("music_rows_v2"), MusicRowsV3: flagOn("music_rows_v3"), MusicRowsV6: flagOn("music_rows_v6"), MusicRowsV7: flagOn("music_rows_v7"), MusicListsV1: flagOn("music_lists_v1"), MusicShelves: hosted.Service.MusicShelves, MusicPools: hosted.Service.MusicPools, MusicRowsTplOff: flagOn("music_rows_tpl_off"), MusicListeners: hosted.Service.MusicListeners, MusicSimilarSet: hosted.Service.MusicSimilarSet, MusicArtistData: hosted.Service.MusicArtistData, MusicSuggest: hosted.Service.MusicSuggest, MusicTopSongs: hosted.Service.MusicTopSongs, MusicRising: hosted.Service.MusicRisingToday, MusicDiscography: hosted.Service.MusicDiscography,
			MusicDeezer: serverDeezer, MusicDeezerAlbum: serverDeezerAlbum, MusicArtistBio: serverBio}})
	// "Count this install" rides on every request to the metadata server.
	a.ApplyCountryCount(ctx)
	// Matching in the reader's queue is link-only: it never adds a title.
	dls.Match = a.MatchLinkOnly
	// Push updates of the snapshots to the Downloads page (only while on).
	dls.Pub = &dlsync.Publisher{Store: dls.Store, Build: a.PublishDL, Health: a.PublishDLHealth}
	a.StartSpeedHistory(ctx)
	if !c.SafeMode {
		a.StartExplore(ctx)
	}
	for _, l := range a.MusicLoops() {
		optional(l)
	}
	for _, l := range a.TrailerLoops() {
		optional(l)
	}
	startTrailers(a, he)
	pc.Start(ctx)
	swap.set(urlbase.Handler(c.URLBase, compress.Handler(limitBodies(a.Handler()))))
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

// showSources are the show details sources: TMDB and TVmaze through the
// metadata server, and TVDB with the person's own key only.
// HOLYARRS_TVMAZE_URL and HOLYARRS_TVDB_URL point at stand-ins (tests); a
// TVmaze stand-in is asked directly, and HOLYARRS_DIRECT_SOURCES=1
// (developers only) keeps the real TVmaze behind the server for TVDB and
// IMDb ids.
func showSources(v *secrets.Vault, hosted *hostedMetadata) metadata.ShowSources {
	var maze metadata.ShowProvider = &metadata.TVMaze{BaseURL: os.Getenv("HOLYARRS_TVMAZE_URL")}
	if !standIn("HOLYARRS_TVMAZE_URL") {
		var direct metadata.ShowProvider
		if directSources() {
			direct = maze
		}
		maze = hosted.Service.Shows("tvmaze", direct)
	}
	return metadata.ShowSources{
		"tmdb":   hosted.TMDB(),
		"tvmaze": maze,
		"tvdb": &metadata.TVDB{BaseURL: os.Getenv("HOLYARRS_TVDB_URL"),
			Key: func(ctx context.Context) (string, error) { return v.Setting(ctx, api.TVDBKeyGroup, api.TVDBKeyName) }},
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
	t.DialContext = source.PolicyDialer(source.PublicOnly).DialContext
	return &metadata.ArtistImages{BaseURL: metadata.DeezerBase, HTTP: &http.Client{Transport: t}}
}

// musicSource is MusicBrainz at no more than one request a second (one
// shared limiter), with answers kept in the lookup_cache table. The base
// URL and cache hours are settings (Settings > Music), read on each
// request; HOLYARRS_MUSICBRAINZ_URL points at a stand-in (tests) and wins.
// Its own clients reach public addresses only, or the local network too
// (never cloud metadata addresses) when the music settings allow it or
// the stand-in is used.
func musicSource(s *store.Store, version string) *metadata.MusicBrainz {
	env := os.Getenv("HOLYARRS_MUSICBRAINZ_URL")
	client := func(allow func(net.IP) bool) *http.Client {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil // the policy must see the real address
		t.DialContext = source.PolicyDialer(allow).DialContext
		return &http.Client{Transport: t}
	}
	public, local := client(source.PublicOnly), client(source.NotMetadata)
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
		Cache:     store.LookupCache{Store: s},
	}
}

// healthcheck asks a running server for its health: 0 when it answers 200, 1
// otherwise. The image has no shell or curl, so its HEALTHCHECK uses this.
func healthcheck(base string) int {
	cl := &http.Client{Timeout: 5 * time.Second}
	resp, err := cl.Get(strings.TrimRight(base, "/") + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		// Under a URL base the health check lives under it too.
		base, _ := urlbase.Clean(os.Getenv(urlbase.Env))
		os.Exit(healthcheck(env("HOLYARRS_HEALTH_URL", "http://127.0.0.1:18730"+base)))
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

// resetOwner sets the owner's name and password from stdin ("name: ..." and
// "password: ..." lines), for when the password is lost. Every session and API
// key ends. It works while the server runs.
func resetOwner(args []string) int {
	c, err := parseFlags(args, os.Stderr)
	if err != nil {
		return 2
	}
	name, pw, err := auth.ReadNewOwner(os.Stdin)
	if err != nil {
		log.Printf("reset-owner: %v", err)
		return 1
	}
	s, err := store.Open(c.ConfigDir, store.Options{})
	if err != nil {
		log.Printf("reset-owner: %v", err)
		return 1
	}
	defer s.Close()
	if err := auth.ResetOwner(context.Background(), s, name, pw); err != nil {
		log.Printf("reset-owner: %v", err)
		return 1
	}
	log.Printf("reset-owner: the owner is now %q; sign in again everywhere", name)
	return 0
}

// flagOn reports whether a feature flag is listed in HOLYARRS_FLAGS
// (comma separated, e.g. "music_rows_v2").
func flagOn(name string) bool {
	for _, f := range strings.Split(os.Getenv("HOLYARRS_FLAGS"), ",") {
		if strings.TrimSpace(f) == name {
			return true
		}
	}
	return false
}

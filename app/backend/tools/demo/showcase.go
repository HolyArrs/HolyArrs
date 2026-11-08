package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// The showcase seeder fills the redesign preview (the showcase backend on
// 18728) with one or more titles in every Library state, so each state can
// be looked at. It only runs on a stopped showcase runtime folder that
// carries the showcase marker, never on the shipped app (18730, 18731) and
// never on the lab. Nothing is downloaded: the Downloader and the indexer
// it adds are stand-ins at a closed local port, and every file it writes is
// a few-KB placeholder.

// showcaseMarker marks a runtime folder as the showcase's.
const showcaseMarker = "showcase.marker"

// showcaseOptions are the showcase command's flags.
type showcaseOptions struct {
	Runtime string // the runtime folder: config, movies, shows, music, downloads inside
	AppURL  string // where the showcase backend answers when it runs
	Reset   bool   // wipe the showcase Library, then seed again
	Live    bool   // music part only, while the backend runs (no stop check, no config lock)
	Init    bool   // mark the runtime folder as the showcase's and stop
	Agent   bool   // add or update the agents' test account from stdin, while the backend runs
	// ClearDownloads removes every download and the stand-in Downloader, then stops.
	ClearDownloads bool
}

func parseShowcase(args []string) (showcaseOptions, error) {
	fs := flag.NewFlagSet("showcase", flag.ContinueOnError)
	var o showcaseOptions
	def := ""
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		def = filepath.Join(la, "Temp", "holyarrs-showcase-18728")
	}
	fs.StringVar(&o.Runtime, "runtime", def, "the showcase runtime folder (config, movies, shows, music, downloads)")
	fs.StringVar(&o.AppURL, "app-url", "http://127.0.0.1:18728", "the showcase backend; it must be stopped")
	fs.BoolVar(&o.Reset, "reset", false, "wipe every title, ask and stand-in download in the showcase, then seed again")
	fs.BoolVar(&o.Live, "live", false, "seed only the music part (folders, members, artists, asks) while the showcase backend runs; not with -reset or -init")
	fs.BoolVar(&o.Init, "init", false, "mark the runtime folder as the showcase's ("+showcaseMarker+") and stop")
	fs.BoolVar(&o.Agent, "agent", false, "add or update the agents' test account (an admin) from \"name:\" and \"password:\" lines on stdin, then stop; the backend may run")
	fs.BoolVar(&o.ClearDownloads, "clear-downloads", false, "remove every download (active ones through the Downloader manager) and the stand-in Downloader, then stop; the backend must be stopped")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return o, nil
}

// checkShowcase refuses anything that is not a stopped showcase: the app
// address must not be the shipped ports, the runtime folder must be named
// as the showcase's, hold a HolyArrs database and the showcase marker, and
// must not be the lab.
func checkShowcase(o showcaseOptions) error {
	if o.Runtime == "" {
		return errors.New("no -runtime folder")
	}
	if u, err := url.Parse(o.AppURL); err == nil {
		switch u.Port() {
		case "18730", "18731":
			return fmt.Errorf("refusing %s: 18730 and 18731 are the shipped app, never the showcase", o.AppURL)
		}
	}
	base := strings.ToLower(filepath.Base(filepath.Clean(o.Runtime)))
	if !strings.HasPrefix(base, "holyarrs-showcase-") {
		return fmt.Errorf("%s is not a showcase runtime folder (its name must start with holyarrs-showcase-)", o.Runtime)
	}
	cfg := filepath.Join(o.Runtime, "config")
	if _, err := os.Stat(filepath.Join(cfg, labMarker)); err == nil {
		return fmt.Errorf("%s carries the lab marker; the showcase seeder never touches the lab", cfg)
	}
	if _, err := os.Stat(filepath.Join(cfg, "holyarrs.db")); err != nil {
		return fmt.Errorf("%s has no HolyArrs database; start the showcase once first", cfg)
	}
	if fi, err := os.Stat(filepath.Join(o.Runtime, showcaseMarker)); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%s has no %s; if it really is the showcase, mark it once with: holyarrs-demo showcase -runtime %s -init", o.Runtime, showcaseMarker, o.Runtime)
	}
	return nil
}

func showcaseCommand(ctx context.Context, o showcaseOptions, out io.Writer) error {
	if o.Live && (o.Reset || o.Init) {
		return errors.New("-live cannot be combined with -reset or -init; stop the backend for those")
	}
	if o.Agent {
		if o.Reset || o.Init || o.Live {
			return errors.New("-agent runs on its own")
		}
		if err := checkShowcase(o); err != nil {
			return err
		}
		w, err := openWorld(ctx, options{ConfigDir: filepath.Join(o.Runtime, "config"), MediaRoot: o.Runtime, DownloadsDir: filepath.Join(o.Runtime, "downloads")}, providers{}, out)
		if err != nil {
			return err
		}
		defer w.Close()
		return w.showcaseAgent(ctx, os.Stdin)
	}
	if !o.Live {
		if err := checkStopped(ctx, o.AppURL, false); err != nil {
			return err
		}
	}
	if o.Init {
		base := strings.ToLower(filepath.Base(filepath.Clean(o.Runtime)))
		if !strings.HasPrefix(base, "holyarrs-showcase-") {
			return fmt.Errorf("%s is not a showcase runtime folder", o.Runtime)
		}
		if err := os.WriteFile(filepath.Join(o.Runtime, showcaseMarker), []byte("This folder is the HolyArrs redesign showcase; holyarrs-demo showcase may wipe and seed it.\n"), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "marked %s as the showcase\n", o.Runtime)
		return nil
	}
	if err := checkShowcase(o); err != nil {
		return err
	}
	cfg := filepath.Join(o.Runtime, "config")
	if !o.Live {
		unlock, err := lockConfig(cfg)
		if err != nil {
			return err
		}
		defer unlock()
	}
	lo := options{ConfigDir: cfg, MediaRoot: o.Runtime, DownloadsDir: filepath.Join(o.Runtime, "downloads")}
	w, err := openWorld(ctx, lo, providers{}, out)
	if err != nil {
		return err
	}
	defer w.Close()
	// A failed download stays failed in the showcase: no search after it.
	w.dm.OnFailed = nil
	if o.ClearDownloads {
		if o.Reset || o.Live {
			return errors.New("-clear-downloads runs on its own")
		}
		return w.showcaseClearDownloads(ctx)
	}
	if o.Reset {
		if err := w.showcaseWipe(ctx); err != nil {
			return fmt.Errorf("reset: %w", err)
		}
	}
	p, err := showcaseProviders(ctx, w)
	if err != nil {
		return err
	}
	w.lib.Metadata, w.lib.Shows, w.lib.Artists = p.Movies, p.Shows, p.Artists
	return w.showcaseSeed(ctx, o.Live)
}

// showcaseProviders reach TMDB through the HolyArrs metadata server with
// the credential the showcase install already keeps in its vault (it never
// registers a new one), and MusicBrainz through the same server.
func showcaseProviders(ctx context.Context, w *world) (providers, error) {
	tok, err := w.vault.Setting(ctx, "links", "metadata_service_installation_token")
	if err != nil {
		return providers{}, err
	}
	if tok == "" {
		return providers{}, errors.New("the showcase has no metadata server credential yet; start it once so it registers")
	}
	access := func(context.Context) (string, error) { return tok, nil }
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = outbound.PolicyDialer(outbound.PublicOnly).DialContext
	cl := &http.Client{Transport: t, Timeout: 5 * time.Minute}
	svc, err := metadata.NewService(metadata.ServiceOptions{URL: metadata.ServiceURL(), HTTP: cl, Access: access})
	if err != nil {
		return providers{}, err
	}
	tmdb := &metadata.TMDB{Access: access, BaseURL: svc.Base() + metadata.TMDBPath, HTTP: cl}
	mb := &metadata.MusicBrainz{
		Client: func(context.Context) *http.Client { return cl },
		BaseURL: func(ctx context.Context) string {
			d, _ := library.ReadMusicDefaults(ctx, w.s.Read())
			return d.MusicBrainzURL
		},
		TTL: func(ctx context.Context) time.Duration {
			d, _ := library.ReadMusicDefaults(ctx, w.s.Read())
			return time.Duration(d.CacheHours) * time.Hour
		},
		UserAgent: metadata.UserAgentFor("showcase-seed"),
		Cache:     metadata.LookupCache{Store: w.s},
	}
	return providers{Movies: tmdb, Shows: metadata.ShowSources{"tmdb": tmdb, "tvmaze": &metadata.TVMaze{}}, Artists: svc.Artists(mb)}, nil
}

// showcaseWipe removes every title in the showcase Library (through the
// app's delete), every ask, every stand-in download, the stand-in
// Downloader and indexer, and every placeholder file. Accounts, settings
// and the vault stay.
func (w *world) showcaseWipe(ctx context.Context) error {
	db := w.s.Read()
	// Stand-in downloads first, through the Downloader manager.
	dls, err := database.Select[domain.Download](ctx, db, "downloads", "state IN ('sent', 'queued', 'downloading', 'stalled', 'paused') ORDER BY id")
	if err != nil {
		return err
	}
	for _, d := range dls {
		if err := w.dm.Remove(ctx, d.ID, downloads.RemoveChoice{}, w.by, "showcase seed"); err != nil {
			w.say("   download %d: %v", d.ID, err)
		}
	}
	owner, err := ownerPerson(ctx, db)
	if err != nil {
		return err
	}
	as, err := database.Select[domain.Ask](ctx, db, "asks", "1=1 ORDER BY id")
	if err != nil {
		return err
	}
	svc := &requests.Service{Store: w.s, Library: w.lib}
	for _, a := range as {
		if a.State == domain.AskAsked || a.State == domain.AskApproved {
			if err := svc.Withdraw(ctx, a.ID, owner, true); err != nil {
				w.say("   ask %d: %v", a.ID, err)
			}
		}
	}
	// Asks that are settled (turned down, available) cannot be withdrawn;
	// the showcase drops them so a reseed starts clean.
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM asks")
		return err
	}); err != nil {
		return err
	}
	ts, err := database.Select[mediafiles.Title](ctx, db, "titles", "state != 'removed' ORDER BY id")
	if err != nil {
		return err
	}
	for _, t := range ts {
		if _, err := w.lib.Remove(ctx, t.ID, library.RemoveChoice{}, true, w.by); err != nil {
			w.say("   %s: %v", t.Name, err)
		}
	}
	_ = w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE jobs SET state = 'cancelled', ended_at = ?, updated_at = ? WHERE state = 'queued'", database.FormatTime(time.Now()), database.FormatTime(time.Now()))
		return err
	})
	ds, _ := database.Select[domain.Downloader](ctx, db, "downloaders", "name = ? AND removed = 0", standInDownloader)
	for _, d := range ds {
		if err := downloads.Remove(ctx, w.s, d.ID, w.by); err != nil {
			w.say("   Downloader %d: %v", d.ID, err)
		}
	}
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM sources WHERE name = ?", standInSource)
		return err
	}); err != nil {
		w.say("   stand-in indexer kept: %v", err)
	}
	files, dirs := w.removeFiles()
	w.say("== reset: %d titles deleted, %d asks, %d downloads, %d files and %d folders removed", len(ts), len(as), len(dls), files, dirs)
	w.m = newManifest()
	return w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM settings WHERE grp = 'this_app' AND key = ?", manifestKey)
		return err
	})
}

// showcaseClearDownloads removes every download from the showcase: active
// ones through the Downloader manager (their Slots go back to wanted), the
// rest (finished, failed, imported) as rows, and then the stand-in
// Downloader, and moves Slots that wait on a decision back to wanted. The
// Downloads page then shows only what a real Downloader
// reports. Titles, files, asks and accounts stay.
func (w *world) showcaseClearDownloads(ctx context.Context) error {
	db := w.s.Read()
	active, err := database.Select[domain.Download](ctx, db, "downloads", "state IN ('sent', 'queued', 'downloading', 'stalled', 'paused') ORDER BY id")
	if err != nil {
		return err
	}
	for _, d := range active {
		if err := w.dm.Remove(ctx, d.ID, downloads.RemoveChoice{}, w.by, "showcase: no stand-in downloads"); err != nil {
			w.say("   download %d: %v", d.ID, err)
		}
	}
	var rows int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("DELETE FROM downloads")
		if err == nil {
			rows, _ = res.RowsAffected()
		}
		return err
	}); err != nil {
		return err
	}
	ds, _ := database.Select[domain.Downloader](ctx, db, "downloaders", "name = ? AND removed = 0", standInDownloader)
	for _, d := range ds {
		if err := downloads.Remove(ctx, w.s, d.ID, w.by); err != nil {
			w.say("   Downloader %d: %v", d.ID, err)
		}
	}
	// Slots left waiting on a decision about a seeded download go back to
	// wanted (better_wanted when a copy is kept), as the page's own "Stop"
	// choice would move them, so no Review rows remain.
	var decided int
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		r, err := tx.Query("SELECT id, file_id IS NOT NULL FROM slots WHERE state = ?", string(mediafiles.SlotNeedsADecision))
		if err != nil {
			return err
		}
		type slot struct {
			id   int64
			copy bool
		}
		var ss []slot
		for r.Next() {
			var x slot
			if err := r.Scan(&x.id, &x.copy); err != nil {
				r.Close()
				return err
			}
			ss = append(ss, x)
		}
		r.Close()
		for _, x := range ss {
			to := mediafiles.SlotWanted
			if x.copy {
				to = mediafiles.SlotBetterWanted
			}
			if err := mediafiles.MoveSlotTx(tx, x.id, to, "showcase: no stand-in downloads", w.by); err != nil {
				return err
			}
			decided++
		}
		return nil
	}); err != nil {
		return err
	}
	w.say("== cleared: %d active downloads removed, %d download rows deleted, %d stand-in Downloaders removed, %d decisions cleared", len(active), rows, len(ds), decided)
	return nil
}

const (
	standInDownloader = "Showcase stand-in (no real downloads)"
	standInSource     = "Showcase stand-in indexer (disabled)"
)

func ownerPerson(ctx context.Context, db database.Querier) (requests.Person, error) {
	var p requests.Person
	err := db.QueryRowContext(ctx, "SELECT id, name FROM users WHERE role = 'owner' ORDER BY id LIMIT 1").Scan(&p.UserID, &p.Name)
	if err != nil {
		return p, fmt.Errorf("the showcase has no owner: %w", err)
	}
	p.AutoApprove, p.NoLimit = true, true
	return p, nil
}

// showcaseSeed adds each scenario once; a scenario already recorded in the
// seed record is skipped, so running it again adds nothing twice.
func (w *world) showcaseSeed(ctx context.Context, musicOnly bool) error {
	start := time.Now()
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"library folders", w.showcaseFolders},
		{"stand-ins", w.standIns},
		{"members", w.members},
		{"movies", w.showcaseMovies},
		{"shows", w.showcaseShows},
		{"artists", w.showcaseArtists},
		{"posters", w.posters},
		{"asks", w.showcaseAsks},
	}
	for _, st := range steps {
		if musicOnly && !map[string]bool{"library folders": true, "members": true, "artists": true, "asks": true}[st.name] {
			continue
		}
		w.say("== %s", st.name)
		if err := st.fn(ctx); err != nil {
			_ = w.saveManifest(ctx)
			return fmt.Errorf("%s: %w", st.name, err)
		}
		if err := w.saveManifest(ctx); err != nil {
			return err
		}
	}
	res, err := w.lib.Rescan(ctx, w.by)
	if err != nil {
		return fmt.Errorf("rescan: %w", err)
	}
	w.say("== rescan: %d folders, %d missing, %d back, %d found", res.Folders, res.Missing, res.Back, res.Found)
	w.say("== done in %s", time.Since(start).Round(time.Second))
	return w.summary(ctx)
}

// showcaseFolders makes sure movies, shows and music Library folders exist
// inside the runtime folder.
func (w *world) showcaseFolders(ctx context.Context) error {
	dps, _ := library.DownloadPaths(ctx, w.s.Read(), w.Downloads)
	for _, f := range []library.FolderInput{
		{Path: filepath.Join(w.o.MediaRoot, "movies"), Kind: "movie", Label: "Movies"},
		{Path: filepath.Join(w.o.MediaRoot, "shows"), Kind: "show", Label: "TV shows"},
		{Path: filepath.Join(w.o.MediaRoot, "music"), Kind: "music", Label: "Music"},
	} {
		var n int
		if err := w.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM library_folders WHERE kind = ?", f.Kind).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if err := os.MkdirAll(f.Path, 0o755); err != nil {
			return err
		}
		lf, err := library.AddFolder(ctx, w.s, f, dps, w.by)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Path, err)
		}
		w.m.Folders = append(w.m.Folders, lf.ID)
		w.say("   added %s Library folder %s", f.Kind, f.Path)
	}
	return nil
}

// standIns adds a disabled indexer at a closed local port so Releases have
// something to belong to. It reaches nothing. The showcase adds no
// Downloader and no downloads: the Downloads page shows only real data.
func (w *world) standIns(ctx context.Context) error {
	db := w.s.Read()
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE name = ?", standInSource).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		src := &domain.Source{Name: standInSource, Protocol: "torrent", Format: "torznab", URL: "http://127.0.0.1:9/showcase", AllowLocal: true,
			Kinds: []string{"movie", "show", "music"}, Categories: map[string][]int{"movie": {2000}}, Priority: 50, State: "disabled", Extra: map[string]string{}}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "sources", src, w.by); return err }); err != nil {
			return err
		}
		w.say("   added the stand-in indexer (disabled)")
	}
	return nil
}

// showcaseMembers are the requesters: a member and a Kid limited to PG.
var showcaseMembers = []struct{ Name, MaxRating string }{
	{"Maya", ""},
	{"Leo (Kid)", "PG"},
}

// members adds the requesters with no password: they exist to ask, and
// nobody signs in as them.
func (w *world) members(ctx context.Context) error {
	perms, _ := json.Marshal(webserver.DefaultPerms(webserver.RoleMember))
	for _, m := range showcaseMembers {
		var n int
		if err := w.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM users WHERE name = ?", m.Name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		u := &domain.User{Name: m.Name, Role: webserver.RoleMember, State: "active", Perms: string(perms), MaxRating: m.MaxRating, RatingCountry: "US"}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "users", u, w.by); return err }); err != nil {
			return err
		}
		w.say("   added member %s (age limit %q, no password)", m.Name, m.MaxRating)
	}
	return nil
}

// showcaseAgent adds or updates the agents' test account: an admin with a
// password, so agents can check signed-in pages on the showcase. The name
// and password come from stdin, never from flags, so they stay out of
// process lists and shell history. It never touches the owner.
func (w *world) showcaseAgent(ctx context.Context, in io.Reader) error {
	name, pw, err := accounts.ReadNewOwner(in)
	if err != nil {
		return err
	}
	h, err := accounts.HashPassword(pw)
	if err != nil {
		return err
	}
	perms, _ := json.Marshal(webserver.DefaultPerms(webserver.RoleAdmin))
	err = w.s.Write(ctx, func(tx *sql.Tx) error {
		var id int64
		var role string
		err := tx.QueryRow("SELECT id, role FROM users WHERE name = ?", name).Scan(&id, &role)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = database.Insert(tx, "users", &domain.User{Name: name, Role: webserver.RoleAdmin, PasswordHash: h, State: "active", Perms: string(perms), RatingCountry: "US"}, w.by)
			return err
		}
		if err != nil {
			return err
		}
		if role == webserver.RoleOwner {
			return fmt.Errorf("%q is the owner; pick another name for the agents' account", name)
		}
		_, err = tx.Exec("UPDATE users SET password_hash = ?, state = 'active' WHERE id = ?", h, id)
		return err
	})
	if err == nil {
		w.say("   agents' test account %q ready (admin)", name)
	}
	return err
}

func (w *world) member(ctx context.Context, name string) (requests.Person, error) {
	var p requests.Person
	err := w.s.Read().QueryRowContext(ctx, "SELECT id, name, max_rating FROM users WHERE name = ?", name).Scan(&p.UserID, &p.Name, &p.MaxRating)
	return p, err
}

// claim deletes a title that is already in the Library but not part of the
// seed, so the scenario can add it fresh. Only the showcase does this.
func (w *world) claim(ctx context.Context, id int64) error {
	if id == 0 {
		return nil
	}
	_, err := w.lib.Remove(ctx, id, library.RemoveChoice{}, true, w.by)
	return err
}

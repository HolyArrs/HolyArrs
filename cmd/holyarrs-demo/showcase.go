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
	"strconv"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs/internal/asks"
	"github.com/CoderLuii/holyarrs/internal/auth"
	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/downloader"
	"github.com/CoderLuii/holyarrs/internal/library"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/naming"
	"github.com/CoderLuii/holyarrs/internal/source"
	"github.com/CoderLuii/holyarrs/internal/store"
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
	fs.BoolVar(&o.Live, "live", false, "seed only the music part (folders, members, artists, activity, asks) while the showcase backend runs; not with -reset or -init")
	fs.BoolVar(&o.Init, "init", false, "mark the runtime folder as the showcase's ("+showcaseMarker+") and stop")
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
	t.DialContext = source.PolicyDialer(source.PublicOnly).DialContext
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
		Cache:     store.LookupCache{Store: w.s},
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
	dls, err := store.Select[domain.Download](ctx, db, "downloads", "state IN ('sent', 'queued', 'downloading', 'stalled', 'paused') ORDER BY id")
	if err != nil {
		return err
	}
	for _, d := range dls {
		if err := w.dm.Remove(ctx, d.ID, downloader.RemoveChoice{}, w.by, "showcase seed"); err != nil {
			w.say("   download %d: %v", d.ID, err)
		}
	}
	owner, err := ownerPerson(ctx, db)
	if err != nil {
		return err
	}
	as, err := store.Select[domain.Ask](ctx, db, "asks", "1=1 ORDER BY id")
	if err != nil {
		return err
	}
	svc := &asks.Service{Store: w.s, Library: w.lib}
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
	ts, err := store.Select[domain.Title](ctx, db, "titles", "state != 'removed' ORDER BY id")
	if err != nil {
		return err
	}
	for _, t := range ts {
		if _, err := w.lib.Remove(ctx, t.ID, library.RemoveChoice{}, true, w.by); err != nil {
			w.say("   %s: %v", t.Name, err)
		}
	}
	_ = w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE jobs SET state = 'cancelled', ended_at = ?, updated_at = ? WHERE state = 'queued'", store.FormatTime(time.Now()), store.FormatTime(time.Now()))
		return err
	})
	ds, _ := store.Select[domain.Downloader](ctx, db, "downloaders", "name = ? AND removed = 0", standInDownloader)
	for _, d := range ds {
		if err := downloader.Remove(ctx, w.s, d.ID, w.by); err != nil {
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

const (
	standInDownloader = "Showcase stand-in (no real downloads)"
	standInSource     = "Showcase stand-in indexer (disabled)"
)

func ownerPerson(ctx context.Context, db store.Querier) (asks.Person, error) {
	var p asks.Person
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
		{"activity", w.showcaseActivity},
		{"asks", w.showcaseAsks},
	}
	for _, st := range steps {
		if musicOnly && !map[string]bool{"library folders": true, "members": true, "artists": true, "activity": true, "asks": true}[st.name] {
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

// standIns adds a disabled indexer and a Downloader at a closed local port,
// so Releases and Downloads have something to belong to. Neither can reach
// anything: nothing is searched for or downloaded.
func (w *world) standIns(ctx context.Context) error {
	db := w.s.Read()
	var n int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sources WHERE name = ?", standInSource).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		src := &domain.Source{Name: standInSource, Protocol: "torrent", Format: "torznab", URL: "http://127.0.0.1:9/showcase", AllowLocal: true,
			Kinds: []string{"movie", "show", "music"}, Categories: map[string][]int{"movie": {2000}}, Priority: 50, State: "disabled", Extra: map[string]string{}}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := store.Insert(tx, "sources", src, w.by); return err }); err != nil {
			return err
		}
		w.say("   added the stand-in indexer (disabled)")
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM downloaders WHERE name = ? AND removed = 0", standInDownloader).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		d := &domain.Downloader{Name: standInDownloader, Adapter: "qbittorrent", Protocol: "torrent", URL: "http://127.0.0.1:9", Category: "showcase",
			Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := store.Insert(tx, "downloaders", d, w.by); return err }); err != nil {
			return err
		}
		w.m.Downloaders = append(w.m.Downloaders, d.ID)
		w.say("   added the stand-in Downloader at a closed port")
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
	perms, _ := json.Marshal(auth.DefaultPerms(auth.RoleMember))
	for _, m := range showcaseMembers {
		var n int
		if err := w.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM users WHERE name = ?", m.Name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		u := &domain.User{Name: m.Name, Role: auth.RoleMember, State: "active", Perms: string(perms), MaxRating: m.MaxRating, RatingCountry: "US"}
		if err := w.s.Write(ctx, func(tx *sql.Tx) error { _, err := store.Insert(tx, "users", u, w.by); return err }); err != nil {
			return err
		}
		w.say("   added member %s (age limit %q, no password)", m.Name, m.MaxRating)
	}
	return nil
}

func (w *world) member(ctx context.Context, name string) (asks.Person, error) {
	var p asks.Person
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

// showcaseMovies adds every movie scenario.
func (w *world) showcaseMovies(ctx context.Context) error {
	lf, err := w.folderOf(ctx, "movie")
	if err != nil {
		return err
	}
	var fourK int64
	_ = w.s.Read().QueryRowContext(ctx, "SELECT id FROM pick_rules WHERE name = ?", domain.FourKName).Scan(&fourK)
	for i, e := range showcaseMovieList {
		key := "movie:" + e.TMDB
		if w.m.Titles[key] != 0 || e.Ask != "" {
			continue
		}
		old, err := library.TitleByTMDB(ctx, w.s.Read(), e.TMDB)
		if err != nil {
			return err
		}
		if err := w.claim(ctx, old); err != nil {
			return fmt.Errorf("%s: %w", e.Name, err)
		}
		m, err := w.lib.Metadata.Movie(ctx, "tmdb:"+e.TMDB)
		if err != nil {
			w.skip("movie %s (tmdb %s): %v", e.Name, e.TMDB, err)
			continue
		}
		if !sameName(m.Name, e.Name) {
			w.skip("movie tmdb %s is %q, not %q", e.TMDB, m.Name, e.Name)
			continue
		}
		var id int64
		if e.Quality != "" && e.Plan != planGrab {
			if id, err = w.adoptMovie(ctx, lf, m, movieEntry{TMDB: e.TMDB, Name: e.Name, Quality: e.Quality}, 500+i); err != nil {
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		} else {
			if id, _, err = w.lib.Add(ctx, m, library.AddOptions{LibraryFolderID: lf.ID}, w.by); err != nil {
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		}
		w.m.Titles[key] = id
		if e.FourK && fourK != 0 {
			dps, _ := library.DownloadPaths(ctx, w.s.Read(), w.Downloads)
			if _, err := library.AddVersion(ctx, w.s, id, library.VersionInput{Label: "4K", PickRulesID: fourK}, dps, w.lib.Clock(), w.by); err != nil {
				w.say("   %s 4K Version: %v", m.Name, err)
			}
		}
		if e.Keep == "off" {
			off := false
			if _, err := w.lib.Edit(ctx, id, library.TitleChange{KeepUpToDate: &off}, false, w.by); err != nil {
				w.say("   %s keep up to date off: %v", m.Name, err)
			}
		}
		if e.Plan == planGone {
			w.removeMovieFile(ctx, id)
		}
		w.say("   %-48s %s", m.Name+" ("+strconv.Itoa(m.Year)+")", e.Scenario)
	}
	for _, name := range showcaseUpcoming {
		if err := w.upcoming(ctx, lf, name); err != nil {
			return err
		}
	}
	return nil
}

// removeMovieFile deletes a movie's placeholder from disk; the rescan at the
// end finds it gone and marks the File missing, as when a disk loses it.
func (w *world) removeMovieFile(ctx context.Context, titleID int64) {
	var rel, lfPath string
	err := w.s.Read().QueryRowContext(ctx, `SELECT f.relative_path, lf.path FROM files f JOIN versions v ON v.id = f.version_id
		JOIN library_folders lf ON lf.id = v.library_folder_id WHERE v.title_id = ? AND f.role = 'main' ORDER BY f.id LIMIT 1`, titleID).Scan(&rel, &lfPath)
	if err != nil {
		w.say("   no file to remove for title %d: %v", titleID, err)
		return
	}
	p := filepath.Join(lfPath, filepath.FromSlash(rel))
	if isPlaceholder(p) {
		_ = os.Remove(p)
	}
}

// showcaseShows adds every show scenario.
func (w *world) showcaseShows(ctx context.Context) error {
	lf, err := w.folderOf(ctx, "show")
	if err != nil {
		return err
	}
	for i, e := range showcaseShowList {
		key := "show:" + e.TMDB
		if w.m.Titles[key] != 0 {
			continue
		}
		sh, err := w.lib.LookupShow(ctx, "tmdb:"+e.TMDB, "tmdb")
		if err != nil {
			w.skip("show %s (tmdb %s): %v", e.Name, e.TMDB, err)
			continue
		}
		if !sameName(sh.Name, e.Name) {
			w.skip("show tmdb %s is %q, not %q", e.TMDB, sh.Name, e.Name)
			continue
		}
		old, err := library.ShowByIDs(ctx, w.s.Read(), sh.IDs)
		if err != nil {
			return err
		}
		if err := w.claim(ctx, old); err != nil {
			return fmt.Errorf("%s: %w", sh.Name, err)
		}
		folder := naming.ShowFolder(w.ns, naming.Facts{Title: sh.Name, Year: sh.Year, TMDB: sh.IDs["tmdb"], IMDB: sh.IDs["imdb"]})
		dir := filepath.Join(lf.Path, folder)
		if !hasVideo(dir) {
			eps := episodesWithFiles(sh, e.showEntry, time.Now())
			if e.Specials > 0 {
				eps = nil
				for _, ep := range sh.Episodes {
					if ep.Season == 0 && ep.Number > 0 && ep.Number <= e.Specials {
						eps = append(eps, ep)
					}
				}
			}
			for j, ep := range eps {
				release := fmt.Sprintf("%s.S%02dE%02d.%s", dotted(sh.Name), ep.Season, ep.Number, qualityTags[e.Quality])
				facts := naming.FactsFrom(sh.Name, sh.Year, sh.IDs, parseName(release), release, ".mkv")
				ef := naming.EpisodeFacts{Facts: facts, Order: naming.OrderAired, Season: ep.Season, Episodes: []int{ep.Number}, Titles: []string{ep.Name}}
				rel := naming.EpisodePath(w.ns, folder, ef)
				if err := w.placeholder(filepath.Join(lf.Path, filepath.FromSlash(rel)), i*1000+j); err != nil {
					return err
				}
			}
		}
		opt := library.ShowAddOptions{LibraryFolderID: lf.ID, Seasons: e.Choice, EpisodeOrder: e.Order, Source: "tmdb", NewSeasons: "all"}
		if e.Keep == "off" {
			off := false
			opt.KeepUpToDate = &off
		}
		var a *library.ShowAdoption
		if hasVideo(dir) {
			a = &library.ShowAdoption{FolderName: folder}
			opt.Adopt, opt.WhyAdded = a, "import"
		}
		id, _, err := w.lib.AddShow(ctx, "tmdb:"+e.TMDB, opt, w.by)
		if err != nil {
			return fmt.Errorf("%s: %w", sh.Name, err)
		}
		w.m.Titles[key] = id
		if e.Specials > 0 {
			if err := w.lib.SetSeason(ctx, id, 0, true, w.by); err != nil {
				w.say("   %s specials: %v", sh.Name, err)
			}
		}
		how := "no files"
		if a != nil {
			how = fmt.Sprintf("%d files adopted", a.Adopted)
		}
		w.say("   %-48s %s (%s)", sh.Name, e.Scenario, how)
	}
	return nil
}

// showcaseArtists adds every artist scenario. A complete artist's albums are
// written as placeholder tracks first (named from the album's own track
// list) and the artist folder is adopted, as Library > Adopt does.
func (w *world) showcaseArtists(ctx context.Context) error {
	if w.lib.Artists == nil {
		w.skip("artists: no music source")
		return nil
	}
	lf, err := w.folderOf(ctx, "music")
	if err != nil {
		return err
	}
	for _, e := range showcaseArtistList {
		key := "artist:" + e.MBID
		if e.Ask {
			continue
		}
		if w.m.Titles[key] != 0 {
			// A complete artist whose album track lists timed out has
			// fewer albums on disk than planned: redo it (and its follows).
			if !e.Complete || !w.artistShort(ctx, e) {
				continue
			}
			w.say("   %s: albums missing on disk, redoing", e.Name)
			for _, who := range e.Follow {
				delete(w.m.Steps, "follow artist "+e.MBID+" "+who)
			}
		}
		old, err := library.ArtistByMBID(ctx, w.s.Read(), e.MBID)
		if err != nil {
			return err
		}
		if err := w.claim(ctx, old); err != nil {
			return fmt.Errorf("%s: %w", e.Name, err)
		}
		id, _, err := w.lib.AddArtist(ctx, e.MBID, library.ArtistAddOptions{LibraryFolderID: lf.ID, Albums: "none", NewAlbums: e.NewAlbums}, w.by)
		if err != nil {
			w.skip("artist %s: %v", e.Name, err)
			continue
		}
		kept := w.keepAlbums(ctx, id, e.Albums, e.AllBut)
		if _, err := w.lib.RefreshArtist(ctx, id, w.by, nil); err != nil {
			w.say("   %s: refresh: %v", e.Name, err)
		}
		if e.Complete {
			if nid, err := w.adoptArtist(ctx, lf, id, e); err != nil {
				w.say("   %s: adopt: %v", e.Name, err)
			} else {
				id = nid
			}
		}
		w.m.Titles[key] = id
		_ = w.saveManifest(ctx)
		var future int
		_ = w.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM parts WHERE title_id = ? AND kind = 'album' AND release_date > ?", id, store.FormatTime(time.Now())).Scan(&future)
		w.say("   %-48s %s (%d albums kept, %d not out yet)", e.Name, e.Scenario, kept, future)
	}
	return nil
}

// keepAlbums keeps up to n studio albums spread over the career, or with
// allBut every studio album except the newest allBut.
func (w *world) keepAlbums(ctx context.Context, id int64, n, allBut int) int {
	parts, err := store.Select[domain.Part](ctx, w.s.Read(), "parts",
		"title_id = ? AND kind = 'album' AND included = 1 AND release_type = 'album' AND release_date IS NOT NULL AND release_date < ? ORDER BY release_date",
		id, store.FormatTime(time.Now()))
	if err != nil {
		return 0
	}
	kept := 0
	idx := spread(len(parts), n)
	if allBut > 0 {
		idx = nil
		for i := 0; i < len(parts)-allBut; i++ {
			idx = append(idx, i)
		}
	}
	for _, i := range idx {
		keep := true
		if _, err := w.lib.SetAlbum(ctx, parts[i].ID, &keep, 0, w.by); err == nil {
			kept++
		}
	}
	return kept
}

// adoptArtist writes one placeholder per wanted track (named "NN - Track"
// inside "Album (Year)"), deletes the artist and adopts its folder, so the
// albums are matched to the files by the app's own adopt.
func (w *world) adoptArtist(ctx context.Context, lf domain.LibraryFolder, id int64, e showcaseArtist) (int64, error) {
	db := w.s.Read()
	var folder string
	if err := db.QueryRowContext(ctx, "SELECT folder_name FROM versions WHERE title_id = ? AND is_main = 1", id).Scan(&folder); err != nil {
		return id, err
	}
	rows, err := db.QueryContext(ctx, `SELECT p.name, COALESCE(strftime('%Y', p.release_date), ''), COALESCE(json_extract(i.numbers, '$.track'), 0), COALESCE(i.name, '')
		FROM items i JOIN parts p ON p.id = i.part_id WHERE i.title_id = ? AND i.kind = 'track' AND i.keep_up_to_date = 1 ORDER BY p.id, i.id`, id)
	if err != nil {
		return id, err
	}
	type track struct {
		album, year, name string
		n                 int
	}
	var tracks []track
	for rows.Next() {
		var t track
		if err := rows.Scan(&t.album, &t.year, &t.n, &t.name); err == nil {
			tracks = append(tracks, t)
		}
	}
	rows.Close()
	if len(tracks) == 0 {
		return id, errors.New("no wanted tracks")
	}
	if folder == "" {
		folder = safeName(e.Name)
	}
	for i, t := range tracks {
		album := safeName(t.album)
		if t.year != "" {
			album += " (" + t.year + ")"
		}
		p := filepath.Join(lf.Path, folder, album, fmt.Sprintf("%02d - %s.flac", t.n, safeName(t.name)))
		if err := w.placeholder(p, 9000+i); err != nil {
			return id, err
		}
	}
	if _, err := w.lib.Remove(ctx, id, library.RemoveChoice{}, true, w.by); err != nil {
		return id, err
	}
	// MusicBrainz track lists time out often: a failed adopt is undone and
	// tried again, three times in all.
	for try := 1; ; try++ {
		res, err := w.lib.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: folder, ID: e.MBID}}, w.by)
		if err != nil {
			return 0, err
		}
		if len(res) == 1 && res[0].Error == "" && res[0].TitleID != 0 {
			return res[0].TitleID, nil
		}
		if try == 3 || len(res) != 1 || res[0].TitleID == 0 {
			return 0, fmt.Errorf("adopt %s: %+v", folder, res)
		}
		w.say("   %s: adopt try %d: %s; trying again", e.Name, try, res[0].Error)
		if _, err := w.lib.Remove(ctx, res[0].TitleID, library.RemoveChoice{}, true, w.by); err != nil {
			return res[0].TitleID, err
		}
	}
}

// artistShort reports whether a complete artist has fewer studio albums
// on disk than its scenario keeps.
func (w *world) artistShort(ctx context.Context, e showcaseArtist) bool {
	id, err := library.ArtistByMBID(ctx, w.s.Read(), e.MBID)
	if err != nil || id == 0 {
		return true
	}
	var n, had int
	_ = w.s.Read().QueryRowContext(ctx, `SELECT count(*), coalesce(sum(EXISTS (SELECT 1 FROM items i JOIN slots s ON s.item_id = i.id
		WHERE i.part_id = p.id AND i.kind = 'track' AND s.file_id IS NOT NULL)), 0)
		FROM parts p WHERE p.title_id = ? AND p.kind = 'album' AND p.dropped_at IS NULL AND p.release_type = 'album'
		AND coalesce(p.secondary_types, '[]') IN ('[]', 'null', '') AND p.release_date IS NOT NULL AND p.release_date < ?`,
		id, store.FormatTime(time.Now())).Scan(&n, &had)
	want := min(e.Albums, n)
	if e.AllBut > 0 {
		want = n - e.AllBut
	}
	return had < want
}

// safeName drops characters Windows does not allow in a file name.
func safeName(s string) string {
	return strings.TrimRight(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return -1
		}
		return r
	}, s), ". ")
}

// showcaseActivity makes the grabs, imports, failed and unfinished
// downloads through the app's own pick, Downloader and import code with the
// stand-in Downloader.
func (w *world) showcaseActivity(ctx context.Context) error {
	ds, err := store.Select[domain.Downloader](ctx, w.s.Read(), "downloaders", "name = ? AND removed = 0", standInDownloader)
	if err != nil || len(ds) == 0 {
		w.skip("activity: no stand-in Downloader")
		return err
	}
	w.dm.Sleep = func(context.Context, time.Duration) error { return nil }
	if _, err := w.dm.Check(ctx, ds[0]); err != nil {
		return fmt.Errorf("stand-in Downloader: %w", err)
	}
	gb := int64(1 << 30)
	now := time.Now().UTC()
	var stalled []int64
	for _, e := range showcaseMovieList {
		id := w.m.Titles["movie:"+e.TMDB]
		key := "activity movie " + e.TMDB
		if id == 0 || w.m.Steps[key] || e.Plan == "" {
			continue
		}
		var t domain.Title
		if err := store.Get(ctx, w.s.Read(), "titles", id, &t); err != nil {
			return err
		}
		m := movieFromTitle(t)
		q := e.Quality
		if q == "" || e.Plan == planGone {
			q = "bluray1080"
		}
		name := movieRelease(m, q)
		var p payload
		switch e.Plan {
		case planGrab:
			p = payload{Files: []string{name + ".mkv", name + ".nfo"}}
		case planUpgrade:
			name = movieRelease(m, "bluray1080")
			p = payload{Files: []string{name + ".mkv"}}
		case planFailed:
			p = payload{Fail: true}
		case planDecide:
			p = payload{Files: []string{name + ".iso", name + ".nfo"}}
		case planDownloading, planQueued, planPaused, planStalled:
			st := map[string]string{planDownloading: downloader.StateDownloading, planQueued: downloader.StateQueued,
				planPaused: downloader.StatePaused, planStalled: downloader.StateDownloading}[e.Plan]
			p = payload{State: st, Progress: e.Progress, Speed: map[bool]int64{true: 6 << 20, false: 0}[e.Plan == planDownloading]}
		default:
			continue
		}
		slot, err := w.slotOf(ctx, id, "Main")
		if err != nil {
			w.skip("%s: no slot: %v", t.Name, err)
			continue
		}
		rel, err := w.releaseFor(ctx, []int64{slot}, name, 9*gb, "chosen", 40)
		if err != nil {
			return err
		}
		if err := w.chooseAuto(ctx, id, slot, rel, name, 6); err != nil {
			w.skip("%s: choose: %v", t.Name, err)
			continue
		}
		if e.Plan == planStalled {
			// Sent nine hours ago and never moved since.
			w.dm.Now = func() time.Time { return now.Add(-9 * time.Hour) }
		}
		if err := w.grab(ctx, slot, rel, name, p); err != nil {
			w.skip("%s: %v", t.Name, err)
		}
		w.dm.Now = nil
		if e.Plan == planStalled {
			stalled = append(stalled, id)
		}
		w.m.Steps[key] = true
		w.say("   %-48s %s", t.Name, e.Scenario)
	}
	if len(stalled) > 0 {
		w.dm.StallAfter, w.dm.ActAfter = time.Hour, 60*24*time.Hour
		if _, err := w.dm.SyncOnce(ctx); err != nil {
			w.say("   stall check: %v", err)
		}
		w.dm.StallAfter, w.dm.ActAfter = 0, 0
	}
	// Episodes and an album downloading now.
	for _, ep := range showcaseEpisodeDownloads {
		id := w.m.Titles["show:"+ep.TMDB]
		key := fmt.Sprintf("activity episode %s S%02dE%02d", ep.TMDB, ep.Season, ep.Episode)
		if id == 0 || w.m.Steps[key] {
			continue
		}
		var t domain.Title
		if err := store.Get(ctx, w.s.Read(), "titles", id, &t); err != nil {
			return err
		}
		slots, err := w.episodeSlots(ctx, id, ep.Season, ep.Episode)
		if err != nil || len(slots) == 0 {
			w.skip("%s S%02dE%02d: no wanted episode (%v)", t.Name, ep.Season, ep.Episode, err)
			w.m.Steps[key] = true
			continue
		}
		name := fmt.Sprintf("%s.S%02dE%02d.%s", dotted(t.Name), ep.Season, ep.Episode, qualityTags["web1080"])
		rel, err := w.releaseFor(ctx, slots, name, 3*gb, "chosen", 30)
		if err != nil {
			return err
		}
		if err := w.chooseAuto(ctx, id, slots[0], rel, name, 5); err != nil {
			w.skip("%s: choose: %v", name, err)
			continue
		}
		if err := w.grab(ctx, slots[0], rel, name, payload{State: ep.State, Progress: ep.Progress, Speed: 4 << 20}); err != nil {
			w.skip("%s: %v", name, err)
		}
		w.m.Steps[key] = true
		w.say("   %-48s S%02dE%02d %s %.0f%%", t.Name, ep.Season, ep.Episode, ep.State, ep.Progress*100)
	}
	for _, e := range showcaseArtistList {
		id := w.m.Titles["artist:"+e.MBID]
		key := "activity album " + e.MBID
		if id == 0 || !e.Downloading || w.m.Steps[key] {
			continue
		}
		w.m.Steps[key] = true
		var partID int64
		var album string
		if err := w.s.Read().QueryRowContext(ctx, `SELECT p.id, p.name FROM parts p WHERE p.title_id = ? AND p.kind = 'album' AND p.keep_up_to_date = 1
			AND EXISTS (SELECT 1 FROM items i WHERE i.part_id = p.id AND i.kind = 'track') ORDER BY p.release_date DESC LIMIT 1`, id).Scan(&partID, &album); err != nil {
			w.skip("%s: no kept album to download: %v", e.Name, err)
			continue
		}
		slots, err := w.albumSlots(ctx, partID)
		if err != nil || len(slots) == 0 {
			w.skip("%s %s: no wanted tracks (%v)", e.Name, album, err)
			continue
		}
		name := dotted(e.Name) + "-" + dotted(album) + "-WEB-FLAC-2024"
		rel, err := w.releaseFor(ctx, slots, name, gb/2, "chosen", 30)
		if err != nil {
			return err
		}
		if _, err := w.sr.Choose(ctx, rel, 0, true, w.by); err != nil {
			w.skip("%s: choose album: %v", name, err)
			continue
		}
		if err := w.grab(ctx, slots[0], rel, name, payload{State: downloader.StateDownloading, Progress: 0.71, Speed: 9 << 20}); err != nil {
			w.skip("%s: %v", name, err)
		}
		w.say("   %-48s %s downloading 71%%", e.Name, album)
	}
	for _, p := range w.eng.made {
		w.m.Paths = append(w.m.Paths, p)
	}
	w.eng.made = nil
	return nil
}

func (w *world) albumSlots(ctx context.Context, partID int64) ([]int64, error) {
	rows, err := w.s.Read().QueryContext(ctx, `SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id JOIN versions v ON v.id = s.version_id
		WHERE i.part_id = ? AND v.is_main = 1 AND s.state IN ('wanted', 'better_wanted') ORDER BY i.id`, partID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// showcaseAsks makes the Wishlist: asks waiting for approval (a member's
// and a Kid's), one approved (added and looked for), one turned down, a new
// season of a show already kept, and an artist.
func (w *world) showcaseAsks(ctx context.Context) error {
	owner, err := ownerPerson(ctx, w.s.Read())
	if err != nil {
		return err
	}
	svc := &asks.Service{Store: w.s, Library: w.lib, Metadata: w.lib.Metadata}
	defer svc.Wait()
	for _, e := range showcaseMovieList {
		key := "ask movie " + e.TMDB
		if e.Ask == "" || w.m.Steps[key] {
			continue
		}
		p, err := w.member(ctx, e.AskedBy)
		if err != nil {
			return fmt.Errorf("member %s: %w", e.AskedBy, err)
		}
		if old, _ := library.TitleByTMDB(ctx, w.s.Read(), e.TMDB); old != 0 {
			if err := w.claim(ctx, old); err != nil {
				return err
			}
		}
		res, err := svc.Ask(ctx, p, e.TMDB)
		if err != nil {
			w.skip("ask %s by %s: %v", e.Name, p.Name, err)
			w.m.Steps[key] = true
			continue
		}
		switch e.Ask {
		case "approved":
			lf, _ := w.folderOf(ctx, "movie")
			a, err := svc.Approve(ctx, res.Ask.ID, owner, asks.ApproveOptions{LibraryFolderID: lf.ID})
			if err != nil {
				w.skip("approve %s: %v", e.Name, err)
			} else if a.TitleID != nil {
				w.m.Titles["movie:"+e.TMDB] = *a.TitleID
			}
		case "turned_down":
			if _, err := svc.TurnDown(ctx, res.Ask.ID, owner, "Already on a streaming service we have"); err != nil {
				w.skip("turn down %s: %v", e.Name, err)
			}
		}
		w.m.Steps[key] = true
		w.say("   %-48s asked by %s, %s", e.Name, p.Name, e.Ask)
	}
	for _, a := range showcaseShowAsks {
		key := "ask show " + a.TMDB
		if w.m.Steps[key] {
			continue
		}
		p, err := w.member(ctx, a.AskedBy)
		if err != nil {
			return err
		}
		if _, err := svc.AskShow(ctx, p, "tmdb:"+a.TMDB, a.Seasons); err != nil {
			w.skip("ask show %s: %v", a.Name, err)
		} else {
			w.say("   %-48s seasons %v asked by %s", a.Name, a.Seasons, p.Name)
		}
		w.m.Steps[key] = true
	}
	for _, e := range showcaseArtistList {
		key := "ask artist " + e.MBID
		if !e.Ask || w.m.Steps[key] {
			continue
		}
		p, err := w.member(ctx, "Maya")
		if err != nil {
			return err
		}
		if old, _ := library.ArtistByMBID(ctx, w.s.Read(), e.MBID); old != 0 {
			if err := w.claim(ctx, old); err != nil {
				return err
			}
		}
		if _, err := svc.AskArtist(ctx, p, e.MBID, nil); err != nil {
			w.skip("ask artist %s: %v", e.Name, err)
		} else {
			w.say("   %-48s asked by %s", e.Name, p.Name)
		}
		w.m.Steps[key] = true
	}
	// Follows of artists now in the Library (the owner's and a member's), so
	// the "New from your artists" row has followed artists to draw on.
	for _, e := range showcaseArtistList {
		id, _ := library.ArtistByMBID(ctx, w.s.Read(), e.MBID) // the current Library title (adopt can replace the id)
		if id != 0 {
			w.m.Titles["artist:"+e.MBID] = id
		}
		for _, who := range e.Follow {
			key := "follow artist " + e.MBID + " " + who
			if id == 0 || w.m.Steps[key] {
				continue
			}
			p := owner
			if who != "owner" {
				if p, err = w.member(ctx, who); err != nil {
					return fmt.Errorf("member %s: %w", who, err)
				}
			}
			if err := svc.Follow(ctx, p, id); err != nil {
				w.skip("follow %s by %s: %v", e.Name, p.Name, err)
				continue
			}
			w.say("   %-48s followed by %s", e.Name, p.Name)
			w.m.Steps[key] = true
		}
	}
	return nil
}

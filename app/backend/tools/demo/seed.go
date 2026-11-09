package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// manifestKey is the setting (group this_app) where seed records what it
// added, so a second run adds nothing twice and -reset-demo removes only that.
const manifestKey = "demo_seed"

// manifest is what seed added.
type manifest struct {
	Titles      map[string]int64 `json:"titles"` // "movie:603", "show:1396", "artist:<mbid>" -> Title id
	Folders     []int64          `json:"library_folders"`
	Downloaders []int64          `json:"downloaders"`
	Releases    []int64          `json:"releases"`
	Paths       []string         `json:"paths"` // files and folders made, in the order made
	Steps       map[string]bool  `json:"steps"` // steps done (grabs, holds, clean-ups)
	Skipped     []string         `json:"skipped"`
}

func newManifest() *manifest {
	return &manifest{Titles: map[string]int64{}, Steps: map[string]bool{}}
}

func readManifest(ctx context.Context, db database.Querier) (*manifest, error) {
	var v string
	err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", manifestKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return newManifest(), nil
	}
	if err != nil {
		return nil, err
	}
	m := newManifest()
	if err := json.Unmarshal([]byte(v), m); err != nil {
		return nil, fmt.Errorf("the demo seed record is damaged: %w", err)
	}
	if m.Titles == nil {
		m.Titles = map[string]int64{}
	}
	if m.Steps == nil {
		m.Steps = map[string]bool{}
	}
	return m, nil
}

func (w *world) saveManifest(ctx context.Context) error {
	b, err := json.Marshal(w.m)
	if err != nil {
		return err
	}
	now := database.FormatTime(time.Now())
	return w.s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			manifestKey, string(b), now, `{"kind":"system"}`, now, `{"kind":"system"}`)
		return err
	})
}

// providers are the metadata sources; tests pass fakes.
type providers struct {
	Movies  metadata.Provider
	Shows   metadata.ShowSources
	Artists metadata.ArtistProvider
}

// world is everything a seed run uses: the app's own services on the
// stopped config folder.
type world struct {
	o     options
	s     *database.Store
	vault *secrets.Vault
	lib   *library.Service
	sr    *sources.Searcher
	dm    *downloads.Manager
	im    *importer.Importer
	eng   *fakeEngine
	m     *manifest
	out   io.Writer
	by    domain.Actor
	ns    mediafiles.NamingSettings
	// Downloads is the save folder (as the Downloader and HolyArrs see it).
	Downloads string
	// sabINI is the lab SABnzbd config file the key is read from ("" skips SABnzbd).
	sabINI string
	// finished are Downloads the fake Downloader finished in this run.
	finished []int64
	// stamps are the activity spans to move back in time; addSpan and
	// addSpanEnd bound the titles and Events of the adding steps.
	stamps     []stamp
	addSpan    *spanMark
	addSpanEnd spanMark
}

// spanMark is the highest Title and Event id at one moment.
type spanMark struct{ titles, events int64 }

func (w *world) spanNow(ctx context.Context) spanMark {
	var m spanMark
	_ = w.s.Read().QueryRowContext(ctx, "SELECT COALESCE(max(id), 0) FROM titles").Scan(&m.titles)
	_ = w.s.ReadEvents().QueryRowContext(ctx, "SELECT COALESCE(max(id), 0) FROM events").Scan(&m.events)
	return m
}

func (w *world) say(format string, a ...any) { fmt.Fprintf(w.out, format+"\n", a...) }

// openWorld opens the config folder with the app's store, vault and
// services. Nothing runs in the background: no Job queue, no searches.
func openWorld(ctx context.Context, o options, p providers, out io.Writer) (*world, error) {
	s, err := database.Open(o.ConfigDir, database.Options{})
	if err != nil {
		return nil, err
	}
	key, err := secrets.LoadKey(o.ConfigDir, os.Getenv(secrets.EnvKeyFile))
	if err != nil {
		s.Close()
		return nil, err
	}
	vault, err := secrets.Open(ctx, s, key)
	if err != nil {
		s.Close()
		return nil, err
	}
	m, err := readManifest(ctx, s.Read())
	if err != nil {
		s.Close()
		return nil, err
	}
	ns, err := mediafiles.ReadNaming(ctx, s.Read())
	if err != nil {
		s.Close()
		return nil, err
	}
	w := &world{o: o, s: s, vault: vault, m: m, out: out, by: domain.System, ns: ns, Downloads: o.downloads()}
	q := jobs.New(s, jobs.Options{}) // only queued into: the app runs these Jobs when it starts
	w.lib = &library.Service{Store: s, Queue: q, Metadata: p.Movies, Shows: p.Shows, Artists: p.Artists, SavePath: w.Downloads,
		Artwork: &library.ArtworkCache{Dir: filepath.Join(o.ConfigDir, "artwork"), Allowed: []string{metadata.ImageBase + "/"}}}
	w.sr = &sources.Searcher{Store: s, Secrets: vault, Queue: q}
	w.eng = newFakeEngine(w.Downloads)
	w.im = &importer.Importer{Store: s, NoSizeFloors: true} // placeholder files are a few KB
	w.dm = &downloads.Manager{Store: s, SavePath: w.Downloads, VanishGrace: time.Hour,
		Engine: func(domain.Downloader) (downloads.Engine, error) { return w.eng, nil },
		Link:   func(_ context.Context, rel domain.Release) (string, error) { return rel.LinkRef, nil },
		OnFinished: func(_ context.Context, id int64) {
			w.finished = append(w.finished, id)
		},
		// A failed download queues the search for the next Release, as
		// the app does; the app runs it when it starts.
		OnFailed: func(ctx context.Context, slotID int64) {
			if err := w.sr.QueueAfterFailure(ctx, slotID, w.by); err != nil {
				w.say("   search after a failed download: %v", err)
			}
		}}
	return w, nil
}

func (w *world) Close() { w.s.Close() }

func (o options) downloads() string {
	if o.DownloadsDir != "" {
		return o.DownloadsDir
	}
	return "/data/downloads/complete"
}

// realProviders are TMDB (token from HOLYARRS_TMDB_TOKEN_FILE, read by the
// app's own client and never printed) and MusicBrainz at one request a
// second with its answers cached, as the app runs them.
func realProviders(s *database.Store) (providers, error) {
	f := os.Getenv(metadata.EnvTokenFile)
	if f == "" {
		return providers{}, fmt.Errorf("%s is not set: the seed looks titles up on TMDB with the lab's token file", metadata.EnvTokenFile)
	}
	tmdb := &metadata.TMDB{Access: metadata.TokenFile(f)}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = outbound.PolicyDialer(outbound.PublicOnly).DialContext
	client := &http.Client{Transport: t}
	mb := &metadata.MusicBrainz{
		Client: func(context.Context) *http.Client { return client },
		BaseURL: func(ctx context.Context) string {
			d, _ := library.ReadMusicDefaults(ctx, s.Read())
			return d.MusicBrainzURL
		},
		TTL: func(ctx context.Context) time.Duration {
			d, _ := library.ReadMusicDefaults(ctx, s.Read())
			return time.Duration(d.CacheHours) * time.Hour
		},
		UserAgent: metadata.UserAgentFor("demo-seed"),
		Cache:     metadata.LookupCache{Store: s},
	}
	return providers{Movies: tmdb, Shows: metadata.ShowSources{"tmdb": tmdb, "tvmaze": &metadata.TVMaze{}}, Artists: mb}, nil
}

// seedCommand checks it is safe, then seeds or resets.
func seedCommand(ctx context.Context, o options, out io.Writer) error {
	if err := checkStopped(ctx, o.AppURL, o.Force); err != nil {
		return err
	}
	if o.InitLab {
		unlock, err := lockConfig(o.ConfigDir)
		if err != nil {
			return err
		}
		defer unlock()
		return initLab(o, out)
	}
	if err := checkLab(o); err != nil {
		return err
	}
	unlock, err := lockConfig(o.ConfigDir)
	if err != nil {
		return err
	}
	defer unlock()
	w, err := openWorld(ctx, o, providers{}, out)
	if err != nil {
		return err
	}
	defer w.Close()
	w.sabINI = o.SabINI
	if o.Reset {
		return w.reset(ctx)
	}
	rp, err := realProviders(w.s)
	if err != nil {
		return err
	}
	w.lib.Metadata, w.lib.Shows, w.lib.Artists = rp.Movies, rp.Shows, rp.Artists
	return w.seed(ctx)
}

// seed adds the demo library in steps; each step is skipped when an
// earlier run did it.
func (w *world) seed(ctx context.Context) error {
	start := time.Now()
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"clean-ups", w.cleanUps},
		{"library folders", w.folders},
		{"downloaders", w.downloaders},
		{"movies", w.movies},
		{"shows", w.shows},
		{"artists", w.artists},
		{"posters", w.posters},
		{"activity", w.activity},
		{"history times", w.historyTimes},
	}
	for _, st := range steps {
		if st.name == "movies" {
			sm := w.spanNow(ctx)
			w.addSpan = &sm
		}
		w.say("== %s", st.name)
		if err := st.fn(ctx); err != nil {
			_ = w.saveManifest(ctx)
			return fmt.Errorf("%s: %w", st.name, err)
		}
		if err := w.saveManifest(ctx); err != nil {
			return err
		}
		if st.name == "posters" {
			w.addSpanEnd = w.spanNow(ctx)
		}
	}
	res, err := w.lib.Rescan(ctx, w.by)
	if err != nil {
		return fmt.Errorf("rescan: %w", err)
	}
	w.say("== rescan: %d folders, %d missing, %d back, %d found, %d not reachable", res.Folders, res.Missing, res.Back, res.Found, res.Unreachable)
	w.say("== done in %s", time.Since(start).Round(time.Second))
	return w.summary(ctx)
}

// summary prints what the Library now holds.
func (w *world) summary(ctx context.Context) error {
	db := w.s.Read()
	for _, q := range []string{
		"SELECT 'titles ' || kind, count(*) FROM titles WHERE state != 'removed' GROUP BY kind",
		"SELECT 'slots ' || s.state, count(*) FROM slots s JOIN items i ON i.id = s.item_id JOIN titles t ON t.id = i.title_id WHERE t.state != 'removed' GROUP BY s.state",
		"SELECT 'releases ' || state, count(*) FROM releases GROUP BY state",
		"SELECT 'files ' || f.state, count(*) FROM files f JOIN versions v ON v.id = f.version_id JOIN titles t ON t.id = v.title_id WHERE t.state != 'removed' GROUP BY f.state",
		"SELECT 'posters', count(*) FROM titles WHERE state != 'removed' AND artwork LIKE '%poster:%'",
		"SELECT 'titles without poster: ' || group_concat(name, ', '), count(*) FROM titles WHERE kind != 'artist' AND state != 'removed' AND artwork NOT LIKE '%poster:%'",
	} {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k sql.NullString
			var n int
			if rows.Scan(&k, &n) == nil && n > 0 {
				w.say("   %-40s %d", k.String, n)
			}
		}
		rows.Close()
	}
	return nil
}

// cleanUps fixes lab configuration the diagnosis found wrong: a Library
// folder whose path is a Windows path mangled by Git Bash (it can never be
// reached from the container) is removed when no title uses it.
func (w *world) cleanUps(ctx context.Context) error {
	lfs, err := database.Select[mediafiles.LibraryFolder](ctx, w.s.Read(), "library_folders", "1=1 ORDER BY id")
	if err != nil {
		return err
	}
	for _, lf := range lfs {
		if library.FullPath(lf.Path) {
			continue
		}
		if err := library.DeleteFolder(ctx, w.s, lf.ID); errors.Is(err, library.ErrFolderInUse) {
			w.say("   kept Library folder %s: titles use it", lf.Path)
			continue
		} else if err != nil {
			return err
		}
		w.say("   removed Library folder %q: not a path inside the container (Git Bash turned /data/media/movies into a Windows path)", lf.Path)
		w.m.Steps["removed folder "+lf.Path] = true
	}
	return nil
}

// folders adds the show and music Library folders next to the movie one.
func (w *world) folders(ctx context.Context) error {
	dps, _ := library.DownloadPaths(ctx, w.s.Read(), w.Downloads)
	for _, f := range []library.FolderInput{
		{Path: filepath.Join(w.o.MediaRoot, "movies"), Kind: "movie", Label: "Movies"},
		{Path: filepath.Join(w.o.MediaRoot, "tv"), Kind: "show", Label: "TV"},
		{Path: filepath.Join(w.o.MediaRoot, "music"), Kind: "music", Label: "Music"},
	} {
		var n int
		if err := w.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM library_folders WHERE path = ?", f.Path).Scan(&n); err != nil {
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

// folderOf returns the first Library folder of a kind.
func (w *world) folderOf(ctx context.Context, kind string) (mediafiles.LibraryFolder, error) {
	fs, err := database.Select[mediafiles.LibraryFolder](ctx, w.s.Read(), "library_folders", "kind = ? ORDER BY id LIMIT 1", kind)
	if err != nil {
		return mediafiles.LibraryFolder{}, err
	}
	if len(fs) == 0 {
		return mediafiles.LibraryFolder{}, fmt.Errorf("no %s Library folder", kind)
	}
	return fs[0], nil
}

// downloaders adds the lab SABnzbd next to the lab qBittorrent, with its
// key read from the lab SABnzbd's own config file and kept in the vault.
func (w *world) downloaders(ctx context.Context) error {
	var n int
	if err := w.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM downloaders WHERE adapter = 'sabnzbd' AND removed = 0").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if w.sabINI == "" {
		w.say("   SABnzbd skipped: no -sabnzbd-ini (its key lives in the lab SABnzbd config)")
		w.m.Skipped = append(w.m.Skipped, "SABnzbd: no key file given")
		return nil
	}
	key, err := readSabKey(w.sabINI)
	if err != nil {
		w.say("   SABnzbd skipped: %v", err)
		w.m.Skipped = append(w.m.Skipped, "SABnzbd: "+err.Error())
		return nil
	}
	cat := "holyarrs"
	allow := true
	prio := 1
	d, err := downloads.Add(ctx, w.s, w.vault, downloads.Input{Name: "Lab SABnzbd", Adapter: "sabnzbd", URL: sabURL(), Secret: key,
		Category: &cat, Priority: &prio, AllowLocal: &allow}, w.by)
	if err != nil {
		return err
	}
	w.m.Downloaders = append(w.m.Downloaders, d.ID)
	w.say("   added Downloader Lab SABnzbd (%s), key kept in the vault", d.URL)
	return nil
}

// sabURL is the lab SABnzbd by its name on the lab network (the lab
// SABnzbd lists that name among the host names it accepts).
func sabURL() string { return "http://sabnzbd:8080" }

// readSabKey reads api_key from a SABnzbd ini file. The key is never shown.
func readSabKey(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("the SABnzbd config file cannot be read")
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok && strings.TrimSpace(k) == "api_key" {
			if v = strings.TrimSpace(v); v != "" {
				return v, nil
			}
		}
	}
	return "", errors.New("the SABnzbd config file has no api_key")
}

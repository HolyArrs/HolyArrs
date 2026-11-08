package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// sameName says whether a looked-up name is the expected one (letters and
// digits only, ignoring case; the expected name may be the start).
func sameName(got, want string) bool {
	norm := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	g, w := norm(got), norm(want)
	return g != "" && w != "" && (strings.HasPrefix(g, w) || strings.HasPrefix(w, g))
}

// dotted makes a release-style name: "The Matrix" -> "The.Matrix".
func dotted(name string) string {
	var b strings.Builder
	dot := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dot && b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteRune(r)
			dot = false
		case r == '\'' || r == '’':
		default:
			dot = true
		}
	}
	return b.String()
}

// movieRelease is a believable release name for a movie at a quality.
func movieRelease(m metadata.Movie, quality string) string {
	return dotted(m.Name) + "." + strconv.Itoa(m.Year) + "." + qualityTags[quality]
}

// placeholder writes a tiny stand-in video (a few KB) and records it. An
// existing file is left alone.
func (w *world) placeholder(p string, seed int) error {
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	dir := filepath.Dir(p)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		w.recordDirs(dir)
	}
	size := 2048 + (seed*7919)%7168
	body := []byte("HolyArrs demo placeholder: not a real video.\n")
	buf := make([]byte, size)
	copy(buf, body)
	if err := os.WriteFile(p, buf, 0o644); err != nil {
		return err
	}
	w.m.Paths = append(w.m.Paths, p)
	return nil
}

// recordDirs records a new folder and the new folders above it inside
// the media or downloads roots, outermost first.
func (w *world) recordDirs(dir string) {
	var made []string
	for d := dir; ; d = filepath.Dir(d) {
		if d == w.o.MediaRoot || d == w.Downloads || d == filepath.Dir(d) || inList(w.m.Paths, d) {
			break
		}
		made = append(made, d)
		if parent := filepath.Dir(d); parent == filepath.Join(w.o.MediaRoot, "movies") || parent == filepath.Join(w.o.MediaRoot, "tv") || parent == filepath.Join(w.o.MediaRoot, "shows") ||
			parent == filepath.Join(w.o.MediaRoot, "music") || parent == w.Downloads {
			break
		}
	}
	for i := len(made) - 1; i >= 0; i-- {
		w.m.Paths = append(w.m.Paths, made[i]+string(filepath.Separator))
	}
}

func inList(xs []string, x string) bool {
	for _, y := range xs {
		if strings.TrimSuffix(y, string(filepath.Separator)) == x {
			return true
		}
	}
	return false
}

func hasVideo(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			switch strings.ToLower(filepath.Ext(p)) {
			case ".mkv", ".mp4", ".avi", ".m4v", ".webm":
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}

// movies adds every catalog movie by TMDB id through the app's own add
// and adopt paths.
func (w *world) movies(ctx context.Context) error {
	lf, err := w.folderOf(ctx, "movie")
	if err != nil {
		return err
	}
	var fourK int64
	_ = w.s.Read().QueryRowContext(ctx, "SELECT id FROM pick_rules WHERE name = ?", mediafiles.FourKName).Scan(&fourK)
	for i, e := range demoMovies {
		key := "movie:" + e.TMDB
		if w.m.Titles[key] != 0 {
			continue
		}
		if id, err := library.TitleByTMDB(ctx, w.s.Read(), e.TMDB); err != nil {
			return err
		} else if id != 0 {
			w.say("   %s: already in the Library (not added by seed), left alone", e.Name)
			continue
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
		switch e.Plan {
		case planHave, planLow, planUpgrade:
			if id, err = w.adoptMovie(ctx, lf, m, e, i); err != nil {
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		default:
			if id, _, err = w.lib.Add(ctx, m, library.AddOptions{LibraryFolderID: lf.ID}, w.by); err != nil {
				return fmt.Errorf("%s: %w", m.Name, err)
			}
		}
		w.m.Titles[key] = id
		if e.FourK != "" && fourK != 0 {
			dps, _ := library.DownloadPaths(ctx, w.s.Read(), w.Downloads)
			if _, err := library.AddVersion(ctx, w.s, id, library.VersionInput{Label: "4K", PickRulesID: fourK}, dps, w.lib.Clock(), w.by); err != nil {
				return fmt.Errorf("%s 4K Version: %w", m.Name, err)
			}
		}
		w.say("   %-50s %s", m.Name+" ("+strconv.Itoa(m.Year)+")", e.Plan)
	}
	for _, name := range demoUpcoming {
		if err := w.upcoming(ctx, lf, name); err != nil {
			return err
		}
	}
	return nil
}

func (w *world) skip(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	w.say("   skipped: %s", msg)
	w.m.Skipped = append(w.m.Skipped, msg)
}

// adoptMovie makes the movie's folder with a placeholder file named by the
// app's naming pattern (or uses the lab's own folder when it is there) and
// adopts it, as Library > Adopt does.
func (w *world) adoptMovie(ctx context.Context, lf mediafiles.LibraryFolder, m metadata.Movie, e movieEntry, seed int) (int64, error) {
	release := movieRelease(m, e.Quality)
	p := releasename.Parse(release, releasename.Hints{})
	facts := mediafiles.FactsFrom(m.Name, m.Year, m.IDs, p, release, ".mkv")
	folder := mediafiles.Folder(w.ns, facts)
	dir := filepath.Join(lf.Path, folder)
	if !hasVideo(dir) {
		if err := w.placeholder(filepath.Join(dir, mediafiles.FileName(w.ns, facts)), seed); err != nil {
			return 0, err
		}
	}
	res, err := w.lib.Adopt(ctx, lf.ID, []library.AdoptChoice{{Folder: folder, TMDB: m.IDs["tmdb"]}}, w.by)
	if err != nil {
		return 0, err
	}
	if len(res) != 1 || res[0].Error != "" || res[0].TitleID == 0 {
		return 0, fmt.Errorf("adopt %s: %v", folder, res)
	}
	return res[0].TitleID, nil
}

// upcoming adds the newest movie of that name that is not out yet.
func (w *world) upcoming(ctx context.Context, lf mediafiles.LibraryFolder, name string) error {
	found, err := w.lib.Metadata.Search(ctx, name, 0)
	if err != nil {
		w.skip("upcoming %q: %v", name, err)
		return nil
	}
	now := time.Now()
	for _, c := range found {
		if !sameName(c.Name, name) || c.IDs["tmdb"] == "" || c.Year != 0 && c.Year < now.Year() {
			continue
		}
		key := "movie:" + c.IDs["tmdb"]
		if w.m.Titles[key] != 0 {
			return nil
		}
		if id, err := library.TitleByTMDB(ctx, w.s.Read(), c.IDs["tmdb"]); err != nil || id != 0 {
			return err
		}
		m, err := w.lib.Metadata.Movie(ctx, "tmdb:"+c.IDs["tmdb"])
		if err != nil {
			continue
		}
		out := false
		for _, d := range []*time.Time{m.CinemaDate, m.DigitalDate, m.PhysicalDate} {
			if d != nil && d.Before(now) {
				out = true
			}
		}
		if out {
			continue
		}
		id, _, err := w.lib.Add(ctx, m, library.AddOptions{LibraryFolderID: lf.ID}, w.by)
		if err != nil {
			return fmt.Errorf("%s: %w", m.Name, err)
		}
		w.m.Titles[key] = id
		when := "date not known yet"
		if m.CinemaDate != nil {
			when = "in cinemas " + m.CinemaDate.Format("2006-01-02")
		}
		w.say("   %-50s upcoming (%s)", m.Name, when)
		return nil
	}
	w.skip("upcoming %q: no match that is not out yet", name)
	return nil
}

// shows adds every catalog show by TMDB id; shows with files get a folder
// of placeholder episode files named by the app's naming pattern, adopted
// as the show is added.
func (w *world) shows(ctx context.Context) error {
	lf, err := w.folderOf(ctx, "show")
	if err != nil {
		return err
	}
	for i, e := range demoShows {
		key := "show:" + e.TMDB
		if w.m.Titles[key] != 0 {
			continue
		}
		sh, err := shows.New(w.lib).LookupShow(ctx, "tmdb:"+e.TMDB, "tmdb")
		if err != nil {
			w.skip("show %s (tmdb %s): %v", e.Name, e.TMDB, err)
			continue
		}
		if !sameName(sh.Name, e.Name) {
			w.skip("show tmdb %s is %q, not %q", e.TMDB, sh.Name, e.Name)
			continue
		}
		if id, err := library.ShowByIDs(ctx, w.s.Read(), sh.IDs); err != nil {
			return err
		} else if id != 0 {
			w.say("   %s: already in the Library (not added by seed), left alone", sh.Name)
			continue
		}
		folder := mediafiles.ShowFolder(w.ns, mediafiles.NameFacts{Title: sh.Name, Year: sh.Year, TMDB: sh.IDs["tmdb"], IMDB: sh.IDs["imdb"]})
		dir := filepath.Join(lf.Path, folder)
		files := 0
		if !hasVideo(dir) {
			for j, ep := range episodesWithFiles(sh, e, time.Now()) {
				q := e.Quality
				if ep.Season <= e.Low {
					q = "hdtv720"
				}
				release := fmt.Sprintf("%s.S%02dE%02d.%s", dotted(sh.Name), ep.Season, ep.Number, qualityTags[q])
				p := releasename.Parse(release, releasename.Hints{})
				facts := mediafiles.FactsFrom(sh.Name, sh.Year, sh.IDs, p, release, ".mkv")
				ef := mediafiles.EpisodeFacts{NameFacts: facts, Order: mediafiles.OrderAired, Season: ep.Season, Episodes: []int{ep.Number}, Titles: []string{ep.Name}}
				rel := mediafiles.EpisodePath(w.ns, folder, ef)
				if err := w.placeholder(filepath.Join(lf.Path, filepath.FromSlash(rel)), i*1000+j); err != nil {
					return err
				}
				files++
			}
		}
		opt := shows.ShowAddOptions{LibraryFolderID: lf.ID, Seasons: e.Choice, EpisodeOrder: e.Order, Source: "tmdb", NewSeasons: "all"}
		var a *shows.ShowAdoption
		if hasVideo(dir) {
			a = &shows.ShowAdoption{FolderName: folder}
			opt.Adopt, opt.WhyAdded = a, "import"
		}
		id, _, err := shows.New(w.lib).AddShow(ctx, "tmdb:"+e.TMDB, opt, w.by)
		if err != nil {
			return fmt.Errorf("%s: %w", sh.Name, err)
		}
		w.m.Titles[key] = id
		how := "no files"
		if a != nil {
			how = fmt.Sprintf("%d files adopted for %d episodes", a.Adopted, a.Episodes)
			if len(a.Unknown) > 0 {
				how += fmt.Sprintf(", %d left alone", len(a.Unknown))
			}
		}
		w.say("   %-50s %s, %s (%s)", sh.Name, e.Plan, how, path.Base(dir))
		_ = files
	}
	return nil
}

// episodesWithFiles picks the aired regular episodes that get a file.
func episodesWithFiles(sh metadata.Show, e showEntry, now time.Time) []metadata.Episode {
	var aired []metadata.Episode
	latest := 0
	for _, ep := range sh.Episodes {
		if ep.Season > latest {
			latest = ep.Season
		}
	}
	for _, ep := range sh.Episodes {
		if ep.Season > 0 && ep.Number > 0 && ep.AirDate != nil && ep.AirDate.Before(now) {
			aired = append(aired, ep)
		}
	}
	sort.SliceStable(aired, func(i, j int) bool {
		if aired[i].Season != aired[j].Season {
			return aired[i].Season < aired[j].Season
		}
		return aired[i].Number < aired[j].Number
	})
	var out []metadata.Episode
	switch e.Plan {
	case showComplete:
		out = aired
	case showAiring:
		for _, ep := range aired {
			if ep.Season == latest {
				out = append(out, ep)
			}
		}
		if len(out) > 1 {
			out = out[:len(out)-1] // the newest aired one is still looked for
		}
	default:
		next := 0
		for _, ep := range aired {
			switch {
			case ep.Season <= e.Seasons:
				out = append(out, ep)
			case ep.Season == e.Seasons+1 && next < e.Episodes:
				out = append(out, ep)
				next++
			}
		}
	}
	return out
}

// artists adds every catalog artist by MusicBrainz id with no albums kept,
// then keeps a handful of its studio albums spread over its career.
func (w *world) artists(ctx context.Context) error {
	if w.lib.Artists == nil {
		w.skip("artists: no MusicBrainz")
		return nil
	}
	for _, e := range demoArtists {
		key := "artist:" + e.MBID
		if w.m.Titles[key] != 0 {
			continue
		}
		if id, err := library.ArtistByMBID(ctx, w.s.Read(), e.MBID); err != nil {
			return err
		} else if id != 0 {
			w.say("   %s: already in the Library (not added by seed), left alone", e.Name)
			continue
		}
		found, err := w.lib.LookupArtists(ctx, "mbid:"+e.MBID)
		if err != nil || len(found) == 0 {
			w.skip("artist %s: %v", e.Name, err)
			continue
		}
		if !sameName(found[0].Name, e.Name) {
			w.skip("artist %s is %q, not %q", e.MBID, found[0].Name, e.Name)
			continue
		}
		id, _, err := w.lib.AddArtist(ctx, e.MBID, library.ArtistAddOptions{Albums: "none", NewAlbums: "new"}, w.by)
		if err != nil {
			return fmt.Errorf("%s: %w", e.Name, err)
		}
		w.m.Titles[key] = id
		parts, err := database.Select[mediafiles.Part](ctx, w.s.Read(), "parts",
			"title_id = ? AND kind = 'album' AND included = 1 AND release_type = 'album' AND release_date IS NOT NULL AND release_date < ? ORDER BY release_date",
			id, database.FormatTime(time.Now()))
		if err != nil {
			return err
		}
		kept := 0
		for _, i := range spread(len(parts), e.Albums) {
			keep := true
			if _, err := w.lib.SetAlbum(ctx, parts[i].ID, &keep, 0, w.by); err != nil {
				w.say("   %s, album %s: %v", e.Name, parts[i].Name, err)
				continue
			}
			kept++
		}
		// The refresh reads the kept albums' Pressings and tracks.
		if _, err := w.lib.RefreshArtist(ctx, id, w.by, nil); err != nil {
			w.say("   %s: refresh: %v", e.Name, err)
		}
		var tracks int
		_ = w.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM items WHERE title_id = ? AND kind = 'track' AND keep_up_to_date = 1", id).Scan(&tracks)
		w.say("   %-50s %d of %d albums kept, %d tracks wanted", found[0].Name, kept, len(parts), tracks)
	}
	return nil
}

// spread picks up to k indexes out of n, first and last included, evenly.
func spread(n, k int) []int {
	if n <= 0 || k <= 0 {
		return nil
	}
	if k >= n {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	if k == 1 {
		return []int{n - 1}
	}
	seen := map[int]bool{}
	var out []int
	for i := 0; i < k; i++ {
		x := i * (n - 1) / (k - 1)
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// posters refreshes every movie and show without a cached poster, which
// fills the artwork cache through the app's own refresh.
func (w *world) posters(ctx context.Context) error {
	ts, err := database.Select[mediafiles.Title](ctx, w.s.Read(), "titles", "kind IN ('movie', 'show') AND state != 'removed' AND artwork NOT LIKE '%poster:%' ORDER BY id")
	if err != nil {
		return err
	}
	for _, t := range ts {
		var err error
		if t.Kind == "movie" {
			_, err = w.lib.RefreshTitle(ctx, t.ID, w.by, nil)
		} else {
			_, err = shows.New(w.lib).RefreshShow(ctx, t.ID, w.by, nil)
		}
		if err != nil {
			w.say("   refresh %s: %v", t.Name, err)
		}
	}
	var n, all int
	_ = w.s.Read().QueryRowContext(ctx, "SELECT count(*), sum(artwork LIKE '%poster:%') FROM titles WHERE kind IN ('movie', 'show') AND state != 'removed'").Scan(&all, &n)
	w.say("   %d of %d movies and shows have a poster in the artwork cache", n, all)
	return nil
}

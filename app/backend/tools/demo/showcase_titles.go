package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// showcaseMovies adds every movie scenario.
func (w *world) showcaseMovies(ctx context.Context) error {
	lf, err := w.folderOf(ctx, "movie")
	if err != nil {
		return err
	}
	var fourK int64
	_ = w.s.Read().QueryRowContext(ctx, "SELECT id FROM pick_rules WHERE name = ?", mediafiles.FourKName).Scan(&fourK)
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
		sh, err := shows.New(w.lib).LookupShow(ctx, "tmdb:"+e.TMDB, "tmdb")
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
		folder := mediafiles.ShowFolder(w.ns, mediafiles.NameFacts{Title: sh.Name, Year: sh.Year, TMDB: sh.IDs["tmdb"], IMDB: sh.IDs["imdb"]})
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
				facts := mediafiles.FactsFrom(sh.Name, sh.Year, sh.IDs, parseName(release), release, ".mkv")
				ef := mediafiles.EpisodeFacts{NameFacts: facts, Order: mediafiles.OrderAired, Season: ep.Season, Episodes: []int{ep.Number}, Titles: []string{ep.Name}}
				rel := mediafiles.EpisodePath(w.ns, folder, ef)
				if err := w.placeholder(filepath.Join(lf.Path, filepath.FromSlash(rel)), i*1000+j); err != nil {
					return err
				}
			}
		}
		opt := shows.ShowAddOptions{LibraryFolderID: lf.ID, Seasons: e.Choice, EpisodeOrder: e.Order, Source: "tmdb", NewSeasons: "all"}
		if e.Keep == "off" {
			off := false
			opt.KeepUpToDate = &off
		}
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
		if e.Specials > 0 {
			if err := shows.New(w.lib).SetSeason(ctx, id, 0, true, w.by); err != nil {
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
		_ = w.s.Read().QueryRowContext(ctx, "SELECT count(*) FROM parts WHERE title_id = ? AND kind = 'album' AND release_date > ?", id, database.FormatTime(time.Now())).Scan(&future)
		w.say("   %-48s %s (%d albums kept, %d not out yet)", e.Name, e.Scenario, kept, future)
	}
	return nil
}

// keepAlbums keeps up to n studio albums spread over the career, or with
// allBut every studio album except the newest allBut.
func (w *world) keepAlbums(ctx context.Context, id int64, n, allBut int) int {
	parts, err := database.Select[mediafiles.Part](ctx, w.s.Read(), "parts",
		"title_id = ? AND kind = 'album' AND included = 1 AND release_type = 'album' AND release_date IS NOT NULL AND release_date < ? ORDER BY release_date",
		id, database.FormatTime(time.Now()))
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
func (w *world) adoptArtist(ctx context.Context, lf mediafiles.LibraryFolder, id int64, e showcaseArtist) (int64, error) {
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
		id, database.FormatTime(time.Now())).Scan(&n, &had)
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

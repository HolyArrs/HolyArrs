package shows

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// maxShowVideos is the most videos one show folder scan reads.
const maxShowVideos = 5000

var reShowTag = regexp.MustCompile(`(?i)[\[{(]?(tvdb|tmdb|tvmaze|imdb)(?:id)?[-_=: ]?(tt\d+|\d+)[\]})]?`)

// showFolderName reads "Name (Year)", a bare name or a release-style name
// from a show folder name.
func showFolderName(s string) (string, int) {
	if library.ReFolderYear.MatchString(s) {
		return library.FolderName(s)
	}
	p := releasename.Parse(s, releasename.Hints{Show: true})
	if p.IsShow() && strings.TrimSpace(p.Title.Value) != "" {
		return p.Title.Value, p.Year.Value
	}
	return strings.Trim(s, " -._"), 0
}

// showID is the id a show answer is looked up by: "<source>:<id>".
func showID(sh metadata.Show) string {
	if sh.IDs[sh.Source] != "" {
		return sh.Source + ":" + sh.IDs[sh.Source]
	}
	for _, k := range idOrder {
		if sh.IDs[k] != "" {
			return k + ":" + sh.IDs[k]
		}
	}
	return ""
}

// matchShowFolder looks a show folder up at the details source.
func matchShowFolder(ctx context.Context, p metadata.ShowProvider, folder string) (string, int, []library.AdoptMatch, error) {
	if m := reShowTag.FindStringSubmatch(folder); m != nil {
		sh, err := p.Show(ctx, strings.ToLower(m[1])+":"+m[2])
		if err == nil {
			return sh.Name, sh.Year, []library.AdoptMatch{{ID: showID(sh), TMDB: sh.IDs["tmdb"], Name: sh.Name, Year: sh.Year, Score: 1}}, nil
		}
		if !errors.Is(err, metadata.ErrShowNotFound) {
			return "", 0, nil, err
		}
	}
	name, year := showFolderName(strings.TrimSpace(reShowTag.ReplaceAllString(folder, "")))
	shs, err := p.SearchShows(ctx, name)
	if err != nil {
		return name, year, nil, err
	}
	var out []library.AdoptMatch
	for _, sh := range shs {
		id := showID(sh)
		if id == "" {
			continue
		}
		out = append(out, library.AdoptMatch{ID: id, TMDB: sh.IDs["tmdb"], Name: sh.Name, Year: sh.Year, Score: library.NameScore(name, year, sh.Name, sh.Year)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > 4 {
		out = out[:4]
	}
	return name, year, out, nil
}

// mapEpisodeFiles matches each video of a show folder to its episodes by
// name (S01E02, multi-episode names, an air day or a running number). A
// file naming another show, no episode, or an episode a bigger file holds
// too, is listed with the reason and left alone.
func mapEpisodeFiles(vids []library.AdoptVideo, sh metadata.Show, order string) []library.AdoptEpisodeFile {
	refs := make([]mediafiles.EpisodeRef, len(sh.Episodes))
	for i, e := range sh.Episodes {
		refs[i] = mediafiles.EpisodeRef{ItemID: int64(i + 1), Season: e.Season, Episode: e.Number, Absolute: e.Absolute, AirDay: e.AirDay}
	}
	lib := []mediafiles.TitleRef{{Name: sh.Name, Aliases: sh.OtherNames, Year: sh.Year, Kind: "show"}}
	taken := map[[2]int]bool{}
	out := make([]library.AdoptEpisodeFile, 0, len(vids))
	for _, v := range vids { // biggest first
		f := library.AdoptEpisodeFile{Path: v.Path, Size: v.Size, Episodes: []string{}}
		p := releasename.Parse(path.Base(v.Path), releasename.Hints{Size: v.Size, Show: true})
		switch {
		case !p.IsShow():
			f.Why = "the file name gives no episode numbers"
		case mediafiles.IsPack(p):
			f.Why = "the file name names whole seasons, not episodes"
		default:
			if strings.TrimSpace(p.Title.Value) != "" {
				if m := mediafiles.MatchTitle(p, lib); m.Title == nil {
					f.Why = "the file name names another show (" + p.Title.Value + ")"
					break
				}
			}
			found, why := mediafiles.MatchEpisodes(p, order, refs)
			if len(found) == 0 {
				f.Why = why
				break
			}
			clash := false
			for _, r := range found {
				clash = clash || taken[[2]int{r.Season, r.Episode}]
			}
			if clash {
				f.Why = "a bigger file in this folder holds the same episode"
				break
			}
			for _, r := range found {
				taken[[2]int{r.Season, r.Episode}] = true
				f.Keys = append(f.Keys, [2]int{r.Season, r.Episode})
				f.Episodes = append(f.Episodes, r.Label())
			}
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// PlanShowAdoptFrom scans a show Library folder: each folder in it is
// matched to a show at the details source with a confidence score, and
// its videos to the episodes of the best match. Nothing is written.
func PlanShowAdoptFrom(ctx context.Context, db library.Reader, shows metadata.ShowSources, folderID int64, after string) (library.AdoptPlan, error) {
	var lf mediafiles.LibraryFolder
	if err := database.Get(ctx, db, "library_folders", folderID, &lf); err != nil {
		return library.AdoptPlan{}, library.ErrNoFolder
	}
	if lf.Kind != "show" {
		return library.AdoptPlan{}, library.ErrNotShowFolder
	}
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return library.AdoptPlan{}, err
	}
	defs, err := library.ReadShowDefaults(ctx, db)
	if err != nil {
		return library.AdoptPlan{}, err
	}
	p := library.AdoptPlan{Kind: "show", LibraryFolderID: lf.ID, Path: lf.Path, Folders: []library.AdoptCandidate{}, Counts: map[string]int{}, Limit: ns.AdoptScanLimit}
	if shows == nil {
		return p, library.ErrNoShowSources
	}
	src, err := shows.Get(defs.Source)
	if err != nil {
		return p, err
	}
	ents, err := os.ReadDir(lf.Path)
	if err != nil {
		return p, fmt.Errorf("the Library folder cannot be read: %w", err)
	}
	known := map[string]int64{}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "library_folder_id = ? AND title_id IN (SELECT id FROM titles WHERE state != 'removed')", lf.ID)
	if err != nil {
		return p, err
	}
	fold := library.FoldsCase(lf.Path)
	for _, v := range vs {
		known[library.FolderKey(v.FolderName, fold)] = v.TitleID
	}
	p.Warning = library.MountWarning(ctx, db, lf, len(ents))
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || library.NasSystemDir(e.Name()) || (after != "" && e.Name() <= after) {
			continue
		}
		if len(p.Folders) >= p.Limit {
			p.More, p.Next = true, p.Folders[len(p.Folders)-1].Folder
			break
		}
		vids := library.ListVideosN(lf.Path, e.Name(), maxShowVideos)
		c := library.AdoptCandidate{Folder: e.Name(), Videos: vids, Others: []library.AdoptMatch{}}
		if id, ok := known[library.FolderKey(e.Name(), fold)]; ok {
			c.Status, c.Why, c.TitleID = "in_library", "this folder already belongs to a show in the Library", id
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		name, year, ms, err := matchShowFolder(ctx, src, e.Name())
		c.ParsedName, c.ParsedYear = name, year
		if err != nil {
			if ctx.Err() != nil {
				return p, ctx.Err()
			}
			c.Status, c.Why = "unmatched", "the show lookup failed ("+err.Error()+"); look it up by hand or scan again later"
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		if len(ms) > 0 && ms[0].Score >= library.MatchUnsure {
			c.Match = &ms[0]
			c.Others = append(c.Others, ms[1:]...)
		} else {
			c.Others = append(c.Others, ms...)
		}
		if c.Match == nil {
			c.Status, c.Why = "unmatched", "no show at the details source fits this folder name; pick one by hand"
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		sh, err := src.Show(ctx, c.Match.ID)
		if err != nil {
			if ctx.Err() != nil {
				return p, ctx.Err()
			}
			c.Status, c.Why = "unmatched", "the episode list could not be read ("+err.Error()+"); scan again later"
			p.Folders = append(p.Folders, c)
			p.Counts[c.Status]++
			continue
		}
		ids := sh.IDs
		if ids == nil {
			ids = map[string]string{}
		}
		inLib, err := library.ShowByIDs(ctx, db, ids)
		if err != nil {
			return p, err
		}
		never, err := library.ShowOnNeverAdd(ctx, db, ids)
		if err != nil {
			return p, err
		}
		c.Files = mapEpisodeFiles(vids, sh, defs.EpisodeOrder)
		for _, f := range c.Files {
			if len(f.Episodes) > 0 {
				c.Episodes += len(f.Episodes)
			} else {
				c.Unknown++
			}
		}
		close := len(ms) > 1 && ms[1].Score > c.Match.Score-0.1
		label := c.Match.Name
		if c.Match.Year > 0 {
			label += fmt.Sprintf(" (%d)", c.Match.Year)
		}
		switch {
		case inLib != 0:
			c.Status, c.Why, c.TitleID = "in_library", "this show is already in the Library in another folder", inLib
		case never:
			c.Status, c.Why = "never_add", "this show is on the Never-add list"
		case len(vids) == 0:
			c.Status, c.Why = "no_video", "no video file in the folder; adopting adds the show with every episode missing"
		case c.Match.Score < library.MatchSure || close:
			c.Status, c.Why = "unsure", fmt.Sprintf("best match %s at %.0f%%; check it", label, c.Match.Score*100)
		default:
			c.Status, c.Why = "matched", fmt.Sprintf("%s at %.0f%%: %d episodes in %d files, %d files with no episode", label, c.Match.Score*100,
				c.Episodes, len(c.Files)-c.Unknown, c.Unknown)
		}
		p.Folders = append(p.Folders, c)
		p.Counts[c.Status]++
	}
	return p, nil
}

// ShowAdoption is a show folder found on disk that the person confirmed:
// the folder is kept as it is and its videos become adopted Files of the
// episodes their names give.
type ShowAdoption struct {
	FolderName  string
	LibraryPath string
	// Filled in by AddShow: the files adopted and the ones left alone.
	Adopted  int
	Episodes int
	Unknown  []string
}

// adoptEpisodesTx records the mapped files of a show folder as adopted
// Files of the new show's main Version: each file fills the Slots of the
// episodes it holds (in the Library, or looking for a better copy when it
// is below good enough and kept up to date).
func adoptEpisodesTx(ctx context.Context, tx *sql.Tx, titleID, versionID, pickRulesID int64, keep bool, files []library.AdoptEpisodeFile, a *ShowAdoption, now time.Time, by domain.Actor) error {
	var pr mediafiles.PickRules
	if err := database.Get(ctx, tx, "pick_rules", pickRulesID, &pr); err != nil {
		return err
	}
	for _, af := range files {
		if len(af.Keys) == 0 {
			a.Unknown = append(a.Unknown, af.Path+": "+af.Why)
			continue
		}
		var slotIDs []int64
		for _, k := range af.Keys {
			var sid int64
			err := tx.QueryRow(`SELECT s.id FROM slots s JOIN items i ON i.id = s.item_id WHERE i.title_id = ? AND s.version_id = ?
				AND json_extract(i.numbers, '$.season') = ? AND json_extract(i.numbers, '$.episode') = ?`, titleID, versionID, k[0], k[1]).Scan(&sid)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			slotIDs = append(slotIDs, sid)
		}
		if len(slotIDs) == 0 {
			a.Unknown = append(a.Unknown, af.Path+": its episodes are not in the show's episode list")
			continue
		}
		ext := path.Ext(af.Path)
		release := strings.TrimSuffix(path.Base(af.Path), ext)
		p := releasename.Parse(release, releasename.Hints{Size: af.Size, Show: true})
		info := map[string]any{"step": mediafiles.StepFor(p)}
		if inspected, ok := library.InspectLibraryVideo(a.LibraryPath, af.Path, info, false); ok {
			info = inspected
		}
		f := &mediafiles.File{SlotIDs: slotIDs, VersionID: versionID, RelativePath: af.Path, Size: af.Size, AddedAt: now,
			Quality:   mediafiles.Quality{Resolution: p.Resolution.Value, Source: p.Source.Value, Codec: p.Codec.Value, HDR: p.HDR.Value},
			Languages: library.InspectedLanguages(info), MediaInfo: info, ReleaseName: &release, Origin: "adopted",
			PlacedBy: "none", Role: "main", State: "present"}
		if f.Languages == nil {
			f.Languages = []string{}
		}
		if g := p.Group.Value; g != "" {
			f.Group = &g
		}
		fileID, err := database.Insert(tx, "files", f, by)
		if err != nil {
			return err
		}
		ge := mediafiles.ReachesGoodEnough(release, pr.Body)
		state, why := mediafiles.SlotInLibrary, "adopted from "+a.FolderName
		if !ge && keep {
			state, why = mediafiles.SlotBetterWanted, "adopted below good enough, so a better copy is sought"
		}
		for _, sid := range slotIDs {
			// The Slots were written moments ago in this same write, so they
			// take the adopted state as a new movie's Slot does.
			if _, err := tx.Exec(`UPDATE slots SET file_id = ?, good_enough = ?, state = ?, last_reason = ?, held_release_id = NULL WHERE id = ? AND state != 'removed'`,
				fileID, ge, string(state), why, sid); err != nil {
				return err
			}
		}
		a.Adopted++
		a.Episodes += len(slotIDs)
	}
	return nil
}

// adoptShowOne adds one show folder as a show with its folder and files as
// they are.
func (s *Service) adoptShowOne(ctx context.Context, lf mediafiles.LibraryFolder, c library.AdoptChoice, by domain.Actor) (library.AdoptResult, error) {
	r := library.AdoptResult{Folder: c.Folder}
	if c.Folder == "" || strings.ContainsAny(c.Folder, `/\`) || !filepath.IsLocal(c.Folder) || strings.HasPrefix(c.Folder, ".") {
		return r, errors.New("the folder must be one folder directly in the Library folder")
	}
	if fi, err := os.Lstat(filepath.Join(lf.Path, c.Folder)); err != nil || !fi.IsDir() {
		return r, errors.New("no such folder in the Library folder")
	}
	if taken, err := library.FolderTaken(ctx, s.Store.Read(), lf, c.Folder); err != nil {
		return r, err
	} else if taken {
		return r, errors.New("this folder already belongs to a show in the Library")
	}
	a := &ShowAdoption{FolderName: c.Folder, LibraryPath: lf.Path}
	id, _, err := s.AddShow(ctx, c.ID, ShowAddOptions{LibraryFolderID: lf.ID, WhyAdded: "import", Adopt: a}, by)
	if err != nil {
		return r, err
	}
	r.TitleID, r.Episodes, r.Unknown = id, a.Episodes, a.Unknown
	var name string
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT name FROM titles WHERE id = ?", id).Scan(&name)
	summary := fmt.Sprintf("Adopted %s from %s: %d episodes in %d files", name, c.Folder, a.Episodes, a.Adopted)
	if len(a.Unknown) > 0 {
		summary += fmt.Sprintf(", %d files left alone", len(a.Unknown))
	}
	s.Emit(ctx, &domain.Event{Actor: by, Kind: domain.EventAdopted, TitleID: &id, Summary: summary,
		Data:     map[string]any{"library_folder_id": lf.ID, "folder": c.Folder, "id": c.ID, "files": a.Adopted, "episodes": a.Episodes, "left_alone": a.Unknown},
		GroupKey: library.GroupKey(id)})
	return r, nil
}

// recordFoundEpisodes records unknown videos in a show's folder as found
// Files with the episodes their names give: shown, never renamed, moved
// or deleted, and never filling a Slot.
func (s *Service) recordFoundEpisodes(ctx context.Context, lf mediafiles.LibraryFolder, v mediafiles.Version, known map[string]bool, by domain.Actor) (int, error) {
	db := s.Store.Read()
	var t mediafiles.Title
	if err := database.Get(ctx, db, "titles", v.TitleID, &t); err != nil {
		return 0, err
	}
	items, err := database.Select[mediafiles.Item](ctx, db, "items", "title_id = ? AND kind = 'episode' ORDER BY id", t.ID)
	if err != nil {
		return 0, err
	}
	sh := metadata.Show{Name: t.Name, OtherNames: t.Aliases}
	if t.Year != nil {
		sh.Year = *t.Year
	}
	byKey := map[[2]int]int64{}
	for _, it := range items {
		if it.Numbers.Season == nil || it.Numbers.Episode == nil {
			continue
		}
		e := metadata.Episode{Season: *it.Numbers.Season, Number: *it.Numbers.Episode}
		if it.Numbers.Absolute != nil {
			e.Absolute = *it.Numbers.Absolute
		}
		if it.AirDay != nil {
			e.AirDay = *it.AirDay
		}
		sh.Episodes = append(sh.Episodes, e)
		byKey[[2]int{e.Season, e.Number}] = it.ID
	}
	order := mediafiles.OrderAired
	if t.EpisodeOrder != nil && *t.EpisodeOrder != "" {
		order = *t.EpisodeOrder
	}
	var unknown []library.AdoptVideo
	for _, vid := range library.ListVideosN(lf.Path, v.FolderName, maxShowVideos) {
		if !known[strings.ToLower(vid.Path)] {
			unknown = append(unknown, vid)
		}
	}
	n := 0
	for _, af := range mapEpisodeFiles(unknown, sh, order) {
		var exists int
		_ = db.QueryRowContext(ctx, "SELECT count(*) FROM files WHERE version_id = ? AND relative_path = ?", v.ID, af.Path).Scan(&exists)
		if exists > 0 {
			continue
		}
		slotIDs := []int64{}
		for _, k := range af.Keys {
			var sid int64
			if db.QueryRowContext(ctx, "SELECT id FROM slots WHERE version_id = ? AND item_id = ?", v.ID, byKey[k]).Scan(&sid) == nil {
				slotIDs = append(slotIDs, sid)
			}
		}
		release := strings.TrimSuffix(path.Base(af.Path), path.Ext(af.Path))
		info, _ := library.InspectLibraryVideo(lf.Path, af.Path, nil, false)
		if info == nil {
			info = map[string]any{}
		}
		f := &mediafiles.File{SlotIDs: slotIDs, VersionID: v.ID, RelativePath: af.Path, Size: af.Size, AddedAt: s.Clock(), Languages: []string{},
			MediaInfo: info, ReleaseName: &release, Origin: "found", PlacedBy: "none", Role: "main", State: "present"}
		f.Languages = library.InspectedLanguages(info)
		if err := s.Store.Write(ctx, func(tx *sql.Tx) error { _, err := database.Insert(tx, "files", f, by); return err }); err != nil {
			return n, err
		}
		n++
		known[strings.ToLower(af.Path)] = true
	}
	return n, nil
}

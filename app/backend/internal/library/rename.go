package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// KindRename is the Job that renames Files to the naming pattern.
const KindRename = "rename_files"

// reader reads rows and single rows (the store's read pool fits).
type Reader interface {
	database.Lister
	database.Querier
}

// RenameStep is one File in a rename. From and To are relative to the
// Library folder. A step with Skip set is shown but never carried out.
type RenameStep struct {
	FileID          int64  `json:"file_id"`
	TitleID         int64  `json:"title_id"`
	TitleName       string `json:"title_name"`
	VersionID       int64  `json:"version_id"`
	LibraryFolderID int64  `json:"library_folder_id"`
	Origin          string `json:"origin"`
	From            string `json:"from"`
	To              string `json:"to,omitempty"`
	Skip            string `json:"skip,omitempty"`
	// AlbumID is the album of a track file, so an album page can show its
	// own part of the artist's rename.
	AlbumID int64 `json:"album_id,omitempty"`
	// Result is what the rename did with this step: renamed, or failed
	// and why.
	Result string `json:"result,omitempty"`
	// Subtitles are the subtitle files beside the video
	// (name.lang[.hi|.forced].ext); they follow it to its new name.
	Subtitles []SubtitleMove `json:"subtitles,omitempty"`
	root      string
}

// SubtitleMove is one subtitle file that follows its video in a rename.
// From and To are relative to the Library folder; a move with Skip set
// is shown but never carried out (the video still moves).
type SubtitleMove struct {
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
	Skip   string `json:"skip,omitempty"`
	Result string `json:"result,omitempty"`
	// pair marks the .sub of a VobSub pair: it goes with the .idx just
	// before it, or not at all.
	pair bool
}

// besideSubtitles lists the subtitle files next to a video (relative to
// the Library folder), in name order, as groups that move together. Only
// plain files count. A picture (VobSub) .sub is a pair with the .idx of
// the same name, listed .idx first; a .sub with no .idx goes alone only
// when it is clearly text, else it is left where it is.
func besideSubtitles(root, rel string) [][]string {
	dir := filepath.Join(root, filepath.FromSlash(path.Dir(rel)))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	base := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	idx := map[string]string{} // lower-case stem of each .idx: its name
	for _, e := range ents {
		if n := e.Name(); e.Type().IsRegular() && strings.EqualFold(path.Ext(n), ".idx") {
			idx[strings.ToLower(strings.TrimSuffix(n, path.Ext(n)))] = n
		}
	}
	var out [][]string
	for _, e := range ents {
		n := e.Name()
		if !e.Type().IsRegular() || !subtitleNames().IsBeside(base, n) {
			continue
		}
		one := path.Join(path.Dir(rel), n)
		if strings.EqualFold(path.Ext(n), ".sub") {
			if partner, ok := idx[strings.ToLower(strings.TrimSuffix(n, path.Ext(n)))]; ok {
				out = append(out, []string{path.Join(path.Dir(rel), partner), one})
				continue
			}
			if !textSub(filepath.Join(dir, n)) {
				continue
			}
		}
		out = append(out, []string{one})
	}
	return out
}

// textSub reads the head of a .sub file and says whether it is clearly text.
func textSub(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := io.ReadFull(f, head)
	return subtitleNames().TextSub(head[:n])
}

// planSubtitles works out where each subtitle beside a renamed video goes:
// the video's new name, then the same language and tags. The files of a
// pair are skipped together when either cannot move.
func planSubtitles(root, from, to string, taken map[string]bool) []SubtitleMove {
	oldBase := strings.TrimSuffix(path.Base(from), path.Ext(from))
	newBase := strings.TrimSuffix(path.Base(to), path.Ext(to))
	var out []SubtitleMove
	for _, group := range besideSubtitles(root, from) {
		ms := make([]SubtitleMove, len(group))
		skip := ""
		for i, sub := range group {
			ms[i] = SubtitleMove{From: sub, pair: i > 0}
			nt := path.Join(path.Dir(to), newBase+path.Base(sub)[len(oldBase):])
			fromAbs := filepath.Join(root, filepath.FromSlash(sub))
			toAbs := filepath.Join(root, filepath.FromSlash(nt))
			switch {
			case !filepath.IsLocal(filepath.FromSlash(nt)):
				ms[i].Skip = "the name would leave the Library folder"
			case taken[strings.ToLower(nt)]:
				ms[i].Skip = "another file in this rename gets the same name"
			case importer.PathTooLong(toAbs) != "":
				ms[i].Skip = importer.PathTooLong(toAbs)
			default:
				if other, err := os.Stat(toAbs); err == nil {
					if self, _ := os.Stat(fromAbs); !os.SameFile(self, other) {
						ms[i].Skip = "a file named " + path.Base(nt) + " is already there"
					}
				}
			}
			ms[i].To = nt
			if skip == "" {
				skip = ms[i].Skip
			}
		}
		for i := range ms {
			if skip != "" {
				if ms[i].Skip == "" {
					ms[i].Skip = "its other half cannot move: " + skip
				}
				ms[i].To = ""
				continue
			}
			taken[strings.ToLower(ms[i].To)] = true
		}
		out = append(out, ms...)
	}
	return out
}

// renameCrash lets tests stop a rename at a stage ("before_move",
// "after_move") as if the process died there.
var renameCrash func(stage string) error

// fileFacts are the naming facts for a File already in the Library: the
// same ones its import used (the release name and its quality step).
func fileFacts(t mediafiles.Title, f mediafiles.File) mediafiles.NameFacts {
	release := ""
	if f.ReleaseName != nil {
		release = *f.ReleaseName
	}
	year := 0
	if t.Year != nil {
		year = *t.Year
	}
	ext := path.Ext(f.RelativePath)
	fa := mediafiles.FactsFrom(t.Name, year, t.SourceIDs, releasename.Parse(release, releasename.Hints{}), release, ext)
	if edition, ok := f.MediaInfo["edition"].(string); ok {
		fa.Edition = edition
	}
	if step, ok := f.MediaInfo["step"].(string); ok && step != "" {
		fa.Step = step
	}
	return fa
}

// fileEpisodes lists the episodes a File holds, in episode order.
func fileEpisodes(ctx context.Context, db Reader, f mediafiles.File) ([]importer.Episode, error) {
	var out []importer.Episode
	for _, sid := range f.SlotIDs {
		var it mediafiles.Item
		var itemID int64
		if err := db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", sid).Scan(&itemID); err != nil {
			continue
		}
		if err := database.Get(ctx, db, "items", itemID, &it); err != nil {
			return nil, err
		}
		e := importer.Episode{Ref: mediafiles.EpisodeRef{ItemID: it.ID}, SlotID: sid}
		if it.Numbers.Season != nil {
			e.Ref.Season = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			e.Ref.Episode = *it.Numbers.Episode
		}
		if it.Numbers.Absolute != nil {
			e.Ref.Absolute = *it.Numbers.Absolute
		}
		if it.Numbers.AltSeason != nil && it.Numbers.AltEpisode != nil {
			e.Ref.AltSeason, e.Ref.AltEpisode = *it.Numbers.AltSeason, *it.Numbers.AltEpisode
		}
		if it.AirDay != nil {
			e.Ref.AirDay = *it.AirDay
		}
		if it.Name != nil {
			e.Name = *it.Name
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref.Season != out[j].Ref.Season {
			return out[i].Ref.Season < out[j].Ref.Season
		}
		return out[i].Ref.Episode < out[j].Ref.Episode
	})
	return out, nil
}

// episodeFacts are the naming facts of an episode File in the Library.
func episodeFacts(t mediafiles.Title, f mediafiles.File, eps []importer.Episode) mediafiles.EpisodeFacts {
	release := ""
	if f.ReleaseName != nil {
		release = *f.ReleaseName
	}
	year := 0
	if t.Year != nil {
		year = *t.Year
	}
	order := mediafiles.OrderAired
	if t.EpisodeOrder != nil && *t.EpisodeOrder != "" {
		order = *t.EpisodeOrder
	}
	if t.EpisodeGroupID != nil && *t.EpisodeGroupID != "" {
		eps = importer.NamedEpisodes(eps)
		sort.SliceStable(eps, func(i, j int) bool {
			if eps[i].Ref.Season != eps[j].Ref.Season {
				return eps[i].Ref.Season < eps[j].Ref.Season
			}
			return eps[i].Ref.Episode < eps[j].Ref.Episode
		})
	}
	fa := importer.EpisodeFactsFor(t.Name, year, t.SourceIDs, order, releasename.Parse(release, releasename.Hints{Show: true}), release, path.Ext(f.RelativePath), eps)
	if step, ok := f.MediaInfo["step"].(string); ok && step != "" {
		fa.Step = step
	}
	return fa
}

// albumFacts are an album's naming facts and its disc count.
type albumFacts struct {
	f     mediafiles.TrackFacts
	discs int
}

// trackFacts are the naming facts of a track File in the Library: its
// track (the first Slot's), album (and its id) and artist. ok is false for
// a File that holds no track.
func trackFacts(ctx context.Context, db Reader, t mediafiles.Title, f mediafiles.File, cache map[int64]albumFacts) (mediafiles.TrackFacts, int64, bool, error) {
	if len(f.SlotIDs) == 0 {
		return mediafiles.TrackFacts{}, 0, false, nil
	}
	var it mediafiles.Item
	var itemID int64
	if err := db.QueryRowContext(ctx, "SELECT item_id FROM slots WHERE id = ?", f.SlotIDs[0]).Scan(&itemID); err != nil {
		return mediafiles.TrackFacts{}, 0, false, nil
	}
	if err := database.Get(ctx, db, "items", itemID, &it); err != nil {
		return mediafiles.TrackFacts{}, 0, false, err
	}
	if it.Kind != "track" || it.PartID == nil {
		return mediafiles.TrackFacts{}, 0, false, nil
	}
	af, ok := cache[*it.PartID]
	if !ok {
		var err error
		af.f, af.discs, err = importer.AlbumFacts(ctx, db, t.Name, *it.PartID)
		if err != nil {
			return mediafiles.TrackFacts{}, 0, false, err
		}
		cache[*it.PartID] = af
	}
	tf := af.f
	tf.Discs, tf.Disc = af.discs, 1
	if it.Numbers.Disc != nil && *it.Numbers.Disc > 0 {
		tf.Disc = *it.Numbers.Disc
	}
	if it.Numbers.Track != nil {
		tf.Track = *it.Numbers.Track
	}
	if it.Name != nil {
		tf.Title = *it.Name
	}
	tf.Ext = path.Ext(f.RelativePath)
	return tf, *it.PartID, true, nil
}

// PlanRename works out the new name of every main File of the given Titles
// (every Title when titleIDs is empty) under the naming settings. Files that
// would keep their name are left out. Nothing on disk changes.
func PlanRename(ctx context.Context, db Reader, titleIDs []int64) ([]RenameStep, error) {
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return nil, err
	}
	return planRename(ctx, db, ns, titleIDs)
}

// PlanRenameHashed is PlanRename with the plan hash a rename must be
// asked with, so it does only what this preview showed.
func PlanRenameHashed(ctx context.Context, db Reader, titleIDs []int64) ([]RenameStep, string, error) {
	ns, err := mediafiles.ReadNaming(ctx, db)
	if err != nil {
		return nil, "", err
	}
	plan, err := planRename(ctx, db, ns, titleIDs)
	if err != nil {
		return nil, "", err
	}
	return plan, planHash(plan, ns), nil
}

// planHash is the SHA-256 of every step's file, old and new name and skip,
// and of the naming settings.
func planHash(plan []RenameStep, ns mediafiles.NamingSettings) string {
	h := sha256.New()
	ns.AdoptScanLimit, ns.MusicAdoptScanLimit = 0, 0 // not naming choices
	set, _ := json.Marshal(ns)
	h.Write(set)
	for _, st := range plan {
		fmt.Fprintf(h, "\n%d\x00%s\x00%s\x00%s", st.FileID, st.From, st.To, st.Skip)
		for _, m := range st.Subtitles {
			fmt.Fprintf(h, "\n\x01%s\x00%s\x00%s", m.From, m.To, m.Skip)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func planRename(ctx context.Context, db Reader, ns mediafiles.NamingSettings, titleIDs []int64) ([]RenameStep, error) {
	where, args := "state != 'removed' ORDER BY sort_name, id", []any{}
	if len(titleIDs) > 0 {
		where = "state != 'removed' AND id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(titleIDs)), ",") + ") ORDER BY sort_name, id"
		for _, id := range titleIDs {
			args = append(args, id)
		}
	}
	titles, err := database.Select[mediafiles.Title](ctx, db, "titles", where, args...)
	if err != nil {
		return nil, err
	}
	out := []RenameStep{}
	for _, t := range titles {
		files, err := database.Select[mediafiles.File](ctx, db, "files",
			"state = 'present' AND role = 'main' AND version_id IN (SELECT id FROM versions WHERE title_id = ?) ORDER BY id", t.ID)
		if err != nil {
			return nil, err
		}
		taken := map[string]bool{}
		albums := map[int64]albumFacts{}
		for _, f := range files {
			var v mediafiles.Version
			var lf mediafiles.LibraryFolder
			if err := database.Get(ctx, db, "versions", f.VersionID, &v); err != nil {
				return nil, err
			}
			if err := database.Get(ctx, db, "library_folders", v.LibraryFolderID, &lf); err != nil {
				return nil, err
			}
			st := RenameStep{FileID: f.ID, TitleID: t.ID, TitleName: t.Name, VersionID: v.ID, LibraryFolderID: lf.ID, Origin: f.Origin,
				From: f.RelativePath, root: lf.Path}
			facts := fileFacts(t, f)
			to := path.Join(path.Dir(f.RelativePath), importer.EditionFileName(mediafiles.FileName(ns, facts), facts.Edition))
			if path.Dir(f.RelativePath) == "." {
				to = path.Join(v.FolderName, importer.EditionFileName(mediafiles.FileName(ns, facts), facts.Edition))
			}
			if t.Kind == "show" {
				// An episode file: show folder, season folder, episode name,
				// the same function the import uses.
				eps, err := fileEpisodes(ctx, db, f)
				if err != nil {
					return nil, err
				}
				if len(eps) == 0 {
					continue
				}
				to = mediafiles.EpisodePath(ns, v.FolderName, episodeFacts(t, f, eps))
			}
			if t.Kind == "artist" {
				// A track file: artist folder, album folder, disc folder,
				// track name, the same function the album import uses.
				tf, partID, ok, err := trackFacts(ctx, db, t, f, albums)
				if err != nil {
					return nil, err
				}
				if !ok {
					continue
				}
				to = mediafiles.TrackPath(ns, v.FolderName, tf)
				st.AlbumID = partID
			}
			fromAbs := filepath.Join(lf.Path, filepath.FromSlash(f.RelativePath))
			toAbs := filepath.Join(lf.Path, filepath.FromSlash(to))
			switch {
			case to == f.RelativePath:
				continue
			case f.Origin != "imported" && f.Origin != "adopted":
				st.Skip = "found on disk, not imported or adopted; HolyArrs never renames it"
			case !filepath.IsLocal(filepath.FromSlash(to)) || !filepath.IsLocal(filepath.FromSlash(f.RelativePath)):
				st.Skip = "the name would leave the Library folder"
			case taken[strings.ToLower(to)]:
				st.Skip = "another file in this rename gets the same name"
			case importer.PathTooLong(toAbs) != "":
				st.Skip = importer.PathTooLong(toAbs)
			case !importer.InsideReal(lf.Path, filepath.Dir(fromAbs)) || !importer.InsideReal(lf.Path, filepath.Dir(toAbs)):
				st.Skip = "a link leads the file or its new name outside the Library folder"
			default:
				if _, err := os.Stat(lf.Path); err != nil {
					st.Skip = "the Library folder cannot be reached"
				} else if _, err := os.Stat(fromAbs); err != nil {
					st.Skip = "the file is not on disk"
				} else if other, err := os.Stat(toAbs); err == nil {
					if self, _ := os.Stat(fromAbs); !os.SameFile(self, other) {
						st.Skip = "a file named " + path.Base(to) + " is already there"
					}
				}
			}
			if st.Skip == "" {
				st.To = to
				taken[strings.ToLower(to)] = true
				if t.Kind != "artist" { // music has no subtitles
					st.Subtitles = planSubtitles(lf.Path, f.RelativePath, to, taken)
				}
			}
			out = append(out, st)
		}
	}
	return out, nil
}

// ApplyRename carries out the steps of a plan that are not skipped. Each
// step is written to rename_steps before the disk changes, so a run cut off
// halfway is finished by RecoverRenames. It returns how many Files moved.
func (s *Service) ApplyRename(ctx context.Context, plan []RenameStep, jobID int64, by domain.Actor) (int, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	return s.applyRenameLocked(ctx, plan, jobID, by)
}

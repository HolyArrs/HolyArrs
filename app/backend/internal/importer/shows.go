package importer

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Episode is one known episode of a show.
type Episode struct {
	Ref    mediafiles.EpisodeRef
	Name   string
	SlotID int64
	Wanted bool
}

// ShowTarget describes the show a download is placed into.
type ShowTarget struct {
	LibraryPath string
	FolderName  string
	Show        mediafiles.TitleRef
	IDs         map[string]string
	Order       string
	Episodes    []Episode
	ReleaseName string
	Naming      *mediafiles.NamingSettings
	// ByOrder names files by the show's chosen episode order.
	ByOrder bool
}

func (t ShowTarget) episodePath(p releasename.Result, source string, eps []Episode) string {
	ns := mediafiles.NamingDefaults()
	if t.Naming != nil {
		ns = *t.Naming
	}
	ext := filepath.Ext(source)
	release := strings.TrimSuffix(filepath.Base(source), ext)
	if t.ByOrder {
		eps = NamedEpisodes(eps)
	}
	f := EpisodeFactsFor(t.Show.Name, t.Show.Year, t.IDs, t.Order, p, release, ext, eps)
	if !ns.RenameOnImport {
		return path.Join(t.FolderName, mediafiles.SeasonFolder(ns, f.Season), mediafiles.CleanWith(ns, release)+strings.ToLower(ext))
	}
	return mediafiles.EpisodePath(ns, t.FolderName, f)
}

// EpisodeFactsFor builds the naming facts for one episode file.
func EpisodeFactsFor(show string, year int, ids map[string]string, order string, p releasename.Result, release, ext string, eps []Episode) mediafiles.EpisodeFacts {
	f := mediafiles.EpisodeFacts{NameFacts: mediafiles.FactsFrom(show, year, ids, p, release, ext), Order: order}
	for i, e := range eps {
		if i == 0 {
			f.Season, f.AirDate = e.Ref.Season, e.Ref.AirDay
		}
		f.Episodes = append(f.Episodes, e.Ref.Episode)
		f.Absolute = append(f.Absolute, e.Ref.Absolute)
		f.Titles = append(f.Titles, e.Name)
	}
	return f
}

// NamedEpisodes copies eps with each episode on its release-name numbers,
// for naming files by a chosen episode order.
func NamedEpisodes(eps []Episode) []Episode {
	out := make([]Episode, len(eps))
	for i, e := range eps {
		e.Ref.Season, e.Ref.Episode = e.Ref.Named()
		e.Ref.AltSeason, e.Ref.AltEpisode = 0, 0
		out[i] = e
	}
	return out
}

// orderClash names an episode of eps the chosen order leaves out whose
// TMDB numbers another episode has in the order, or "" when none does.
func (t ShowTarget) orderClash(eps []Episode) string {
	if !t.ByOrder {
		return ""
	}
	for _, e := range eps {
		if e.Ref.AltSeason > 0 {
			continue
		}
		for _, o := range t.Episodes {
			if o.Ref.AltSeason == e.Ref.Season && o.Ref.AltEpisode == e.Ref.Episode {
				return e.Ref.Label()
			}
		}
	}
	return ""
}

func labels(eps []Episode) string {
	out := make([]string, len(eps))
	for i, e := range eps {
		out[i] = e.Ref.Label()
	}
	return strings.Join(out, ", ")
}

// PlanEpisodes turns candidate files into copy steps for a show.
func PlanEpisodes(cands []Candidate, t ShowTarget) []mediafiles.PlanStep {
	t.Show.Kind = "show"
	lib := []mediafiles.TitleRef{t.Show}
	refs := make([]mediafiles.EpisodeRef, len(t.Episodes))
	byItem := map[int64]Episode{}
	for i, e := range t.Episodes {
		refs[i] = e.Ref
		byItem[e.Ref.ItemID] = e
	}
	var steps []mediafiles.PlanStep
	for _, c := range cands {
		ext := strings.ToLower(filepath.Ext(c.Path))
		s := mediafiles.PlanStep{SourceFile: c.Path, TargetSlotIDs: []int64{}, Why: []string{}}
		p := releasename.Parse(filepath.Base(c.Path), releasename.Hints{Size: c.Size, Show: true})
		switch {
		case programExt[ext] || p.Has("executable") || p.Has("double_extension"):
			s.Method, s.Why = "refuse", []string{"a program file, not a video"}
		case !videoExt[ext]:
			s.Method, s.Why = "skip", []string{"not a video file (" + ext + ")"}
		case isSample(c.Path, p):
			s.Method, s.Why = "refuse", []string{"a sample, not the full video"}
		case !p.IsShow():
			s.Method, s.Why = "skip", []string{"the file name gives no episode numbers; left alone"}
		case mediafiles.IsPack(p):
			s.Method, s.Why = "skip", []string{"the file name names whole seasons, not episodes; left alone"}
		default:
			m := mediafiles.MatchTitle(p, lib)
			if m.Title == nil && strings.TrimSpace(p.Title.Value) == "" {
				for _, alt := range []string{filepath.Base(filepath.Dir(c.Path)), t.ReleaseName} {
					if alt == "" {
						continue
					}
					if fm := mediafiles.MatchTitle(releasename.Parse(alt, releasename.Hints{Show: true}), lib); fm.Title != nil {
						m = fm
						break
					}
				}
			}
			if m.Title == nil {
				s.Method, s.Why = "refuse", []string{"does not match the show: " + m.Detail}
				break
			}
			found, why := mediafiles.MatchEpisodes(p, t.Order, refs)
			if len(found) == 0 {
				s.Method, s.Why = "skip", []string{why + "; left alone"}
				break
			}
			var eps, want []Episode
			for _, f := range found {
				e := byItem[f.ItemID]
				eps = append(eps, e)
				if e.Wanted {
					want = append(want, e)
				}
			}
			if len(want) == 0 {
				s.Method, s.Why = "skip", []string{labels(eps) + " is not filled by this download; left alone"}
				break
			}
			if c := t.orderClash(eps); c != "" {
				s.Method, s.Why = "refuse", []string{c + " is not in the chosen episode order, and its TMDB number names another episode in that order"}
				break
			}
			s.Method = "hardlink"
			for _, e := range want {
				s.TargetSlotIDs = append(s.TargetSlotIDs, e.SlotID)
			}
			s.TargetPath = filepath.Join(t.LibraryPath, filepath.FromSlash(t.episodePath(p, c.Path, eps)))
			s.Confidence = confidence(m.Conf)
			s.Why = []string{m.Detail, "holds " + labels(eps)}
			if s.Confidence < MinConfidence {
				s.Method = "refuse"
				s.Why = append(s.Why, "too unsure to import without you")
			}
		}
		steps = append(steps, s)
	}
	order := make([]int, 0, len(steps))
	for i := range steps {
		if steps[i].Method == "hardlink" {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		return sizeOf(cands, steps[order[a]].SourceFile) > sizeOf(cands, steps[order[b]].SourceFile)
	})
	taken := map[int64]bool{}
	for _, i := range order {
		clash := false
		for _, id := range steps[i].TargetSlotIDs {
			clash = clash || taken[id]
		}
		if clash {
			steps[i].Method, steps[i].TargetPath = "skip", ""
			steps[i].Why = append(steps[i].Why, "a bigger file in the same download holds the same episode")
			continue
		}
		for _, id := range steps[i].TargetSlotIDs {
			taken[id] = true
		}
	}
	return steps
}

func heldFrom(recorded []string, rel string) string {
	want := "/" + strings.TrimPrefix(filepath.ToSlash(rel), "/")
	for _, r := range recorded {
		s := filepath.ToSlash(r)
		if i := strings.LastIndex(s, "."); i > 0 && strings.HasSuffix(s[:i], want) {
			if _, err := strconv.Atoi(s[i+1:]); err == nil {
				s = s[:i]
			}
		}
		if strings.HasSuffix(s, want) {
			if _, err := os.Lstat(r); err == nil {
				return r
			}
		}
	}
	return ""
}

type showSlot struct {
	sl      mediafiles.Slot
	it      mediafiles.Item
	live    bool
	step    int
	lie     string
	outcome string
	to      mediafiles.SlotState
	reason  string
}

var showImportEntered func(id int64)

package sources

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// copySources is how one copy (Version) of a title picks its Sources. Search
// and the feed both go by it, so they decide alike.
type copySources struct {
	// tags are the title's, plus those of the copy's Library folder when that folder asks only tagged Sources.
	tags []string
	// folder is the tags of the copy's Library folder when it asks only tagged Sources.
	folder []string
	// only leaves Sources with no tags out.
	only bool
	// passed says the folder asks only tagged Sources but no Source has one of its tags, so untagged Sources serve the copy too.
	passed bool
}

// takes says whether a Source with these tags is used for the copy; rules are
// the tags of its Pick rules.
func (c copySources) takes(source, rules []string) bool {
	return mediafiles.SourceTakesTitle(source, c.tags, rules, c.only)
}

// copyOf works one copy out. folder is its Library folder's tags when the
// folder asks only tagged Sources. kind is the tags of every Source that
// carries the title's kind, whatever its state and whatever it is used for:
// while one of them has a tag of the folder, untagged Sources stay out. The
// folder's tags alone decide that. When no Source has one, the copy picks
// its Sources by the title's own tags, as it would in any other folder.
func copyOf(title, folder []string, kind [][]string) copySources {
	own := mediafiles.CleanTags(folder)
	if len(own) == 0 {
		return copySources{tags: title}
	}
	if !mediafiles.OnlyTagged(true, own, kind) {
		return copySources{tags: title, folder: own, passed: len(kind) > 0}
	}
	tags, _ := mediafiles.SourceTags(title, own)
	return copySources{tags: tags, folder: own, only: true}
}

// ofKind says whether a Source carries a title kind ("show", "artist",
// anything else is a movie).
func ofKind(src domain.Source, kind string) bool {
	switch kind {
	case "show":
		return handlesTV(src)
	case "artist":
		return handlesMusic(src)
	}
	return handlesMovies(src)
}

// kindTags is the tags of each Source that carries a title kind.
func kindTags(srcs []domain.Source, kind string) [][]string {
	var out [][]string
	for _, src := range srcs {
		if ofKind(src, kind) {
			out = append(out, src.Tags)
		}
	}
	return out
}

// pausedNow says whether a Source is paused at this moment.
func pausedNow(src domain.Source, now time.Time) bool {
	return src.State == "paused" && (src.PausedUntil == nil || src.PausedUntil.After(now))
}

// keptApartLine is what a search says when the copy's folder asks only tagged
// Sources and this search has no Source to ask: it names the folder's tags
// and, for each Source that has one of them, why it cannot be searched. ""
// when the folder is not kept to tagged Sources, or one of those Sources can
// be searched (something else, like the Pick rules, then stands in the way).
func (c copySources) keptApartLine(srcs []domain.Source, kind string, hand bool, now time.Time) string {
	if !c.only {
		return ""
	}
	var whys []string
	for _, src := range srcs {
		if !ofKind(src, kind) || !mediafiles.Shares(src.Tags, c.folder) {
			continue
		}
		switch {
		case src.State == "disabled":
			whys = append(whys, src.Name+" is paused by you")
		case src.Format == "rss":
			whys = append(whys, src.Name+" only has a feed")
		case hand && !src.UseFor.HandSearch, !hand && !src.UseFor.AutoSearch:
			whys = append(whys, src.Name+" has search turned off")
		case pausedNow(src, now):
			whys = append(whys, src.Name+" is paused")
		default:
			return ""
		}
	}
	if len(whys) == 0 {
		return ""
	}
	return "This folder only takes Sources tagged " + strings.Join(c.folder, ", ") + ". None can be searched right now (" + strings.Join(whys, "; ") + "), so nothing was asked."
}

// copySourcesOf reads how one copy of a title picks its Sources; srcs are all
// the Sources, the turned-off ones too.
func (s *Searcher) copySourcesOf(ctx context.Context, ti mediafiles.Title, versionID int64, srcs []domain.Source) (copySources, error) {
	folder, err := mediafiles.FolderSourceTags(ctx, s.Store.Read(), ti.ID)
	if err != nil {
		return copySources{}, err
	}
	return copyOf(ti.Tags, folder[ti.ID][versionID], kindTags(srcs, ti.Kind)), nil
}

// copyTakes keeps the feed Releases one copy may use: those whose Source its
// Pick rules may use and that serves the copy's tags, the same rules Search
// follows when it picks Sources to ask.
func copyTakes(c slotCtx, fs []found) []found {
	out := make([]found, 0, len(fs))
	for _, f := range fs {
		if mediafiles.RulesTakeSource(f.src.Tags, c.pr.Body.Tags) && c.from.takes(f.src.Tags, c.pr.Body.Tags) {
			out = append(out, f)
		}
	}
	return out
}

// heldStillTaken says whether the Source of a Held Release still serves the
// copy of its Slot. A Release held before its folder asked only tagged Sources
// may come from a Source the copy no longer uses. Anything that cannot be
// read counts as taken.
func (s *Searcher) heldStillTaken(ctx context.Context, slotID int64, rel domain.Release) bool {
	c, err := s.loadSlot(ctx, slotID)
	if err != nil {
		return true
	}
	var src domain.Source
	if err := database.Get(ctx, s.Store.Read(), "sources", rel.SourceID, &src); err != nil {
		return true
	}
	srcs, err := database.Select[domain.Source](ctx, s.Store.Read(), "sources", anyState)
	if err != nil {
		return true
	}
	cp, err := s.copySourcesOf(ctx, c.ti, c.ver.ID, srcs)
	if err != nil {
		return true
	}
	return cp.takes(src.Tags, c.pr.Body.Tags)
}

// heldNotTakenWhy is the reason on a Slot whose Held Release was let go.
const heldNotTakenWhy = "not sent: its Source is not one this folder's tags allow"

// dropHeldNotTaken sends one Slot holding a Release back to looking when the
// Release's Source no longer serves its copy. Other copies holding the same
// Release keep it. A Slot with a file goes to better wanted so the file is kept.
func (s *Searcher) dropHeldNotTaken(ctx context.Context, slotID int64, rel domain.Release, by domain.Actor) {
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		list, err := mediafiles.SlotsHeldBy(tx, rel.ID)
		if err != nil {
			return err
		}
		for _, h := range list {
			if h.ID != slotID {
				continue
			}
			to := mediafiles.SlotWanted
			if h.HasFile {
				to = mediafiles.SlotBetterWanted
			}
			if err := mediafiles.ClearSlotHeldTx(tx, h.ID); err != nil {
				return err
			}
			return mediafiles.MoveSlotTx(tx, h.ID, to, heldNotTakenWhy, by)
		}
		return nil
	})
}

// keptApartDone ends a search that asked nobody because its folder's tagged
// Sources cannot be asked. The Slot already says why. A search by hand is
// also written down, as a finished search, so the person who asked reads the
// line; the automatic one writes nothing, turn after turn.
func (s *Searcher) keptApartDone(ctx context.Context, r *jobs.Run, by domain.Actor, titleID, slotID int64, hand bool, line string) {
	if hand {
		jobID := r.Job.ID
		s.addEvent(ctx, by, &jobID, titleID, slotID, EventSearchDone, line, noted(nil, line))
	}
	r.SetResult(map[string]any{"skipped": line})
}

// touchSearched notes that Items were searched for now. A search that asked
// nobody counts too, so the nightly search moves on to the titles after it.
func (s *Searcher) touchSearched(ctx context.Context, items ...int64) {
	now := time.Now().UTC()
	_ = s.Store.Write(ctx, func(tx *sql.Tx) error {
		for _, id := range items {
			if err := mediafiles.TouchItemSearchedTx(tx, id, now); err != nil {
				return err
			}
		}
		return nil
	})
}

package sources

import (
	"log/slog"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Why a feed check let a Release go without storing it.
const (
	dropSeen            = "seen_before"       // stored by an earlier feed check, or waiting as judged
	dropSame            = "same_as_other"     // the same Release from another Source
	dropNoTitle         = "no_title_match"    // names no Title in the Library
	dropTitleNotLooking = "title_not_looking" // names a Title with nothing wanted (episode not listed yet, all held or not kept)
	dropSourceTags      = "source_tags"       // the Source's tags do not serve that Title
	dropNoSlot          = "no_slot_looking"   // the Title stopped looking during the check
	dropNotKept         = "not_kept"          // a show whose looking episodes are not kept up to date in this Version
)

// dropExamples is how many names per reason the event keeps. Names that
// match nothing in the Library are only counted: they are most of a feed.
const dropExamples = 5

// feedDrops counts why feed Releases were let go, with a few names for the
// reasons that point at a Title in the Library.
type feedDrops struct {
	count    map[string]int
	examples map[string][]string
}

func newFeedDrops() *feedDrops {
	return &feedDrops{count: map[string]int{}, examples: map[string][]string{}}
}

// add records one Release let go. Each one gets a debug line; the counts go
// on the feed check event.
func (d *feedDrops) add(src domain.Source, name, reason string) {
	d.count[reason]++
	if reason != dropNoTitle && reason != dropSeen && reason != dropSame && len(d.examples[reason]) < dropExamples {
		d.examples[reason] = append(d.examples[reason], name)
	}
	slog.Debug("feed check let a release go", "source", src.Name, "release", name, "reason", reason)
}

// into puts the counts on a feed check result when there are any.
func (d *feedDrops) into(result map[string]any) {
	if len(d.count) == 0 {
		return
	}
	result["dropped"] = d.count
	if len(d.examples) > 0 {
		result["dropped_examples"] = d.examples
	}
}

// matchFeedItem matches one feed Release to a Title the way the feed check
// and its probe do: as named, then as a show, then as an album.
func matchFeedItem(it Item, refs []mediafiles.TitleRef, hasShow, hasArtist bool) mediafiles.Match {
	m := mediafiles.MatchTitle(releasename.Parse(it.Title, releasename.Hints{Size: it.Size}), refs)
	if m.Title == nil && hasShow {
		// Read again as a show name ("Part 2" before tags, "- 12").
		m = mediafiles.MatchTitle(releasename.Parse(it.Title, releasename.Hints{Size: it.Size, Show: true}), refs)
	}
	if m.Title == nil && hasArtist {
		// Read again as an album name ("Artist - Album (Year) [FLAC]").
		m = mediafiles.MatchTitle(releasename.Parse(it.Title, releasename.Hints{Size: it.Size, Music: true}), refs)
	}
	return m
}

// titleRefs turns Titles into the small records a feed is matched with.
func titleRefs(titles []mediafiles.Title) []mediafiles.TitleRef {
	refs := make([]mediafiles.TitleRef, 0, len(titles))
	for _, title := range titles {
		ref := mediafiles.TitleRef{ID: title.ID, Name: title.Name, Aliases: title.Aliases, Tags: title.Tags}
		if title.Year != nil {
			ref.Year = *title.Year
		}
		if title.OriginalLanguage != nil {
			ref.OriginalLanguage = *title.OriginalLanguage
		}
		if title.Kind == "show" || title.Kind == "artist" {
			ref.Kind = title.Kind
		}
		refs = append(refs, ref)
	}
	return refs
}

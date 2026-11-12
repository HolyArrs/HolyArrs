package arrapi

import (
	"context"
	"encoding/json"
	"log"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// The events HolyArrs sends to hooks: one per file imported into a
// title's main Version, in the old apps' shape. Pulsarr reads an import
// as "this title is ready" and tells the people who asked, so an upgrade
// is sent only to a hook with its upgrade switch on, and nothing but
// imports is ever sent.

// hookSeries, hookEpisode, hookFile and hookMovie are the parts of an
// import event.
type hookSeries struct {
	ID     int64    `json:"id"`
	Title  string   `json:"title"`
	TvdbID int64    `json:"tvdbId"`
	Tags   []string `json:"tags"`
}

type hookEpisode struct {
	ID            int64  `json:"id"`
	SeasonNumber  int    `json:"seasonNumber"`
	EpisodeNumber int    `json:"episodeNumber"`
	Title         string `json:"title"` // empty text when the episode has no name; Pulsarr needs the field
	AirDateUtc    *utc   `json:"airDateUtc,omitempty"`
}

type hookFile struct {
	ID             int64  `json:"id"`
	RelativePath   string `json:"relativePath"`
	Quality        string `json:"quality"`
	QualityVersion int    `json:"qualityVersion"`
	Size           int64  `json:"size"`
}

type hookMovie struct {
	ID     int64    `json:"id"`
	Title  string   `json:"title"`
	TmdbID int64    `json:"tmdbId"`
	Tags   []string `json:"tags"`
}

type hookEvent struct {
	EventType    string        `json:"eventType"`
	InstanceName string        `json:"instanceName"`
	Series       *hookSeries   `json:"series,omitempty"`
	Episodes     []hookEpisode `json:"episodes,omitempty"`
	EpisodeFile  *hookFile     `json:"episodeFile,omitempty"`
	Movie        *hookMovie    `json:"movie,omitempty"`
	IsUpgrade    bool          `json:"isUpgrade"`
}

// hookNote is one event to send and whether it is an upgrade.
type hookNote struct {
	body    []byte
	upgrade bool
}

// hookAttempt bounds one call of an event.
const hookAttempt = 15 * time.Second

// hookBackoff is the wait before each new try of a failed event.
var hookBackoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}

// hookSender tells the hooks about imports.
type hookSender struct {
	store *database.Store
	vault *secrets.Vault
	// allowed says whether a side may send now: the arr API is on and the
	// side has a key. A side whose key was taken away tells nobody.
	allowed func(ctx context.Context, side string) bool
	backoff []time.Duration
	wg      sync.WaitGroup
}

// watch sends each import Event the store records, on its own goroutine;
// ending base stops the sends and their new tries.
func (s *hookSender) watch(base context.Context) {
	s.store.OnEvent(func(e domain.Event) {
		if (e.Kind != "import_done" && e.Kind != "import_partly_done") || e.TitleID == nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			jobs.Guard("arrapi hooks", func() {
				ctx, cancel := context.WithTimeout(base, 10*time.Minute)
				defer cancel()
				s.deliver(ctx, e)
			})
		}()
	})
}

func (s *hookSender) wait() { s.wg.Wait() }

// deliver sends an import Event to every hook of the title's side that
// speaks for it.
func (s *hookSender) deliver(ctx context.Context, e domain.Event) {
	db := s.store.Read()
	var t mediafiles.Title
	if database.Get(ctx, db, "titles", *e.TitleID, &t) != nil || t.State == "removed" {
		return
	}
	side := map[string]string{"show": "sonarr", "movie": "radarr"}[t.Kind]
	if side == "" || !s.allowed(ctx, side) {
		return
	}
	hooks, err := listHookRows(ctx, db, side)
	if err != nil {
		log.Printf("arrapi: hooks: %v", err)
		return
	}
	hooks = slices.DeleteFunc(hooks, func(hk hook) bool { return !hk.OnImport })
	if len(hooks) == 0 {
		return
	}
	notes, err := s.notesFor(ctx, e, t)
	if err != nil {
		log.Printf("arrapi: hooks for title %d: %v", t.ID, err)
		return
	}
	if len(notes) == 0 {
		return
	}
	tags, err := tagsOf(ctx, db, []int64{t.ID})
	if err != nil {
		log.Printf("arrapi: hooks for title %d: %v", t.ID, err)
		return
	}
	// Each hook gets its own goroutine, so one that keeps failing does not
	// hold up the others; deliver waits for them all.
	var each sync.WaitGroup
	defer each.Wait()
	for _, hk := range hooks {
		if len(hk.Tags) > 0 && !slices.ContainsFunc(hk.Tags, func(id int64) bool { return slices.Contains(tags[t.ID], id) }) {
			continue
		}
		sec, err := hookSecretOf(ctx, s.vault, hk)
		if err != nil {
			log.Printf("arrapi: hook %d: its sealed part could not be read: %v", hk.ID, err)
			continue
		}
		each.Add(1)
		go func() {
			defer each.Done()
			jobs.Guard("arrapi hooks", func() {
				for _, n := range notes {
					if n.upgrade && !hk.OnUpgrade {
						continue
					}
					if err := s.send(ctx, hk, sec, n.body); err != nil {
						log.Printf("arrapi: hook %d (%s side): an import event was not taken: %v", hk.ID, hk.Side, err)
					}
				}
			})
		}()
	}
}

// send calls a hook with an event, trying again after each backoff wait
// while the failure is worth it.
func (s *hookSender) send(ctx context.Context, hk hook, sec hookSecret, body []byte) error {
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "arr_hook", Identity: []string{strconv.FormatInt(hk.ID, 10), sec.URL}})
	if err != nil {
		return err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	waits := s.backoff
	if waits == nil {
		waits = hookBackoff
	}
	attempt := func() error {
		actx, cancel := op.AttemptContext(ctx, hookAttempt)
		defer cancel()
		return callHook(actx, hk.Method, sec, body)
	}
	err = attempt()
	for i := 0; err != nil && worthRetry(err) && i < len(waits); i++ {
		t := time.NewTimer(waits[i])
		select {
		case <-ctx.Done():
			t.Stop()
			return err
		case <-t.C:
		}
		err = attempt()
	}
	switch {
	case ctx.Err() != nil:
	case err == nil:
		outcome = outbound.Success
	case worthRetry(err):
		outcome = outbound.TransientFailure
	default:
		outcome = outbound.TerminalFailure
	}
	return err
}

// placedStep is one file an import placed: its Slots, and whether it
// replaced an older copy.
type placedStep struct {
	slots   []int64
	upgrade bool
}

// placedSteps reads what an import Event placed, from the import's own
// plan. An Event without one counts its Slots as one file, not an upgrade.
func placedSteps(ctx context.Context, db database.Querier, e domain.Event) []placedStep {
	if id := int64Of(e.Data["import_id"]); id > 0 {
		var imp mediafiles.Import
		if database.Get(ctx, db, "imports", id, &imp) == nil {
			var out []placedStep
			for i, st := range imp.Plan {
				if i < len(imp.Result) && imp.Result[i].Outcome == "placed" && len(st.TargetSlotIDs) > 0 {
					out = append(out, placedStep{slots: st.TargetSlotIDs, upgrade: st.ReplacesFileID != nil})
				}
			}
			return out
		}
	}
	for _, k := range []string{"placed_slot_ids", "slot_ids"} {
		if ids := int64sOf(e.Data[k]); len(ids) > 0 {
			return []placedStep{{slots: ids}}
		}
	}
	if e.SlotID != nil {
		return []placedStep{{slots: []int64{*e.SlotID}}}
	}
	return nil
}

// notesFor builds the events for an import of title t: one per file placed
// in its main Version.
func (s *hookSender) notesFor(ctx context.Context, e domain.Event, t mediafiles.Title) ([]hookNote, error) {
	db := s.store.Read()
	steps := placedSteps(ctx, db, e)
	if len(steps) == 0 {
		return nil, nil
	}
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? AND is_main = 1", t.ID)
	if err != nil || len(vs) == 0 {
		return nil, err
	}
	main := vs[0]
	var lf mediafiles.LibraryFolder
	_ = database.Get(ctx, db, "library_folders", main.LibraryFolderID, &lf)
	labels, err := tagLabelsOf(ctx, db, t.ID)
	if err != nil {
		return nil, err
	}
	var out []hookNote
	for _, st := range steps {
		slots, err := database.Select[mediafiles.Slot](ctx, db, "slots", "id IN "+idList(st.slots)+" ORDER BY id")
		if err != nil {
			return nil, err
		}
		slots = slices.DeleteFunc(slots, func(sl mediafiles.Slot) bool { return sl.VersionID != main.ID || sl.FileID == nil })
		if len(slots) == 0 {
			continue
		}
		var f mediafiles.File
		if err := database.Get(ctx, db, "files", *slots[0].FileID, &f); err != nil {
			continue // the file went away since
		}
		mf := mediaFileOf(f, lf.Path, main.FolderName)
		quality, _ := mf.Quality["quality"].(map[string]any)["name"].(string)
		ev := hookEvent{EventType: "Download", InstanceName: eventName, IsUpgrade: st.upgrade}
		if t.Kind == "movie" {
			ev.Movie = &hookMovie{ID: t.ID, Title: t.Name, TmdbID: sourceID(t.SourceIDs, "tmdb"), Tags: labels}
		} else {
			ev.Series = &hookSeries{ID: t.ID, Title: t.Name, TvdbID: sourceID(t.SourceIDs, "tvdb"), Tags: labels}
			ev.EpisodeFile = &hookFile{ID: f.ID, RelativePath: mf.RelativePath, Quality: quality, QualityVersion: 1, Size: f.Size}
			items := make([]int64, 0, len(slots))
			for _, sl := range slots {
				items = append(items, sl.ItemID)
			}
			its, err := database.Select[mediafiles.Item](ctx, db, "items", "kind = 'episode' AND id IN "+idList(items))
			if err != nil {
				return nil, err
			}
			for _, it := range its {
				ep := hookEpisode{ID: it.ID, AirDateUtc: utcPtr(it.AirDate)}
				if it.Numbers.Season != nil {
					ep.SeasonNumber = *it.Numbers.Season
				}
				if it.Numbers.Episode != nil {
					ep.EpisodeNumber = *it.Numbers.Episode
				}
				if it.Name != nil {
					ep.Title = *it.Name
				}
				ev.Episodes = append(ev.Episodes, ep)
			}
			slices.SortFunc(ev.Episodes, func(a, b hookEpisode) int {
				if a.SeasonNumber != b.SeasonNumber {
					return a.SeasonNumber - b.SeasonNumber
				}
				return a.EpisodeNumber - b.EpisodeNumber
			})
			if len(ev.Episodes) == 0 {
				continue // Pulsarr refuses an import with no episodes
			}
		}
		b, _ := json.Marshal(ev)
		out = append(out, hookNote{body: b, upgrade: st.upgrade})
	}
	return out, nil
}

// int64Of reads a number an Event holds, stored or as sent in memory.
func int64Of(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	}
	return 0
}

// int64sOf reads a list of numbers an Event holds.
func int64sOf(v any) []int64 {
	switch x := v.(type) {
	case []int64:
		return x
	case []any:
		var out []int64
		for _, e := range x {
			if n := int64Of(e); n > 0 {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

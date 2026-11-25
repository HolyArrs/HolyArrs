package alerts

import (
	"context"
	"log"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// PluginOnlyEvents are the plugin events that are not Alerts moments, by
// the Event kind they come from. The Alerts moments reach plugins through
// Tap; these through WatchPlugins.
var PluginOnlyEvents = map[string]string{
	domain.EventTitleEdited:       "library.edited",
	domain.EventTitleRefreshed:    "library.refreshed",
	domain.EventTitleMoved:        "library.moved",
	domain.EventEpisodesChanged:   "library.episodes_changed",
	domain.EventAlbumsChanged:     "library.episodes_changed",
	domain.EventAdopted:           "library.adopted",
	"file_replaced":               "library.upgraded",
	domain.EventFileMissing:       "library.file_missing",
	domain.EventFileBack:          "library.file_back",
	"import_rejected":             "download.rejected",
	"download_vanished":           "download.vanished",
	"search_done":                 "search.done",
	"search_failed":               "search.failed",
	"release_held":                "search.release_held",
	domain.EventAskMade:           "request.made",
	domain.EventAskApproved:       "request.approved",
	domain.EventAskTurnedDown:     "request.declined",
	domain.EventAskAvailable:      "request.available",
	domain.EventReportMade:        "report.made",
	domain.EventReportFixed:       "report.fixed",
	domain.EventMediaRefreshed:    "system.media_server_updated",
	domain.EventListSynced:        "system.list_synced",
	domain.EventFolderUnreachable: "system.folder_unreachable",
	domain.EventFreeSpaceLow:      "system.disk_low",
}

// privateData are the only data keys passed on for request and report
// Events: their summaries and other data name people or hold their notes.
var privateData = map[string][]string{
	domain.EventAskMade:       {"ask_id", "tmdb_id"},
	domain.EventAskApproved:   {"ask_id", "tmdb_id"},
	domain.EventAskTurnedDown: {"ask_id", "tmdb_id"},
	domain.EventAskAvailable:  {"ask_id", "tmdb_id"},
	domain.EventReportMade:    {"report_id", "kind"},
	domain.EventReportFixed:   {"report_id", "kind"},
}

// PluginEventPayload is what a plugin is told for an Event that is not an
// Alerts moment: the Event's kind and time, the title it is about, and
// (outside requests and reports) its summary and data with links and full
// paths taken out. Requests and reports carry only their ids, never who
// asked, who decided or what anyone wrote.
func (s *Sender) PluginEventPayload(ctx context.Context, e domain.Event) Payload {
	m := Message{Event: &e, lang: s.lang(ctx)}
	s.titleInto(ctx, e, &m)
	p := Payload{Moment: PluginOnlyEvents[e.Kind], Kind: e.Kind, At: e.At, Movie: m.Movie, Show: m.Show, Episodes: m.Episodes, Artist: m.Artist, Tracks: m.Tracks}
	if p.At.IsZero() {
		p.At = time.Now().UTC()
	}
	if keep, private := privateData[e.Kind]; private {
		p.Data = map[string]any{}
		for _, k := range keep {
			if v, ok := e.Data[k]; ok {
				p.Data[k] = v
			}
		}
		if e.TitleID != nil && p.Movie == nil && p.Show == nil && p.Artist == nil {
			p.Data["title_id"] = *e.TitleID
		}
		return p
	}
	full := PluginPayload(Message{Moment: p.Moment, Text: e.Summary, Event: &e})
	p.Text, p.Data = full.Text, full.Data
	return p
}

// WatchPlugins hands every stored Event in PluginOnlyEvents to tell, each
// on its own goroutine (Wait waits for them).
func (s *Sender) WatchPlugins(base context.Context, tell func(event string, id int64, p Payload)) {
	s.Store.OnEvent(func(e domain.Event) {
		name := PluginOnlyEvents[e.Kind]
		if name == "" {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			jobs.Guard("plugin events", func() {
				ctx, cancel := context.WithTimeout(base, time.Minute)
				defer cancel()
				defer func() {
					if r := recover(); r != nil {
						log.Printf("plugin events: %s: %v", e.Kind, r)
					}
				}()
				tell(name, e.ID, s.PluginEventPayload(ctx, e))
			})
		}()
	})
}

// PluginMomentEvents names the plugin event for each staff Alerts moment.
// Moments about the people who ask are not here: requests and reports
// reach plugins as PluginOnlyEvents, without names.
var PluginMomentEvents = map[string]string{
	MomentAdd: "library.add", MomentDelete: "library.delete", MomentImport: "library.import",
	MomentRename: "library.rename", MomentArtwork: "library.artwork", MomentSubtitle: "library.subtitle",
	MomentDecision: "library.needs_decision", MomentGrab: "download.grab", MomentDownloaded: "download.done",
	MomentDownloadFailed: "download.failed", MomentHealth: "system.health",
}

// momentKinds are the Event kinds MomentOf turns into each staff moment.
var momentKinds = map[string][]string{
	MomentGrab: {"download_sent"}, MomentDownloaded: {"download_finished"}, MomentDownloadFailed: {"download_failed"},
	MomentImport: {"import_done", "import_partly_done"}, MomentRename: {domain.EventRenamed}, MomentAdd: {domain.EventTitleAdded},
	MomentArtwork: {domain.EventArtworkChanged}, MomentDelete: {domain.EventTitleRemoved, domain.EventEpisodesRemoved},
	MomentHealth: {domain.EventHealthBroken, domain.EventHealthRecovered}, MomentDecision: {domain.EventNeedsDecision},
	MomentSubtitle: {"subtitle_saved", "subtitle_upgraded", "subtitle_failed", "subtitle_site_paused"},
}

// PluginEventOf names the plugin event for a stored Event, "" for none.
func PluginEventOf(e domain.Event) string {
	if m := MomentOf(e); m != "" {
		return PluginMomentEvents[m]
	}
	return PluginOnlyEvents[e.Kind]
}

// PluginReplayed is one missed event for a plugin.
type PluginReplayed struct {
	Event   string
	ID      int64
	Payload Payload
}

// PluginReplay lists up to limit stored Events after id after that are one
// of the plugin events names, oldest first, with what a plugin is told for
// each: what a stopped plugin missed.
func (s *Sender) PluginReplay(ctx context.Context, names []string, after int64, limit int) []PluginReplayed {
	var kinds []string
	for _, n := range names {
		for m, ev := range PluginMomentEvents {
			if ev == n {
				kinds = append(kinds, momentKinds[m]...)
			}
		}
		for k, ev := range PluginOnlyEvents {
			if ev == n {
				kinds = append(kinds, k)
			}
		}
	}
	es, err := s.Store.EventsAfter(ctx, after, kinds, limit)
	if err != nil {
		log.Printf("plugin events: catch-up: %v", err)
		return nil
	}
	var out []PluginReplayed
	for _, e := range es {
		name := PluginEventOf(e)
		if name == "" {
			continue // an import that placed nothing, say
		}
		var p Payload
		if m, ok := s.MessageFor(ctx, e); ok {
			p = PluginPayload(m)
		} else {
			p = s.PluginEventPayload(ctx, e)
		}
		out = append(out, PluginReplayed{Event: name, ID: e.ID, Payload: p})
	}
	return out
}

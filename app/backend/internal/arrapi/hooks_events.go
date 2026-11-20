package arrapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"path"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// The events other than imports. Each goes only to the hooks whose own
// switch for it is on, as the record was sent: Pulsarr turns on imports
// alone, and a phone app such as Helmarr turns on nearly all of them. The
// bodies follow the old apps' webhook payloads, with the parts HolyArrs
// knows; HolyArrs has no application updates of its own, so that one is
// never sent.

// hookSwitches maps an Event kind to the switch a hook turns on for it,
// per side. Health has no title and goes to both sides.
var hookSwitches = map[string]map[string]string{
	"download_sent":             {"sonarr": "onGrab", "radarr": "onGrab"},
	domain.EventRenamed:         {"sonarr": "onRename", "radarr": "onRename"},
	domain.EventTitleAdded:      {"sonarr": "onSeriesAdd", "radarr": "onMovieAdded"},
	domain.EventTitleRemoved:    {"sonarr": "onSeriesDelete", "radarr": "onMovieDelete"},
	domain.EventEpisodesRemoved: {"sonarr": "onEpisodeFileDelete"},
	"file_replaced":             {"sonarr": "onEpisodeFileDeleteForUpgrade", "radarr": "onMovieFileDeleteForUpgrade"},
	domain.EventHealthBroken:    {"sonarr": "onHealthIssue", "radarr": "onHealthIssue"},
	domain.EventHealthRecovered: {"sonarr": "onHealthRestored", "radarr": "onHealthRestored"},
	domain.EventNeedsDecision:   {"sonarr": "onManualInteractionRequired", "radarr": "onManualInteractionRequired"},
}

// hookSwitchOn reads one of a hook's event switches from the record as
// sent.
func hookSwitchOn(hk hook, key string) bool {
	var m map[string]any
	if json.Unmarshal([]byte(hk.Body), &m) != nil {
		return false
	}
	v, _ := valueOf(m, key)
	return toBool(v, false)
}

// hookWants says whether a hook takes an event of this kind on this side.
// A file deleted for an upgrade needs the delete switch too, as in the old
// apps; a health warning needs includeHealthWarnings.
func hookWants(hk hook, kind, key string, warning bool) bool {
	if !hookSwitchOn(hk, key) {
		return false
	}
	switch {
	case kind == "file_replaced" && hk.Side == "sonarr":
		return hookSwitchOn(hk, "onEpisodeFileDelete")
	case kind == "file_replaced":
		return hookSwitchOn(hk, "onMovieFileDelete")
	case warning:
		return hookSwitchOn(hk, "includeHealthWarnings")
	}
	return true
}

// deliverOther sends an Event other than an import to the hooks that
// asked for its kind.
func (s *hookSender) deliverOther(ctx context.Context, e domain.Event) {
	switches := hookSwitches[e.Kind]
	if switches == nil {
		return
	}
	db := s.store.Read()
	if e.Kind == domain.EventHealthBroken || e.Kind == domain.EventHealthRecovered {
		body, warning := healthBody(e)
		for _, side := range []string{"sonarr", "radarr"} {
			s.fanOut(ctx, side, 0, []hookNote{{body: body}}, func(hk hook, _ hookNote) bool {
				return hookWants(hk, e.Kind, switches[side], warning)
			})
		}
		return
	}
	if e.TitleID == nil {
		return
	}
	var t mediafiles.Title
	if database.Get(ctx, db, "titles", *e.TitleID, &t) != nil {
		return
	}
	if t.State == "removed" && e.Kind != domain.EventTitleRemoved {
		return
	}
	side := map[string]string{"show": "sonarr", "movie": "radarr"}[t.Kind]
	key := switches[side]
	if key == "" {
		return
	}
	body, err := s.otherBody(ctx, e, t, side)
	if err != nil {
		log.Printf("arrapi: hooks for title %d: %v", t.ID, err)
		return
	}
	if body == nil {
		return
	}
	s.fanOut(ctx, side, t.ID, []hookNote{{body: body}}, func(hk hook, _ hookNote) bool { return hookWants(hk, e.Kind, key, false) })
}

// healthBody is the Health or HealthRestored event, and whether it is only
// a warning.
func healthBody(e domain.Event) ([]byte, bool) {
	state, _ := e.Data["state"].(string)
	kind, _ := e.Data["kind"].(string)
	msg, _ := e.Data["message"].(string)
	if msg == "" {
		msg = e.Summary
	}
	ev := map[string]any{"eventType": "Health", "instanceName": eventName, "level": "error", "message": msg, "type": kind, "wikiUrl": nil}
	warning := state == "warning"
	if warning {
		ev["level"] = "warning"
	}
	if e.Kind == domain.EventHealthRecovered {
		ev["eventType"], warning = "HealthRestored", false
		if was, _ := e.Data["was"].(string); was == "warning" {
			ev["level"] = "warning"
		}
	}
	b, _ := json.Marshal(ev)
	return b, warning
}

// otherBody builds the event for one title. Nil means there is nothing to
// send (an event with no episodes of the main Version, say).
func (s *hookSender) otherBody(ctx context.Context, e domain.Event, t mediafiles.Title, side string) ([]byte, error) {
	db := s.store.Read()
	ev := map[string]any{"instanceName": eventName}
	labels, err := tagLabelsOf(ctx, db, t.ID)
	if err != nil {
		return nil, err
	}
	main, lf := mainVersionOf(ctx, db, t.ID)
	folder := ""
	if main != nil {
		folder = path.Join(lf.Path, main.FolderName)
	}
	if side == "sonarr" {
		ev["series"] = &hookSeries{ID: t.ID, Title: t.Name, Path: folder, TvdbID: sourceID(t.SourceIDs, "tvdb"), TmdbID: sourceID(t.SourceIDs, "tmdb"),
			ImdbID: t.SourceIDs["imdb"], Year: derefInt(t.Year), Tags: labels}
	} else {
		ev["movie"] = &hookMovie{ID: t.ID, Title: t.Name, Year: derefInt(t.Year), FolderPath: folder, TmdbID: sourceID(t.SourceIDs, "tmdb"),
			ImdbID: t.SourceIDs["imdb"], Tags: labels}
	}
	episodes := func(slots []int64) ([]hookEpisode, error) {
		if side != "sonarr" || len(slots) == 0 {
			return nil, nil
		}
		of, err := mediafiles.SlotItems(ctx, db, slots)
		if err != nil {
			return nil, err
		}
		var items []int64
		for _, id := range of {
			items = append(items, id)
		}
		return episodesOfItems(ctx, db, t.ID, items)
	}
	slots := int64sOf(e.Data["slot_ids"])
	if len(slots) == 0 && e.SlotID != nil {
		slots = []int64{*e.SlotID}
	}
	fileOf := func(id int64) (map[string]any, bool) {
		var f mediafiles.File
		if id <= 0 || database.Get(ctx, db, "files", id, &f) != nil || main == nil {
			return nil, false
		}
		mf := mediaFileOf(side, f, lf.Path, main.FolderName)
		return map[string]any{"id": f.ID, "relativePath": mf.RelativePath, "path": path.Join(folder, mf.RelativePath),
			"quality": mf.Quality["quality"].(wireQuality).Name, "qualityVersion": 1, "releaseGroup": mf.ReleaseGroup,
			"sceneName": mf.SceneName, "size": f.Size, "dateAdded": utc(f.AddedAt)}, true
	}
	switch e.Kind {
	case "download_sent":
		ev["eventType"] = "Grab"
		name, _ := e.Data["release"].(string)
		q, _ := qualityOfRelease(side, name)["quality"].(wireQuality)
		rel := map[string]any{"quality": q.Name, "qualityVersion": 1, "releaseTitle": name, "releaseGroup": releaseGroupOf(name), "indexer": "", "size": 0,
			"customFormats": []string{}, "customFormatScore": 0, "indexerFlags": []string{}}
		if id := int64Of(e.Data["release_id"]); id > 0 {
			facts, _ := sources.ReleaseFactsOf(ctx, db, []int64{id})
			if f, ok := facts[id]; ok && f.FromSource {
				rel["size"], rel["indexer"] = f.Size, f.SourceName
			}
		}
		ev["release"] = rel
		ev["downloadClient"], _ = e.Data["downloader"].(string)
		ev["downloadClientType"], ev["downloadId"] = "", ""
		if e.DownloadID != nil {
			if ext, kind, err := downloads.DownloadClient(ctx, db, *e.DownloadID); err == nil {
				ev["downloadId"], ev["downloadClientType"] = ext, kind
			}
		}
		ev["customFormatInfo"] = map[string]any{"customFormats": []any{}, "customFormatScore": 0}
		if side == "radarr" {
			ev["remoteMovie"] = map[string]any{"tmdbId": sourceID(t.SourceIDs, "tmdb"), "imdbId": t.SourceIDs["imdb"], "title": t.Name, "year": derefInt(t.Year)}
		} else {
			eps, err := episodes(slots)
			if err != nil || len(eps) == 0 {
				return nil, err
			}
			ev["episodes"] = eps
		}
	case domain.EventRenamed:
		ev["eventType"] = "Rename"
		f, ok := fileOf(int64Of(e.Data["file_id"]))
		if !ok {
			return nil, nil
		}
		from, _ := e.Data["from"].(string)
		f["previousRelativePath"], f["previousPath"] = from, path.Join(lf.Path, from)
		if from != "" && main != nil {
			f["previousRelativePath"] = strings.TrimPrefix(strings.TrimPrefix(from, main.FolderName), "/")
		}
		if side == "radarr" {
			ev["renamedMovieFiles"] = []any{f}
		} else {
			ev["renamedEpisodeFiles"] = []any{f}
		}
	case domain.EventTitleAdded:
		if side == "radarr" {
			ev["eventType"], ev["addMethod"] = "MovieAdded", "manual"
		} else {
			ev["eventType"] = "SeriesAdd"
		}
	case domain.EventTitleRemoved:
		deleted, _ := e.Data["delete_files"].(bool)
		ev["deletedFiles"] = deleted
		if side == "radarr" {
			ev["eventType"], ev["movieFolderSize"] = "MovieDelete", 0
		} else {
			ev["eventType"] = "SeriesDelete"
		}
	case domain.EventEpisodesRemoved:
		ev["eventType"], ev["deleteReason"] = "EpisodeFileDelete", "manual"
		eps, err := episodesOfItems(ctx, db, t.ID, int64sOf(e.Data["item_ids"]))
		if err != nil || len(eps) == 0 {
			return nil, err
		}
		ev["episodes"] = eps
		ev["episodeFile"] = map[string]any{"id": 0, "relativePath": "", "path": folder, "quality": "Unknown", "qualityVersion": 1, "size": 0}
	case "file_replaced":
		ev["deleteReason"] = "upgrade"
		f := map[string]any{"id": int64Of(e.Data["old_file_id"]), "relativePath": "", "path": "", "quality": "Unknown", "qualityVersion": 1, "size": 0}
		if moved, _ := e.Data["moved_to"].(string); moved != "" {
			f["recycleBinPath"] = moved
		}
		if r, _ := e.Data["old_release"].(string); r != "" {
			f["sceneName"] = r
		}
		if side == "radarr" {
			ev["eventType"], ev["movieFile"] = "MovieFileDelete", f
		} else {
			eps, err := episodes(slots)
			if err != nil || len(eps) == 0 {
				return nil, err
			}
			ev["eventType"], ev["episodeFile"], ev["episodes"] = "EpisodeFileDelete", f, eps
		}
	case domain.EventNeedsDecision:
		ev["eventType"] = "ManualInteractionRequired"
		reason, _ := e.Data["reason"].(string)
		if reason == "" {
			reason = e.Summary
		}
		ev["downloadStatus"] = "warning"
		ev["downloadStatusMessages"] = []map[string]any{{"title": t.Name, "messages": []string{reason}}}
		ev["downloadClient"], ev["downloadClientType"], ev["downloadId"] = "", "", ""
		ev["customFormatInfo"] = map[string]any{"customFormats": []any{}, "customFormatScore": 0}
		if side == "sonarr" {
			eps, err := episodes(slots)
			if err != nil {
				return nil, err
			}
			ev["episodes"] = eps
		}
	default:
		return nil, nil
	}
	b, err := json.Marshal(ev)
	return b, err
}

// mainVersionOf reads a title's main Version and its Library folder; nil
// when it has none.
func mainVersionOf(ctx context.Context, db *sql.DB, titleID int64) (*mediafiles.Version, mediafiles.LibraryFolder) {
	var lf mediafiles.LibraryFolder
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? AND is_main = 1", titleID)
	if err != nil || len(vs) == 0 {
		return nil, lf
	}
	_ = database.Get(ctx, db, "library_folders", vs[0].LibraryFolderID, &lf)
	return &vs[0], lf
}

// episodesOfItems is the old apps' episode list for some Items of a show,
// in season and episode order.
func episodesOfItems(ctx context.Context, db database.Lister, seriesID int64, items []int64) ([]hookEpisode, error) {
	if len(items) == 0 {
		return nil, nil
	}
	its, err := database.Select[mediafiles.Item](ctx, db, "items", "kind = 'episode' AND id IN "+idList(items))
	if err != nil {
		return nil, err
	}
	var out []hookEpisode
	for _, it := range its {
		ep := hookEpisode{ID: it.ID, SeriesID: seriesID, AirDateUtc: utcPtr(it.AirDate)}
		if it.Numbers.Season != nil {
			ep.SeasonNumber = *it.Numbers.Season
		}
		if it.Numbers.Episode != nil {
			ep.EpisodeNumber = *it.Numbers.Episode
		}
		if it.Name != nil {
			ep.Title = *it.Name
		}
		if it.AirDate != nil {
			ep.AirDate = it.AirDate.UTC().Format("2006-01-02")
		}
		out = append(out, ep)
	}
	slices.SortFunc(out, func(a, b hookEpisode) int {
		if a.SeasonNumber != b.SeasonNumber {
			return a.SeasonNumber - b.SeasonNumber
		}
		return a.EpisodeNumber - b.EpisodeNumber
	})
	return out, nil
}

// releaseGroupOf reads the group from a release name.
func releaseGroupOf(name string) string {
	return releasename.Parse(name, releasename.Hints{}).Group.Value
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

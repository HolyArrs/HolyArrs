package parts

import (
	"strings"
	"sync"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// Screens gives every page of the web app (its path, with [x] for a
// changing part) its owner. A test fails for a page file or a menu entry
// missing here. The web app reads this table from GET /api/v1/parts.
var screenOwners = map[string]Owner{
	"/people/[id]":                   movies,
	"/library/collections/[id]":      movies,
	"/library/copies":                movies,
	"/":                              Core,
	"/explore":                       Core,
	"/popular":                       Media,
	"/dev/ui":                        Core,
	"/account":                       Core,
	"/welcome":                       Core,
	"/login":                         Core,
	"/invite/[token]":                Core,
	"/downloads":                     Core,
	"/downloads/history":             Core,
	"/health":                        Core,
	"/downloads/decide":              Media,
	"/downloads/turned-down":         Core,
	"/library":                       Media,
	"/library/[id]":                  ByTitle,
	"/library/[id]/albums/[album]":   music,
	"/library/held":                  Media,
	"/library/deleted":               Media,
	"/library/turned-down":           Media,
	"/library/import":                Media,
	"/library/import-files":          Media,
	"/library/rename":                Media,
	"/library/missing-subtitles":     subtitles,
	"/search":                        Media,
	"/title/[kind]/[tmdb]":           Media,
	"/music/artist/[mbid]":           music,
	"/music/genre/[slug]":            music,
	"/music/[kind=musicpage]/[slug]": music,
	"/music/album/[mbid]":            music,
	"/calendar":                      Media,
	"/requests":                      wishlist,
	"/requests/reports":              Core,
	"/settings":                      Core,
	"/settings/account":              Core,
	"/settings/appearance":           Core,
	"/settings/danger":               Core,
	"/settings/health":               Core,
	"/settings/alerts":               Core,
	"/settings/backups":              Core,
	"/settings/move":                 Core,
	"/settings/downloaders":          Core,
	"/settings/library-folders":      Core,
	"/settings/media-servers":        Core,
	"/settings/parts":                Core,
	"/settings/sign-in":              Core,
	"/settings/sources":              Core,
	"/settings/sources/[id]":         Core,
	"/settings/this-app":             Core,
	"/settings/logs":                 Core,
	"/settings/schedules":            Core,
	"/settings/api":                  Core,
	"/settings/ai":                   Core,
	"/ai":                            Core,
	"/settings/tmdb":                 Core,
	"/settings/users":                Core,
	"/settings/naming":               Media,
	"/settings/pick-rules":           Media,
	"/settings/pick-rules/[id]":      Media,
	"/settings/title-defaults":       Media,
	"/settings/outside-lists":        Media,
	"/settings/search":               Media,
	"/settings/music":                music,
	"/settings/subtitles":            subtitles,
	"/settings/requests":             wishlist,
}

// Loops gives every supervised background loop and every other stage the
// health page shows (worker pools, guarded work) its owner. A loop of a
// part stops while the part is off.
var loopOwners = map[string]Owner{
	"backups":          Core,
	"downloads":        Core,
	"schedules":        Core,
	"sessions":         Core,
	"log_secrets":      Core,
	"health":           Core,
	"alerts":           Core,
	"events":           Core,
	"media_servers":    Core,
	"jobs_core":        Core,
	"jobs_io":          Core,
	"jobs_heavy":       Core,
	"jobs_migrate":     Core,
	"stranded_imports": Media,
	"gates":            movies,
	"wishlist":         wishlist,
	"music_warm":       music,
	"trailer_pass":     Media,
}

// JobKinds gives every Job kind its owner. A Job of a part that is off
// waits in the queue (it is not lost) and runs once the part is on again;
// a scheduled kind of a part that is off is not queued.
var jobOwners = map[string]Owner{
	"health_check":          Core,
	"check_library_folders": Core,
	"clean_history":         Core,
	"check_bridges":         Core,
	// A move checks each part itself (a part that is off is left out).
	"migrate_move": Core,
	"migrate_undo": Core,
	// Shared work: on while any of Movies, TV shows and Music is on.
	"search_missing":     Media,
	"feed_check":         Media,
	"grab_held":          Media,
	"sync_outside_lists": Media,
	"rename_files":       Media,
	"clean_holding":      Media,
	"rescan_library":     Media,
	"recheck_pick_rules": Media,
	// The title (or Library folder) the Job is about decides.
	"search_slot":         ByTitle,
	"grab":                ByTitle,
	"import_download":     ByTitle,
	"refresh_title":       ByTitle,
	"scan_title":          ByTitle,
	"move_title":          ByTitle,
	"scan_library_folder": ByTitle,
	"adopt_library":       ByTitle,
	// One part.
	"refresh_titles":   movies,
	"refresh_shows":    shows,
	"search_season":    shows,
	"refresh_artists":  music,
	"artist_pictures":  music,
	"search_album":     music,
	"subtitle_search":  subtitles,
	"subtitle_missing": subtitles,
	"subtitle_better":  subtitles,
	"subtitle_look":    subtitles,
	// Requests.
	"sync_plex_watchlists": wishlist,
}

// ScreenOwner finds the owner of a web path such as /library/12: the
// table row that matches, with [x] standing for one path segment; a row
// with fewer [x] segments wins (/library/held over /library/[id]).
func ScreenOwner(path string) (Owner, bool) {
	got := strings.Split(strings.Trim(path, "/"), "/")
	best, bestWild := Owner(""), -1
	for pattern, o := range startup.OwnersOf(startup.ScreenOwners) {
		want := strings.Split(strings.Trim(pattern, "/"), "/")
		if len(want) != len(got) {
			continue
		}
		wild := 0
		for i := range want {
			switch {
			case strings.HasPrefix(want[i], "["):
				wild++
			case want[i] != got[i]:
				wild = -1
			}
			if wild < 0 {
				break
			}
		}
		if wild >= 0 && (bestWild < 0 || wild < bestWild) {
			best, bestWild = o, wild
		}
	}
	return best, bestWild >= 0
}

// SafeLibrary are the routes that stay reachable in safe mode although
// every part is off: the Library list and one title's pages (read), and
// editing or removing a title. Nothing optional runs through them: Jobs
// they queue wait for the next normal start.
var SafeLibrary = map[string]bool{
	"GET /api/v1/titles":               true,
	"GET /api/v1/titles/owned":         true,
	"GET /api/v1/home":                 true,
	"GET /api/v1/artwork/{name}":       true,
	"GET /api/v1/titles/{id}":          true,
	"PUT /api/v1/titles/{id}":          true,
	"POST /api/v1/titles/{id}/delete":  true,
	"GET /api/v1/titles/{id}/seasons":  true,
	"GET /api/v1/titles/{id}/episodes": true,
	"GET /api/v1/titles/{id}/albums":   true,
	"GET /api/v1/albums/{id}":          true,
	"GET /api/v1/albums/{id}/tracks":   true,
}

// SafeScreens are the pages that show in safe mode for the same reason.
var SafeScreens = []string{"/library", "/library/[id]", "/library/[id]/albums/[album]"}

// Register adds the five parts and their route, screen, loop and Job kind
// owners to the registry. The program wiring calls it once before anything
// reads the registry; calling it again does nothing.
func Register() {
	registerOnce.Do(register)
}

var registerOnce sync.Once

func register() {
	for _, p := range []struct {
		part  Part
		label string
	}{{Movies, "Movies"}, {Shows, "TV shows"}, {Music, "Music"}, {Subtitles, "Subtitles"}, {Wishlist, "Requests"}} {
		startup.RegisterPart(p.part, p.label)
	}
	startup.RegisterOwners(startup.RouteOwners, routeOwners)
	startup.RegisterOwners(startup.ScreenOwners, screenOwners)
	startup.RegisterOwners(startup.LoopOwners, loopOwners)
	startup.RegisterOwners(startup.JobOwners, jobOwners)
}

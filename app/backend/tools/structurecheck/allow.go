package structurecheck

// oversized lists files allowed over the 800-line cap. It stays empty.
var oversized = map[string]bool{}

// bannedNameAllowed lists folders that keep a banned name until their
// task moves them.
var bannedNameAllowed = map[string]bool{}

// layers is the module layer order, bottom first. A module imports only
// modules on a lower layer, and only their root package. Core packages sit
// below layer 1 and are ordered by coreRows.
var layers = map[string]int{
	"suggest":      1, // picks and orders rows and shelves
	"metadata":     1,
	"accounts":     2,
	"mediafiles":   3,
	"importer":     4, // file import: unpack, tags, matching; below library
	"library":      5, // the Library: folders, adopt, rename
	"sources":      6,
	"downloads":    7,
	"subtitles":    8,
	"mediaservers": 8,
	"alerts":       8,
	"movies":       9,
	"shows":        9,
	"music":        11,
	"requests":     10,
	"discover":     13,
	"arrapi":       11,
	"migrate":      11,
	"ai":           11,
	"backups":      11,
	"health":       10,
	"updates":      11,
	"setup":        11,
}

// coreRows orders the core packages. A core package imports only core
// packages on its own row or a lower one.
var coreRows = map[string]int{
	"database":    0,
	"files":       0,
	"releasename": 0,
	"outbound":    1,
	"secrets":     1,
	"logs":        2,
	"jobs":        2,
	"model":       2,
	"webserver":   3,
	"startup":     3,
	"parts":       4,
}

// packageHome says where each package under internal/ ends up. A package
// that is not yet a module or a core package is judged by its future home,
// so an import that breaks the layer order shows up before the move. A dash
// means the package is split or dissolved and is not judged.
var packageHome = map[string]string{
	"ratings":       "metadata",
	"youtube":       "metadata",
	"judge":         "mediafiles",
	"naming":        "mediafiles",
	"mediainfo":     "mediafiles",
	"tagset":        "mediafiles",
	"importsecrets": "migrate",
	"compat":        "arrapi",
	"doctor":        "health",
	"backup":        "backups",
}

// tableOwner names the package that owns each table: a module, or the core
// package for the shared tables. Only the owner's store files may name the
// table in SQL.
var tableOwner = map[string]string{
	// shared tables, owned by core
	"settings":        "core/database",
	"events":          "core/database",
	"schedules":       "core/jobs",
	"jobs":            "core/jobs",
	"secrets":         "core/secrets",
	"secrets_retired": "core/secrets",

	// accounts
	"users":                   "accounts",
	"sessions":                "accounts",
	"api_keys":                "accounts",
	"backup_codes":            "accounts",
	"invites":                 "accounts",
	"known_places":            "accounts",
	"sso_links":               "accounts",
	"user_links":              "accounts",
	"user_avatars":            "accounts",
	"user_profiles":           "accounts",
	"explore_counts":          "accounts",
	"explore_category_counts": "accounts",
	"explore_title_counts":    "accounts",
	"explore_prefs":           "accounts",

	// metadata
	"lookup_cache":   "metadata",
	"explore_pool":   "metadata",
	"explore_titles": "metadata",
	"card_art":       "metadata",

	// mediafiles
	"titles":                  "mediafiles",
	"items":                   "mediafiles",
	"versions":                "mediafiles",
	"slots":                   "mediafiles",
	"files":                   "mediafiles",
	"parts":                   "mediafiles",
	"library_folders":         "mediafiles",
	"never_add":               "mediafiles",
	"own_traits":              "mediafiles",
	"pick_rules":              "mediafiles",
	"movie_collections":       "mediafiles",
	"movie_collection_titles": "mediafiles",
	"pressings":               "mediafiles",
	"imports":                 "mediafiles",
	"rename_steps":            "mediafiles",
	"title_search":            "mediafiles",

	// sources
	"sources":      "sources",
	"releases":     "sources",
	"source_calls": "sources",
	"bridges":      "sources",

	// downloads
	"downloads":           "downloads",
	"downloaders":         "downloads",
	"downloader_attempts": "downloads",
	"dl_items":            "downloads",
	"dl_match_queue":      "downloads",
	"torrent_log":         "downloads",
	"torrent_seen":        "downloads",
	"client_links":        "downloads",

	// subtitles
	"subtitles":            "subtitles",
	"subtitle_looks":       "subtitles",
	"subtitle_saves":       "subtitles",
	"subtitle_sites":       "subtitles",
	"subtitle_turned_down": "subtitles",
	"subtitle_wishes":      "subtitles",

	// mediaservers
	"media_servers": "mediaservers",

	// alerts
	"alerts": "alerts",

	// requests
	"asks":                  "requests",
	"ask_parts":             "requests",
	"ask_people":            "requests",
	"user_ask_settings":     "requests",
	"messages":              "requests",
	"reports":               "requests",
	"follows":               "requests",
	"tmdb_follows":          "requests",
	"tmdb_movie_follows":    "requests",
	"album_follows":         "requests",
	"new_album_notices":     "requests",
	"new_episode_notices":   "requests",
	"plex_watchlist_items":  "requests",
	"plex_watchlist_people": "requests",
	"outside_lists":         "requests",

	// music
	"music_packages":       "music",
	"music_package_albums": "music",
	"music_package_sync":   "music",

	// health
	"health_checks": "health",

	// migrate
	"moves":             "migrate",
	"move_made":         "migrate",
	"move_credentials":  "migrate",
	"migrate_snapshots": "migrate",
	"import_created":    "migrate",

	// arrapi
	"compat_keys": "accounts", // key checks run in auth, below arrapi
	"compat_tags": "arrapi",
}

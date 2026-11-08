package structurecheck

// oversized lists files over the 800-line cap that a later task splits.
// Remove an entry when its file is split.
var oversized = map[string]bool{
	"app/frontend/src/lib/downloads/DlSections.svelte":       true,
	"app/backend/internal/migrate/apply.go":                  true,
	"app/backend/internal/migrate/plan.go":                   true,
	"app/frontend/src/lib/TitlePage.svelte":                  true,
	"app/backend/internal/api/musicpages.go":                 true,
	"app/frontend/src/lib/ai-apps.ts":                        true,
	"app/backend/internal/auth/auth.go":                      true,
	"app/backend/internal/downloader/manager.go":             true,
	"app/backend/internal/importer/importer.go":              true,
	"app/backend/internal/api/api.go":                        true,
	"app/frontend/src/lib/music/MusicArtistPage.svelte":      true,
	"app/backend/internal/backup/backup.go":                  true,
	"app/backend/internal/api/exploremusic_v3.go":            true,
	"app/backend/internal/asks/asks.go":                      true,
	"app/backend/cmd/holyarrs/main.go":                       true,
	"app/backend/internal/source/shows.go":                   true,
	"app/backend/internal/api/explore_new.go":                true,
	"app/frontend/src/lib/types.ts":                          true,
	"app/backend/internal/importer/shows.go":                 true,
	"app/backend/internal/api/downloads.go":                  true,
	"app/backend/tools/demo/showcase.go":                     true,
	"app/backend/internal/api/trailer.go":                    true,
	"app/backend/internal/api/explore.go":                    true,
	"app/backend/internal/api/exploremusic_v7.go":            true,
	"app/frontend/src/lib/explore.ts":                        true,
	"app/backend/internal/api/exploremusic.go":               true,
	"app/backend/internal/music/rowtpl/catalog.go":           true,
	"app/backend/internal/metadata/discover.go":              true,
	"app/backend/internal/library/rename.go":                 true,
	"app/backend/internal/auth/perms.go":                     true,
	"app/backend/internal/source/source.go":                  true,
	"app/frontend/src/lib/SigninBackdrop.svelte":             true,
	"app/backend/internal/music/rowtpl/page.go":              true,
	"app/frontend/src/lib/Account.svelte":                    true,
	"app/backend/internal/compat/shows.go":                   true,
	"app/backend/internal/api/titles.go":                     true,
	"app/backend/internal/library/adopt.go":                  true,
	"app/backend/internal/api/asks.go":                       true,
	"app/backend/internal/auth/users.go":                     true,
	"app/backend/internal/library/artist.go":                 true,
	"app/backend/internal/migrate/arr.go":                    true,
	"app/backend/internal/auth/outside.go":                   true,
	"app/backend/internal/judge/judge.go":                    true,
	"app/frontend/src/lib/approvals/ApprovalsDesktop.svelte": true,
	"app/backend/internal/metadata/musicbrainz.go":           true,
	"app/backend/internal/api/explore_new_music.go":          true,
	"app/backend/internal/parts/table.go":                    true,
}

// bannedNameAllowed lists folders that keep a banned name until their
// task moves them.
var bannedNameAllowed = map[string]bool{
	"app/backend/internal/domain": true,
}

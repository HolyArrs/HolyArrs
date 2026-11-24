package mediafiles

// What each TRaSH pattern names the two sample releases: the full one, then the bare one.
var trashMovieFileWant = [][3]string{
	{"emby-anime-imdb", "The Matrix Reloaded (2003) [imdb-tt0234215] - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][JA+EN][DV HDR10][x265 10bit]-EVOLVE.mkv", "The Matrix Reloaded (2003) - [WEBDL-1080p].mkv"},
	{"emby-anime-tmdb", "The Matrix Reloaded (2003) [tmdb-604] - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][JA+EN][DV HDR10][x265 10bit]-EVOLVE.mkv", "The Matrix Reloaded (2003) [tmdb-604] - [WEBDL-1080p].mkv"},
	{"emby-imdb", "The Matrix Reloaded (2003) [imdb-tt0234215] - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) - [WEBDL-1080p].mkv"},
	{"emby-tmdb", "The Matrix Reloaded (2003) [tmdb-604] - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) [tmdb-604] - [WEBDL-1080p].mkv"},
	{"jellyfin-anime-imdb", "The Matrix Reloaded (2003) [imdbid-tt0234215] - {Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][JA+EN][DV HDR10][x265 10bit]-EVOLVE.mkv", "The Matrix Reloaded (2003) - [WEBDL-1080p].mkv"},
	{"jellyfin-anime-tmdb", "The Matrix Reloaded (2003) [tmdbid-604] - {Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][JA+EN][DV HDR10][x265 10bit]-EVOLVE.mkv", "The Matrix Reloaded (2003) [tmdbid-604] - [WEBDL-1080p].mkv"},
	{"jellyfin-imdb", "The Matrix Reloaded (2003) [imdbid-tt0234215] - {Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) - [WEBDL-1080p].mkv"},
	{"jellyfin-tmdb", "The Matrix Reloaded (2003) [tmdbid-604] - {Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) [tmdbid-604] - [WEBDL-1080p].mkv"},
	{"original", "The.Matrix.Reloaded.2003.2160p.BluRay-EVOLVE.mkv", "Unnamed.mkv"},
	{"p2p-scene", "The.Matrix.Reloaded.2003.Directors.Cut.Bluray-2160p.Proper.TrueHD.7.1.DV.HDR10.x265-EVOLVE.mkv", "The.Matrix.Reloaded.2003.WEBDL-1080p.mkv"},
	{"plex-anime-imdb", "The Matrix Reloaded (2003) {imdb-tt0234215} - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][JA+EN][DV HDR10][x265 10bit]-EVOLVE.mkv", "The Matrix Reloaded (2003) - [WEBDL-1080p].mkv"},
	{"plex-anime-tmdb", "The Matrix Reloaded (2003) {tmdb-604} - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][JA+EN][DV HDR10][x265 10bit]-EVOLVE.mkv", "The Matrix Reloaded (2003) {tmdb-604} - [WEBDL-1080p].mkv"},
	{"plex-edition-alt-imdb", "The Matrix Reloaded (2003) {imdb-tt0234215} - {Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) - [WEBDL-1080p].mkv"},
	{"plex-edition-alt-tmdb", "The Matrix Reloaded (2003) {tmdb-604} - {Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) {tmdb-604} - [WEBDL-1080p].mkv"},
	{"plex-imdb", "The Matrix Reloaded (2003) {imdb-tt0234215} - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) - [WEBDL-1080p].mkv"},
	{"plex-tmdb", "The Matrix Reloaded (2003) {tmdb-604} - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) {tmdb-604} - [WEBDL-1080p].mkv"},
	{"standard", "The Matrix Reloaded (2003) - {Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "The Matrix Reloaded (2003) - [WEBDL-1080p].mkv"},
}

var trashMovieFolderWant = [][3]string{
	{"default", "The Matrix Reloaded (2003)", "The Matrix Reloaded (2003)"},
	{"emby-imdb", "The Matrix Reloaded (2003) [imdb-tt0234215]", "The Matrix Reloaded (2003)"},
	{"emby-tmdb", "The Matrix Reloaded (2003) [tmdb-604]", "The Matrix Reloaded (2003) [tmdb-604]"},
	{"jellyfin-imdb", "The Matrix Reloaded (2003) [imdbid-tt0234215]", "The Matrix Reloaded (2003)"},
	{"jellyfin-tmdb", "The Matrix Reloaded (2003) [tmdbid-604]", "The Matrix Reloaded (2003) [tmdbid-604]"},
	{"plex-imdb", "The Matrix Reloaded (2003) {imdb-tt0234215}", "The Matrix Reloaded (2003)"},
	{"plex-tmdb", "The Matrix Reloaded (2003) {tmdb-604}", "The Matrix Reloaded (2003) {tmdb-604}"},
}

var trashShowFolderWant = [][3]string{
	{"default", "The Matrix Reloaded (2003)", "The Matrix Reloaded (2003)"},
	{"emby-imdb", "The Matrix Reloaded (2003) [imdb-tt0234215]", "The Matrix Reloaded (2003)"},
	{"emby-tvdb", "The Matrix Reloaded (2003) [tvdb-81189]", "The Matrix Reloaded (2003)"},
	{"jellyfin-tvdb", "The Matrix Reloaded (2003) [tvdbid-81189]", "The Matrix Reloaded (2003)"},
	{"plex-imdb", "The Matrix Reloaded (2003) {imdb-tt0234215}", "The Matrix Reloaded (2003)"},
	{"plex-tvdb", "The Matrix Reloaded (2003) {tvdb-81189}", "The Matrix Reloaded (2003)"},
}

var trashEpisodeWant = [][3]string{
	{"anime/default", "X/Season 01/The Matrix Reloaded (2003) - S01E02-E03 - 012-013 - The Mill + Flood [Bluray-2160p Proper][TrueHD 7.1][JA+EN][DV HDR10][x265 10bit]-EVOLVE.mkv", "X/Season 01/The Matrix Reloaded (2003) - S01E02 - 012 - The Mill [WEBDL-1080p].mkv"},
	{"daily/default", "X/Season 2026/The Matrix Reloaded (2003) - 2026-03-14 - The Mill + Flood [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "X/Season 2026/The Matrix Reloaded (2003) - 2026-03-14 - The Mill [WEBDL-1080p].mkv"},
	{"daily/original", "X/Season 2026/The.Matrix.Reloaded.2003.2160p.BluRay-EVOLVE.mkv", "X/Season 2026/Unnamed.mkv"},
	{"standard/default", "X/Season 01/The Matrix Reloaded (2003) - S01E02-E03 - The Mill + Flood [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv", "X/Season 01/The Matrix Reloaded (2003) - S01E02 - The Mill [WEBDL-1080p].mkv"},
	{"standard/original", "X/Season 01/The.Matrix.Reloaded.2003.2160p.BluRay-EVOLVE.mkv", "X/Season 01/Unnamed.mkv"},
	{"standard/p2p-scene", "X/Season 01/The.Matrix.Reloaded.2003.S01E02-E03.The.Mill.+.Flood.Bluray-2160p.Proper.TrueHD.7.1.DV.HDR10.x265-EVOLVE.mkv", "X/Season 01/The.Matrix.Reloaded.2003.S01E02.The.Mill.WEBDL-1080p.mkv"},
}

package migrate

import "github.com/HolyArrs/holyarrs/app/backend/internal/library"

// starting is a size table row as a fresh old app fills it, in MB a
// minute; max 0 means no largest size.
type starting struct{ min, max float64 }

// startingSizes are the size tables a fresh Sonarr and Radarr start with
// (see the manuals' size table sections).
var startingSizes = map[Kind]map[string]starting{
	Sonarr: {
		"Unknown": {1, 199.9}, "SDTV": {2, 100}, "WEBRip-480p": {2, 100}, "WEBDL-480p": {2, 100}, "DVD": {2, 100},
		"Bluray-480p": {2, 100}, "Bluray-576p": {2, 100}, "HDTV-720p": {3, 125}, "HDTV-1080p": {4, 125}, "Raw-HD": {4, 0},
		"WEBRip-720p": {3, 130}, "WEBDL-720p": {3, 130}, "Bluray-720p": {4, 130}, "WEBRip-1080p": {4, 130},
		"WEBDL-1080p": {4, 130}, "Bluray-1080p": {4, 155}, "Bluray-1080p Remux": {35, 0}, "HDTV-2160p": {35, 199.9},
		"WEBRip-2160p": {35, 0}, "WEBDL-2160p": {35, 0}, "Bluray-2160p": {35, 0}, "Bluray-2160p Remux": {35, 0},
	},
	Radarr: radarrStarting(),
}

func radarrStarting() map[string]starting {
	out := map[string]starting{}
	for _, q := range []string{"Unknown", "WORKPRINT", "CAM", "TELESYNC", "TELECINE", "REGIONAL", "DVDSCR", "SDTV", "DVD", "DVD-R",
		"WEBDL-480p", "WEBRip-480p", "Bluray-480p", "Bluray-576p", "HDTV-720p", "WEBDL-720p", "WEBRip-720p", "Bluray-720p",
		"HDTV-1080p", "WEBDL-1080p", "WEBRip-1080p"} {
		out[q] = starting{0, 100}
	}
	for _, q := range []string{"Bluray-1080p", "Remux-1080p", "HDTV-2160p", "WEBDL-2160p", "WEBRip-2160p", "Bluray-2160p", "Remux-2160p", "BR-DISK", "Raw-HD"} {
		out[q] = starting{0, 0}
	}
	return out
}

// sizesChanged says someone changed the old app's size table from the one
// it starts with. Rows of a quality not in the starting table (a renamed
// or newer one) are not counted as a change.
func sizesChanged(kind Kind, rows []library.OldQualitySize) bool {
	start := startingSizes[kind]
	near := func(a, b float64) bool { return a-b < 0.05 && b-a < 0.05 }
	for _, r := range rows {
		d, ok := start[r.Quality]
		if ok && (!near(r.Min, d.min) || !near(r.Max, d.max)) {
			return true
		}
	}
	return false
}

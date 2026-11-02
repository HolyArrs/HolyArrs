package lists

import (
	"fmt"
	"strings"
	"time"
)

// Kind is what a source reads.
type Kind string

const (
	KindSongs   Kind = "songs"   // daily song chart (top 200)
	KindAlbums  Kind = "albums"  // weekly album chart
	KindArtists Kind = "artists" // weekly artist chart
	KindNew     Kind = "new"     // new releases (first page)
	// KindCategory is a Browse category page: the ids of its editorial
	// lists (read monthly; internal only, never served).
	KindCategory Kind = "category"
	// KindList is one editorial list of a Browse tile (slot n of its
	// category page), read weekly.
	KindList Kind = "list"
)

// Tile is one Browse tile in our own words: our key, the category page
// that feeds it (internal id) and whether it is a mood.
type Tile struct {
	Key, Category string
	Mood          bool
}

// Tiles are the tiles with editorial lists, in budget priority: the
// biggest genres and the two mood rows first, so a small month cap still
// fills Explore's genre and mood rows.
var Tiles = []Tile{
	{Key: "pop", Category: "0JQ5DAqbMKFEC4WFtoNRpw"},
	{Key: "latin", Category: "0JQ5DAqbMKFxXaXKP7zcDp"},
	{Key: "rock", Category: "0JQ5DAqbMKFDXXwE9BDJAr"},
	{Key: "country", Category: "0JQ5DAqbMKFKLfwjuJMoNC"},
	{Key: "dance", Category: "0JQ5DAqbMKFHOzuVTgTizF"},
	{Key: "rnb", Category: "0JQ5DAqbMKFEZPnFQSFB1T"},
	{Key: "chill", Category: "0JQ5DAqbMKFFzDl7qN9Apr", Mood: true},
	{Key: "party", Category: "0JQ5DAqbMKFA6SOHvT3gck", Mood: true},
	{Key: "indie", Category: "0JQ5DAqbMKFCWjUTdzaG0e"},
	{Key: "decades", Category: "0JQ5DAqbMKFIVNxQgRNSg0"},
	{Key: "fresh_faces", Category: "0JQ5DAqbMKFImHYGo3eTSg"},
	{Key: "kpop", Category: "0JQ5DAqbMKFGvOw3O4nLAf"},
	{Key: "metal", Category: "0JQ5DAqbMKFDkd668ypn6O"},
	{Key: "jazz", Category: "0JQ5DAqbMKFAJ5xb0fwo9m"},
	{Key: "workout", Category: "0JQ5DAqbMKFAXlCG6QvYQ4", Mood: true},
	{Key: "love", Category: "0JQ5DAqbMKFAUsdyVjCQuL", Mood: true},
	{Key: "focus", Category: "0JQ5DAqbMKFCbimwdOYlsl", Mood: true},
	{Key: "sleep", Category: "0JQ5DAqbMKFCuoRTxhYWow", Mood: true},
	{Key: "alternative", Category: "0JQ5DAqbMKFFtlLYUHv8bT"},
	{Key: "folk", Category: "0JQ5DAqbMKFy78wprEpAjl"},
	{Key: "soul", Category: "0JQ5DAqbMKFIpEuaCnimBj"},
	{Key: "classical", Category: "0JQ5DAqbMKFPrEiAOxgac3"},
	{Key: "punk", Category: "0JQ5DAqbMKFAjfauKLOZiv"},
	{Key: "blues", Category: "0JQ5DAqbMKFQiK2EHwyjcU"},
	{Key: "funk", Category: "0JQ5DAqbMKFFsW9N8maB6z"},
	{Key: "ambient", Category: "0JQ5DAqbMKFLjmiZRss79w"},
	{Key: "feelings", Category: "0JQ5DAqbMKFzHmL4tf05da", Mood: true},
	{Key: "afro", Category: "0JQ5DAqbMKFNQ0fGp4byGU"},
	{Key: "brazil", Category: "0JQ5DAqbMKFNT6Du8qM9kt"},
}

// freeTiles is how many tiles get a list on a small cap (one list each):
// the biggest genres and the mood pages that lean on lists most.
const freeTiles = 18

// SeasonTile is the one seasonal slot: its category page is the active
// paid season's (SeasonTiles), read when the season changes.
var SeasonTile = Tile{Key: "season", Mood: true}

// SeasonTiles are the paid seasons' category pages (internal ids, found
// on the Browse grid; a season without an id here is skipped).
var SeasonTiles = map[string]string{"halloween": "0JQ5DAqbMKFBpcMbuTfhR7"}

// ActiveSeason answers the paid season on at t (north windows: the lists
// are read for the US), or "".
func ActiveSeason(t time.Time) string {
	md := int(t.UTC().Month())*100 + t.UTC().Day()
	switch {
	case md >= 1001 && md <= 1031:
		return "halloween"
	case md >= 1115 && md <= 1226:
		return "christmas"
	case md >= 621 && md <= 922:
		return "summer"
	}
	return ""
}

// Source is one list we keep, with its refresh period.
type Source struct {
	Key    string // our key, e.g. "songs:us", "list:pop:0"
	Kind   Kind
	Region string // "global" or a lower-case ISO country
	Every  time.Duration
	// Tile and Slot name a category or list source's tile and its list
	// number on the category page (lists only).
	Tile Tile
	Slot int
}

// Path is the source's request path and query.
func (s Source) Path() string {
	c := strings.ToUpper(s.Region)
	switch s.Kind {
	case KindSongs:
		return "/top_200_tracks?country=" + c + "&period=daily"
	case KindAlbums:
		return "/top_albums?country=" + c + "&period=weekly"
	case KindArtists:
		return "/top_artists?country=" + c + "&period=weekly"
	case KindCategory:
		return "/browse/categories/" + s.Tile.Category + "?country=US"
	case KindList:
		return "" // resolved from the stored category page (listPath)
	}
	return "/browse/new-releases?country=" + c
}

// ListPath is a list source's request path for one list id.
func ListPath(id string) string { return "/playlist_tracks?id=" + id }

const (
	day       = 24 * time.Hour
	daily     = day
	weekly    = 7 * day
	twiceWeek = 84 * time.Hour
	monthly   = 31 * day // category pages: one call a month
)

// monthCost is the calls one source spends in a 31-day month.
func monthCost(every time.Duration) int {
	n := int((31*day + every - 1) / every)
	return max(n, 1)
}

// TopRegions are the chart regions in priority order: the biggest
// streaming countries first. Global leads.
var TopRegions = []string{"global", "us", "gb", "de", "br", "mx", "ca", "au", "fr",
	"es", "it", "nl", "se", "jp", "in", "kr", "ph", "id", "ar", "co", "cl", "pl", "tr", "za", "ng",
	"no", "dk", "fi", "be", "at", "ch", "ie", "nz", "pt", "pe", "ec", "ve", "do", "gt", "cr", "pa",
	"py", "uy", "bo", "hn", "sv", "ni", "cz", "sk", "hu", "ro", "bg", "gr", "il", "ae", "sa", "eg",
	"ma", "my", "sg", "th", "vn", "tw", "hk", "pk", "ua", "lt", "lv", "ee", "is", "lu", "kz"}

// firstWave is how many regions get their daily song chart before any
// weekly list beyond global and the first country.
const firstWave = 9

// Plan picks the sources in priority order whose month cost fits in 90%
// of capMonth (10% stays for retries): daily song charts of the top
// regions first, then weekly album and artist charts for global and the
// US, new releases, the other first-wave weeklies, then more regions.
// New releases are read for the US only: the source's other countries
// answer lists one to two years old (checked 2026-10-26), so every region
// shows the US list, filled up with ListenBrainz's fresh releases.
func Plan(capMonth int) []Source { return planWith(capMonth, false) }

// planWith is the plan; songsBackup leaves the song charts out of the
// budget (another reader keeps them, and the source's are only its backup).
func planWith(capMonth int, songsBackup bool) []Source {
	budget := capMonth * 9 / 10
	var out []Source
	used := 0
	add := func(k Kind, region string, every time.Duration) {
		c := monthCost(every)
		if k == KindSongs && songsBackup {
			c = 0
		}
		if used+c > budget {
			return
		}
		used += c
		out = append(out, Source{Key: string(k) + ":" + region, Kind: k, Region: region, Every: every})
	}
	wave := TopRegions[:firstWave]
	for _, r := range wave {
		add(KindSongs, r, daily)
	}
	for _, r := range wave[:2] {
		add(KindAlbums, r, weekly)
		add(KindArtists, r, weekly)
	}
	add(KindNew, "us", twiceWeek)
	for _, r := range wave[2:] {
		add(KindAlbums, r, weekly)
		add(KindArtists, r, weekly)
	}
	// Editorial lists, a small share: each tile's category page monthly
	// and its first list weekly (more lists on a bigger cap, below).
	tile := func(t Tile, slot int) {
		cat := Source{Key: "category:" + t.Key, Kind: KindCategory, Region: "global", Every: monthly, Tile: t}
		lst := Source{Key: fmt.Sprintf("list:%s:%d", t.Key, slot), Kind: KindList, Region: "global", Every: weekly, Tile: t, Slot: slot}
		c := monthCost(lst.Every)
		if slot == 0 {
			c += monthCost(cat.Every)
		}
		if used+c > budget {
			return
		}
		used += c
		if slot == 0 {
			out = append(out, cat)
		}
		out = append(out, lst)
	}
	for _, t := range Tiles[:freeTiles] {
		tile(t, 0)
	}
	tile(SeasonTile, 0)
	for _, r := range TopRegions[firstWave:] {
		add(KindSongs, r, daily)
		add(KindAlbums, r, weekly)
		add(KindArtists, r, weekly)
	}
	if capMonth >= 3000 {
		for _, t := range Tiles[freeTiles:] {
			tile(t, 0)
		}
		for _, slot := range []int{1, 2} {
			for _, t := range Tiles {
				tile(t, slot)
			}
		}
	}
	return out
}

// PlanCost is the month cost of a plan.
func PlanCost(p []Source) int {
	n := 0
	for _, s := range p {
		n += monthCost(s.Every)
	}
	return n
}

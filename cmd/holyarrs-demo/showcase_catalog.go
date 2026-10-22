package main

import "github.com/CoderLuii/holyarrs/internal/parse"

// The showcase catalog: real, well-known titles by their TMDB and
// MusicBrainz ids (so art loads), each with the Library state it shows.
// SEED.md beside the showcase scripts lists the same in words.

func parseName(release string) parse.Result { return parse.Parse(release, parse.Hints{}) }

// Showcase-only movie plans (the lab plans are in catalog.go).
const (
	planGone        = "gone"        // a file adopted, then deleted from disk (File missing)
	planDownloading = "downloading" // grabbed, downloading at Progress
	planQueued      = "queued"      // grabbed, waiting in the Downloader
	planPaused      = "paused"      // grabbed, paused in the Downloader at Progress
	planStalled     = "stalled"     // grabbed, no progress for hours
)

type showcaseMovie struct {
	TMDB     string
	Name     string
	Scenario string  // what it demonstrates (SEED.md)
	Quality  string  // a file at this quality is adopted ("" none); see qualityTags
	Plan     string  // what happens after it is added (grab, failed, decide, upgrade, gone, downloading, ...)
	Progress float64 // unfinished downloads
	FourK    bool    // a 4K Version is wanted next to the main one
	Keep     string  // "off": Keep up to date off
	Ask      string  // "asked", "approved", "turned_down": comes in through the Wishlist instead
	AskedBy  string
}

var showcaseMovieList = []showcaseMovie{
	// In the Library.
	{TMDB: "603", Name: "The Matrix", Scenario: "in Library, 1080p Blu-ray", Quality: "bluray1080"},
	{TMDB: "438631", Name: "Dune", Scenario: "in Library, 4K HDR", Quality: "uhd"},
	{TMDB: "872585", Name: "Oppenheimer", Scenario: "in Library, 4K Dolby Vision/HDR web", Quality: "web2160"},
	{TMDB: "157336", Name: "Interstellar", Scenario: "in Library 1080p, 4K Version still wanted", Quality: "bluray1080", FourK: true},
	// Wanted, released, not on disk.
	{TMDB: "680", Name: "Pulp Fiction", Scenario: "wanted: released, not on disk"},
	{TMDB: "769", Name: "GoodFellas", Scenario: "wanted: released, not on disk"},
	// A movie series (The Lord of the Rings collection).
	{TMDB: "120", Name: "The Lord of the Rings: The Fellowship of the Ring", Scenario: "movie series: in Library", Quality: "bluray1080"},
	{TMDB: "121", Name: "The Lord of the Rings: The Two Towers", Scenario: "movie series: in Library", Quality: "bluray1080"},
	{TMDB: "122", Name: "The Lord of the Rings: The Return of the King", Scenario: "movie series: wanted"},
	// Downloads.
	{TMDB: "155", Name: "The Dark Knight", Scenario: "downloading 63%", Plan: planDownloading, Progress: 0.63},
	{TMDB: "27205", Name: "Inception", Scenario: "downloading 12%", Plan: planDownloading, Progress: 0.12},
	{TMDB: "335984", Name: "Blade Runner 2049", Scenario: "queued in the Downloader", Plan: planQueued},
	{TMDB: "76341", Name: "Mad Max: Fury Road", Scenario: "paused at 48%", Plan: planPaused, Progress: 0.48},
	{TMDB: "348", Name: "Alien", Scenario: "stalled at 31% (no progress for 9 hours)", Plan: planStalled, Progress: 0.31},
	{TMDB: "545611", Name: "Everything Everywhere All at Once", Scenario: "failed download", Plan: planFailed},
	{TMDB: "98", Name: "Gladiator", Scenario: "failed download", Plan: planFailed},
	{TMDB: "274", Name: "The Silence of the Lambs", Scenario: "import needs a decision (disc image, no video)", Plan: planDecide},
	{TMDB: "329865", Name: "Arrival", Scenario: "import needs a decision (disc image, no video)", Plan: planDecide},
	{TMDB: "329", Name: "Jurassic Park", Scenario: "grabbed and imported (History)", Plan: planGrab, Quality: "bluray1080"},
	{TMDB: "807", Name: "Se7en", Scenario: "720p replaced by a 1080p copy (History)", Plan: planUpgrade, Quality: "hdtv720"},
	// Better copy wanted.
	{TMDB: "240", Name: "The Godfather Part II", Scenario: "better copy wanted (720p below good enough)", Quality: "bluray720"},
	{TMDB: "78", Name: "Blade Runner", Scenario: "better copy wanted (720p HDTV)", Quality: "hdtv720"},
	// Keep up to date off.
	{TMDB: "238", Name: "The Godfather", Scenario: "not kept up to date, has a file", Quality: "bluray1080", Keep: "off"},
	{TMDB: "13", Name: "Forrest Gump", Scenario: "not kept up to date, no file", Keep: "off"},
	// File gone from disk.
	{TMDB: "105", Name: "Back to the Future", Scenario: "file missing (deleted from disk)", Quality: "bluray1080", Plan: planGone},
	{TMDB: "85", Name: "Raiders of the Lost Ark", Scenario: "file missing (deleted from disk)", Quality: "web1080", Plan: planGone},
	// Kid limit: R-rated titles the Kid cannot see; G/PG titles they can.
	{TMDB: "550", Name: "Fight Club", Scenario: "R: hidden from the Kid member", Quality: "bluray1080"},
	{TMDB: "862", Name: "Toy Story", Scenario: "G: visible to the Kid member", Quality: "bluray1080"},
	// Wishlist.
	{TMDB: "496243", Name: "Parasite", Scenario: "asked by a member, waiting for approval", Ask: "asked", AskedBy: "Maya"},
	{TMDB: "419430", Name: "Get Out", Scenario: "asked by a member, waiting for approval", Ask: "asked", AskedBy: "Maya"},
	{TMDB: "346648", Name: "Paddington 2", Scenario: "asked by the Kid, waiting for approval", Ask: "asked", AskedBy: "Leo (Kid)"},
	{TMDB: "120467", Name: "The Grand Budapest Hotel", Scenario: "asked, approved, looking for a copy", Ask: "approved", AskedBy: "Maya"},
	{TMDB: "354912", Name: "Coco", Scenario: "asked by the Kid, approved, looking", Ask: "approved", AskedBy: "Leo (Kid)"},
	{TMDB: "280", Name: "Terminator 2: Judgment Day", Scenario: "asked, turned down with a note", Ask: "turned_down", AskedBy: "Maya"},
}

// showcaseUpcoming are found by name: the newest match not out yet.
var showcaseUpcoming = []string{"Avengers: Doomsday", "Spider-Man: Beyond the Spider-Verse", "The Batman Part II", "Dune: Part Three"}

type showcaseShow struct {
	showEntry
	Scenario string
	Keep     string // "off": Keep up to date off
	Specials int    // only these first specials (season 0) have files and are kept
}

func sc(tmdb, name, plan string, seasons, episodes int, order, choice, quality, scenario string) showcaseShow {
	return showcaseShow{showEntry: showEntry{TMDB: tmdb, Name: name, Plan: plan, Seasons: seasons, Episodes: episodes, Order: order, Choice: choice, Quality: quality}, Scenario: scenario}
}

var showcaseShowList = []showcaseShow{
	sc("1396", "Breaking Bad", showComplete, 0, 0, "aired", "all", "bluray1080", "ended, complete"),
	sc("87108", "Chernobyl", showComplete, 0, 0, "aired", "all", "web2160", "miniseries, complete in 4K"),
	sc("456", "The Simpsons", showAiring, 0, 0, "aired", "latest", "web1080", "huge (35+ seasons), continuing, latest season only"),
	sc("2190", "South Park", showAiring, 0, 0, "aired", "latest", "web1080", "continuing, next episode upcoming"),
	sc("66732", "Stranger Things", showPartial, 3, 0, "aired", "all", "web1080", "partial: 3 seasons on disk, later ones missing"),
	sc("2316", "The Office", showPartial, 5, 3, "aired", "all", "web1080", "partial: 5 seasons plus 3 episodes of season 6"),
	sc("95480", "Slow Horses", showPartial, 2, 3, "aired", "all", "web1080", "partial: episode bars at different fills"),
	sc("100088", "The Last of Us", showPartial, 1, 2, "aired", "all", "web1080", "episodes downloading now (S02E03, S02E04)"),
	sc("95396", "Severance", showPartial, 1, 0, "aired", "all", "web2160", "season 2 missing"),
	sc("136315", "The Bear", showPartial, 2, 0, "aired", "all", "web1080", "later seasons missing"),
	sc("1398", "The Sopranos", showPartial, 2, 4, "aired", "all", "dvd", "ended, partial at DVD quality"),
	sc("1416", "Grey's Anatomy", showPartial, 2, 0, "aired", "all", "web1080", "huge (20+ seasons), mostly missing"),
	sc("209867", "Frieren: Beyond Journey's End", showPartial, 0, 20, "absolute", "all", "web1080", "anime, absolute numbering"),
	sc("85937", "Demon Slayer: Kimetsu no Yaiba", showPartial, 1, 0, "absolute", "all", "web1080", "anime, absolute numbering"),
	{showEntry: showEntry{TMDB: "1668", Name: "Friends", Plan: showPartial, Order: "aired", Choice: "all", Quality: "web1080"}, Scenario: "not kept up to date", Keep: "off"},
	{showEntry: showEntry{TMDB: "57243", Name: "Doctor Who", Plan: showPartial, Order: "aired", Choice: "none", Quality: "web1080"}, Scenario: "specials only", Specials: 4},
	sc("97546", "Ted Lasso", showAiring, 0, 0, "aired", "latest", "web1080", "latest season kept; earlier seasons asked for by a member"),
}

// showcaseEpisodeDownloads are episodes downloading now.
var showcaseEpisodeDownloads = []struct {
	TMDB            string
	Season, Episode int
	State           string
	Progress        float64
}{
	{"100088", 2, 3, "downloading", 0.55},
	{"100088", 2, 4, "queued", 0},
	{"95480", 3, 4, "downloading", 0.18},
}

// showcaseShowAsks ask for seasons of shows.
var showcaseShowAsks = []struct {
	TMDB, Name, AskedBy string
	Seasons             []int
}{
	{"97546", "Ted Lasso", "Maya", []int{1, 2}},
	{"119051", "Wednesday", "Maya", nil},
}

type showcaseArtist struct {
	MBID        string
	Name        string
	Scenario    string
	Albums      int      // studio albums kept
	NewAlbums   string   // all, new, none
	Complete    bool     // every kept album has placeholder tracks
	Downloading bool     // the newest kept album is downloading
	Ask         bool     // asked for by a member (Maya) instead of added
	Follow      []string // who follows the artist once it is in the Library: "owner" or a member name
}

var showcaseArtistList = []showcaseArtist{
	{MBID: "cc2c9c3c-b7bc-4b8b-84d8-4fbd8779e493", Name: "Adele", Scenario: "complete: every kept album on disk", Albums: 4, NewAlbums: "new", Complete: true},
	{MBID: "a74b1b7f-71a5-4011-9441-d0b5e4122711", Name: "Radiohead", Scenario: "albums missing", Albums: 5, NewAlbums: "new"},
	{MBID: "83d91898-7763-47d7-b03b-b92132375c47", Name: "Pink Floyd", Scenario: "albums missing", Albums: 4, NewAlbums: "none"},
	{MBID: "056e4f3e-d505-4dad-8ec1-d04f521cbb56", Name: "Daft Punk", Scenario: "album downloading", Albums: 3, NewAlbums: "new", Downloading: true},
	{MBID: "20244d07-534f-4eff-b4d4-930878889970", Name: "Taylor Swift", Scenario: "new albums kept; upcoming album when MusicBrainz lists one", Albums: 3, NewAlbums: "all", Follow: []string{"Maya"}},
	{MBID: "f4abc0b5-3f7a-4eff-8f78-ac078dbce533", Name: "Billie Eilish", Scenario: "asked for by a member", Ask: true},
	{MBID: "381086ea-f511-4aba-bdf9-71c753dc5077", Name: "Kendrick Lamar", Scenario: "asked for by a member", Ask: true},
	// Music part 2 (2026-10-22): well-known artists so the personal Explore
	// music rows have something to show. "Albums: 30" keeps every studio album.
	{MBID: "ada7a83c-e3e1-40f1-93f9-3e73dbc9298a", Name: "Arctic Monkeys", Scenario: "indie: 4 albums on disk, the rest missing", Albums: 4, NewAlbums: "new", Complete: true, Follow: []string{"owner"}},
	{MBID: "b8a7c51f-362c-4dcb-a259-bc6e0095f0a6", Name: "Ed Sheeran", Scenario: "pop: 3 albums on disk, the rest missing", Albums: 3, NewAlbums: "new", Complete: true, Follow: []string{"Maya"}},
	{MBID: "0383dadf-2a4e-4d10-a46a-e9e041da8eb3", Name: "Queen", Scenario: "rock: 5 albums on disk, the rest missing", Albums: 5, NewAlbums: "none", Complete: true, Follow: []string{"Maya"}},
	{MBID: "cc197bad-dc9c-440d-a5b5-d52ba2e14234", Name: "Coldplay", Scenario: "rock: 4 albums on disk, the rest missing", Albums: 4, NewAlbums: "new", Complete: true, Follow: []string{"owner"}},
	{MBID: "b95ce3ff-3d05-4e87-9e01-c97b66af13d4", Name: "Eminem", Scenario: "hip-hop: 5 albums on disk, the rest missing", Albums: 5, NewAlbums: "none", Complete: true},
	{MBID: "9fff2f8a-21e6-47de-a2b8-7f449929d43f", Name: "Drake", Scenario: "hip-hop: 4 albums on disk, the rest missing", Albums: 4, NewAlbums: "new", Complete: true, Follow: []string{"owner"}},
	{MBID: "164f0d73-1234-4e2c-8743-d77bf2191051", Name: "Kanye West", Scenario: "hip-hop: 3 albums on disk, the rest missing", Albums: 3, NewAlbums: "none", Complete: true},
	{MBID: "73e5e69d-3554-40d8-8516-00cb38737a1c", Name: "Rihanna", Scenario: "R&B: 4 albums on disk, the rest missing", Albums: 4, NewAlbums: "none", Complete: true},
	{MBID: "859d0860-d480-4efd-970c-c05d5f1776b8", Name: "Beyoncé", Scenario: "R&B: 3 albums on disk, the rest missing", Albums: 3, NewAlbums: "new", Complete: true},
	{MBID: "c8b03190-306c-4120-bb0b-6f2ebfc06ea9", Name: "The Weeknd", Scenario: "R&B: 3 albums on disk, the rest missing", Albums: 3, NewAlbums: "new", Complete: true, Follow: []string{"owner", "Maya"}},
	{MBID: "b1e26560-60e5-4236-bbdb-9aa5a8d5ee19", Name: "Post Malone", Scenario: "hip-hop/pop: every album on disk", Albums: 30, NewAlbums: "new", Complete: true},
	{MBID: "b7539c32-53e7-4908-bda3-81449c367da6", Name: "Lana Del Rey", Scenario: "indie: every album on disk", Albums: 30, NewAlbums: "new", Complete: true, Follow: []string{"owner"}},
	{MBID: "afb680f2-b6eb-4cd7-a70b-a63b25c763d5", Name: "Bruno Mars", Scenario: "pop: every album on disk", Albums: 30, NewAlbums: "new", Complete: true},
	{MBID: "63aa26c3-d59b-4da4-84ac-716b54f1ef4d", Name: "Tame Impala", Scenario: "electronic/indie: asked for by a member", Ask: true},
	{MBID: "5b11f4ce-a62d-471e-81fc-a69a8278c7da", Name: "Nirvana", Scenario: "rock: albums kept, none on disk", Albums: 3, NewAlbums: "none"},
}

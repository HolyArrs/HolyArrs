package main

// The demo catalog: well-known titles by their metadata ids. Each entry
// names what it should look like in the Library. The expected name guards
// against a wrong id: an entry whose looked-up name does not match is
// skipped and said.

// Movie plans.
const (
	planHave     = "have"     // a file at a good enough quality (adopted)
	planLow      = "low"      // a file below good enough: a better copy is sought
	planMissing  = "missing"  // no file yet
	planGrab     = "grab"     // missing, then grabbed and imported (History)
	planUpgrade  = "upgrade"  // a low file, then a better copy grabbed and imported over it
	planHeld     = "held"     // missing, with a Release held until its wait is over
	planTurned   = "turned"   // missing, with Releases turned down by hand
	planFailed   = "failed"   // grabbed, the download failed, still missing
	planUpcoming = "upcoming" // not out yet (found by name: the newest match)
)

type movieEntry struct {
	TMDB    string
	Name    string // expected name (start of it)
	Plan    string
	Quality string // release tag for the file or the grab, see qualityTags
	FourK   string // "": no 4K Version; "want": a 4K Version wanted; "have": a 4K copy grabbed and imported
}

// qualityTags are the release name tails used for files and grabs.
var qualityTags = map[string]string{
	"bluray1080": "1080p.BluRay.x264-SPARKS",
	"bluray720":  "720p.BluRay.x264-AMIABLE",
	"web1080":    "1080p.WEB-DL.DDP5.1.H.264-NTb",
	"webrip1080": "1080p.WEBRip.x265-RARBG",
	"hdtv720":    "720p.HDTV.x264-KILLERS",
	"dvd":        "DVDRip.XviD-DiAMOND",
	"uhd":        "2160p.UHD.BluRay.x265.10bit.HDR.DTS-HD.MA.5.1-SWTYBLZ",
	"web2160":    "2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX",
}

var demoMovies = []movieEntry{
	{"603", "The Matrix", planHave, "bluray1080", "have"},
	{"27205", "Inception", planHave, "bluray1080", "want"},
	{"157336", "Interstellar", planHave, "uhd", ""},
	{"155", "The Dark Knight", planHave, "bluray1080", "want"},
	{"680", "Pulp Fiction", planHave, "bluray1080", ""},
	{"550", "Fight Club", planUpgrade, "dvd", ""},
	{"13", "Forrest Gump", planHave, "web1080", ""},
	{"278", "The Shawshank Redemption", planHave, "bluray1080", ""},
	{"238", "The Godfather", planHave, "bluray1080", ""},
	{"240", "The Godfather Part II", planLow, "bluray720", ""},
	{"11", "Star Wars", planHave, "bluray1080", "want"},
	{"1891", "The Empire Strikes Back", planHave, "bluray1080", ""},
	{"329", "Jurassic Park", planGrab, "bluray1080", ""},
	{"105", "Back to the Future", planHave, "bluray1080", ""},
	{"348", "Alien", planHave, "bluray1080", ""},
	{"78", "Blade Runner", planLow, "hdtv720", ""},
	{"335984", "Blade Runner 2049", planHave, "uhd", ""},
	{"98", "Gladiator", planGrab, "web1080", ""},
	{"120", "The Lord of the Rings: The Fellowship of the Ring", planHave, "bluray1080", "have"},
	{"121", "The Lord of the Rings: The Two Towers", planHave, "bluray1080", ""},
	{"122", "The Lord of the Rings: The Return of the King", planMissing, "", ""},
	{"129", "Spirited Away", planHave, "bluray1080", ""},
	{"496243", "Parasite", planHeld, "web1080", ""},
	{"244786", "Whiplash", planHave, "webrip1080", ""},
	{"76341", "Mad Max: Fury Road", planHave, "bluray1080", "want"},
	{"274", "The Silence of the Lambs", planMissing, "", ""},
	{"769", "GoodFellas", planHave, "bluray1080", ""},
	{"807", "Se7en", planUpgrade, "hdtv720", ""},
	{"280", "Terminator 2: Judgment Day", planTurned, "", ""},
	{"85", "Raiders of the Lost Ark", planHave, "bluray1080", ""},
	{"62", "2001: A Space Odyssey", planHeld, "uhd", ""},
	{"862", "Toy Story", planHave, "bluray1080", ""},
	{"10681", "WALL·E", planGrab, "bluray1080", ""},
	{"872585", "Oppenheimer", planHave, "web2160", ""},
	{"545611", "Everything Everywhere All at Once", planFailed, "web1080", ""},
	{"419430", "Get Out", planMissing, "", ""},
	{"329865", "Arrival", planMissing, "", ""},
	{"354912", "Coco", planTurned, "", ""},
	{"324857", "Spider-Man: Into the Spider-Verse", planHave, "web1080", ""},
	{"361743", "Top Gun: Maverick", planGrab, "web2160", ""},
	{"120467", "The Grand Budapest Hotel", planHeld, "bluray1080", ""},
	{"438631", "Dune", planHave, "uhd", ""},
}

// demoUpcoming are found by name: the newest match not out yet.
var demoUpcoming = []string{"Avengers: Doomsday", "Dune: Part Three", "The Batman Part II", "Shrek 5", "Spider-Man: Beyond the Spider-Verse"}

// Show plans.
const (
	showComplete = "complete" // every aired episode has a file
	showPartial  = "partial"  // the first Seasons seasons, then Episodes more of the next
	showAiring   = "airing"   // latest season only; aired ones have files but the last
)

type showEntry struct {
	TMDB     string
	Name     string
	Plan     string
	Seasons  int // partial: whole seasons with files
	Episodes int // partial: episodes of the next season with files
	Order    string
	Choice   string // season choice (all, latest, ...)
	Quality  string
	Low      int // the first Low seasons are below good enough (a better copy is sought)
}

var demoShows = []showEntry{
	{"1396", "Breaking Bad", showComplete, 0, 0, "aired", "all", "bluray1080", 0},
	{"94605", "Arcane", showComplete, 0, 0, "aired", "all", "web2160", 0},
	{"66732", "Stranger Things", showPartial, 3, 0, "aired", "all", "web1080", 0},
	{"100088", "The Last of Us", showPartial, 1, 2, "aired", "all", "web1080", 0},
	{"95396", "Severance", showPartial, 1, 5, "aired", "all", "web2160", 0},
	{"136315", "The Bear", showPartial, 2, 0, "aired", "all", "web1080", 0},
	{"209867", "Frieren: Beyond Journey's End", showPartial, 0, 20, "absolute", "all", "web1080", 0},
	{"456", "The Simpsons", showAiring, 0, 0, "aired", "latest", "web1080", 0},
	{"95480", "Slow Horses", showPartial, 4, 0, "aired", "all", "web1080", 0},
	{"2316", "The Office", showPartial, 5, 3, "aired", "all", "web1080", 2},
}

type artistEntry struct {
	MBID   string
	Name   string
	Albums int // how many albums to keep
}

var demoArtists = []artistEntry{
	{"a74b1b7f-71a5-4011-9441-d0b5e4122711", "Radiohead", 5},
	{"056e4f3e-d505-4dad-8ec1-d04f521cbb56", "Daft Punk", 4},
	{"83d91898-7763-47d7-b03b-b92132375c47", "Pink Floyd", 5},
	{"381086ea-f511-4aba-bdf9-71c753dc5077", "Kendrick Lamar", 4},
	{"5b11f4ce-a62d-471e-81fc-a69a8278c7da", "Nirvana", 3},
	{"bd13909f-1c29-4c27-a874-d4aaf27c5b1a", "Fleetwood Mac", 4},
	{"cc2c9c3c-b7bc-4b8b-84d8-4fbd8779e493", "Adele", 4},
	{"f4abc0b5-3f7a-4eff-8f78-ac078dbce533", "Billie Eilish", 3},
}

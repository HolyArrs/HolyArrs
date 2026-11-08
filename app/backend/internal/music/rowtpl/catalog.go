package rowtpl

import (
	"math"
)

// familyMax caps rows per family on one page. The personal family gets 10 on Explore.
var familyMax = map[string]int{"yr": 1, "ch": 5, "co": 3, "po": 3, "nw": 5, "cl": 5, "de": 3, "su": 4, "cr": 6, "se": 3, "ar": 4, "vi": 1, "di": 3, "pe": 6}

// genreTile maps page keys to the name of their list tile block.
var genreTile = map[string]string{"genre:pop": "pop", "genre:latin": "latin", "genre:rock": "rock", "genre:country": "country", "genre:dance": "dance", "genre:rnb": "rnb", "genre:indie": "indie", "genre:kpop": "kpop", "genre:metal": "metal", "genre:jazz": "jazz", "genre:alternative": "alternative", "genre:folk": "folk", "genre:soul-funk": "soul", "genre:classical": "classical", "genre:punk": "punk", "genre:blues": "blues", "genre:afro": "afro",
	"mood:chill": "chill", "mood:party": "party", "mood:workout": "workout", "mood:love": "love", "mood:focus": "focus", "mood:sleep": "sleep", "mood:sad": "feelings", "activity:studying": "focus", "activity:gym": "workout", "activity:running": "workout", "decade": "decades"}

// energyActs groups the activity and mood keys that are high energy.
var energyActs = []string{"activity:gym", "activity:running", "activity:driving", "mood:party"}

// instrActs groups the activity keys that suit instrumental music.
var instrActs = []string{"activity:studying", "activity:cooking", "activity:gaming"}

// scoreActs groups the activity keys that suit film and game scores.
var scoreActs = []string{"activity:gaming", "activity:driving", "activity:studying"}

// Catalog is every row template. A row runs on a page when its Pages letter matches, its Need is met and it has a Fill.
var Catalog = []Tpl{
	// World charts.
	t("A01", "ch", 0.9, "EN", "Top {place}", "The most-played songs {place} today, as their albums and artists", "list top_now, else the region chart", "rank", "Have chip kept", "D").chart().
		fill(func(c *Ctx, _ string) L { return union(c.block("top_now"), c.charting(c.albums(c.all()), c.region())) }),
	t("A02", "ch", 0.8, "EN", "Top worldwide", "Charting across 82 countries today", "list top_world, else the many-country score", "rank", "Have chip kept", "D").chart().
		fill(func(c *Ctx, _ string) L { return union(c.block("top_world"), c.cw(c.albums(c.all()))) }),
	t("A03", "ch", 0.8, "EN", "Albums of the week", "The most-played albums {place} this week", "list albums_week", "rank", "Have chip kept", "W").chart().need("ls").
		fill(func(c *Ctx, _ string) L { return c.block("albums_week") }),
	t("A04", "ch", 0.6, "N", "Albums of the week worldwide", "The most-played albums in the world this week", "list albums_world", "rank", "hidden when the region is global", "W").chart().need("ls").
		fill(func(c *Ctx, _ string) L {
			if c.Region == "global" || c.Region == "" {
				return nil
			}
			return c.block("albums_world")
		}),
	t("A05", "ch", 0.8, "EN", "Climbing fast", "Rising on the charts {place} this week", "list climbing, else region chart gains", "momentum", "rank gain", "D").chart().
		fill(func(c *Ctx, _ string) L {
			r := c.region()
			l := c.where(c.albums(c.all()), func(i int) bool { return c.gain(i, r) >= 0.05 })
			return union(c.block("climbing"), c.by(l, func(i int) float64 { return c.gain(i, r) }))
		}),
	t("A06", "ch", 0.7, "EN", "Taking off", "New on the charts {place} in the last 7 days", "list new_entries", "entry rank", "on chart 7 days or less", "D").chart().need("ls").
		fill(func(c *Ctx, _ string) L { return c.block("new_entries") }),
	t("A07", "ch", 0.6, "N", "Number ones this month", "Hit number one {place} in the last 30 days", "list number_ones", "peak date", "lists only", "D").chart().need("ls").
		fill(func(c *Ctx, _ string) L { return c.block("number_ones") }),
	t("A08", "ch", 0.5, "EN", "Weeks on the charts", "On the charts {place} for 40 days or more", "list long_runners", "days on chart", "lists only", "D").chart().need("ls").
		fill(func(c *Ctx, _ string) L { return c.block("long_runners") }),
	t("A09", "ch", 0.5, "N", "Big comebacks", "Back on the charts {place} after 60 days or more away", "list comebacks", "popularity", "lists only", "D").chart().need("ls").
		fill(func(c *Ctx, _ string) L { return c.block("comebacks") }),
	t("A10", "co", 0.6, "EN", "Top {neighbor}", "The most-played songs {neighbor} today", "list neighbor, else the neighbour chart", "rank", "", "D").chart().
		fill(func(c *Ctx, _ string) L {
			if c.Neighbor == "" {
				return c.block("neighbor")
			}
			return union(c.block("neighbor"), c.charting(c.albums(c.all()), c.Neighbor))
		}),
	t("A11", "co", 0.5, "N", "Charting in {country}", "Big in {country} today and not on your chart", "a country chart", "rank", "not on the region chart", "D").chart().slot("country", 1).
		fill(func(c *Ctx, v string) L {
			r := c.region()
			return c.where(c.charting(c.albums(c.all()), v), func(i int) bool { _, on := c.it(i).C[r]; return !on })
		}),
	t("A12", "co", 0.6, "N", "Big across {area}", "Charting in the most {area} countries today", "area charts", "area score", "", "D").chart().slot("area", 2).
		fill(func(c *Ctx, v string) L {
			l := c.where(c.albums(c.all()), func(i int) bool { return c.areaScore(i, v) > 0 })
			return c.by(l, func(i int) float64 { return c.areaScore(i, v) })
		}),
	t("A13", "ch", 0.6, "N", "Number one somewhere this week", "Number one in at least one of 82 countries in the last 7 days", "country charts", "countries at number one, then popularity", "number one within 7 days", "D").chart().
		fill(func(c *Ctx, _ string) L {
			l := c.where(c.albums(c.all()), func(i int) bool { return c.numberOnes(i) > 0 })
			return c.by(l, func(i int) float64 { return float64(c.numberOnes(i)) + c.p(i) })
		}),
	t("A14", "nw", 0.8, "N", "Biggest new albums", "Out in the last 30 days and already charting", "list big_new_albums and charting new albums", "popularity", "30 days", "D").chart().
		fill(func(c *Ctx, _ string) L {
			return union(c.block("big_new_albums"), c.byP(c.onAnyChart(c.newer(c.albums(c.all()), 30))))
		}),
	t("A15", "ch", 0.4, "N", "EPs on the charts", "Short releases charting {place} today", "the region chart", "rank", "EPs", "D").chart().
		fill(func(c *Ctx, _ string) L { return c.typ(c.charting(c.albums(c.all()), c.region()), "EP") }),
	t("A16", "ch", 0.4, "N", "Soundtracks on the charts", "Film, TV and game music charting today", "country charts", "popularity", "soundtracks", "D").chart().
		fill(func(c *Ctx, _ string) L { return c.byP(c.onAnyChart(kalb(c, "genre:soundtracks"))) }),
	t("A17", "po", 0.6, "N", "Most played last week", "The albums listeners played most last week", "listen ranks, last week", "week rank", "", "W").
		fill(func(c *Ctx, _ string) L { return c.by(c.ranked(c.albums(c.all()), rWeek), c.wp) }),
	t("A18", "po", 0.5, "N", "Most played last month", "Last month's most-played albums, out for 2 months or more", "listen ranks, last month", "month rank", "60 days or older", "W").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.where(c.ranked(c.albums(c.all()), rMonth), func(i int) bool { d, ok := c.days(i); return ok && d >= 60 }), c.mp)
		}),
	t("A19", "ar", 0.6, "N", "Singles blowing up", "The artists behind the biggest singles {place} right now", "list singles_artists, else song chart artists", "popularity", "", "D").round().
		fill(func(c *Ctx, _ string) L {
			return union(c.block("singles_artists"), c.charting(c.artists(c.all()), c.region()))
		}),
	t("A20", "ar", 0.6, "EN", "Artists everyone is playing", "The most-played artists this week", "list artists_week, else artist listen ranks", "rank", "followed last", "W").round().
		fill(func(c *Ctx, _ string) L {
			return union(c.block("artists_week"), c.by(c.ranked(c.artists(c.all()), rWeek), c.wp))
		}),
	t("A21", "vi", 0.5, "EN", "Music videos people watch", "Artists charting {place} this week, with their music videos", "list video_artists", "rank", "video known", "W").wide().need("ls").
		fill(func(c *Ctx, _ string) L { return c.block("video_artists") }),

	// New and coming.
	t("B01", "nw", 0.9, "EN", "Out this Friday", "New albums and EPs out in the last two weeks", "list new_friday and new releases", "0.5 F14 + 0.3 popularity + 0.2 library", "14 days", "Fr").
		fill(func(c *Ctx, _ string) L {
			l := c.newer(c.albums(c.all()), 14)
			return union(c.block("new_friday"), c.by(l, func(i int) float64 { return 0.5*c.f(i, 14) + 0.3*math.Max(c.p(i), c.lp(i)) + 0.2*c.l(i) }))
		}),
	t("B02", "nw", 0.7, "N", "{year}'s biggest albums so far", "Released this year and played most so far", "listen ranks, this year", "year rank", "released this year", "W").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.ranked(c.inYear(c.albums(c.all()), c.Now.Year()), rThisYear), c.yp)
		}),
	t("B03", "nw", 0.6, "EN", "Coming soon", "Out within 90 days", "release calendar", "date", "next 90 days", "D").need("cal"),
	t("B04", "di", 0.6, "EN", "Fresh faces", "New artists with a first album out lately", "list fresh tile and debut albums", "0.5 popularity + 0.5 novelty", "not had", "W").disc().
		fill(func(c *Ctx, _ string) L {
			l := c.debuts(c.newer(c.albums(c.all()), 365))
			return union(c.block("tile:fresh_faces"), c.by(c.notHad(l), func(i int) float64 { return 0.5*math.Max(c.p(i), c.lp(i)) + 0.5*c.n(i) }))
		}),
	t("B05", "nw", 0.5, "N", "Debuts on the charts", "First albums charting today", "country charts", "popularity", "debut albums", "D").
		fill(func(c *Ctx, _ string) L { return c.byP(c.debuts(c.onAnyChart(c.albums(c.all())))) }),
	t("B06", "pe", 1.0, "E", "New from your artists", "New releases by artists you have", "library artists' new albums", "F30", "60 days", "D").need("lib").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.where(c.newer(c.albums(c.all()), 60), c.libArtist), func(i int) float64 { return c.f(i, 30) })
		}),
	t("B07", "pe", 0.8, "E", "New from artists like yours", "New albums by artists close to yours", "new albums of similar artists", "0.5 F30 + 0.5 similarity", "not had", "Fr").need("lib").
		fill(func(c *Ctx, _ string) L {
			l := c.where(c.newer(c.albums(c.all()), 60), func(i int) bool { return c.simArtist(i) > 0 && !c.libArtist(i) })
			return c.by(l, func(i int) float64 { return 0.5*c.f(i, 30) + 0.5*c.simArtist(i) })
		}),
	t("B08", "pe", 0.7, "E", "Coming soon from your artists", "Out within 90 days, from artists you have", "release calendar of library artists", "date", "", "D").need("cal"),
	t("B09", "se", 0.7, "EN", "Ready for {season}", "{season} starts soon: albums {basis}", "the season's pool", "0.5 listens + 0.5 fit", "season starts within 21 days", "D").slot("season_up", 1).
		fill(func(c *Ctx, v string) L {
			return c.by(kalb(c, v), func(i int) float64 { return 0.5*c.lp(i) + 0.5*c.t(i, v) })
		}),
	t("B10", "se", 0.8, "EN", "{season} is here", "Popular now and {basis}", "the season's pool, charting or played last week", "0.5 popularity + 0.5 fit", "season on", "D").slot("season_on", 1).
		fill(func(c *Ctx, v string) L {
			l := c.where(kalb(c, v), func(i int) bool { return len(c.it(i).C) > 0 || c.it(i).R[rWeek] > 0 })
			return c.by(l, func(i int) float64 { return 0.5*math.Max(c.p(i), c.wp(i)) + 0.5*c.t(i, v) })
		}),
	t("B11", "nw", 0.6, "N", "Back with a new album", "New albums from artists whose last album was 4 or more years ago", "new albums and the artist's earlier albums", "0.5 F30 + 0.5 artist listens", "gap of 4 years or more", "D").
		fill(func(c *Ctx, _ string) L {
			yrs := c.artistYears()
			l := c.where(c.newer(c.albums(c.all()), 60), func(i int) bool {
				y, prev := c.year(i), 0
				for _, x := range yrs[c.it(i).ArtistID] {
					if x < y && x > prev {
						prev = x
					}
				}
				return prev > 0 && y-prev >= 4
			})
			return c.by(l, func(i int) float64 { return 0.5*c.f(i, 30) + 0.5*c.lp(i) })
		}),
	t("B12", "nw", 0.7, "N", "New from artists everyone knows", "New albums from the most-played artists", "new albums by the top 10% of artists by listens", "F30", "45 days", "D").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.topArtists(c.newer(c.albums(c.all()), 45), 0.1), func(i int) float64 { return c.f(i, 30) })
		}),

	// Explore and New and Popular extras.
	t("C01", "pe", 1.0, "E", "{mix}", "From {artists} in your Library", "mix clusters of library artists", "mix score", "members charting or on a tile", "D").need("v7"),
	t("C02", "pe", 0.9, "E", "Because you have {artist}", "Artists close to {artist}: their best-known album", "similar artists of a seed, charting", "0.5 S + 0.3 popularity + 0.2 F30", "not had", "D").need("v7"),
	t("C03", "pe", 0.8, "E", "Popular near your taste", "Popular now and close to {artist}, {artist2}", "charting albums near the library", "0.5 popularity + 0.5 library", "not had", "D").need("v7"),
	t("C04", "ch", 0.7, "E", "{genre} right now", "Picked from this week's {genre} lists and charts", "list tile and the genre's charting albums", "0.5 popularity + 0.3 list + 0.2 F30", "library genres", "W").slot("libgenre", 3).
		fill(func(c *Ctx, v string) L {
			return union(c.blockIn(tileBlock(v), nil), c.by(c.onAnyChart(kalb(c, v)), func(i int) float64 { return 0.6*c.p(i) + 0.4*c.f(i, 30) }))
		}),
	t("C05", "pe", 0.8, "E", "Missing from your artists", "Studio albums by artists you have, not in your Library or asked for yet", "library artists' studio albums", "0.5 A + 0.3 popularity + 0.2 latest", "2 per artist", "D").need("v7"),
	t("C06", "di", 0.7, "E", "Rising in your genres", "Artists with a first release in the last 3 years, moving up your genres' charts", "climbing albums in library genres", "momentum × genre share", "first release within 3 years", "D").need("v7"),
	t("C07", "pe", 0.6, "E", "Deep cuts", "Lesser-known studio albums by artists you have most", "top library artists' albums", "deep cut score", "not had", "D").need("v7"),
	t("C08", "pe", 0.6, "E", "Finish the set", "Artists you have most of: the studio albums still missing", "library artists with most albums had", "date", "not had", "D").need("v7"),
	t("C09", "pe", 0.5, "E", "Anniversaries", "Albums by your artists with a round birthday around now", "library artists' albums with a round anniversary", "anniversary score", "not had", "D").need("v7"),
	t("C10", "pe", 0.5, "E", "Guest spots", "Releases your artists appear on", "credits of library artists", "affinity, then date", "not had", "D").need("v7"),
	t("C11", "pe", 0.5, "E", "Together with {artist}", "Bands, members and partners of {artist}", "artist relations of a seed", "together score", "not had", "D").need("v7"),
	t("C12", "pe", 0.7, "E", "Since you have {artist}", "You added {artist} lately: the best-known albums of artists close to them", "similar artists of a recent library artist", "0.5 S + 0.3 popularity + 0.2 genre", "not had", "D").need("v7"),
	t("C13", "di", 0.6, "EG", "Fans of {artist} also like", "Artists close to {artist}", "similar artists of a library seed", "S × popularity", "not had", "D").round().slot("seed", 1).need("seed").
		fill(func(c *Ctx, v string) L {
			sim := c.seedSim(v)
			l := c.where(c.artists(c.all()), func(i int) bool { return sim[c.it(i).ID] > 0 && !c.had(i) })
			return c.by(l, func(i int) float64 { return sim[c.it(i).ID] * (0.5 + 0.5*c.lp(i)) })
		}),
	t("C14", "cl", 0.6, "E", "{genre} greats you don't have yet", "Long-loved {genre} albums missing from your Library", "the genre's albums", "listens", "not had", "W").slot("libgenre", 1).need("lib").
		fill(func(c *Ctx, v string) L { return c.byLP(c.notHad(kalb(c, v))) }),
	t("C15", "de", 0.5, "E", "Back to the {decade}", "Long-loved albums from the decade you have most of", "the library's decade", "0.5 listens + 0.5 library", "", "W").slot("libdecade", 1).need("lib").
		fill(func(c *Ctx, v string) L {
			return c.by(kalb(c, v), func(i int) float64 { return 0.5*c.lp(i) + 0.5*c.l(i) })
		}),
	t("C16", "cl", 0.5, "E", "Throwback hits", "Long-loved albums 10 years or older", "list decades tile and older albums", "rank, then listens", "10 years or older", "W").
		fill(func(c *Ctx, _ string) L {
			return union(c.block("tile:decades"), c.byLP(c.ranked(c.older(c.albums(c.all()), 10), rWeek)))
		}),
	t("C17", "ch", 0.6, "N", "Big this week in {genre}", "The albums and artists behind this week's {genre} lists", "list tile", "rank", "", "W").slot("genre", 6).need("ls").
		fill(func(c *Ctx, v string) L { return c.blockIn(tileBlock(v), nil) }),
	t("C18", "nw", 0.7, "N", "New in {genre}", "New {genre} albums from the last 30 days", "the genre's new albums", "F30, then popularity", "30 days", "Fr").slot("genre", 6).
		fill(func(c *Ctx, v string) L {
			return c.by(c.newer(kalb(c, v), 30), func(i int) float64 { return c.f(i, 30) + 0.2*c.p(i) })
		}),

	// Genre page: charts.
	t("D01", "ch", 0.9, "G", "Top {genre} {place}", "Today's chart {place}, {genre} only", "the region chart and list top_now, in the genre", "rank", "Have chip kept", "D").chart().
		fill(func(c *Ctx, _ string) L { return union(c.blockIn("top_now", pool(c)), c.charting(palb(c), c.region())) }),
	t("D02", "ch", 0.7, "G", "Top {genre} worldwide", "Charting across 82 countries today, {genre} only", "the many-country score and list top_world, in the genre", "rank", "hidden when the region is global", "D").chart().
		fill(func(c *Ctx, _ string) L {
			if c.Region == "global" || c.Region == "" {
				return nil
			}
			return union(c.blockIn("top_world", pool(c)), c.cw(palb(c)))
		}),
	t("D03", "ch", 0.8, "GEN", "{genre} climbing {place}", "Rising on the charts {place} this week, {genre} only", "region chart gains in the genre", "momentum", "", "D").chart().slot("genre", 1).
		fill(func(c *Ctx, v string) L {
			r := c.region()
			l := c.where(kalb(c, v), func(i int) bool { return c.gain(i, r) >= 0.05 })
			return union(c.blockIn("climbing", c.key(v)), c.by(l, func(i int) float64 { return c.gain(i, r) }))
		}),
	t("D04", "co", 0.6, "G", "{genre} climbing in {country}", "Climbing the {country} chart this week, {genre} only", "a country chart's gains in the genre", "rank gain", "country other than the region", "D").chart().slot("country", 2).
		fill(func(c *Ctx, v string) L {
			l := c.where(palb(c), func(i int) bool { return c.gain(i, v) >= 0.05 })
			return c.by(l, func(i int) float64 { return c.gain(i, v) })
		}),
	t("D05", "co", 0.6, "G", "{genre} big in {country}", "Charting in {country} today and not on your chart", "a country chart in the genre", "popularity there", "not on the region chart", "D").chart().slot("country", 2).
		fill(func(c *Ctx, v string) L {
			r := c.region()
			return c.where(c.charting(palb(c), v), func(i int) bool { _, on := c.it(i).C[r]; return !on })
		}),
	t("D06", "co", 0.6, "G", "{genre} big across {area}", "Charting in the most {area} countries, {genre} only", "area charts in the genre", "area score", "", "D").chart().slot("area", 2).
		fill(func(c *Ctx, v string) L {
			l := c.where(palb(c), func(i int) bool { return c.areaScore(i, v) > 0 })
			return c.by(l, func(i int) float64 { return c.areaScore(i, v) })
		}),
	t("D07", "ch", 0.7, "G", "{genre} taking off", "New on the charts in the last 7 days, {genre} only", "list new_entries in the genre", "entry rank", "on chart 7 days or less", "D").chart().need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn("new_entries", pool(c)) }),
	t("D08", "ch", 0.6, "G", "{genre} number ones this week", "Number one in at least one country this week, {genre} only", "country charts in the genre", "countries at number one", "number one within 7 days", "D").chart().
		fill(func(c *Ctx, _ string) L {
			l := c.where(palb(c), func(i int) bool { return c.numberOnes(i) > 0 })
			return c.by(l, func(i int) float64 { return float64(c.numberOnes(i)) + c.p(i) })
		}),
	t("D09", "ch", 0.5, "G", "{genre} with staying power", "On the charts {place} for 40 days or more, {genre} only", "list long_runners in the genre", "days on chart", "lists only", "D").chart().need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn("long_runners", pool(c)) }),
	t("D10", "co", 0.6, "G", "{genre} big in many countries", "On the charts in 3 or more countries today, {genre} only", "country charts in the genre", "countries × popularity", "3 countries or more", "D").chart().
		fill(func(c *Ctx, _ string) L {
			l := c.where(palb(c), func(i int) bool { return len(c.it(i).C) >= 3 })
			return c.by(l, func(i int) float64 { return float64(len(c.it(i).C)) * (0.5 + 0.5*float64(c.it(i).P)/1000) })
		}),
	t("D11", "ch", 0.6, "G", "{genre} on this week's album chart", "The most-played albums {place} this week, {genre} only", "list albums_week in the genre", "rank", "", "W").chart().need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn("albums_week", pool(c)) }),
	t("D12", "ar", 0.6, "G", "{genre} artists on the charts", "The artists behind today's top {genre} albums and songs", "chart artists in the genre", "popularity", "", "D").round().
		fill(func(c *Ctx, _ string) L { return c.byP(c.onAnyChart(part(c))) }),
	t("D13", "po", 0.6, "G", "{genre} most played last week", "Albums listeners played most last week, {genre} only", "listen ranks, last week, in the genre", "week rank", "", "W").
		fill(func(c *Ctx, _ string) L { return c.by(c.ranked(palb(c), rWeek), c.wp) }),
	t("D14", "po", 0.5, "G", "{genre} gaining listeners", "Played more last week than last month, {genre} only", "listen ranks, week and month, in the genre", "week rank − month rank", "gain", "W").
		fill(func(c *Ctx, _ string) L {
			l := c.where(c.ranked(palb(c), rWeek), func(i int) bool { r := c.it(i).R; return r[rMonth] == 0 || r[rWeek] < r[rMonth] })
			return c.by(l, func(i int) float64 { return c.wp(i) - c.mp(i) })
		}),
	t("D15", "pe", 0.7, "G", "Your {genre} on the charts", "Albums by your artists on today's {genre} chart", "library artists' charting albums in the genre", "popularity", "Have chip kept", "D").need("lib").chart().
		fill(func(c *Ctx, _ string) L { return c.byP(c.where(c.onAnyChart(palb(c)), c.libArtist)) }),

	// Genre page: new.
	t("E01", "nw", 0.9, "GN", "New {genre} this week", "{genre} albums and EPs out in the last 7 days", "the genre's new albums", "F14, then popularity", "7 days", "D").slot("genre", 1).
		fill(func(c *Ctx, v string) L {
			return c.by(c.newer(kalb(c, v), 7), func(i int) float64 { return c.f(i, 14) + 0.3*c.p(i) })
		}),
	t("E02", "nw", 0.8, "G", "New {genre}", "{genre} albums out in the last 45 days", "the genre's new albums", "0.5 F30 + 0.3 popularity + 0.2 fit", "45 days", "D").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.newer(palb(c), 45), func(i int) float64 { return 0.5*c.f(i, 30) + 0.3*c.p(i) + 0.2*c.t(i, c.Key) })
		}),
	t("E03", "nw", 0.8, "G", "Biggest new {genre} albums", "Out in the last 30 days and already charting", "the genre's charting new albums", "popularity", "30 days", "D").
		fill(func(c *Ctx, _ string) L { return c.byP(c.onAnyChart(c.newer(palb(c), 30))) }),
	t("E04", "nw", 0.7, "G", "Out this Friday in {genre}", "New {genre} albums and EPs from the last two weeks", "list new_friday in the genre", "F14", "14 days", "Fr").need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn("new_friday", pool(c)) }),
	t("E05", "su", 0.5, "G", "Fresh {substyle}", "New {substyle} from the last 60 days", "the substyle's new albums", "F60", "60 days", "W").slot("sub", 2).
		fill(func(c *Ctx, v string) L {
			return c.by(c.newer(kalb(c, v), 60), func(i int) float64 { return c.f(i, 60) })
		}),
	t("E06", "nw", 0.5, "G", "{genre} EPs worth a listen", "Short {genre} releases from the last 90 days", "the genre's new EPs", "0.5 popularity + 0.5 F60", "90 days, EPs", "W").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.typ(c.newer(palb(c), 90), "EP"), func(i int) float64 { return 0.5*math.Max(c.p(i), c.lp(i)) + 0.5*c.f(i, 60) })
		}),
	t("E07", "nw", 0.5, "G", "First albums in {genre}", "Debut {genre} albums from the last 90 days", "the genre's new debut albums", "F90", "90 days, debuts", "W").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.debuts(c.newer(palb(c), 90)), func(i int) float64 { return c.f(i, 90) })
		}),
	t("E08", "nw", 0.7, "G", "This year's {genre} so far", "Released this year, {genre}, played most so far", "listen ranks this year, in the genre", "year rank, then listens", "released this year", "W").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.inYear(palb(c), c.Now.Year()), func(i int) float64 { return c.yp(i) + 0.5*c.lp(i) })
		}),
	t("E09", "cl", 0.5, "G", "Last year's {genre} favourites", "Released last year, with the most listeners so far", "the genre's albums from last year", "listens", "released last year", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.inYear(palb(c), c.Now.Year()-1)) }),
	t("E10", "nw", 0.5, "GN", "Coming soon in {genre}", "{genre} albums out within 90 days", "release calendar in the genre", "date", "next 90 days", "D").need("cal"),
	t("E11", "di", 0.5, "G", "Fresh faces in {genre}", "New {genre} artists with a first album out lately", "the genre's debut albums of the last year", "0.5 popularity + 0.5 novelty", "not had", "W").disc().
		fill(func(c *Ctx, _ string) L {
			return c.by(c.notHad(c.debuts(c.newer(palb(c), 365))), func(i int) float64 { return 0.5*math.Max(c.p(i), c.lp(i)) + 0.5*c.n(i) })
		}),
	t("E12", "ch", 0.7, "G", "{genre} picks", "Picked from this week's {genre} lists", "list tile", "rank", "key has a tile", "W").need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn(tileBlock(c.Key), nil) }),
	t("E13", "pe", 0.9, "G", "New from your {genre} artists", "New releases by your {genre} artists", "library artists' new albums in the genre", "F30", "60 days", "D").need("lib").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.where(c.newer(palb(c), 60), c.libArtist), func(i int) float64 { return c.f(i, 30) })
		}),
	t("E14", "nw", 0.6, "G", "New from big {genre} names", "New albums from the most-played {genre} artists", "new albums by the genre's top 10% of artists", "F30", "60 days", "D").
		fill(func(c *Ctx, _ string) L {
			top := map[string]bool{}
			for _, i := range c.topArtists(palb(c), 0.1) {
				top[c.it(i).ArtistID] = true
			}
			return c.by(c.where(c.newer(palb(c), 60), func(i int) bool { return top[c.it(i).ArtistID] }), func(i int) float64 { return c.f(i, 30) })
		}),
	t("E15", "nw", 0.5, "G", "Back in {genre} after years away", "New {genre} albums from artists away 4 years or more", "new albums and the artist's earlier albums, in the genre", "F60", "gap of 4 years or more", "W").
		fill(func(c *Ctx, _ string) L {
			yrs := c.artistYears()
			l := c.where(c.newer(palb(c), 90), func(i int) bool {
				y, prev := c.year(i), 0
				for _, x := range yrs[c.it(i).ArtistID] {
					if x < y && x > prev {
						prev = x
					}
				}
				return prev > 0 && y-prev >= 4
			})
			return c.by(l, func(i int) float64 { return c.f(i, 60) })
		}),

	// Genre page: long-time favourites.
	t("F01", "cl", 0.8, "G", "{genre} all-time greats", "The {genre} albums with the most listeners", "the genre's albums", "listens × fit", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(palb(c), c.Key) }),
	t("F02", "cl", 0.6, "G", "{genre} albums people keep playing", "Out 2 years or more and still played a lot this year", "listen ranks this year, in the genre", "year rank", "2 years or older", "W").
		fill(func(c *Ctx, _ string) L { return c.by(c.ranked(c.older(palb(c), 2), rThisYear), c.yp) }),
	t("F03", "cl", 0.7, "G", "{genre} classics", "Long-loved {genre} albums, 15 years or older", "the genre's older albums", "listens", "15 years or older", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.older(palb(c), 15)) }),
	t("F04", "cl", 0.6, "G", "Modern {genre} classics", "Long-loved {genre} albums from the last 5 to 15 years", "the genre's albums 5 to 15 years old", "listens", "5 to 15 years old", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.ageIn(palb(c), 5, 15)) }),
	t("F05", "de", 0.5, "G", "{genre} from the {decade}", "Long-loved {genre} albums from the {decade}", "the genre's albums in a decade", "listens", "", "W").slot("decade", 3).
		fill(func(c *Ctx, v string) L { return c.byLP(c.in(palb(c), v)) }),
	t("F06", "yr", 0.5, "G", "Turning {n} this year: {genre}", "{genre} albums released {n} years ago", "the genre's albums from one year", "listens", "anniversary year", "Y").slot("n", 1).
		fill(func(c *Ctx, v string) L { return c.byLP(c.inYear(palb(c), c.Now.Year()-atoi(v))) }),
	t("F07", "di", 0.5, "G", "Hidden gems in {genre}", "Well liked by those who play them, less played overall", "the genre's mid-listened albums with loyal listeners", "0.5 fit + 0.5 novelty", "not had", "W").disc().
		fill(func(c *Ctx, _ string) L {
			return c.by(c.notHad(c.loyal(c.pct(palb(c), 0.4, 0.8))), func(i int) float64 { return 0.5*c.t(i, c.Key) + 0.5*c.n(i) })
		}),
	t("F08", "cl", 0.4, "G", "{genre} soundtracks", "Film, TV and game music in {genre}", "the genre's soundtracks", "listens", "soundtracks", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.typ(palb(c), "Soundtrack")) }),
	t("F09", "su", 0.4, "G", "Instrumental {genre}", "{genre} albums tagged instrumental by listeners", "the genre's instrumental albums", "listens × fit", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(c.in(palb(c), "facet:instrumental"), "facet:instrumental") }),
	t("F10", "su", 0.4, "G", "Acoustic {genre}", "{genre} albums tagged acoustic by listeners", "the genre's acoustic albums", "listens × fit", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(c.in(palb(c), "facet:acoustic"), "facet:acoustic") }),
	t("F11", "ar", 0.5, "G", "{genre} legends", "{genre} artists with long-loved albums from 20 or more years ago", "the genre's artists", "artist listens", "most-played album 20 years or older", "W").round().
		fill(func(c *Ctx, _ string) L {
			top := c.artistTopYear()
			return c.byLP(c.where(part(c), func(i int) bool { y := top[c.it(i).ID]; return y > 0 && c.Now.Year()-y >= 20 }))
		}),
	t("F12", "pe", 0.8, "G", "Missing from your {genre} artists", "Albums by your {genre} artists, not in your Library or asked for yet", "library artists' albums in the genre", "0.5 A + 0.3 listens + 0.2 F", "not had", "D").need("lib").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.notHad(c.where(palb(c), c.libArtist)), func(i int) float64 { return 0.5*c.l(i) + 0.3*c.lp(i) + 0.2*c.f(i, 3650) })
		}),
	t("F13", "cl", 0.4, "G", "Great first albums in {genre}", "Debut {genre} albums with the most listeners", "the genre's debut albums", "listens", "debuts", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.debuts(palb(c))) }),
	t("F14", "yr", 0.4, "G", "{genre} in {year}", "The {genre} albums from {year} with the most listeners", "the genre's albums from one year", "listens", "", "W").slot("year", 2).
		fill(func(c *Ctx, v string) L { return c.byLP(c.inYear(palb(c), atoi(v))) }),

	// Genre page: styles, crossovers, people.
	t("G01", "su", 0.6, "G", "{substyle} at its best", "The {substyle} albums with the most listeners", "the substyle's albums", "listens × fit", "", "W").slot("sub", 3).
		fill(func(c *Ctx, v string) L { return c.byLPT(kalb(c, v), v) }),
	t("G02", "su", 0.6, "G", "{substyle} right now", "{substyle} played most last week or charting today", "the substyle's charting or week-ranked albums", "0.6 week rank + 0.4 popularity", "", "W").slot("sub", 2).
		fill(func(c *Ctx, v string) L {
			l := c.where(kalb(c, v), func(i int) bool { return len(c.it(i).C) > 0 || c.it(i).R[rWeek] > 0 })
			return c.by(l, func(i int) float64 { return 0.6*c.wp(i) + 0.4*c.p(i) })
		}),
	t("G03", "cr", 0.5, "G", "Where {genre} meets {genre2}", "Albums tagged both {genre} and {genre2}", "albums in both genres", "listens × fit", "", "W").slot("genre2", 1).
		fill(func(c *Ctx, v string) L { return c.byLPT(c.in(palb(c), v), v) }),
	t("G04", "cr", 0.6, "GM", "{mood} {genre}", "{genre} albums {basis}", "albums in the genre and the mood", "0.5 fit + 0.5 listens", "", "W").cross(map[byte]string{'G': "mood", 'M': "genre"}, 5).
		fill(crossFill),
	t("G05", "cr", 0.5, "GA", "{genre} {for}", "{genre} albums in styles that suit {activity}", "albums in the genre and the activity", "0.5 fit + 0.5 listens", "", "W").cross(map[byte]string{'G': "activity", 'A': "genre"}, 2).
		fill(crossFill),
	t("G06", "se", 0.6, "GS", "{season} {genre}", "{genre} albums {basis}", "albums in the genre and the season", "0.6 fit + 0.4 listens", "season on (genre page)", "W").cross(map[byte]string{'G': "season", 'S': "genre"}, 7).
		fill(crossFill),
	t("G08", "di", 0.5, "G", "{genre} fans also play", "Artists close to your {genre} artists, outside {genre}", "similar artists outside the genre", "S × popularity", "", "W").round().need("lib").disc().
		fill(func(c *Ctx, _ string) L {
			l := c.where(c.artists(c.all()), func(i int) bool { _, in := c.B.byKey[c.Key][i]; return !in && c.simArtist(i) > 0 && !c.had(i) })
			return c.by(l, func(i int) float64 { return c.simArtist(i) * (0.5 + 0.5*c.lp(i)) })
		}),
	t("G09", "ar", 0.6, "G", "Popular {genre} artists", "{genre} artists played most last week", "the genre's artists, last week", "week rank", "not in library", "W").round().
		fill(func(c *Ctx, _ string) L { return c.by(c.notHad(c.ranked(part(c), rWeek)), c.wp) }),
	t("G10", "ar", 0.5, "G", "Rising {genre} artists", "{genre} artists in last week's top, not in this year's", "the genre's artists new to the week's top", "week rank", "", "W").round().
		fill(func(c *Ctx, _ string) L {
			return c.by(c.where(c.ranked(part(c), rWeek), func(i int) bool { return c.it(i).R[rThisYear] == 0 }), c.wp)
		}),
	t("G11", "di", 0.5, "G", "Artists to know in {genre}", "{genre} artists on this week's lists, or less played but played often by their listeners", "list tile and mid-listened artists with loyal listeners", "0.5 fit + 0.5 novelty", "not had", "W").round().disc().
		fill(func(c *Ctx, _ string) L {
			b := c.artists(c.blockIn(tileBlock(c.Key), nil))
			return union(b, c.by(c.notHad(c.loyal(c.pct(part(c), 0.4, 0.8))), func(i int) float64 { return 0.5*c.t(i, c.Key) + 0.5*c.n(i) }))
		}),
	t("G12", "pe", 0.7, "G", "Your artists in {genre}", "Artists in your Library tagged {genre} or charting in it", "library artists in the genre", "affinity", "", "L").round().need("lib").
		fill(func(c *Ctx, _ string) L { return c.by(c.where(part(c), c.libArtist), c.l) }),
	t("G13", "pe", 0.7, "GMDA", "More like {artist}", "Albums by artists close to {artist}", "similar artists of a library seed, in the page", "S × popularity", "not had", "D").slot("seed", 1).need("seed").
		fill(func(c *Ctx, v string) L {
			sim := c.seedSim(v)
			l := c.where(palb(c), func(i int) bool { return sim[c.it(i).ArtistID] > 0 && !c.had(i) })
			return c.by(l, func(i int) float64 { return sim[c.it(i).ArtistID] * (0.5 + 0.5*c.lp(i)) })
		}),
	t("G14", "pe", 0.7, "G", "A {genre} mix for you", "Albums near your {genre} artists", "albums of artists near the library, in the genre", "S × listens", "not had", "D").need("lib").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.notHad(c.where(palb(c), func(i int) bool { return c.simArtist(i) > 0 })), func(i int) float64 { return c.simArtist(i) * (0.5 + 0.5*c.lp(i)) })
		}),
	t("G15", "vi", 0.4, "G", "{genre} music videos", "{genre} artists charting now, with their music videos", "list video_artists in the genre", "rank", "video known", "W").wide().need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn("video_artists", pool(c)) }),
	t("G16", "ar", 0.4, "G", "{genre} artists across the decades", "{genre} artists with long-loved albums in 3 or more decades", "the genre's artists and their album years", "artist listens", "3 decades or more", "W").round().
		fill(func(c *Ctx, _ string) L {
			yrs := c.artistYears()
			return c.byLP(c.where(part(c), func(i int) bool {
				d := map[int]bool{}
				for _, y := range yrs[c.it(i).ID] {
					d[y/10] = true
				}
				return len(d) >= 3
			}))
		}),

	// Mood page.
	t("H01", "ch", 0.9, "M", "{mood} right now", "Popular now and {basis}", "the mood's charting or week-ranked albums", "0.5 fit + 0.5 popularity", "", "D").
		fill(rightNow),
	t("H02", "ch", 0.8, "ME", "{mood} picks", "Albums and artists from this week's {mood} lists", "list tile", "rank", "key has a tile", "W").slot("mood", 2).need("ls").
		fill(func(c *Ctx, v string) L { return c.blockIn(tileBlock(v), nil) }),
	t("H03", "cl", 0.7, "M", "{mood} all-time greats", "Albums with the most listeners, {basis}", "the mood's albums", "listens × fit", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(palb(c), c.Key) }),
	t("H04", "cl", 0.6, "M", "{mood} classics", "Albums 15 years or older, {basis}", "the mood's older albums", "listens", "15 years or older", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.older(palb(c), 15)) }),
	t("H05", "de", 0.5, "M", "{mood} from the {decade}", "Albums from the {decade}, {basis}", "the mood's albums in a decade", "listens × fit", "", "W").slot("decade", 4).
		fill(func(c *Ctx, v string) L { return c.byLPT(c.in(palb(c), v), c.Key) }),
	t("H06", "co", 0.6, "M", "{mood} on the charts", "Charting {place} today and {basis}", "the mood's albums on the region chart", "popularity × fit", "", "D").chart().
		fill(func(c *Ctx, _ string) L {
			return c.by(c.charting(palb(c), c.region()), func(i int) float64 { return c.p(i) * (0.5 + 0.5*c.t(i, c.Key)) })
		}),
	t("H07", "nw", 0.6, "M", "{mood} albums of {year}", "Released this year, played most so far, {basis}", "the mood's albums from this year", "year rank, then listens", "released this year", "W").slot("thisyear", 1).
		fill(func(c *Ctx, _ string) L {
			return c.by(c.inYear(palb(c), c.Now.Year()), func(i int) float64 { return c.yp(i) + 0.5*c.lp(i) })
		}),
	t("H08", "nw", 0.6, "M", "New {mood} sounds", "Out in the last 90 days, {basis}", "the mood's new albums", "0.6 fit + 0.4 F90", "90 days", "W").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.newer(palb(c), 90), func(i int) float64 { return 0.6*c.t(i, c.Key) + 0.4*c.f(i, 90) })
		}),
	t("H09", "su", 0.5, "M", "Instrumental {mood}", "Albums tagged instrumental, {basis}", "the mood's instrumental albums", "listens × fit", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(c.in(palb(c), "facet:instrumental"), c.Key) }),
	t("H10", "cr", 0.5, "MA", "{mood} {for}", "Albums {basis} and in styles that suit {activity}", "albums in the mood and the activity", "fit", "", "W").cross(map[byte]string{'M': "activity", 'A': "mood"}, 3).
		fill(crossFill),
	t("H11", "se", 0.6, "MS", "{mood} picks for {season}", "Albums {basis} that fit {season}", "albums in the mood and the season", "fit", "season on (mood page)", "Sn").cross(map[byte]string{'M': "season", 'S': "mood"}, 4).notWith("mood:sleep", "mood:focus", "mood:workout", "mood:throwback", "season:back-to-school").
		fill(crossFill),
	t("H12", "di", 0.5, "M", "Hidden {mood} gems", "Well liked by those who play them, less played, {basis}", "the mood's mid-listened albums with loyal listeners", "0.5 fit + 0.5 novelty", "not had", "W").disc().
		fill(gems),
	t("H13", "su", 0.4, "M", "{mood} soundtracks", "Film, TV and game music {basis}", "the mood's soundtracks", "listens × fit", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(c.in(palb(c), "genre:soundtracks"), c.Key) }),
	t("H14", "co", 0.5, "M", "{mood} big in {country}", "Charting in {country} today and {basis}", "the mood's albums on a country chart", "popularity there", "", "D").chart().slot("country", 1).
		fill(func(c *Ctx, v string) L { return c.charting(palb(c), v) }),
	t("H15", "ar", 0.5, "M", "{mood} artists", "Artists whose albums are {basis}", "the mood's artists", "0.6 fit + 0.4 listens", "", "W").round().
		fill(func(c *Ctx, _ string) L {
			return c.by(part(c), func(i int) float64 { return 0.6*c.t(i, c.Key) + 0.4*c.lp(i) })
		}),
	t("H16", "vi", 0.4, "M", "{mood} music videos", "Artists {basis}, with their music videos", "list video_artists in the mood", "rank", "video known", "W").wide().need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn("video_artists", pool(c)) }),
	t("H17", "pe", 0.7, "M", "{mood} from artists like yours", "Albums by artists close to yours, {basis}", "albums of similar artists in the mood", "0.5 S + 0.5 fit", "not had", "D").need("lib").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.notHad(c.where(palb(c), func(i int) bool { return c.simArtist(i) > 0 })), func(i int) float64 { return 0.5*c.simArtist(i) + 0.5*c.t(i, c.Key) })
		}),

	// Season page.
	t("I01", "po", 0.8, "S", "{season} favourites", "Played most last week, {basis}", "the season's week-ranked albums in season, else long-loved ones", "week rank or listens", "", "Sn").
		fill(func(c *Ctx, _ string) L {
			if c.seasonOn() {
				return c.by(c.ranked(palb(c), rWeek), c.wp)
			}
			return nil
		}),
	t("I02", "cl", 0.8, "S", "{season} all-time greats", "Albums with the most listeners, {basis}", "the season's albums", "listens × fit", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(palb(c), c.Key) }),
	t("I03", "cl", 0.7, "S", "{season} classics", "Albums 20 years or older, {basis}", "the season's older albums", "listens", "20 years or older", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.older(palb(c), 20)) }),
	t("I04", "ch", 0.8, "S", "{season} on the charts", "Charting {place} today and {basis}", "the season's albums on the region chart", "popularity", "season on", "Sn").chart().need("season").
		fill(func(c *Ctx, _ string) L {
			return c.union2(c.charting(palb(c), c.region()), c.byP(c.onAnyChart(palb(c))))
		}),
	t("I05", "de", 0.5, "S", "{season} from the {decade}", "Albums from the {decade}, {basis}", "the season's albums in a decade", "listens", "", "W").slot("decade", 4).
		fill(func(c *Ctx, v string) L { return c.byLP(c.in(palb(c), v)) }),
	t("I06", "nw", 0.6, "S", "New for {season}", "Out in the last year, {basis}", "the season's albums of the last year", "F365 × fit", "season on", "Sn").need("season").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.newer(palb(c), 365), func(i int) float64 { return c.f(i, 365) * (0.5 + 0.5*c.t(i, c.Key)) })
		}),
	t("I07", "cr", 0.4, "S", "{season} for kids", "Children's albums {basis}", "the season's children's albums", "fit × listens", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(c.in(palb(c), "genre:kids"), c.Key) }),
	t("I08", "su", 0.4, "S", "{season} soundtracks", "Film, TV and game music {basis}", "the season's soundtracks", "fit × listens", "", "W").
		fill(func(c *Ctx, _ string) L {
			return c.byLPT(c.union2(c.in(palb(c), "genre:soundtracks"), c.typ(palb(c), "Soundtrack")), c.Key)
		}),
	t("I09", "ch", 0.8, "SEN", "{season} picks", "Albums from this week's {season} lists", "list seasonal slot", "rank", "season on, list of this season", "Sn").slot("season_on", 1).need("ls").
		fill(func(c *Ctx, v string) L {
			if c.Blocks["season_key"] == nil || len(c.Blocks["season_key"]) == 0 || c.Blocks["season_key"][0] != v {
				return nil
			}
			return c.block("tile:season")
		}),
	t("I10", "ar", 0.5, "S", "Artists of {season}", "Artists whose albums are {basis}", "the season's artists", "fit × listens", "", "W").round().
		fill(func(c *Ctx, _ string) L { return c.byLPT(part(c), c.Key) }),
	t("I11", "di", 0.5, "S", "{season} hidden gems", "Well liked by those who play them, less played, {basis}", "the season's mid-listened albums with loyal listeners", "0.5 fit + 0.5 novelty", "not had", "W").disc().
		fill(gems),
	t("I12", "vi", 0.4, "S", "{season} music videos", "Artists {basis}, with their music videos", "list video_artists in the season", "rank", "season on", "Sn").wide().need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn("video_artists", pool(c)) }),

	// Decade page.
	t("J01", "cl", 0.9, "D", "{decade} all-time greats", "The {decade} albums with the most listeners", "the decade's albums", "listens", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(palb(c)) }),
	t("J02", "cr", 0.7, "D", "{decade} {genre} greats", "The {decade} {genre} albums with the most listeners", "the decade's albums in a genre", "listens", "", "W").cross(map[byte]string{'D': "genre"}, 6).
		fill(func(c *Ctx, v string) L { return c.byLP(c.in(palb(c), v)) }),
	t("J03", "yr", 0.6, "D", "Biggest albums of {year}", "The {year} albums with the most listeners", "the albums of one year", "listens", "", "W").slot("year", 3).
		fill(func(c *Ctx, v string) L { return c.byLP(c.inYear(palb(c), atoi(v))) }),
	t("J04", "ar", 0.6, "D", "{decade} icons", "Artists whose most-played albums came out in the {decade}", "artists by the year of their top album", "artist listens", "", "W").round().
		fill(func(c *Ctx, _ string) L {
			top := c.artistTopYear()
			lo, hi := c.decadeRange()
			return c.byLP(c.where(c.artists(c.all()), func(i int) bool { y := top[c.it(i).ID]; return y >= lo && y <= hi }))
		}),
	t("J05", "po", 0.7, "DE", "Still loved from the {decade}", "{decade} albums listeners played most last week", "listen ranks, last week, in the decade", "week rank", "", "W").slot("decade_page", 1).
		fill(func(c *Ctx, v string) L { return c.by(c.ranked(kalb(c, v), rWeek), c.wp) }),
	t("J06", "cr", 0.5, "D", "{decade} {mood}", "{decade} albums {basis}", "the decade's albums in a mood", "listens × fit", "", "W").cross(map[byte]string{'D': "mood"}, 4).
		fill(func(c *Ctx, v string) L { return c.byLPT(c.in(palb(c), v), v) }),
	t("J07", "su", 0.4, "D", "{decade} soundtracks", "Film, TV and game music from the {decade}", "the decade's soundtracks", "listens", "", "W").
		fill(func(c *Ctx, _ string) L {
			return c.byLP(c.union2(c.in(palb(c), "genre:soundtracks"), c.typ(palb(c), "Soundtrack")))
		}),
	t("J08", "su", 0.4, "D", "{decade} EPs", "Short releases from the {decade} with the most listeners", "the decade's EPs", "listens", "EPs", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.typ(palb(c), "EP")) }),
	t("J09", "di", 0.5, "D", "{decade} hidden gems", "Well liked by those who play them, less played, from the {decade}", "the decade's mid-listened albums with loyal listeners", "0.5 fit + 0.5 novelty", "not had", "W").disc().
		fill(gems),
	t("J10", "yr", 0.5, "D", "{decade} albums turning {n}", "{decade} albums released {n} years ago", "the decade's albums from one year", "listens", "anniversary year", "Y").slot("n", 1).
		fill(func(c *Ctx, v string) L { return c.byLP(c.inYear(palb(c), c.Now.Year()-atoi(v))) }),
	t("J11", "ch", 0.5, "D", "{decade} on the charts now", "{decade} albums charting today", "the decade's albums on any chart", "popularity", "", "D").chart().
		fill(func(c *Ctx, _ string) L { return c.byP(c.onAnyChart(palb(c))) }),
	t("J12", "pe", 0.6, "D", "{decade} artists like yours", "{decade} albums by artists close to yours", "albums of similar artists in the decade", "S × listens", "not had", "D").need("lib").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.notHad(c.where(palb(c), func(i int) bool { return c.simArtist(i) > 0 })), func(i int) float64 { return c.simArtist(i) * (0.5 + 0.5*c.lp(i)) })
		}),
	t("J13", "vi", 0.4, "D", "{decade} music videos", "{decade} artists with their music videos", "list video_artists in the decade", "rank", "video known", "W").wide().need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn("video_artists", c.artistsOfAlbums(palb(c))) }),
	t("J14", "cl", 0.4, "D", "Debut albums of the {decade}", "First albums from the {decade} with the most listeners", "the decade's debut albums", "listens", "debuts", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.debuts(palb(c))) }),
	t("J15", "ar", 0.4, "D", "{decade} artists still releasing", "{decade} artists with a new release in the last 2 years", "artists with albums in the decade and lately", "artist listens", "", "W").round().
		fill(func(c *Ctx, _ string) L {
			yrs := c.artistYears()
			lo, hi := c.decadeRange()
			now := c.Now.Year()
			return c.byLP(c.where(c.artists(c.all()), func(i int) bool {
				in, recent := false, false
				for _, y := range yrs[c.it(i).ID] {
					in = in || y >= lo && y <= hi
					recent = recent || now-y <= 2
				}
				return in && recent && hi < now-2
			}))
		}),

	// Activity page.
	t("K01", "ch", 0.9, "A", "Popular now {for}", "Popular now, in styles that suit {activity}", "the activity's charting or week-ranked albums", "0.5 fit + 0.5 popularity", "", "D").
		fill(rightNow),
	t("K02", "cl", 0.7, "A", "All-time greats {for}", "Albums with the most listeners, in styles that suit {activity}", "the activity's albums", "listens × fit", "", "W").
		fill(func(c *Ctx, _ string) L { return c.byLPT(palb(c), c.Key) }),
	t("K03", "ch", 0.8, "A", "Picks {for}", "Albums from this week's {activity} lists", "list tile", "rank", "key has a tile", "W").need("ls").
		fill(func(c *Ctx, _ string) L { return c.blockIn(tileBlock(c.Key), nil) }),
	t("K04", "de", 0.5, "A", "{decade} picks {for}", "{decade} albums in styles that suit {activity}", "the activity's albums in a decade", "listens", "", "W").slot("decade", 3).
		fill(func(c *Ctx, v string) L { return c.byLP(c.in(palb(c), v)) }),
	t("K05", "cr", 0.6, "A", "High energy {for}", "Fast, loud styles that suit {activity}", "the activity's high-energy albums", "fit × listens", "", "W").only(energyActs...).
		fill(func(c *Ctx, _ string) L { return c.byLPT(c.in(palb(c), "mood:energy"), "mood:energy") }),
	t("K06", "su", 0.6, "A", "Instrumentals {for}", "Albums tagged instrumental, in styles that suit {activity}", "the activity's instrumental albums", "fit × listens", "", "W").only(instrActs...).
		fill(func(c *Ctx, _ string) L { return c.byLPT(c.in(palb(c), "facet:instrumental"), c.Key) }),
	t("K07", "nw", 0.6, "A", "New {for}", "Out in the last 90 days, in styles that suit {activity}", "the activity's new albums", "0.6 fit + 0.4 F90", "90 days", "W").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.newer(palb(c), 90), func(i int) float64 { return 0.6*c.t(i, c.Key) + 0.4*c.f(i, 90) })
		}),
	t("K08", "di", 0.5, "A", "Hidden gems {for}", "Well liked by those who play them, less played, in styles that suit {activity}", "the activity's mid-listened albums with loyal listeners", "0.5 fit + 0.5 novelty", "not had", "W").disc().
		fill(gems),
	t("K09", "su", 0.4, "A", "Soundtracks {for}", "Film, TV and game music that suits {activity}", "the activity's soundtracks", "fit × listens", "", "W").only(scoreActs...).
		fill(func(c *Ctx, _ string) L {
			return c.byLPT(c.union2(c.in(palb(c), "genre:soundtracks"), c.typ(palb(c), "Soundtrack")), c.Key)
		}),
	t("K10", "ar", 0.5, "A", "Artists {for}", "Artists whose albums suit {activity}", "the activity's artists", "fit × listens", "", "W").round().
		fill(func(c *Ctx, _ string) L { return c.byLPT(part(c), c.Key) }),
	t("K11", "co", 0.6, "A", "On the charts, good {for}", "Charting {place} today, in styles that suit {activity}", "the activity's albums on the region chart", "popularity × fit", "", "D").chart().
		fill(func(c *Ctx, _ string) L {
			return c.by(c.charting(palb(c), c.region()), func(i int) float64 { return c.p(i) * (0.5 + 0.5*c.t(i, c.Key)) })
		}),

	// Shared library row.
	t("L01", "pe", 0.6, "MSDA", "{name} from your Library", "Albums you have, {basis}", "library albums in the page key", "affinity × fit", "", "L").need("lib").
		fill(func(c *Ctx, _ string) L {
			return c.by(c.where(palb(c), func(i int) bool { return c.Lib != nil && c.Lib.Albums[c.it(i).ID] }), func(i int) float64 { return c.l(i) * (0.5 + 0.5*c.t(i, c.Key)) })
		}).keep(),

	t("M01", "po", 0.6, "M", "{mood} most played last week", "Albums listeners played most last week, {basis}", "listen ranks, last week, in the mood", "week rank", "", "W").
		fill(weekPlayed),
	t("M02", "cl", 0.5, "M", "Modern {mood} classics", "Long-loved albums from the last 5 to 15 years, {basis}", "the mood's albums 5 to 15 years old", "listens", "5 to 15 years old", "W").
		fill(modern),
	t("M03", "yr", 0.4, "M", "{mood} in {year}", "The {year} albums with the most listeners, {basis}", "the mood's albums from one year", "listens", "", "W").slot("year", 2).
		fill(func(c *Ctx, v string) L { return c.byLP(c.inYear(palb(c), atoi(v))) }),
	t("M04", "cl", 0.4, "M", "{mood} debut albums", "Debut albums with the most listeners, {basis}", "the mood's debut albums", "listens", "debuts", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.debuts(palb(c))) }),
	t("M05", "ar", 0.5, "M", "{mood} artists on the charts", "Artists charting today whose albums are {basis}", "the mood's charting artists", "popularity", "", "D").round().
		fill(func(c *Ctx, _ string) L { return c.byP(c.onAnyChart(part(c))) }),
	t("M06", "ar", 0.4, "M", "Rising {mood} artists", "In last week's top, not in this year's, {basis}", "the mood's artists new to the week's top", "week rank", "", "W").round().
		fill(rising),
	t("M07", "co", 0.5, "M", "{mood} big across {area}", "Charting in the most {area} countries, {basis}", "area charts in the mood", "area score", "", "D").chart().slot("area", 2).
		fill(acrossArea),
	t("M08", "su", 0.4, "M", "{mood} EPs", "Short releases with the most listeners, {basis}", "the mood's EPs", "listens", "EPs", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.typ(palb(c), "EP")) }),

	t("S01", "po", 0.6, "S", "{season} most played last week", "Albums listeners played most last week, {basis}", "listen ranks, last week, in the season", "week rank", "", "W").
		fill(weekPlayed),
	t("S02", "cl", 0.5, "S", "Modern {season} classics", "Long-loved albums from the last 5 to 15 years, {basis}", "the season's albums 5 to 15 years old", "listens", "5 to 15 years old", "W").
		fill(modern),
	t("S03", "yr", 0.4, "S", "{season} in {year}", "The {year} albums with the most listeners, {basis}", "the season's albums from one year", "listens", "", "W").slot("year", 3).
		fill(func(c *Ctx, v string) L { return c.byLP(c.inYear(palb(c), atoi(v))) }),
	t("S04", "cl", 0.4, "S", "{season} debut albums", "Debut albums with the most listeners, {basis}", "the season's debut albums", "listens", "debuts", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.debuts(palb(c))) }),
	t("S05", "ar", 0.5, "S", "{season} artists on the charts", "Artists charting today whose albums are {basis}", "the season's charting artists", "popularity", "", "D").round().
		fill(func(c *Ctx, _ string) L { return c.byP(c.onAnyChart(part(c))) }),
	t("S06", "cr", 0.5, "S", "{season} {for}", "Albums {basis} and in styles that suit {activity}", "albums in the season and the activity", "fit", "", "W").cross(map[byte]string{'S': "activity"}, 3).
		fill(crossFill),
	t("S07", "co", 0.5, "S", "{season} big across {area}", "Charting in the most {area} countries, {basis}", "area charts in the season", "area score", "", "D").chart().slot("area", 2).vars("{season} across {area}").
		fill(acrossArea),
	t("S08", "su", 0.4, "S", "{season} EPs", "Short releases with the most listeners, {basis}", "the season's EPs", "listens", "EPs", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.typ(palb(c), "EP")) }),

	t("V01", "po", 0.6, "A", "Played most last week {for}", "Albums listeners played most last week, in styles that suit {activity}", "listen ranks, last week, in the activity", "week rank", "", "W").
		fill(weekPlayed),
	t("V02", "cl", 0.5, "A", "Modern classics {for}", "Long-loved albums from the last 5 to 15 years, in styles that suit {activity}", "the activity's albums 5 to 15 years old", "listens", "5 to 15 years old", "W").
		fill(modern),
	t("V03", "yr", 0.4, "A", "{year} picks {for}", "The {year} albums with the most listeners, in styles that suit {activity}", "the activity's albums from one year", "listens", "", "W").slot("year", 2).
		fill(func(c *Ctx, v string) L { return c.byLP(c.inYear(palb(c), atoi(v))) }),
	t("V04", "cl", 0.4, "A", "First albums {for}", "Debut albums with the most listeners, in styles that suit {activity}", "the activity's debut albums", "listens", "debuts", "W").
		fill(func(c *Ctx, _ string) L { return c.byLP(c.debuts(palb(c))) }),
	t("V05", "ar", 0.5, "A", "Charting artists {for}", "Artists charting today, in styles that suit {activity}", "the activity's charting artists", "popularity", "", "D").round().
		fill(func(c *Ctx, _ string) L { return c.byP(c.onAnyChart(part(c))) }),
	t("V06", "ar", 0.4, "A", "Rising artists {for}", "In last week's top, not in this year's, in styles that suit {activity}", "the activity's artists new to the week's top", "week rank", "", "W").round().
		fill(rising),
	t("V07", "co", 0.5, "A", "Big across {area}, {for}", "Charting in the most {area} countries, in styles that suit {activity}", "area charts in the activity", "area score", "", "D").chart().slot("area", 2).
		fill(acrossArea),
	t("V08", "cr", 0.5, "A", "{season} {for}", "Albums for {season}, in styles that suit {activity}", "albums in the activity and the season", "fit", "season on", "Sn").cross(map[byte]string{'A': "season"}, 1).
		fill(crossFill),

	t("J16", "su", 0.5, "D", "{decade} {substyle}", "The {decade} {substyle} albums with the most listeners", "the decade's albums in a substyle", "listens", "", "W").cross(map[byte]string{'D': "sub"}, 4).
		fill(func(c *Ctx, v string) L { return c.byLP(c.in(palb(c), v)) }),
	t("J17", "cr", 0.4, "D", "{decade} picks {for}", "{decade} albums in styles that suit {activity}", "the decade's albums in an activity", "listens", "", "W").cross(map[byte]string{'D': "activity"}, 2).
		fill(func(c *Ctx, v string) L { return c.byLP(c.in(palb(c), v)) }),
	t("J18", "se", 0.4, "D", "{decade} {season}", "{decade} albums for {season}", "the decade's albums in a season", "listens", "", "W").cross(map[byte]string{'D': "season"}, 1).
		fill(func(c *Ctx, v string) L { return c.byLP(c.in(palb(c), v)) }),
	t("J19", "po", 0.5, "D", "{decade} favourites this year", "{decade} albums still played a lot this year", "listen ranks, this year, in the decade", "year rank", "", "W").
		fill(func(c *Ctx, _ string) L { return c.by(c.ranked(palb(c), rThisYear), c.yp) }),
}

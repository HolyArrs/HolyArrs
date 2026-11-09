package rowtpl

import (
	"strconv"
	"strings"
)

// shapeOr gives square when a template names no shape.
func shapeOr(s string) string {
	if s == "" {
		return "square"
	}
	return s
}

// render fills a template's title and reason for a filled row. The title is the first wording that fits 40 characters, rotated by member and day.
func (c *Ctx) render(tp Tpl, v string, items []int, rowID string) (string, string) {
	vars := map[string]string{}
	set := func(key string) {
		switch kindOf(key) {
		case "genre":
			if vars["{genre}"] == "" {
				vars["{genre}"] = c.name(key)
			} else {
				vars["{genre2}"] = c.name(key)
			}
		case "mood":
			vars["{mood}"] = lowerName(c.name(key))
		case "season":
			vars["{season}"] = c.name(key)
			if commonSeasons[key] {
				vars["{season}"] = lowerName(c.name(key))
			}
		case "decade":
			vars["{decade}"] = c.name(key)
		case "activity":
			vars["{activity}"] = strings.ToLower(c.name(key))
			vars["{for}"] = forOf[key]
		case "sub":
			vars["{substyle}"] = lowerName(c.name(key))
		}
	}
	if c.Key != "" {
		set(c.Key)
		vars["{name}"] = c.name(c.Key)
		if k := kindOf(c.Key); k == "mood" || commonSeasons[c.Key] {
			vars["{name}"] = lowerName(c.name(c.Key))
		}
	}
	claim := c.Key
	switch tp.Slot {
	case "country":
		vars["{country}"] = c.country(v)
	case "area":
		vars["{area}"] = areaName[v]
	case "year", "thisyear":
		vars["{year}"] = v
	case "n":
		vars["{n}"] = v
	case "seed":
		vars["{artist}"] = c.seedName(v)
	case "":
	default:
		if strings.Contains(v, ":") && v != c.Key {
			set(v)
			if k := kindOf(v); k == "mood" || k == "season" || k == "activity" {
				claim = v
			}
		}
	}
	if tp.Slot == "season_on" || tp.Slot == "season_up" {
		claim = v
	}
	if tp.Slot == "year" || tp.Slot == "thisyear" {
		vars["{year}"] = v
	}
	if vars["{year}"] == "" {
		vars["{year}"] = strconv.Itoa(c.Now.Year())
	}
	vars["{place}"] = c.place(c.Region)
	vars["{neighbor}"] = c.place(c.Neighbor)
	vars["{basis}"] = c.basis(claim, items)
	fill := func(s string) string {
		for k, x := range vars {
			s = strings.ReplaceAll(s, k, x)
		}
		return capFirst(s)
	}
	words := append([]string{}, tp.Vars...)
	words = append(words, tp.Title)
	start := int(hash(strconv.FormatInt(c.Member, 10), c.Day(), rowID) % uint64(len(words)))
	title := fill(tp.Title)
	for k := 0; k < len(words); k++ {
		w := fill(words[(start+k)%len(words)])
		if len([]rune(w)) <= 40 || strings.Contains(words[(start+k)%len(words)], "{artist}") {
			title = w
			break
		}
	}
	return title, fill(tp.Reason)
}

// basis says how a row's items fit its claim key. It says "tagged ... by listeners" only when every item carries the key's own tag.
func (c *Ctx) basis(claim string, items []int) string {
	if claim == "" || kindOf(claim) == "genre" || kindOf(claim) == "decade" {
		return "in styles that suit " + strings.ToLower(c.name(claim))
	}
	m := c.B.byKey[claim]
	own := len(items) > 0
	for _, i := range items {
		if !m[i].Own {
			own = false
			break
		}
	}
	n := strings.ToLower(c.name(claim))
	if kindOf(claim) == "activity" {
		n = strings.TrimPrefix(forOf[claim], "for ")
	}
	if own {
		return "tagged " + n + " by listeners"
	}
	return "in styles that suit " + n
}

// capFirst upper-cases a leading ASCII letter.
func capFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

// country answers a country's name for titles. The API's own table comes first.
func (c *Ctx) country(code string) string {
	if c.CountryName != nil {
		if n := c.CountryName(code); n != "" {
			return n
		}
	}
	if n, ok := countryNames[strings.ToLower(code)]; ok {
		return n
	}
	return strings.ToUpper(code)
}

// lowerName writes a mood or style name for the middle of a title. Names with inner capitals stay as they are, like K-R&B.
func lowerName(n string) string {
	r := []rune(n)
	if len(r) > 1 && strings.ToLower(string(r[1:])) == string(r[1:]) {
		return strings.ToLower(string(r[:1])) + string(r[1:])
	}
	return n
}

// place answers "worldwide" or "in <country>" for a region code.
func (c *Ctx) place(code string) string {
	if code == "" || code == "global" {
		return "worldwide"
	}
	n := c.country(code)
	return "in " + n
}

// seedName answers a library seed artist's name, or empty if unknown.
func (c *Ctx) seedName(id string) string {
	if c.Lib != nil {
		for _, s := range c.Lib.Seeds {
			if s.ID == id {
				return s.Name
			}
		}
	}
	return ""
}

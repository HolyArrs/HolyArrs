package requests

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Trakt's public charts can be followed like a list. They are stored as
// "chart:<name>", which no Trakt user/list pair can look like.
const traktChartPrefix = "chart:"

// traktCharts maps each chart name to its path after /movies or /shows.
var traktCharts = map[string]string{
	"trending":        "trending",
	"popular":         "popular",
	"anticipated":     "anticipated",
	"boxoffice":       "boxoffice",
	"watched-weekly":  "watched/weekly",
	"watched-monthly": "watched/monthly",
	"watched-yearly":  "watched/yearly",
	"watched-all":     "watched/all",
}

// ErrBoxOfficeShows: Trakt's box office chart has movies only.
var ErrBoxOfficeShows = errors.New("the Trakt box office chart has movies only")

var traktChartURL = regexp.MustCompile(`^https?://(?:www\.|app\.)?trakt\.tv/(?:movies|shows)/([a-z]+)(?:/([a-z]+))?/?(?:[?#].*)?$`)

// traktChartRef reads a chart name, "chart:<name>" or a Trakt chart link.
func traktChartRef(ref string) (string, bool) {
	name := strings.ToLower(strings.TrimPrefix(strings.ToLower(ref), traktChartPrefix))
	if m := traktChartURL.FindStringSubmatch(ref); m != nil {
		name = m[1]
		if m[2] != "" {
			name += "-" + m[2]
		}
	}
	if _, ok := traktCharts[name]; ok {
		return traktChartPrefix + name, true
	}
	return "", false
}

// traktChartPath gives the API path of a stored chart ref for movies or shows.
func traktChartPath(ref, titleKind string) (string, error) {
	name := strings.TrimPrefix(ref, traktChartPrefix)
	p, ok := traktCharts[name]
	if !ok {
		return "", ErrRef
	}
	if titleKind == "show" {
		if name == "boxoffice" {
			return "", ErrBoxOfficeShows
		}
		return "/shows/" + p, nil
	}
	return "/movies/" + p, nil
}

// unwrapChart turns a chart answer into list items. Popular sends bare
// titles; every other chart wraps each title with a count.
func unwrapChart(raw []json.RawMessage, titleKind string) []json.RawMessage {
	key := "movie"
	if titleKind == "show" {
		key = "show"
	}
	out := make([]json.RawMessage, 0, len(raw))
	for _, r := range raw {
		var w map[string]json.RawMessage
		if json.Unmarshal(r, &w) != nil {
			continue
		}
		if inner, ok := w[key]; ok {
			out = append(out, inner)
		} else if _, ok := w["ids"]; ok {
			out = append(out, r)
		}
	}
	return out
}

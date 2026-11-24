package downloads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// fitNames are the names the old apps use for each kind, best first: what
// Sonarr, Radarr and Lidarr fill in, then the plain words people pick.
var fitNames = map[string][]string{
	"show":   {"tv", "tv-sonarr", "sonarr", "shows", "series"},
	"movie":  {"movies", "radarr", "movie"},
	"artist": {"music", "lidarr"},
}

// foldCategory is a category name for comparing: trimmed, lower case, accents dropped.
func foldCategory(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.TrimSpace(s)) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// PickCategory returns the category in have that fits kind (show, movie,
// artist) by name, as written in have, or "" when none does.
func PickCategory(kind string, have []string) string {
	for _, want := range fitNames[kind] {
		for _, c := range have {
			if foldCategory(c) == want {
				return c
			}
		}
	}
	return ""
}

// categoryLister is a client HolyArrs can read the category list of.
type categoryLister interface {
	ListCategories(ctx context.Context) ([]string, error)
}

// ListCategories returns qBittorrent's categories.
func (q *Qbittorrent) ListCategories(ctx context.Context) ([]string, error) {
	b, err := q.call(ctx, http.MethodGet, "torrents/categories", nil)
	if err != nil {
		return nil, err
	}
	var cats map[string]json.RawMessage
	if err := json.Unmarshal(b, &cats); err != nil {
		return nil, fmt.Errorf("qbittorrent torrents/categories: %w", err)
	}
	out := make([]string, 0, len(cats))
	for name := range cats {
		out = append(out, name)
	}
	slices.Sort(out)
	return out, nil
}

// ListCategories returns SABnzbd's categories, without its "*" default.
func (s *Sabnzbd) ListCategories(ctx context.Context) ([]string, error) {
	b, err := s.call(ctx, "get_cats", nil, true)
	if err != nil {
		return nil, err
	}
	var c struct {
		Categories []string `json:"categories"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("sabnzbd get_cats: %w", err)
	}
	return slices.DeleteFunc(c.Categories, func(x string) bool { return x == "*" || strings.TrimSpace(x) == "" }), nil
}

// ListCategories returns NZBGet's categories (Category1.Name and on).
func (n *NZBGet) ListCategories(ctx context.Context) ([]string, error) {
	var opts []nzbParam
	if err := n.rpc(ctx, "config", nil, &opts); err != nil {
		return nil, err
	}
	o := make(map[string]string, len(opts))
	for _, p := range opts {
		o[strings.ToLower(p.Name)] = p.Value
	}
	var out []string
	for i := 1; i < 1000; i++ {
		name, ok := o["category"+strconv.Itoa(i)+".name"]
		if !ok {
			break
		}
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

// ListCategories returns Deluge's labels; none when its Label plugin is off.
func (d *Deluge) ListCategories(ctx context.Context) ([]string, error) {
	return d.ready(ctx)
}

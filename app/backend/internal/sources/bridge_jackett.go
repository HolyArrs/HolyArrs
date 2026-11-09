package sources

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// BridgeJackett is a client for a Jackett the person runs. Its key alone lists
// the configured sites and their categories.
type BridgeJackett struct {
	URL  string // as the person typed it, any base path included
	Key  string
	http *http.Client
}

// BridgeSite is one site set up in Jackett.
type BridgeSite struct {
	ID         string
	Title      string
	Type       string           // public, private or semi-private
	Categories map[string][]int // category numbers by HolyArrs kind (movie, tv, music), from its caps
}

// jackettCategory is one category in a site's caps, with its children.
type jackettCategory struct {
	ID   int               `xml:"id,attr"`
	Subs []jackettCategory `xml:"subcat"`
}

// jackettList is the XML answer to the site list, or an error element.
type jackettList struct {
	XMLName  xml.Name
	Code     int    `xml:"code,attr"`
	Text     string `xml:"description,attr"`
	Indexers []struct {
		ID         string `xml:"id,attr"`
		Configured string `xml:"configured,attr"`
		Title      string `xml:"title"`
		Type       string `xml:"type"`
		Caps       struct {
			Categories []jackettCategory `xml:"categories>category"`
		} `xml:"caps"`
	} `xml:"indexer"`
}

// NewBridgeJackett returns a client for one Jackett.
func NewBridgeJackett(base, key string) *BridgeJackett {
	return &BridgeJackett{URL: strings.TrimRight(base, "/"), Key: key, http: client(30 * time.Second)}
}

// FeedURL returns the Torznab address of a site in this Jackett.
func (j *BridgeJackett) FeedURL(id string) string {
	return j.URL + "/api/v2.0/indexers/" + url.PathEscape(id) + "/results/torznab/"
}

// Sites lists the sites set up in this Jackett. Jackett answers a wrong key
// with status 200 and an error in the body, so the body decides.
func (j *BridgeJackett) Sites(ctx context.Context) ([]BridgeSite, error) {
	q := url.Values{"apikey": {j.Key}, "t": {"indexers"}, "configured": {"true"}}
	status, b, err := call(ctx, j.http, http.MethodGet, j.URL+"/api/v2.0/indexers/all/results/torznab/api?"+q.Encode(), nil, nil, 16<<20)
	if err != nil {
		return nil, err
	}
	var l jackettList
	if xml.Unmarshal(b, &l) != nil {
		if status == http.StatusNotFound || status == http.StatusOK {
			return nil, fail("That address answers, but not like Jackett. Check the address (and its base path, if Jackett has one).")
		}
		return nil, fail("Jackett answered with status %d.", status)
	}
	if l.XMLName.Local == "error" {
		if l.Code == 100 {
			return nil, fail("Jackett turned down the key. Copy it again from the top of Jackett's page.")
		}
		return nil, &BridgeError{Words: "Jackett answered with an error.", Detail: clip(strings.TrimSpace(l.Text), 200)}
	}
	if l.XMLName.Local != "indexers" {
		return nil, fail("That address answers, but not like Jackett.")
	}
	var out []BridgeSite
	for _, ix := range l.Indexers {
		id := strings.TrimSpace(ix.ID)
		// The combined feeds are never sites of their own.
		if id == "" || id == "all" || strings.Contains(id, ":") || (ix.Configured != "" && ix.Configured != "true") {
			continue
		}
		title := strings.TrimSpace(ix.Title)
		if title == "" {
			title = id
		}
		out = append(out, BridgeSite{ID: id, Title: clipRunes(title, 64), Type: strings.TrimSpace(ix.Type), Categories: kindsOf(ix.Caps.Categories)})
	}
	sort.Slice(out, func(a, b int) bool { return strings.ToLower(out[a].Title) < strings.ToLower(out[b].Title) })
	return out, nil
}

// kindsOf splits a site's categories into HolyArrs kinds by the standard
// numbers (movies 2000s, audio 3000s, TV 5000s). A site's own numbers start
// at 100000 and are left out, since Jackett also lists their standard parent.
func kindsOf(cats []jackettCategory) map[string][]int {
	out := map[string][]int{}
	var walk func([]jackettCategory)
	walk = func(cs []jackettCategory) {
		for _, c := range cs {
			switch {
			case c.ID >= 2000 && c.ID < 3000:
				out["movie"] = append(out["movie"], c.ID)
			case c.ID >= 5000 && c.ID < 6000:
				out["tv"] = append(out["tv"], c.ID)
			case c.ID >= 3000 && c.ID < 4000:
				out["music"] = append(out["music"], c.ID)
			}
			walk(c.Subs)
		}
	}
	walk(cats)
	for k, v := range out {
		sort.Ints(v)
		uniq := v[:0]
		for i, n := range v {
			if i == 0 || n != v[i-1] {
				uniq = append(uniq, n)
			}
		}
		out[k] = uniq
	}
	return out
}

// clipRunes keeps at most n letters, so a Source name with a suffix such as
// " (Jackett 2)" stays within the 80 letters a name may hold.
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

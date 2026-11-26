package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// YTS serves yts.pw's movie catalogue through its v2 JSON API. Its API
// moved to movies-api.accel.li; the Source address remains configurable.
func (c *Client) ytsSearch(ctx context.Context, base string, q Query) ([]Item, error) {
	if q.TV || q.Music {
		return nil, nil
	}
	v := url.Values{"limit": {"50"}, "sort_by": {"date_added"}, "order_by": {"desc"}}
	if !q.feed {
		term := ytsIMDb(q.IMDb)
		if term == "" {
			term = textQuery(Query{Title: q.Title})
		}
		if term == "" {
			return nil, ErrNothingToSearch
		}
		v.Set("query_term", term)
	}
	raw, err := c.send(ctx, http.MethodGet, base+"/api/v2/list_movies.json?"+v.Encode(), nil, nil)
	if err != nil {
		return nil, err
	}
	items, err := parseYTS(raw)
	if !q.feed && ytsIMDb(q.IMDb) != "" {
		// Do not accept an unrelated newest list if the API ignores the ID.
		wanted := strings.TrimLeft(strings.TrimPrefix(ytsIMDb(q.IMDb), "tt"), "0")
		matched := items[:0]
		for _, it := range items {
			if it.IMDb == wanted {
				matched = append(matched, it)
			}
		}
		items = matched
	}
	return items, err
}

// ytsIMDb restores IMDb's prefix and leading zeros from HolyArrs' numeric IDs.
func ytsIMDb(id string) string {
	id = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(id)), "tt")
	if id == "" || len(id) > 12 || strings.Trim(id, "0123456789") != "" {
		return ""
	}
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil || n == 0 {
		return ""
	}
	return fmt.Sprintf("tt%07d", n)
}

type ytsTorrent struct {
	Hash     string `json:"hash"`
	Quality  string `json:"quality"`
	Type     string `json:"type"`
	Codec    string `json:"video_codec"`
	Depth    string `json:"bit_depth"`
	Audio    string `json:"audio_channels"`
	Size     int64  `json:"size_bytes"`
	Seeds    int    `json:"seeds"`
	Peers    int    `json:"peers"`
	Uploaded int64  `json:"date_uploaded_unix"`
}

func parseYTS(raw []byte) ([]Item, error) {
	var x struct {
		Status string `json:"status"`
		Data   *struct {
			Movies []struct {
				Title    string       `json:"title"`
				Year     int          `json:"year"`
				IMDb     string       `json:"imdb_code"`
				URL      string       `json:"url"`
				Language string       `json:"language"`
				Torrents []ytsTorrent `json:"torrents"`
			} `json:"movies"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &x); err != nil || x.Status != "ok" || x.Data == nil {
		return nil, &Error{Kind: KindBroken, Msg: "YTS answered with something that is not its movie list"}
	}
	var out []Item
	for _, m := range x.Data.Movies {
		if strings.TrimSpace(m.Title) == "" {
			continue
		}
		for _, t := range m.Torrents {
			hash := strings.ToLower(strings.TrimSpace(t.Hash))
			if !releasename.ValidInfoHash(hash) || t.Size <= 0 {
				continue
			}
			name := ytsTitle(m.Title, m.Year, t)
			it := Item{Title: name, GUID: hash, InfoHash: hash, Protocol: "torrent", Size: t.Size,
				Link: "magnet:?xt=urn:btih:" + hash + "&dn=" + url.QueryEscape(name), InfoURL: webLink(m.URL),
				Seeders: intPtr(max(0, t.Seeds)), Peers: intPtr(max(0, t.Seeds) + max(0, t.Peers)),
				IMDb: strings.TrimLeft(strings.TrimPrefix(ytsIMDb(m.IMDb), "tt"), "0"), Languages: addLanguages(nil, m.Language)}
			if t.Uploaded > 0 {
				it.Published = time.Unix(t.Uploaded, 0).UTC()
			}
			out = append(out, it)
		}
	}
	return out, nil
}

// ytsTitle supplies the release facts HolyArrs normally reads from a name.
func ytsTitle(title string, year int, t ytsTorrent) string {
	words := []string{strings.TrimSpace(title)}
	if year > 0 {
		words = append(words, fmt.Sprintf("(%d)", year))
	}
	words = append(words, t.Quality)
	switch strings.ToLower(t.Type) {
	case "web":
		words = append(words, "WEBRip")
	case "bluray":
		words = append(words, "BluRay")
	case "dvd":
		words = append(words, "DVDRip")
	}
	if t.Audio != "" {
		words = append(words, t.Audio)
	}
	if t.Depth == "10" || t.Depth == "12" {
		words = append(words, t.Depth+"bit")
	}
	words = append(words, t.Codec, "-YTS")
	return strings.Join(strings.Fields(strings.Join(words, " ")), " ")
}

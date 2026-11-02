package providers

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
)

// Shows without a trailer of their own (Grey's Anatomy) get their newest
// season's trailer: seasons are walked newest first, at most tvSeasonWalk.

const tvSeasonWalk = 5

var tvDetailsPath = regexp.MustCompile(`^/tv/[0-9]+$`)

type tmdbVideo struct {
	Key      string `json:"key"`
	Site     string `json:"site"`
	Type     string `json:"type"`
	Official bool   `json:"official"`
	Lang     string `json:"iso_639_1"`
}

func youTubeTrailers(list []tmdbVideo) []tmdbVideo {
	var out []tmdbVideo
	for _, v := range list {
		if v.Site == "YouTube" && v.Key != "" && (v.Type == "Trailer" || v.Type == "Teaser") {
			out = append(out, v)
		}
	}
	return out
}

// TVNeedsTrailer says q is a TMDB show details read that asks for videos
// and data holds no YouTube trailer or teaser.
func TVNeedsTrailer(q model.Query, data []byte) bool {
	if q.Operation != model.OperationTMDB || !tvDetailsPath.MatchString(q.Path) {
		return false
	}
	v, _ := url.ParseQuery(q.Params)
	if !strings.Contains(v.Get("append_to_response"), "videos") {
		return false
	}
	var obj struct {
		Videos struct {
			Results []tmdbVideo `json:"results"`
		} `json:"videos"`
	}
	_ = json.Unmarshal(data, &obj)
	return len(youTubeTrailers(obj.Videos.Results)) == 0
}

// withSeasonTrailer adds the newest season's best trailer to a show answer
// that has none; r is unchanged when no season has one or a read fails.
func (c *Client) withSeasonTrailer(ctx context.Context, q model.Query, p *provider, r model.Result) model.Result {
	if !TVNeedsTrailer(q, r.Data) {
		return r
	}
	var obj map[string]any
	if json.Unmarshal(r.Data, &obj) != nil {
		return r
	}
	var show struct {
		Seasons []struct {
			Number int `json:"season_number"`
		} `json:"seasons"`
	}
	_ = json.Unmarshal(r.Data, &show)
	var nums []int
	for _, s := range show.Seasons {
		if s.Number > 0 {
			nums = append(nums, s.Number)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(nums)))
	for i, n := range nums {
		if i >= tvSeasonWalk || ctx.Err() != nil {
			break
		}
		sq := model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: q.Path + "/season/" + strconv.Itoa(n) + "/videos", Page: 1, Limit: 20}
		sr, err := c.lookup(ctx, sq, p)
		if err != nil {
			continue
		}
		var got struct {
			Results []tmdbVideo `json:"results"`
		}
		if json.Unmarshal(sr.Data, &got) != nil {
			continue
		}
		list := youTubeTrailers(got.Results)
		if len(list) == 0 {
			continue
		}
		// Official first, then English.
		sort.SliceStable(list, func(a, b int) bool {
			if list[a].Official != list[b].Official {
				return list[a].Official
			}
			return list[a].Lang == "en" && list[b].Lang != "en"
		})
		videos, _ := obj["videos"].(map[string]any)
		if videos == nil {
			videos = map[string]any{}
		}
		results, _ := videos["results"].([]any)
		var best map[string]any
		b, _ := json.Marshal(list[0])
		_ = json.Unmarshal(b, &best)
		best["season_number"] = n
		videos["results"] = append([]any{best}, results...)
		obj["videos"] = videos
		if out, err := json.Marshal(obj); err == nil {
			r.Data = out
		}
		return r
	}
	return r
}

package musictags

import (
	"encoding/json"
	"io"
	"sort"
	"strings"
)

type lbTag struct {
	ArtistMBID string `json:"artist_mbid"`
	Count      int    `json:"count"`
	Tag        string `json:"tag"`
}

type lbGroup struct {
	CAAID *int64 `json:"caa_id"`
	Date  string `json:"date"`
	Name  string `json:"name"`
	Type  string `json:"type"`
}

func toTags(l []lbTag, artist string) []Tag {
	var out []Tag
	for _, t := range l {
		if artist != "" && !strings.EqualFold(t.ArtistMBID, artist) {
			continue
		}
		if n := strings.ToLower(strings.TrimSpace(t.Tag)); n != "" && t.Count > 0 {
			out = append(out, Tag{Name: n, Votes: t.Count})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Votes > out[j].Votes })
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}

// ParseMBGroups reads a MusicBrainz release group browse page: studio
// albums, EPs and soundtracks (type "Soundtrack"); live, compilation,
// remix and other secondary types are left out. The debut is the earliest
// studio Album. It also answers the artist's total release group count.
func ParseMBGroups(r io.Reader) ([]Album, int, error) {
	var page struct {
		Count  int `json:"release-group-count"`
		Groups []struct {
			ID        string   `json:"id"`
			Title     string   `json:"title"`
			Primary   string   `json:"primary-type"`
			Secondary []string `json:"secondary-types"`
			Date      string   `json:"first-release-date"`
		} `json:"release-groups"`
	}
	if err := json.NewDecoder(r).Decode(&page); err != nil {
		return nil, 0, err
	}
	var out []Album
	debut, debutDate := -1, ""
	for _, g := range page.Groups {
		if g.Primary != "Album" && g.Primary != "EP" {
			continue
		}
		typ := g.Primary
		switch {
		case len(g.Secondary) == 0:
		case len(g.Secondary) == 1 && g.Secondary[0] == "Soundtrack":
			typ = "Soundtrack"
		default:
			continue
		}
		if typ == "Album" && len(g.Date) >= 4 && (debutDate == "" || g.Date < debutDate) {
			debut, debutDate = len(out), g.Date
		}
		out = append(out, Album{ID: strings.ToLower(g.ID), Title: g.Title, Date: g.Date, Type: typ})
	}
	if debut >= 0 {
		out[debut].Debut = true
	}
	return out, page.Count, nil
}

// ParsePopularity reads a release group popularity answer: per id, the
// total listens and listeners.
func ParsePopularity(r io.Reader) (map[string][2]int64, error) {
	var rows []struct {
		ID        string `json:"release_group_mbid"`
		Listens   *int64 `json:"total_listen_count"`
		Listeners *int64 `json:"total_user_count"`
	}
	if err := json.NewDecoder(r).Decode(&rows); err != nil {
		return nil, err
	}
	out := map[string][2]int64{}
	for _, x := range rows {
		var v [2]int64
		if x.Listens != nil {
			v[0] = *x.Listens
		}
		if x.Listeners != nil {
			v[1] = *x.Listeners
		}
		out[strings.ToLower(x.ID)] = v
	}
	return out, nil
}

// ParseMBSearch reads a MusicBrainz release group search page: studio
// albums, EPs and soundtracks with their first credited artist.
func ParseMBSearch(r io.Reader) ([]Album, error) {
	var page struct {
		Groups []struct {
			ID        string   `json:"id"`
			Title     string   `json:"title"`
			Primary   string   `json:"primary-type"`
			Secondary []string `json:"secondary-types"`
			Date      string   `json:"first-release-date"`
			Credit    []struct {
				Artist struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"artist-credit"`
		} `json:"release-groups"`
	}
	if err := json.NewDecoder(r).Decode(&page); err != nil {
		return nil, err
	}
	var out []Album
	for _, g := range page.Groups {
		if g.Primary != "Album" && g.Primary != "EP" || len(g.Credit) == 0 {
			continue
		}
		typ := g.Primary
		switch {
		case len(g.Secondary) == 0:
		case len(g.Secondary) == 1 && g.Secondary[0] == "Soundtrack":
			typ = "Soundtrack"
		default:
			continue
		}
		out = append(out, Album{ID: strings.ToLower(g.ID), Title: g.Title, Date: g.Date, Type: typ, ArtistID: strings.ToLower(g.Credit[0].Artist.ID), Artist: g.Credit[0].Artist.Name})
	}
	return out, nil
}

// Untagged answers the albums with no tags yet.
func (s *Source) Untagged() []string {
	var out []string
	for id, a := range s.albums {
		if len(a.Tags) == 0 {
			out = append(out, id)
		}
	}
	return out
}

// ParseGroupTags reads a release group metadata answer (with tags): per
// lower-case id, the album's tags, its artist tags, date, type and cover
// flag.
func ParseGroupTags(r io.Reader) (map[string]Album, error) {
	var m map[string]struct {
		RG  lbGroup `json:"release_group"`
		Tag struct {
			Artist []lbTag `json:"artist"`
			RG     []lbTag `json:"release_group"`
		} `json:"tag"`
	}
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		return nil, err
	}
	out := map[string]Album{}
	for id, v := range m {
		id = strings.ToLower(id)
		out[id] = Album{ID: id, Title: v.RG.Name, Date: v.RG.Date, Type: v.RG.Type, Tags: toTags(v.Tag.RG, ""), ArtistTags: toTags(v.Tag.Artist, ""), Art: v.RG.CAAID != nil && *v.RG.CAAID > 0}
	}
	return out, nil
}

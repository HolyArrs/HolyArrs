package musictrend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Extra global lists, all from ListenBrainz (keyless):
//   - fresh: notable album and EP releases of the last 7 days, most played first;
//   - rising: artists in this week's top 100 who are not in this year's;
//   - classic: the all-time most played release groups.
//
// Albums carry their release date and up to five tags (release group tags,
// then the artist's), read from ListenBrainz's metadata API.
const (
	KindFresh   = "fresh"
	KindRising  = "rising"
	KindClassic = "classic"
	classicSize = 100
)

type extraAlbum struct {
	Rank        int      `json:"rank"`
	MBID        string   `json:"mbid"`
	Title       string   `json:"title"`
	Artist      string   `json:"artist_name"`
	ArtistMBID  string   `json:"artist_mbid,omitempty"`
	CAARelease  string   `json:"caa_release_mbid,omitempty"`
	Listens     int64    `json:"listen_count,omitempty"`
	ReleaseDate string   `json:"release_date,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// lbRoot is the ListenBrainz API root (…/1) from the sitewide stats URL.
func (s *Service) lbRoot() string {
	return strings.TrimSuffix(strings.TrimRight(s.opts.ListenBrainzURL, "/"), "/stats/sitewide")
}

// ParseLBFresh reads the fresh releases answer: albums and EPs only, one
// per release group, most played first.
func ParseLBFresh(r io.Reader) ([]Entry, map[string]string, map[string][]string, error) {
	var body struct {
		Payload struct {
			Releases []struct {
				Artist      string   `json:"artist_credit_name"`
				ArtistMBIDs []string `json:"artist_mbids"`
				CAARelease  *string  `json:"caa_release_mbid"`
				Listens     int64    `json:"listen_count"`
				Date        string   `json:"release_date"`
				Group       string   `json:"release_group_mbid"`
				Type        string   `json:"release_group_primary_type"`
				Name        string   `json:"release_name"`
				Tags        []string `json:"release_tags"`
			} `json:"releases"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, nil, nil, err
	}
	dates, tags := map[string]string{}, map[string][]string{}
	seen := map[string]bool{}
	var out []Entry
	for _, x := range body.Payload.Releases {
		id := strings.ToLower(x.Group)
		t := strings.ToLower(x.Type)
		if !validMBID(id) || seen[id] || x.Name == "" || (t != "album" && t != "ep") {
			continue
		}
		seen[id] = true
		e := Entry{MBID: id, Name: x.Name, Artist: x.Artist, Listens: x.Listens}
		if len(x.ArtistMBIDs) > 0 && validMBID(strings.ToLower(x.ArtistMBIDs[0])) {
			e.ArtistMBID = strings.ToLower(x.ArtistMBIDs[0])
		}
		if x.CAARelease != nil && validMBID(strings.ToLower(*x.CAARelease)) {
			e.CAARelease = strings.ToLower(*x.CAARelease)
		}
		if e.ArtistMBID == "" {
			continue
		}
		dates[id], tags[id] = x.Date, cleanTags(x.Tags)
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Releases with a cover first, then the most played.
		if (out[i].CAARelease != "") != (out[j].CAARelease != "") {
			return out[i].CAARelease != ""
		}
		return out[i].Listens > out[j].Listens
	})
	return out, dates, tags, nil
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" && !seen[t] && len(out) < 5 {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// ParseLBGroupMeta reads the release group metadata answer: date and tags
// (release group tags by count, then the artist's) per id.
func ParseLBGroupMeta(r io.Reader) (map[string]string, map[string][]string, error) {
	type tag struct {
		Tag   string `json:"tag"`
		Count int    `json:"count"`
	}
	var body map[string]struct {
		Group struct {
			Date string `json:"date"`
		} `json:"release_group"`
		Tag struct {
			Artist []tag `json:"artist"`
			Group  []tag `json:"release_group"`
		} `json:"tag"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, nil, err
	}
	dates, tags := map[string]string{}, map[string][]string{}
	for id, m := range body {
		id = strings.ToLower(id)
		dates[id] = m.Group.Date
		var names []string
		for _, l := range [][]tag{m.Tag.Group, m.Tag.Artist} {
			l = append([]tag{}, l...)
			sort.SliceStable(l, func(i, j int) bool { return l[i].Count > l[j].Count })
			for _, t := range l {
				names = append(names, t.Tag)
			}
		}
		tags[id] = cleanTags(names)
	}
	return dates, tags, nil
}

// groupMeta reads dates and tags for ids, 25 at a time.
func (s *Service) groupMeta(ctx context.Context, ids []string, dates map[string]string, tags map[string][]string) {
	for i := 0; i < len(ids); i += 25 {
		part := ids[i:min(i+25, len(ids))]
		q := url.Values{"release_group_mbids": {strings.Join(part, ",")}, "inc": {"tag"}}
		_ = s.get(ctx, s.lbRoot()+"/metadata/release_group/?"+q.Encode(), func(r io.Reader) error {
			d, t, err := ParseLBGroupMeta(r)
			for k, v := range d {
				if dates[k] == "" {
					dates[k] = v
				}
			}
			for k, v := range t {
				if len(tags[k]) == 0 {
					tags[k] = v
				}
			}
			return err
		})
		if ctx.Err() != nil {
			return
		}
	}
}

// refreshExtras rebuilds the fresh, rising and classic lists. A list that
// fails keeps its last good copy.
func (s *Service) refreshExtras(ctx context.Context) (map[listKey][]byte, error) {
	out := map[listKey][]byte{}
	now := time.Now().Unix()
	var errs []error
	album := func(kind string, entries []Entry, size int, dates map[string]string, tags map[string][]string) {
		var ids []string
		for _, e := range entries[:min(size, len(entries))] {
			if dates[e.MBID] == "" || len(tags[e.MBID]) == 0 {
				ids = append(ids, e.MBID)
			}
		}
		s.groupMeta(ctx, ids, dates, tags)
		items := []extraAlbum{}
		for _, e := range entries {
			if len(items) == size {
				break
			}
			if e.ArtistMBID == "" {
				continue
			}
			items = append(items, extraAlbum{Rank: len(items) + 1, MBID: e.MBID, Title: e.Name, Artist: e.Artist, ArtistMBID: e.ArtistMBID, CAARelease: e.CAARelease, Listens: e.Listens, ReleaseDate: dates[e.MBID], Tags: tags[e.MBID]})
		}
		if len(items) == 0 {
			return
		}
		b, err := json.Marshal(map[string]any{"schema_version": listsSchema, "kind": kind, "region": "global", "source": "listenbrainz", "updated": now, "items": items, "regions": []string{"global"}})
		if err == nil {
			out[listKey{kind, "global"}] = b
		}
	}

	var fresh []Entry
	dates, tags := map[string]string{}, map[string][]string{}
	err := s.get(ctx, s.lbRoot()+"/explore/fresh-releases/?days=7&sort=release_date&past=true&future=false", func(r io.Reader) (err error) {
		fresh, dates, tags, err = ParseLBFresh(r)
		return err
	})
	if err != nil {
		errs = append(errs, err)
	} else {
		album(KindFresh, fresh, s.opts.Size, dates, tags)
	}

	var classic []Entry
	for _, off := range []int{0, 100} {
		err := s.get(ctx, s.lbRoot()+"/stats/sitewide/release-groups?range=all_time&count=100&offset="+itoa(off), func(r io.Reader) error {
			e, _, err := ParseLBAlbums(r)
			classic = append(classic, e...)
			return err
		})
		if err != nil {
			errs = append(errs, err)
			break
		}
	}
	if len(classic) > 0 {
		album(KindClassic, classic, classicSize, map[string]string{}, map[string][]string{})
	}

	// Rising: this week's ListenBrainz artists that are not in the year's.
	var week []Entry
	weekErr := s.get(ctx, s.lbRoot()+"/stats/sitewide/artists?range=week&count=100", func(r io.Reader) (err error) {
		week, _, err = ParseLBArtists(r)
		return err
	})
	if weekErr != nil {
		errs = append(errs, weekErr)
	} else {
		var year []Entry
		err := s.get(ctx, s.lbRoot()+"/stats/sitewide/artists?range=year&count=100", func(r io.Reader) (err error) {
			year, _, err = ParseLBArtists(r)
			return err
		})
		if err != nil {
			errs = append(errs, err)
		} else {
			old := map[string]bool{}
			for _, e := range year {
				old[e.MBID] = true
			}
			items := []artistItem{}
			for _, e := range week {
				if !old[e.MBID] && len(items) < s.opts.Size {
					items = append(items, artistItem{Rank: len(items) + 1, MBID: e.MBID, Name: e.Name, Listens: e.Listens})
				}
			}
			if len(items) > 0 {
				b, err := json.Marshal(map[string]any{"schema_version": listsSchema, "kind": KindRising, "region": "global", "source": "listenbrainz", "updated": now, "items": items, "regions": []string{"global"}})
				if err == nil {
					out[listKey{KindRising, "global"}] = b
				}
			}
		}
	}
	return out, errors.Join(errs...)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

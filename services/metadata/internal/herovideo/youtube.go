package herovideo

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	videoID      = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
	channelID    = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	channelLink  = regexp.MustCompile(`^https?://(?:www\.|m\.)?youtube\.com/channel/(UC[A-Za-z0-9_-]{22})(?:[/?#]|$)`)
	wikidataLink = regexp.MustCompile(`^https?://(?:www\.)?wikidata\.org/wiki/(Q[0-9]+)$`)
	isoDuration  = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)
	// errSkipped: a check could not run now (off, no key, paused, quota
	// spent); its videos stay unchecked.
	errSkipped = errors.New("herovideo: check skipped")
)

const (
	// topicChannel ends the name of YouTube's automatic channel for an
	// artist's audio, which holds no music videos.
	topicChannel = " - Topic"
	// shortMax: a video of this many seconds or less is no music video.
	shortMax = 60
)

// badTitle marks uploads that are audio, a visualizer or a lyric video.
var badTitle = regexp.MustCompile(`(?i)official audio|[(\[]audio[)\]]|visuali[sz]er|\blyrics?\b`)

// apiOn reports whether the Data API can be asked: a key, switched on.
func (s *Service) apiOn() bool { return s.o.Key != "" && s.on("youtube_api") }

// api makes one Data API call of 1 point from the shared daily quota. The
// key goes in a header, never in the address or an error. errSkipped when
// the API is off, paused or the day's quota is spent; a 403 (quota or key)
// or 429 pauses it for an hour and is skipped too. Other statuses and
// network errors are returned (the job tries again later).
func (s *Service) api(ctx context.Context, path string, q url.Values, out any) error {
	if !s.apiOn() || s.allow(ctx, srcAPI) != nil || (s.o.Limits != nil && !s.o.Limits.Spend("youtube", 1)) {
		return errSkipped
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.o.API+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Goog-Api-Key", s.o.Key)
	req.Header.Set("User-Agent", s.o.UserAgent)
	resp, err := s.o.HTTP.Do(req)
	if err != nil {
		s.done(srcAPI, 0, err)
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return errors.New("herovideo: YouTube Data API: " + strings.ReplaceAll(err.Error(), s.o.Key, "[redacted]"))
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		s.backoff(srcAPI, time.Hour)
		return errSkipped
	case resp.StatusCode != http.StatusOK:
		s.done(srcAPI, resp.StatusCode, nil)
		return &StatusError{resp.StatusCode}
	}
	s.done(srcAPI, resp.StatusCode, nil)
	return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

// flexInt reads a number YouTube may send as a string ("embedWidth").
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	n, _ := strconv.Atoi(strings.Trim(string(b), `"`))
	*f = flexInt(n)
	return nil
}

// seconds reads an ISO 8601 duration (PT4M13S); -1 when unreadable.
func seconds(d string) int {
	m := isoDuration.FindStringSubmatch(d)
	if m == nil {
		return -1
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	return n(m[1])*86400 + n(m[2])*3600 + n(m[3])*60 + n(m[4])
}

// shape is one video's shape and length.
type shape struct {
	ID      string `json:"id"`
	Details struct {
		Duration string `json:"duration"`
	} `json:"contentDetails"`
	Player struct {
		Width  flexInt `json:"embedWidth"`
		Height flexInt `json:"embedHeight"`
	} `json:"player"`
}

// verdict answers a shape's verdict: VerdictTall for a phone-shaped video
// (a Short at any length), VerdictShort for 60 seconds or less, "" when it
// passes. ok is false while the length is unknown (live or upcoming: P0D),
// so the video is looked at again later.
func (v shape) verdict() (verdict string, secs int, ok bool) {
	secs = seconds(v.Details.Duration)
	switch {
	case v.Player.Height > v.Player.Width:
		return VerdictTall, secs, true
	case secs <= 0:
		return "", 0, false
	case secs <= shortMax:
		return VerdictShort, secs, true
	}
	return "", secs, true
}

// shapes reads up to 50 videos' shape and length with one videos call
// (contentDetails and player, at maxWidth 1000 so the player has a size).
// A video missing from the answer is removed or private.
func (s *Service) shapes(ctx context.Context, ids []string) (map[string]shape, error) {
	var d struct {
		Items []shape `json:"items"`
	}
	q := url.Values{"part": {"contentDetails,player"}, "maxWidth": {"1000"}, "id": {strings.Join(ids, ",")}}
	if err := s.api(ctx, "/videos", q, &d); err != nil {
		return nil, err
	}
	out := make(map[string]shape, len(d.Items))
	for _, it := range d.Items {
		out[it.ID] = it
	}
	return out, nil
}

// playlistUploads lists a channel's 50 newest uploads with one
// playlistItems call on its uploads playlist (UC... as UU...).
func (s *Service) playlistUploads(ctx context.Context, ch, source string) ([]cand, error) {
	var d struct {
		Items []struct {
			Snippet struct {
				Title    string `json:"title"`
				Resource struct {
					VideoID string `json:"videoId"`
				} `json:"resourceId"`
			} `json:"snippet"`
		} `json:"items"`
	}
	q := url.Values{"part": {"snippet"}, "maxResults": {"50"}, "playlistId": {"UU" + ch[2:]}}
	err := s.api(ctx, "/playlistItems", q, &d)
	var se *StatusError
	if errors.As(err, &se) && se.Status == http.StatusNotFound {
		return nil, nil // no uploads, or the channel is gone
	}
	if err != nil {
		return nil, err
	}
	var out []cand
	for _, it := range d.Items {
		if id := it.Snippet.Resource.VideoID; videoID.MatchString(id) {
			out = append(out, cand{ID: id, Title: it.Snippet.Title, Source: source})
		}
	}
	return out, nil
}

// feed is a channel's public feed: its name and newest uploads.
type feed struct {
	Name    string
	Entries []cand
}

func (f feed) topic() bool { return strings.HasSuffix(f.Name, topicChannel) }

// readFeed reads a channel's public feed (free, no quota). YouTube answers
// a datacenter address's feed reads with 404 or 5xx at times, so any status
// but 200 is a failure (never "no uploads") and pauses feed reads: 30 s,
// doubling up to an hour, until one answers. An upload linked as a Short
// is marked so it is dropped without a check.
func (s *Service) readFeed(ctx context.Context, ch, source string) (f feed, err error) {
	if err := s.allow(ctx, srcFeed); err != nil {
		return f, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.o.Feeds+"/feeds/videos.xml?channel_id="+url.QueryEscape(ch), nil)
	if err != nil {
		return f, err
	}
	req.Header.Set("User-Agent", s.o.UserAgent)
	resp, err := s.o.HTTP.Do(req)
	if err != nil {
		s.done(srcFeed, 0, err)
		return f, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.backoff(srcFeed, retryAfter(resp))
		return f, &StatusError{resp.StatusCode}
	}
	s.done(srcFeed, resp.StatusCode, nil)
	var x struct {
		Title   string `xml:"title"`
		Entries []struct {
			VideoID string `xml:"videoId"`
			Title   string `xml:"title"`
			Links   []struct {
				Rel  string `xml:"rel,attr"`
				Href string `xml:"href,attr"`
			} `xml:"link"`
		} `xml:"entry"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&x); err != nil {
		return f, err
	}
	f.Name = strings.TrimSpace(x.Title)
	for _, e := range x.Entries {
		if !videoID.MatchString(e.VideoID) {
			continue
		}
		c := cand{ID: e.VideoID, Title: e.Title, Source: source}
		for _, l := range e.Links {
			if l.Rel == "alternate" && strings.Contains(l.Href, "/shorts/") {
				c.Short = true
			}
		}
		f.Entries = append(f.Entries, c)
	}
	return f, nil
}

// wikidataChannels answers the YouTube channel IDs (P2397) on the artist's
// Wikidata item, preferred first, deprecated ones left out.
func (s *Service) wikidataChannels(ctx context.Context, item string) ([]string, error) {
	q := url.Values{"action": {"wbgetclaims"}, "entity": {item}, "property": {"P2397"}, "format": {"json"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.o.Wikidata+"/w/api.php?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.o.UserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := s.o.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{resp.StatusCode}
	}
	var d struct {
		Claims map[string][]struct {
			Rank     string `json:"rank"`
			Mainsnak struct {
				Value struct {
					Value any `json:"value"`
				} `json:"datavalue"`
			} `json:"mainsnak"`
		} `json:"claims"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&d); err != nil {
		return nil, err
	}
	var preferred, normal []string
	for _, c := range d.Claims["P2397"] {
		id, _ := c.Mainsnak.Value.Value.(string)
		if !channelID.MatchString(id) {
			continue
		}
		switch c.Rank {
		case "preferred":
			preferred = append(preferred, id)
		case "normal", "":
			normal = append(normal, id)
		}
	}
	return append(preferred, normal...), nil
}

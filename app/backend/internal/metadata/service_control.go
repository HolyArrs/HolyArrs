package metadata

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fetch-ahead marks, the server's switch (/v1/control) and the artist's
// music videos. Every part is read by feature: an older metadata server
// without them answers 404 or leaves the fields out, and the app keeps its
// old behaviour.

type aheadKey struct{}

// Ahead marks ctx as a fetch made ahead of a person (a prebuild or a
// next-page read): the request carries X-Fetch: ahead (the same value from
// every install), and it is not sent while the server asked us to wait.
func Ahead(ctx context.Context) context.Context { return context.WithValue(ctx, aheadKey{}, true) }

// IsAhead says whether ctx was marked by Ahead.
func IsAhead(ctx context.Context) bool { v, _ := ctx.Value(aheadKey{}).(bool); return v }

// serviceHold is the time the server last asked us to wait (Retry-After on
// a 429 or 503 we gave up on). Fetch-ahead requests are not sent before it.
type serviceHold struct {
	mu    sync.Mutex
	until time.Time
}

// holdMax bounds one Retry-After; a longer one is read as this.
const holdMax = time.Hour

func (h *serviceHold) set(value string, body []byte, now time.Time) {
	d := time.Minute
	if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && n >= 0 {
		d = time.Duration(n) * time.Second
	} else if at, err := http.ParseTime(value); err == nil && at.After(now) {
		d = at.Sub(now)
	} else {
		var b struct {
			Error struct {
				Retry int `json:"retry_after_seconds"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &b) == nil && b.Error.Retry > 0 {
			d = time.Duration(b.Error.Retry) * time.Second
		}
	}
	d = min(d, holdMax)
	h.mu.Lock()
	if u := now.Add(d); u.After(h.until) {
		h.until = u
	}
	h.mu.Unlock()
}

func (h *serviceHold) held(now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return now.Before(h.until)
}

// HeldUntil is when fetch-ahead may ask the server again (zero: now).
func (s *Service) HeldUntil() time.Time {
	s.hold.mu.Lock()
	defer s.hold.mu.Unlock()
	return s.hold.until
}

// aheadRequest sets the fetch-ahead header on req when ctx is marked.
func aheadRequest(ctx context.Context, req *http.Request) {
	if IsAhead(ctx) {
		req.Header.Set("X-Fetch", "ahead")
	}
}

// Control is the server's switch for installs (GET /v1/control).
type Control struct {
	// Prefetch is on, slow (double the gaps, half the counts) or off (only
	// what a person asks for is fetched).
	Prefetch string `json:"prefetch"`
	// PrebuildMinHours is the shortest gap between two music prebuilds.
	PrebuildMinHours float64 `json:"prebuild_min_hours"`
	// MaxRPS caps fetch-ahead requests a second (0: our own pace).
	MaxRPS    float64         `json:"max_rps"`
	Providers map[string]bool `json:"providers,omitempty"`
}

// Normal answers the switch with unknown values read safely.
func (c Control) Normal() Control {
	switch c.Prefetch {
	case "on", "slow", "off":
	default:
		c.Prefetch = "on"
	}
	if c.PrebuildMinHours < 0 || c.PrebuildMinHours > 48 {
		c.PrebuildMinHours = 0
	}
	if c.MaxRPS < 0 {
		c.MaxRPS = 0
	}
	return c
}

// Control reads the server's switch. ErrNotFound means the server has none
// (an older server): the caller treats that as on.
func (s *Service) Control(ctx context.Context) (Control, error) {
	var c Control
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/v1/control", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return c, ErrServiceUnavailable
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return c, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return c, ErrServiceBusy
	case resp.StatusCode != http.StatusOK:
		return c, ErrServiceUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&c) != nil {
		return Control{}, ErrServiceInvalid
	}
	return c.Normal(), nil
}

// ArtistVideo is one official music video of an artist (a YouTube id),
// with the most replayed point in seconds when the server knows it.
type ArtistVideo struct {
	ID           string  `json:"id"`
	Title        string  `json:"title,omitempty"`
	MostReplayed float64 `json:"most_replayed,omitempty"`
	Duration     float64 `json:"duration,omitempty"`
}

var youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// ValidYouTubeID says whether id is an 11-character YouTube video id.
func ValidYouTubeID(id string) bool { return youtubeID.MatchString(id) }

// parseVideos reads a videos list: plain ids, or objects with id (or
// youtube_id) and the optional most_replayed and duration in seconds.
func parseVideos(raw json.RawMessage) []ArtistVideo {
	if len(raw) == 0 {
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	var out []ArtistVideo
	seen := map[string]bool{}
	for _, it := range list {
		var v ArtistVideo
		var id string
		if json.Unmarshal(it, &id) == nil {
			v.ID = id
		} else {
			var o struct {
				ID           string  `json:"id"`
				YouTubeID    string  `json:"youtube_id"`
				Title        string  `json:"title"`
				MostReplayed float64 `json:"most_replayed"`
				Duration     float64 `json:"duration"`
			}
			if json.Unmarshal(it, &o) != nil {
				continue
			}
			v = ArtistVideo{ID: o.ID, Title: o.Title, MostReplayed: o.MostReplayed, Duration: o.Duration}
			if o.YouTubeID != "" {
				v.ID = o.YouTubeID
			}
		}
		if !ValidYouTubeID(v.ID) || seen[v.ID] {
			continue
		}
		if v.MostReplayed < 0 || v.Duration > 0 && v.MostReplayed >= v.Duration {
			v.MostReplayed = 0
		}
		seen[v.ID] = true
		out = append(out, v)
		if len(out) == 50 {
			break
		}
	}
	return out
}

// ArtistVideos reads the official music videos the metadata server keeps
// with an artist (data.videos, or source_data.videos). None (an older
// server, or no videos) answers an empty list.
func (s *Service) ArtistVideos(ctx context.Context, mbid string) ([]ArtistVideo, error) {
	if !ValidMBID(mbid) {
		return nil, ErrServiceInvalid
	}
	mbid = strings.ToLower(mbid)
	e, err := s.get(ctx, "/v1/music/musicbrainz/artist/"+mbid, "musicbrainz", mbid, nil)
	if err != nil {
		return nil, err
	}
	var item struct {
		Videos     json.RawMessage `json:"videos"`
		SourceData json.RawMessage `json:"source_data"`
	}
	if json.Unmarshal(e.Data, &item) != nil {
		return nil, ErrServiceInvalid
	}
	if v := parseVideos(item.Videos); len(v) > 0 {
		return v, nil
	}
	var src struct {
		Videos json.RawMessage `json:"videos"`
	}
	_ = json.Unmarshal(item.SourceData, &src)
	return parseVideos(src.Videos), nil
}

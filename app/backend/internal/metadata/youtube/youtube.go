// Package youtube is HolyArrs's own small YouTube reader: for one video it
// finds the addresses HolyArrs plays (YouTube's HLS manifest and its H.264
// MP4s), the length, the "most replayed" point and the storyboard spec.
// Nothing is downloaded, and there is no login, no cookie and no age or
// content-warning bypass: a video YouTube keeps behind a sign-in is
// answered with a reason instead.
//
// It makes two calls, at once when it can: the watch page (YouTube's
// visitor id, the "most replayed" graph and the storyboard spec) and one
// player call as the Apple Vision Pro app, the client that answers with
// plain addresses without a JavaScript player or a proof-of-origin token.
// When YouTube moves that door, Client is the one place to change.
package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is how the player call introduces itself (the visionOS app).
var Client = struct {
	Name, Number, Version, UserAgent, Make, Model, OS, OSVersion string
}{
	Name: "VISIONOS", Number: "101", Version: "1.02",
	UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 15_7_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15",
	Make:      "Apple", Model: "RealityDevice17,1", OS: "visionOS", OSVersion: "26.5.23O471",
}

// pageAgent is the browser the watch page is read as.
const pageAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

// DefaultBase is YouTube's address.
const DefaultBase = "https://www.youtube.com"

// Limits on what is read.
const (
	maxPlayer  = 8 << 20  // the player answer
	maxPage    = 12 << 20 // the watch page
	visitorFor = time.Hour
)

// Reason says why a video gives no address. Video reasons are about that
// one video (another trailer may play); the others are about the door.
type Reason string

const (
	AgeRestricted Reason = "age_restricted" // YouTube asks to sign in to confirm the age
	Unavailable   Reason = "unavailable"    // removed, private or does not exist
	SignInNeeded  Reason = "sign_in"        // private or members-only
	Unplayable    Reason = "unplayable"     // blocked here, not embeddable, or a live stream not on
	ContentCheck  Reason = "content_check"  // behind a content warning
	BotCheck      Reason = "bot_check"      // "confirm you're not a bot"
	NoAddress     Reason = "no_address"     // only scrambled or SABR formats: the door changed
	Refused       Reason = "refused"        // YouTube answered an error status
	Network       Reason = "network"        // YouTube could not be reached or read
	BadID         Reason = "bad_id"
)

// VideoReason says whether r is about this one video.
func (r Reason) VideoReason() bool {
	switch r {
	case AgeRestricted, Unavailable, SignInNeeded, Unplayable, ContentCheck, BadID:
		return true
	}
	return false
}

// Error is a lookup that found no address.
type Error struct {
	Reason Reason
	// Detail is YouTube's own words or the failing step, for Health and logs.
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return string(e.Reason)
	}
	return string(e.Reason) + ": " + e.Detail
}

// ReasonOf is err's reason ("" when err is not from the reader).
func ReasonOf(err error) Reason {
	var e *Error
	if errors.As(err, &e) {
		return e.Reason
	}
	return ""
}

// Format is one plain MP4 address.
type Format struct {
	URL      string
	MimeType string
	Width    int
	Height   int
	// Audio says whether it carries sound (a muxed format).
	Audio bool
	// AVC says whether the picture is H.264.
	AVC bool
}

// Video is what one lookup found.
type Video struct {
	ID       string
	Title    string
	HLS      string // YouTube's adaptive HLS manifest (with sound), "" when none
	Formats  []Format
	Duration float64 // seconds, 0 when unknown
	// Peak is the start of the most replayed part, in seconds, and
	// PeakFound whether YouTube showed the graph at all.
	Peak      float64
	PeakFound bool
	// Storyboard is YouTube's storyboard spec ("" when none).
	Storyboard string
	// Page says the watch page was read, so a missing graph really is
	// missing (not just unread).
	Page bool
}

// Reader looks videos up. The zero value is ready.
type Reader struct {
	// Base is YouTube's address (tests use a fake).
	Base string
	HTTP *http.Client

	mu        sync.Mutex
	visitor   string
	visitorAt time.Time
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// ValidID says whether id looks like a YouTube video id.
func ValidID(id string) bool { return idRe.MatchString(id) }

func (r *Reader) base() string {
	if r.Base != "" {
		return strings.TrimRight(r.Base, "/")
	}
	return DefaultBase
}

// defaultHTTP follows redirects only within YouTube; the proxy settings of
// the environment apply.
var defaultHTTP = &http.Client{
	Timeout: 20 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !strings.HasSuffix(req.URL.Hostname(), "youtube.com") {
			return errors.New("redirect refused")
		}
		return nil
	},
}

func (r *Reader) client() *http.Client {
	if r.HTTP != nil {
		return r.HTTP
	}
	return defaultHTTP
}

func (r *Reader) cachedVisitor(now time.Time) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if now.Sub(r.visitorAt) > visitorFor {
		return ""
	}
	return r.visitor
}

func (r *Reader) keepVisitor(v string) {
	if v == "" {
		return
	}
	r.mu.Lock()
	r.visitor, r.visitorAt = v, time.Now()
	r.mu.Unlock()
}

// Look finds the video's addresses, its "most replayed" point and its
// storyboard. A failure is an *Error with a reason.
func (r *Reader) Look(ctx context.Context, id string) (Video, error) {
	return r.look(ctx, id, true)
}

// Addresses finds the addresses only: while a visitor id is kept, the watch
// page (about a second) is not read at all, so a trailer is ready after the
// player call alone. Peak and Storyboard are then empty.
func (r *Reader) Addresses(ctx context.Context, id string) (Video, error) {
	return r.look(ctx, id, false)
}

func (r *Reader) look(ctx context.Context, id string, withPage bool) (Video, error) {
	if !ValidID(id) {
		return Video{}, &Error{Reason: BadID}
	}
	var (
		pa      playerAnswer
		pErr    error
		pg      watchPage
		pgErr   error
		read    bool
		visitor = r.cachedVisitor(time.Now())
	)
	readPage := func() {
		pg, pgErr = r.page(ctx, id)
		read = true
		r.keepVisitor(pg.visitor)
	}
	if visitor != "" {
		// The visitor id is known: page and player call at once (or the
		// player call alone).
		var wg sync.WaitGroup
		if withPage {
			wg.Add(1)
			go func() { defer wg.Done(); readPage() }()
		}
		pa, pErr = r.player(ctx, id, visitor)
		wg.Wait()
		// A visitor id YouTube no longer takes is refused as a bot: once
		// more with the one a fresh page gives.
		if pErr == nil && botRefusal(pa) {
			if !read {
				readPage()
			}
			if pg.visitor != "" && pg.visitor != visitor {
				pa, pErr = r.player(ctx, id, pg.visitor)
			}
		}
	} else {
		// The player call needs a visitor id, so the page goes first.
		readPage()
		pa, pErr = r.player(ctx, id, pg.visitor)
	}
	if pErr != nil {
		return Video{}, pErr
	}
	v := Video{ID: id, Title: pa.VideoDetails.Title}
	v.Duration, _ = strconv.ParseFloat(pa.VideoDetails.Length, 64)
	if read && pgErr == nil {
		v.Page = true
		v.Peak, v.PeakFound = pg.peak, pg.peakFound
		if v.Duration == 0 {
			v.Duration = pg.duration
		}
		if v.Title == "" {
			v.Title = pg.title
		}
	}
	// The watch page lists every storyboard size; the visionOS answer fewer.
	v.Storyboard = pa.Storyboards.Spec.Spec
	if pg.storyboard != "" {
		v.Storyboard = pg.storyboard
	}
	if err := playable(pa); err != nil {
		return v, err
	}
	sd := pa.StreamingData
	scrambled := 0
	for _, f := range append(append([]playerFormat{}, sd.Formats...), sd.Adaptive...) {
		if f.URL == "" {
			if f.Cipher != "" {
				scrambled++
			}
			continue
		}
		if !strings.HasPrefix(f.MimeType, "video/mp4") {
			continue
		}
		v.Formats = append(v.Formats, Format{URL: f.URL, MimeType: f.MimeType, Width: f.Width, Height: f.Height,
			Audio: strings.Contains(f.MimeType, "mp4a"), AVC: strings.Contains(f.MimeType, "avc1")})
	}
	v.HLS = sd.HLS
	if v.HLS == "" && len(v.Formats) == 0 {
		switch {
		case sd.SABR != "":
			return v, &Error{Reason: NoAddress, Detail: "only SABR formats"}
		case scrambled > 0:
			return v, &Error{Reason: NoAddress, Detail: "only scrambled addresses"}
		}
		return v, &Error{Reason: NoAddress, Detail: "no playable format"}
	}
	return v, nil
}

type playerFormat struct {
	URL      string `json:"url"`
	Cipher   string `json:"signatureCipher"`
	MimeType string `json:"mimeType"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type playerAnswer struct {
	PlayabilityStatus struct {
		Status   string   `json:"status"`
		Reason   string   `json:"reason"`
		Messages []string `json:"messages"`
	} `json:"playabilityStatus"`
	StreamingData struct {
		HLS      string         `json:"hlsManifestUrl"`
		Formats  []playerFormat `json:"formats"`
		Adaptive []playerFormat `json:"adaptiveFormats"`
		SABR     string         `json:"serverAbrStreamingUrl"`
	} `json:"streamingData"`
	VideoDetails struct {
		Title  string `json:"title"`
		Length string `json:"lengthSeconds"`
	} `json:"videoDetails"`
	Storyboards struct {
		Spec struct {
			Spec string `json:"spec"`
		} `json:"playerStoryboardSpecRenderer"`
	} `json:"storyboards"`
}

var ageRe = regexp.MustCompile(`(^|[^a-z])age([^a-z]|$)|age-restricted`)

func botRefusal(pa playerAnswer) bool {
	return pa.PlayabilityStatus.Status == "LOGIN_REQUIRED" && strings.Contains(strings.ToLower(pa.PlayabilityStatus.Reason), "bot")
}

// playable turns YouTube's playability status into a reason.
func playable(pa playerAnswer) error {
	p := pa.PlayabilityStatus
	if p.Status == "OK" {
		return nil
	}
	words := strings.TrimSpace(p.Reason)
	if words == "" && len(p.Messages) > 0 {
		words = strings.TrimSpace(p.Messages[0])
	}
	low := strings.ToLower(words)
	reason := Unplayable
	switch {
	case botRefusal(pa):
		reason = BotCheck
	case strings.Contains(low, "try again later"):
		// YouTube is holding this door back for a while, not this video.
		reason = Refused
	case ageRe.MatchString(low), p.Status == "AGE_CHECK_REQUIRED", p.Status == "AGE_VERIFICATION_REQUIRED":
		reason = AgeRestricted
	case p.Status == "CONTENT_CHECK_REQUIRED":
		reason = ContentCheck
	case p.Status == "LOGIN_REQUIRED":
		reason = SignInNeeded
	case p.Status == "ERROR":
		reason = Unavailable
	case p.Status == "":
		reason = NoAddress
		words = "no playability status"
	}
	return &Error{Reason: reason, Detail: words}
}

func (r *Reader) player(ctx context.Context, id, visitor string) (playerAnswer, error) {
	var pa playerAnswer
	c := Client
	body, _ := json.Marshal(map[string]any{
		"context": map[string]any{"client": map[string]any{
			"clientName": c.Name, "clientVersion": c.Version, "deviceMake": c.Make, "deviceModel": c.Model,
			"userAgent": c.UserAgent, "osName": c.OS, "osVersion": c.OSVersion, "hl": "en", "gl": "US", "visitorData": visitor,
		}},
		"videoId": id,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base()+"/youtubei/v1/player?prettyPrint=false", bytes.NewReader(body))
	if err != nil {
		return pa, &Error{Reason: Network, Detail: "player call"}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("X-YouTube-Client-Name", c.Number)
	req.Header.Set("X-YouTube-Client-Version", c.Version)
	req.Header.Set("Origin", DefaultBase)
	if visitor != "" {
		req.Header.Set("X-Goog-Visitor-Id", visitor)
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return pa, &Error{Reason: Network, Detail: "player call"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return pa, &Error{Reason: Refused, Detail: fmt.Sprintf("player call answered %d", resp.StatusCode)}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPlayer+1))
	if err != nil || len(b) > maxPlayer || json.Unmarshal(b, &pa) != nil {
		return pa, &Error{Reason: Network, Detail: "player answer unreadable"}
	}
	return pa, nil
}

type watchPage struct {
	visitor    string
	title      string
	duration   float64
	storyboard string
	peak       float64
	peakFound  bool
}

var visitorRe = regexp.MustCompile(`"VISITOR_DATA":"([A-Za-z0-9%_=-]{1,1000})"`)

// page reads the watch page; whatever it holds is used, and a missing part
// is simply missing.
func (r *Reader) page(ctx context.Context, id string) (watchPage, error) {
	var w watchPage
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base()+"/watch?"+url.Values{"v": {id}, "hl": {"en"}}.Encode(), nil)
	if err != nil {
		return w, err
	}
	req.Header.Set("User-Agent", pageAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	resp, err := r.client().Do(req)
	if err != nil {
		return w, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return w, fmt.Errorf("watch page answered %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxPage))
	if err != nil {
		return w, err
	}
	html := string(b)
	if m := visitorRe.FindStringSubmatch(html); m != nil {
		w.visitor = m[1]
	}
	var pr playerAnswer
	if jsonAfter(html, "ytInitialPlayerResponse = ", &pr) == nil {
		w.title = pr.VideoDetails.Title
		w.duration, _ = strconv.ParseFloat(pr.VideoDetails.Length, 64)
		w.storyboard = pr.Storyboards.Spec.Spec
	}
	var data any
	if jsonAfter(html, "ytInitialData = ", &data) == nil {
		marks := heatMarks(data)
		w.peak, w.peakFound = Peak(marks)
		// Without the player part (YouTube's bot check can withhold it) the
		// graph still spans the whole video: its parts give the length.
		if w.duration == 0 && len(marks) >= 2 {
			sort.Slice(marks, func(i, j int) bool { return marks[i].Start < marks[j].Start })
			last, prev := marks[len(marks)-1].Start, marks[len(marks)-2].Start
			w.duration = last + (last - prev)
		}
	}
	return w, nil
}

// jsonAfter decodes the JSON value that starts right after marker.
func jsonAfter(s, marker string, v any) error {
	i := strings.Index(s, marker)
	if i < 0 {
		return errors.New("not found")
	}
	return json.NewDecoder(strings.NewReader(s[i+len(marker):])).Decode(v)
}

// Mark is one part of the "most replayed" graph: its start in seconds and
// how often it is replayed (0 to 1).
type Mark struct{ Start, Value float64 }

// heatMarks finds the graph's parts anywhere in the page data (YouTube has
// used two shapes for them).
func heatMarks(v any) []Mark {
	var marks []Mark
	var walk func(any, int)
	walk = func(v any, depth int) {
		if depth > 64 {
			return
		}
		switch t := v.(type) {
		case map[string]any:
			if t["markerType"] == "MARKER_TYPE_HEATMAP" {
				if ms, ok := t["markers"].([]any); ok {
					for _, m := range ms {
						mm, _ := m.(map[string]any)
						s, sok := num(mm["startMillis"])
						in, iok := num(mm["intensityScoreNormalized"])
						if sok && iok {
							marks = append(marks, Mark{s / 1000, in})
						}
					}
				}
			}
			if h, ok := t["heatMarkerRenderer"].(map[string]any); ok {
				s, sok := num(h["timeRangeStartMillis"])
				in, iok := num(h["heatMarkerIntensityScoreNormalized"])
				if sok && iok {
					marks = append(marks, Mark{s / 1000, in})
				}
			}
			for _, c := range t {
				walk(c, depth+1)
			}
		case []any:
			for _, c := range t {
				walk(c, depth+1)
			}
		}
	}
	walk(v, 0)
	return marks
}

// Peak is the start of the most replayed part. The opening is nearly always
// the highest part, because every play starts there, so the first part and
// the first 5% of the video are skipped; with only one part, that one.
func Peak(marks []Mark) (float64, bool) {
	if len(marks) == 0 {
		return 0, false
	}
	m := append([]Mark{}, marks...)
	sort.Slice(m, func(i, j int) bool { return m[i].Start < m[j].Start })
	if len(m) == 1 {
		return m[0].Start, true
	}
	// The last 10% are skipped as well: a start there leaves nothing to watch
	// (trailers and music videos often peak at the closing card).
	last, prev := m[len(m)-1].Start, m[len(m)-2].Start
	length := last + (last - prev)
	cut, end := 0.05*length, 0.9*length
	peak, best := m[1].Start, -1.0
	for _, x := range m[1:] {
		if x.Start >= cut && x.Start < end && x.Value > best {
			peak, best = x.Start, x.Value
		}
	}
	return peak, true
}

func num(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case string:
		f, err := strconv.ParseFloat(t, 64)
		return f, err == nil
	}
	return 0, false
}

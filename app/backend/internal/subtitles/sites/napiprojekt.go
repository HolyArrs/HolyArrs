package sites

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/subtext"
)

// napiprojekt finds Polish subtitles by the video file's own NapiProjekt hash:
// the MD5 of its first 10 MiB. It uses the site's old hash endpoint, which
// answers with the subtitle itself. The site's search pages sit behind a bot
// check, so they are not used. Files come in several old formats and usually in
// Windows-1250; Download hands back UTF-8, as SRT whenever the timing allows.
type napiprojekt struct {
	c *client
}

const (
	napiHashBytes = 10 << 20
	napiNotFound  = "NPc0"
)

func newNapiProjekt(c *client) *napiprojekt {
	c.keyHeader = ""
	return &napiprojekt{c: c}
}

// NapiHash returns the NapiProjekt hash of a video file: the MD5 of its first
// 10 MiB, as 32 hex digits.
func NapiHash(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() || st.Size() < 1<<20 {
		return "", fmt.Errorf("file too small to hash")
	}
	h := md5.New()
	if _, err := io.Copy(h, io.LimitReader(f, napiHashBytes)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// napiToken is the check value the site wants beside the hash.
func napiToken(hash string) string {
	idx := []int{0xe, 0x3, 0x6, 0x8, 0x2}
	mul := []int64{2, 2, 5, 4, 3}
	add := []int{0, 0xd, 0x10, 0xb, 0x5}
	var out strings.Builder
	for i := range idx {
		d, _ := strconv.ParseInt(hash[idx[i]:idx[i]+1], 16, 64)
		t := add[i] + int(d)
		// The reference client slices past the end without complaint, which
		// leaves one digit; do the same.
		v, _ := strconv.ParseInt(hash[t:min(t+2, len(hash))], 16, 64)
		s := strconv.FormatInt(v*mul[i], 16)
		out.WriteByte(s[len(s)-1])
	}
	return out.String()
}

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (n *napiprojekt) fetch(ctx context.Context, hash string) ([]byte, error) {
	v := url.Values{}
	v.Set("l", "PL")
	v.Set("f", hash)
	v.Set("t", napiToken(hash))
	v.Set("v", "dreambox")
	v.Set("kolejka", "false")
	v.Set("nick", "")
	v.Set("pass", "")
	v.Set("napios", "Linux")
	raw, err := n.c.ref("/unit_napisy/dl.php?" + v.Encode())
	if err != nil {
		return nil, err
	}
	body, err := n.c.do(ctx, http.MethodGet, raw, nil, nil, "", maxDownloadBody, false)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte(napiNotFound)) || len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	return body, nil
}

func (n *napiprojekt) Search(ctx context.Context, q Query) ([]Candidate, error) {
	if (q.Kind != "movie" && q.Kind != "episode") || q.File == "" || !langWanted(wantedLangs(q.Languages), "pl") {
		return nil, nil
	}
	hash, err := NapiHash(q.File)
	if err != nil {
		return nil, nil
	}
	body, err := n.fetch(ctx, hash)
	if err != nil || body == nil {
		return nil, err
	}
	_, format, ok := napiConvert(body)
	if !ok {
		return nil, nil
	}
	cand := Candidate{Site: n.c.cfg.Name, Key: hash, Language: "pl", Title: q.Title, Year: q.Year, MatchedByID: true, Format: format}
	if q.Kind == "episode" {
		cand.Season, cand.Episode = q.Season, q.Episode
	}
	if q.ReleaseName != "" {
		// The hash matches this exact file, so the subtitle is made for this release.
		cand.ReleaseNames = []string{q.ReleaseName}
	}
	return []Candidate{cand}, nil
}

func (n *napiprojekt) Download(ctx context.Context, c Candidate) ([]byte, error) {
	hash := strings.ToLower(strings.TrimSpace(c.Key))
	if !hex32.MatchString(hash) {
		return nil, &Error{Kind: KindUnreadable, Msg: "the subtitle has no file hash"}
	}
	body, err := n.fetch(ctx, hash)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site no longer has this subtitle"}
	}
	out, _, ok := napiConvert(body)
	if !ok {
		return nil, &Error{Kind: KindUnreadable, Msg: "the site sent a subtitle in a format HolyArrs cannot read"}
	}
	return out, nil
}

// napiConvert turns a NapiProjekt file into UTF-8 and, where it can, into SRT.
// It returns the text, its format ("srt" or "sub") and whether it was read.
func napiConvert(b []byte) ([]byte, string, bool) {
	dec, _, err := subtext.Decode(b, "pl")
	if err != nil {
		return nil, "", false
	}
	text := strings.ReplaceAll(dec, "\r\n", "\n")
	if strings.Contains(text, "-->") {
		return []byte(text), "srt", true
	}
	lines := strings.Split(text, "\n")
	if cues, ok := napiMPL2(lines); ok {
		return napiSRT(cues), "srt", true
	}
	if cues, ok := napiMicroDVD(lines); ok {
		if cues != nil {
			return napiSRT(cues), "srt", true
		}
		return []byte(text), "sub", true // no frame rate given; kept as MicroDVD
	}
	if cues, ok := napiTMPlayer(lines); ok {
		return napiSRT(cues), "srt", true
	}
	return nil, "", false
}

type napiCue struct {
	start, end time.Duration
	text       string
}

var (
	reMPL2     = regexp.MustCompile(`^\[(\d+)\]\[(\d*)\](.*)$`)
	reMicroDVD = regexp.MustCompile(`^\{(\d+)\}\{(\d*)\}(.*)$`)
	reTMP      = regexp.MustCompile(`^(\d{1,2}):(\d{2}):(\d{2})[:=](.*)$`)
)

// napiLine turns the old "|" line breaks and "/" italics into SRT text.
func napiLine(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "|")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "/") {
			p = "<i>" + strings.TrimSpace(p[1:]) + "</i>"
		}
		parts[i] = p
	}
	return strings.Join(parts, "\n")
}

func napiMPL2(lines []string) ([]napiCue, bool) {
	var cues []napiCue
	for _, l := range lines {
		m := reMPL2.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		s, _ := strconv.ParseInt(m[1], 10, 64)
		e, _ := strconv.ParseInt(m[2], 10, 64)
		cues = append(cues, napiCue{start: napiTenths(s), end: napiTenths(e), text: napiLine(m[3])})
	}
	return napiFix(cues), len(cues) > 0
}

// napiMicroDVD reads frame-timed lines. The usual first line "{1}{1}23.976"
// gives the frame rate; without it the cues come back nil and the file stays as
// it is.
func napiMicroDVD(lines []string) ([]napiCue, bool) {
	var fps float64
	var cues []napiCue
	seen := false
	for _, l := range lines {
		m := reMicroDVD.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		seen = true
		if len(cues) == 0 && fps == 0 {
			if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(m[3]), ",", "."), 64); err == nil && f > 10 && f < 100 {
				fps = f
				continue
			}
		}
		s, _ := strconv.ParseInt(m[1], 10, 64)
		e, _ := strconv.ParseInt(m[2], 10, 64)
		cues = append(cues, napiCue{start: time.Duration(s), end: time.Duration(e), text: napiLine(m[3])})
	}
	if !seen {
		return nil, false
	}
	if fps == 0 {
		return nil, true
	}
	frame := func(n time.Duration) time.Duration {
		d := math.Round(float64(n) / fps * float64(time.Second))
		if d > float64(napiMaxCue) {
			return napiMaxCue + 1
		}
		return time.Duration(d)
	}
	for i := range cues {
		cues[i].start, cues[i].end = frame(cues[i].start), frame(cues[i].end)
	}
	return napiFix(cues), true
}

func napiTMPlayer(lines []string) ([]napiCue, bool) {
	var cues []napiCue
	for _, l := range lines {
		m := reTMP.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		h, _ := strconv.Atoi(m[1])
		mi, _ := strconv.Atoi(m[2])
		s, _ := strconv.Atoi(m[3])
		cues = append(cues, napiCue{start: time.Duration(h)*time.Hour + time.Duration(mi)*time.Minute + time.Duration(s)*time.Second, text: napiLine(m[4])})
	}
	return napiFix(cues), len(cues) > 0
}

// napiMaxCue is the latest time a cue may start or end. Anything later is a
// broken line (a huge number in a timing field) and is dropped, so it cannot
// wrap around into a wrong time.
const napiMaxCue = 100 * time.Hour

// napiTenths turns an MPL2 time in tenths of a second into a duration, past
// napiMaxCue for numbers too big to hold.
func napiTenths(n int64) time.Duration {
	if n > int64(napiMaxCue/(100*time.Millisecond)) {
		return napiMaxCue + 1
	}
	return time.Duration(n) * 100 * time.Millisecond
}

// napiFix drops broken cues and fills missing end times: up to the next cue,
// at most 5 seconds.
func napiFix(cues []napiCue) []napiCue {
	out := cues[:0]
	for i, c := range cues {
		if c.text == "" || c.start > napiMaxCue {
			continue
		}
		if c.end > napiMaxCue {
			c.end = 0
		}
		if c.end <= c.start {
			c.end = c.start + 5*time.Second
			if i+1 < len(cues) && cues[i+1].start > c.start && cues[i+1].start < c.end {
				c.end = cues[i+1].start
			}
		}
		out = append(out, c)
	}
	return out
}

func napiSRT(cues []napiCue) []byte {
	ts := func(d time.Duration) string {
		ms := d.Milliseconds()
		return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
	}
	var b strings.Builder
	for i, c := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, ts(c.start), ts(c.end), c.text)
	}
	return []byte(b.String())
}

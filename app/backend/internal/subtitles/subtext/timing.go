package subtext

import (
	"errors"
	"fmt"
	"strings"
)

// Limits on a timing fix.
const (
	MaxOffsetMS = 600000 // ten minutes either way
	MinFPS      = 1.0
	MaxFPS      = 120.0
	maxTimeMS   = 100*3600*1000 - 1
)

// Fix is a timing change. Times are first scaled by FromFPS/ToFPS when both
// are set, then shifted by OffsetMS.
type Fix struct {
	OffsetMS int64   `json:"offset_ms"`
	FromFPS  float64 `json:"from_fps,omitempty"`
	ToFPS    float64 `json:"to_fps,omitempty"`
}

// ErrBadFix is a timing change outside its limits.
var ErrBadFix = errors.New("timing change out of range")

// Check refuses offsets beyond ten minutes and frame rates outside 1 to 120.
func (f Fix) Check() error {
	if f.OffsetMS < -MaxOffsetMS || f.OffsetMS > MaxOffsetMS {
		return fmt.Errorf("%w: offset_ms must be within ±%d", ErrBadFix, MaxOffsetMS)
	}
	if (f.FromFPS == 0) != (f.ToFPS == 0) {
		return fmt.Errorf("%w: give both from_fps and to_fps, or neither", ErrBadFix)
	}
	if f.FromFPS != 0 && (f.FromFPS < MinFPS || f.FromFPS > MaxFPS || f.ToFPS < MinFPS || f.ToFPS > MaxFPS) {
		return fmt.Errorf("%w: frame rates must be %v to %v", ErrBadFix, MinFPS, MaxFPS)
	}
	if f.OffsetMS == 0 && f.FromFPS == 0 {
		return fmt.Errorf("%w: nothing to change", ErrBadFix)
	}
	return nil
}

func (f Fix) apply(ms int64) int64 {
	if f.FromFPS != 0 && f.ToFPS != 0 {
		ms = int64(float64(ms)*f.FromFPS/f.ToFPS + 0.5)
	}
	ms += f.OffsetMS
	return min(max(ms, 0), maxTimeMS)
}

// Retime applies fix to every cue of text in SRT, VTT, ASS or SSA and returns
// the new text. Everything except the times stays as it was.
func Retime(text, format string, fix Fix) (string, error) {
	if err := fix.Check(); err != nil {
		return "", err
	}
	if len(text) > MaxText {
		return "", ErrTooBig
	}
	switch format {
	case SRT, VTT:
		return retimeCues(text, fix, format == VTT)
	case ASS, SSA:
		return retimeASS(text, fix)
	}
	return "", fmt.Errorf("timing fixes work on srt, vtt, ass and ssa, not %s", format)
}

func retimeCues(text string, fix Fix, vtt bool) (string, error) {
	var sb strings.Builder
	sb.Grow(len(text) + len(text)/20)
	n := 0
	for line := range strings.Lines(text) {
		a, b, ok := cueTimes(line)
		if !ok {
			sb.WriteString(line)
			continue
		}
		n++
		body := strings.TrimRight(line, "\r\n")
		end := line[len(body):]
		_, rest, _ := strings.Cut(body, "-->")
		rest = strings.TrimLeft(rest, " \t")
		// rest starts with the end time; cue settings after it are kept.
		tail := ""
		if i := strings.IndexAny(rest, " \t"); i >= 0 {
			tail = rest[i:]
		}
		sb.WriteString(fmtCue(fix.apply(a), vtt) + " --> " + fmtCue(fix.apply(b), vtt) + tail + end)
	}
	if n == 0 {
		return "", ErrNotSubtitle
	}
	return sb.String(), nil
}

// cueTimes reads "HH:MM:SS,mmm --> HH:MM:SS,mmm". It also accepts a dot for the
// comma and MM:SS.mmm without hours, as WebVTT allows.
func cueTimes(line string) (int64, int64, bool) {
	l, r, ok := strings.Cut(strings.TrimSpace(line), "-->")
	if !ok || len(l) > 20 {
		return 0, 0, false
	}
	r = strings.TrimSpace(r)
	if i := strings.IndexAny(r, " \t"); i >= 0 {
		r = r[:i]
	}
	a, ok1 := parseClock(strings.TrimSpace(l))
	b, ok2 := parseClock(r)
	return a, b, ok1 && ok2
}

// parseClock reads [H]H:MM:SS[,.]mmm or MM:SS.mmm into milliseconds.
func parseClock(s string) (int64, bool) {
	if len(s) < 8 || len(s) > 16 {
		return 0, false
	}
	sep := strings.LastIndexAny(s, ",.")
	if sep < 0 || len(s)-sep-1 < 1 || len(s)-sep-1 > 3 {
		return 0, false
	}
	frac, ok := digits(s[sep+1:])
	if !ok {
		return 0, false
	}
	for i := len(s) - sep - 1; i < 3; i++ {
		frac *= 10
	}
	parts := strings.Split(s[:sep], ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	var v int64
	for i, p := range parts {
		n, ok := digits(p)
		if !ok || len(p) == 0 || len(p) > 3 || (i > 0 && (len(p) != 2 || n > 59)) {
			return 0, false
		}
		v = v*60 + n
	}
	return v*1000 + frac, true
}

func digits(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var v int64
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		v = v*10 + int64(s[i]-'0')
	}
	return v, true
}

func fmtCue(ms int64, vtt bool) string {
	sep := ","
	if vtt {
		sep = "."
	}
	return fmt.Sprintf("%02d:%02d:%02d%s%03d", ms/3600000, ms/60000%60, ms/1000%60, sep, ms%1000)
}

// retimeASS changes the Start and End fields of Dialogue lines, located by the
// [Events] Format line, or fields 2 and 3 when there is none.
func retimeASS(text string, fix Fix) (string, error) {
	start, end := 1, 2
	var sb strings.Builder
	sb.Grow(len(text) + len(text)/20)
	n := 0
	for line := range strings.Lines(text) {
		trim := strings.TrimSpace(line)
		if f, ok := strings.CutPrefix(trim, "Format:"); ok {
			for i, name := range strings.Split(f, ",") {
				switch strings.ToLower(strings.TrimSpace(name)) {
				case "start":
					start = i
				case "end":
					end = i
				}
			}
		}
		rest, ok := strings.CutPrefix(strings.TrimLeft(line, " \t"), "Dialogue:")
		if !ok {
			sb.WriteString(line)
			continue
		}
		// The last field is the text and may contain commas, so split only as far as needed.
		fields := strings.SplitN(rest, ",", max(start, end)+2)
		if len(fields) <= max(start, end) {
			sb.WriteString(line)
			continue
		}
		okAll := true
		for _, i := range []int{start, end} {
			ms, ok := parseASSClock(strings.TrimSpace(fields[i]))
			if !ok {
				okAll = false
				break
			}
			lead := fields[i][:len(fields[i])-len(strings.TrimLeft(fields[i], " "))]
			fields[i] = lead + fmtASS(fix.apply(ms))
		}
		if !okAll {
			sb.WriteString(line)
			continue
		}
		n++
		lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		sb.WriteString(lead + "Dialogue:" + strings.Join(fields, ","))
	}
	if n == 0 {
		return "", ErrNotSubtitle
	}
	return sb.String(), nil
}

// parseASSClock reads H:MM:SS.cc.
func parseASSClock(s string) (int64, bool) {
	if len(s) < 7 || len(s) > 12 {
		return 0, false
	}
	return parseClock(s)
}

func fmtASS(ms int64) string {
	cs := (ms + 5) / 10
	return fmt.Sprintf("%d:%02d:%02d.%02d", cs/360000, cs/6000%60, cs/100%60, cs%100)
}

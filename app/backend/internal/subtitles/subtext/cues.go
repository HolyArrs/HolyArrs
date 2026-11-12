package subtext

import (
	"fmt"
	"sort"
	"strings"
)

// Cue is one timed block of a subtitle. Text keeps its line breaks as \n and
// simple tags such as <i>.
type Cue struct {
	StartMS, EndMS int64
	Text           string
}

// MaxCues caps the cues read from one subtitle.
const MaxCues = 20000

// ParseCues reads the cues of SRT, VTT, ASS or SSA text in time order. ASS
// styling codes are dropped except italic, bold and underline, which become
// tags, and its \N breaks become new lines.
func ParseCues(text, format string) ([]Cue, error) {
	if len(text) > MaxText {
		return nil, ErrTooBig
	}
	var cues []Cue
	switch format {
	case SRT, VTT:
		cues = parseTimed(text)
	case ASS, SSA:
		cues = parseASS(text)
	default:
		return nil, fmt.Errorf("only srt, vtt, ass and ssa subtitles can be read as cues, not %s", format)
	}
	if len(cues) == 0 {
		return nil, ErrNotSubtitle
	}
	if len(cues) > MaxCues {
		return nil, ErrTooBig
	}
	sort.SliceStable(cues, func(i, j int) bool { return cues[i].StartMS < cues[j].StartMS })
	return cues, nil
}

func parseTimed(text string) []Cue {
	var out []Cue
	var cur *Cue
	var body []string
	flush := func() {
		if cur != nil {
			cur.Text = strings.Join(body, "\n")
			out = append(out, *cur)
		}
		cur, body = nil, nil
	}
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\r\n")
		if a, b, ok := cueTimes(line); ok {
			flush()
			if len(out) >= MaxCues+1 {
				break
			}
			cur = &Cue{StartMS: a, EndMS: b}
			continue
		}
		if cur == nil {
			continue // numbers, a WEBVTT header or a note before the next times
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		body = append(body, line)
	}
	flush()
	return out
}

func parseASS(text string) []Cue {
	start, end, textAt, fields := 1, 2, 9, 10
	var out []Cue
	for line := range strings.Lines(text) {
		trim := strings.TrimSpace(line)
		if f, ok := strings.CutPrefix(trim, "Format:"); ok {
			names := strings.Split(f, ",")
			fields = len(names)
			for i, name := range names {
				switch strings.ToLower(strings.TrimSpace(name)) {
				case "start":
					start = i
				case "end":
					end = i
				case "text":
					textAt = i
				}
			}
			continue
		}
		rest, ok := strings.CutPrefix(trim, "Dialogue:")
		if !ok || textAt != fields-1 || start >= fields || end >= fields {
			continue
		}
		parts := strings.SplitN(rest, ",", fields)
		if len(parts) != fields {
			continue
		}
		a, ok1 := parseASSClock(strings.TrimSpace(parts[start]))
		b, ok2 := parseASSClock(strings.TrimSpace(parts[end]))
		if !ok1 || !ok2 {
			continue
		}
		out = append(out, Cue{StartMS: a, EndMS: b, Text: ASSText(parts[textAt])})
		if len(out) > MaxCues {
			break
		}
	}
	return out
}

// ASSText turns the text field of an ASS or SSA event into plain cue text:
// override blocks go, except italic, bold and underline, which become tags;
// \N and \n become line breaks and \h a space.
func ASSText(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '{':
			j := strings.IndexByte(s[i:], '}')
			if j < 0 {
				return strings.TrimSpace(sb.String())
			}
			sb.WriteString(styleTags(s[i+1 : i+j]))
			i += j
		case c == '\\' && i+1 < len(s) && (s[i+1] == 'N' || s[i+1] == 'n'):
			sb.WriteByte('\n')
			i++
		case c == '\\' && i+1 < len(s) && s[i+1] == 'h':
			sb.WriteByte(' ')
			i++
		default:
			sb.WriteByte(c)
		}
	}
	return strings.TrimSpace(sb.String())
}

// styleTags turns the italic, bold and underline codes of one override block
// into tags, in the order they appear.
func styleTags(block string) string {
	var sb strings.Builder
	for _, code := range strings.Split(block, `\`) {
		if len(code) != 2 || (code[1] != '0' && code[1] != '1') {
			continue
		}
		var tag string
		switch code[0] {
		case 'i':
			tag = "i"
		case 'b':
			tag = "b"
		case 'u':
			tag = "u"
		default:
			continue
		}
		if code[1] == '0' {
			sb.WriteString("</" + tag + ">")
		} else {
			sb.WriteString("<" + tag + ">")
		}
	}
	return sb.String()
}

// WriteSRT writes cues as an SRT subtitle numbered from 1. A cue with no text
// is left out, since SRT readers stop at an empty cue.
func WriteSRT(cues []Cue) string {
	var sb strings.Builder
	n := 0
	for _, c := range cues {
		text := strings.TrimSpace(strings.ReplaceAll(c.Text, "\r\n", "\n"))
		if text == "" {
			continue
		}
		// A blank line inside a cue would end it early.
		for strings.Contains(text, "\n\n") {
			text = strings.ReplaceAll(text, "\n\n", "\n")
		}
		n++
		fmt.Fprintf(&sb, "%d\n%s --> %s\n%s\n\n", n, fmtCue(max(c.StartMS, 0), false), fmtCue(max(c.EndMS, c.StartMS, 0), false), text)
	}
	return sb.String()
}

package translate

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// decodeLoose reads JSON that may sit inside a code fence or after a short
// lead-in, as models often write it.
func decodeLoose(b []byte, v any) error {
	if err := json.Unmarshal(b, v); err == nil {
		return nil
	}
	i := bytes.IndexAny(b, "{[")
	if i < 0 {
		return json.Unmarshal(b, v)
	}
	return json.NewDecoder(bytes.NewReader(b[i:])).Decode(v)
}

// The names translators use for a line's number and its text.
var (
	numberKeys = []string{"n", "id", "number", "num", "index", "i", "position", "line_number"}
	textKeys   = []string{"text", "t", "translation", "translated", "line", "content"}
)

// readLines reads the lines of an answer. It takes a JSON object holding a
// list, a bare JSON list, or numbered text lines such as "3: hello". An answer
// cut off in the middle gives the lines that were complete.
func readLines(raw string) []Line {
	b := []byte(strings.TrimSpace(raw))
	if i := bytes.IndexAny(b, "{["); i >= 0 {
		if ls := jsonLines(b[i:]); len(ls) > 0 {
			return ls
		}
	}
	return numberedLines(raw)
}

// jsonLines finds the list of lines in JSON and reads its items one by one,
// so a cut-off list still gives its complete items.
func jsonLines(b []byte) []Line {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	// Walk to the first list: either the top value or the first list inside
	// the top object.
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		if d, ok := tok.(json.Delim); ok && d == '[' {
			break
		}
	}
	var out []Line
	for dec.More() {
		var item map[string]any
		if err := dec.Decode(&item); err != nil {
			break
		}
		if l, ok := lineOf(item); ok {
			out = append(out, l)
		}
	}
	return out
}

func lineOf(item map[string]any) (Line, bool) {
	n, okN := 0, false
	for _, k := range numberKeys {
		if v, ok := item[k]; ok {
			n, okN = number(v)
			break
		}
	}
	text, okT := "", false
	for _, k := range textKeys {
		if v, ok := item[k].(string); ok {
			text, okT = v, true
			break
		}
	}
	return Line{N: n, Text: text}, okN && okT
}

func number(v any) (int, bool) {
	var s string
	switch x := v.(type) {
	case json.Number:
		s = x.String()
	case float64:
		return int(x), x == float64(int(x))
	case string:
		s = strings.TrimSpace(x)
	default:
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// numberedLines reads text lines that start with a number, such as "12: text",
// "12. text", "[12] text" or "12) text".
func numberedLines(raw string) []Line {
	var out []Line
	for line := range strings.Lines(raw) {
		s := strings.TrimSpace(line)
		s = strings.TrimPrefix(s, "[")
		j := 0
		for j < len(s) && j < 6 && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j == 0 || j == len(s) || !strings.ContainsRune("].:)", rune(s[j])) {
			continue
		}
		n, _ := strconv.Atoi(s[:j])
		out = append(out, Line{N: n, Text: strings.TrimSpace(s[j+1:])})
	}
	return out
}

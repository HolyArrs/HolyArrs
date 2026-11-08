package ai

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// drop are answer fields an AI never needs: pictures, links, file paths
// and other bulk.
var drop = []string{"poster", "backdrop", "artwork", "art", "logo", "still", "photo", "picture", "image", "thumb", "avatar",
	"url", "path", "hash", "cover", "trailer", "color", "colour", "client_item", "streaming", "fresh", "swarm", "files", "links", "blur"}

// Tidy keeps an answer small and safe to hand an AI: no picture or link
// fields, short lists, short text without control or invisible characters.
func Tidy(v any, depth int) any {
	if depth > 6 {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			lk := strings.ToLower(k)
			if slices.ContainsFunc(drop, func(d string) bool { return strings.Contains(lk, d) }) {
				continue
			}
			if t := Tidy(val, depth+1); t != nil {
				out[clean(k, 64)] = t
			}
		}
		return out
	case []any:
		out := []any{}
		for _, val := range x {
			if len(out) == listMax {
				break
			}
			if t := Tidy(val, depth+1); t != nil {
				out = append(out, t)
			}
		}
		return out
	case string:
		return clean(x, TextMax)
	}
	return v
}

// clean drops control and invisible (zero-width, direction) characters
// and cuts s to max characters.
func clean(s string, max int) string {
	var b strings.Builder
	n := 0
	for _, c := range s {
		if c == utf8.RuneError || (unicode.IsControl(c) && c != '\n') || unicode.Is(unicode.Cf, c) || c == ' ' || c == ' ' {
			continue
		}
		if n == max {
			b.WriteString("…")
			break
		}
		b.WriteRune(c)
		n++
	}
	return strings.TrimSpace(b.String())
}

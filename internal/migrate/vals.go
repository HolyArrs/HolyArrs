package migrate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// str reads a value as text.
func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

// num reads a value as a whole number (0 when it is not one).
func num(v any) int64 {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n
		}
		f, _ := x.Float64()
		return int64(f)
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case bool:
		if x {
			return 1
		}
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	}
	return 0
}

// boolean reads a value as true or false (1, "true", "yes", "on" are true).
func boolean(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "on":
			return true
		}
		return false
	}
	return num(v) != 0
}

// obj reads a value as an object; a JSON text column is decoded.
func obj(v any) map[string]any {
	switch x := v.(type) {
	case map[string]any:
		return x
	case string:
		var m map[string]any
		if decodeJSON([]byte(x), DefaultLimits.MaxDepth, &m) == nil {
			return m
		}
	}
	return nil
}

// list reads a value as a list; a JSON text column is decoded.
func list(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case string:
		var l []any
		if decodeJSON([]byte(x), DefaultLimits.MaxDepth, &l) == nil {
			return l
		}
	}
	return nil
}

// ids reads a list of numbers (tag ids).
func ids(v any) []int64 {
	var out []int64
	for _, e := range list(v) {
		out = append(out, num(e))
	}
	return out
}

// normalize turns json.Number into int64 or float64 so plainValue keeps it.
func normalize(v any) any {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = normalize(e)
		}
	case []any:
		for i, e := range x {
			x[i] = normalize(e)
		}
	}
	return v
}

// iniFile is a parsed INI file: section (lower case, nested sections
// joined by "/") to key (lower case) to value. It reads SABnzbd's
// "[[sub]]" sections and Qt's "Group\Key" keys.
type iniFile map[string]map[string]string

func parseINI(data []byte) iniFile {
	out := iniFile{}
	top, cur := "", ""
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]") {
			cur = top + "/" + strings.ToLower(strings.TrimSpace(line[2:len(line)-2]))
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			top = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			cur = top
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if out[cur] == nil {
			out[cur] = map[string]string{}
		}
		out[cur][strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}

// subsections lists the nested sections of top ("servers" gives
// "servers/news.example.com"...).
func (f iniFile) subsections(top string) []string {
	var out []string
	for s := range f {
		if strings.HasPrefix(s, top+"/") {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// parseYAML reads the simple YAML Bazarr writes: nested mappings by
// indentation, scalar values, inline [a, b] lists and "- x" lists.
// Anything else is skipped. Keys are joined with "." ("general.enabled_providers").
func parseYAML(data []byte) map[string]any {
	out := map[string]any{}
	type level struct {
		indent int
		key    string
	}
	var stack []level
	var lastKey string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		raw := sc.Text()
		trim := strings.TrimSpace(raw)
		if trim == "" || strings.HasPrefix(trim, "#") || trim == "---" {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if strings.HasPrefix(trim, "- ") {
			if lastKey != "" {
				l, _ := out[lastKey].([]any)
				out[lastKey] = append(l, yamlScalar(strings.TrimSpace(trim[2:])))
			}
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		k, v, ok := strings.Cut(trim, ":")
		if !ok {
			continue
		}
		k = strings.Trim(strings.TrimSpace(k), `'"`)
		full := k
		if len(stack) > 0 {
			full = stack[len(stack)-1].key + "." + k
		}
		v = strings.TrimSpace(v)
		if v == "" {
			stack = append(stack, level{indent, full})
			lastKey = full
			continue
		}
		lastKey = ""
		out[full] = yamlScalar(v)
	}
	return out
}

func yamlScalar(v string) any {
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		var l []any
		for _, e := range strings.Split(v[1:len(v)-1], ",") {
			if e = strings.TrimSpace(e); e != "" {
				l = append(l, yamlScalar(e))
			}
		}
		return l
	}
	if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	switch strings.ToLower(v) {
	case "true":
		return true
	case "false":
		return false
	case "null", "~":
		return nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	return v
}

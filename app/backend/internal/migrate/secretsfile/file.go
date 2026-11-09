package secretsfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MaxSize is the largest secrets file read.
const MaxSize = 256 << 10

// Addr is an address-bearing block: an address and, when the owner says so,
// leave to reach the local network.
type Addr struct {
	Address           string `json:"address"`
	AllowLocalNetwork bool   `json:"allow_local_network"`
}

// KeyApp is an outside app reached with an address and an API key.
type KeyApp struct {
	Address           string `json:"address"`
	APIKey            string `json:"api_key"`
	AllowLocalNetwork bool   `json:"allow_local_network"`
}

// LoginApp is an outside app reached with an address, a user name and a
// password.
type LoginApp struct {
	Address           string `json:"address"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	AllowLocalNetwork bool   `json:"allow_local_network"`
}

// Indexer is one Source: a Torznab or Newznab feed.
type Indexer struct {
	Name              string `json:"name"`
	Address           string `json:"address"`
	APIKey            string `json:"api_key"`
	Format            string `json:"format"` // torznab (default) or newznab
	AllowLocalNetwork bool   `json:"allow_local_network"`
	ClearKey          bool   `json:"clear_key"` // an empty api_key keeps a stored key; this removes it
}

// File is the secrets file. Field names are the file's keys.
type File struct {
	Version int `json:"version"`
	TMDB    struct {
		ReadToken string `json:"read_token"`
		APIKey    string `json:"api_key"`
	} `json:"tmdb"`
	TVDB struct {
		APIKey        string `json:"api_key"`
		UniqueUserKey string `json:"unique_user_key"`
	} `json:"tvdb"`
	Trakt struct {
		ClientID     string `json:"client_id"`
		RedirectURLs string `json:"redirect_urls"`
	} `json:"trakt"`
	OpenSubtitles struct {
		APIKey   string `json:"api_key"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"opensubtitles"`
	Subdl struct {
		APIKey string `json:"api_key"`
	} `json:"subdl"`
	Addic7ed struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"addic7ed"`
	Jackett     KeyApp    `json:"jackett"`
	Prowlarr    KeyApp    `json:"prowlarr"`
	Indexers    []Indexer `json:"indexers"`
	QBittorrent struct {
		Address           string `json:"address"`
		APIKey            string `json:"api_key"` // qBittorrent 5.2 or newer; used before a user name and password
		Username          string `json:"username"`
		Password          string `json:"password"`
		AllowLocalNetwork bool   `json:"allow_local_network"`
	} `json:"qbittorrent"`
	SABnzbd struct {
		Address           string `json:"address"`
		APIKey            string `json:"api_key"`
		NZBKey            string `json:"nzb_key"`
		Username          string `json:"username"`
		Password          string `json:"password"`
		AllowLocalNetwork bool   `json:"allow_local_network"`
	} `json:"sabnzbd"`
	NZBGet       LoginApp `json:"nzbget"`
	Transmission LoginApp `json:"transmission"`
	Deluge       struct {
		Address           string `json:"address"`
		Password          string `json:"password"`
		AllowLocalNetwork bool   `json:"allow_local_network"`
	} `json:"deluge"`
	Jellyfin KeyApp `json:"jellyfin"`
	Emby     KeyApp `json:"emby"`
	Plex     struct {
		Address           string `json:"address"`
		Token             string `json:"token"`
		AllowLocalNetwork bool   `json:"allow_local_network"`
	} `json:"plex"`
	Radarr KeyApp `json:"radarr"`
	Sonarr KeyApp `json:"sonarr"`
	Lidarr KeyApp `json:"lidarr"`
	Bazarr KeyApp `json:"bazarr"`
	Seerr  KeyApp `json:"seerr"`
	SMTP   struct {
		Host              string `json:"host"`
		Port              string `json:"port"`
		Username          string `json:"username"`
		Password          string `json:"password"`
		From              string `json:"from"`
		AllowLocalNetwork bool   `json:"allow_local_network"`
	} `json:"smtp"`
	Discord struct {
		WebhookURL string `json:"webhook_url"`
	} `json:"discord"`
	Webhook struct {
		URL               string `json:"url"`
		SigningSecret     string `json:"signing_secret"`
		AllowLocalNetwork bool   `json:"allow_local_network"`
	} `json:"webhook"`
	HolyArrs struct {
		Address     string `json:"address"`
		OwnerAPIKey string `json:"owner_api_key"`
	} `json:"holyarrs"`
}

// Values lists every non-empty text value in f, so callers can make sure
// none of them is ever said.
func (f *File) Values() []string {
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, y := range x {
				walk(y)
			}
		case []any:
			for _, y := range x {
				walk(y)
			}
		case string:
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
	}
	b, _ := json.Marshal(f)
	var tree any
	_ = json.Unmarshal(b, &tree)
	walk(tree)
	return out
}

// ErrTooBig refuses a file over MaxSize.
var ErrTooBig = fmt.Errorf("the file is over %d KB; a secrets file is far smaller", MaxSize>>10)

// Parse reads a secrets file: a small, strict part of YAML (nested blocks
// by indentation, "- " list items, plain, 'single' or "double" quoted
// values, # comments). Unknown keys, repeated keys, tabs, anchors and
// multi-line values are refused with the line number.
func Parse(data []byte) (*File, error) {
	if len(data) > MaxSize {
		return nil, ErrTooBig
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	lines, err := split(string(data))
	if err != nil {
		return nil, err
	}
	p := &parser{lines: lines}
	var tree any = map[string]any{}
	if len(lines) > 0 {
		tree, err = p.block(lines[0].indent)
		if err != nil {
			return nil, err
		}
		if p.i < len(p.lines) {
			return nil, fmt.Errorf("line %d: indented less than the start of the file", p.lines[p.i].n)
		}
	}
	m, ok := tree.(map[string]any)
	if !ok {
		return nil, errors.New("the file must be a list of keys (version:, tmdb:, ...)")
	}
	if err := typed(m, ""); err != nil {
		return nil, err
	}
	return Decode(m)
}

// Decode turns a parsed tree (or a JSON body) into a File, refusing keys
// the file does not have.
func Decode(tree any) (*File, error) {
	b, err := json.Marshal(tree)
	if err != nil {
		return nil, errors.New("the file could not be read")
	}
	return DecodeJSON(b)
}

// DecodeJSON reads a File sent as JSON (the API takes the parsed file this
// way), refusing unknown keys.
func DecodeJSON(b []byte) (*File, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, plainDecodeError(err)
	}
	if f.Version != 1 {
		return nil, errors.New("version must be 1")
	}
	return &f, nil
}

// plainDecodeError rewords a JSON error without any value in it.
func plainDecodeError(err error) error {
	msg := err.Error()
	if name, ok := strings.CutPrefix(msg, "json: unknown field "); ok {
		return fmt.Errorf("unknown key %s: check the spelling against the template", name)
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		return fmt.Errorf("key %s has the wrong kind of value (expected %s)", te.Field, te.Type.String())
	}
	return errors.New("the file's layout does not match the template")
}

// typed turns the text of the few non-text keys into their kinds:
// version is a number, allow_local_network and clear_key are true or
// false. Everything
// else stays text.
func typed(m map[string]any, path string) error {
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			if err := typed(x, path+k+"."); err != nil {
				return err
			}
		case []any:
			for i, it := range x {
				if mm, ok := it.(map[string]any); ok {
					if err := typed(mm, fmt.Sprintf("%s%s[%d].", path, k, i+1)); err != nil {
						return err
					}
				}
			}
		case string:
			switch {
			case k == "allow_local_network" || k == "clear_key":
				switch strings.ToLower(x) {
				case "true", "yes", "on":
					m[k] = true
				case "false", "no", "off", "":
					m[k] = false
				default:
					return fmt.Errorf("%s%s must be true or false", path, k)
				}
			case k == "version" && path == "":
				n, err := strconv.Atoi(x)
				if err != nil {
					return errors.New("version must be 1")
				}
				m[k] = n
			}
		}
	}
	return nil
}

type line struct {
	n      int // 1-based
	indent int
	text   string // without indent and comment
}

// split cuts the file into meaningful lines.
func split(s string) ([]line, error) {
	var out []line
	for i, raw := range strings.Split(s, "\n") {
		raw = strings.TrimRight(raw, "\r")
		n := i + 1
		body := strings.TrimLeft(raw, " ")
		if strings.HasPrefix(body, "\t") || strings.Contains(raw[:len(raw)-len(body)], "\t") {
			return nil, fmt.Errorf("line %d: use spaces, not tabs, to indent", n)
		}
		text, err := stripComment(body, n)
		if err != nil {
			return nil, err
		}
		text = strings.TrimRight(text, " \t")
		if text == "" || (text == "---" && len(out) == 0) {
			continue
		}
		if text == "..." {
			break
		}
		out = append(out, line{n: n, indent: len(raw) - len(body), text: text})
	}
	return out, nil
}

// stripComment cuts a # comment that starts the line or follows a space,
// outside quotes.
func stripComment(s string, n int) (string, error) {
	var q byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q == '"' && c == '\\':
			i++
		case q != 0 && c == q:
			if q == '\'' && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			q = 0
		case q == 0 && (c == '"' || c == '\'') && (i == 0 || s[i-1] == ' ' || s[i-1] == ':' || s[i-1] == '-'):
			q = c
		case q == 0 && c == '#' && (i == 0 || s[i-1] == ' '):
			return s[:i], nil
		}
	}
	if q != 0 {
		return "", fmt.Errorf("line %d: a quoted value is not closed (values cannot span lines)", n)
	}
	return s, nil
}

type parser struct {
	lines []line
	i     int
}

// block reads a map or a list whose lines start at indent.
func (p *parser) block(indent int) (any, error) {
	if p.i < len(p.lines) && isItem(p.lines[p.i].text) {
		return p.list(indent)
	}
	return p.mapping(indent)
}

func isItem(t string) bool { return t == "-" || strings.HasPrefix(t, "- ") }

func (p *parser) mapping(indent int) (map[string]any, error) {
	m := map[string]any{}
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if l.indent < indent {
			break
		}
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: indented more than the key above it", l.n)
		}
		if isItem(l.text) {
			return nil, fmt.Errorf("line %d: a list item where a key was expected", l.n)
		}
		key, rest, ok := strings.Cut(l.text, ":")
		if !ok || (rest != "" && rest[0] != ' ') {
			return nil, fmt.Errorf("line %d: expected key: value", l.n)
		}
		key = strings.TrimSpace(key)
		if !validKey(key) {
			return nil, fmt.Errorf("line %d: keys are letters, digits and _ only", l.n)
		}
		if _, dup := m[key]; dup {
			return nil, fmt.Errorf("line %d: key %s appears twice", l.n, key)
		}
		rest = strings.TrimSpace(rest)
		p.i++
		if rest != "" {
			v, err := scalar(rest, l.n)
			if err != nil {
				return nil, err
			}
			m[key] = v
			continue
		}
		// A nested block: deeper lines, or list items at the same indent.
		if p.i < len(p.lines) {
			next := p.lines[p.i]
			if next.indent > indent || (next.indent == indent && isItem(next.text)) {
				v, err := p.block(next.indent)
				if err != nil {
					return nil, err
				}
				m[key] = v
				continue
			}
		}
		m[key] = ""
	}
	return m, nil
}

func (p *parser) list(indent int) ([]any, error) {
	out := []any{}
	for p.i < len(p.lines) {
		l := p.lines[p.i]
		if l.indent != indent || !isItem(l.text) {
			if l.indent > indent {
				return nil, fmt.Errorf("line %d: indented more than the list item above it", l.n)
			}
			break
		}
		rest := strings.TrimPrefix(strings.TrimPrefix(l.text, "-"), " ")
		pad := len(l.text) - len(strings.TrimLeft(rest, " "))
		rest = strings.TrimLeft(rest, " ")
		if rest == "" {
			p.i++
			if p.i < len(p.lines) && p.lines[p.i].indent > indent {
				v, err := p.block(p.lines[p.i].indent)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
				continue
			}
			out = append(out, "")
			continue
		}
		if strings.Contains(rest, ": ") || strings.HasSuffix(rest, ":") {
			// "- key: value" opens a map whose keys line up with key.
			p.lines[p.i] = line{n: l.n, indent: indent + pad, text: rest}
			v, err := p.mapping(indent + pad)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
			continue
		}
		v, err := scalar(rest, l.n)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		p.i++
	}
	return out, nil
}

func validKey(k string) bool {
	if k == "" || len(k) > 64 {
		return false
	}
	for _, r := range k {
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// scalar reads one value. The error never quotes it.
func scalar(s string, n int) (any, error) {
	switch s {
	case "[]":
		return []any{}, nil
	case "{}":
		return map[string]any{}, nil
	case "~", "null", "Null", "NULL", `""`, "''":
		return "", nil
	}
	if s[0] == '{' && s[len(s)-1] == '}' {
		return flowMap(s[1:len(s)-1], n)
	}
	switch s[0] {
	case '"':
		if len(s) < 2 || s[len(s)-1] != '"' {
			return nil, fmt.Errorf("line %d: text after a quoted value", n)
		}
		v, err := strconv.Unquote(s)
		if err != nil {
			return nil, fmt.Errorf("line %d: a \"double-quoted\" value has an escape HolyArrs cannot read; use 'single quotes'", n)
		}
		return v, nil
	case '\'':
		if len(s) < 2 || s[len(s)-1] != '\'' {
			return nil, fmt.Errorf("line %d: text after a quoted value", n)
		}
		inner := s[1 : len(s)-1]
		if strings.Contains(strings.ReplaceAll(inner, "''", ""), "'") {
			return nil, fmt.Errorf("line %d: a ' inside a 'single-quoted' value must be written twice", n)
		}
		return strings.ReplaceAll(inner, "''", "'"), nil
	case '&', '*', '!', '|', '>', '{', '[', '%', '@', '`':
		return nil, fmt.Errorf("line %d: put this value in quotes (it starts with a character YAML gives a meaning to)", n)
	}
	if strings.Contains(s, ": ") {
		return nil, fmt.Errorf("line %d: put this value in quotes (it holds \": \")", n)
	}
	return s, nil
}

// flowMap reads a one-line {key: value, key: value} of plain or quoted
// values.
func flowMap(s string, n int) (map[string]any, error) {
	m := map[string]any{}
	var parts []string
	var q byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case q == '"' && c == '\\':
			i++
		case q != 0 && c == q:
			q = 0
		case q == 0 && (c == '"' || c == '\''):
			q = c
		case q == 0 && c == ',':
			parts = append(parts, s[start:i])
			start = i + 1
		case q == 0 && (c == '{' || c == '[' || c == '}' || c == ']'):
			return nil, fmt.Errorf("line %d: only a one-level {key: value} is read on one line", n)
		}
	}
	parts = append(parts, s[start:])
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key, rest, ok := strings.Cut(p, ":")
		key = strings.TrimSpace(key)
		if !ok || !validKey(key) {
			return nil, fmt.Errorf("line %d: expected {key: value, ...}", n)
		}
		if _, dup := m[key]; dup {
			return nil, fmt.Errorf("line %d: key %s appears twice", n, key)
		}
		rest = strings.TrimSpace(rest)
		if rest == "" {
			m[key] = ""
			continue
		}
		v, err := scalar(rest, n)
		if err != nil {
			return nil, err
		}
		if _, isText := v.(string); !isText {
			return nil, fmt.Errorf("line %d: only a one-level {key: value} is read on one line", n)
		}
		m[key] = v
	}
	return m, nil
}

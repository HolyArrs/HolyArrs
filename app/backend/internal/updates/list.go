package updates

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Where the update list is read.
const (
	// DefaultURL is the metadata server's copy.
	DefaultURL = "https://api.holyarrs.com/v1/manifest.json"
	// PagesURL is the GitHub Pages copy, tried when the server fails.
	// Empty until the Pages copy is published.
	PagesURL = ""
	// SigSuffix is added to a list's address to get its signature.
	SigSuffix = ".minisig"
)

// ProjectPages is the only allowed home of changelog_url.
const ProjectPages = "https://github.com/HolyArrs/HolyArrs/"

// Limits.
const (
	MaxNotes  = 280
	MaxFuture = 10 * time.Minute
	Schema    = 1
)

// ErrOlder says the list's serial is lower than one already accepted.
var ErrOlder = errors.New("older update list ignored")

// List is the update list.
type List struct {
	Schema      int                `json:"schema"`
	Serial      int64              `json:"serial"`
	GeneratedAt time.Time          `json:"generated_at"`
	Channels    map[string]Channel `json:"channels"`
	APICompat   json.RawMessage    `json:"api_compat,omitempty"`
	Revoked     []string           `json:"revoked"`
}

// Channel is one channel's newest release (stable, beta or nightly).
type Channel struct {
	Version        string            `json:"version"`
	ReleasedAt     time.Time         `json:"released_at"`
	ChangelogURL   string            `json:"changelog_url"`
	NotesShort     string            `json:"notes_short"`
	NeedsAttention bool              `json:"needs_attention"`
	Migrates       bool              `json:"migrates"`
	OneWay         bool              `json:"one_way"`
	MinFrom        string            `json:"min_from"`
	AutoUpdateSafe bool              `json:"auto_update_safe"`
	Security       bool              `json:"security"`
	RolloutPercent *int              `json:"rollout_percent"`
	Hold           bool              `json:"hold"`
	Digests        map[string]string `json:"digests"`
}

// Channels the app knows.
var Channels = []string{"stable", "beta", "nightly"}

// Parse verifies the signature with keys and applies the rules: no future
// generated_at (over MaxFuture ahead of now), serial not below minSerial,
// changelog links only under ProjectPages, plain text only.
func Parse(data, sig []byte, keys []PublicKey, now time.Time, minSerial int64) (*List, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: no pinned key", ErrSignature)
	}
	if _, err := Verify(data, sig, keys); err != nil {
		return nil, err
	}
	var l List
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, errors.New("the update list is not readable")
	}
	if l.Schema != Schema {
		return nil, fmt.Errorf("the update list schema %d is not known", l.Schema)
	}
	if l.GeneratedAt.IsZero() || l.GeneratedAt.After(now.Add(MaxFuture)) {
		return nil, errors.New("the update list is dated in the future")
	}
	if l.Serial < minSerial {
		return nil, ErrOlder
	}
	for name, c := range l.Channels {
		if !slices.Contains(Channels, name) {
			delete(l.Channels, name)
			continue
		}
		c.Version = Plain(c.Version, 40)
		c.MinFrom = Plain(c.MinFrom, 40)
		c.NotesShort = Plain(c.NotesShort, MaxNotes)
		c.ChangelogURL = Changelog(c.ChangelogURL)
		for k, v := range c.Digests {
			c.Digests[k] = Plain(v, 200)
		}
		l.Channels[name] = c
	}
	for i, v := range l.Revoked {
		l.Revoked[i] = Plain(v, 40)
	}
	return &l, nil
}

// Plain makes text safe to show as plain text: HTML tags are removed,
// angle brackets and control characters dropped, spaces collapsed, and
// the result cut to max characters.
func Plain(s string, max int) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
		case r == '>':
			in = false
		case in:
		case r == utf8.RuneError, unicode.IsControl(r) && r != '\n':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if utf8.RuneCountInString(out) > max {
		out = string([]rune(out)[:max])
	}
	return out
}

// Changelog keeps a link only when it is plain https under ProjectPages;
// anything else becomes "".
func Changelog(link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" || u.Opaque != "" ||
		strings.Contains(u.Path, "..") || strings.Contains(u.Path, "//") || u.String() != link {
		return ""
	}
	if strings.HasPrefix(link, ProjectPages) {
		return link
	}
	return ""
}

// Offer is the release a channel offers this install, or false when the
// channel is missing, held, revoked or outside the rollout bucket (0-99).
func (l *List) Offer(channel string, bucket int) (Channel, bool) {
	c, ok := l.Channels[channel]
	if !ok || c.Version == "" || c.Hold || slices.Contains(l.Revoked, c.Version) {
		return Channel{}, false
	}
	pct := 100
	if c.RolloutPercent != nil {
		pct = *c.RolloutPercent
	}
	if bucket >= pct {
		return Channel{}, false
	}
	return c, true
}

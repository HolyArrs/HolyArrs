// Package logs keeps the app's log for the log viewer and the log
// download on This app: every line the program logs goes to a file in the
// config folder (logs/holyarrs.log, rotated to holyarrs.log.1, so at most
// twice MaxFile on disk), with secrets masked as the line is written and
// again whenever it is read. The program's own output (docker logs) goes
// through the same masking (Mask). The set of masked values only grows: a
// value that leaves the vault (changed or removed) stays masked in memory,
// and the files are rewritten with it masked before it is forgotten.
package logs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// File names in the logs folder.
const (
	FileName = "holyarrs.log"
	OldName  = "holyarrs.log.1"
	// MaxFile is the size at which the log file is rotated.
	MaxFile = 5 << 20
	// MaxLine caps one stored line.
	MaxLine = 4 << 10
	// minSecret is the shortest vault value masked wherever its text
	// appears.
	minSecret = 6
	// minShort is the shortest vault value masked at all: values of
	// minShort..minSecret-1 characters are masked only as a whole token (not
	// inside a longer word) and never when they are a common word.
	minShort = 4
	// maxPartial caps a write held while waiting for its line end.
	maxPartial = 4 * MaxLine
)

// Hidden replaces every masked value.
const Hidden = "[hidden]"

// Levels, from least to most serious.
const (
	Info    = "info"
	Warning = "warning"
	Error   = "error"
)

// maxRead caps how much of the files one read looks at.
const maxRead = 2 * MaxFile

// shortStop are common words never masked as short secrets: a short vault
// value such as "admin" would otherwise hide that word in every line.
var shortStop = map[string]bool{
	"true": true, "false": true, "null": true, "none": true, "admin": true, "user": true, "guest": true,
	"test": true, "info": true, "error": true, "warn": true, "debug": true, "http": true, "https": true,
	"local": true, "home": true, "root": true, "movie": true, "show": true, "music": true, "pass": true,
	"login": true, "open": true, "path": true, "host": true, "port": true, "file": true, "data": true,
}

var (
	// scheme://user:password@host
	urlCreds = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s@:]+:[^/\s@]+@`)
	// ?apikey=..., &token=... in links
	queryKey = regexp.MustCompile(`(?i)([?&;](?:api_?key|apikey|key|token|access_token|refresh_token|passkey|password|passwd|pass|secret|auth|sig|signature|session)=)[^&\s"'<>]+`)
	// key: value or key=value for secret names (headers, settings, JSON); a
	// quoted value is masked up to its closing quote
	pairKey = regexp.MustCompile(`(?i)\b((?:x-)?api[-_ ]?key|token|access[-_ ]?token|password|passwd|secret|client[-_ ]?secret|cookie|set-cookie|authorization|passkey|setup[-_ ]?code)("?\s*[:=]\s*)(?:"([^"]*)"|'([^']*)'|([^\s"',;&]+))`)
	// Authorization: Bearer xyz, Basic xyz
	bearer = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)
	// HolyArrs API keys
	hak = regexp.MustCompile(`\bhak_[A-Za-z0-9_-]+`)
	// long token-like runs: 32+ letters, digits, - or _ holding both letters and digits
	longTok = regexp.MustCompile(`\b[A-Za-z0-9_-]{32,}\b`)
)

var (
	errorWords   = regexp.MustCompile(`(?i)\b(panic|panicked|crash|crashed|error|errors|failed|fatal)\b`)
	warningWords = regexp.MustCompile(`(?i)\b(warn|warning|could not|cannot|not answering|refused|retry|retrying|timed out|stalled|rolled back|left behind|did not)\b`)
)

// Line is one log line.
type Line struct {
	At    time.Time `json:"at"`
	Level string    `json:"level"`
	Text  string    `json:"text"`
}

// Log is the app log on disk.
type Log struct {
	dir     string
	maxFile int64
	// mu guards the write path (f, size, partial).
	mu      sync.Mutex
	f       *os.File
	size    int64
	partial []byte
	// files is held for writing while a file is renamed (rotation,
	// rewrite) and for reading while the viewer reads, so a read never
	// blocks an ordinary write.
	files sync.RWMutex
	// maskMu guards adding values; known is the grow-only mask set.
	maskMu    sync.Mutex
	known     atomic.Pointer[masks]
	vaultLast map[string]bool
	// cache holds the masked lines of the last read.
	cacheMu sync.Mutex
	cacheAt string
	cache   []string
}

// masks is the set of masked values: long ones replaced wherever they
// appear (longest first), short ones only as a whole token.
type masks struct {
	all   map[string]bool
	long  []string
	short []string
	gen   int
}

type maskWriter struct {
	l *Log
	w io.Writer
}

// Filter picks lines for the viewer.
type Filter struct {
	// Level is the least serious level shown ("" shows every line).
	Level string
	// Text must appear in the line (any case).
	Text     string
	From, To time.Time
	Offset   int
	Limit    int
}

// Vault is what Sync reads: the current secret values, and the old values
// of changed or removed ones still waiting to be masked in the files.
type Vault interface {
	Values(ctx context.Context) ([]string, error)
	RetiredValues(ctx context.Context) ([]string, int64, error)
	ForgetRetired(ctx context.Context, upTo int64) error
}

// Open opens (or makes) the logs folder dir.
func Open(dir string) (*Log, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	l := &Log{dir: dir, maxFile: MaxFile}
	if err := l.openFile(); err != nil {
		return nil, err
	}
	return l, nil
}

// Dir is the logs folder.
func (l *Log) Dir() string { return l.dir }

// Close closes the file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// SetSecrets is told the vault's current values. They join the mask set,
// which never shrinks; a value told last time and missing now has left the
// vault, so the files are rewritten with it masked (Retire).
func (l *Log) SetSecrets(vals []string) {
	cur := map[string]bool{}
	for _, v := range vals {
		for _, s := range leaves(v) {
			if s != "" {
				cur[s] = true
			}
		}
	}
	l.add(vals)
	l.maskMu.Lock()
	var gone []string
	for v := range l.vaultLast {
		if !cur[v] {
			gone = append(gone, v)
		}
	}
	l.vaultLast = cur
	l.maskMu.Unlock()
	if len(gone) > 0 {
		_ = l.Retire(gone)
	}
}

// AddSecret masks one more value from the next line on. The vault calls it
// before a save returns, so a new secret never reaches the file unmasked.
func (l *Log) AddSecret(v string) { l.add([]string{v}) }

// Retire masks values that leave the vault and rewrites the log files so
// none of them stays on disk in the clear. Call it before the value is
// forgotten.
func (l *Log) Retire(vals []string) error {
	forms := l.add(vals)
	if l.dir == "" || len(forms) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.files.Lock()
	defer l.files.Unlock()
	for _, name := range []string{OldName, FileName} {
		path := filepath.Join(l.dir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		hit := false
		for _, f := range forms {
			if bytes.Contains(b, []byte(f)) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		var out bytes.Buffer
		for _, ln := range strings.SplitAfter(string(b), "\n") {
			text, nl := strings.CutSuffix(ln, "\n")
			out.WriteString(l.Redact(text))
			if nl {
				out.WriteByte('\n')
			}
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, out.Bytes(), 0o600); err != nil {
			return err
		}
		cur := name == FileName && l.f != nil
		if cur {
			_ = l.f.Close()
			l.f = nil
		}
		err = os.Rename(tmp, path)
		if cur {
			if oerr := l.openFile(); oerr != nil && err == nil {
				err = oerr
			}
		}
		if err != nil {
			_ = os.Remove(tmp)
			return err
		}
	}
	return nil
}

// Write takes what the standard logger writes: one entry per call, ending
// in a newline. Control characters inside an entry (a newline in a logged
// value included) are shown escaped, so a value cannot forge a line or keep
// terminal colours; the entry is masked, then appended to the file. A write
// without a line end is held until one comes. It never fails the caller.
func (l *Log) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = append(l.partial, p...)
	if n := len(l.partial); n > 0 && l.partial[n-1] == '\n' {
		l.writeLine(string(l.partial[:n-1]))
		l.partial = nil
	} else if n > maxPartial {
		l.writeLine(string(l.partial))
		l.partial = nil
	}
	return len(p), nil
}

// Clean shows control characters as escapes: carriage return and newline
// as \r and \n, a tab as a space, every other C0 character, ESC and DEL as
// \xNN.
func Clean(s string) string {
	if !strings.ContainsFunc(s, isCtl) {
		return s
	}
	const hex = "0123456789abcdef"
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteByte(' ')
		case isCtl(r):
			b.WriteString(`\x`)
			b.WriteByte(hex[r>>4])
			b.WriteByte(hex[r&15])
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Mask wraps the program's own output (docker logs) so it gets the same
// masking and escaping as the file.
func (l *Log) Mask(w io.Writer) io.Writer { return maskWriter{l, w} }

func (m maskWriter) Write(p []byte) (int, error) {
	s, nl := strings.CutSuffix(string(p), "\n")
	out := m.l.redact(Clean(s))
	if nl {
		out += "\n"
	}
	if _, err := io.WriteString(m.w, out); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Redact masks secrets in one line: the known vault values (and their
// encoded forms), credentials in links, secret query parameters, key=value
// and key: value pairs with a secret name, bearer tokens, HolyArrs API keys,
// and long token-like runs.
func (l *Log) Redact(s string) string { return l.redact(s) }

// RedactPatterns masks what the patterns of Redact catch, without the
// vault values.
func RedactPatterns(s string) string { return redactPatterns(s) }

// LevelOf guesses a line's level from its words.
func LevelOf(text string) string {
	switch {
	case errorWords.MatchString(text):
		return Error
	case warningWords.MatchString(text):
		return Warning
	}
	return Info
}

// Read answers the lines that match f, newest first, and how many match.
// Every line is masked again as it is read.
func (l *Log) Read(f Filter) ([]Line, int) {
	all := l.lines()
	text := strings.ToLower(strings.TrimSpace(f.Text))
	min := rank(f.Level)
	var match []Line
	for i := len(all) - 1; i >= 0; i-- {
		ln := parse(all[i])
		if rank(ln.Level) < min {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(ln.Text), text) {
			continue
		}
		if !f.From.IsZero() && (ln.At.IsZero() || ln.At.Before(f.From)) {
			continue
		}
		if !f.To.IsZero() && (ln.At.IsZero() || !ln.At.Before(f.To)) {
			continue
		}
		match = append(match, ln)
	}
	total := len(match)
	if f.Offset >= total {
		return []Line{}, total
	}
	match = match[f.Offset:]
	if f.Limit > 0 && len(match) > f.Limit {
		match = match[:f.Limit]
	}
	return match, total
}

// Tail is the newest log text, at most max bytes of whole lines, masked
// again as it is read: what the log download hands out.
func (l *Log) Tail(max int) []byte {
	all := l.lines()
	var keep []string
	n := 0
	for i := len(all) - 1; i >= 0; i-- {
		s := all[i]
		if n+len(s)+1 > max {
			break
		}
		n += len(s) + 1
		keep = append(keep, s)
	}
	var b bytes.Buffer
	for i := len(keep) - 1; i >= 0; i-- {
		b.WriteString(keep[i])
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// Sync brings the mask set up to date with the vault: current values are
// added, retired ones are masked in the files and only then forgotten by
// the vault.
func (l *Log) Sync(ctx context.Context, v Vault) error {
	vals, err := v.Values(ctx)
	if err != nil {
		return err
	}
	l.SetSecrets(vals)
	old, upTo, err := v.RetiredValues(ctx)
	if err != nil || upTo == 0 {
		return err
	}
	if err := l.Retire(old); err != nil {
		return err
	}
	return v.ForgetRetired(ctx, upTo)
}

func (l *Log) openFile() error {
	f, err := os.OpenFile(filepath.Join(l.dir, FileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	l.f, l.size = f, st.Size()
	return nil
}

// add puts values (their JSON leaves and encoded forms too) in the mask set
// and answers the forms they give.
func (l *Log) add(vals []string) []string {
	l.maskMu.Lock()
	defer l.maskMu.Unlock()
	old := l.known.Load()
	var forms []string
	var fresh []string
	for _, v := range vals {
		for _, s := range leaves(v) {
			for _, f := range variants(s) {
				forms = append(forms, f)
				if old == nil || !old.all[f] {
					fresh = append(fresh, f)
				}
			}
		}
	}
	if len(fresh) == 0 {
		return forms
	}
	next := &masks{all: map[string]bool{}}
	if old != nil {
		for k := range old.all {
			next.all[k] = true
		}
		next.gen = old.gen
	}
	for _, f := range fresh {
		next.all[f] = true
	}
	for k := range next.all {
		if len(k) >= minSecret {
			next.long = append(next.long, k)
		} else {
			next.short = append(next.short, k)
		}
	}
	// Longest first, so a value inside another is not left half shown.
	sort.Slice(next.long, func(i, j int) bool { return len(next.long[i]) > len(next.long[j]) })
	next.gen++
	l.known.Store(next)
	return forms
}

// variants is a value and the forms it takes in links and headers: query
// and path escaped, and base64 (standard and URL, padded and not). A value
// shorter than minShort, or a short common word, gives nothing.
func variants(s string) []string {
	switch {
	case len(s) < minShort:
		return nil
	case len(s) < minSecret:
		if shortStop[strings.ToLower(s)] {
			return nil
		}
		return []string{s}
	}
	out := []string{s}
	seen := map[string]bool{s: true}
	b := []byte(s)
	for _, f := range []string{url.QueryEscape(s), url.PathEscape(s),
		base64.StdEncoding.EncodeToString(b), base64.URLEncoding.EncodeToString(b),
		base64.RawStdEncoding.EncodeToString(b), base64.RawURLEncoding.EncodeToString(b)} {
		if !seen[f] && len(f) >= minSecret {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

// leaves is the value itself and, for a JSON value, every string in it.
func leaves(v string) []string {
	out := []string{strings.TrimSpace(v)}
	var x any
	if t := strings.TrimSpace(v); (strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")) && json.Unmarshal([]byte(t), &x) == nil {
		var walk func(any)
		walk = func(y any) {
			switch z := y.(type) {
			case string:
				out = append(out, z)
			case map[string]any:
				for _, w := range z {
					walk(w)
				}
			case []any:
				for _, w := range z {
					walk(w)
				}
			}
		}
		walk(x)
	}
	return out
}

func isCtl(r rune) bool { return r < 0x20 || r == 0x7f }

func (l *Log) writeLine(s string) {
	if l.f == nil {
		return
	}
	s = l.Redact(Clean(s))
	if len(s) > MaxLine {
		s = s[:MaxLine] + " [cut]"
	}
	b := []byte(s + "\n")
	if l.size+int64(len(b)) > l.maxFile && l.size > 0 {
		l.files.Lock()
		_ = l.f.Close()
		_ = os.Remove(filepath.Join(l.dir, OldName))
		_ = os.Rename(filepath.Join(l.dir, FileName), filepath.Join(l.dir, OldName))
		err := l.openFile()
		l.files.Unlock()
		if err != nil {
			l.f = nil
			return
		}
	}
	n, _ := l.f.Write(b)
	l.size += int64(n)
}

func (l *Log) redact(s string) string {
	if p := l.known.Load(); p != nil {
		for _, v := range p.long {
			if strings.Contains(s, v) {
				s = strings.ReplaceAll(s, v, Hidden)
			}
		}
		for _, v := range p.short {
			s = replaceToken(s, v)
		}
	}
	return redactPatterns(s)
}

// replaceToken masks v where it stands alone: not next to a letter, digit,
// underscore or non-ASCII byte.
func replaceToken(s, v string) string {
	if !strings.Contains(s, v) {
		return s
	}
	var b strings.Builder
	rest := s
	before := byte(0)
	for {
		i := strings.Index(rest, v)
		if i < 0 {
			break
		}
		end := i + len(v)
		prev := before
		if i > 0 {
			prev = rest[i-1]
		}
		if !isWord(prev) && (end == len(rest) || !isWord(rest[end])) {
			b.WriteString(rest[:i])
			b.WriteString(Hidden)
		} else {
			b.WriteString(rest[:end])
		}
		before = rest[end-1]
		rest = rest[end:]
	}
	b.WriteString(rest)
	return b.String()
}

func isWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c >= 0x80
}

func redactPatterns(s string) string {
	s = urlCreds.ReplaceAllString(s, "${1}"+Hidden+"@")
	s = queryKey.ReplaceAllString(s, "${1}"+Hidden)
	s = bearer.ReplaceAllString(s, "${1} "+Hidden)
	s = pairKey.ReplaceAllStringFunc(s, func(m string) string {
		sm := pairKey.FindStringSubmatch(m)
		switch {
		case sm[5] == "" && strings.HasSuffix(m, `"`):
			return sm[1] + sm[2] + `"` + Hidden + `"`
		case sm[5] == "" && strings.HasSuffix(m, `'`):
			return sm[1] + sm[2] + `'` + Hidden + `'`
		case sm[5] == Hidden:
			return m
		}
		return sm[1] + sm[2] + Hidden
	})
	s = hak.ReplaceAllString(s, "hak_"+Hidden)
	s = longTok.ReplaceAllStringFunc(s, func(m string) string {
		if strings.ContainsAny(m, "0123456789") && strings.ContainsFunc(m, func(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }) {
			return Hidden
		}
		return m
	})
	return s
}

func rank(level string) int {
	switch level {
	case Error:
		return 2
	case Warning:
		return 1
	}
	return 0
}

// parse reads the standard logger's "2006/01/02 15:04:05 " prefix (local
// time); a line without it has a zero time.
func parse(raw string) Line {
	ln := Line{Text: raw}
	if len(raw) >= 20 && raw[4] == '/' && raw[19] == ' ' {
		if t, err := time.ParseInLocation("2006/01/02 15:04:05", raw[:19], time.Local); err == nil {
			ln.At, ln.Text = t.UTC(), raw[20:]
		}
	}
	ln.Level = LevelOf(ln.Text)
	return ln
}

// lines reads the old and the current file, oldest line first, each line
// masked and escaped. The write lock is not held while reading: the file
// sizes are taken first and only that much is read. The masked lines are
// kept until a file or the mask set changes.
func (l *Log) lines() []string {
	l.files.RLock()
	defer l.files.RUnlock()
	type part struct {
		name string
		size int64
	}
	var parts []part
	key := ""
	if p := l.known.Load(); p != nil {
		key = strconv.Itoa(p.gen)
	}
	for _, name := range []string{OldName, FileName} {
		st, err := os.Stat(filepath.Join(l.dir, name))
		if err != nil {
			continue
		}
		parts = append(parts, part{name, st.Size()})
		key += "|" + name + ":" + strconv.FormatInt(st.Size(), 10) + ":" + strconv.FormatInt(st.ModTime().UnixNano(), 10)
	}
	l.cacheMu.Lock()
	if l.cache != nil && l.cacheAt == key {
		out := l.cache
		l.cacheMu.Unlock()
		return out
	}
	l.cacheMu.Unlock()
	out := []string{}
	budget := int64(maxRead)
	for _, pt := range parts {
		f, err := os.Open(filepath.Join(l.dir, pt.name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(io.LimitReader(f, pt.size))
		sc.Buffer(make([]byte, 64<<10), MaxLine*4)
		for sc.Scan() && budget > 0 {
			t := sc.Text()
			budget -= int64(len(t) + 1)
			out = append(out, l.Redact(Clean(t)))
		}
		_ = f.Close()
	}
	l.cacheMu.Lock()
	l.cache, l.cacheAt = out, key
	l.cacheMu.Unlock()
	return out
}

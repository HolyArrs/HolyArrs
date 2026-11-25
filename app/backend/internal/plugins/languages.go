package plugins

import (
	"bytes"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// LanguageCodes are the languages that come as signed packs. English is
// built into the app and never is one.
var LanguageCodes = []string{"es", "pt-br", "pt-pt", "fr", "de", "it", "nl", "sv", "nb", "da", "fi", "ru", "uk", "pl", "cs", "hu", "ro", "el",
	"sk", "bg", "sr", "hr", "ca", "lt", "lv", "et", "is", "zh-cn", "zh-tw", "ja", "ko", "hi", "bn", "ta", "id", "ms", "fil", "vi", "th", "tr", "he", "fa", "ar"}

// Size caps for a pack: packed as sent, and unpacked.
const (
	maxPackSize  = 4 << 20
	maxPackPlain = 32 << 20
)

// ErrPack is a pack file whose contents are not the language and app
// version its signed list entry names.
var ErrPack = errors.New("language pack does not match its list entry")

// Languages downloads, checks and keeps the language packs. A pack is text
// only: it is never run, so none of the plugin sandbox is involved. It uses
// the Engine's list address, pinned keys and app version.
type Languages struct {
	// Dir holds one folder per language with <version>.json.bz2 and the
	// state file.
	Dir    string
	Engine *Engine
	// Upgraded says whether this install existed before packs did; it is
	// read once, when the state file is first written, and kept there.
	Upgraded func() bool

	mu    sync.Mutex
	st    *langState
	busy  map[string]chan struct{}
	fails map[string]langFail
	// sizes are the listed pack sizes for this app version, for languages
	// not installed yet; sizesAt is when they were last fetched.
	sizes   map[string]int64
	sizesAt time.Time
}

type langFail struct {
	reason string
	at     time.Time
}

type langState struct {
	ListCounter  uint64 `json:"list_counter"`
	BlockCounter uint64 `json:"block_counter"`
	// App is the app version the packs were last brought up to date for.
	App string `json:"app"`
	// AutoBrowser: browsers' own languages install without asking (an
	// install that had every language built in before).
	AutoBrowser bool                     `json:"auto_browser"`
	Packs       map[string]langInstalled `json:"packs"`
}

type langInstalled struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
}

// LanguageRow is one language as the API shows it.
type LanguageRow struct {
	Code string `json:"code"`
	// State is built_in, installed, installing, failed or available.
	State   string `json:"state"`
	Version string `json:"version,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Error   string `json:"error,omitempty"`
}

const langStateFile = "state.json"

// load reads the state once; the caller holds mu.
func (l *Languages) load() *langState {
	if l.st != nil {
		return l.st
	}
	l.busy, l.fails = map[string]chan struct{}{}, map[string]langFail{}
	st := &langState{Packs: map[string]langInstalled{}}
	path := filepath.Join(l.Dir, langStateFile)
	raw, err := os.ReadFile(path)
	switch {
	case err == nil && json.Unmarshal(raw, st) == nil:
		if st.Packs == nil {
			st.Packs = map[string]langInstalled{}
		}
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		// Unreadable for now (a busy disk): work from the packs on disk and
		// leave the file alone, so the next start reads it again.
		st = &langState{Packs: l.packsOnDisk(), AutoBrowser: l.Upgraded != nil && l.Upgraded()}
		log.Printf("languages: reading %s: %v", langStateFile, err)
	default:
		// Missing or damaged: a damaged copy is kept aside, and the packs
		// already on disk are taken back instead of being forgotten.
		st = &langState{Packs: map[string]langInstalled{}, AutoBrowser: l.Upgraded != nil && l.Upgraded()}
		if err == nil {
			os.WriteFile(path+".bad", raw, 0o600)
			st.Packs = l.packsOnDisk()
			log.Printf("languages: %s was damaged; read %d packs back from disk", langStateFile, len(st.Packs))
		}
		if err := l.saveLocked(st); err != nil {
			log.Printf("languages: %v", err)
		}
	}
	l.st = st
	return st
}

// packsOnDisk reads back the packs kept in their folders. Only a file
// that passes the same checks as an install is taken.
func (l *Languages) packsOnDisk() map[string]langInstalled {
	out := map[string]langInstalled{}
	dirs, _ := os.ReadDir(l.Dir)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(l.Dir, d.Name()))
		for _, f := range files {
			version, ok := strings.CutSuffix(f.Name(), ".json.bz2")
			if !ok {
				continue
			}
			packed, err := os.ReadFile(filepath.Join(l.Dir, d.Name(), f.Name()))
			if err != nil || checkPack(packed, d.Name(), version) != nil {
				continue
			}
			s := sha256.Sum256(packed)
			out[d.Name()] = langInstalled{Version: version, SHA256: hex.EncodeToString(s[:]), Size: int64(len(packed))}
		}
	}
	return out
}

func (l *Languages) saveLocked(st *langState) error {
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return fmt.Errorf("languages folder: %w", err)
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return writeFileSynced(filepath.Join(l.Dir, langStateFile), b)
}

func knownLanguage(code string) bool { return slices.Contains(LanguageCodes, code) }

func (l *Languages) appVersion() string {
	if l.Engine == nil {
		return ""
	}
	return l.Engine.AppVersion
}

// devBuild has no published packs of its own; it takes the newest.
func devBuild(v string) bool { return v == "" || v == "dev" || strings.HasPrefix(v, "0.0.0") }

// AutoBrowser reports whether a browser's own language installs without
// asking first.
func (l *Languages) AutoBrowser() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.load().AutoBrowser
}

// NewVersion reports whether the packs have not yet been brought up to
// date for this app version (the first start after an update).
func (l *Languages) NewVersion() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.load()
	return st.App != l.appVersion()
}

// Current reports whether code's pack is installed for this app version.
func (l *Languages) Current(code string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	p, ok := l.load().Packs[code]
	return ok && p.Version == l.appVersion()
}

// List shows English, then every pack language with its state.
func (l *Languages) List() []LanguageRow {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.load()
	rows := []LanguageRow{{Code: "en", State: "built_in"}}
	for _, c := range LanguageCodes {
		r := LanguageRow{Code: c, State: "available"}
		if p, ok := st.Packs[c]; ok {
			r.State, r.Version, r.Size = "installed", p.Version, p.Size
		} else {
			r.Size = l.sizes[c]
		}
		if _, ok := l.busy[c]; ok {
			r.State = "installing"
		} else if f, ok := l.fails[c]; ok && r.State != "installed" {
			r.State, r.Error = "failed", f.reason
		}
		rows = append(rows, r)
	}
	return rows
}

// Install fetches the pack for this app version, checks it against the
// signed lists, and keeps it. Two calls for one language share one fetch.
func (l *Languages) Install(ctx context.Context, code string) error {
	if !knownLanguage(code) {
		return fmt.Errorf("%w: %s", ErrNotListed, code)
	}
	l.mu.Lock()
	l.load()
	if ch, ok := l.busy[code]; ok {
		l.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
		l.mu.Lock()
		f, failed := l.fails[code]
		_, ok := l.st.Packs[code]
		l.mu.Unlock()
		if failed && !ok {
			return errors.New(f.reason)
		}
		return nil
	}
	ch := make(chan struct{})
	l.busy[code] = ch
	l.mu.Unlock()
	err := l.install(ctx, code)
	l.mu.Lock()
	delete(l.busy, code)
	if err != nil {
		l.fails[code] = langFail{reason: shortReason(err), at: time.Now()}
	} else {
		delete(l.fails, code)
	}
	l.mu.Unlock()
	close(ch)
	if err != nil {
		log.Printf("languages: %s pack for %s: %v", code, l.appVersion(), err)
	}
	return err
}

func (l *Languages) install(ctx context.Context, code string) error {
	e := l.Engine
	if e == nil || len(e.Keys) == 0 {
		return ErrNoKey
	}
	l.mu.Lock()
	last, lastBlock := l.st.ListCounter, l.st.BlockCounter
	l.mu.Unlock()
	list, block, err := l.fetchLists(ctx, last, lastBlock)
	if err != nil {
		return err
	}
	l.noteSizes(list)
	en, ok := pickPack(list.Entries, code, l.appVersion())
	if !ok {
		return fmt.Errorf("%w: no %s pack for app %s", ErrNotListed, code, l.appVersion())
	}
	l.mu.Lock()
	cur, had := l.st.Packs[code]
	l.mu.Unlock()
	if had && blockedPack(block.Entries, Entry{ID: en.ID, Version: cur.Version, SHA256: cur.SHA256}) {
		log.Printf("languages: %s pack named by the blocklist, dropped", code)
		l.drop(code)
	}
	if blockedPack(block.Entries, en) {
		return fmt.Errorf("%w: %s %s", ErrBlocked, en.ID, en.Version)
	}
	if !safeVersion(en.Version) {
		return fmt.Errorf("%w: version %q", ErrPack, en.Version)
	}
	rel := en.FileURL
	if rel == "" {
		rel = "languages/" + code + "/" + en.Version + ".json.bz2"
	}
	body, err := e.fetchBody(ctx, rel, maxPackSize)
	if err != nil {
		return err
	}
	tmp, err := spoolPack(body, l.Dir, en.SHA256)
	body.Close()
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	packed, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	if err := checkPack(packed, code, en.Version); err != nil {
		return err
	}
	dir := filepath.Join(l.Dir, code)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := en.Version + ".json.bz2"
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.st
	st.Packs[code] = langInstalled{Version: en.Version, SHA256: en.SHA256, Size: int64(len(packed))}
	st.ListCounter = max(st.ListCounter, list.Counter)
	st.BlockCounter = max(st.BlockCounter, block.Counter)
	if err := l.saveLocked(st); err != nil {
		return err
	}
	// Older versions go once the new one is kept.
	if old, err := os.ReadDir(dir); err == nil {
		for _, f := range old {
			if f.Name() != name {
				os.Remove(filepath.Join(dir, f.Name()))
			}
		}
	}
	return nil
}

// noteSizes keeps the listed size of each language's pack for this app
// version, so the list can show it before install.
func (l *Languages) noteSizes(list *List) {
	m := map[string]int64{}
	for _, c := range LanguageCodes {
		if en, ok := pickPack(list.Entries, c, l.appVersion()); ok && en.Size > 0 {
			m[c] = en.Size
		}
	}
	l.mu.Lock()
	l.sizes, l.sizesAt = m, time.Now()
	l.mu.Unlock()
}

// LoadSizes fetches the signed lists for the pack sizes when they were not
// fetched in the last hour. Errors only leave the sizes out.
func (l *Languages) LoadSizes(ctx context.Context) {
	e := l.Engine
	if e == nil || len(e.Keys) == 0 {
		return
	}
	l.mu.Lock()
	st := l.load()
	fresh := !l.sizesAt.IsZero() && time.Since(l.sizesAt) < time.Hour
	last, lastBlock := st.ListCounter, st.BlockCounter
	l.mu.Unlock()
	if fresh {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	list, _, err := l.fetchLists(ctx, last, lastBlock)
	if err != nil {
		return
	}
	l.noteSizes(list)
}

// pickPack is the entry for this app version; a development build takes
// the newest one (its pages check the pack fits them before using it).
func pickPack(entries []Entry, code, app string) (Entry, bool) {
	var best Entry
	found := false
	for _, en := range entries {
		if en.ID != "lang-"+code {
			continue
		}
		if en.Version == app {
			return en, true
		}
		if devBuild(app) && (!found || newerVersion(en.Version, best.Version)) {
			best, found = en, true
		}
	}
	return best, found
}

// spoolPack keeps r in a temp file below dir when its hash is want.
func spoolPack(r io.Reader, dir, want string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("languages folder: %w", err)
	}
	f, err := os.CreateTemp(dir, "pack-*.part")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, maxPackSize+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	fail := func(e error) (string, error) { os.Remove(f.Name()); return "", e }
	if err != nil {
		return fail(fmt.Errorf("%w: %v", ErrFetch, err))
	}
	if n > maxPackSize {
		return fail(fmt.Errorf("%w: language pack over %d bytes", ErrFileTooLarge, maxPackSize))
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fail(fmt.Errorf("%w: language pack %s, list %s", ErrHashMismatch, got, want))
	}
	return f.Name(), nil
}

// checkPack unpacks a pack and checks it is the language and app version
// its list entry names.
func checkPack(packed []byte, code, version string) error {
	plain, err := unpackPack(packed)
	if err != nil {
		return err
	}
	var head struct {
		Lang   string            `json:"lang"`
		App    string            `json:"app"`
		Fit    string            `json:"fit"`
		Values []json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(plain, &head); err != nil {
		return fmt.Errorf("%w: %v", ErrPack, err)
	}
	if head.Lang != code || head.App != version || head.Fit == "" || len(head.Values) == 0 {
		return fmt.Errorf("%w: file is %s for %s", ErrPack, head.Lang, head.App)
	}
	return nil
}

func unpackPack(packed []byte) ([]byte, error) {
	plain, err := io.ReadAll(io.LimitReader(bzip2.NewReader(bytes.NewReader(packed)), maxPackPlain+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPack, err)
	}
	if len(plain) > maxPackPlain {
		return nil, fmt.Errorf("%w: unpacks to over %d bytes", ErrPack, maxPackPlain)
	}
	return plain, nil
}

// Pack is a language's pack, unpacked, with its hash. The stored file is
// checked against the hash from the signed list each time. A file that
// reads but no longer matches the entry it was read for is dropped; a
// read error only fails this request, so a busy disk never drops a pack.
func (l *Languages) Pack(code string) ([]byte, string, error) {
	for range 3 {
		l.mu.Lock()
		p, ok := l.load().Packs[code]
		l.mu.Unlock()
		if !ok {
			return nil, "", ErrNotInstalled
		}
		packed, err := os.ReadFile(filepath.Join(l.Dir, code, p.Version+".json.bz2"))
		if err != nil {
			return nil, "", fmt.Errorf("reading the %s pack: %w", code, err)
		}
		s := sha256.Sum256(packed)
		if hex.EncodeToString(s[:]) == p.SHA256 {
			plain, err := unpackPack(packed)
			if err != nil {
				return nil, "", err
			}
			return plain, p.SHA256, nil
		}
		// An install may have replaced the pack meanwhile: drop only when
		// the entry is still the one the file was read for.
		l.mu.Lock()
		cur, still := l.load().Packs[code]
		if still && cur == p {
			l.dropLocked(code)
			l.mu.Unlock()
			log.Printf("languages: %s pack dropped: %v", code, ErrHashMismatch)
			return nil, "", ErrNotInstalled
		}
		l.mu.Unlock()
	}
	return nil, "", fmt.Errorf("%s pack kept changing while read", code)
}

func (l *Languages) drop(code string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dropLocked(code)
}

// dropLocked removes a pack; the caller holds mu.
func (l *Languages) dropLocked(code string) {
	delete(l.load().Packs, code)
	if err := l.saveLocked(l.st); err != nil {
		log.Printf("languages: %v", err)
	}
	os.RemoveAll(filepath.Join(l.Dir, code))
}

// Remove deletes a language's pack.
func (l *Languages) Remove(code string) error {
	if !knownLanguage(code) {
		return fmt.Errorf("%w: %s", ErrNotListed, code)
	}
	l.drop(code)
	return nil
}

// Ensure brings every installed pack and every language in want up to date
// for this app version. Nothing is fetched when all are current.
func (l *Languages) Ensure(ctx context.Context, want []string) error {
	l.mu.Lock()
	st := l.load()
	app := l.appVersion()
	need := []string{}
	for c, p := range st.Packs {
		if p.Version != app && !devBuild(app) {
			need = append(need, c)
		}
	}
	for _, c := range want {
		if _, ok := st.Packs[c]; !ok && knownLanguage(c) && !slices.Contains(need, c) {
			need = append(need, c)
		}
	}
	l.mu.Unlock()
	slices.Sort(need)
	var first error
	for _, c := range need {
		if err := l.Install(ctx, c); err != nil && first == nil {
			first = err
		}
	}
	if first != nil {
		return first
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.st.App != app {
		l.st.App = app
		return l.saveLocked(l.st)
	}
	return nil
}

// Keep runs Ensure until it succeeds, waiting 1 minute after the first
// failure and up to 6 hours between later ones, then once a day for packs
// a new language or a replaced file needs. Every round first rechecks the
// installed packs, so a withdrawn pack goes even while Ensure keeps failing.
func (l *Languages) Keep(ctx context.Context, want func() []string) {
	wait := time.Minute
	for {
		next := 24 * time.Hour
		if err := l.keepCycle(ctx, want()); err != nil {
			next = wait
			wait = min(wait*2, 6*time.Hour)
		} else {
			wait = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(next):
		}
	}
}

// keepCycle is one round of Keep: recheck, then Ensure. It returns
// Ensure's error; a recheck error is only logged.
func (l *Languages) keepCycle(ctx context.Context, want []string) error {
	if err := l.Recheck(ctx); err != nil {
		log.Printf("languages: recheck: %v", err)
	}
	return l.Ensure(ctx, want)
}

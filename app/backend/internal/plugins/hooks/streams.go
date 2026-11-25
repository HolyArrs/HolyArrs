package hooks

import (
	"crypto/rand"
	"sync"
	"time"
)

// streamLife is how long a media stream link works.
const streamLife = 10 * time.Minute

type stream struct {
	plugin, ref string // ref: a file id for media, a page id for pages
	until       time.Time
}

// streams are the signed, short-lived media links handed to plugins. A link
// names a file by a random token, never by its path.
type streams struct {
	mu  sync.Mutex
	m   map[string]stream
	now func() time.Time
}

func newStreams() *streams { return &streams{m: map[string]stream{}, now: time.Now} }

func (s *streams) issue(plugin, ref string) (string, time.Time) {
	tok := "hms_" + rand.Text()
	until := s.now().Add(streamLife)
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.m {
		if s.now().After(v.until) {
			delete(s.m, k)
		}
	}
	s.m[tok] = stream{plugin: plugin, ref: ref, until: until}
	return tok, until
}

// open is the plugin and reference a live token names.
func (s *streams) open(tok string) (plugin, ref string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[tok]
	if !ok || s.now().After(v.until) {
		delete(s.m, tok)
		return "", "", false
	}
	return v.plugin, v.ref, true
}

// drop ends every link of plugin.
func (s *streams) drop(plugin string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.m {
		if v.plugin == plugin {
			delete(s.m, k)
		}
	}
}

// owner is the plugin a live token belongs to.
func (s *streams) owner(tok string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[tok]
	if !ok || s.now().After(v.until) {
		return "", false
	}
	return v.plugin, true
}

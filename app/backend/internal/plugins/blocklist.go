package plugins

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Run checks the blocklist every BlockEvery until ctx ends.
func (e *Engine) Run(ctx context.Context) {
	every := e.BlockEvery
	if every <= 0 {
		every = DefaultBlockEvery
	}
	if err := e.CheckBlocklist(ctx); err != nil {
		log.Printf("plugins: blocklist check: %v", err)
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := e.CheckBlocklist(ctx); err != nil {
				log.Printf("plugins: blocklist check: %v", err)
			}
		}
	}
}

// CheckBlocklist fetches both lists and stops and disables every enabled
// plugin they no longer allow. A fetch or signature failure changes nothing.
func (e *Engine) CheckBlocklist(ctx context.Context) error {
	if err := e.init(); err != nil {
		return err
	}
	st, err := e.store.Load()
	if err != nil {
		return err
	}
	if len(st.Plugins) == 0 || len(e.Keys) == 0 {
		return nil
	}
	listRaw, err := e.fetch(ctx, ListName(e.platform()))
	if err != nil {
		return err
	}
	l, key, err := e.verify(listRaw, KindPlugins, st.ListCounter)
	if err != nil {
		return fmt.Errorf("plugin list: %w", err)
	}
	blockRaw, err := e.fetch(ctx, "blocklist.json")
	if err != nil {
		return err
	}
	block, _, err := e.verify(blockRaw, KindBlocklist, st.BlockCounter)
	if err != nil {
		return fmt.Errorf("blocklist: %w", err)
	}
	testRaw := e.testList(ctx, st, true)
	// The cache is written before the counters, so a failed write never
	// leaves the stored counters newer than the cached lists (which would
	// refuse every later start).
	if err := e.writeCache(listRaw, blockRaw, l.Counter, block.Counter); err != nil {
		return err
	}
	if err := e.store.SaveCounters(l.Counter, block.Counter, time.Now()); err != nil {
		return err
	}
	// Hold opMu and reload, so an install or start during the fetch is
	// checked against the new lists too.
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if st, err = e.store.Load(); err != nil {
		return err
	}
	for id, p := range st.Plugins {
		if !p.Enabled {
			// Off the blocklist again: the reason goes, so Start works (it is not started).
			if strings.HasPrefix(p.DisabledReason, ErrBlocked.Error()) && !blockedBy(block, p) {
				_ = e.store.SetEnabled(id, false, stoppedByOwner)
			}
			continue
		}
		if _, err := e.store.CheckStart(id, StartInput{PublicKey: key, List: listRaw, Blocklist: blockRaw, TestList: testRaw}); err != nil {
			log.Printf("plugins: %s stopped and disabled: %v", id, err)
			if serr := e.stopProc(id); serr != nil {
				log.Printf("plugins: stopping %s: %v", id, serr)
			}
		}
	}
	return nil
}

// writeCache writes the cached lists before their counters are stored. It
// skips (without error) lists older than the stored counters or the ones
// already cached, so a slower check never puts back an older list.
func (e *Engine) writeCache(listRaw, blockRaw []byte, list, block uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := e.store.Load()
	if err != nil {
		return err
	}
	if list < st.ListCounter || block < st.BlockCounter || list < e.cachedCounter(e.cacheListName(), KindPlugins) || block < e.cachedCounter(cacheBlocklist, KindBlocklist) {
		return nil
	}
	if err := writeFileSynced(filepath.Join(e.Dir, e.cacheListName()), listRaw); err != nil {
		return fmt.Errorf("caching plugin list: %w", err)
	}
	if err := writeFileSynced(filepath.Join(e.Dir, cacheBlocklist), blockRaw); err != nil {
		return fmt.Errorf("caching blocklist: %w", err)
	}
	return nil
}

// cachedCounter is the counter of a cached list, 0 when none or unreadable.
func (e *Engine) cachedCounter(name, kind string) uint64 {
	raw, err := os.ReadFile(filepath.Join(e.Dir, name))
	if err != nil {
		return 0
	}
	l, _, err := e.verify(raw, kind, 0)
	if err != nil {
		return 0
	}
	return l.Counter
}

// testList is the test-version list: fetched again when fresh is true or
// none is cached, and cached when it verifies; else the cached one. nil
// when there is none (most lists have no test versions).
func (e *Engine) testList(ctx context.Context, st *State, fresh bool) []byte {
	path := filepath.Join(e.Dir, e.cacheTestListName())
	cached, err := os.ReadFile(path)
	if err == nil && !fresh {
		return cached
	}
	if len(e.Keys) == 0 {
		return cached
	}
	raw, ferr := e.fetch(ctx, TestListName(e.platform()))
	if ferr != nil {
		return cached
	}
	if _, _, verr := e.verify(raw, KindTest, st.TestCounter); verr != nil {
		log.Printf("plugins: test-version list check failed: %v", verr)
		return cached
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if cached != nil && e.cachedCounter(e.cacheTestListName(), KindTest) > 0 {
		if l, _, err := e.verify(raw, KindTest, e.cachedCounter(e.cacheTestListName(), KindTest)); err != nil || l == nil {
			return cached // never put back an older list
		}
	}
	_ = writeFileSynced(path, raw)
	return raw
}

// verify checks raw with each pinned key and returns the key that signed it.
func (e *Engine) verify(raw []byte, kind string, last uint64) (*List, ed25519.PublicKey, error) {
	if len(e.Keys) == 0 {
		return nil, nil, ErrNoKey
	}
	var first error
	for _, k := range e.Keys {
		l, err := VerifyList(k, raw, kind, last)
		if err == nil {
			return l, k, nil
		}
		if first == nil || !errors.Is(err, ErrListSignature) {
			first = err
		}
	}
	return nil, nil, first
}

// blockFloor is the newest blocklist counter the plugin engine has seen,
// stored or cached, so the language packs never accept an older one.
func (e *Engine) blockFloor() uint64 {
	if e.init() != nil {
		return 0
	}
	var n uint64
	if st, err := e.store.Load(); err == nil {
		n = st.BlockCounter
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return max(n, e.cachedCounter(cacheBlocklist, KindBlocklist))
}

// blockedBy says whether the blocklist holds p's installed version.
func blockedBy(block *List, p *Installed) bool {
	m := &Manifest{ID: p.ID, Version: p.Version, SHA256: p.SHA256}
	for _, en := range block.Entries {
		if en.blocks(m) {
			return true
		}
	}
	return false
}

// startInput reads the cached lists and picks the key they verify with.
// Called with e.mu held.
func (e *Engine) startInput() (StartInput, error) {
	list, err1 := os.ReadFile(filepath.Join(e.Dir, e.cacheListName()))
	block, err2 := os.ReadFile(filepath.Join(e.Dir, cacheBlocklist))
	if err := errors.Join(err1, err2); err != nil {
		return StartInput{}, fmt.Errorf("cached plugin lists: %w", err)
	}
	st, err := e.store.Load()
	if err != nil {
		return StartInput{}, err
	}
	if _, _, err := e.verify(block, KindBlocklist, st.BlockCounter); err != nil {
		return StartInput{}, fmt.Errorf("cached blocklist: %w", err)
	}
	_, key, err := e.verify(list, KindPlugins, st.ListCounter)
	if err != nil {
		key = nil
		if len(e.Keys) > 0 {
			key = e.Keys[0] // CheckStart refuses and disables with the reason
		}
	}
	if key == nil {
		return StartInput{}, ErrNoKey
	}
	test, _ := os.ReadFile(filepath.Join(e.Dir, e.cacheTestListName()))
	return StartInput{PublicKey: key, List: list, Blocklist: block, TestList: test}, nil
}
